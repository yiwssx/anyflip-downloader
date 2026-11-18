#!/usr/bin/env bash
# Controlled, owner-invoked Svelte-only IMAGE upgrade on the quota-owned Host.
# --prepare builds/tags an immutable image; --apply changes ONLY AnyFlip quota
# systemd's pinned image and does one deliberate service restart.
# Never run a second competing boot service, force-recreate or prune volumes.
set -Eeuo pipefail

fail() { printf 'SVELTE_ONLY_UPGRADE=BLOCKED reason=%s\n' "$*" >&2; exit 1; }
[[ "$EUID" -eq 0 ]] || fail "root-required"
[[ "$#" -ge 2 ]] || fail "usage: $0 /opt/anyflip-downloader --prepare|--apply --jobs-idle"
host="$(cd "$1" && pwd -P)"
[[ "$host" == /opt/anyflip-downloader ]] || fail "unexpected-host"
phase="$2"
[[ "$phase" == --prepare || "$phase" == --apply ]] || fail "invalid-phase"
if [[ "$phase" == --prepare ]]; then
  [[ "$#" -eq 2 ]] || fail "prepare-takes-no-additional-arguments"
else
  [[ "$#" -eq 3 && "$3" == --jobs-idle ]] ||
    fail "apply-requires-explicit---jobs-idle"
fi

src="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd -P)"
[[ "$src" != "$host" ]] || fail "requires-clean-detached-release-worktree"
revision="$(git -C "$src" rev-parse --verify HEAD)"
[[ "$revision" =~ ^[a-f0-9]{40}$ ]] || fail "invalid-release-revision"
[[ -z "$(git -C "$src" status --porcelain --untracked-files=no)" ]] ||
  fail "release-worktree-has-tracked-modifications"
[[ -f "$src/Dockerfile.svelte" && -f "$src/scripts/verify-svelte-ui.sh" ]] ||
  fail "release-build-files-missing"
grep -Fq 'go build -tags=svelte_only' "$src/Dockerfile.svelte" ||
  fail "release-not-svelte-only"
for cmd in docker systemctl python3 git mountpoint mktemp install; do
  command -v "$cmd" >/dev/null 2>&1 || fail "missing-$cmd"
done
docker compose version >/dev/null 2>&1 || fail "compose-v2-missing"

state=/etc/rcat-anyflip-svelte
active="$state/active.env"
pinned="$state/compose.pinned.yaml"
dropin=/etc/systemd/system/anyflip-quota.service.d/90-svelte-pinned.conf
receipt="$state/candidate-${revision:0:12}.env"
[[ -f "$active" && -f "$pinned" && -f "$dropin" ]] ||
  fail "quota-owned-pinned-startup-not-installed"
[[ "$(stat -c %a "$active")" == 600 ]] || fail "pinned-state-permissions-unexpected"
# shellcheck disable=SC1091
source "$active"
[[ "${BOOT_HOST:-}" == "$host" ]] || fail "boot-host-mismatch"
[[ "${BOOT_PROJECT:-}" =~ ^[A-Za-z0-9][A-Za-z0-9_.-]*$ ]] || fail "boot-project-unexpected"
[[ "${BOOT_IMAGE_ID:-}" =~ ^sha256:[a-f0-9]{64}$ ]] || fail "boot-image-id-invalid"
[[ "${BOOT_IMAGE_TAG:-}" =~ ^rcat-anyflip-svelte:boot-[a-f0-9]{12}-[a-f0-9]{12}$ ]] || fail "boot-image-tag-invalid"
[[ "${BOOT_SOURCE_SHA:-}" =~ ^[a-f0-9]{40}$ ]] || fail "boot-source-invalid"
[[ "${BOOT_PORT:-}" == 10.10.1.100:8094 || "${BOOT_PORT:-}" == 127.0.0.1:8094 ]] ||
  fail "unexpected-LAN-port"
[[ -n "${BOOT_MOUNTS:-}" && "$BOOT_MOUNTS" == *"/data/jobs"* ]] ||
  fail "jobs-storage-not-pinned"
[[ "$(systemctl is-enabled rcat-anyflip-svelte-boot.service || true)" == disabled ]] ||
  fail "competing-boot-recovery-not-disabled"
[[ "$(systemctl is-enabled anyflip-quota.service)" == enabled ]] ||
  fail "quota-unit-must-be-enabled"
systemctl is-active --quiet docker.service || fail "docker-inactive"
systemctl is-active --quiet anyflip-quota.service || fail "quota-unit-not-active"
mountpoint -q /mnt/anyflip-quota-stage || fail "quota-mount-missing"
[[ -r "$host/.env" && -f "$host/compose.web.yaml" && -f "$host/compose.quota.yaml" ]] ||
  fail "host-compose-missing"
