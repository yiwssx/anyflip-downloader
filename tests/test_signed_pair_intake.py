"""Only the new ZIP -> signed Stage pair intake boundary; no Host or Docker."""
import hashlib
import importlib.util
import io
import json
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch
from zipfile import ZipFile

FILE = Path(__file__).resolve().parents[1] / "scripts" / "accept-signed-stage-pair.py"
spec = importlib.util.spec_from_file_location("stage_pair_intake", FILE)
intake = importlib.util.module_from_spec(spec)
spec.loader.exec_module(intake)


class SignedPairIntakeTests(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.TemporaryDirectory()
        self.addCleanup(self.dir.cleanup)
        self.root = Path(self.dir.name)
        self.sha = "a" * 40
        self.name = "anyflip-web_v0.2.0-rc.115_linux_amd64.docker.tar"
        self.payload = b"synthetic immutable Docker transport bytes"
        self.pair = {
            "from": {
                "filename": self.name,
                "source_commit": self.sha,
                "archive_digest": "sha256:" + hashlib.sha256(self.payload).hexdigest(),
                "image_id": "sha256:" + "f" * 64,
            }
        }
        self.zip = self.root / "candidate.zip"
        self.extract = self.root / "store"
        self.extract.mkdir()

    def make_zip(self, *, source=None, archive=None, image=None):
        name = "staging/" + self.name
        with ZipFile(self.zip, "w") as z:
            z.writestr(name, self.payload if archive is None else archive)
            z.writestr(name + ".sha256",
                       hashlib.sha256(self.payload).hexdigest() + "  " + self.name + "\n")
            z.writestr(name + ".source-sha", (source or self.sha) + "\n")
            z.writestr(name + ".image-id", (image or self.pair["from"]["image_id"]) + "\n")
        return hashlib.sha256(self.zip.read_bytes()).hexdigest()

    def test_exact_zip_and_inner_receipts_stage_only_bytes(self):
        digest = self.make_zip()
        with patch.dict(intake.ARCHIVES, {115: (digest, self.sha)}):
            intake._extract_candidate(self.zip, 115, self.pair, self.extract)
        self.assertEqual((self.extract / self.name).read_bytes(), self.payload)
        self.assertEqual((self.extract / self.name).stat().st_mode & 0o777, 0o600)

    def test_zip_modified_after_review_refused(self):
        self.make_zip()
        with patch.dict(intake.ARCHIVES, {115: ("0" * 64, self.sha)}):
            with self.assertRaises(intake.IntakeRefused):
                intake._extract_candidate(self.zip, 115, self.pair, self.extract)
        self.assertEqual(list(self.extract.iterdir()), [])

    def test_source_receipt_and_real_archive_disagree_refused(self):
        for changes in ({"source": "b" * 40}, {"archive": b"modified transport"},
                        {"image": "sha256:" + "e" * 64}):
            with self.subTest(changes=changes):
                self.make_zip(**changes)
                digest = hashlib.sha256(self.zip.read_bytes()).hexdigest()
                with patch.dict(intake.ARCHIVES, {115: (digest, self.sha)}):
                    with self.assertRaises(intake.IntakeRefused):
                        intake._extract_candidate(self.zip, 115, self.pair, self.extract)
                for f in self.extract.iterdir():
                    f.unlink()

    def test_no_host_authority_without_root(self):
        with patch.object(intake.os, "geteuid", return_value=1000):
            with self.assertRaises(intake.IntakeRefused):
                intake._host_guard()


if __name__ == "__main__":
    unittest.main()
