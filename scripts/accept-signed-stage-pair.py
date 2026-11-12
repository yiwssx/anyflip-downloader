#!/usr/bin/env python3
"""One-shot signed A/B archive intake for RCAT IOT Staging. NEVER imports Docker.

Requires explicit Host root authorization and exact already-reviewed source
checkout. Freezes downloaded ZIPs, verifies exact digest and inner receipts,
then calls the EXISTING independent GitHub attestation + OCI pair verifier.
Only after both succeed does it store immutable archives and a root-only
version-pair declaration. No changes to running services, Docker, or volumes.
"""
import argparse
import hashlib
import json
import os
from pathlib import Path
import re
import shutil
import stat
import subprocess
import sys
import tempfile
import zipfile

BASE = Path("/var/lib/rcat-staging-artifacts")
FINAL = BASE / "verified-rc115-rc116"
ROOT_PAIR = Path("/etc/rcat-staging/anyflip-verified-version-pair.json")
LOCK = Path("/etc/rcat-staging/anyflip-approved-image")
REVIEWED = Path(__file__).resolve().parents[1]
MANIFEST = REVIEWED / "deploy/staging/verified-version-pair-rc115-rc116.json"
VERIFIER = REVIEWED / "scripts/verify-stage-version-pair.py"
PAIR_HASH = "73d90d47890272a52e3ce6c9895a754de3f8f7abe2495751ac12e74a59067334"
ARCHIVES = {
    115: ("9e671cf7f21e78d359e90beb1cf6debd0ac6eb7a8370f2f69aafecf32453c36c",
          "3dca0d19ce18048129d7506bbba7aff98b852c06"),
    116: ("f0de65bba9c255b85a25ec129a005196a347b04f433fc6528749624ba157f80b",
          "d14cc1e8a0bb268795cb24057e16aaa3a9e73131"),
}
MAX_ZIP_BYTES = 256 * 1024 * 1024
MAX_ARCHIVE_BYTES = 1024 * 1024 * 1024
_SHA = re.compile(r"[0-9a-f]{40}\Z")


class IntakeRefused(ValueError):
    pass


def _hash_file(path):
    h = hashlib.sha256()
    with path.open("rb") as stream:
        for block in iter(lambda: stream.read(1024 * 1024), b""):
            h.update(block)
    return h.hexdigest()


def _checkout(reviewed_sha):
    if not _SHA.fullmatch(reviewed_sha):
        raise IntakeRefused("exact reviewed 40-hex main SHA required")
    # Root must never execute Python from an operator-writable checkout:
    # immutable commit verification alone does not prevent a local TOCTOU.
    for node in (
        REVIEWED, REVIEWED / ".git", REVIEWED / "scripts",
        REVIEWED / "deploy", REVIEWED / "deploy/staging",
        Path(__file__), MANIFEST, VERIFIER,
        REVIEWED / "scripts/verify-attested-staging-archive.py",
    ):
        s = node.lstat()
        if (node.is_symlink() or s.st_uid != 0 or s.st_mode & 0o022):
            raise IntakeRefused("trusted Stage source checkout must be root-owned and not writable")
    try:
        def git(*args):
            return subprocess.check_output(
                ["/usr/bin/git", "-C", str(REVIEWED), *args],
                stderr=subprocess.DEVNULL, text=True, timeout=15,
            ).strip()
        origin = git("remote", "get-url", "origin")
        if (git("rev-parse", "HEAD") != reviewed_sha
                or git("branch", "--show-current") != "main"
                or git("status", "--porcelain", "--untracked-files=all")
                or origin not in {
                    "https://github.com/yiwssx/anyflip-downloader",
                    "https://github.com/yiwssx/anyflip-downloader.git",
                    "git@github.com:yiwssx/anyflip-downloader.git",
                }):
            raise IntakeRefused("source checkout must be exact clean reviewed main")
    except (OSError, subprocess.CalledProcessError, subprocess.TimeoutExpired):
        raise IntakeRefused("trusted GitHub source checkout unavailable") from None


