#!/usr/bin/env python3
"""One-time Staging-only stopped-volume snapshot + scratch-volume restore proof.

Operator-triggered only, *never* invoked by the v1 Stage executor. Requires the
accepted existing Staging container to have been safely stopped beforehand.
Does not alter, start, stop or replace any existing container or data volume.
"""
import hashlib
import json
import os
from pathlib import Path
import re
import shutil
import stat
import subprocess
import sys

PROJECT = "rcat-anyflip-staging"
VOLUME = "rcat-anyflip-staging-jobs"
CONTAINER = "/rcat-anyflip-staging-anyflip-web-1"
LOCK = Path("/etc/rcat-staging/anyflip-approved-image")
SECRETS = Path("/etc/rcat-staging/anyflip.env")
BACKUP_DIR = Path("/var/lib/rcat-staging-snapshots")
SHA = re.compile(r"[0-9a-f]{40}\Z")
DIGEST = re.compile(r"sha256:[0-9a-f]{64}\Z")
LIMIT = 4 * 1024 ** 3
ENV = {"PATH": "/usr/sbin:/usr/bin:/sbin:/bin", "LANG": "C",
       "DOCKER_CONFIG": "/etc/rcat-stage-exec/docker-client"}


class SnapshotRefused(ValueError):
    pass


def _command(*args):
    try:
        out = subprocess.run(args, shell=False, env=ENV, stdin=subprocess.DEVNULL,
                             stdout=subprocess.PIPE, stderr=subprocess.DEVNULL,
                             text=True, check=True, timeout=40).stdout
    except (OSError, subprocess.CalledProcessError, subprocess.TimeoutExpired):
        raise SnapshotRefused("staging Docker/host precondition failed") from None
    return out.strip()


def _protected(path, mode):
    if path.is_symlink() or not path.is_file():
        raise SnapshotRefused("required root-only Staging file unavailable")
    s = path.stat()
    if s.st_uid != 0 or stat.S_IMODE(s.st_mode) != mode:
        raise SnapshotRefused("Staging file owner/permissions unsafe")


def _volume_mount(name):
    volumes = json.loads(_command("/usr/bin/docker", "volume", "inspect", name))
    if not isinstance(volumes, list) or len(volumes) != 1:
        raise SnapshotRefused("unexpected staging volume")
    item = volumes[0]
    if (item.get("Name") != name or item.get("Driver") != "local"
            or not isinstance(item.get("Mountpoint"), str)):
        raise SnapshotRefused("only a local named volume is supported")
    root = Path(_command("/usr/bin/docker", "info", "--format", "{{.DockerRootDir}}"))
    source = Path(item["Mountpoint"])
    # Local-driver mountpoints may not point outside Docker's dedicated volume
    # tree, to a symlink, to Production storage, or to an operator mount path.
    expected = root / "volumes" / name / "_data"
    if (source != expected or source.is_symlink() or not source.is_dir()
            or source.resolve() != expected.resolve()
            or root.is_symlink()):
        raise SnapshotRefused("unexpected or unsafe Docker volume mountpoint")
    return source, item


