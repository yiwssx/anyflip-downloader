"""Offline, synthetic signed Staging Docker archive identity tests."""
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
FILENAME = "anyflip-web_v0.2.0-rc.114_linux_amd64.docker.tar"


def create_archive(path, *, source=SOURCE, tag_source=SOURCE, duplicate=False):
    config = json.dumps({
        "architecture": "amd64", "os": "linux",
        "config": {"Labels": {"org.opencontainers.image.revision": source}},
    }, sort_keys=True).encode()
    image_id = "sha256:" + hashlib.sha256(config).hexdigest()
    manifest = json.dumps([{
        "Config": image_id[7:] + ".json",
        "RepoTags": ["rcat-anyflip-staging:sha-" + tag_source],
        "Layers": [],
    }]).encode()
    with tarfile.open(path, "w") as archive:
        for name, body in [
            ("manifest.json", manifest),
            (image_id[7:] + ".json", config),
        ] + ([("manifest.json", manifest)] if duplicate else []):
            info = tarfile.TarInfo(name)
            info.size = len(body)
            archive.addfile(info, io.BytesIO(body))
    file_digest = "sha256:" + hashlib.sha256(path.read_bytes()).hexdigest()
    return file_digest, image_id


class SignedDockerArchiveTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.path = Path(self.tmp.name) / FILENAME

    @patch.object(verifier, "_verify_attestation")
    def test_verified_archive_binds_source_digest_and_docker_config(self, gh):
        archive_sha, image_id = create_archive(self.path)
        result = verifier.verify_archive(self.path, SOURCE, archive_sha, image_id)
        self.assertEqual(result["status"], "signed_staging_archive_verified")
        self.assertEqual(result["image_id"], image_id)
        self.assertFalse(result["docker_image_loaded"])
        gh.assert_called_once()
        self.assertEqual(gh.call_args.args[1], SOURCE)

    @patch.object(verifier, "_verify_attestation")
    def test_wrong_archive_hash_refused_before_attestation(self, gh):
        _, image_id = create_archive(self.path)
        with self.assertRaises(verifier.ArtifactRefused):
            verifier.verify_archive(self.path, SOURCE, "sha256:" + "0" * 64, image_id)
        gh.assert_not_called()

    @patch.object(verifier, "_verify_attestation")
    def test_modified_source_label_refused(self, gh):
        archive_sha, image_id = create_archive(self.path, source="f" * 40)
        with self.assertRaises(verifier.ArtifactRefused):
            verifier.verify_archive(self.path, SOURCE, archive_sha, image_id)

    @patch.object(verifier, "_verify_attestation")
    def test_wrong_pinned_tag_refused(self, gh):
        archive_sha, image_id = create_archive(self.path, tag_source="e" * 40)
        with self.assertRaises(verifier.ArtifactRefused):
            verifier.verify_archive(self.path, SOURCE, archive_sha, image_id)

    @patch.object(verifier, "_verify_attestation")
    def test_wrong_image_id_and_duplicate_manifest_refused(self, gh):
        archive_sha, image_id = create_archive(self.path)
        with self.assertRaises(verifier.ArtifactRefused):
            verifier.verify_archive(self.path, SOURCE, archive_sha,
                                    "sha256:" + "c" * 64)
        archive_sha, image_id = create_archive(self.path, duplicate=True)
        with self.assertRaises(verifier.ArtifactRefused):
            verifier.verify_archive(self.path, SOURCE, archive_sha, image_id)

    @patch.object(verifier, "_verify_attestation", side_effect=verifier.ArtifactRefused("no signature"))
    def test_missing_attestation_refused(self, gh):
        archive_sha, image_id = create_archive(self.path)
        with self.assertRaises(verifier.ArtifactRefused):
            verifier.verify_archive(self.path, SOURCE, archive_sha, image_id)


if __name__ == "__main__":
    unittest.main()