def _host_guard():
    if os.geteuid() != 0:
        raise IntakeRefused("explicit root RCAT IOT Host operator required")
    try:
        ip = subprocess.check_output(
            ["/usr/sbin/ip", "-o", "-4", "addr", "show"],
            stderr=subprocess.DEVNULL, timeout=10, text=True,
        )
    except (OSError, subprocess.CalledProcessError, subprocess.TimeoutExpired):
        raise IntakeRefused("approved IOT Host identity unavailable") from None
    if re.search(r"\binet 10\.10\.1\.100/[0-9]+\b", ip) is None:
        raise IntakeRefused("refusing non-approved RCAT IOT Host")
    for d in (ROOT_PAIR.parent, LOCK.parent):
        s = d.lstat()
        if not stat.S_ISDIR(s.st_mode) or s.st_uid != 0 or stat.S_IMODE(s.st_mode) != 0o700:
            raise IntakeRefused("root-owned private Stage directory unsafe")
    s = LOCK.lstat()
    if not stat.S_ISREG(s.st_mode) or s.st_uid != 0 or stat.S_IMODE(s.st_mode) != 0o600:
        raise IntakeRefused("original immutable Stage lock unsafe")
    if LOCK.read_text("ascii").strip().split() != [
        "bc72fe9c959ca91c61557b41712ce0ed5b9e1c79",
        "sha256:9b08f4c2342596e906a0429d09384650b3d9fae540285e8c7e1856912901a399",
        "rcat-anyflip-staging:sha-bc72fe9c959ca91c61557b41712ce0ed5b9e1c79",
    ]:
        raise IntakeRefused("accepted Legacy image lock changed")
    if ROOT_PAIR.exists() or ROOT_PAIR.is_symlink() or FINAL.exists() or FINAL.is_symlink():
        raise IntakeRefused("verified signed A/B artifacts already provisioned; no overwrite")
    if BASE.exists():
        s = BASE.lstat()
        if not stat.S_ISDIR(s.st_mode) or s.st_uid != 0 or stat.S_IMODE(s.st_mode) != 0o700:
            raise IntakeRefused("private Stage archive directory unsafe")
    if shutil.which("gh") is None:
        raise IntakeRefused("GitHub CLI required for independent signature verification")


def _expect_manifest():
    data = json.loads(MANIFEST.read_text("utf-8"))
    raw = json.dumps(data, sort_keys=True, separators=(",", ":"), ensure_ascii=True)
    if hashlib.sha256(raw.encode("ascii")).hexdigest() != PAIR_HASH:
        raise IntakeRefused("reviewed A/B manifest fingerprint changed")
    return data


def _extract_candidate(zip_path, version, pair, target):
    if (zip_path.is_symlink() or not zip_path.is_file()
            or not 0 < zip_path.stat().st_size <= MAX_ZIP_BYTES):
        raise IntakeRefused("candidate ZIP missing, symlinked or unbounded")
    expected_zip_hash, expected_source = ARCHIVES[version]
    if _hash_file(zip_path) != expected_zip_hash:
        raise IntakeRefused("downloaded candidate ZIP SHA-256 mismatch")
    entry = pair["from" if version == 115 else "to"]
    archive_name = entry["filename"]
    location = "staging/" + archive_name
    try:
        with zipfile.ZipFile(zip_path) as z:
            for key, suffix in (
                ("image_id", ".image-id"),
                ("source_commit", ".source-sha"),
            ):
                value = z.read(location + suffix).decode("ascii").strip()
                if value != entry[key]:
                    raise IntakeRefused("signed archive source/image receipt mismatch")
            expected_file_sum = entry["archive_digest"][7:] + "  " + archive_name
            if z.read(location + ".sha256").decode("ascii").strip() != expected_file_sum:
                raise IntakeRefused("candidate archive hash receipt mismatch")
            item = z.getinfo(location)
            if not 0 < item.file_size <= MAX_ARCHIVE_BYTES:
                raise IntakeRefused("candidate archive size out of bounds")
            output = target / archive_name
            with z.open(item, "r") as inp, output.open("xb") as out:
                copied = 0
                for block in iter(lambda: inp.read(1024 * 1024), b""):
                    copied += len(block)
                    if copied > MAX_ARCHIVE_BYTES:
                        raise IntakeRefused("unbounded candidate archive")
                    out.write(block)
            if "sha256:" + _hash_file(output) != entry["archive_digest"]:
                raise IntakeRefused("actual Docker archive bytes mismatch")
            if entry["source_commit"] != expected_source:
                raise IntakeRefused("candidate source not exactly trusted")
            output.chmod(0o600)
    except (OSError, zipfile.BadZipFile, KeyError, UnicodeError):
        raise IntakeRefused("candidate ZIP/archive receipt unreadable") from None


