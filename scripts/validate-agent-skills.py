#!/usr/bin/env python3
"""Check repository-local Agent Skills without third-party dependencies."""

from pathlib import Path
import re
import sys

ROOT = Path(__file__).resolve().parents[1]
SKILLS = ROOT / ".agents" / "skills"
NAME_RE = re.compile(r"^[a-z0-9]+(?:-[a-z0-9]+)*$")
FRONTMATTER_RE = re.compile(r"\A---\r?\n(.*?)\r?\n---\r?\n", re.S)


def validate() -> list[str]:
    problems: list[str] = []
    manifest = ROOT / "AGENTS.md"
    guide = manifest.read_text(encoding="utf-8") if manifest.is_file() else ""
    paths = sorted(SKILLS.glob("*/SKILL.md"))
    if not paths:
        return ["No SKILL.md files found under .agents/skills/"]
    names: set[str] = set()
    for path in paths:
        text = path.read_text(encoding="utf-8")
        match = FRONTMATTER_RE.match(text)
        if not match:
            problems.append(f"{path.relative_to(ROOT)}: missing YAML frontmatter")
            continue
        meta: dict[str, str] = {}
        for line in match.group(1).splitlines():
            if ":" not in line:
                problems.append(f"{path.relative_to(ROOT)}: invalid metadata line")
                continue
            key, value = line.split(":", 1)
            key, value = key.strip(), value.strip()
            if key in meta:
                problems.append(f"{path.relative_to(ROOT)}: duplicated {key}")
            meta[key] = value
        name = meta.get("name", "")
        description = meta.get("description", "")
        if (name != path.parent.name or len(name) > 64 or
                not NAME_RE.fullmatch(name)):
            problems.append(f"{path.relative_to(ROOT)}: invalid skill name")
        if not description or len(description) > 1024:
            problems.append(f"{path.relative_to(ROOT)}: invalid skill description")
        if name in names:
            problems.append(f"{path.relative_to(ROOT)}: duplicated skill name")
        names.add(name)
        if len(text[match.end():].strip()) < 40:
            problems.append(f"{path.relative_to(ROOT)}: empty instructions")
        if name not in guide:
            problems.append(f"{path.relative_to(ROOT)}: missing from AGENTS.md")
    if not guide:
        problems.append("AGENTS.md missing")
    return problems


if __name__ == "__main__":
    errors = validate()
    if errors:
        for error in errors:
            print("SKILL_ERROR:", error, file=sys.stderr)
        raise SystemExit(1)
    print("SKILL_VALIDATION_PASS")
