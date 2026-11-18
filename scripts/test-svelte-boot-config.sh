#!/usr/bin/env bash
# CI only: verify pinned Svelte reboot overlay and quota-safe systemd ordering.
# No host connections, container creation, credentials or publisher jobs.
set -Eeuo pipefail
docker compose version >/dev/null
bash -n scripts/install-svelte-boot.sh
bash -n scripts/reconcile-svelte-boot.sh
bash -n scripts/install-svelte-boot-readonly-guard.sh
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
assert "ExecStart=/usr/local/libexec/rcat-anyflip-svelte-boot.sh check" in unit
assert "ExecStart=/usr/local/libexec/rcat-anyflip-svelte-boot.sh ensure" not in unit
print("SVELTE_BOOT_NON_MUTATING_STARTUP=PASS")
assert "WantedBy=multi-user.target" in unit
print("SVELTE_BOOT_SYSTEMD_ORDER=PASS")
PY

# Regression for an incident on 2026-11-18: a multiline process
# substitution accidentally executed --filter as a separate shell command.
# A shell syntax check alone did not catch it. Execute only the extracted
# docker-ps expression with a synthetic docker function (no Host access).
python3 - <<'PY'
from pathlib import Path
import subprocess

source=Path("scripts/reconcile-svelte-boot.sh").read_text()
start=source.index("mapfile -t ids < <(docker ps -aq")
end=source.index("\n[[ ",start)
snippet=source[start:end]
harness=r"""
set -Eeuo pipefail
BOOT_PROJECT=test-production
docker() {
  [[ "$#" -eq 6 && "$1" == ps && "$2" == -aq &&
     "$3" == --filter &&
     "$4" == label=com.docker.compose.project=test-production &&
     "$5" == --filter &&
     "$6" == label=com.docker.compose.service=anyflip-web ]] || return 87
  printf '%s\n' 'synthetic-container-id'
}
""" + snippet + """
[[ ${#ids[@]} -eq 1 && "${ids[0]}" == synthetic-container-id ]]
"""
result=subprocess.run(["bash","-c",harness],capture_output=True,text=True)
if result.returncode:
    raise SystemExit("SVELTE_BOOT_DOCKER_FILTER_REGRESSION=FAIL\n"+result.stderr)
print("SVELTE_BOOT_DOCKER_FILTER_REGRESSION=PASS")
PY
