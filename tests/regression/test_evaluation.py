"""Evidence completeness and fixture ownership, without a cluster or credentials."""
import importlib.util
import json
import gzip
from pathlib import Path
import sys
import tempfile
import unittest
from unittest.mock import patch, MagicMock

ROOT = Path(__file__).resolve().parents[2]
EVALUATION = ROOT / 'tests/evaluation'
# Isolated module names avoid the e2e runner's unrelated common/host modules.
def module(name):
    spec = importlib.util.spec_from_file_location('evaluation_' + name, EVALUATION / (name + '.py'))
    value = importlib.util.module_from_spec(spec)
    saved = {key: sys.modules.get(key) for key in ('common', 'load')}
    for key in saved:
        if key in globals():
            sys.modules[key] = globals()[key]
    try:
        spec.loader.exec_module(value)
    finally:
        for key, previous in saved.items():
            if previous is None:
                sys.modules.pop(key, None)
            else:
                sys.modules[key] = previous
    return value

common = module('common')
load = module('load')
human = module('human')
analyzer = module('analyze')


class EvaluationTests(unittest.TestCase):
    def test_missing_or_duplicate_scenarios_cannot_pass(self):
        plan = common.load_plan(EVALUATION / 'plan.json')
        self.assertFalse(common.lifecycle_acceptance([], plan)['passed'])
        rows = [{'method': method, 'repetition': i, 'stage': stage, 'seconds': seconds}
                for method in ('api', 'helm') for i in range(1, 4)
                for stage, seconds in (('deploy-v1', 10), ('update-v2', 5), ('unhealthy-v3-and-restoration', 310))]
        self.assertTrue(common.lifecycle_acceptance(rows, plan)['passed'])
        rows[0]['seconds'] = 31
        self.assertFalse(common.lifecycle_acceptance(rows, plan)['passed'])
        rows[0]['seconds'] = float('nan')
        self.assertFalse(common.lifecycle_acceptance(rows, plan)['passed'])
        rows[0]['seconds'] = 10
        rows.append(dict(rows[0]))
        self.assertFalse(common.lifecycle_acceptance(rows, plan)['passed'])

    def test_load_requires_complete_count_and_keeps_errors(self):
        row = {'sequence': 0, 'warmup': False, 'success': True, 'latencyMilliseconds': 5}
        self.assertFalse(load.summarize([row])['targetsPassed'])
        self.assertFalse(load.summarize([row], expected_count=2)['targetsPassed'])
        self.assertTrue(load.summarize([row], expected_count=1)['targetsPassed'])
        duplicate = [row, row]
        self.assertFalse(load.summarize(duplicate, expected_count=2)['targetsPassed'])
        failed = {**row, 'sequence': 1, 'success': False, 'error': 'TimeoutError'}
        result = load.summarize([row, failed], expected_count=2)
        self.assertEqual(result['successRate'], .5)
        self.assertFalse(result['targetsPassed'])
        late = {**row, 'scheduleDelaySeconds': 3}
        self.assertFalse(load.summarize([late], expected_count=1)['targetsPassed'])

    def test_foreign_recreated_release_refuses_cleanup(self):
        run = 'a' * 12
        name = 'eval-' + run + '-helm-1'
        live = {'metadata': {'uid': 'original', 'labels': {'name': name, 'owner': 'helm'}}}
        common.require_owned_release(run, name, 'original', live)
        for uid, release, identity in [('recreated', name, run), ('original', 'platform', run), ('original', name, 'bad')]:
            with self.assertRaises(ValueError):
                common.require_owned_release(identity, release, uid, live)

    def test_reference_headroom_rounds_up_and_rejects_no_evidence(self):
        self.assertEqual(common.derive_reference_preset(6, 7 * 2**20),
                         {'cpuRequest': '25m', 'cpuLimit': '250m', 'memoryRequest': '32Mi', 'memoryLimit': '128Mi'})
        self.assertEqual(common.derive_reference_preset(26, 20 * 2**20),
                         {'cpuRequest': '75m', 'cpuLimit': '250m', 'memoryRequest': '48Mi', 'memoryLimit': '128Mi'})
        with self.assertRaises(ValueError):
            common.derive_reference_preset(0, 1)

    def test_absent_human_data_remains_pending(self):
        for value in (human.summarize(), human.summarize(EVALUATION / 'human.csv')):
            self.assertEqual(value['status'], 'pending')
            self.assertFalse(value['humanOperatorTimeMeasured'])
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / 'human.csv'
            header = (EVALUATION / 'human.csv').read_text()
            path.write_text(header + 't,p,0.1.10,ui,deploy-update-delete,1,20,30,5,true,true,\n')
            self.assertEqual(human.summarize(path)['status'], 'incomplete')
            path.write_text(path.read_text() + 't,p,0.1.10,helm,deploy-update-delete,2,25,40,6,false,true,\n')
            result = human.summarize(path)
            self.assertTrue(result['pairedTrialsComplete'])
            self.assertFalse(result['allTasksSucceeded'])

    def test_invalid_numbers_are_not_statistics(self):
        for value in (float('nan'), float('inf'), -1, True):
            with self.assertRaises(ValueError):
                common.describe([value])
        self.assertEqual(common.percentile([3, 1, 2], 95), 3)
        self.assertIsNone(common.describe([])['median'])

    def test_evidence_files_are_private(self):
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / 'result.json'
            common.write_json(path, {'human': None})
            self.assertEqual(path.stat().st_mode & 0o777, 0o600)
            self.assertIsNone(json.loads(path.read_text())['human'])

    def test_missing_reference_is_not_a_cold_layer_pull(self):
        descriptors = [{'digest': 'manifest', 'size': 10, 'kind': 'manifest'},
                       {'digest': 'layer', 'size': 50, 'kind': 'layer'}]
        result = common.content_state(descriptors, {'layer'})
        self.assertEqual(result['condition'], 'partial-cache')
        self.assertEqual(result['missingLayerBytes'], 0)
        self.assertEqual(common.content_state(descriptors, set())['condition'], 'compressed-content-absent')
        self.assertFalse(common.content_state(descriptors, set())['fullyColdPullProven'])
        self.assertFalse(common.content_state(descriptors, set(), ['chain'], {'chain'})['fullyColdPullProven'])
        self.assertTrue(common.content_state(descriptors, set(), ['chain'], set())['fullyColdPullProven'])
        self.assertEqual(common.content_state(descriptors, {'manifest', 'layer'})['condition'], 'all-content-cached')
        with self.assertRaises(ValueError):
            common.content_state([], set())

    def test_active_or_renamed_application_refuses_cleanup(self):
        run = 'a' * 12
        recorded = {'name': 'eval-' + run + '-load'}
        live = {'id': 'app_test', 'name': recorded['name'], 'activeOperationId': ''}
        common.require_application(run, 'app_test', recorded, live)
        for changes in ({'id': 'app_other'}, {'name': 'production'}, {'activeOperationId': 'op_active'}):
            with self.assertRaises(ValueError):
                common.require_application(run, 'app_test', recorded, {**live, **changes})

    def test_resource_analysis_refuses_partial_data_and_reports_oom(self):
        plan = common.load_plan(EVALUATION / 'plan.json')
        plan.update(idleSeconds=2, loadedSeconds=2, warmupSeconds=1, requestRatePerSecond=2)
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory); host = root/'host'; client = root/'client'
            host.mkdir(); client.mkdir()
            fixture = {'appID': 'app_fixture', 'image': 'demo@sha256:pinned', 'access': {'url': 'https://fixture.test/'}}
            identity = {'containerID': 'original', 'restartCount': 0}
            common.write_json(host/'identity-before.json', {'appID': fixture['appID'], 'containers': [identity]})
            common.write_json(host/'summary.json', {'appID': fixture['appID'], 'evidenceValid': True, 'sampleCount': 7, 'containersAfter': [identity]})
            common.write_json(client/'summary.json', {'url': 'https://fixture.test/version', 'ratePerSecond': 2, 'durationSeconds': 2, 'warmupSeconds': 1, 'maximumOutstandingRequests': 32})
            observations = [{'sequence': i, 'warmup': i < 2, 'scheduledEpochSeconds': 1+i/2, 'success': True, 'latencyMilliseconds': 10} for i in range(6)]
            (client/'requests.jsonl').write_text(''.join(json.dumps(x)+'\n' for x in observations))
            samples = [{'epochSeconds': i, 'hostCPU': f'cpu {i*10} 0 0 {i*90} 0 0 0 0 0 0', 'hostMemoryBytes': {'MemTotal': 8*2**30, 'MemAvailable': 6*2**30, 'SwapTotal': 4*2**30, 'SwapFree': 4*2**30}, 'temperatureCelsius': 50, 'freeDiskBytes': 10*2**30, 'containers': [
                {'containerID': 'original', 'memoryPeakBytes': 10*2**20, 'memoryCurrentBytes': 8*2**20,
                 'memoryEvents': {'oom_kill': 0}, 'CPU': {'usage_usec': i*1000, 'throttled_usec': 0, 'nr_periods': i*10, 'nr_throttled': 0}}]} for i in range(7)]
            def save():
                (host/'samples.jsonl').write_text(''.join(json.dumps(x)+'\n' for x in samples))
            save()
            self.assertTrue(analyzer.analyze(host, client, fixture, plan)['targetsPassed'])
            samples[-1]['containers'][0]['memoryEvents']['oom_kill'] = 1
            save()
            result = analyzer.analyze(host, client, fixture, plan)
            self.assertFalse(result['targetsPassed'])
            self.assertFalse(result['targetChecks']['OOMKills'])
            raw = (host/'samples.jsonl').read_bytes()
            (host/'samples.jsonl.gz').write_bytes(gzip.compress(raw))
            (host/'samples.jsonl').unlink()
            self.assertFalse(analyzer.analyze(host, client, fixture, plan)['targetsPassed'])
            samples.pop();save()
            with self.assertRaises(ValueError):
                analyzer.analyze(host, client, fixture, plan)

    def test_interrupted_load_retains_partial_requests_and_refuses_overwrite(self):
        connection = MagicMock()
        response = connection.getresponse.return_value
        response.status = 200
        response.read.return_value = b'{"version":"v1"}'
        with tempfile.TemporaryDirectory() as directory:
            folder = Path(directory)/'attempt'
            with patch.object(load.ssl, 'create_default_context'), patch.object(load.http.client, 'HTTPSConnection', return_value=connection), patch.object(load.time, 'sleep', side_effect=KeyboardInterrupt):
                with self.assertRaises(KeyboardInterrupt):
                    load.run('https://fixture.test/', folder, 'test-ca', rate=20, seconds=1, warmup=0, expected_version='v1')
                retained = (folder/'requests.jsonl').read_text().splitlines()
                self.assertGreaterEqual(len(retained), 1)
                self.assertFalse((folder/'summary.json').exists())
                original = (folder/'requests.jsonl').read_bytes()
                with self.assertRaises(FileExistsError):
                    load.run('https://fixture.test/', folder, 'test-ca', rate=20, seconds=1, warmup=0, expected_version='v1')
                self.assertEqual((folder/'requests.jsonl').read_bytes(), original)