[[ "$(docker image inspect -f '{{.Id}}' "$BOOT_IMAGE_TAG")" == "$BOOT_IMAGE_ID" ]] ||
  fail "previous-pinned-image-mismatch"
[[ "$(docker image inspect -f '{{index .Config.Labels "org.opencontainers.image.revision"}}' "$BOOT_IMAGE_TAG")" == "$BOOT_SOURCE_SHA" ]] ||
  fail "previous-image-revision-mismatch"

# Refuse to mutate unreviewed host-managed unit changes.
[[ "$(grep -Fxc "Environment=RCAT_SVELTE_IMAGE=$BOOT_IMAGE_TAG" "$dropin")" == 1 ]] ||
  fail "systemd-pinned-tag-and-state-differ"
[[ "$(systemctl show -P DropInPaths anyflip-quota.service)" == "$dropin" ]] ||
  fail "unrecognized-quota-drop-ins"
effective="$(systemctl show -P ExecStart anyflip-quota.service)"
[[ "$effective" == *"$pinned"* && "$effective" == *"up --no-build --no-color anyflip-web"* ]] ||
  fail "quota-start-command-not-pinned"

mapfile -t ids < <(docker ps -aq --filter "label=com.docker.compose.project=$BOOT_PROJECT" --filter "label=com.docker.compose.service=anyflip-web")
[[ "${#ids[@]}" -eq 1 ]] || fail "must-have-exactly-one-prod-container"
cid="${ids[0]}"
[[ "$(docker inspect -f '{{.State.Running}}' "$cid")" == true ]] ||
  fail "app-not-running"
[[ "$(docker inspect -f '{{.Image}}' "$cid")" == "$BOOT_IMAGE_ID" ]] ||
  fail "running-image-not-current-pinned"
[[ "$(docker inspect -f '{{if .State.Health}}{{.State.Health.Status}}{{end}}' "$cid")" == healthy ]] ||
  fail "current-app-not-healthy"
[[ "$(docker inspect -f '{{range .Config.Env}}{{if eq . "ANYFLIP_AUTH_MODE=google"}}yes{{end}}{{end}}' "$cid")" == yes ]] ||
  fail "google-auth-not-active"
[[ "$(docker inspect -f '{{range .Config.Env}}{{if eq . "ANYFLIP_WEB_UI=svelte"}}yes{{end}}{{end}}' "$cid")" == yes ]] ||
  fail "current-app-not-svelte"
[[ "$(docker inspect -f '{{range .Mounts}}{{.Type}}|{{.Name}}|{{.Source}}|{{.Destination}};{{end}}' "$cid")" == "$BOOT_MOUNTS" ]] ||
  fail "job-mounts-mismatch"
[[ "$(docker inspect -f '{{range (index .HostConfig.PortBindings "8080/tcp")}}{{.HostIp}}:{{.HostPort}}{{end}}' "$cid")" == "$BOOT_PORT" ]] ||
  fail "public-bind-changed"

if [[ "$phase" == --prepare ]]; then
  [[ ! -e "$receipt" ]] || fail "candidate-exists-use-existing-receipt"
  stage="rcat-anyflip-svelte-only:candidate-${revision:0:12}"
  echo "SVELTE_ONLY_UPGRADE=BUILDING_SOURCE_PINNED_IMAGE"
  # Build only the new candidate; the live web image, Compose and quota remain untouched.
  docker build --pull=false --build-arg "SOURCE_COMMIT=$revision" \
    -f "$src/Dockerfile.svelte" -t "$stage" "$src"
  image_id="$(docker image inspect -f '{{.Id}}' "$stage")"
  [[ "$image_id" =~ ^sha256:[a-f0-9]{64}$ ]] || fail "new-image-id-invalid"
  [[ "$image_id" != "$BOOT_IMAGE_ID" ]] || fail "candidate-image-identical-to-old"
  [[ "$(docker image inspect -f '{{index .Config.Labels "org.opencontainers.image.revision"}}' "$stage")" == "$revision" ]] ||
    fail "candidate-source-not-exact"
  [[ "$(docker image inspect -f '{{index .Config.Labels "th.ac.rcat.anyflip.frontend"}}' "$stage")" == "svelte-only" ]] ||
    fail "candidate-not-svelte-only"
  new_tag="rcat-anyflip-svelte:boot-${revision:0:12}-${image_id:7:12}"
  if docker image inspect "$new_tag" >/dev/null 2>&1; then
    [[ "$(docker image inspect -f '{{.Id}}' "$new_tag")" == "$image_id" ]] ||
      fail "immutable-release-tag-already-used-by-different-image"
  else
    docker image tag "$image_id" "$new_tag"
  fi
  tmp="$(mktemp "$state/.candidate.XXXXXX")"
  chmod 600 "$tmp"
  {
    echo "# RCAT AnyFlip Svelte-only immutable image candidate (no secrets)"
    printf 'NEXT_IMAGE_ID=%q\n' "$image_id"
    printf 'NEXT_IMAGE_TAG=%q\n' "$new_tag"
    printf 'NEXT_SOURCE_SHA=%q\n' "$revision"
    printf 'PREVIOUS_IMAGE_ID=%q\n' "$BOOT_IMAGE_ID"
    printf 'PREVIOUS_SOURCE_SHA=%q\n' "$BOOT_SOURCE_SHA"
  } > "$tmp"
  mv -T "$tmp" "$receipt"
  echo "SVELTE_ONLY_PREPARE=PASS"
  echo "NEW_IMAGE=$image_id"
  echo "NEW_SOURCE=$revision"
  echo "DOCKER_CONTAINERS_CHANGED=no"
  exit 0
