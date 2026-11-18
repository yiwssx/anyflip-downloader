#!/usr/bin/env bash
# Fix the *owner-controlled* AnyFlip quota startup at its source.
# Install a reversible, Svelte-pinned drop-in. Does NOT restart a container.
set -Eeuo pipefail
fail() { printf 'QUOTA_SVELTE_STARTUP=BLOCKED reason=%s\n' "$*" >&2; exit 1; }
[[ "$EUID" == 0 ]] || fail "requires-root"
[[ "$#" -eq 1 ]] || fail "usage: $0 /opt/anyflip-downloader"
host="$(cd "$1" && pwd -P)"
[[ "$host" == /opt/anyflip-downloader ]] || fail "unexpected-host-path"
src="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd -P)"
[[ "$src" != "$host" ]] || fail "use-detached-verified-release"
state=/etc/rcat-anyflip-svelte
unit=/etc/systemd/system/anyflip-quota.service
dropdir=/etc/systemd/system/anyflip-quota.service.d
dropin="$dropdir/90-svelte-pinned.conf"
for cmd in docker systemctl git python3 install findmnt mountpoint; do
  command -v "$cmd" >/dev/null 2>&1 || fail "missing-$cmd"
done
docker compose version >/dev/null 2>&1 || fail "compose-v2-required"
[[ -f "$state/active.env" && -f "$state/compose.pinned.yaml" ]] ||
  fail "approved-pinned-metadata-absent"
# This metadata is a root-owned local release receipt, not a GitHub secret.
# shellcheck disable=SC1091
source "$state/active.env"
[[ "${BOOT_HOST:-}" == "$host" ]] || fail "host-identity-mismatch"
[[ "${BOOT_PROJECT:-}" =~ ^[a-zA-Z0-9][a-zA-Z0-9_.-]*$ ]] ||
  fail "invalid-compose-project"
[[ "${BOOT_IMAGE_ID:-}" =~ ^sha256:[a-f0-9]{64}$ ]] || fail "invalid-image-id"
[[ "${BOOT_SOURCE_SHA:-}" =~ ^[a-f0-9]{40}$ ]] || fail "invalid-source-sha"
[[ "${BOOT_IMAGE_TAG:-}" =~ ^rcat-anyflip-svelte:boot-[a-f0-9]{12}-[a-f0-9]{12}$ ]] ||
  fail "invalid-pinned-image-tag"
case "${BOOT_PORT:-}" in 10.10.1.100:8094|127.0.0.1:8094) ;; *) fail "unexpected-port";; esac
[[ -n "${BOOT_MOUNTS:-}" && "$BOOT_MOUNTS" == *"/data/jobs"* ]] ||
  fail "pinned-quota-volume-missing"
[[ -f "$host/.env" && -f "$host/compose.web.yaml" &&
   -f "$host/compose.quota.yaml" ]] || fail "host-config-missing"
[[ -f "$unit" ]] || fail "quota-unit-not-locally-owned"
[[ "$(systemctl show -P FragmentPath anyflip-quota.service)" == "$unit" ]] ||
  fail "quota-unit-not-expected-fragment"
[[ "$(systemctl is-enabled anyflip-quota.service)" == enabled ]] ||
  fail "quota-unit-not-enabled"
[[ "$(systemctl is-enabled rcat-anyflip-svelte-boot.service || true)" == disabled ]] ||
  fail "competing-boot-recovery-must-be-disabled"
systemctl is-active --quiet docker.service || fail "docker-daemon-inactive"
systemctl is-active --quiet anyflip-quota.service || fail "quota-unit-not-running"
mountpoint -q /mnt/anyflip-quota-stage || fail "hard-quota-not-mounted"
[[ "$(findmnt -M /mnt/anyflip-quota-stage -n -o FSTYPE)" == ext4 ]] ||
  fail "quota-filesystem-not-ext4"

