#!/usr/bin/env python3
"""Fixed AnyFlip L/A/B Stage-only switch hook, callable ONLY by a trusted caller.

NO --apply CLI. The existing v1 Stage executor must not import this module.
A future separate, approved versioned executor must authenticate the human
GitHub approval, bind the exact operation, and consume replay BEFORE calling
switch_with_authenticated_approval. No root lock changes or Production access.
"""
import hashlib
import importlib.util
import json
from pathlib import Path
import re
import subprocess
import os
import stat
import urllib.error
import urllib.request

PROJECT = "rcat-anyflip-staging"
COMPOSE = Path("/usr/local/libexec/rcat-staging/anyflip-compose.yaml")
PAIR = Path("/etc/rcat-staging/anyflip-verified-version-pair.json")
RECOVERY = Path("/var/lib/rcat-staging-snapshots/legacy-9b08f4c23425")
LOCK = Path("/etc/rcat-staging/anyflip-approved-image")
SECRET = Path("/etc/rcat-staging/anyflip.env")
VOLUME = "rcat-anyflip-staging-jobs"
PAIR_HASH = "sha256:73d90d47890272a52e3ce6c9895a754de3f8f7abe2495751ac12e74a59067334"
LEGACY = ("bc72fe9c959ca91c61557b41712ce0ed5b9e1c79",
          "sha256:9b08f4c2342596e906a0429d09384650b3d9fae540285e8c7e1856912901a399")
SIGNED = {
    "A": ("3dca0d19ce18048129d7506bbba7aff98b852c06",
          "sha256:bc1491a6d0b2416969cad81f83a64f327dcc2bef0aeecb2b35f89d06c10f361a"),
    "B": ("d14cc1e8a0bb268795cb24057e16aaa3a9e73131",
          "sha256:3a4ea4cd1b1a43bf6c7742b7e475edc273952577a2ea564604ae9e44d4980258"),
}
VERSIONS = {"legacy": LEGACY, **SIGNED}
_DOCKER = "/usr/bin/docker"
_ENV = {"PATH": "/usr/sbin:/usr/bin:/sbin:/bin", "LANG": "C",
        "DOCKER_CONFIG": "/etc/rcat-stage-exec/docker-client"}
_ID = re.compile(r"sha256:[a-f0-9]{64}\Z")
_SHA = re.compile(r"[a-f0-9]{40}\Z")


class VersionedStageRefused(ValueError):
    pass


def _root_file(path, mode):
    if path.is_symlink() or not path.is_file():
        raise VersionedStageRefused("protected Staging input missing")
    st = path.stat()
    if st.st_uid != 0 or stat.S_IMODE(st.st_mode) != mode:
        raise VersionedStageRefused("protected Staging file permissions invalid")


def _docker(args, *, image=None):
    env = dict(_ENV)
    if image:
        env["RCAT_STAGE_IMAGE"] = image
    try:
        proc = subprocess.run([_DOCKER, *args], env=env, shell=False,
                              stdin=subprocess.DEVNULL, stdout=subprocess.PIPE,
                              stderr=subprocess.DEVNULL, text=True, check=True,
                              timeout=90)
    except (OSError, subprocess.CalledProcessError, subprocess.TimeoutExpired):
        raise VersionedStageRefused("Staging Docker transaction failed") from None
    return proc.stdout.strip()


def _read_json(path, max_bytes=8192):
    _root_file(path, 0o600)
    if path.stat().st_size > max_bytes:
        raise VersionedStageRefused("protected Staging receipt oversized")
    try:
        return json.loads(path.read_text("utf-8"))
    except (OSError, UnicodeError, ValueError):
        raise VersionedStageRefused("Staging receipt invalid") from None


def _tag(version):
    return "rcat-anyflip-staging:sha-" + VERSIONS[version][0]


