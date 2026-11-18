#!/usr/bin/env bash
# CI only: verify pinned Svelte reboot overlay and quota-safe systemd ordering.
# No host connections, container creation, credentials or publisher jobs.
set -Eeuo pipefail
docker compose version >/dev/null
bash -n scripts/install-svelte-boot.sh
bash -n scripts/reconcile-svelte-boot.sh
bash -n scripts/install-svelte-boot-readonly-guard.sh
bash -n scripts/test-svelte-boot-config.sh
bash -n scripts/install-svelte-quota-startup.sh
bash -n scripts/apply-svelte-quota-startup.sh

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

# The permanent fix changes the ORIGINAL quota unit, not another boot
# service. Synthetic contract: source must preserve quota readiness and
# bind/mount identity, pin one image, and never restart a live service while
# merely installing its systemd drop-in.
python3 - <<'PY'
from pathlib import Path
installer=Path("scripts/install-svelte-quota-startup.sh").read_text()
apply=Path("scripts/apply-svelte-quota-startup.sh").read_text()
for token in (
  "ExecStartPre=/usr/local/sbin/anyflip-quota-ready",
  "RequiresMountsFor=/mnt/anyflip-quota-stage",
  "BindsTo=mnt-anyflip",
  "systemctl daemon-reload",
  "Environment=RCAT_SVELTE_IMAGE=$BOOT_IMAGE_TAG",
  "ExecStart=/usr/bin/docker compose --project-directory $host -p $BOOT_PROJECT",
  "-f $state/compose.pinned.yaml up --no-build --no-color anyflip-web",
  "-f $state/compose.pinned.yaml stop anyflip-web",
  '[[ "$(systemctl is-enabled rcat-anyflip-svelte-boot.service || true)" == disabled ]]',
):
    assert token in installer, f"Permanent quota startup guard missing: {token}"
assert "systemctl restart anyflip-quota.service" not in installer, "installer must not restart production"
assert "docker compose down" not in installer
assert "systemctl restart anyflip-quota.service" in apply, "controlled cutover missing"
assert '[[ "$#" -eq 2 && "$2" == --apply ]]' in apply, "apply must require explicit operator flag"
assert "docker compose down" not in apply
assert "docker image rm" not in apply
assert "docker volume" not in apply
print("SVELTE_ORIGINAL_QUOTA_STARTUP_CONTRACT=PASS")
PY

# 2026-11-18 Host regression: systemctl reset-failed may return nonzero
# because the already-disabled guard is no longer loaded by systemd.
# Execute the actual cleanup / fail-closed disabled checks with a mock
# systemctl; no production service is invoked.
python3 - <<'PY'
from pathlib import Path
import subprocess
source=Path("scripts/install-svelte-boot-readonly-guard.sh").read_text()
begin=source.index("systemctl daemon-reload\n",source.index("# Disable the old force-recreate unit"))
end=source.index("grep -Fq 'ExecStart=/usr/local/libexec/",begin)
snippet=source[begin:end]
mock=r"""
set -Eeuo pipefail
die() { echo "fail: $*" >&2; exit 90; }
systemctl() {
 case "$1" in
  daemon-reload) return 0 ;;
  reset-failed) echo "Unit not loaded" >&2; return 5 ;;
  is-enabled) printf '%s\n' disabled; return 0 ;;
  *) return 99 ;;
 esac
}
"""
result=subprocess.run(["bash","-c",mock+snippet],capture_output=True,text=True)
if result.returncode:
    raise SystemExit("SVELTE_BOOT_UNLOADED_UNIT_REGRESSION=FAIL\n"+result.stderr)
assert 'enabled="$(systemctl is-enabled' in snippet
print("SVELTE_BOOT_UNLOADED_UNIT_REGRESSION=PASS")
quota=Path("scripts/install-svelte-quota-startup.sh").read_text()
assert 'candidate="$(mktemp "$dropdir/.90-svelte-pinned.conf.XXXXXX")"' in quota
assert 'mv -T -- "$candidate" "$dropin"' in quota
print("SVELTE_QUOTA_ATOMIC_DROPIN=PASS")
PY