def _baseline():
    if os.geteuid() != 0:
        raise SnapshotRefused("explicit Host root operator required")
    if not re.search(r"\binet 10\.10\.1\.100/[0-9]+\b",
                     _command("/usr/sbin/ip", "-o", "-4", "addr", "show")):
        raise SnapshotRefused("not RCAT IOT approved host")
    if (LOCK.parent.is_symlink() or LOCK.parent.stat().st_uid != 0
            or stat.S_IMODE(LOCK.parent.stat().st_mode) != 0o700):
        raise SnapshotRefused("Staging private directory unsafe")
    _protected(LOCK, 0o600)
    _protected(SECRETS, 0o600)
    parts = LOCK.read_text("ascii").strip().split()
    if len(parts) != 3:
        raise SnapshotRefused("invalid immutable legacy staging lock")
    source_sha, image_id, tag = parts
    if (not SHA.fullmatch(source_sha) or not DIGEST.fullmatch(image_id)
            or tag != "rcat-anyflip-staging:sha-" + source_sha):
        raise SnapshotRefused("invalid root-approved Staging image identity")
    ids = _command("/usr/bin/docker", "ps", "-aq",
                   "--filter", "label=com.docker.compose.project=" + PROJECT).splitlines()
    if len(ids) != 1:
        raise SnapshotRefused("one and only one isolated Stage container required")
    info = json.loads(_command("/usr/bin/docker", "inspect", ids[0]))
    if not isinstance(info, list) or len(info) != 1:
        raise SnapshotRefused("Staging container inspect invalid")
    stage = info[0]
    if (stage.get("Name") != CONTAINER
            or stage.get("Image") != image_id
            or stage.get("State", {}).get("Running") is not False
            or stage.get("Config", {}).get("Labels", {}).get("com.docker.compose.project") != PROJECT
            or stage.get("Config", {}).get("Labels", {}).get("com.docker.compose.service") != "anyflip-web"):
        raise SnapshotRefused("Staging must be the known, STOPPED, original pinned container")
    mounts = stage.get("Mounts")
    if (not isinstance(mounts, list) or len(mounts) != 1
            or mounts[0].get("Type") != "volume"
            or mounts[0].get("Name") != VOLUME
            or mounts[0].get("Destination") != "/data/jobs"):
        raise SnapshotRefused("unexpected Stage volume mount")
    volume_path, meta = _volume_mount(VOLUME)
    if meta.get("Labels", {}).get("com.docker.compose.project") != PROJECT:
        raise SnapshotRefused("Stage volume is not Compose-owned")
    return source_sha, image_id, volume_path


def _tree_receipt(path):
    """Strict deterministic content and ownership manifest; no links/devices."""
    if path.is_symlink() or not path.is_dir():
        raise SnapshotRefused("snapshot path unsafe")
    entries = []
    total = 0
    for base, dirs, files in os.walk(path, followlinks=False):
        dirs.sort()
        files.sort()
        for name in dirs + files:
            p = Path(base) / name
            s = p.lstat()
            if not (stat.S_ISDIR(s.st_mode) or stat.S_ISREG(s.st_mode)):
                raise SnapshotRefused("special file/symlink in Stage data is forbidden")
            relative = p.relative_to(path).as_posix()
            if s.st_mode & stat.S_ISUID or s.st_mode & stat.S_ISGID:
                raise SnapshotRefused("privileged file mode in Stage data")
            digest = None
            if stat.S_ISREG(s.st_mode):
                total += s.st_size
                if total > LIMIT:
                    raise SnapshotRefused("Stage volume exceeds bounded snapshot limit")
                h = hashlib.sha256()
                with p.open("rb") as f:
                    for chunk in iter(lambda: f.read(1024 * 1024), b""):
                        h.update(chunk)
                digest = h.hexdigest()
            entries.append([relative, "d" if stat.S_ISDIR(s.st_mode) else "f",
                            stat.S_IMODE(s.st_mode), s.st_uid, s.st_gid,
                            s.st_size if digest else 0, digest])
    receipt = json.dumps(entries, sort_keys=True, separators=(",", ":"), ensure_ascii=True)
    return "sha256:" + hashlib.sha256(receipt.encode("ascii")).hexdigest(), total


def _copy_stage_tree(source, target):
    if target.exists() or target.is_symlink():
        raise SnapshotRefused("snapshot directory already exists")
    # A stopped, single-container Stage volume is the only accepted input.
    shutil.copytree(source, target, symlinks=False, copy_function=shutil.copy2)
    for base, dirs, files in os.walk(source, followlinks=False):
        for name in dirs + files:
            original = Path(base) / name
            copied = target / original.relative_to(source)
            if original.is_symlink() or not (original.is_dir() or original.is_file()):
                raise SnapshotRefused("Stage source changed while copying")
            stat_info = original.stat()
            os.chown(copied, stat_info.st_uid, stat_info.st_gid)
            os.chmod(copied, stat.S_IMODE(stat_info.st_mode))


