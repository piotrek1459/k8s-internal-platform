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


class PublishedReferenceEvidenceTests(unittest.TestCase):
    def test_v0111_artifacts_and_six_joined_load_results(self):
        data = ROOT/'tests/experiments/2026-11-27-v0.1.11-native'
        report = json.loads(Path(str(data) + '.json').read_text())
        plan = common.load_plan(data/'resource-plan.json')
        self.assertEqual(report['runtimeRelease'], '0.1.11')
        self.assertEqual(report['resourcePlanSHA256'], common.canonical_hash(plan))
        self.assertFalse(report['publishedDefaultValidationPending'])
        self.assertTrue(report['publishedDefaultValidation']['resourcesOmittedAtCreation'])
        self.assertEqual(report['publishedDefaultValidation']['nativeSleepSeconds'], 5)
        self.assertIsNone(report['operatorTime'])
        self.assertFalse(report['coreEvaluationComplete'])
        for name, expected in report['artifacts'].items():
            with self.subTest(artifact=name):
                path = data/name
                self.assertTrue(path.resolve().is_relative_to(data.resolve()))
                raw = path.read_bytes()
                self.assertEqual(hashlib.sha256(raw).hexdigest(), expected['SHA256'])
                expanded = gzip.decompress(raw) if name.endswith('.gz') else raw
                self.assertEqual(hashlib.sha256(expanded).hexdigest(), expected['expandedSHA256'])
        self.assertEqual(len(report['resources']), 6)
        for expected in report['resources']:
            folder = data/'resources'/expected['case']
            actual = analyzer.analyze(folder/'host', folder/'client', json.loads((folder/'fixture.json').read_text()),
                                      plan, report['clockOffset']['offsetHostMinusClient'])
            actual.pop('inputSHA256'); actual['case'] = expected['case']
            self.assertEqual(actual, expected)
            self.assertTrue(actual['targetsPassed'])
        timings = json.loads((data/'lifecycle/timings.json').read_text())
        self.assertEqual(common.lifecycle_acceptance(timings, plan, True), report['lifecycle']['acceptance'])
        requests = analyzer.rows(data/'availability/client/app/requests.jsonl')
        self.assertEqual(len(requests), 375)
        self.assertEqual(sum(row['success'] for row in requests), 373)
        failures = [row for row in requests if not row['success']]
        self.assertEqual([row['status'] for row in failures], [502, None])
        self.assertFalse(report['publishedRolloutContinuityPassed'])
        timeline = json.loads((data/'availability/timeline.json').read_text())
        broker_start = next(row['epochSeconds'] for row in timeline if row['stage'] == 'broker-delete-submitted')
        self.assertTrue(all(row['startedEpochSeconds'] + row['latencyMilliseconds']/1000 < broker_start for row in failures))
        continuity = report['requestContinuity']
        self.assertTrue(continuity['complete'])
        self.assertTrue(continuity['clientSchedulingValid'])
        self.assertEqual(continuity['successful'], 373)
        self.assertFalse(continuity['targetsPassed'])
        self.assertEqual(continuity['requests'], 375)
        before = json.loads((data/'retention/kube-before-retention.json').read_text())
        after = json.loads((data/'retention/kube-after-reboot.json').read_text())
        self.assertNotEqual(before['bootID'], after['bootID'])
        for field in report['reinstall']['identicalFields']:
            self.assertEqual(before[field], after[field], field)


class RoutingCorrectionEvidenceTests(unittest.TestCase):
    def test_control_failure_and_candidate_requests_recompute_from_raw_evidence(self):
        data = ROOT/'tests/experiments/2026-11-27-rollout-routing'
        report = json.loads(Path(str(data) + '.json').read_text())
        self.assertTrue(report['publishedReleaseAcceptancePending'])
        self.assertIsNone(report['humanOperatorTime'])
        self.assertFalse(report['coreEvaluationComplete'])
        for name, expected in report['artifacts'].items():
            with self.subTest(artifact=name):
                path = data/name
                self.assertTrue(path.resolve().is_relative_to(data.resolve()))
                raw = path.read_bytes()
                self.assertEqual(hashlib.sha256(raw).hexdigest(), expected['SHA256'])
                expanded = gzip.decompress(raw) if name.endswith('.gz') else raw
                self.assertEqual(hashlib.sha256(expanded).hexdigest(), expected['expandedSHA256'])
        self.assertEqual(len(report['cases']), 5)
        for case in report['cases']:
            for policy, expected in case['clients'].items():
                records = analyzer.rows(data/case['case']/'client'/policy/'requests.jsonl')
                actual = analyzer.summarize(records, expected_count=275, expected_total=275)
                for key, value in actual.items():
                    self.assertEqual(value, expected[key], (case['case'], policy, key))
                self.assertTrue(actual['complete'])
                self.assertTrue(actual['clientSchedulingValid'])
                self.assertEqual(actual['successful'], 230 if case['case'] == 'control-slow' else 275)
        fixture = json.loads((data/'native-verify/fixture.json').read_text())
        expected_url = 'http://' + fixture['serviceSpec']['clusterIP'] + ':80'
        configs = analyzer.rows(data/'native-verify/proxy-services.jsonl')
        urls = {s['url'] for r in configs for service in r.get('services', []) for s in service['loadBalancer']['servers']}
        self.assertEqual(urls, {expected_url})
        control = json.loads((data/'control-slow/fixture.json').read_text())
        # kubectl logs also contains ordinary startup lines; access records are JSON.
        raw_access = gzip.decompress((data/'control-slow/proxy-access.jsonl.gz').read_bytes()).decode()
        access = [json.loads(line) for line in raw_access.splitlines() if line.startswith('{')]
        failures = [r for r in access if control['oldPodIP'] in r.get('ServiceAddr', '') and r.get('DownstreamStatus') != 200]
        self.assertEqual(len(failures), 90)
        audit = json.loads((data/'final-audit.json').read_text())
        self.assertTrue(audit['passed'])
        self.assertTrue(audit['originalPrivateTemplateRestored'])


