"""Focused source-only tests for the TWO-version signed archive gate."""
import importlib.util
import json
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch

FILE = Path(__file__).resolve().parents[1] / "scripts" / "verify-stage-version-pair.py"
spec = importlib.util.spec_from_file_location("verified_stage_pair", FILE)
pair = importlib.util.module_from_spec(spec)
spec.loader.exec_module(pair)

SHA_A, SHA_B = "a" * 40, "b" * 40


def entry(version, source, idx):
    return {
        "filename": f"anyflip-web_{version}_linux_amd64.docker.tar",
        "source_commit": source,
        "archive_digest": "sha256:" + str(idx) * 64,
        "image_id": "sha256:" + str(idx + 2) * 64,
    }


class PairReadOnlyTests(unittest.TestCase):
    def setUp(self):
        self.directory = tempfile.TemporaryDirectory()
        self.addCleanup(self.directory.cleanup)
        self.base = Path(self.directory.name)
        self.request = {
            "schema_version": 1,
            "kind": "rcat.anyflip.stage.signed_version_pair",
            "application": "anyflip-downloader",
            "environment": "staging",
            "from": entry("v0.2.0-rc.115", SHA_A, 1),
            "to": entry("v0.2.0-rc.116", SHA_B, 2),
        }
        self.path = self.base / "version-pair.json"

    def save(self):
        self.path.write_text(json.dumps(self.request))
        for key in ("from", "to"):
            (self.base / self.request[key]["filename"]).touch()

    def fake_verifier(self, path, source, archive, image):
        return {
            "status": "signed_staging_archive_verified",
            "source_commit": source,
            "archive_digest": archive,
            "image_id": image,
        }

    def test_two_different_signed_files_make_data_only_pair(self):
        self.save()
        with patch.object(pair.archive_verifier, "verify_archive", side_effect=self.fake_verifier) as fn:
            result = pair.verify_version_pair(self.path)
        self.assertEqual(fn.call_count, 2)
        self.assertEqual(result["status"], "two_signed_staging_archive_inputs_verified")
        for key in ("host_image_import_verified", "host_volume_backup_restore_verified",
                    "versioned_switch_authorized", "rollback_execution_enabled",
                    "production_execution_enabled"):
            self.assertFalse(result[key])

    def test_same_source_image_or_archive_refused_before_verifier(self):
        for field in ("source_commit", "image_id", "archive_digest", "filename"):
            with self.subTest(field=field):
                self.request["to"][field] = self.request["from"][field]
                self.save()
                with patch.object(pair.archive_verifier, "verify_archive") as fn:
                    with self.assertRaises(pair.VersionPairRefused):
                        pair.verify_version_pair(self.path)
                    fn.assert_not_called()
                self.request["to"] = entry("v0.2.0-rc.116", SHA_B, 2)

    def test_non_stage_and_extra_authority_refused(self):
        for update in ({"environment": "production"}, {"command": "docker load"},
                       {"schema_version": True}, {"kind": "rcat.stage.execute.request"}):
            with self.subTest(update=update):
                self.save()
                doc = json.loads(self.path.read_text())
                doc.update(update)
                self.path.write_text(json.dumps(doc))
                with patch.object(pair.archive_verifier, "verify_archive") as fn:
                    with self.assertRaises(pair.VersionPairRefused):
                        pair.verify_version_pair(self.path)
                    fn.assert_not_called()

    def test_missing_archive_and_malicious_filename_refused(self):
        self.save()
        (self.base / self.request["to"]["filename"]).unlink()
        with patch.object(pair.archive_verifier, "verify_archive") as fn:
            with self.assertRaises(pair.VersionPairRefused):
                pair.verify_version_pair(self.path)
            fn.assert_not_called()
        self.request["to"]["filename"] = "../staging-secret.env"
        # Never create the malicious filename on disk, even in this test.
        self.path.write_text(json.dumps(self.request))
        with patch.object(pair.archive_verifier, "verify_archive") as fn:
            with self.assertRaises(pair.VersionPairRefused):
                pair.verify_version_pair(self.path)
            fn.assert_not_called()

    def test_signature_failure_stops_without_false_success(self):
        self.save()
        with patch.object(pair.archive_verifier, "verify_archive",
                          side_effect=pair.archive_verifier.ArtifactRefused("no GitHub attestation")) as fn:
            with self.assertRaises(pair.archive_verifier.ArtifactRefused):
                pair.verify_version_pair(self.path)
            fn.assert_called_once()


if __name__ == "__main__":
    unittest.main()
