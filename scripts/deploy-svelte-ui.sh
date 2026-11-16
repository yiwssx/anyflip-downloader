#!/usr/bin/env bash
# SUI-05 — operator-invoked, one-service Svelte UI cutover.
# Run from a clean, detached release worktree; the existing host checkout and
# its local compose/quota/OAuth files are never reset, overwritten or copied.
set -Eeuo pipefail

die() { printf 'SUI05_REFUSED: %s\n' "$*" >&2; exit 1; }
[[ "${EUID}" -eq 0 ]] || die "run with sudo/root (Docker + private rollback receipt)"
[[ "$#" -eq 1 ]] || die "usage: sudo bash scripts/deploy-svelte-ui.sh /opt/anyflip-downloader"
for exe in docker git curl sha256sum; do
  command -v "$exe" >/dev/null 2>&1 || die "$exe unavailable"
done
docker compose version >/dev/null 2>&1 || die "Docker Compose v2 required"

src="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd -P)"
host="$(cd "$1" && pwd -P)"
[[ -f "$host/.env" ]] || die "existing private host .env missing"
[[ -f "$host/compose.web.yaml" ]] || die "host compose.web.yaml missing"
[[ -f "$host/compose.quota.yaml" ]] || die "existing host quota overlay is required; never bypass it"
[[ -f "$src/compose.svelte.yaml" && -f "$src/Dockerfile.svelte" ]] || die "Svelte release source is incomplete"
[[ "$src" != "$host" ]] || die "use a separate pinned release worktree to preserve host-local files"
[[ "$(git -C "$src" rev-parse --is-inside-work-tree)" == "true" ]] || die "release not a Git worktree"

# Reject unreviewed local modifications of the released application code, but
# deliberately do NOT inspect/reset the host checkout's operator-owned edits.
git -C "$src" diff --quiet -- frontend web.go web_svelte.go Dockerfile.svelte compose.svelte.yaml scripts/deploy-svelte-ui.sh scripts/verify-svelte-ui.sh ||
  die "release source has unreviewed tracked modifications"
git -C "$src" diff --cached --quiet -- frontend web.go web_svelte.go Dockerfile.svelte compose.svelte.yaml scripts/deploy-svelte-ui.sh scripts/verify-svelte-ui.sh ||
  die "release source has unreviewed staged modifications"

source_sha="$(git -C "$src" rev-parse --verify HEAD)"
[[ "$source_sha" =~ ^[0-9a-f]{40}$ ]] || die "invalid source SHA"
export SUI_SOURCE_DIR="$src" SUI_SOURCE_SHA="$source_sha"

base=(--project-directory "$host" --env-file "$host/.env" -f "$host/compose.web.yaml" -f "$host/compose.quota.yaml")
old_id="$(docker compose "${base[@]}" ps -q anyflip-web)"
[[ -n "$old_id" ]] || die "cannot identify the existing running AnyFlip service; stop rather than create new volumes"
old_running="$(docker inspect --format '{{.State.Running}}' "$old_id")"
[[ "$old_running" == "true" ]] || die "existing container not running"
old_google="$(docker inspect --format '{{range .Config.Env}}{{if eq . "ANYFLIP_AUTH_MODE=google"}}yes{{end}}{{end}}' "$old_id")"
[[ "$old_google" == yes ]] || die "production Google Workspace auth not confirmed; do not change mode"
project="$(docker inspect --format '{{index .Config.Labels "com.docker.compose.project"}}' "$old_id")"
[[ "$project" =~ ^[A-Za-z0-9][A-Za-z0-9_.-]*$ ]] || die "invalid existing Compose project name"
old_image_id="$(docker inspect --format '{{.Image}}' "$old_id")"
[[ "$old_image_id" =~ ^sha256:[0-9a-f]{64}$ ]] || die "previous image identity unavailable"
old_mounts="$(docker inspect --format '{{range .Mounts}}{{.Type}}|{{.Name}}|{{.Source}}|{{.Destination}};{{end}}' "$old_id")"

# Preserve old runtime image BY ID; its tag may be replaced by docker compose build.
stamp="$(date -u +%Y%m%dT%H%M%SZ)"
rollback_tag="rcat-anyflip-rollback:sui05-${stamp}"
receipt="/root/anyflip-svelte-rollback-${stamp}"
(umask 077; mkdir -m 700 "$receipt") || die "cannot create private rollback receipt"
docker image tag "$old_image_id" "$rollback_tag"
printf '%s\n' "source_sha=$source_sha" "previous_image_id=$old_image_id" "rollback_image=$rollback_tag" "project=$project" > "$receipt/receipt.txt"
chmod 600 "$receipt/receipt.txt"

