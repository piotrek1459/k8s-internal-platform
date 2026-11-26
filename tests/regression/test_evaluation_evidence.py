"""Recompute the committed Pi dataset and reject lost, altered or misjoined evidence."""
import gzip
import hashlib
import json
from pathlib import Path
import unittest

from test_evaluation import analyzer, common, ROOT

DATA = ROOT/'tests/experiments/2026-11-26-core-evaluation'
REPORT = DATA.with_suffix('.json')


class ReferenceEvidenceTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.report = json.loads(REPORT.read_text())
        cls.plan = common.load_plan(DATA/'resource-plan.json')

    def test_every_retained_artifact_matches_stored_and_expanded_hash(self):
        for name, expected in self.report['artifacts'].items():
            with self.subTest(artifact=name):
                path = DATA/name
                self.assertTrue(path.resolve().is_relative_to(DATA.resolve()))
                raw = path.read_bytes()
                self.assertEqual(hashlib.sha256(raw).hexdigest(), expected['SHA256'])
                expanded = gzip.decompress(raw) if name.endswith('.gz') else raw
                self.assertEqual(hashlib.sha256(expanded).hexdigest(), expected['expandedSHA256'])

    def test_six_reported_results_recompute_from_raw_observations(self):
        self.assertEqual(self.report['resourcePlanSHA256'], common.canonical_hash(self.plan))
        self.assertEqual(len(self.report['resources']), 6)
        for expected in self.report['resources']:
            case = expected['case'];folder = DATA/'resources'/case
            actual = analyzer.analyze(folder/'host', folder/'client', json.loads((folder/'fixture.json').read_text()),
                                      self.plan, self.report['clockOffset']['offsetHostMinusClient'])
            actual.pop('inputSHA256');actual['case'] = case
            self.assertEqual(actual, expected)
            self.assertTrue(actual['targetsPassed'])
        self.assertIsNone(self.report['operatorTime'])
        self.assertFalse(self.report['coreEvaluationComplete'])

    def test_late_and_saturated_attempts_do_not_turn_into_passing_samples(self):
        folder = DATA/'excluded/late-start'
        with self.assertRaisesRegex(ValueError, 'cover'):
            analyzer.analyze(folder/'host', folder/'client', json.loads((folder/'fixture.json').read_text()),
                             self.plan, self.report['clockOffset']['offsetHostMinusClient'])
        saturated = json.loads((DATA/'excluded/four-slot/nginx-2/client/summary.json').read_text())
        self.assertFalse(saturated['clientSchedulingValid'])
        self.assertFalse(saturated['targetsPassed'])
        cases = {r['condition']: r for r in self.report['sourceDrainComparison']['cases']}
        self.assertLess(cases['control']['client']['successRate'], 1)
        self.assertEqual(cases['candidate']['client']['successRate'], 1)
        self.assertEqual(len(cases['candidate']['timeline']), 3)
