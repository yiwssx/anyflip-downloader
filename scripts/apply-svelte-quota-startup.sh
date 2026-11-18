#!/usr/bin/env bash
# Operator-controlled single-service cutover after the safe quota startup
# drop-in has passed all preflights. Requires an explicit --apply flag.
# No background watchers, no build, no Docker daemon restart, no volume deletion.
set -Eeuo pipefail
fail() { printf 'QUOTA_SVELTE_APPLY=BLOCKED reason=%s\n' "$*" >&2; exit 1; }
[[ "$EUID" == 0 ]] || fail "root-required"
[[ "$#" -eq 2 && "$2" == --apply ]] ||
  fail "usage: $0 /opt/anyflip-downloader --apply"
host="$(cd "$1" && pwd -P)"
[[ "$host" == /opt/anyflip-downloader ]] || fail "unexpected-host"
# shellcheck disable=SC1091
source /etc/rcat-anyflip-svelte/active.env
[[ "${BOOT_HOST:-}" == "$host" ]] || fail "pinned-host-mismatch"
[[ "${BOOT_IMAGE_ID:-}" =~ ^sha256:[a-f0-9]{64}$ ]] || fail "invalid-image"
[[ "${BOOT_IMAGE_TAG:-}" =~ ^rcat-anyflip-svelte:boot-[a-f0-9]{12}-[a-f0-9]{12}$ ]] ||
  fail "invalid-tag"
[[ "${BOOT_PROJECT:-}" =~ ^[A-Za-z0-9][A-Za-z0-9_.-]*$ ]] ||
  fail "invalid-project"
[[ "$(systemctl is-enabled rcat-anyflip-svelte-boot.service || true)" == disabled ]] ||
  fail "competing-recovery-must-be-disabled"
systemctl is-active --quiet docker.service || fail "docker-not-running"
systemctl is-active --quiet anyflip-quota.service || fail "quota-unit-inactive"
mountpoint -q /mnt/anyflip-quota-stage || fail "quota-volume-unmounted"
[[ -r /etc/systemd/system/anyflip-quota.service.d/90-svelte-pinned.conf ]] ||
  fail "pinned-systemd-dropin-not-installed"
effective="$(systemctl show -P ExecStart anyflip-quota.service)"
[[ "$effective" == *"/etc/rcat-anyflip-svelte/compose.pinned.yaml"* &&
  "$effective" == *"up --no-build --no-color anyflip-web"* ]] ||
  fail "systemd-runtime-not-pinned"
[[ "$(docker image inspect -f '{{.Id}}' "$BOOT_IMAGE_TAG")" == "$BOOT_IMAGE_ID" ]] ||
  fail "pinned-image-unavailable"

mapfile -t ids < <(docker ps -q --filter "label=com.docker.compose.project=$BOOT_PROJECT" --filter "label=com.docker.compose.service=anyflip-web")
[[ "${#ids[@]}" -eq 1 ]] || fail "expected-one-running-web-container"
cid="${ids[0]}"
[[ "$(docker inspect -f '{{range .Mounts}}{{.Type}}|{{.Name}}|{{.Source}}|{{.Destination}};{{end}}' "$cid")" == "$BOOT_MOUNTS" ]] ||
  fail "jobs-volume-changed"
[[ "$(docker inspect -f '{{range .Config.Env}}{{if eq . "ANYFLIP_AUTH_MODE=google"}}yes{{end}}{{end}}' "$cid")" == yes ]] ||
  fail "google-auth-not-active"
[[ "$(docker inspect -f '{{.Image}}' "$cid")" == "$BOOT_IMAGE_ID" ]] ||
  fail "current-image-unexpected"
[[ "$(docker inspect -f '{{range (index .HostConfig.PortBindings "8080/tcp")}}{{.HostIp}}:{{.HostPort}}{{end}}' "$cid")" == "$BOOT_PORT" ]] ||
  fail "lan-port-changed"

# The operator MUST ensure there is no in-progress download. Service restart
# briefly interrupts the web; do not attempt while jobs are in progress.
echo "QUOTA_SVELTE_APPLY=STARTING_SINGLE_SERVICE_RESTART"
systemctl restart anyflip-quota.service ||
  fail "service-restart-failed-investigate-journal-do-not-repeat"

new_cid=""
for i in {1..45}; do
  mapfile -t ids < <(docker ps -q --filter "label=com.docker.compose.project=$BOOT_PROJECT" --filter "label=com.docker.compose.service=anyflip-web")
  if [[ "${#ids[@]}" -eq 1 ]]; then
    new_cid="${ids[0]}"
    if [[ "$(docker inspect -f '{{.Image}}' "$new_cid")" == "$BOOT_IMAGE_ID" &&
      "$(docker inspect -f '{{range .Config.Env}}{{if eq . "ANYFLIP_WEB_UI=svelte"}}yes{{end}}{{end}}' "$new_cid")" == yes &&
      "$(docker inspect -f '{{if .State.Health}}{{.State.Health.Status}}{{end}}' "$new_cid")" == healthy ]]; then
      break
    fi
  fi
  sleep 2
done
[[ -n "$new_cid" ]] || fail "no-app-container-after-restart"
[[ "$(docker inspect -f '{{.Image}}' "$new_cid")" == "$BOOT_IMAGE_ID" ]] ||
  fail "new-image-not-approved"
[[ "$(docker inspect -f '{{range .Config.Env}}{{if eq . "ANYFLIP_WEB_UI=svelte"}}yes{{end}}{{end}}' "$new_cid")" == yes ]] ||
  fail "frontend-mode-not-svelte"
[[ "$(docker inspect -f '{{range .Mounts}}{{.Type}}|{{.Name}}|{{.Source}}|{{.Destination}};{{end}}' "$new_cid")" == "$BOOT_MOUNTS" ]] ||
  fail "job-volume-changed-on-start"
[[ "$(docker inspect -f '{{range (index .HostConfig.PortBindings "8080/tcp")}}{{.HostIp}}:{{.HostPort}}{{end}}' "$new_cid")" == "$BOOT_PORT" ]] ||
  fail "binding-changed-on-start"
[[ "$(docker inspect -f '{{if .State.Health}}{{.State.Health.Status}}{{end}}' "$new_cid")" == healthy ]] ||
  fail "new-container-unhealthy"
BASE_URL="http://$BOOT_PORT" CANONICAL_HOST=anyflip.rcat.ac.th \
  sh /usr/local/libexec/rcat-anyflip-verify-svelte.sh ||
  fail "svelte-http-smoke-failed-inspect-host-no-auto-rollback"
echo "QUOTA_SVELTE_APPLY=PASS"
echo "SUI05_SVELTE_PRIMARY=PASS"
echo "PINNED_SOURCE=$BOOT_SOURCE_SHA"
echo "QUOTA_SVELTE_REBOOT_PROOF=PENDING_NORMAL_REBOOT"
