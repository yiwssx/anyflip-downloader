#!/usr/bin/env bash
# Read-only Google Workspace cutover preflight: NEVER rebuilds/stops/restarts containers.
set -Eeuo pipefail
repo="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$repo"

for cmd in docker python3 curl; do
  if ! command -v "$cmd" >/dev/null 2>&1; then
    echo "BLOCK: Missing $cmd; no changes made." >&2
    exit 1
  fi
done
if ! docker compose version >/dev/null 2>&1; then
  echo "BLOCK: Docker Compose v2 required; no changes made." >&2
  exit 1
fi

env_file="${RCAT_AUTH_ENV_FILE:-$repo/.env}"
if [[ ! -f "$env_file" || ! -r "$env_file" ]]; then
  echo "BLOCK: private .env missing or unreadable; no changes made." >&2
  exit 1
fi

args=(-f "$repo/compose.web.yaml")
overlay="${RCAT_AUTH_QUOTA_COMPOSE:-$repo/compose.quota.yaml}"
if [[ -f "$overlay" ]]; then
  args+=(-f "$overlay")
elif [[ "${RCAT_AUTH_ALLOW_NO_QUOTA_OVERLAY:-0}" != "1" ]]; then
  echo "BLOCK: host-specific compose.quota.yaml overlay missing. Preserve the active hard quota." >&2
  exit 1
fi

# Do not echo or write resolved Compose config: it contains the Google client secret.
if ! docker compose --env-file "$env_file" "${args[@]}" config --format json |
  python3 "$repo/scripts/check-google-auth-compose.py"; then
  echo "BLOCK: no production changes were made." >&2
  exit 1
fi

if [[ "${RCAT_AUTH_SKIP_HOST_CHECKS:-0}" != "1" ]]; then
  if ! systemctl is-active --quiet anyflip-quota.service; then
    echo "BLOCK: anyflip-quota.service is not active; no changes made." >&2
    exit 1
  fi
  if ! curl --noproxy "*" --fail --silent --show-error --connect-timeout 3 --max-time 8 \
     http://10.10.1.100:8094/healthz >/dev/null; then
    echo "BLOCK: existing LAN-bound AnyFlip health check failed; no changes made." >&2
    exit 1
  fi
  if ! curl --fail --silent --show-error --connect-timeout 3 --max-time 8 \
     https://anyflip.rcat.ac.th/healthz >/dev/null; then
    echo "BLOCK: canonical HTTPS AnyFlip health failed; no changes made." >&2
    exit 1
  fi
  echo "PASS: quota service active and both LAN and canonical public HTTPS are healthy"
fi
echo "READ-ONLY PREFLIGHT PASS. Application has NOT been rebuilt or restarted."