cat > "$receipt/rollback.yaml" <<EOF
# Preserve the original job mount and old image; never delete volumes.
services:
  anyflip-web:
    image: "$rollback_tag"
    environment:
      ANYFLIP_WEB_UI: legacy
EOF
chmod 600 "$receipt/rollback.yaml"

deploy=(--project-directory "$host" -p "$project" --env-file "$host/.env" -f "$host/compose.web.yaml" -f "$host/compose.quota.yaml" -f "$src/compose.svelte.yaml")
rollback=(--project-directory "$host" -p "$project" --env-file "$host/.env" -f "$host/compose.web.yaml" -f "$host/compose.quota.yaml" -f "$receipt/rollback.yaml")
applied=0
completed=0

on_exit() {
  code=$?
  if [[ "$code" != 0 && "$applied" == 1 && "$completed" != 1 ]]; then
    printf 'SUI05_DEPLOY=FAILED; attempting preserved-image rollback\n' >&2
    if docker compose "${rollback[@]}" up -d --no-build --no-deps --force-recreate anyflip-web; then
      printf 'SUI05_ROLLBACK=RESTORED_PREVIOUS_IMAGE\n' >&2
    else
      printf 'SUI05_ROLLBACK=FAILED; operator action required using %s\n' "$receipt/rollback.yaml" >&2
    fi
  fi
}
trap on_exit EXIT

docker compose "${deploy[@]}" config --quiet || die "Svelte + host quota Compose validation failed"
printf 'SUI05_SOURCE=%s\n' "$source_sha"
printf 'SUI05_PREVIOUS_IMAGE=%s\n' "$old_image_id"
printf 'SUI05_ROLLBACK_RECEIPT=%s\n' "$receipt"
printf 'SUI05_STEP=BUILD_SOURCE_PINNED_SVELTE_IMAGE\n'
docker compose "${deploy[@]}" build anyflip-web
printf 'SUI05_STEP=RECREATE_ONLY_ANYFLIP_WEB\n'
applied=1
docker compose "${deploy[@]}" up -d --no-build --no-deps --force-recreate anyflip-web

new_id="$(docker compose "${deploy[@]}" ps -q anyflip-web)"
[[ -n "$new_id" ]] || die "new service container missing"
new_image_id="$(docker inspect --format '{{.Image}}' "$new_id")"
revision="$(docker image inspect --format '{{index .Config.Labels "org.opencontainers.image.revision"}}' "$new_image_id")"
[[ "$revision" == "$source_sha" ]] || die "new image revision mismatch"
mode="$(docker inspect --format '{{range .Config.Env}}{{if eq . "ANYFLIP_WEB_UI=svelte"}}yes{{end}}{{end}}' "$new_id")"
[[ "$mode" == yes ]] || die "new container is not Svelte primary mode"
new_mounts="$(docker inspect --format '{{range .Mounts}}{{.Type}}|{{.Name}}|{{.Source}}|{{.Destination}};{{end}}' "$new_id")"
[[ "$old_mounts" == "$new_mounts" ]] || die "persistent volumes/mounts changed; restoring previous image"

healthy=0
for attempt in {1..30}; do
  health="$(docker inspect --format '{{if .State.Health}}{{.State.Health.Status}}{{else}}unknown{{end}}' "$new_id")"
  if [[ "$health" == healthy ]]; then healthy=1; break; fi
  sleep 2
done
[[ "$healthy" == 1 ]] || die "new service failed Docker health check"

# Preserve the established RCAT LAN binding; do not use a new public listener.
port="$(docker port "$new_id" 8080/tcp | head -n 1)"
case "$port" in
  10.10.1.100:8094|127.0.0.1:8094) ;;
  *) die "unexpected service bind (must preserve approved LAN/loopback port)" ;;
esac
BASE_URL="http://$port" CANONICAL_HOST="anyflip.rcat.ac.th" sh "$src/scripts/verify-svelte-ui.sh" ||
  die "Svelte HTTP/CSP/Google smoke failed"
completed=1
printf 'SUI05_CODE_CUTOVER=PASS\n'
printf 'SUI05_RUNTIME_IMAGE=%s\n' "$new_image_id"
printf 'SUI05_ROLLBACK_RECEIPT=%s\n' "$receipt"
printf 'SUI05_REAL_ACCOUNT_DEVICE_UAT=PENDING_OPERATOR\n'
