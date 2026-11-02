#!/usr/bin/env bash
# Non-disruptive, host-local RCAT AnyFlip tunnel connector startup.
# Requires operator access to Ubuntu host 10.10.1.100 (cannot be run from GitHub).
set -Eeuo pipefail
repository="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$repository"

if ! command -v docker >/dev/null 2>&1 || ! docker compose version >/dev/null 2>&1; then
  echo "ERROR: Docker Compose v2 with Docker access is required." >&2
  exit 1
fi

if ! curl -fsS --connect-timeout 2 --max-time 5 http://127.0.0.1:8094/healthz >/dev/null; then
  echo "ERROR: Existing AnyFlip backend is not healthy on 127.0.0.1:8094. No changes made." >&2
  exit 1
fi

config_home="${XDG_CONFIG_HOME:-$HOME/.config}"
secret_dir="$config_home/rcat-anyflip"
env_file="$secret_dir/tunnel.env"
umask 077
mkdir -p -m 0700 "$secret_dir"
if [[ ! -s "$env_file" ]]; then
  if [[ ! -t 0 ]]; then
    echo "ERROR: Missing $env_file and no interactive terminal; no changes made." >&2
    exit 1
  fi
  printf '%s\n' "Retrieve the token from Cloudflare > Networking > Tunnels > rcat-anyflip-direct-ingress."
  IFS= read -r -s -p "Paste tunnel token (hidden; not printed): " tunnel_token
  printf '\n'
  if [[ ${#tunnel_token} -lt 50 || "$tunnel_token" == *$'\n'* ]]; then
    echo "ERROR: Invalid tunnel token format/length; no changes made." >&2
    exit 1
  fi
  printf 'RCAT_ANYFLIP_TUNNEL_TOKEN=%s\n' "$tunnel_token" > "$env_file"
  unset tunnel_token
fi
chmod 600 "$env_file"
project="rcat-anyflip-ingress"
args=(--project-name "$project" --env-file "$env_file" -f "$repository/compose.tunnel.yaml")
docker compose "${args[@]}" config --quiet
# This command starts ONLY the independent tunnel; no AnyFlip app rebuild/restart and no orphan cleanup.
docker compose "${args[@]}" up -d --pull always --no-deps anyflip-tunnel
docker compose "${args[@]}" ps
printf '\nConnector launched. Validate Cloudflare Tunnel status and HTTPS using:\n'
printf '  bash scripts/verify-tunnel-ingress.sh\n'
