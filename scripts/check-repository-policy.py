#!/usr/bin/env python3
"""Offline policy checks for repository-owned configuration files.

GitHub-side rulesets and permissions require a separate admin audit.
"""
from pathlib import Path
import re
import sys

ROOT = Path(__file__).resolve().parent.parent
WORKFLOWS = [
    ".github/workflows/build.yml",
    ".github/workflows/browser-e2e.yml",
    ".github/workflows/release.yml",
]
REQUIRED = [
    ".github/CODEOWNERS",
    ".github/PULL_REQUEST_TEMPLATE.md",
    ".github/ISSUE_TEMPLATE/config.yml",
    ".github/dependabot.yml",
    "SECURITY.md",
    "CONTRIBUTING.md",
    ".editorconfig",
    ".gitattributes",
    "docs/GITHUB_REPOSITORY_POLICY.md",
    "scripts/gh-repo-standards.sh",
]
errors = []

for name in REQUIRED + WORKFLOWS:
    if not (ROOT / name).is_file():
        errors.append(f"missing required file: {name}")

for name in WORKFLOWS:
    path = ROOT / name
    if not path.is_file():
        continue
    data = path.read_text(encoding="utf-8")
    if not re.search(r"(?m)^permissions:\s*\n\s+contents:\s*read\s*$", data):
        errors.append(f"{name}: missing least-privilege contents: read")
    if re.search(r"(?m)^\s*pull_request_target\s*:", data):
        errors.append(f"{name}: pull_request_target is forbidden without security review")
    actions = re.findall(r"(?m)^\s*-\s*uses:\s*([^\s#]+)", data)
    if not actions:
        errors.append(f"{name}: no actions to verify")
    for action in actions:
        if not re.fullmatch(r"[^@\s]+@[a-fA-F0-9]{40}", action):
            errors.append(f"{name}: unpinned action: {action}")
    if "persist-credentials: false" not in data:
        errors.append(f"{name}: checkout must disable persisted credentials")

dependabot = ROOT / ".github/dependabot.yml"
if dependabot.is_file():
    data = dependabot.read_text(encoding="utf-8")
    for ecosystem in ("gomod", "github-actions", "docker"):
        if f'package-ecosystem: "{ecosystem}"' not in data:
            errors.append(f"Dependabot must cover {ecosystem}")

owners = ROOT / ".github/CODEOWNERS"
if owners.is_file() and "* @yiwssx" not in owners.read_text(encoding="utf-8"):
    errors.append("CODEOWNERS must identify a maintained default owner")

if errors:
    for error in errors:
        print(f"FAIL: {error}", file=sys.stderr)
    sys.exit(1)
print(f"PASS: {len(WORKFLOWS)} pinned/read-only workflows and governance files")
