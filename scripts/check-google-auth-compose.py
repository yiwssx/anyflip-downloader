#!/usr/bin/env python3
"""Read-only guard: consume redacted Docker Compose JSON via stdin; print NO secrets."""
import json
import os
import subprocess
import sys
from pathlib import Path

def abort(message):
    print(f"BLOCK: {message}", file=sys.stderr)
    sys.exit(1)

try:
    config = json.load(sys.stdin)
except (ValueError, UnicodeError):
    abort("cannot parse Docker Compose JSON; no deployment performed")

service = config.get("services", {}).get("anyflip-web")
if not isinstance(service, dict):
    abort("anyflip-web service not defined")
env = service.get("environment") or {}
if env.get("ANYFLIP_AUTH_MODE") != "google":
    abort("ANYFLIP_AUTH_MODE must equal google before deployment")
for key in ("GOOGLE_CLIENT_ID", "GOOGLE_CLIENT_SECRET"):
    value = env.get(key)
    if not isinstance(value, str) or not value.strip() or "REPLACE" in value.upper() or "YOUR_" in value.upper():
        abort(f"{key} must be configured privately")
if env.get("GOOGLE_REDIRECT_URI") != "https://anyflip.rcat.ac.th/auth/google/callback":
    abort("GOOGLE_REDIRECT_URI must match the canonical production HTTPS callback exactly")
if not str(env.get("GOOGLE_CLIENT_ID", "")).endswith(".apps.googleusercontent.com"):
    abort("GOOGLE_CLIENT_ID does not look like a Google Web OAuth client ID")

ports = service.get("ports") or []
port_matches = [p for p in ports if isinstance(p, dict) and str(p.get("target")) == "8080"]
if len(ports) != 1 or len(port_matches) != 1:
    abort("exactly one published port is required; include the operator's LAN-binding overlay")
port = port_matches[0]
if str(port.get("published")) != "8094" or str(port.get("host_ip", "")) != "10.10.1.100":
    abort("planned Compose host bind is not 10.10.1.100:8094; do not replace the live LAN bind")

mounts = [v for v in (service.get("volumes") or []) if isinstance(v, dict) and v.get("target") == "/data/jobs"]
if len(mounts) != 1 or mounts[0].get("type") not in ("bind", "volume"):
    abort("exactly one persistent /data/jobs volume or bind mount is required")
planned_mount = mounts[0]

if os.getenv("RCAT_AUTH_SKIP_HOST_CHECKS") == "1":
    print("PASS: Google configuration, canonical callback, LAN port and persistent mount (CI fixture only)")
    sys.exit(0)

def docker(*args):
    try:
        cp = subprocess.run(["docker", *args], text=True, capture_output=True, timeout=12, check=True)
    except (OSError, subprocess.CalledProcessError, subprocess.TimeoutExpired):
        abort("Docker inspection failed; no deployment performed")
    return cp.stdout

ids = docker("ps", "-q", "--filter", "label=com.docker.compose.service=anyflip-web").split()
if len(ids) != 1:
    abort("expected exactly one running anyflip-web container; no deployment performed")
try:
    current = json.loads(docker("inspect", ids[0]))[0]
except (ValueError, IndexError, KeyError, TypeError):
    abort("cannot inspect the running AnyFlip container")

live = (current.get("HostConfig", {}).get("PortBindings") or {}).get("8080/tcp") or []
if len(live) != 1 or live[0].get("HostIp") != "10.10.1.100" or str(live[0].get("HostPort")) != "8094":
    abort("live container port differs from the expected 10.10.1.100:8094")

live_mounts = [v for v in (current.get("Mounts") or []) if v.get("Destination") == "/data/jobs"]
if len(live_mounts) != 1:
    abort("live container persistent job mount missing or ambiguous")
live_mount = live_mounts[0]
if live_mount.get("Type") != planned_mount["type"]:
    abort("planned mount type differs from the live hard-quota storage")
if planned_mount["type"] == "bind":
    if Path(live_mount.get("Source", "")).resolve() != Path(planned_mount.get("source", "")).resolve():
        abort("planned /data/jobs host path differs from the live hard-quota mount")
else:
    name = str(planned_mount.get("source", ""))
    live_name = str(live_mount.get("Name", ""))
    if not (live_name == name or live_name.endswith("_"+name)):
        abort("planned persistent Docker volume name differs from the live volume")

print("PASS: Google-only auth configuration, callback, host LAN bind and live persistent mount match")
print("READY TO CUT OVER: preflight made no changes; OAuth browser login must still be tested")