def _approved_inputs():
    if os.geteuid() != 0:
        raise VersionedStageRefused("only the separate root-owned Stage executor may call")
    host = subprocess.run(["/usr/sbin/ip", "-o", "-4", "addr", "show"],
                          shell=False, env=_ENV, stdin=subprocess.DEVNULL,
                          stdout=subprocess.PIPE, stderr=subprocess.DEVNULL,
                          text=True, timeout=10, check=False)
    if host.returncode != 0 or not re.search(r"\binet 10\.10\.1\.100/[0-9]+\b", host.stdout):
        raise VersionedStageRefused("not the approved RCAT IOT Staging host")
    for directory in (PAIR.parent, RECOVERY.parent, LOCK.parent):
        s = directory.lstat()
        if not stat.S_ISDIR(s.st_mode) or s.st_uid != 0 or stat.S_IMODE(s.st_mode) != 0o700:
            raise VersionedStageRefused("protected Staging directory unsafe")
    _root_file(LOCK, 0o600)
    _root_file(SECRET, 0o600)
    _root_file(COMPOSE, 0o644)
    lock = LOCK.read_text("ascii").strip().split()
    if lock != [LEGACY[0], LEGACY[1], _tag("legacy")]:
        raise VersionedStageRefused("original Legacy Stage lock changed")
    pair = _read_json(PAIR)
    canonical = json.dumps(pair, sort_keys=True, separators=(",", ":"), ensure_ascii=True)
    if "sha256:" + hashlib.sha256(canonical.encode("ascii")).hexdigest() != PAIR_HASH:
        raise VersionedStageRefused("signed version pair receipt changed")
    if (not isinstance(pair, dict) or set(pair) != {
            "schema_version", "kind", "application", "environment", "from", "to"}
            or pair.get("schema_version") != 1
            or pair.get("kind") != "rcat.anyflip.stage.signed_version_pair"
            or pair.get("environment") != "staging"
            or pair.get("application") != "anyflip-downloader"):
        raise VersionedStageRefused("wrong signed version pair")
    for label, side in (("A", "from"), ("B", "to")):
        entry = pair[side]
        if (not isinstance(entry, dict)
                or (entry.get("source_commit"), entry.get("image_id")) != SIGNED[label]):
            raise VersionedStageRefused("signed image does not match fixed release")
    backup = _read_json(RECOVERY / "receipt.json")
    if (backup.get("kind") != "rcat.stage.volume.snapshot"
            or backup.get("environment") != "staging"
            or backup.get("source_commit") != LEGACY[0]
            or backup.get("image_id") != LEGACY[1]
            or backup.get("scratch_restored") is not True
            or not isinstance(backup.get("data_sha256"), str)
            or not _ID.fullmatch(backup["data_sha256"])):
        raise VersionedStageRefused("verified legacy Stage snapshot unavailable")
    # Verify the actual root-only baseline still matches its scratch-restored
    # receipt; do not accept a receipt for missing or altered recovery data.
    verifier_path = Path(__file__).with_name("stopped-stage-snapshot.py")
    spec = importlib.util.spec_from_file_location("rcat_stage_snapshot", verifier_path)
    verifier = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(verifier)
    try:
        actual_hash, actual_bytes = verifier._tree_receipt(RECOVERY / "data")
    except (OSError, ValueError):
        raise VersionedStageRefused("legacy backup data unreadable") from None
    if actual_hash != backup["data_sha256"] or actual_bytes != backup.get("bytes"):
        raise VersionedStageRefused("legacy backup content changed")
    return backup


