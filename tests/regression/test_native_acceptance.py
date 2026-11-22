"""Native checks allow bounded Kubernetes garbage collection after deletion."""
from pathlib import Path
import sys
import unittest
from unittest.mock import Mock, patch

sys.path.insert(0, str(Path(__file__).resolve().parents[1] / 'e2e'))
from native import Native


class NativeDeletionTests(unittest.TestCase):
    def test_pod_deletion_allows_asynchronous_garbage_collection(self):
        environment = Native.__new__(Native)
        environment.pods = Mock(side_effect=[[{'metadata': {'name': 'terminating'}}], []])
        with patch('native.time.sleep') as sleep:
            environment.wait_for_pod_deletion('owned-app')
        sleep.assert_called_once_with(1)
        self.assertEqual(environment.pods.call_count, 2)

    def test_pod_deletion_has_a_deadline(self):
        environment = Native.__new__(Native)
        environment.pods = Mock(return_value=[{'metadata': {'name': 'orphan'}}])
        with patch('native.time.sleep'), patch('native.time.monotonic', side_effect=[0, 0, 61]):
            with self.assertRaisesRegex(AssertionError, 'remain after 60 seconds'):
                environment.wait_for_pod_deletion('owned-app')
