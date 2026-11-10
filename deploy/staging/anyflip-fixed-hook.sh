#!/bin/sh
# Fixed RCAT IOT staging hook. Host-local only; NOT an Issue listener.
# No general shell, Production Compose, DNS, external port, or volume deletion.
set -eu
fail() { printf 'REFUSED: %s\n' "$1" >&2; exit 2; }

[ "$(id -u)" -eq 0 ] || fail 'root-owned fixed hook requires a separately authorized caller'
[ "$#" -eq 3 ] || fail 'expected deploy SOURCE_SHA sha256:LOCAL_IMAGE_ID'
[ "$1" = deploy ] || fail 'only deploy is supported'
source_sha=$2
image_id=$3
printf '%s\n' "$source_sha" | LC_ALL=C grep -Eq '^[a-f0-9]{40}$' ||
  fail 'immutable source SHA required'
printf '%s\n' "$image_id" | LC_ALL=C grep -Eq '^sha256:[a-f0-9]{64}$' ||
  fail 'immutable local image ID required'

command -v docker >/dev/null 2>&1 || fail 'Docker unavailable'
command -v curl >/dev/null 2>&1 || fail 'curl unavailable'
ip -o -4 addr show | grep -Eq 'inet 10\.10\.1\.100/[0-9]+' ||
  fail 'not approved IOT host'

project=rcat-anyflip-staging
compose=/usr/local/libexec/rcat-staging/anyflip-compose.yaml
approval=/etc/rcat-staging/anyflip-approved-image
private_dir=/etc/rcat-staging
token_file=/etc/rcat-staging/anyflip.env
image="rcat-anyflip-staging:sha-$source_sha"
volume=rcat-anyflip-staging-jobs

# A root-managed, local image lock is mandatory. No GitHub Issue input can
# select an arbitrary new image, and install does not grant sudo/Docker access.
[ ! -L "$private_dir" ] && [ ! -L "$approval" ] &&
[ ! -L "$compose" ] && [ ! -L "$token_file" ] || fail 'symlinks forbidden'
[ "$(stat -c '%u %a' "$private_dir")" = "0 700" ] ||
  fail 'staging config directory not root-owned 0700'
[ "$(stat -c '%u %a' "$approval")" = "0 600" ] ||
  fail 'approved-image lock not root-owned 0600'
[ "$(stat -c '%u %a' "$token_file")" = "0 600" ] ||
  fail 'staging token not root-owned 0600'
[ "$(stat -c '%u %a' "$compose")" = "0 644" ] ||
  fail 'staging compose not root-owned 0644'
[ "$(stat -c '%s' "$approval")" -le 256 ] ||
  fail 'approved image lock too large'
[ "$(wc -l < "$approval")" -eq 1 ] ||
  fail 'approved-image lock must have exactly one line'
read -r locked_sha locked_id locked_ref extra < "$approval" || fail 'invalid approved-image lock'
[ -z "${extra:-}" ] && [ "$locked_sha" = "$source_sha" ] &&
[ "$locked_id" = "$image_id" ] && [ "$locked_ref" = "$image" ] ||
  fail 'requested image differs from root-managed lock'
docker_id=$(docker image inspect --format '{{.Id}}' "$image" 2>/dev/null) ||
  fail 'approved local stage image missing'
revision=$(docker image inspect \
  --format '{{index .Config.Labels "org.opencontainers.image.revision"}}' \
  "$image" 2>/dev/null) || fail 'local image revision unreadable'
[ "$docker_id" = "$image_id" ] && [ "$revision" = "$source_sha" ] ||
  fail 'local stage image ID or revision mismatch'

# Never turn this into a generic "deploy latest" endpoint. It may only resume
# the ALREADY approved stage image and never replace another deployed image.
ids=$(docker ps -aq --filter "label=com.docker.compose.project=$project")
stage_id=$(docker ps -aq --filter "label=com.docker.compose.project=$project" \
  --filter "label=com.docker.compose.service=anyflip-web")
[ -n "$stage_id" ] && [ "$ids" = "$stage_id" ] ||
  fail 'one existing dedicated staging container required'
[ "$(docker inspect --format '{{.Image}}' "$stage_id")" = "$image_id" ] ||
  fail 'existing staging container image differs; rollback gate not accepted'
[ "$(docker volume inspect --format '{{index .Labels "com.docker.compose.project"}}' "$volume")" = "$project" ] ||
  fail 'staging jobs volume not owned by approved project'

RCAT_STAGE_IMAGE=$image
export RCAT_STAGE_IMAGE
docker compose --project-name "$project" -f "$compose" config --quiet ||
  fail 'approved staging configuration invalid'

# No rebuild, no image pull, no Production access, and no --volumes or down.
docker compose --project-name "$project" -f "$compose" up \
  --detach --no-build --no-deps --pull never anyflip-web >/dev/null ||
  fail 'stage-only startup failed'
running=$(docker ps -q --filter "label=com.docker.compose.project=$project" \
  --filter "label=com.docker.compose.service=anyflip-web")
[ -n "$running" ] || fail 'staging service did not start'
[ "$(docker inspect --format '{{.Image}}' "$running")" = "$image_id" ] ||
  fail 'running container image ID mismatched; stop and investigate'

try=0
while [ "$try" -lt 20 ]; do
  try=$((try+1))
  code=$(curl --silent --output /dev/null --write-out '%{http_code}' \
    --max-time 2 http://127.0.0.1:18094/healthz || true)
  [ "$code" = 200 ] && break
  sleep 1
done
if [ "$code" != 200 ]; then
  docker compose --project-name "$project" -f "$compose" stop anyflip-web >/dev/null || true
  fail 'staging health failed, staging service stopped'
fi
auth=$(curl --silent --output /dev/null --write-out '%{http_code}' \
  --max-time 3 http://127.0.0.1:18094/api/jobs/00000000000000000000000000000000 || true)
if [ "$auth" != 401 ]; then
  docker compose --project-name "$project" -f "$compose" stop anyflip-web >/dev/null || true
  fail 'staging unauthenticated API policy failed'
fi
printf '%s\n' 'RESTRICTED_STAGING_HOOK_PASS (existing pinned image, health 200, auth 401)'
