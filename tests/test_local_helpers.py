"""Regression checks for lifecycle evidence; no running cluster is required."""

import copy
import importlib.util
import os
from pathlib import Path
import subprocess
import tempfile
import unittest


ROOT = Path(__file__).resolve().parents[1]
module_spec = importlib.util.spec_from_file_location(
    "assert_rollback", ROOT / "scripts/lib/assert_rollback.py"
)
rollback = importlib.util.module_from_spec(module_spec)
module_spec.loader.exec_module(rollback)


class RollbackEvidenceTests(unittest.TestCase):
    def setUp(self):
        self.state = [
            {"version": 3, "info": {"status": "failed"}},
            {"version": 4, "info": {"status": "deployed"}},
            {
                "metadata": {"generation": 4},
                "spec": {
                    "replicas": 1,
                    "template": {"spec": {"containers": [{"image": "platform-demo:v2"}]}},
                },
                "status": {
                    "observedGeneration": 4, "replicas": 1, "updatedReplicas": 1,
                    "readyReplicas": 1, "availableReplicas": 1,
                },
            },
            {"items": [{"spec": {
                "replicas": 0,
                "template": {"spec": {"containers": [{"image": "platform-demo:v3"}]}},
            }}]},
            {"items": [{"spec": {"containers": [{"image": "platform-demo:v2"}]}}]},
        ]

    def test_restored_release_allows_retained_replica_set_history(self):
        rollback.validate_rollback(*self.state)

    def test_surviving_v2_during_failed_upgrade_is_not_recovery(self):
        self.state[1] = copy.deepcopy(self.state[0])
        self.state[2]["spec"]["template"]["spec"]["containers"][0]["image"] = "platform-demo:v3"
        self.state[3]["items"][0]["spec"]["replicas"] = 1
        self.state[4]["items"].append({"spec": {"containers": [{"image": "platform-demo:v3"}]}})
        with self.assertRaisesRegex(AssertionError, "No new Helm revision"):
            rollback.validate_rollback(*self.state)

    def test_new_revision_must_be_deployed(self):
        self.state[1]["info"]["status"] = "pending-rollback"
        with self.assertRaisesRegex(AssertionError, "not deployed"):
            rollback.validate_rollback(*self.state)

    def test_helm_success_without_restored_deployment_is_rejected(self):
        self.state[2]["spec"]["template"]["spec"]["containers"][0]["image"] = "platform-demo:v3"
        with self.assertRaisesRegex(AssertionError, "failed image"):
            rollback.validate_rollback(*self.state)

    def test_stale_controller_status_is_rejected(self):
        self.state[2]["status"]["observedGeneration"] = 3
        with self.assertRaisesRegex(AssertionError, "not observed"):
            rollback.validate_rollback(*self.state)

    def test_incomplete_rollout_is_rejected(self):
        for field in ("replicas", "updatedReplicas", "readyReplicas", "availableReplicas"):
            with self.subTest(field=field):
                state = copy.deepcopy(self.state)
                state[2]["status"][field] = 0
                with self.assertRaisesRegex(AssertionError, field):
                    rollback.validate_rollback(*state)

    def test_failed_replica_set_must_be_scaled_down(self):
        self.state[3]["items"][0]["spec"]["replicas"] = 1
        with self.assertRaisesRegex(AssertionError, "still desired"):
            rollback.validate_rollback(*self.state)

    def test_terminating_failed_pods_must_disappear(self):
        self.state[4]["items"].append({
            "metadata": {"deletionTimestamp": "2026-10-26T12:00:00Z"},
            "spec": {"containers": [{"image": "platform-demo:v3"}]},
        })
        with self.assertRaisesRegex(AssertionError, "Pods still exist"):
            rollback.validate_rollback(*self.state)


class TranscriptTests(unittest.TestCase):
    def test_drain_and_exit_status(self):
        for command_status, logger_status, expected in ((0, 0, 0), (23, 0, 23), (0, 7, 7), (23, 7, 23)):
            with self.subTest(command=command_status, logger=logger_status), tempfile.TemporaryDirectory() as directory:
                path = Path(directory)
                # Delay writing until EOF to expose both missing waits and waits
                # performed before the pipe writers close. No Docker is needed.
                tee = path / "tee"
                tee.write_text(
                    "#!/usr/bin/env python3\n"
                    "import os, pathlib, sys, time\n"
                    "data = sys.stdin.buffer.read()\n"
                    "time.sleep(0.2)\n"
                    "pathlib.Path(sys.argv[1]).write_bytes(data)\n"
                    "sys.exit(int(os.environ['TEST_LOGGER_STATUS']))\n"
                )
                tee.chmod(0o755)
                env = dict(os.environ, PATH=f"{path}:{os.environ['PATH']}", TEST_LOGGER_STATUS=str(logger_status))
                result = subprocess.run(
                    ["bash", "-c", '''
set -euo pipefail
source "$1"
start_transcript "$2"
printf 'stdout\\n'
printf 'stderr\\n' >&2
status="$3"
finish_transcript "$status" || status=$?
# This check runs immediately after finishing, while delayed writers would
# still be alive if finish_transcript did not wait for them.
[[ "$(cat "$2")" == $'stdout\\nstderr' ]] || exit 99
exit "$status"
''', "transcript-test", str(ROOT / "scripts/lib/transcript.sh"), str(path / "transcript.log"), str(command_status)],
                    env=env, stdout=subprocess.DEVNULL, stderr=subprocess.PIPE,
                    timeout=5, text=True,
                )
                self.assertEqual(result.returncode, expected, result.stderr)
                self.assertEqual((path / "transcript.log").read_text(), "stdout\nstderr\n")


if __name__ == "__main__":
    unittest.main()