fi

[[ -f "$receipt" ]] || fail "candidate-receipt-missing-run---prepare"
# shellcheck disable=SC1090
source "$receipt"
[[ "${NEXT_IMAGE_ID:-}" =~ ^sha256:[a-f0-9]{64}$ ]] || fail "receipt-invalid-image"
[[ "${NEXT_SOURCE_SHA:-}" == "$revision" ]] || fail "receipt-source-mismatch"
[[ "${PREVIOUS_IMAGE_ID:-}" == "$BOOT_IMAGE_ID" ]] || fail "receipt-old-image-mismatch"
[[ "${PREVIOUS_SOURCE_SHA:-}" == "$BOOT_SOURCE_SHA" ]] || fail "receipt-old-revision-mismatch"
[[ "${NEXT_IMAGE_TAG:-}" == "rcat-anyflip-svelte:boot-${revision:0:12}-${NEXT_IMAGE_ID:7:12}" ]] ||
  fail "receipt-invalid-tag"
[[ "$(docker image inspect -f '{{.Id}}' "$NEXT_IMAGE_TAG")" == "$NEXT_IMAGE_ID" ]] ||
  fail "candidate-tag-was-replaced"
[[ "$(docker image inspect -f '{{index .Config.Labels "org.opencontainers.image.revision"}}' "$NEXT_IMAGE_TAG")" == "$revision" ]] ||
  fail "candidate-source-label-wrong"
[[ "$(docker image inspect -f '{{index .Config.Labels "th.ac.rcat.anyflip.frontend"}}' "$NEXT_IMAGE_TAG")" == svelte-only ]] ||
  fail "candidate-policy-not-svelte-only"

# Validate the merged Compose configuration without exposing OAuth secrets.
export RCAT_SVELTE_IMAGE="$NEXT_IMAGE_TAG"
docker compose --project-directory "$host" -p "$BOOT_PROJECT" --env-file "$host/.env" \
  -f "$host/compose.web.yaml" -f "$host/compose.quota.yaml" -f "$pinned" config --format json |
  python3 -c '
import json,sys
cfg=json.load(sys.stdin)["services"]["anyflip-web"]
assert cfg.get("image")==sys.argv[1]
assert not cfg.get("build"), "legacy builder not reset"
assert cfg["environment"].get("ANYFLIP_WEB_UI")=="svelte"
assert cfg["environment"].get("ANYFLIP_AUTH_MODE")=="google"
assert any(x.get("target")=="/data/jobs" for x in cfg.get("volumes", []))
assert any(str(x.get("published"))=="8094" and
           str(x.get("host_ip"))==sys.argv[2].split(":")[0] and
           int(x.get("target"))==8080 for x in cfg.get("ports", []))
' "$NEXT_IMAGE_TAG" "$BOOT_PORT" || fail "new-compose-config-rejected"

# Preserve a private, deterministic rollback receipt. Never delete volumes.
stamp="$(date -u +%Y%m%dT%H%M%SZ)"
backup="/root/anyflip-svelte-only-${revision:0:12}-$stamp"
install -d -m 0700 "$backup"
install -m 0600 "$active" "$backup/active.env"
install -m 0600 "$dropin" "$backup/90-svelte-pinned.conf"
printf '%s\n' "OLD_IMAGE=$BOOT_IMAGE_ID" "NEW_IMAGE=$NEXT_IMAGE_ID" \
  "APPROVED_SOURCE=$revision" > "$backup/release.txt"
chmod 600 "$backup/release.txt"