class PublishedV0112EvidenceTests(unittest.TestCase):
    def test_published_acceptance_recomputes_without_supplying_human_results(self):
        data = ROOT/'tests/experiments/2026-11-27-v0.1.12-native'
        report = json.loads(Path(str(data) + '.json').read_text())
        self.assertEqual(report['runtimeRelease'], '0.1.12')
        self.assertTrue(report['publishedNativeAcceptancePassed'])
        self.assertTrue(report['publishedRolloutContinuityPassed'])
        self.assertIsNone(report['humanOperatorTime'])
        self.assertFalse(report['coreEvaluationComplete'])
        for name, expected in report['artifacts'].items():
            with self.subTest(artifact=name):
                path = data/name
                self.assertTrue(path.resolve().is_relative_to(data.resolve()))
                raw = path.read_bytes()
                self.assertEqual(len(raw), expected['bytes'])
                self.assertEqual(hashlib.sha256(raw).hexdigest(), expected['SHA256'])
                expanded = gzip.decompress(raw) if name.endswith('.gz') else raw
                self.assertEqual(hashlib.sha256(expanded).hexdigest(), expected['expandedSHA256'])
        self.assertEqual(len(report['cases']), 7)
        counts = {'rollout': 0, 'steady': 0, 'restoration': 0}
        for case in report['cases']:
            folder = data/case['case']
            policy = json.loads((folder/'private-policy.json').read_text())['arguments']
            self.assertEqual(policy.count('--providers.kubernetesingress.nativelbbydefault=true'), 1)
            self.assertFalse(any(arg.startswith(('--api.insecure=', '--providers.kubernetesingress.throttleduration=')) for arg in policy))
            summary = json.loads((folder/'summary.json').read_text())
            self.assertTrue(summary['passed'])
            self.assertTrue(summary['originalApplicationsRetained'])
            self.assertEqual(summary['owned']['applications'], {})
            self.assertEqual(summary['owned']['releases'], {})
            for client, expected in case['clients'].items():
                with self.subTest(case=case['case'], client=client):
                    records = analyzer.rows(folder/'client'/client/'requests.jsonl')
                    rate = expected['ratePerSecond']
                    actual = analyzer.summarize(records,
                                                expected_count=rate * expected['durationSeconds'],
                                                expected_total=rate * (expected['durationSeconds'] + expected['warmupSeconds']))
                    for key, value in actual.items():
                        self.assertEqual(value, expected[key], key)
                    self.assertTrue(actual['complete'])
                    self.assertTrue(actual['clientSchedulingValid'])
                    self.assertTrue(expected['certificateVerification'])
                    if client == 'management':
                        self.assertEqual(case['case'], 'broker-continuity')
                        self.assertEqual(actual['successful'], 367)
                        self.assertEqual(actual['requests'], 375)
                        self.assertFalse(actual['targetsPassed'])
                        continue
                    self.assertTrue(actual['targetsPassed'])
                    group = ('steady' if case['case'].startswith('steady-') else
                             'restoration' if case['case'] == 'unhealthy-restoration' else 'rollout')
                    counts[group] += actual['successful']
                    if case['case'].startswith('routing-'):
                        timeline = json.loads((folder/'timeline.json').read_text())
                        self.assertEqual([row['action'] for row in timeline], ['update', 'rollback', 'restart'])
                        self.assertLessEqual(min(r['startedEpochSeconds'] for r in records), timeline[0]['submitted'])
                        self.assertGreaterEqual(max(r['startedEpochSeconds'] for r in records), max(row['retired'] for row in timeline) + 2)
                        for action in timeline:
                            self.assertTrue(action['oldPodUIDs'])
                            self.assertLessEqual(action['ready'] - action['submitted'], 30)
                            self.assertGreaterEqual(action['retired'], action['ready'])
        self.assertEqual(counts, {'rollout': 2400, 'steady': 2400, 'restoration': 660})
        timeline = json.loads((data/'broker-continuity/timeline.json').read_text())
        self.assertEqual(timeline[-1]['stage'], 'broker-ready-original-session-valid')
        self.assertNotEqual(timeline[-2]['oldPodUID'], timeline[-1]['newPodUID'])
        timing = json.loads((data/'unhealthy-restoration/timing.json').read_text())
        self.assertEqual(timing['failurePolicySeconds'], 300)
        self.assertGreaterEqual(timing['elapsedSeconds'], 300)
        self.assertLessEqual(timing['elapsedSeconds'], 360)
        self.assertEqual(timing['states'][-1]['state'], 'failed')
        self.assertTrue(json.loads((data/'unhealthy-restoration/summary.json').read_text())['verifiedRestoration'])
        before = json.loads((data/'retention/kube-before-retention.json').read_text())
        after = json.loads((data/'retention/kube-after-reboot.json').read_text())
        self.assertNotEqual(before['bootID'], after['bootID'])
        for field in report['reinstall']['identicalFields']:
            self.assertEqual(before[field], after[field], field)
        self.assertFalse(report['reboot']['SSHOrManualServiceStartBeforeReady'])
        self.assertTrue(json.loads((data/'retention/final-audit.json').read_text())['passed'])
        self.assertTrue(json.loads((data/'retention/access-cleanup.json').read_text())['passed'])
