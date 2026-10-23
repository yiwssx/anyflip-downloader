#!/bin/sh
# Safe, read-only smoke tests for an AnyFlip Web GUI deployment.
set -eu

BASE_URL=${BASE_URL:-http://127.0.0.1:8094}
BASE_URL=${BASE_URL%/}
case "$BASE_URL" in
  http://127.0.0.1:*|http://localhost:*|http://\[::1\]:*|https://*) ;;
  *) echo "Refusing insecure non-loopback BASE_URL (use HTTPS)" >&2; exit 2 ;;
esac

if ! command -v curl >/dev/null 2>&1; then
  echo "curl is required" >&2
  exit 2
fi

expect_status() {
  expected=$1
  label=$2
  actual=$3
  if [ "$actual" != "$expected" ]; then
    printf 'FAIL: %s returned HTTP %s (expected %s)\n' "$label" "$actual" "$expected" >&2
    exit 1
  fi
  printf 'PASS: %s (HTTP %s)\n' "$label" "$expected"
}

health=$(curl --silent --show-error --output /dev/null --write-out '%{http_code}' --max-time 10 "$BASE_URL/healthz")
expect_status 200 'health endpoint' "$health"

missing=$(curl --silent --show-error --output /dev/null --write-out '%{http_code}' --max-time 10 "$BASE_URL/api/jobs/00000000000000000000000000000000")
expect_status 401 'unauthenticated API' "$missing"

if [ -n "${ANYFLIP_WEB_TOKEN:-}" ]; then
  # Read header from stdin, not command-line arguments or echoed log output.
  authenticated=$(printf 'Authorization: Bearer %s\n' "$ANYFLIP_WEB_TOKEN" |
    curl --silent --show-error --output /dev/null --write-out '%{http_code}' \
      --max-time 10 --header @- \
      "$BASE_URL/api/jobs/00000000000000000000000000000000")
  expect_status 404 'authenticated missing job' "$authenticated"

  rejected=$(printf 'Authorization: Bearer %s\n' "$ANYFLIP_WEB_TOKEN" |
    curl --silent --show-error --output /dev/null --write-out '%{http_code}' \
      --max-time 10 --header @- \
      --header 'Content-Type: application/json' \
      --data '{"url":"https://evil.example/book/not-allowed","threads":2}' \
      "$BASE_URL/api/jobs")
  expect_status 400 'invalid book URL rejected' "$rejected"
else
  echo 'SKIP: authenticated checks (ANYFLIP_WEB_TOKEN is unset)'
fi

if command -v docker >/dev/null 2>&1 &&
   docker compose version >/dev/null 2>&1 &&
   [ -f compose.web.yaml ]; then
  docker compose -f compose.web.yaml config --quiet
  echo 'PASS: Docker Compose configuration'
else
  echo 'SKIP: Compose configuration check (Docker Compose or compose.web.yaml unavailable)'
fi

echo 'SMOKE_CHECK_PASS — deploy-specific tests have completed'
