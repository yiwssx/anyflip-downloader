#!/usr/bin/env bash
# Once per approved Host: pin the *currently running* Svelte image for boot.
# No container recreation, no image build/pull, no quota/DNS/Tunnel changes.
set -Eeuo pipefail

die() { printf 'SVELTE_BOOT_INSTALL=BLOCKED reason=%s\n' "$*" >&2; exit 1; }
[[ "${EUID}" -eq 0 ]] || die "run-with-sudo"
[[ "$#" -ge 1 && "$#" -le 2 ]] || die "usage: $0 /opt/anyflip-downloader [--re-pin]"
host="$(cd "$1" && pwd -P)"
[[ "$host" == "/opt/anyflip-downloader" ]] || die "unexpected-host-path"
[[ "$#" -eq 1 || "$2" == --re-pin ]] || die "invalid-option"
repin=0
[[ "$#" -eq 1 ]] || repin=1
src="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd -P)"
[[ "$src" != "$host" ]] || die "use-approved-detached-release-worktree"
for exe in docker git systemctl curl python3 install; do
  command -v "$exe" >/dev/null 2>&1 || die "missing-$exe"
done
docker compose version >/dev/null 2>&1 || die "compose-v2-required"
[[ -r "$host/.env" && -f "$host/compose.web.yaml" && -f "$host/compose.quota.yaml" ]] ||
  die "existing-host-env-or-quota-overlay-missing"
systemctl is-active --quiet anyflip-quota.service ||
  die "hard-quota-service-not-active"
[[ -f "$src/scripts/reconcile-svelte-boot.sh" &&
   -f "$src/deploy/systemd/rcat-anyflip-svelte-boot.service" &&
   -f "$src/scripts/verify-svelte-ui.sh" ]] || die "release-files-missing"
[[ -z "$(git -C "$src" status --porcelain --untracked-files=no -- scripts/reconcile-svelte-boot.sh scripts/install-svelte-boot.sh deploy/systemd/rcat-anyflip-svelte-boot.service scripts/verify-svelte-ui.sh)" ]] ||
  die "unreviewed-boot-script-modifications"

mapfile -t ids < <(docker ps -q --filter label=com.docker.compose.service=anyflip-web)
[[ "${#ids[@]}" -eq 1 ]] || die "expected-one-running-anyflip-service"
cid="${ids[0]}"
project="$(docker inspect --format '{{index .Config.Labels "com.docker.compose.project"}}' "$cid")"
[[ "$project" =~ ^[A-Za-z0-9][A-Za-z0-9_.-]*$ ]] || die "missing-project-identity"
mode="$(docker inspect --format '{{range .Config.Env}}{{if eq . "ANYFLIP_WEB_UI=svelte"}}yes{{end}}{{end}}' "$cid")"
auth="$(docker inspect --format '{{range .Config.Env}}{{if eq . "ANYFLIP_AUTH_MODE=google"}}yes{{end}}{{end}}' "$cid")"
[[ "$mode" == yes && "$auth" == yes ]] || die "current-container-not-svelte-with-google"
img="$(docker inspect --format '{{.Image}}' "$cid")"
[[ "$img" =~ ^sha256:[a-f0-9]{64}$ ]] || die "no-immutable-image-id"
revision="$(docker image inspect --format '{{index .Config.Labels "org.opencontainers.image.revision"}}' "$img")"
[[ "$revision" =~ ^[a-f0-9]{40}$ ]] || die "image-does-not-have-verified-source"
source="$(docker image inspect --format '{{index .Config.Labels "org.opencontainers.image.source"}}' "$img")"
[[ "$source" == "https://github.com/yiwssx/anyflip-downloader" ]] ||
  die "unexpected-image-provenance"
mounts="$(docker inspect --format '{{range .Mounts}}{{.Type}}|{{.Name}}|{{.Source}}|{{.Destination}};{{end}}' "$cid")"
[[ -n "$mounts" && "$mounts" == *"/data/jobs"* ]] || die "job-quota-mount-missing"
port="$(docker inspect --format '{{range (index .HostConfig.PortBindings "8080/tcp")}}{{.HostIp}}:{{.HostPort}}{{end}}' "$cid")"
case "$port" in
  10.10.1.100:8094|127.0.0.1:8094) ;;
  *) die "nonapproved-port" ;;
esac

# Verify active Svelte HTML/assets + Google auth and anonymous denials before
# writing boot state. No login secret or real book needed.
BASE_URL="http://$port" CANONICAL_HOST=anyflip.rcat.ac.th \
  sh "$src/scripts/verify-svelte-ui.sh" >/dev/null ||
  die "running-svelte-origin-failed-smoke"

state="/etc/rcat-anyflip-svelte"
unit="/etc/systemd/system/rcat-anyflip-svelte-boot.service"
unit_source="$src/deploy/systemd/rcat-anyflip-svelte-boot.service"
if [[ -e "$unit" ]] && ! cmp -s "$unit" "$unit_source"; then
  die "existing-boot-unit-was-modified-refuse-overwrite"
