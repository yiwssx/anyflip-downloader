"""Focused OCI-layout tests for independent AnyFlip release-candidate consumer."""
import hashlib
import importlib.util
import io
import json
from pathlib import Path
import tarfile
import tempfile
import unittest
from unittest.mock import patch

SCRIPT = Path(__file__).resolve().parents[1] / "scripts" / "verify-attested-staging-archive.py"
spec = importlib.util.spec_from_file_location("stage_archive_verifier", SCRIPT)
verifier = importlib.util.module_from_spec(spec)
spec.loader.exec_module(verifier)

SOURCE = "b" * 40
FILENAME = "anyflip-web_v0.2.0-rc.115_linux_amd64.docker.tar"


def _json(obj):
    return json.dumps(obj, sort_keys=True, separators=(",", ":")).encode()


def create_archive(path, *, source=SOURCE, tag_source=SOURCE,
                   duplicate=False, corrupt_layer=False):
    """Minimal real-shape OCI Docker-save, with distinct index and config IDs."""
    blobs = {}

    def blob(raw):
        digest = "sha256:" + hashlib.sha256(raw).hexdigest()
        blobs["blobs/sha256/" + digest[7:]] = raw
        return {"digest": digest, "size": len(raw)}

    layer = blob(b"compressed-layer-placeholder")
    config = blob(_json({
        "architecture": "amd64", "os": "linux",
        "config": {"Labels": {"org.opencontainers.image.revision": source}},
    }))
    manifest = blob(_json({
        "schemaVersion": 2, "mediaType": "application/vnd.oci.image.manifest.v1+json",
        "config": {**config, "mediaType": "application/vnd.oci.image.config.v1+json"},
        "layers": [{**layer, "mediaType": "application/vnd.oci.image.layer.v1.tar+gzip"}],
    }))
    nested_index = blob(_json({
        "schemaVersion": 2, "mediaType": "application/vnd.oci.image.index.v1+json",
        "manifests": [{
            **manifest, "mediaType": "application/vnd.oci.image.manifest.v1+json",
            "platform": {"os": "linux", "architecture": "amd64"},
        }],
    }))
    index = _json({
        "schemaVersion": 2, "mediaType": "application/vnd.oci.image.index.v1+json",
        "manifests": [{
            **nested_index, "mediaType": "application/vnd.oci.image.index.v1+json",
        }],
    })
    docker_manifest = _json([{
        "Config": "blobs/sha256/" + config["digest"][7:],
        "RepoTags": ["rcat-anyflip-staging:sha-" + tag_source],
        "Layers": ["blobs/sha256/" + layer["digest"][7:]],
    }])
    if corrupt_layer:
        blobs["blobs/sha256/" + layer["digest"][7:]] = b"tampered!"
    entries = [
        ("oci-layout", _json({"imageLayoutVersion": "1.0.0"})),
        ("index.json", index),
        ("manifest.json", docker_manifest),
    ] + list(blobs.items())
    if duplicate:
        entries += [("manifest.json", docker_manifest)]
    with tarfile.open(path, "w") as archive:
        for name, raw in entries:
            entry = tarfile.TarInfo(name)
            entry.size = len(raw)
            archive.addfile(entry, io.BytesIO(raw))
    archive_digest = "sha256:" + hashlib.sha256(path.read_bytes()).hexdigest()
    return archive_digest, nested_index["digest"]


class SignedDockerArchiveTests(unittest.TestCase):
    def setUp(self):
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        self.path = Path(tmp.name) / FILENAME

    @patch.object(verifier, "_verify_attestation")
    def test_genuine_oci_shape_binds_archive_index_id_source_and_tag(self, verify):
        archive_digest, index_id = create_archive(self.path)
        result = verifier.verify_archive(self.path, SOURCE, archive_digest, index_id)
        self.assertEqual(result["status"], "signed_staging_archive_verified")
        self.assertEqual(result["image_id"], index_id)
        self.assertFalse(result["docker_image_loaded"])
        verify.assert_called_once()
        self.assertEqual(verify.call_args.args[1], SOURCE)

    @patch.object(verifier, "_verify_attestation")
    def test_archive_tampering_refused_before_attestation(self, verify):
        _, index_id = create_archive(self.path)
        with self.assertRaises(verifier.ArtifactRefused):
            verifier.verify_archive(self.path, SOURCE, "sha256:" + "0" * 64, index_id)
        verify.assert_not_called()

    @patch.object(verifier, "_verify_attestation")
    def test_wrong_config_source_or_docker_tag_refused(self, _):
        for kwargs in ({"source": "f" * 40}, {"tag_source": "e" * 40}):
            digest, image_id = create_archive(self.path, **kwargs)
            with self.subTest(kwargs=kwargs), self.assertRaises(verifier.ArtifactRefused):
                verifier.verify_archive(self.path, SOURCE, digest, image_id)

    @patch.object(verifier, "_verify_attestation")
    def test_wrong_index_id_and_duplicate_manifest_refused(self, _):
        digest, image_id = create_archive(self.path)
        with self.assertRaises(verifier.ArtifactRefused):
            verifier.verify_archive(self.path, SOURCE, digest, "sha256:" + "c" * 64)
        digest, image_id = create_archive(self.path, duplicate=True)
        with self.assertRaises(verifier.ArtifactRefused):
            verifier.verify_archive(self.path, SOURCE, digest, image_id)

    @patch.object(verifier, "_verify_attestation")
    def test_content_addressed_layer_corruption_refused(self, _):
        digest, image_id = create_archive(self.path, corrupt_layer=True)
        with self.assertRaises(verifier.ArtifactRefused):
            verifier.verify_archive(self.path, SOURCE, digest, image_id)

    @patch.object(verifier, "_verify_attestation",
                  side_effect=verifier.ArtifactRefused("no signature"))
    def test_missing_gitHub_attestation_refused(self, _):
        digest, image_id = create_archive(self.path)
        with self.assertRaises(verifier.ArtifactRefused):
            verifier.verify_archive(self.path, SOURCE, digest, image_id)


if __name__ == "__main__":
    unittest.main()
