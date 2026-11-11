"""New Staging-only snapshot trust boundary: offline, no Docker/Host access."""
import importlib.util
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch

SCRIPT = Path(__file__).resolve().parents[1] / "deploy/staging/stopped-stage-snapshot.py"
spec = importlib.util.spec_from_file_location("stage_snapshot", SCRIPT)
stage = importlib.util.module_from_spec(spec)
spec.loader.exec_module(stage)


class StageSnapshotTests(unittest.TestCase):
    def setUp(self):
        d = tempfile.TemporaryDirectory()
        self.addCleanup(d.cleanup)
        self.root = Path(d.name)
        self.data = self.root / "stage"
        self.data.mkdir()
        (self.data / "jobs").mkdir()
        (self.data / "jobs/job-synthetic.pdf").write_bytes(b"synthetic stage only")

    def test_original_and_root_only_copy_have_identical_hash(self):
        before, total = stage._tree_receipt(self.data)
        self.assertEqual(total, len(b"synthetic stage only"))
        target = self.root / "backup"
        with patch.object(stage.os, "chown"):
            stage._copy_stage_tree(self.data, target)
        after, copied_size = stage._tree_receipt(target)
        self.assertEqual((after, copied_size), (before, total))
        (target / "jobs/job-synthetic.pdf").write_bytes(b"tampered")
        self.assertNotEqual(stage._tree_receipt(target)[0], before)

    def test_stage_symlinks_refused_and_copied_data_not_followed(self):
        (self.data / "jobs/escape").symlink_to("/etc/passwd")
        with self.assertRaises(stage.SnapshotRefused):
            stage._tree_receipt(self.data)

    def test_stage_fifo_refused(self):
        import os
        os.mkfifo(self.data / "jobs/fifo")
        with self.assertRaises(stage.SnapshotRefused):
            stage._tree_receipt(self.data)

    def test_cannot_overwrite_existing_snapshot(self):
        target = self.root / "already-backed-up"
        target.mkdir()
        with self.assertRaises(stage.SnapshotRefused):
            stage._copy_stage_tree(self.data, target)

    def test_non_root_operator_refused_before_docker(self):
        with patch.object(stage.os, "geteuid", return_value=10001):
            with patch.object(stage, "_command") as cmd:
                with self.assertRaises(stage.SnapshotRefused):
                    stage._baseline()
                cmd.assert_not_called()


if __name__ == "__main__":
    unittest.main()
