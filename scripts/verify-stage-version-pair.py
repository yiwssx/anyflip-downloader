#!/usr/bin/env python3
"""Offline, fail-closed proof of TWO different signed AnyFlip Stage archives.

This is a read-only verification of release inputs, NOT a deployment planner,
owner approval, host import, backup, rollback, or Docker client.
"""
import hashlib
import importlib.util
import json
import re
import sys
from pathlib import Path

_ARCHIVE_VERIFIER = Path(__file__).with_name("verify-attested-staging-archive.py")
spec = importlib.util.spec_from_file_location("rcat_verified_stage_archive", _ARCHIVE_VERIFIER)
archive_verifier = importlib.util.module_from_spec(spec)
spec.loader.exec_module(archive_verifier)

SHA = re.compile(r"[0-9a-f]{40}\Z")
DIGEST = re.compile(r"sha256:[0-9a-f]{64}\Z")
FILENAME = re.compile(
    r"anyflip-web_v[0-9]+\.[0-9]+\.[0-9]+(?:-rc\.[0-9]+)?_linux_amd64\.docker\.tar\Z"
)
ROOT_KEYS = {"schema_version", "kind", "application", "environment", "from", "to"}
ARTIFACT_KEYS = {"filename", "source_commit", "archive_digest", "image_id"}


class VersionPairRefused(ValueError):
    pass


def _archive_entry(entry):
    if not isinstance(entry, dict) or set(entry) != ARTIFACT_KEYS:
        raise VersionPairRefused("version receipt has missing or unexpected fields")
    name, source = entry["filename"], entry["source_commit"]
    archive_digest, image_id = entry["archive_digest"], entry["image_id"]
    if (not isinstance(name, str) or not FILENAME.fullmatch(name)
            or not isinstance(source, str) or not SHA.fullmatch(source)
            or not isinstance(archive_digest, str) or not DIGEST.fullmatch(archive_digest)
            or not isinstance(image_id, str) or not DIGEST.fullmatch(image_id)):
        raise VersionPairRefused("version receipt must have exact immutable identities")
    if archive_digest == "sha256:" + "0" * 64 or image_id == "sha256:" + "0" * 64:
        raise VersionPairRefused("placeholder image/archive digest forbidden")
    return entry


def verify_version_pair(manifest_path):
    manifest_path = Path(manifest_path)
    if (manifest_path.is_symlink() or not manifest_path.is_file()
            or manifest_path.stat().st_size > 8192):
        raise VersionPairRefused("unsafe or oversized version pair manifest")
    try:
        pair = json.loads(manifest_path.read_text(encoding="utf-8"))
    except (OSError, ValueError, UnicodeError):
        raise VersionPairRefused("unreadable version pair JSON") from None
    if (not isinstance(pair, dict) or set(pair) != ROOT_KEYS
            or type(pair["schema_version"]) is not int or pair["schema_version"] != 1
            or pair["kind"] != "rcat.anyflip.stage.signed_version_pair"
            or pair["application"] != "anyflip-downloader"
            or pair["environment"] != "staging"):
        raise VersionPairRefused("unsupported or non-staging version pair")
    before = _archive_entry(pair["from"])
    after = _archive_entry(pair["to"])
    if (before["source_commit"] == after["source_commit"]
            or before["archive_digest"] == after["archive_digest"]
            or before["image_id"] == after["image_id"]
            or before["filename"] == after["filename"]):
        raise VersionPairRefused("two distinct signed source/image versions required")

    root = manifest_path.parent
    # Reject missing or unsafe members BEFORE doing either expensive GitHub
    # signature verification; never report partial readiness as a valid pair.
    for entry in (before, after):
        archive = root / entry["filename"]
        if not archive.is_file() or archive.is_symlink():
            raise VersionPairRefused("both signed archive files must be present")
    for entry in (before, after):
        archive = root / entry["filename"]
        # The shared verifier validates independently frozen BYTES, GitHub
        # attestation (trusted main+workflow+source), OCI graph and Docker IDs.
        result = archive_verifier.verify_archive(
            archive, entry["source_commit"],
            entry["archive_digest"], entry["image_id"],
        )
        if (result["status"] != "signed_staging_archive_verified"
                or result["source_commit"] != entry["source_commit"]
                or result["archive_digest"] != entry["archive_digest"]
                or result["image_id"] != entry["image_id"]):
            raise VersionPairRefused("independently signed archive receipt disagrees")

    canonical = json.dumps(pair, sort_keys=True, separators=(",", ":"), ensure_ascii=True)
    return {
        "status": "two_signed_staging_archive_inputs_verified",
        "application": "anyflip-downloader",
        "from_source_commit": before["source_commit"],
        "to_source_commit": after["source_commit"],
        "from_image_id": before["image_id"],
        "to_image_id": after["image_id"],
        "version_pair_sha256": "sha256:" + hashlib.sha256(canonical.encode("ascii")).hexdigest(),
        "host_image_import_verified": False,
        "host_volume_backup_restore_verified": False,
        "versioned_switch_authorized": False,
        "rollback_execution_enabled": False,
        "production_execution_enabled": False,
    }


def main(args):
    if len(args) != 2:
        print("usage: verify-stage-version-pair.py /trusted/path/version-pair.json", file=sys.stderr)
        return 2
    try:
        print(json.dumps(verify_version_pair(args[1]), sort_keys=True))
        return 0
    except (OSError, VersionPairRefused, archive_verifier.ArtifactRefused) as exc:
        print(f"REFUSED: {exc}", file=sys.stderr)
        return 2


if __name__ == "__main__":
    sys.exit(main(sys.argv))