tmp_active="$(mktemp "$state/.active.XXXXXX")"
tmp_unit="$(mktemp "$(dirname "$dropin")/.90-svelte-pinned.XXXXXX")"
trap 'rm -f -- "$tmp_active" "$tmp_unit"' EXIT
sed -e "s|^BOOT_IMAGE_ID=.*|BOOT_IMAGE_ID=$NEXT_IMAGE_ID|" \
    -e "s|^BOOT_IMAGE_TAG=.*|BOOT_IMAGE_TAG=$NEXT_IMAGE_TAG|" \
    -e "s|^BOOT_SOURCE_SHA=.*|BOOT_SOURCE_SHA=$revision|" "$active" > "$tmp_active"
sed "s|^Environment=RCAT_SVELTE_IMAGE=$BOOT_IMAGE_TAG$|Environment=RCAT_SVELTE_IMAGE=$NEXT_IMAGE_TAG|" \
  "$dropin" > "$tmp_unit"
[[ "$(grep -Fxc "Environment=RCAT_SVELTE_IMAGE=$NEXT_IMAGE_TAG" "$tmp_unit")" == 1 ]] ||
  fail "new-systemd-drop-in-unexpected"
[[ "$(grep -Fxc "BOOT_IMAGE_ID=$NEXT_IMAGE_ID" "$tmp_active")" == 1 ]] ||
  fail "new-state-image-unexpected"
chmod 600 "$tmp_active"
chmod 644 "$tmp_unit"

# Switch the *same* quota service to the prebuilt image. The unit itself
# performs one orderly stop and start; no other service or Docker restart.
mv -T "$tmp_unit" "$dropin"
mv -T "$tmp_active" "$active"
systemctl daemon-reload
[[ " $(systemctl show -P Environment anyflip-quota.service) " == *" RCAT_SVELTE_IMAGE=$NEXT_IMAGE_TAG "* ]] ||
  fail "unit-new-image-env-missing-check-backup"
echo "SVELTE_ONLY_UPGRADE=RESTARTING_ANYFLIP_QUOTA_ONCE"
echo "PREVIOUS_CONFIGURATION=$backup"
systemctl restart anyflip-quota.service ||
  fail "quota-restart-failed-operator-recovery-required"

new_cid=""
for _ in {1..45}; do
  mapfile -t current < <(docker ps -q --filter "label=com.docker.compose.project=$BOOT_PROJECT" --filter "label=com.docker.compose.service=anyflip-web")
  if [[ "${#current[@]}" -eq 1 ]]; then
    new_cid="${current[0]}"
    if [[ "$(docker inspect -f '{{.Image}}' "$new_cid")" == "$NEXT_IMAGE_ID" &&
      "$(docker inspect -f '{{if .State.Health}}{{.State.Health.Status}}{{end}}' "$new_cid")" == healthy ]]; then
      break
    fi
  fi
  sleep 2
done
[[ -n "$new_cid" ]] || fail "no-running-app-investigate-quota-journal"
[[ "$(docker inspect -f '{{.Image}}' "$new_cid")" == "$NEXT_IMAGE_ID" ]] ||
  fail "wrong-image-after-cutover"
[[ "$(docker inspect -f '{{if .State.Health}}{{.State.Health.Status}}{{end}}' "$new_cid")" == healthy ]] ||
  fail "new-image-not-healthy"
[[ "$(docker inspect -f '{{range .Config.Env}}{{if eq . "ANYFLIP_WEB_UI=svelte"}}yes{{end}}{{end}}' "$new_cid")" == yes ]] ||
  fail "new-mode-not-svelte"
[[ "$(docker inspect -f '{{range .Config.Env}}{{if eq . "ANYFLIP_AUTH_MODE=google"}}yes{{end}}{{end}}' "$new_cid")" == yes ]] ||
  fail "new-google-auth-not-active"
[[ "$(docker inspect -f '{{range .Mounts}}{{.Type}}|{{.Name}}|{{.Source}}|{{.Destination}};{{end}}' "$new_cid")" == "$BOOT_MOUNTS" ]] ||
  fail "new-job-volume-differs"
[[ "$(docker inspect -f '{{range (index .HostConfig.PortBindings "8080/tcp")}}{{.HostIp}}:{{.HostPort}}{{end}}' "$new_cid")" == "$BOOT_PORT" ]] ||
  fail "new-LAN-port-differs"
BASE_URL="http://$BOOT_PORT" CANONICAL_HOST=anyflip.rcat.ac.th \
  sh "$src/scripts/verify-svelte-ui.sh" ||
  fail "new-Svelte-HTTP-smoke-failed"
echo "SVELTE_ONLY_UPGRADE=PASS"
echo "SVELTE_ONLY_PRODUCTION_BINARY=PASS"
echo "NEW_IMAGE=$NEXT_IMAGE_ID"
echo "NEW_SOURCE=$revision"
echo "PREVIOUS_CONFIGURATION=$backup"
echo "REBOOT_PROOF=PENDING_NEXT_NORMAL_REBOOT"
