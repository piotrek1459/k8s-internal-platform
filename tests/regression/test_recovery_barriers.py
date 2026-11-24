"""Crash fixtures must reject a user cluster before touching runtime resources."""
import json
import sys
import unittest
from pathlib import Path
from types import SimpleNamespace
from unittest.mock import Mock, patch

sys.path.insert(0,str(Path(__file__).resolve().parents[1]/'e2e'))
from recovery_matrix import Barrier, frozen_identity
from support import Environment


class CrashFixtureSafetyTests(unittest.TestCase):
    def test_barrier_refuses_non_owned_or_non_test_cluster_before_initialization(self):
        for owned,name in ((False,'platform-e2e-fixture'),(True,'platform-local')):
            with self.subTest(owned=owned,name=name),patch('recovery_matrix.tempfile.TemporaryDirectory') as directory:
                with self.assertRaisesRegex(RuntimeError,'owned disposable'):
                    Barrier(SimpleNamespace(owns_cluster=owned,name=name))
                directory.assert_not_called()

    def test_crash_refuses_foreign_node_before_killing_or_releasing_gate(self):
        e=Environment.__new__(Environment);e.owns_cluster=True;e.name='platform-e2e-fixture'
        e.kubectl=Mock(return_value=SimpleNamespace(stdout=json.dumps({'items':[{'metadata':{'name':'platform-fixture'},'spec':{'nodeName':'production-node'}}]})))
        e.run=Mock();callback=Mock()
        with self.assertRaisesRegex(RuntimeError,'owned E2E node'):
            e.crash_broker(after_kill=callback)
        e.run.assert_not_called();callback.assert_not_called()

    def test_partial_snapshot_cannot_establish_a_crash_barrier(self):
        complete={k:'controlled' for k in ('image','chartVersion','chart','values','frozenAt')}
        for key in complete:
            with self.subTest(field=key):
                with self.assertRaisesRegex(AssertionError,'complete durable'):
                    frozen_identity({'frozen':{**complete,key:''}})