# Validate the exact operator-provided unit structure. Do not overwrite the
# unit, quota-ready hook or its mount dependencies. Reject unknown overrides.
for line in \
  'WorkingDirectory=/opt/anyflip-downloader' \
  'ExecStartPre=/usr/local/sbin/anyflip-quota-ready' \
  'ExecStart=/usr/bin/docker compose -f compose.web.yaml -f compose.quota.yaml up --no-build --no-color anyflip-web' \
  'ExecStop=/usr/bin/docker compose -f compose.web.yaml -f compose.quota.yaml stop anyflip-web' \
  'BindsTo=mnt-anyflip\x2dquota\x2dstage.mount' \
  'RequiresMountsFor=/mnt/anyflip-quota-stage'; do
  grep -Fqx "$line" "$unit" || fail "unexpected-host-unit-contract"
done
other_dropins="$(systemctl show -P DropInPaths anyflip-quota.service)"
if [[ -n "$other_dropins" && "$other_dropins" != "$dropin" ]]; then
  fail "unknown-systemd-overrides"
fi

# No host-local code or secrets leave this process.
[[ -z "$(git -C "$src" status --porcelain --untracked-files=no -- \
  scripts/install-svelte-quota-startup.sh scripts/apply-svelte-quota-startup.sh)" ]] ||
  fail "installer-source-modified"
docker image inspect "$BOOT_IMAGE_TAG" >/dev/null 2>&1 ||
  fail "pinned-image-not-available"
[[ "$(docker image inspect -f '{{.Id}}' "$BOOT_IMAGE_TAG")" == "$BOOT_IMAGE_ID" ]] ||
  fail "pinned-image-id-drift"
[[ "$(docker image inspect -f '{{index .Config.Labels "org.opencontainers.image.revision"}}' "$BOOT_IMAGE_TAG")" == "$BOOT_SOURCE_SHA" ]] ||
  fail "pinned-source-revision-drift"

mapfile -t ids < <(docker ps -aq --filter "label=com.docker.compose.project=$BOOT_PROJECT" --filter "label=com.docker.compose.service=anyflip-web")
[[ "${#ids[@]}" -eq 1 ]] || fail "expected-exactly-one-production-container-no-stale-recreate"
cid="${ids[0]}"
[[ "$(docker inspect -f '{{.State.Running}}' "$cid")" == true ]] ||
  fail "web-container-not-running"
[[ "$(docker inspect -f '{{range .Config.Env}}{{if eq . "ANYFLIP_AUTH_MODE=google"}}yes{{end}}{{end}}' "$cid")" == yes ]] ||
  fail "google-workspace-auth-not-active"
[[ "$(docker inspect -f '{{range .Mounts}}{{.Type}}|{{.Name}}|{{.Source}}|{{.Destination}};{{end}}' "$cid")" == "$BOOT_MOUNTS" ]] ||
  fail "data-mount-changed"
[[ "$(docker inspect -f '{{range (index .HostConfig.PortBindings "8080/tcp")}}{{.HostIp}}:{{.HostPort}}{{end}}' "$cid")" == "$BOOT_PORT" ]] ||
  fail "lan-binding-changed"
[[ "$(docker inspect -f '{{.Image}}' "$cid")" == "$BOOT_IMAGE_ID" ]] ||
  fail "unexpected-current-image-requires-investigation"
# A healthy legacy-mode container can use the SAME immutable Svelte image,
# but without ANYFLIP_WEB_UI=svelte it falls back to the old Go HTML.
[[ "$(docker inspect -f '{{if .State.Health}}{{.State.Health.Status}}{{end}}' "$cid")" == healthy ]] ||
  fail "current-container-unhealthy"

export RCAT_SVELTE_IMAGE="$BOOT_IMAGE_TAG"
# Compose config is potentially secret-bearing; send it only to Python.
docker compose --project-directory "$host" -p "$BOOT_PROJECT" \
  --env-file "$host/.env" \
  -f "$host/compose.web.yaml" -f "$host/compose.quota.yaml" \
  -f "$state/compose.pinned.yaml" config --format json |
