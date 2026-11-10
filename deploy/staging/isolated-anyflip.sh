#!/bin/sh
# RCAT IOT staging ONLY. Never references /opt Production Compose or volumes.
# Host activation requires a separate authorized operator/executor.
set -eu

die() { printf 'REFUSED: %s\n' "$1" >&2; exit 2; }

if [ "$#" -lt 1 ]; then
  die 'usage: sh isolated-anyflip.sh status | activate|stop <rcat-anyflip-staging:sha-40hex>'
fi
operation=$1
case "$operation" in
  status) [ "$#" -eq 1 ] || die 'status takes no image argument' ;;
  activate|stop) [ "$#" -eq 2 ] || die 'image argument required' ;;
  *) die 'only status, activate and stop are supported' ;;
esac

command -v ip >/dev/null 2>&1 || die 'iproute2 required'
ip -o -4 addr show | grep -Eq 'inet 10\.10\.1\.100/[0-9]+' ||
  die 'host does not own approved RCAT IOT address 10.10.1.100'
command -v docker >/dev/null 2>&1 || die 'Docker unavailable'
docker compose version >/dev/null 2>&1 || die 'Docker Compose v2 required'

stage_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd -P)
compose_file=$stage_dir/compose.yaml
secret_dir=/etc/rcat-staging
secret_file=$secret_dir/anyflip.env
project=rcat-anyflip-staging

if [ "$operation" = status ]; then
  stage_ids=$(docker ps -q --filter "label=com.docker.compose.project=$project" \
    --filter "label=com.docker.compose.service=anyflip-web")
  [ -n "$stage_ids" ] || die 'no running dedicated staging container'
  docker ps --filter "label=com.docker.compose.project=$project" \
    --format '{{.ID}} {{.Status}}'
  code=$(curl --silent --output /dev/null --write-out '%{http_code}' \
    --max-time 3 http://127.0.0.1:18094/healthz || true)
  [ "$code" = 200 ] || die 'isolated stage is not healthy'
  printf '%s\n' 'STAGING_HEALTH_PASS (health endpoint only; not release acceptance)'
  exit 0
fi

[ "$(id -u)" -eq 0 ] || die 'host staging operation requires restricted administrator'
image=$2
printf '%s\n' "$image" | LC_ALL=C grep -Eq '^rcat-anyflip-staging:sha-[0-9a-f]{40}$' ||
  die 'image must be pinned to a 40-hex source SHA in staging namespace'
source_sha=$(printf '%s\n' "$image" | cut -d '-' -f 4)
# Image tag is only a label, NOT signed provenance. Source attestation must
# have been checked before building the local runtime image.
label=$(docker image inspect --format '{{index .Config.Labels "org.opencontainers.image.revision"}}' \
  "$image" 2>/dev/null) || die 'preverified staging image not found locally'
[ "$label" = "$source_sha" ] || die 'local staging image source label mismatch'

[ -f "$compose_file" ] || die 'staging Compose is missing'
[ ! -L "$secret_dir" ] && [ ! -L "$secret_file" ] ||
  die 'symlinks forbidden for private stage configuration'

if [ "$operation" = stop ]; then
  [ -f "$secret_file" ] || die 'private stage secret missing'
  RCAT_STAGE_IMAGE=$image
  export RCAT_STAGE_IMAGE
  docker compose --project-name "$project" -f "$compose_file" stop anyflip-web
  printf '%s\n' 'STAGING_STOPPED (stage-only container; data volume preserved)'
  exit 0
fi

command -v ss >/dev/null 2>&1 || die 'ss (iproute2) required'
command -v curl >/dev/null 2>&1 || die 'curl required'
command -v openssl >/dev/null 2>&1 || die 'openssl required for isolated token'
[ -z "$(ss -H -ltn '( sport = :18094 )')" ] || die 'loopback port 18094 is in use'
[ -z "$(docker ps -aq --filter "label=com.docker.compose.project=$project")" ] ||
  die 'staging project already has containers; refusing replacement'
if docker volume inspect rcat-anyflip-staging-jobs >/dev/null 2>&1; then
  die 'staging volume already exists; refusing replacement'
fi
if docker network inspect rcat-anyflip-staging_stage_private >/dev/null 2>&1; then
  die 'staging network already exists; refusing replacement'
fi

if [ -e "$secret_dir" ]; then
  [ -d "$secret_dir" ] &&
    [ "$(stat -c '%u %a' "$secret_dir")" = '0 700' ] ||
    die 'staging secret directory must be root-owned mode 0700'
else
  install -d -o root -g root -m 0700 "$secret_dir"
fi

if [ -e "$secret_file" ]; then
  [ -f "$secret_file" ] &&
    [ "$(stat -c '%u %a' "$secret_file")" = '0 600' ] ||
    die 'staging secret file must be root-owned mode 0600'
else
  umask 077
  temp_secret=$(mktemp "$secret_dir/.anyflip.XXXXXXXX")
  cleanup_secret() { if [ -n "$temp_secret" ]; then rm -f -- "$temp_secret"; fi; }
  trap cleanup_secret EXIT HUP INT TERM
  printf 'ANYFLIP_WEB_TOKEN=%s\n' "$(openssl rand -hex 32)" > "$temp_secret"
  chmod 0600 "$temp_secret"
  [ ! -e "$secret_file" ] || die 'staging token was created concurrently'
  mv -- "$temp_secret" "$secret_file"
  temp_secret=
fi

RCAT_STAGE_IMAGE=$image
export RCAT_STAGE_IMAGE
# --quiet never prints the token. This Compose is staged, not Production.
docker compose --project-name "$project" -f "$compose_file" config --quiet ||
  die 'isolated stage Compose configuration invalid'
docker compose --project-name "$project" -f "$compose_file" up \
  --detach --no-build --no-deps --pull never anyflip-web ||
  die 'staging-only startup failed'

good=0
attempt=0
while [ "$attempt" -lt 20 ]; do
  attempt=$((attempt + 1))
  code=$(curl --silent --output /dev/null --write-out '%{http_code}' \
    --max-time 2 http://127.0.0.1:18094/healthz || true)
  if [ "$code" = 200 ]; then
    good=1
    break
  fi
  sleep 1
done
if [ "$good" -ne 1 ]; then
  docker compose --project-name "$project" -f "$compose_file" stop anyflip-web || true
  die 'stage health failed; only staging service stopped, volume retained'
fi

# No bearer token or user data sent by these checks.
api_code=$(curl --silent --output /dev/null --write-out '%{http_code}' \
  --max-time 3 http://127.0.0.1:18094/api/jobs/00000000000000000000000000000000 || true)
if [ "$api_code" != 401 ]; then
  docker compose --project-name "$project" -f "$compose_file" stop anyflip-web || true
  die 'unauthenticated API was not denied; only staging service stopped'
fi
printf '%s\n' 'STAGING_START_PASS (loopback health 200, unauth API 401; no Production changes)'
