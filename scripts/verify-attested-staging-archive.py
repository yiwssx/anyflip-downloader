#!/usr/bin/env python3
"""Independently audit an attested AnyFlip Docker-save OCI-layout archive.

No Docker load, extractall, host write, or release authorization. The archive
SHA, GitHub SLSA subject and immutable Docker/OCI descriptor tree are separate
gates. A source LABEL is not accepted as proof of signed provenance.
"""
import hashlib
import json
import re
import subprocess
import sys
import tarfile
import tempfile
from pathlib import Path

SHA = re.compile(r"[0-9a-f]{40}\Z")
DIGEST = re.compile(r"sha256:[0-9a-f]{64}\Z")
BLOB_PATH = re.compile(r"blobs/sha256/([0-9a-f]{64})\Z")
MAX_BYTES = 1024 * 1024 * 1024
WORKFLOW = "yiwssx/anyflip-downloader/.github/workflows/release.yml"
REPOSITORY = "yiwssx/anyflip-downloader"


class ArtifactRefused(ValueError):
    pass


def _read_json(tar, members, name, *, max_bytes=8 * 1024 * 1024):
    item = members.get(name)
    if item is None or not item.isfile() or not 0 < item.size <= max_bytes:
        raise ArtifactRefused("missing or oversized OCI descriptor")
    handle = tar.extractfile(item)
    if handle is None:
        raise ArtifactRefused("unreadable OCI descriptor")
    raw = handle.read(max_bytes + 1)
    try:
        return json.loads(raw), raw
    except (ValueError, UnicodeError):
        raise ArtifactRefused("invalid OCI descriptor JSON") from None


def _blob(tar, members, digest, size=None):
    if not isinstance(digest, str) or not DIGEST.fullmatch(digest):
        raise ArtifactRefused("invalid content-addressed OCI digest")
    path = "blobs/sha256/" + digest[7:]
    member = members.get(path)
    if (member is None or not member.isfile() or member.size > MAX_BYTES
            or (size is not None and member.size != size)):
        raise ArtifactRefused("OCI referenced blob missing or size mismatch")
    handle = tar.extractfile(member)
    if handle is None:
        raise ArtifactRefused("OCI blob unreadable")
    hasher = hashlib.sha256()
    for chunk in iter(lambda: handle.read(1024 * 1024), b""):
        hasher.update(chunk)
    if "sha256:" + hasher.hexdigest() != digest:
        raise ArtifactRefused("OCI content-addressed blob hash mismatch")
    return path


def _descriptor(tar, members, value, *, max_json=8 * 1024 * 1024):
    if (not isinstance(value, dict) or set(("digest", "size")) - set(value)
            or type(value["size"]) is not int or not 0 < value["size"] <= MAX_BYTES):
        raise ArtifactRefused("malformed OCI descriptor")
    path = _blob(tar, members, value["digest"], value["size"])
    if value["size"] > max_json:
        raise ArtifactRefused("OCI metadata too large")
    return _read_json(tar, members, path, max_bytes=max_json)[0]


