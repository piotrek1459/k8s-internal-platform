"""Manual-release identity and publication safety without remote mutations."""
import importlib.util
import json
import os
from pathlib import Path
import subprocess
import tempfile
import unittest
from unittest.mock import patch

ROOT = Path(__file__).resolve().parents[2]
SPEC = importlib.util.spec_from_file_location("release_workflow", ROOT / "release/workflow.py")
workflow = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(workflow)
SHA = "a" * 40
OTHER = "b" * 40


class ReleasePreparationTests(unittest.TestCase):
    def test_only_valid_release_versions_are_accepted(self):
        self.assertEqual(workflow.normalize_version("0.0.2"), "v0.0.2")
        for value in ("v0.0.2", "v1.20.300"):
            self.assertEqual(workflow.normalize_version(value), value)
        for value in ("v01.0.2", "v0.0.2;exit", "v0.0.2\n", "v0.0.2-alpha", "main"):
            with self.subTest(value=value), self.assertRaises(ValueError):
                workflow.normalize_version(value)

    def test_selected_branch_head_is_resolved_once_and_version_is_input_driven(self):
        with patch.object(workflow, "command", side_effect=["feature/deployment", SHA + "\trefs/heads/feature/deployment"] ) as command, \
                patch.object(workflow, "check_remote_identity") as identity:
            result = workflow.prepare("v0.0.2", "feature/deployment")
        self.assertEqual(result, {"version": "v0.0.2", "app_version": "0.0.2", "sha": SHA, "branch": "feature/deployment"})
        self.assertEqual(command.call_args_list[1].args, ("git", "ls-remote", "--heads", "origin", "refs/heads/feature/deployment"))
        identity.assert_called_once_with("v0.0.2", SHA)

    def test_a_tag_cannot_substitute_for_a_missing_source_branch(self):
        with patch.object(workflow, "command", side_effect=["v0.0.2", ""]), \
                patch.object(workflow, "check_remote_identity") as identity:
            with self.assertRaisesRegex(ValueError, "branch does not exist"):
                workflow.prepare("v0.0.2", "v0.0.2")
        identity.assert_not_called()

    def test_annotated_tags_are_compared_using_their_commit(self):
        with patch.object(workflow, "command", return_value=OTHER + "\trefs/tags/v0.0.2\n" + SHA + "\trefs/tags/v0.0.2^{}"):
            self.assertEqual(workflow.remote_tag("v0.0.2"), SHA)

    def test_tag_conflict_published_release_and_foreign_draft_are_rejected(self):
        cases = [(OTHER, None), (SHA, {"draft": False, "target_commitish": SHA}),
                 (SHA, {"draft": True, "target_commitish": OTHER})]
        for target, release in cases:
            with self.subTest(target=target, release=release), \
                    patch.object(workflow, "remote_tag", return_value=target), \
                    patch.object(workflow, "find_release", return_value=release):
                with self.assertRaises(ValueError):
                    workflow.check_remote_identity("v0.0.2", SHA)

    def test_matching_unpublished_draft_can_be_retried(self):
        draft = {"draft": True, "target_commitish": SHA}
        with patch.object(workflow, "remote_tag", return_value=SHA), \
                patch.object(workflow, "find_release", return_value=draft):
            self.assertEqual(workflow.check_remote_identity("v0.0.2", SHA), draft)

    def test_drafts_and_old_versions_are_found_across_pages(self):
        first = [{"tag_name": "v1.0." + str(i)} for i in range(100)]
        draft = {"tag_name": "v0.0.2", "draft": True}
        with patch.object(workflow, "api", side_effect=[first, [draft]]) as api:
            self.assertEqual(workflow.find_release("v0.0.2"), draft)
        self.assertEqual(api.call_count, 2)

    def test_api_failures_are_not_interpreted_as_a_missing_release(self):
        for status, code in (("403 Forbidden", 1), ("404 Not Found", 1), ("429 Too Many Requests", 1), (None, 1)):
            raw = "" if status is None else "HTTP/2.0 " + status + "\nContent-Type: application/json\n\n{}"
            with self.subTest(status=status), patch.dict(os.environ, GH_REPO="owner/repo"), \
                    patch.object(workflow.subprocess, "run", return_value=subprocess.CompletedProcess([], code, raw, "API failed")):
                with self.assertRaises(RuntimeError):
                    workflow.api("releases?per_page=100&page=1")

    def test_api_headers_are_separated_from_json(self):
        raw = 'HTTP/2.0 200 OK\nContent-Type: application/json\n\n[{"tag_name":"v0.0.2"}]\n'
        with patch.dict(os.environ, GH_REPO="owner/repo"), \
                patch.object(workflow.subprocess, "run", return_value=subprocess.CompletedProcess([], 0, raw, "")):
            self.assertEqual(workflow.api("releases"), [{"tag_name": "v0.0.2"}])


