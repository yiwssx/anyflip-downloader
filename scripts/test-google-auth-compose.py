#!/usr/bin/env python3
"""Hermetic checks for auth preflight: no live Docker, Google, secrets or downloads."""
import copy
import json
import os
import subprocess
import sys
from pathlib import Path

checker = Path(__file__).with_name("check-google-auth-compose.py")
fixture = {
    "services": {"anyflip-web": {
        "environment": {
            "ANYFLIP_AUTH_MODE": "google",
            "ANYFLIP_WEB_TOKEN": "",
            "GOOGLE_CLIENT_ID": "12345-synthetic.apps.googleusercontent.com",
            "GOOGLE_CLIENT_SECRET": "ci-synthetic-only",
            "GOOGLE_REDIRECT_URI": "https://anyflip.rcat.ac.th/auth/google/callback",
        },
        "ports": [{"target": 8080, "published": "8094", "host_ip": "10.10.1.100", "protocol": "tcp"}],
        "volumes": [{"type": "bind", "source": "/mnt/anyflip-quota-stage", "target": "/data/jobs"}],
    }}
}

def verify(payload, expect_success, label):
    cp = subprocess.run([sys.executable, str(checker)],
        input=json.dumps(payload), capture_output=True, text=True,
        env={**os.environ, "RCAT_AUTH_SKIP_HOST_CHECKS": "1"}, check=False, timeout=8)
    if (cp.returncode == 0) != expect_success:
        raise AssertionError(f"{label}: unexpected outcome {cp.returncode}: {cp.stdout} {cp.stderr}")
    if "ci-synthetic-only" in cp.stdout + cp.stderr:
        raise AssertionError("OAuth secret leaked through diagnostic output")
    print(f"PASS: {label}")

verify(fixture, True, "Google Workspace mode with no shared bearer token")
for label, mutate in [
    ("token mode rejected", lambda c: c["services"]["anyflip-web"]["environment"].update({"ANYFLIP_AUTH_MODE": "token"})),
    ("absent client secret rejected", lambda c: c["services"]["anyflip-web"]["environment"].update({"GOOGLE_CLIENT_SECRET": ""})),
    ("non-Google OAuth client rejected", lambda c: c["services"]["anyflip-web"]["environment"].update({"GOOGLE_CLIENT_ID": "fake-client"})),
    ("unapproved redirect URI rejected", lambda c: c["services"]["anyflip-web"]["environment"].update({"GOOGLE_REDIRECT_URI": "https://anyflip.rcat.dpdns.org/auth/google/callback"})),
    ("unsafe loopback bind rejected", lambda c: c["services"]["anyflip-web"]["ports"][0].update({"host_ip": "127.0.0.1"})),
    ("unexpected published port rejected", lambda c: c["services"]["anyflip-web"]["ports"][0].update({"published": "8095"})),
    ("missing quota-backed job storage rejected", lambda c: c["services"]["anyflip-web"].update({"volumes": []})),
    ("duplicate published ports rejected", lambda c: c["services"]["anyflip-web"]["ports"].append({"target":8080,"published":"8094","host_ip":"127.0.0.1"})),
]:
    case = copy.deepcopy(fixture)
    mutate(case)
    verify(case, False, label)
