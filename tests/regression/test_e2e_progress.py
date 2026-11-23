"""CI progress visibility and diagnostic redaction without Docker or a cluster."""
import contextlib
import io
from pathlib import Path
import subprocess
import sys
import tempfile
import threading
import unittest
from unittest.mock import patch

sys.path.insert(0, str(Path(__file__).resolve().parents[1] / "e2e"))
import support


class CommandProgressTests(unittest.TestCase):
    def test_crash_injection_refuses_an_unowned_cluster_or_node(self):
        import json
        with tempfile.TemporaryDirectory() as folder:
            environment = support.Environment(Path(folder))
            with patch.object(environment, 'run') as run:
                with self.assertRaisesRegex(RuntimeError, 'owned E2E cluster'):
                    environment.crash_broker()
                run.assert_not_called()
            environment.owns_cluster = True
            result = subprocess.CompletedProcess([], 0, json.dumps({'items': [{
                'metadata': {'name': 'broker'}, 'spec': {'nodeName': 'unrelated-node'}
            }]}), '')
            with patch.object(environment, 'kubectl', return_value=result), patch.object(environment, 'run') as run:
                with self.assertRaisesRegex(RuntimeError, 'owned E2E node'):
                    environment.crash_broker()
                run.assert_not_called()

    def test_long_command_reports_progress_without_streaming_captured_output(self):
        heartbeat = threading.Event()
        printed = []
        def report(*values, **kwargs):
            line = " ".join(map(str, values)); printed.append(line)
            if "Still running" in line: heartbeat.set()
        def execute(*args, **kwargs):
            self.assertTrue(heartbeat.wait(timeout=5), "No progress heartbeat was emitted")
            return subprocess.CompletedProcess(args[0], 0, "captured command detail\n", "")
        with tempfile.TemporaryDirectory() as folder:
            environment = support.Environment(Path(folder))
            environment.secret_values.append("test-sensitive-value")
            with patch.object(support, "PROGRESS_INTERVAL", 0.01), \
                    patch.object(support.subprocess, "run", side_effect=execute), \
                    patch("builtins.print", side_effect=report):
                result = environment.run("build", "test-sensitive-value")
            console = "\n".join(printed)
            self.assertIn("Starting", console)
            self.assertIn("Still running", console)
            self.assertIn("Exit 0", console)
            self.assertNotIn("test-sensitive-value", console)
            self.assertNotIn("captured command detail", console)
            self.assertIn("captured command detail", (Path(folder) / "commands.log").read_text())
            self.assertEqual(result.stdout, "captured command detail\n")

    def test_secret_redaction_covers_plaintext_and_kubernetes_base64_data(self):
        import base64
        with tempfile.TemporaryDirectory() as folder:
            environment = support.Environment(Path(folder))
            value = 'private-configuration-value'
            environment.secret_values.extend([value, 'second-' + value])
            encoded = base64.b64encode(value.encode()).decode()
            newer_encoded = base64.b64encode(('second-' + value).encode()).decode()
            redacted = environment.redact(value + ' ' + encoded + ' ' + newer_encoded)
            self.assertNotIn(value, redacted)
            self.assertNotIn(encoded, redacted)
            self.assertNotIn(newer_encoded, redacted)
            self.assertEqual(redacted.count('[REDACTED]'), 3)

    def test_private_command_has_no_console_or_artifact_output(self):
        with tempfile.TemporaryDirectory() as folder:
            environment = support.Environment(Path(folder))
            console = io.StringIO()
            with contextlib.redirect_stdout(console), \
                    patch.object(support.subprocess, "run", return_value=subprocess.CompletedProcess([], 0, "private result", "")):
                result = environment.run("read-token", private=True)
            self.assertEqual(console.getvalue(), "")
            self.assertFalse((Path(folder) / "commands.log").exists())
            self.assertEqual(result.stdout, "private result")

    def test_kubectl_forces_private_output_without_duplicate_keyword_arguments(self):
        for options in ({}, {"private": True}, {"private": False}):
            with self.subTest(options=options), tempfile.TemporaryDirectory() as folder:
                environment = support.Environment(Path(folder))
                console = io.StringIO()
                secret = '{"data":{"value":"encoded-secret-fixture"}}'
                with contextlib.redirect_stdout(console), \
                        patch.object(support.subprocess, "run", return_value=subprocess.CompletedProcess([], 0, secret, "")):
                    result = environment.kubectl("get", "secret", "fixture", "-o", "json", **options)
                self.assertEqual(result.stdout, secret)
                self.assertEqual(console.getvalue(), "")
                self.assertFalse((Path(folder) / "commands.log").exists())

    def test_timeout_preserves_partial_redacted_diagnostics(self):
        with tempfile.TemporaryDirectory() as folder:
            environment = support.Environment(Path(folder))
            environment.secret_values.append("test-sensitive-value")
            timeout = subprocess.TimeoutExpired(["build"], 10, output=b"partial test-sensitive-value\n", stderr=b"compiler detail\n")
            with contextlib.redirect_stdout(io.StringIO()), \
                    patch.object(support.subprocess, "run", side_effect=timeout):
                with self.assertRaisesRegex(RuntimeError, "10s timeout"):
                    environment.run("build", timeout=10)
            log = (Path(folder) / "commands.log").read_text()
            self.assertIn("partial [REDACTED]", log)
            self.assertIn("compiler detail", log)
            self.assertNotIn("test-sensitive-value", log)

    def test_private_timeout_does_not_persist_partial_output(self):
        with tempfile.TemporaryDirectory() as folder:
            environment = support.Environment(Path(folder))
            timeout = subprocess.TimeoutExpired(["read-token"], 10, output=b"private token")
            with contextlib.redirect_stdout(io.StringIO()), \
                    patch.object(support.subprocess, "run", side_effect=timeout):
                with self.assertRaisesRegex(RuntimeError, "10s timeout") as error:
                    environment.run("read-token", "private-argument-value", private=True, timeout=10)
            self.assertNotIn("private-argument-value", str(error.exception))
            self.assertFalse((Path(folder) / "commands.log").exists())
