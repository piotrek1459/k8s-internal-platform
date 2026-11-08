"""Local command safety and image-reference regressions; no Docker daemon needed."""

import json
import os
from pathlib import Path
import subprocess
import tempfile
import unittest


ROOT = Path(__file__).resolve().parents[2]
SCRIPT = ROOT / "scripts/platform.sh"


class PlatformCommandTests(unittest.TestCase):
    def run_cli(self, *args, settings=None, docker_result=None):
        with tempfile.TemporaryDirectory() as directory:
            env = dict(os.environ)
            for key in (
                "PLATFORM_CLUSTER_NAME", "PLATFORM_REGISTRY_PORT", "PLATFORM_API_PORT",
                "PLATFORM_UI_PORT", "PLATFORM_APP_PORT",
            ):
                env.pop(key, None)
            env.update(settings or {})
            if docker_result is not None:
                docker = Path(directory) / "docker"
                docker.write_text("#!/bin/sh\nprintf '%s\\n' \"$TEST_DOCKER_RESULT\"\n")
                docker.chmod(0o755)
                env["PATH"] = f"{directory}:{env['PATH']}"
                env["TEST_DOCKER_RESULT"] = json.dumps(docker_result)
            return subprocess.run(
                ["bash", str(SCRIPT), *args], cwd=ROOT, env=env,
                capture_output=True, text=True, timeout=10,
            )

    def test_token_survives_interleaved_stdout_and_stderr(self):
        # Kubernetes may merge the structured stdout record between stderr lines.
        token = "a" * 32
        for middle in ("", '{"msg":"bootstrap_setup_available"}\n'):
            with self.subTest(interleaved=bool(middle)):
                logs = "Bootstrap token (valid 1h0m0s, single use):\n" + middle + "  " + token + "\n"
                command = '''source "$1"
require_target() { :; }
service_json() { printf '%s' '{"setupAvailable":true}'; }
local_kubectl() { printf '%s' "$TOKEN_FIXTURE"; }
print_token
'''
                result = subprocess.run(["bash", "-c", command, str(SCRIPT.with_name("token-test")), str(SCRIPT)],
                                        env=dict(os.environ, TOKEN_FIXTURE=logs), capture_output=True, text=True, timeout=10)
                self.assertEqual(result.returncode, 0, result.stderr)
                self.assertEqual(result.stdout.strip(), token)

    def test_help_does_not_require_a_configured_cluster(self):
        result = self.run_cli("--help", settings={"PLATFORM_CLUSTER_NAME": "invalid/name"})
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn("up [--no-ui]", result.stdout)

    def test_unsafe_cluster_name_is_rejected_before_any_action(self):
        result = self.run_cli("stop", settings={"PLATFORM_CLUSTER_NAME": "../../other"})
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("DNS label", result.stderr)

    def test_all_port_pairs_must_be_distinct(self):
        for key in ("PLATFORM_API_PORT", "PLATFORM_REGISTRY_PORT", "PLATFORM_APP_PORT"):
            with self.subTest(key=key):
                result = self.run_cli("up", settings={key: "8080"})
                self.assertNotEqual(result.returncode, 0)
                self.assertIn("distinct ports", result.stderr)

    def test_invalid_version_is_rejected(self):
        result = self.run_cli("demo", "v99")
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("v1, v2 or v3", result.stderr)

    def test_missing_publication_does_not_print_an_invalid_image(self):
        result = self.run_cli("image", "v1", docker_result=[])
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual(result.stdout, "")

    def test_digest_is_selected_for_the_requested_registry(self):
        result = self.run_cli("image", "v2", docker_result=[
            "localhost:5001/platform-demo@sha256:wrong-registry",
            "localhost:5002/platform-demo@sha256:expected",
        ])
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(
            result.stdout.strip(),
            "k3d-platform-local-registry:5002/platform-demo@sha256:expected",
        )


if __name__ == "__main__":
    unittest.main()
