#!/usr/bin/env bash
# CI only: verify pinned Svelte reboot overlay and quota-safe systemd ordering.
# No host connections, container creation, credentials or publisher jobs.
set -Eeuo pipefail
docker compose version >/dev/null
bash -n scripts/install-svelte-boot.sh
bash -n scripts/reconcile-svelte-boot.sh
bash -n scripts/test-svelte-boot-config.sh

# Without a pinned approved Svelte image, this configuration must fail closed.
if env -u RCAT_SVELTE_IMAGE docker compose \
  -f compose.web.yaml -f deploy/systemd/compose.svelte-boot.yaml \
  config --quiet >/dev/null 2>&1; then
  echo "FAIL: unpinned Svelte boot overlay was accepted" >&2
  exit 1
fi

export RCAT_SVELTE_IMAGE=rcat-anyflip-svelte:boot-123456789abc-abcdef123456
export ANYFLIP_AUTH_MODE=google
export GOOGLE_CLIENT_ID=ci-synthetic-client
export GOOGLE_CLIENT_SECRET=ci-synthetic-secret
export GOOGLE_REDIRECT_URI=https://anyflip.rcat.ac.th/auth/google/callback
export ANYFLIP_HOST_BIND_IP=10.10.1.100

# Compose output may contain synthetic credential values; stream only to
# assertions, never print the resolved config.
docker compose \
  -f compose.web.yaml -f deploy/systemd/compose.svelte-boot.yaml \
  config --format json | python3 -c '
import json,sys
svc=json.load(sys.stdin)["services"]["anyflip-web"]
assert svc.get("image")=="rcat-anyflip-svelte:boot-123456789abc-abcdef123456"
assert svc["environment"].get("ANYFLIP_WEB_UI")=="svelte"
assert svc["environment"].get("ANYFLIP_AUTH_MODE")=="google"
assert not svc.get("build"), "boot must never rebuild the legacy Dockerfile"
assert svc.get("restart") in ("no", "none", None)
assert any(str(p.get("host_ip"))=="10.10.1.100" and
           str(p.get("published"))=="8094" and
           int(p.get("target"))==8080 for p in svc.get("ports", []))
assert svc.get("volumes"), "boot must preserve job storage"
print("SVELTE_BOOT_COMPOSE_CONTRACT=PASS")
'

python3 - <<'PY'
from pathlib import Path
unit=Path("deploy/systemd/rcat-anyflip-svelte-boot.service").read_text()
assert "Requires=docker.service anyflip-quota.service" in unit
assert "After=docker.service anyflip-quota.service" in unit
assert "ExecStart=/usr/local/libexec/rcat-anyflip-svelte-boot.sh ensure" in unit
assert "WantedBy=multi-user.target" in unit
print("SVELTE_BOOT_SYSTEMD_ORDER=PASS")
PY