def _running_stage():
    ids = _docker(["ps", "-aq", "--filter", "label=com.docker.compose.project=" + PROJECT]).splitlines()
    if len(ids) != 1:
        raise VersionedStageRefused("exactly one isolated Stage container required")
    rows = json.loads(_docker(["inspect", ids[0]]))
    if not isinstance(rows, list) or len(rows) != 1:
        raise VersionedStageRefused("invalid Docker Stage inspect")
    item = rows[0]
    if (item.get("Name") != "/rcat-anyflip-staging-anyflip-web-1"
            or item.get("State", {}).get("Running") is not True
            or item.get("Config", {}).get("Labels", {}).get("com.docker.compose.service") != "anyflip-web"
            or item.get("Config", {}).get("Labels", {}).get("com.docker.compose.project") != PROJECT):
        raise VersionedStageRefused("unrecognized Stage container")
    ports = item.get("NetworkSettings", {}).get("Ports", {}).get("8080/tcp")
    mounts = item.get("Mounts")
    if (ports != [{"HostIp": "127.0.0.1", "HostPort": "18094"}]
            or not isinstance(mounts, list) or len(mounts) != 1
            or mounts[0].get("Name") != VOLUME
            or mounts[0].get("Destination") != "/data/jobs"
            or mounts[0].get("Type") != "volume"):
        raise VersionedStageRefused("Stage port/volume isolation mismatch")
    for label,(sha,image_id) in VERSIONS.items():
        if item.get("Image") == image_id and item.get("Config", {}).get("Image") == _tag(label):
            return label
    raise VersionedStageRefused("Stage running image not in fixed legacy/A/B set")


def _stage_status(path, *, bearer=None):
    request = urllib.request.Request("http://127.0.0.1:18094" + path)
    if bearer is not None:
        request.add_header("Authorization", "Bearer " + bearer)
    opener = urllib.request.build_opener(urllib.request.ProxyHandler({}))
    try:
        with opener.open(request, timeout=4) as response:
            return response.status, response.read(65537)
    except urllib.error.HTTPError as error:
        return error.code, b""
    except (OSError, TimeoutError, urllib.error.URLError):
        return 0, b""


def _idle():
    if _stage_status("/healthz")[0] != 200:
        raise VersionedStageRefused("Stage unhealthy before switching")
    if _stage_status("/api/jobs/00000000000000000000000000000000")[0] != 401:
        raise VersionedStageRefused("Stage unauthorized API access policy changed")
    raw = SECRET.read_text("ascii")
    if not re.fullmatch(r"ANYFLIP_WEB_TOKEN=[A-Za-z0-9_-]{32,128}\n?", raw):
        raise VersionedStageRefused("expected private Stage token layout missing")
    bearer = raw.split("=", 1)[1].strip()
    status, body = _stage_status("/api/jobs", bearer=bearer)
    if status != 200 or len(body) > 65536:
        raise VersionedStageRefused("authenticated Stage job state unavailable")
    try:
        payload = json.loads(body)
        jobs = payload["jobs"]
    except (ValueError, TypeError, KeyError):
        raise VersionedStageRefused("Stage job list unreadable") from None
    # API returns at most 50 records; if capped, older active jobs could
    # be hidden. Treat cap as ambiguity, not 'idle'.
    if (not isinstance(jobs, list) or len(jobs) >= 50
            or any(not isinstance(item, dict)
                   or item.get("stage") not in {"completed", "failed", "cancelled"}
                   for item in jobs)):
        raise VersionedStageRefused("Stage has active or unbounded job state")


def _image_ready(label):
    sha, digest = VERSIONS[label]
    local_id = _docker(["image", "inspect", "--format", "{{.Id}}", _tag(label)])
    revision = _docker(["image", "inspect", "--format",
                        '{{index .Config.Labels "org.opencontainers.image.revision"}}',
                        _tag(label)])
    if local_id != digest or revision != sha:
        raise VersionedStageRefused("local Stage image ID does not match signed identity")


