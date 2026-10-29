#!/bin/sh
# Read-only P3 LAN host preflight. No token, book download, Docker writes, or restarts.
# This is NOT a disk-quota proof or a public-deployment approval.
set -eu

BASE_URL=${BASE_URL:-http://127.0.0.1:8094}
BASE_URL=${BASE_URL%/}
case "$BASE_URL" in
  http://127.0.0.1:*|http://localhost:*|http://\[::1\]:*|https://*) ;;
  http://10.10.1.100:8094)
    if [ "${ALLOW_LAN_HTTP_READONLY:-0}" != 1 ]; then
      echo 'REFUSED: LAN plain HTTP probe requires ALLOW_LAN_HTTP_READONLY=1' >&2
      exit 2
    fi
    ;;
  *)
    echo 'REFUSED: only loopback HTTP, the operator-approved LAN address (read-only opt-in), or HTTPS are accepted' >&2
    exit 2
    ;;
esac

fail() { printf 'FAIL: %s\n' "$1" >&2; exit 1; }
pass() { printf 'PASS: %s\n' "$1"; }
pending() { printf 'PENDING (physical acceptance): %s\n' "$1"; }

for tool in curl docker; do
  command -v "$tool" >/dev/null 2>&1 || fail "$tool not installed"
done
docker compose version >/dev/null 2>&1 || fail 'Docker Compose v2 unavailable'
[ -f compose.web.yaml ] || fail 'compose.web.yaml not found in current directory'

# Do not read .env or inspect/export the service's secret environment.
docker compose -f compose.web.yaml config --quiet || fail 'Compose configuration invalid'
pass 'Docker Compose syntax'

container_id=$(docker compose -f compose.web.yaml ps -q anyflip-web)
[ -n "$container_id" ] || fail 'anyflip-web container not running'
docker inspect --format '{{.State.Running}}' "$container_id" | grep -qx true || fail 'container is not running'
pass 'Web container running'

user=$(docker inspect --format '{{.Config.User}}' "$container_id")
[ "$user" = '10001:10001' ] || fail 'container is not running as expected non-root UID/GID'
read_only=$(docker inspect --format '{{.HostConfig.ReadonlyRootfs}}' "$container_id")
[ "$read_only" = true ] || fail 'container root filesystem is writable'
privileged=$(docker inspect --format '{{.HostConfig.Privileged}}' "$container_id")
[ "$privileged" = false ] || fail 'privileged container enabled'
pass 'non-root and read-only container hardening'

volume_type=$(docker inspect --format '{{range .Mounts}}{{if eq .Destination "/data/jobs"}}{{.Type}}{{end}}{{end}}' "$container_id")
[ "$volume_type" = volume ] || fail 'expected named volume at /data/jobs is missing or replaced'
pass 'existing named Docker volume retained'

# Never print the container's env vars, mount sources, host paths or token.
bindings=$(docker inspect --format '{{range $port, $hosts := .NetworkSettings.Ports}}{{if eq $port "8080/tcp"}}{{range $hosts}}{{.HostIp}}:{{.HostPort}} {{end}}{{end}}{{end}}' "$container_id")
case "$bindings" in
  *'0.0.0.0:'*|*':::'*) fail 'container published on an all-interface address' ;;
esac
case "$bindings" in
  *'127.0.0.1:8094'*|*'10.10.1.100:8094'*) pass 'Web port restricted to expected local/LAN interface' ;;
  *) fail 'unexpected container port mapping; investigate privately' ;;
esac

health=$(curl --silent --show-error --output /dev/null --write-out '%{http_code}' --max-time 10 "$BASE_URL/healthz") || fail 'health endpoint unreachable'
[ "$health" = 200 ] || fail 'health endpoint did not return HTTP 200'
pass 'HTTP health'

unauth=$(curl --silent --show-error --output /dev/null --write-out '%{http_code}' --max-time 10 "$BASE_URL/api/jobs/00000000000000000000000000000000") || fail 'unauthenticated API unreachable'
[ "$unauth" = 401 ] || fail 'API did not deny missing Bearer token'
pass 'unauthenticated API denied'

headers=$(curl --silent --show-error --dump-header - --output /dev/null --max-time 10 "$BASE_URL/") || fail 'Web HTML unreachable'
printf '%s\n' "$headers" | grep -qi '^Content-Security-Policy:.*script-src' || fail 'HTML Content-Security-Policy absent'
if printf '%s\n' "$headers" | grep -qi 'unsafe-inline\|unsafe-eval'; then
  fail 'CSP permits unsafe inline/eval scripts'
fi
pass 'strict browser CSP headers'

# No fake proof: OS hard quota and genuine user/browser tests cannot be
# certified by a read-only command, including when the probe passes.
pending 'hard volume quota: verify and record filesystem-enforced limit on this Ubuntu host'
pending 'controlled low-disk and SIGKILL/restart/recovery test on this Ubuntu host'
pending 'one complete publisher-authorized book, two PDF viewers and Thai readability'
pending 'desktop/mobile physical browser walkthrough and operator sign-off'
printf '%s\n' 'HOST_PREFLIGHT_PASS — read-only checks passed; release/production acceptance still OPEN'