def _inspect_frozen_archive(path, sha, expected_image_id):
    expected_tag = f"rcat-anyflip-staging:sha-{sha}"
    try:
        with tarfile.open(path, mode="r:") as tar:
            files = tar.getmembers()
            if len(files) > 256 or sum(m.size for m in files) > MAX_BYTES:
                raise ArtifactRefused("unsafe OCI archive size")
            members = {}
            for item in files:
                if item.name in members or not (item.isfile() or item.isdir()):
                    raise ArtifactRefused("duplicate or unsafe OCI tar entry")
                if item.name.startswith("/") or ".." in Path(item.name).parts:
                    raise ArtifactRefused("unsafe OCI archive entry name")
                members[item.name] = item
            layout, _ = _read_json(tar, members, "oci-layout", max_bytes=4096)
            if not isinstance(layout, dict) or layout.get("imageLayoutVersion") != "1.0.0":
                raise ArtifactRefused("Docker-save archive lacks OCI layout")
            docker_manifest, _ = _read_json(tar, members, "manifest.json")
            if not isinstance(docker_manifest, list) or len(docker_manifest) != 1:
                raise ArtifactRefused("exactly one staged Docker image required")
            docker_item = docker_manifest[0]
            if (not isinstance(docker_item, dict) or docker_item.get("RepoTags") != [expected_tag]
                    or not isinstance(docker_item.get("Layers"), list)):
                raise ArtifactRefused("Docker tag or manifest does not match pinned source")

            index, _ = _read_json(tar, members, "index.json")
            if (not isinstance(index, dict) or index.get("schemaVersion") != 2
                    or not isinstance(index.get("manifests"), list)
                    or len(index["manifests"]) != 1):
                raise ArtifactRefused("unrecognized OCI archive top-level index")
            outer = index["manifests"][0]
            if outer.get("mediaType") != "application/vnd.oci.image.index.v1+json":
                raise ArtifactRefused("unexpected OCI top-level media type")
            if outer.get("digest") != expected_image_id:
                raise ArtifactRefused("Docker image/index ID differs from reviewed receipt")
            nested = _descriptor(tar, members, outer)
            if not isinstance(nested, dict) or nested.get("schemaVersion") != 2:
                raise ArtifactRefused("invalid nested OCI image index")
            descriptors = nested.get("manifests")
            if not isinstance(descriptors, list) or not 1 <= len(descriptors) <= 3:
                raise ArtifactRefused("unexpected OCI image manifest set")
            main_images = [
                d for d in descriptors if isinstance(d, dict)
                and d.get("mediaType") == "application/vnd.oci.image.manifest.v1+json"
                and d.get("platform") == {"architecture": "amd64", "os": "linux"}
            ]
            if len(main_images) != 1:
                raise ArtifactRefused("exactly one Linux AMD64 image manifest required")
            image_desc = main_images[0]
            # Authenticate the entire OCI descriptor tree, including optional
            # BuildKit attestation manifests. Internal metadata is not the
            # external trusted GitHub attestation checked separately.
            for entry in descriptors:
                obj = _descriptor(tar, members, entry)
                if not isinstance(obj, dict) or obj.get("schemaVersion") != 2:
                    raise ArtifactRefused("invalid OCI image/attestation manifest")
                for child in [obj.get("config"), *(obj.get("layers") or [])]:
                    if not isinstance(child, dict):
                        raise ArtifactRefused("invalid OCI config/layer descriptor")
                    _blob(tar, members, child.get("digest"), child.get("size"))
            manifest = _descriptor(tar, members, image_desc)
            config_desc = manifest.get("config")
            if not isinstance(config_desc, dict):
                raise ArtifactRefused("missing image config descriptor")
            config_path = _blob(tar, members, config_desc.get("digest"), config_desc.get("size"))
            if docker_item.get("Config") != config_path:
                raise ArtifactRefused("Docker and OCI image configs disagree")
            config, _ = _read_json(tar, members, config_path)
            labels = (config.get("config") or {}).get("Labels") if isinstance(config, dict) else None
            if (config.get("os") != "linux" or config.get("architecture") != "amd64"
                    or not isinstance(labels, dict)
                    or labels.get("org.opencontainers.image.revision") != sha):
                raise ArtifactRefused("OCI config platform/source metadata mismatch")
            layers = manifest.get("layers")
            if (not isinstance(layers, list) or not layers
                    or docker_item["Layers"] != [
                        "blobs/sha256/" + item["digest"][7:] for item in layers
                    ]):
                raise ArtifactRefused("Docker and OCI layer references disagree")
            return expected_image_id
    except (tarfile.TarError, OSError, EOFError, UnicodeError, KeyError, TypeError, AttributeError):
        raise ArtifactRefused("invalid or unreadable OCI staging archive") from None


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

    # Freeze source bytes before hashing, signature, and OCI inspection.
    with tempfile.TemporaryDirectory(prefix="rcat-stage-archive-") as tmp:
        frozen = Path(tmp) / source.name
        digest = hashlib.sha256()
        with source.open("rb") as src, frozen.open("xb") as dst:
            while block := src.read(1024 * 1024):
                digest.update(block)
                dst.write(block)
        if "sha256:" + digest.hexdigest() != expected_archive_digest:
            raise ArtifactRefused("archive bytes do not match reviewed SHA-256")
        _verify_attestation(frozen, sha)
        actual_id = _inspect_frozen_archive(frozen, sha, expected_image_id)
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