fi
if [[ -e "$state/active.env" && "$repin" != 1 ]]; then
  die "boot-release-exists-use-explicit---re-pin-to-change"
fi
if [[ -e "$state/compose.pinned.yaml" && "$repin" != 1 ]]; then
  die "boot-compose-exists-use-explicit---re-pin"
fi
if [[ -e "$state/active.env" ]] && ! grep -q '^# Managed by RCAT AnyFlip' "$state/active.env"; then
  die "unrecognized-existing-release-state"
fi
if [[ -e "$state/compose.pinned.yaml" ]] &&
  ! grep -q '^# Managed by RCAT AnyFlip' "$state/compose.pinned.yaml"; then
  die "unrecognized-existing-compose-state"
fi

tag="rcat-anyflip-svelte:boot-${revision:0:12}-${img:7:12}"
if docker image inspect "$tag" >/dev/null 2>&1; then
  [[ "$(docker image inspect --format '{{.Id}}' "$tag")" == "$img" ]] ||
    die "pinned-tag-already-points-to-different-image"
else
  docker image tag "$img" "$tag" || die "cannot-preserve-image-tag"
fi

# Create a private, host-local, release-pinned overlay. Base, quota, .env,
# Google secrets, original release worktrees and volumes are NEVER modified.
install -d -m 0700 "$state"
tmp="$(mktemp -d "$state/.install.XXXXXX")"
chmod 700 "$tmp"
trap 'rm -rf -- "$tmp"' EXIT
{
  echo "# Managed by RCAT AnyFlip Svelte boot installer (no secrets)"
  printf 'BOOT_HOST=%q\n' "$host"
  printf 'BOOT_PROJECT=%q\n' "$project"
  printf 'BOOT_IMAGE_ID=%q\n' "$img"
  printf 'BOOT_IMAGE_TAG=%q\n' "$tag"
  printf 'BOOT_SOURCE_SHA=%q\n' "$revision"
  printf 'BOOT_MOUNTS=%q\n' "$mounts"
  printf 'BOOT_PORT=%q\n' "$port"
} > "$tmp/active.env"
cat > "$tmp/compose.pinned.yaml" <<EOF
# Managed by RCAT AnyFlip — pinned image, never rebuild on boot.
services:
  anyflip-web:
    image: "$tag"
    build: null
    restart: "no"
    environment:
      ANYFLIP_WEB_UI: svelte
EOF
chmod 600 "$tmp/active.env" "$tmp/compose.pinned.yaml"

# Verify merged config without printing its Google OAuth secrets.
docker compose --project-directory "$host" -p "$project" \
  --env-file "$host/.env" \
  -f "$host/compose.web.yaml" -f "$host/compose.quota.yaml" \
  -f "$tmp/compose.pinned.yaml" config --format json |
  python3 -c '
import json,sys
svc=json.load(sys.stdin)["services"]["anyflip-web"]
assert svc["image"]==sys.argv[1], "image changed"
assert svc["environment"]["ANYFLIP_WEB_UI"]=="svelte", "mode changed"
assert svc["environment"]["ANYFLIP_AUTH_MODE"]=="google", "Google auth changed"
assert not svc.get("build"), "boot would build instead of using image"
assert svc.get("restart") in ("no", "none", None), "must not start before hard quota mount"
' "$tag" || die "merged-compose-check-failed"

install -m 0600 "$tmp/active.env" "$state/active.env"
install -m 0600 "$tmp/compose.pinned.yaml" "$state/compose.pinned.yaml"
install -d -m 0755 /usr/local/libexec
install -m 0755 "$src/scripts/reconcile-svelte-boot.sh" /usr/local/libexec/rcat-anyflip-svelte-boot.sh
install -m 0755 "$src/scripts/verify-svelte-ui.sh" /usr/local/libexec/rcat-anyflip-verify-svelte.sh
install -m 0644 "$unit_source" "$unit"
systemctl daemon-reload
systemctl enable rcat-anyflip-svelte-boot.service >/dev/null

# Docker's early autorestart could run BEFORE the quota mount. The dedicated
# quota-ordered unit now owns boot start/recovery. No application restart now.
old_restart="$(docker inspect --format '{{.HostConfig.RestartPolicy.Name}}' "$cid")"
if [[ "$old_restart" != no ]]; then
  docker update --restart=no "$cid" >/dev/null
fi

bash /usr/local/libexec/rcat-anyflip-svelte-boot.sh check ||
  die "installed-configuration-did-not-verify"
echo "SVELTE_BOOT_INSTALL=PASS"
echo "SVELTE_BOOT_IMAGE=$img"
echo "SVELTE_BOOT_SOURCE=$revision"
echo "SVELTE_BOOT_UNIT=enabled"
echo "SVELTE_BOOT_CONTAINER_RESTARTED=no"
echo "SVELTE_BOOT_REBOOT_PROOF=PENDING_OPERATOR"
