#!/bin/sh
# Read-only verification of the Svelte UI served by the existing AnyFlip Go origin.
# No cookies, admin credentials, user PDF data, or publisher content required.
set -eu

BASE_URL="${BASE_URL:-https://anyflip.rcat.ac.th}"
CANONICAL_HOST="${CANONICAL_HOST:-anyflip.rcat.ac.th}"
case "$BASE_URL" in
  https://*|http://127.0.0.1:*|http://10.10.1.100:*|http://localhost:*) ;;
  *) echo "REFUSE: use HTTPS or an approved loopback/LAN endpoint" >&2; exit 2 ;;
esac
BASE_URL="${BASE_URL%/}"
command -v curl >/dev/null 2>&1 || { echo "curl required" >&2; exit 2; }
tmp="$(mktemp -d)"
trap 'rm -rf -- "$tmp"' EXIT HUP INT TERM

request_status() {
  curl -sS --max-time 15 -H "Host: $CANONICAL_HOST" -o "$tmp/body" -D "$tmp/headers" -w '%{http_code}' "$BASE_URL$1"
}
health="$(request_status /healthz)"
[ "$health" = 200 ] || { echo "FAIL: /healthz HTTP $health" >&2; exit 1; }

home="$(request_status /)"
[ "$home" = 200 ] || { echo "FAIL: Svelte / HTTP $home" >&2; exit 1; }
cp "$tmp/body" "$tmp/index"
cp "$tmp/headers" "$tmp/home.headers"
grep -q 'data-app="functional"' "$tmp/index" || { echo "FAIL: functional Svelte entry not active" >&2; exit 1; }
grep -q 'data-auth-mode="google"' "$tmp/index" || { echo "FAIL: Google mode not active" >&2; exit 1; }
js="$(grep -oE '/svelte/assets/[A-Za-z0-9._-]+\.js' "$tmp/index" | head -n 1 || true)"
css="$(grep -oE '/svelte/assets/[A-Za-z0-9._-]+\.css' "$tmp/index" | head -n 1 || true)"
[ -n "$js" ] && [ -n "$css" ] || { echo "FAIL: Svelte fingerprinted assets absent" >&2; exit 1; }
grep -qi "script-src 'self'" "$tmp/home.headers" || { echo "FAIL: script CSP missing" >&2; exit 1; }
grep -qi "style-src 'self'" "$tmp/home.headers" || { echo "FAIL: style CSP missing" >&2; exit 1; }
grep -qi 'Cache-Control: no-store' "$tmp/home.headers" || { echo "FAIL: no-store missing" >&2; exit 1; }
if grep -Eiq 'unsafe-inline|unsafe-eval' "$tmp/home.headers"; then
  echo "FAIL: unsafe CSP directive" >&2; exit 1
fi

for asset in "$js" "$css"; do
  status="$(request_status "$asset")"
  [ "$status" = 200 ] || { echo "FAIL: $asset HTTP $status" >&2; exit 1; }
  if echo "$asset" | grep -q '\.js$'; then
    grep -qi 'Content-Type: text/javascript' "$tmp/headers" || { echo "FAIL: JS MIME" >&2; exit 1; }
  else
    grep -qi 'Content-Type: text/css' "$tmp/headers" || { echo "FAIL: CSS MIME" >&2; exit 1; }
  fi
done

for api in /auth/me /api/jobs; do
  status="$(request_status "$api")"
  [ "$status" = 401 ] || { echo "FAIL: anonymous $api HTTP $status (expected 401)" >&2; exit 1; }
done

echo 'SUI05_SVELTE_PRIMARY=PASS'
echo 'SUI05_GOOGLE_ENTRY=PASS'
echo 'SUI05_ASSET_CSP_MIME=PASS'
echo 'SUI05_ANONYMOUS_DENIAL=PASS'
echo 'SUI05_REAL_ACCOUNT_AND_DEVICE_UAT=NOT_TESTED'
