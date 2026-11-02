#!/usr/bin/env bash
# Hermetic installer tests: fake docker/curl/sleep; NO live Cloudflare access.
set -Eeuo pipefail
repo="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
mkdir -p "$tmp/bin" "$tmp/home/.config/rcat-anyflip"
printf 'RCAT_ANYFLIP_TUNNEL_TOKEN=ci-synthetic-not-a-real-token\n' > "$tmp/home/.config/rcat-anyflip/tunnel.env"
chmod 600 "$tmp/home/.config/rcat-anyflip/tunnel.env"
cat > "$tmp/bin/docker" <<'MOCK'
#!/usr/bin/env bash
echo "$*" >> "$TEST_TRACE"
if [[ "$*" == *"ps --status running -q anyflip-tunnel"* ]]; then
  echo "mock-connector-running"
fi
MOCK
cat > "$tmp/bin/curl" <<'MOCK'
#!/usr/bin/env bash
if [[ "$*" == *"http://127.0.0.1:8094/healthz"* ]]; then
  [[ "${MOCK_LOCAL_DOWN:-0}" != 1 ]]
  exit
fi
if [[ "$*" == *"https://anyflip.rcat.dpdns.org/healthz"* ]]; then
  count=0
  [[ -f "$MOCK_COUNT" ]] && count="$(cat "$MOCK_COUNT")"
  count=$((count+1))
  printf '%s' "$count" > "$MOCK_COUNT"
  [[ "${MOCK_PUBLIC_DOWN:-0}" != 1 ]] && (( count >= 2 ))
  exit
fi
echo "unexpected curl target" >&2
exit 2
MOCK
cat > "$tmp/bin/sleep" <<'MOCK'
#!/usr/bin/env bash
exit 0
MOCK
chmod +x "$tmp/bin/"*
export HOME="$tmp/home" XDG_CONFIG_HOME="$tmp/home/.config"
export PATH="$tmp/bin:$PATH" TEST_TRACE="$tmp/docker.trace" MOCK_COUNT="$tmp/curl.count"
export RCAT_TUNNEL_WAIT_ATTEMPTS=3
bash "$repo/scripts/start-tunnel-ingress.sh" > "$tmp/log" 2>&1 || {
  cat "$tmp/log" >&2
  echo "FAIL: healthy scenario should succeed" >&2; exit 1
}
grep -q 'PASS: live TLS-verified HTTPS' "$tmp/log"
grep -q 'up -d --pull always --no-deps anyflip-tunnel' "$TEST_TRACE"
! grep -q 'anyflip-web\|down\|restart' "$TEST_TRACE"
[[ "$(cat "$MOCK_COUNT")" == 2 ]]
echo "PASS: healthy connector requires actual HTTPS result and starts only isolated service"

: > "$TEST_TRACE"
rm -f "$MOCK_COUNT"
if MOCK_PUBLIC_DOWN=1 bash "$repo/scripts/start-tunnel-ingress.sh" > "$tmp/log" 2>&1; then
  echo "FAIL: public HTTPS failure must not report success" >&2; exit 1
fi
grep -q 'NOT READY:' "$tmp/log"
grep -q 'up -d --pull always --no-deps anyflip-tunnel' "$TEST_TRACE"
echo "PASS: inaccessible public ingress fails closed while preserving existing application"

: > "$TEST_TRACE"
rm -f "$MOCK_COUNT"
if MOCK_LOCAL_DOWN=1 bash "$repo/scripts/start-tunnel-ingress.sh" > "$tmp/log" 2>&1; then
  echo "FAIL: local origin failure must abort" >&2; exit 1
fi
! grep -q 'up -d' "$TEST_TRACE"
grep -q 'Existing AnyFlip backend is not healthy' "$tmp/log"
echo "PASS: unhealthy local backend never launches the Tunnel"