def _check_fresh_approval(approval, replay, *, source, target, receipt):
    # This module itself cannot authenticate a GitHub comment. Its trusted
    # CALLER must use rcat_ops.approval.audit_live_approval + durable replay
    # consumption before calling, and pass matching sanitized results.
    if (not isinstance(approval, dict) or not isinstance(replay, dict)
            or approval.get("provenance") != "fetched_via_authenticated_private_github_api"
            or approval.get("approval_evidence_status") != "structurally_valid"
            or approval.get("identity_separated") is not True
            or approval.get("approver_actor_id") != 48784420
            or approval.get("executor_actor_id") != 354878936
            or not isinstance(approval.get("github_issue_number"), int)
            or approval.get("github_issue_number", 0) <= 0
            or not isinstance(approval.get("github_comment_id"), int)
            or approval.get("github_comment_id", 0) <= 0
            or replay.get("approval_recorded_once") is not True
            or replay.get("github_comment_id") != approval.get("github_comment_id")
            or replay.get("plan_sha256") != approval.get("plan_sha256")):
        raise VersionedStageRefused("fresh independent GitHub approval and replay required")
    # Domain-separated immutable transaction: both images, direction, pair,
    # protected legacy snapshot and data checkpoint identity are bound.
    operation = {"kind": "rcat.anyflip.stage.versioned.operation", "schema_version": 1,
                 "from": source, "to": target,
                 "from_image_id": VERSIONS[source][1],
                 "to_image_id": VERSIONS[target][1],
                 "version_pair_sha256": PAIR_HASH,
                 "legacy_backup_sha256": receipt["data_sha256"]}
    op_hash = "sha256:" + hashlib.sha256(
        json.dumps(operation, sort_keys=True, separators=(",", ":"), ensure_ascii=True)
        .encode("ascii")).hexdigest()
    if approval.get("versioned_operation_digest") != op_hash or replay.get("versioned_operation_digest") != op_hash:
        raise VersionedStageRefused("GitHub approval not bound to exact version switch")
    return op_hash


def switch_with_authenticated_approval(source, target, *, approval, replay):
    """Privileged internal entry point. NO CLI, NO v1 executor integration.

    Requires an independently authenticated, already consumed one-use GitHub
    approval supplied by a separate, future version-scoped executor.
    """
    if source not in VERSIONS or target not in VERSIONS or source == target:
        raise VersionedStageRefused("fixed distinct Stage images only")
    if (source, target) not in {("legacy","A"),("A","B"),("B","A"),
                                ("A","legacy"),("B","legacy")}:
        raise VersionedStageRefused("unreviewed Stage version transition")
    receipt = _approved_inputs()
    operation = _check_fresh_approval(approval, replay, source=source,
                                      target=target, receipt=receipt)
    if _running_stage() != source:
        raise VersionedStageRefused("actual Stage image differs from approved source")
    _image_ready(source)
    _image_ready(target)
    _idle()
    # The current Stage-specific, root-managed Compose is fixed. No 'down',
    # no --volumes, no Production, no arbitrary action from Issue JSON.
    command = ["compose", "--project-name", PROJECT, "-f", str(COMPOSE),
               "up", "--detach", "--no-build", "--no-deps", "--pull", "never",
               "--force-recreate", "anyflip-web"]
    try:
        _docker(command, image=_tag(target))
        if _running_stage() != target:
            raise VersionedStageRefused("new Stage container/image mismatch")
        _idle()
    except VersionedStageRefused:
        # Attempt to resume the exact previous image (not a mutable tag), and
        # leave the isolated Stage stopped if deterministic fallback fails.
        try:
            _docker(command, image=_tag(source))
            if _running_stage() != source:
                raise VersionedStageRefused("old Stage image not restored")
            _idle()
        except VersionedStageRefused:
            try:
                _docker(["compose", "--project-name", PROJECT, "-f", str(COMPOSE),
                         "stop", "anyflip-web"], image=_tag(source))
            except VersionedStageRefused:
                pass
            raise VersionedStageRefused(
                "Stage switch/rollback failed; retained recovery snapshot, operator required"
            ) from None
        raise VersionedStageRefused(
            "Stage version switch failed; old image restored, approval consumed"
        ) from None
    return {"status": "stage_version_switch_passed",
            "from": source, "to": target,
            "from_image_id": VERSIONS[source][1],
            "to_image_id": VERSIONS[target][1],
            "versioned_operation_digest": operation,
            "legacy_volume_snapshot_preserved": True,
            "production_execution_enabled": False}


if __name__ == "__main__":
    raise SystemExit("REFUSED: no CLI execution; independent signed-version GitHub executor not installed")
