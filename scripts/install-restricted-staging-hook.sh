#!/usr/bin/env bash
# Install only the fixed local AnyFlip staging hook; no GitHub listener or sudo grant.
# Requires explicit host operator approval and a reviewed immutable source ref.
set -Eeuo pipefail
fail() { echo "REFUSED: $*" >&2; exit 2; }
[[ $EUID -eq 0 ]] || fail "installer must run as root"
[[ ${1:-} == "--approve-restricted-staging-hook" && $# -eq 3 ]] ||
  fail "usage: install-restricted-staging-hook.sh --approve-restricted-staging-hook ABSOLUTE_ANYFLIP_SOURCE_DIR REVIEWED_GIT_SHA"
source_dir=$2
reviewed_sha=$3
[[ $source_dir == /* && ! -L $source_dir ]] || fail "absolute nonsymlinked source directory required"
[[ $reviewed_sha =~ ^[a-f0-9]{40}$ ]] || fail "reviewed exact Git SHA required"
[[ -d $source_dir/.git ]] || fail "missing Git checkout"
[[ $(git -C "$source_dir" rev-parse HEAD) == "$reviewed_sha" ]] || fail "source checkout not at approved SHA"
[[ $(git -C "$source_dir" branch --show-current) == main ]] || fail "source checkout must be on main"
[[ -z $(git -C "$source_dir" status --porcelain --untracked-files=no) ]] ||
  fail "modified tracked source refused"
case "$(git -C "$source_dir" remote get-url origin)" in
  https://github.com/yiwssx/anyflip-downloader|https://github.com/yiwssx/anyflip-downloader.git|git@github.com:yiwssx/anyflip-downloader.git) ;;
  *) fail "unapproved source remote" ;;
esac
hook_src="$source_dir/deploy/staging/anyflip-fixed-hook.sh"
compose_src="$source_dir/deploy/staging/compose.yaml"
[[ -f $hook_src && ! -L $hook_src && -f $compose_src && ! -L $compose_src ]] ||
  fail "reviewed staging files missing or symlinked"
[[ -f /etc/rcat-staging/anyflip.env && ! -L /etc/rcat-staging/anyflip.env ]] ||
  fail "existing private staging token missing"
[[ $(stat -c '%u %a' /etc/rcat-staging) == "0 700" ]] ||
  fail "staging private directory must be root 0700"
[[ $(stat -c '%u %a' /etc/rcat-staging/anyflip.env) == "0 600" ]] ||
  fail "staging token must be root 0600"
[[ ! -e /etc/rcat-staging/anyflip-approved-image && ! -L /etc/rcat-staging/anyflip-approved-image ]] ||
  fail "stage approval lock already exists; refusal to overwrite"
[[ ! -e /usr/local/libexec/rcat-staging/anyflip-downloader-stage && ! -L /usr/local/libexec/rcat-staging/anyflip-downloader-stage ]] ||
  fail "stage hook already installed; refusal to overwrite"
[[ ! -e /usr/local/libexec/rcat-staging/anyflip-compose.yaml && ! -L /usr/local/libexec/rcat-staging/anyflip-compose.yaml ]] ||
  fail "stage compose already installed; refusal to overwrite"

# Reuse the already successful signed-candidate/host activation, not a new build.
source_sha=bc72fe9c959ca91c61557b41712ce0ed5b9e1c79
image=rcat-anyflip-staging:sha-$source_sha
command -v docker >/dev/null 2>&1 || fail "Docker unavailable"
container=$(docker ps -q --filter label=com.docker.compose.project=rcat-anyflip-staging \
  --filter label=com.docker.compose.service=anyflip-web)
[[ -n "$container" ]] || fail "approved staging container is not running"
[[ $(docker ps -aq --filter label=com.docker.compose.project=rcat-anyflip-staging) == "$container" ]] ||
  fail "unexpected other staging containers"
image_id=$(docker image inspect --format '{{.Id}}' "$image") || fail "approved local image unavailable"
[[ $image_id =~ ^sha256:[a-f0-9]{64}$ ]] || fail "invalid local image ID"
[[ $(docker inspect --format '{{.Image}}' "$container") == "$image_id" ]] ||
  fail "running stage uses an unexpected image"
ports=$(docker inspect --format '{{range $port, $hosts := .NetworkSettings.Ports}}{{if eq $port "8080/tcp"}}{{range $hosts}}{{.HostIp}}:{{.HostPort}}{{end}}{{end}}{{end}}' "$container")
[[ "$ports" == "127.0.0.1:18094" ]] ||
  fail "running stage is not loopback-only on the reviewed port"
[[ $(docker image inspect --format '{{index .Config.Labels "org.opencontainers.image.revision"}}' "$image") == "$source_sha" ]] ||
  fail "running stage revision label mismatch"
[[ $(docker volume inspect --format '{{index .Labels "com.docker.compose.project"}}' rcat-anyflip-staging-jobs) == rcat-anyflip-staging ]] ||
  fail "staging volume not owned by approved project"
[[ $(curl -fsS --max-time 5 http://127.0.0.1:18094/healthz) == ok ]] ||
  fail "existing stage health must be OK before bootstrap"
[[ $(curl -sS --max-time 5 -o /dev/null -w '%{http_code}' \
    http://127.0.0.1:18094/api/jobs/00000000000000000000000000000000) == 401 ]] ||
  fail "staging auth policy unverified"

# No change to rcat-ops account/service, to Docker groups, or to running stage.
if ! id -u rcat-stage-exec >/dev/null 2>&1; then
  useradd --system --user-group --home-dir /var/lib/rcat-stage-exec \
    --shell /usr/sbin/nologin rcat-stage-exec
fi
if id -nG rcat-stage-exec | tr ' ' '\n' | grep -Eq '^(sudo|docker)$'; then
  fail "dedicated staging account must never join sudo/docker"
fi
if [[ -e /usr/local/libexec/rcat-staging ]]; then
  [[ -d /usr/local/libexec/rcat-staging && ! -L /usr/local/libexec/rcat-staging &&
     $(stat -c '%u' /usr/local/libexec/rcat-staging) == 0 &&
     $(stat -c '%a' /usr/local/libexec/rcat-staging) == 755 ]] ||
    fail "staging hook directory is not root-owned mode 0755"
else
  install -d -o root -g root -m 0755 /usr/local/libexec/rcat-staging
fi
install -o root -g root -m 0755 "$hook_src" /usr/local/libexec/rcat-staging/anyflip-downloader-stage
install -o root -g root -m 0644 "$compose_src" /usr/local/libexec/rcat-staging/anyflip-compose.yaml
umask 077
tmp_lock=$(mktemp /etc/rcat-staging/.anyflip-approved-image.XXXXXXXX)
trap 'rm -f -- "$tmp_lock"' EXIT
printf '%s %s %s\n' "$source_sha" "$image_id" "$image" > "$tmp_lock"
chmod 0600 "$tmp_lock"
chown root:root "$tmp_lock"
# Atomic no-clobber creation: never overwrite an independently approved lock.
ln -- "$tmp_lock" /etc/rcat-staging/anyflip-approved-image ||
  fail "another staging image lock was installed concurrently"
rm -f -- "$tmp_lock"

echo "RESTRICTED_STAGE_HOOK_INSTALLED"
echo "Reviewed AnyFlip code SHA: $reviewed_sha"
echo "Pinned stage source SHA: $source_sha"
echo "Pinned local image ID: $image_id"
echo "Existing staging container unchanged; no sudo grant, GitHub listener or remote execution enabled."
echo "Next gate: separate GitHub executor identity, authenticated short-lived approval and rollback acceptance."
