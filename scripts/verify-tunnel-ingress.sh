#!/usr/bin/env bash
# Read-only end-to-end acceptance for controlled Cloudflare Tunnel alternative.
set -Eeuo pipefail
alt="https://anyflip.rcat.dpdns.org/healthz"
origin="http://10.10.1.100:8094/healthz"
echo "===== Host-LAN-bound AnyFlip origin (run on rcatiotserver) ====="
if curl --noproxy "*" --fail --silent --show-error --connect-timeout 2 --max-time 6 "$origin" -o /dev/null; then
  echo "PASS: local origin returns success"
else
  echo "WARN: local origin unreachable from this machine"
fi
echo "===== Public isolated ingress ====="
status="$(curl --silent --show-error --connect-timeout 5 --max-time 20 -o /dev/null -w '%{http_code}' "$alt")" || {
  echo "FAIL: public Tunnel endpoint unreachable; check cloudflared connector and DNS" >&2
  exit 1
}
if [[ "$status" != "200" ]]; then
  echo "FAIL: expected 200, got HTTP $status (an inactive Tunnel often returns 1016/1033)" >&2
  exit 1
fi
echo "PASS: $alt returned HTTP 200 with TLS verification"
echo "Existing https://anyflip.rcat.ac.th remains a SEPARATE route and is NOT fixed by this fallback."