def prepare_pair(first, second, reviewed_sha):
    _host_guard()
    _checkout(reviewed_sha)
    pair = _expect_manifest()
    os.umask(0o077)
    BASE.mkdir(mode=0o700, exist_ok=True)
    with tempfile.TemporaryDirectory(prefix=".verify-rcat-stage-", dir=BASE) as tmp:
        private = Path(tmp)
        _extract_candidate(Path(first), 115, pair, private)
        _extract_candidate(Path(second), 116, pair, private)
        (private / "version-pair.json").write_text(
            MANIFEST.read_text("utf-8"), encoding="utf-8",
        )
        (private / "version-pair.json").chmod(0o600)
        try:
            check = subprocess.run(
                [sys.executable, str(VERIFIER), str(private / "version-pair.json")],
                stdout=subprocess.PIPE, stderr=subprocess.DEVNULL, text=True,
                check=True, timeout=240,
                env={"PATH": "/usr/local/bin:/usr/bin:/bin", "LANG": "C",
                     "HOME": "/root"},
            )
            result = json.loads(check.stdout)
        except (OSError, ValueError, subprocess.CalledProcessError,
                subprocess.TimeoutExpired):
            raise IntakeRefused("independent GitHub attestations/OCI pair not verified") from None
        if (result.get("status") != "two_signed_staging_archive_inputs_verified"
                or result.get("version_pair_sha256") != "sha256:" + PAIR_HASH
                or result.get("versioned_switch_authorized") is not False
                or result.get("rollback_execution_enabled") is not False):
            raise IntakeRefused("untrusted or inconsistent independent signed pair result")
        private.rename(FINAL)
    # This root-only receipt is declared AFTER both independent signatures pass.
    # The v2 runner still refuses execution without the separate genuine stopped
    # volume snapshot+scratch restore and freshly authenticated human approval.
    try:
        fd = os.open(ROOT_PAIR, os.O_WRONLY | os.O_CREAT | os.O_EXCL | os.O_NOFOLLOW, 0o600)
        with os.fdopen(fd, "w", encoding="utf-8") as f:
            f.write(MANIFEST.read_text("utf-8"))
    except OSError:
        raise IntakeRefused("signed archives stored, but root receipt not provisioned") from None
    print("STAGING_SIGNED_AB_ARCHIVES_STORED_NO_IMPORT")
    print("version_pair_sha256=sha256:" + PAIR_HASH)
    print("Independent GitHub attestation + frozen OCI archive receipts PASS.")
    print("No Docker load, Stage stop, backup or version switch performed.")


def main():
    parser = argparse.ArgumentParser(description="Explicit one-shot Staging A/B intake, never Docker")
    parser.add_argument("--approve-staging-artifact-intake", action="store_true")
    parser.add_argument("--reviewed-main-sha", required=True)
    parser.add_argument("candidate_a_zip")
    parser.add_argument("candidate_b_zip")
    args = parser.parse_args()
    if not args.approve_staging_artifact_intake:
        raise SystemExit("REFUSED: operator must explicitly approve Staging-only artifact intake")
    try:
        prepare_pair(args.candidate_a_zip, args.candidate_b_zip, args.reviewed_main_sha)
    except (OSError, ValueError) as exc:
        raise SystemExit("REFUSED: " + (str(exc) if isinstance(exc, IntakeRefused)
                                      else "unsafe Stage artifact intake")) from None


if __name__ == "__main__":
    main()