python3 -c '
import json,sys
image,port = sys.argv[1:]
svc=json.load(sys.stdin)["services"]["anyflip-web"]
env=svc["environment"]
assert svc.get("image")==image, "not approved pinned image"
assert not svc.get("build"), "legacy Dockerfile build is enabled"
assert env.get("ANYFLIP_WEB_UI")=="svelte", "Svelte mode missing"
assert env.get("ANYFLIP_AUTH_MODE")=="google", "Google auth not preserved"
assert svc.get("restart") in ("no","none",None), "must retain controlled quota boot"
assert any(v.get("target")=="/data/jobs" for v in svc.get("volumes",[])), "jobs volume missing"
assert any(str(p.get("published"))=="8094" and
  str(p.get("host_ip",""))==port.rsplit(":",1)[0] and
  int(p.get("target"))==8080 for p in svc.get("ports",[])), "LAN binding changed"
print("QUOTA_SVELTE_COMPOSE=PASS")
' "$BOOT_IMAGE_TAG" "$BOOT_PORT" || fail "merged-compose-rejected"

# No changes to the original quota unit, mount, image, container or .env.
# systemd percent expansion is not involved: tags/project are allowlisted.
tmp="$(mktemp)"
trap 'rm -f -- "$tmp"' EXIT
cat > "$tmp" <<EOF
# Managed by RCAT AnyFlip: approved Svelte mode in the ORIGINAL quota unit.
# Keep quota-ready ExecStartPre, RequiresMountsFor and BindsTo unchanged.
[Service]
Environment=RCAT_SVELTE_IMAGE=$BOOT_IMAGE_TAG
ExecStart=
ExecStart=/usr/bin/docker compose --project-directory $host -p $BOOT_PROJECT --env-file $host/.env -f $host/compose.web.yaml -f $host/compose.quota.yaml -f $state/compose.pinned.yaml up --no-build --no-color anyflip-web
ExecStop=
ExecStop=/usr/bin/docker compose --project-directory $host -p $BOOT_PROJECT --env-file $host/.env -f $host/compose.web.yaml -f $host/compose.quota.yaml -f $state/compose.pinned.yaml stop anyflip-web
EOF
if [[ -e "$dropin" ]] && ! cmp -s "$tmp" "$dropin"; then
  fail "existing-svelte-dropin-differs-refuse-overwrite"
fi
install -d -m 0755 "$dropdir"
candidate="$dropdir/.90-svelte-pinned.conf.$"
install -m 0644 "$tmp" "$candidate"
mv -T -- "$candidate" "$dropin"
systemctl daemon-reload

# Fail closed: BOTH start and stop must refer to the same pinned overlay.
effective="$(systemctl show -P ExecStart anyflip-quota.service)"
stop_effective="$(systemctl show -P ExecStop anyflip-quota.service)"
environment_effective="$(systemctl show -P Environment anyflip-quota.service)"
[[ "$effective" == *"$state/compose.pinned.yaml"* &&
   "$effective" == *"up --no-build --no-color anyflip-web"* ]] ||
  fail "effective-systemd-start-command-mismatch"
[[ "$stop_effective" == *"$state/compose.pinned.yaml"* &&
   "$stop_effective" == *"stop anyflip-web"* ]] ||
  fail "effective-systemd-stop-command-mismatch"
[[ " $environment_effective " == *" RCAT_SVELTE_IMAGE=$BOOT_IMAGE_TAG "* ]] ||
  fail "pinned-image-not-exported-by-systemd"
[[ "$(systemctl is-enabled rcat-anyflip-svelte-boot.service || true)" == disabled ]] ||
  fail "boot-competitor-reenabled"
echo "QUOTA_SVELTE_STARTUP=CONFIGURED"
echo "QUOTA_SVELTE_IMAGE=$BOOT_IMAGE_ID"
echo "QUOTA_SVELTE_PROJECT=$BOOT_PROJECT"
echo "QUOTA_SVELTE_BOOT_SOURCE=original-quota-service"
echo "DOCKER_CONTAINERS_CHANGED=no"
echo "APPLY=OPERATOR_CONTROLLED_SINGLE_SERVICE_RESTART_PENDING"
