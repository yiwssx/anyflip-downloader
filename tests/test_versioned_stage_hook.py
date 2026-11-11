"""Only NEW bounded Stage L/A/B version-switch code paths, no real Docker."""
import hashlib
import importlib.util
import json
from pathlib import Path
import unittest
from unittest.mock import Mock, patch

FILE = Path(__file__).resolve().parents[1] / "deploy/staging/anyflip-versioned-fixed-hook.py"
spec = importlib.util.spec_from_file_location("anyflip_versioned_fixed_hook", FILE)
hook = importlib.util.module_from_spec(spec)
spec.loader.exec_module(hook)

BACKUP = {"data_sha256": "sha256:" + "e" * 64, "scratch_restored": True}


def valid_evidence(source="A", target="B"):
    op = {"kind": "rcat.anyflip.stage.versioned.operation", "schema_version": 1,
          "from": source, "to": target,
          "from_image_id": hook.VERSIONS[source][1],
          "to_image_id": hook.VERSIONS[target][1],
          "version_pair_sha256": hook.PAIR_HASH,
          "legacy_backup_sha256": BACKUP["data_sha256"]}
    op_hash = "sha256:" + hashlib.sha256(
        json.dumps(op, sort_keys=True, separators=(",", ":"), ensure_ascii=True)
        .encode("ascii")).hexdigest()
    approval = {
        "provenance": "fetched_via_authenticated_private_github_api",
        "approval_evidence_status": "structurally_valid",
        "identity_separated": True, "approver_actor_id": 48784420,
        "executor_actor_id": 354878936, "github_issue_number": 501,
        "github_comment_id": 50002, "plan_sha256": "a" * 64,
        "versioned_operation_digest": op_hash,
    }
    replay = {
        "approval_recorded_once": True, "github_comment_id": 50002,
        "plan_sha256": "a" * 64, "versioned_operation_digest": op_hash,
    }
    return approval, replay


class VersionedHookTests(unittest.TestCase):
    def test_same_image_or_unauthorized_transition_rejected_before_host(self):
        approval, replay = valid_evidence()
        with patch.object(hook, "_approved_inputs") as read:
            for source,target in (("A","A"),("legacy","B"),("B","legacy-but-fake"),
                                  ("B","unknown")):
                with self.subTest(source=source,target=target):
                    with self.assertRaises(hook.VersionedStageRefused):
                        hook.switch_with_authenticated_approval(
                            source,target,approval=approval,replay=replay)
            read.assert_not_called()

    def test_approval_must_be_live_private_and_replay_consumed(self):
        approval, replay = valid_evidence()
        hook._check_fresh_approval(approval, replay, source="A", target="B", receipt=BACKUP)
        for key,value in [
            ("provenance", "user_supplied_json"), ("approver_actor_id", 354878936),
            ("executor_actor_id", 354372345), ("identity_separated", False),
            ("versioned_operation_digest", "sha256:" + "c"*64),
        ]:
            changed = dict(approval, **{key:value})
            with self.subTest(key=key), self.assertRaises(hook.VersionedStageRefused):
                hook._check_fresh_approval(changed,replay,source="A",target="B",receipt=BACKUP)
        for key,value in [
            ("approval_recorded_once", False),("github_comment_id", 60000),
            ("plan_sha256", "b"*64),("versioned_operation_digest", "sha256:" + "b"*64),
        ]:
            changed = dict(replay, **{key:value})
            with self.subTest(key=key), self.assertRaises(hook.VersionedStageRefused):
                hook._check_fresh_approval(approval,changed,source="A",target="B",receipt=BACKUP)

    def test_signed_source_pair_cannot_approve_reverse_direction(self):
        approval,replay = valid_evidence("A","B")
        with self.assertRaises(hook.VersionedStageRefused):
            hook._check_fresh_approval(approval,replay,source="B",target="A",receipt=BACKUP)

    def test_one_approved_a_to_b_never_touches_other_resources(self):
        approval,replay = valid_evidence()
        with patch.object(hook, "_approved_inputs",return_value=BACKUP), \
             patch.object(hook, "_running_stage",side_effect=["A","B"]) as stage, \
             patch.object(hook, "_image_ready") as image, \
             patch.object(hook, "_idle") as idle, \
             patch.object(hook, "_docker",return_value="") as docker:
            result = hook.switch_with_authenticated_approval(
                "A","B",approval=approval,replay=replay)
        self.assertEqual(result["status"], "stage_version_switch_passed")
        self.assertFalse(result["production_execution_enabled"])
        self.assertEqual(stage.call_count,2)
        self.assertEqual(image.call_count,2)
        self.assertEqual(idle.call_count,2)
        docker.assert_called_once()
        args=docker.call_args.args[0]
        self.assertIn("--force-recreate",args)
        self.assertIn("--pull",args)
        self.assertNotIn("down",args)
        self.assertNotIn("--volumes",args)
        self.assertEqual(docker.call_args.kwargs["image"], hook._tag("B"))

    def test_switch_failure_restores_previous_image_not_new_image(self):
        approval,replay=valid_evidence()
        def on_compose(args, *,image=None):
            if image == hook._tag("B"):
                raise hook.VersionedStageRefused("new Stage failed")
            return ""
        with patch.object(hook,"_approved_inputs",return_value=BACKUP), \
             patch.object(hook,"_running_stage",side_effect=["A","A"]), \
             patch.object(hook,"_image_ready"), \
             patch.object(hook,"_idle"), \
             patch.object(hook,"_docker",side_effect=on_compose) as docker:
            with self.assertRaisesRegex(hook.VersionedStageRefused,
                                        "old image restored, approval consumed"):
                hook.switch_with_authenticated_approval("A","B",approval=approval,replay=replay)
        self.assertEqual(docker.call_count,2)
        self.assertEqual(docker.call_args.kwargs["image"],hook._tag("A"))

    def test_running_image_must_be_actual_reviewed_stage(self):
        approval,replay=valid_evidence()
        with patch.object(hook,"_approved_inputs",return_value=BACKUP), \
             patch.object(hook,"_running_stage",return_value="legacy"), \
             patch.object(hook,"_image_ready") as images, \
             patch.object(hook,"_docker") as docker:
            with self.assertRaisesRegex(hook.VersionedStageRefused,"actual Stage image"):
                hook.switch_with_authenticated_approval("A","B",approval=approval,replay=replay)
            images.assert_not_called()
            docker.assert_not_called()


if __name__ == "__main__":
    unittest.main()