def snapshot_and_scratch_restore():
    source_sha, image_id, source = _baseline()
    before, total = _tree_receipt(source)
    if BACKUP_DIR.exists():
        s = BACKUP_DIR.lstat()
        if not stat.S_ISDIR(s.st_mode) or s.st_uid != 0 or stat.S_IMODE(s.st_mode) != 0o700:
            raise SnapshotRefused("unsafe pre-existing Staging-only backup directory")
    else:
        BACKUP_DIR.mkdir(mode=0o700)
    if shutil.disk_usage(BACKUP_DIR).free < total * 3 + 32 * 1024 ** 2:
        raise SnapshotRefused("not enough disk for backup and scratch restore")
    label = image_id[7:19]
    snapshot = BACKUP_DIR / ("legacy-" + label)
    snapshot.mkdir(mode=0o700, exist_ok=False)
    data = snapshot / "data"
    # If any operation fails after this point, preserve the incomplete
    # root-only snapshot for inspection. Never auto-overwrite the baseline.
    _copy_stage_tree(source, data)
    copied, _ = _tree_receipt(data)
    current, _ = _tree_receipt(source)
    if copied != before or current != before:
        raise SnapshotRefused("Stage volume changed or snapshot bytes differ")
    receipt = {"schema_version": 1, "kind": "rcat.stage.volume.snapshot",
               "application": "anyflip-downloader", "environment": "staging",
               "source_commit": source_sha, "image_id": image_id,
               "data_sha256": before, "bytes": total, "scratch_restored": False}
    receipt_file = snapshot / "receipt.json"
    receipt_file.write_text(json.dumps(receipt, sort_keys=True) + "\n", encoding="ascii")
    receipt_file.chmod(0o600)

    scratch_name = "rcat-anyflip-stage-scratch-" + label
    try:
        _command("/usr/bin/docker", "volume", "inspect", scratch_name)
    except SnapshotRefused:
        pass
    else:
        raise SnapshotRefused("scratch volume already exists; refusing to reuse")
    _command("/usr/bin/docker", "volume", "create", "--label",
             "rcat.stage.scratch=verified", scratch_name)
    scratch_path, scratch_meta = _volume_mount(scratch_name)
    if scratch_meta.get("Labels", {}).get("rcat.stage.scratch") != "verified":
        raise SnapshotRefused("scratch volume label not trusted")
    # Restore ONLY to the newly created, isolated scratch Docker volume.
    if any(scratch_path.iterdir()):
        raise SnapshotRefused("new scratch volume was not empty")
    for item in data.iterdir():
        if item.is_dir():
            _copy_stage_tree(item, scratch_path / item.name)
        elif item.is_file():
            shutil.copy2(item, scratch_path / item.name)
            s = item.stat()
            os.chown(scratch_path / item.name, s.st_uid, s.st_gid)
        else:
            raise SnapshotRefused("snapshot source unsafe")
    restored, _ = _tree_receipt(scratch_path)
    if restored != before:
        raise SnapshotRefused("scratch Docker volume restore failed integrity")
    # A scratch volume is disposable; remove ONLY our newly created, labelled
    # isolated scratch object. The real Stage volume is never removed.
    _command("/usr/bin/docker", "volume", "rm", scratch_name)
    receipt["scratch_restored"] = True
    receipt_file.write_text(json.dumps(receipt, sort_keys=True) + "\n", encoding="ascii")
    print("STAGING_STOPPED_VOLUME_SNAPSHOT_SCRATCH_RESTORE_PASS")
    print("Snapshot root:", snapshot)
    print("Source/image:", source_sha, image_id)
    print("Data SHA-256:", before)
    print("Original staging container stayed stopped; no existing volume changed.")


def main(args):
    if args != ["--approve-stopped-stage-snapshot"]:
        raise SystemExit("REFUSED: only --approve-stopped-stage-snapshot is supported")
    try:
        snapshot_and_scratch_restore()
    except (OSError, ValueError, KeyError, TypeError, json.JSONDecodeError) as error:
        # Avoid printing source user-data paths, tokens or Docker raw stderr.
        raise SystemExit("REFUSED: " + (
            str(error) if isinstance(error, SnapshotRefused) else "Staging snapshot unsafe/unavailable"
        )) from None


if __name__ == "__main__":
    main(sys.argv[1:])
