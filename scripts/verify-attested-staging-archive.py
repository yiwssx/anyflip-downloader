#!/usr/bin/env python3
"""Verify a signed, immutable AnyFlip Staging Docker archive WITHOUT loading it.

This is an independent downstream consumer of the existing manual release
workflow's attested image archive. No docker, deployment or Host write occurs.
"""
import hashlib
import json
import re
import shutil
import subprocess
import sys
import tarfile
import tempfile
from pathlib import Path

SHA = re.compile(r"[0-9a-f]{40}\Z")
DIGEST = re.compile(r"sha256:[0-9a-f]{64}\Z")
CONFIG = re.compile(r"[0-9a-f]{64}\.json\Z")
MAX_BYTES = 1024 * 1024 * 1024
WORKFLOW = "yiwssx/anyflip-downloader/.github/workflows/release.yml"
REPOSITORY = "yiwssx/anyflip-downloader"


class ArtifactRefused(ValueError):
    pass


def _tar_json(archive, name, *, max_bytes=131072):
    members = [member for member in archive.getmembers() if member.name == name]
    if len(members) != 1 or not members[0].isfile() or not 0 < members[0].size <= max_bytes:
        raise ArtifactRefused("missing, duplicate or oversized Docker manifest/config")
    stream = archive.extractfile(members[0])
    if stream is None:
        raise ArtifactRefused("Docker archive member unreadable")
    raw = stream.read(max_bytes + 1)
    try:
        return json.loads(raw), raw
    except (ValueError, UnicodeError):
        raise ArtifactRefused("invalid Docker archive JSON") from None


def _inspect_frozen_archive(path, sha):
    try:
        with tarfile.open(path, mode="r:") as tar:
            manifest, _ = _tar_json(tar, "manifest.json")
            if not isinstance(manifest, list) or len(manifest) != 1:
                raise ArtifactRefused("one Docker image required")
            item = manifest[0]
            if not isinstance(item, dict):
                raise ArtifactRefused("invalid Docker image manifest")
            config_name = item.get("Config")
            tag = f"rcat-anyflip-staging:sha-{sha}"
            if (not isinstance(config_name, str) or not CONFIG.fullmatch(config_name)
                    or item.get("RepoTags") != [tag]):
                raise ArtifactRefused("wrong Docker source/tag/config")
            cfg, raw = _tar_json(tar, config_name, max_bytes=8 * 1024 * 1024)
            actual_id = "sha256:" + hashlib.sha256(raw).hexdigest()
            if config_name != actual_id[7:] + ".json":
                raise ArtifactRefused("Docker config name/hash mismatch")
            if (not isinstance(cfg, dict) or cfg.get("architecture") != "amd64"
                    or cfg.get("os") != "linux"
                    or not isinstance(cfg.get("config"), dict)
                    or not isinstance(cfg["config"].get("Labels"), dict)
                    or cfg["config"]["Labels"].get("org.opencontainers.image.revision") != sha):
                raise ArtifactRefused("Docker architecture/source metadata mismatch")
            return actual_id
    except (tarfile.TarError, OSError, EOFError, UnicodeError):
        raise ArtifactRefused("invalid or unreadable Docker archive") from None


def _verify_attestation(path, sha):
    args = [
        "gh", "attestation", "verify", str(path),
        "--repo", REPOSITORY,
        "--signer-workflow", WORKFLOW,
        "--source-ref", "refs/heads/main",
        "--source-digest", sha,
    ]
    try:
        subprocess.run(args, check=True, stdout=subprocess.DEVNULL,
                       stderr=subprocess.DEVNULL, timeout=90)
    except (OSError, subprocess.CalledProcessError, subprocess.TimeoutExpired):
        raise ArtifactRefused("trusted GitHub source attestation not verified") from None


def verify_archive(path, sha, expected_archive_digest, expected_image_id):
    if (not isinstance(sha, str) or not SHA.fullmatch(sha)
            or not isinstance(expected_archive_digest, str)
            or not DIGEST.fullmatch(expected_archive_digest)
            or not isinstance(expected_image_id, str)
            or not DIGEST.fullmatch(expected_image_id)):
        raise ArtifactRefused("exact reviewed source/archive/image SHA-256 required")
    source = Path(path)
    if source.is_symlink() or not source.is_file() or source.stat().st_size > MAX_BYTES:
        raise ArtifactRefused("unsafe or oversized candidate archive")
    if not re.fullmatch(r"anyflip-web_v[0-9]+\.[0-9]+\.[0-9]+(?:-rc\.[0-9]+)?_linux_amd64\.docker\.tar",
                        source.name):
        raise ArtifactRefused("unexpected Staging candidate filename")

    # Freeze bytes before hash/signature validation to avoid TOCTOU between an
    # operator-writable source path and attestation / Docker metadata checking.
    with tempfile.TemporaryDirectory(prefix="rcat-stage-archive-") as tmp:
        frozen = Path(tmp) / source.name
        digest = hashlib.sha256()
        with source.open("rb") as src, frozen.open("xb") as dst:
            while block := src.read(1024 * 1024):
                digest.update(block)
                dst.write(block)
        if "sha256:" + digest.hexdigest() != expected_archive_digest:
            raise ArtifactRefused("archive bytes do not match the reviewed SHA-256")
        _verify_attestation(frozen, sha)
        actual_id = _inspect_frozen_archive(frozen, sha)
        if actual_id != expected_image_id:
            raise ArtifactRefused("Docker image ID differs from the reviewed image ID")
    return {
        "status": "signed_staging_archive_verified",
        "source_commit": sha,
        "archive_digest": expected_archive_digest,
        "image_id": actual_id,
        "production_execution_enabled": False,
        "docker_image_loaded": False,
    }


def main(argv):
    if len(argv) != 5:
        print("usage: verify-attested-staging-archive.py ARCHIVE SOURCE_SHA "
              "sha256:ARCHIVE_BYTES sha256:IMAGE_ID", file=sys.stderr)
        return 2
    try:
        print(json.dumps(verify_archive(*argv[1:]), sort_keys=True))
        return 0
    except (ArtifactRefused, OSError) as e:
        print(f"REFUSED: {e}", file=sys.stderr)
        return 2


if __name__ == "__main__":
    sys.exit(main(sys.argv))