class ReleasePublicationTests(unittest.TestCase):
    def assets(self, folder, commit=SHA, prerelease=False):
        for name in ("platform-v0.0.2-linux-arm64.tar.gz", "platform-v0.0.2-linux-amd64.tar.gz", "install.sh", "INSTALL.md", "SHA256SUMS"):
            (folder / name).write_text("fixture")
        (folder / "release.json").write_text(json.dumps({"version": "0.0.2", "commit": commit, "preview": prerelease}))

    def simulate(self, folder, fail_upload=False, prerelease=False):
        state = {"tag": None, "release": None, "events": []}
        def api(path, method="GET", fields=None):
            state["events"].append(("tag", fields["sha"]))
            state["tag"] = fields["sha"]
        def command(*args, **kwargs):
            state["events"].append(args)
            if args[:3] == ("gh", "release", "create"):
                state["release"] = {"draft": True, "target_commitish": SHA}
            if args[:3] == ("gh", "release", "upload") and fail_upload:
                raise RuntimeError("upload failed")
            if args[:3] == ("gh", "release", "edit"):
                state["release"]["draft"] = False
            return ""
        with patch.object(workflow, "api", side_effect=api), \
                patch.object(workflow, "remote_tag", side_effect=lambda _: state["tag"]), \
                patch.object(workflow, "find_release", side_effect=lambda _: state["release"]), \
                patch.object(workflow, "command", side_effect=command):
            if fail_upload:
                with self.assertRaisesRegex(RuntimeError, "upload failed"):
                    workflow.publish(folder, "v0.0.2", SHA, prerelease)
            else:
                workflow.publish(folder, "v0.0.2", SHA, prerelease)
        return state

    def test_publication_creates_the_tested_tag_and_uploads_before_publishing(self):
        with tempfile.TemporaryDirectory() as directory:
            folder = Path(directory); self.assets(folder)
            state = self.simulate(folder)
        self.assertEqual(state["events"][0], ("sha256sum", "--check", "SHA256SUMS"))
        self.assertEqual(state["events"][1], ("tag", SHA))
        create = state["events"][2]
        self.assertEqual(create[create.index("--target") + 1], SHA)
        self.assertEqual(state["events"][3][:3], ("gh", "release", "upload"))
        self.assertEqual(state["events"][-1][:3], ("gh", "release", "edit"))
        self.assertFalse(state["release"]["draft"])
        self.assertNotIn("--prerelease", create)
        self.assertEqual(create[create.index("--title") + 1], "v0.0.2")
        self.assertIn("--prerelease=false", state["events"][-1])
        self.assertIn("--latest=true", state["events"][-1])

    def test_prerelease_is_explicit_and_not_marked_latest(self):
        with tempfile.TemporaryDirectory() as directory:
            folder = Path(directory); self.assets(folder, prerelease=True)
            state = self.simulate(folder, prerelease=True)
        self.assertIn("--prerelease", state["events"][2])
        self.assertIn("--prerelease=true", state["events"][-1])
        self.assertIn("--latest=false", state["events"][-1])

    def test_metadata_kind_mismatch_prevents_remote_writes(self):
        with tempfile.TemporaryDirectory() as directory, patch.object(workflow, "api") as api:
            folder = Path(directory); self.assets(folder, prerelease=True)
            with self.assertRaisesRegex(ValueError, "prerelease setting"):
                workflow.publish(folder, "v0.0.2", SHA)
        api.assert_not_called()

    def test_failed_upload_leaves_an_unpublished_draft(self):
        with tempfile.TemporaryDirectory() as directory:
            folder = Path(directory); self.assets(folder)
            state = self.simulate(folder, fail_upload=True)
        self.assertTrue(state["release"]["draft"])
        self.assertFalse(any(event[:3] == ("gh", "release", "edit") for event in state["events"]))

    def test_missing_assets_prevent_tag_creation(self):
        with tempfile.TemporaryDirectory() as directory, patch.object(workflow, "api") as api:
            with self.assertRaisesRegex(ValueError, "six release assets"):
                workflow.publish(Path(directory), "v0.0.2", SHA)
        api.assert_not_called()

    def test_checksum_failure_prevents_remote_writes(self):
        with tempfile.TemporaryDirectory() as directory, patch.object(workflow, "api") as api:
            folder = Path(directory); self.assets(folder)
            with patch.object(workflow, "command", side_effect=RuntimeError("checksum mismatch")):
                with self.assertRaisesRegex(RuntimeError, "checksum mismatch"):
                    workflow.publish(folder, "v0.0.2", SHA)
        api.assert_not_called()

    def test_matching_draft_retry_does_not_recreate_its_tag_or_draft(self):
        draft = {"draft": True, "target_commitish": SHA}
        with tempfile.TemporaryDirectory() as directory, \
                patch.object(workflow, "remote_tag", return_value=SHA), \
                patch.object(workflow, "find_release", return_value=draft), \
                patch.object(workflow, "api") as api, \
                patch.object(workflow, "command", return_value="") as command:
            folder = Path(directory); self.assets(folder)
            workflow.publish(folder, "v0.0.2", SHA)
        api.assert_not_called()
        operations = [call.args[:3] for call in command.call_args_list]
        self.assertEqual(operations, [("sha256sum", "--check", "SHA256SUMS"),
                                     ("gh", "release", "upload"), ("gh", "release", "edit")])

    def test_assets_from_another_commit_prevent_tag_creation(self):
        with tempfile.TemporaryDirectory() as directory, patch.object(workflow, "api") as api:
            folder = Path(directory); self.assets(folder, commit=OTHER)
            with self.assertRaisesRegex(ValueError, "tested version/source"):
                workflow.publish(folder, "v0.0.2", SHA)
        api.assert_not_called()
