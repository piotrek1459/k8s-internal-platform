"""Native checks allow bounded Kubernetes garbage collection after deletion."""
from pathlib import Path
import json
import tempfile
import sys
import unittest
import urllib.error
from unittest.mock import Mock, patch

sys.path.insert(0, str(Path(__file__).resolve().parents[1] / 'e2e'))
from native import Native
from native_diagnostics import delete_fixture
from native_secret_cleanup import set_fixture_owner


class NativeDeletionTests(unittest.TestCase):
    def fixture_version(self):
        version = {'name':'app-secret-sec-controlled-v2', 'uid':'controlled-uid', 'version':2}
        obj = {'type':'Opaque', 'immutable':True, 'metadata':{'name':version['name'], 'uid':version['uid'],
            'namespace':'platform-apps', 'resourceVersion':'42', 'labels':{
                'platform.local/secret-id':'sec-controlled', 'platform.local/secret-version':'2',
                'app.kubernetes.io/managed-by':'native-foreign-fixture'}}}
        return version, obj

    def test_secret_fixture_repair_refuses_identity_or_ownership_changes(self):
        for scenario in ('uid', 'namespace', 'secret-id', 'version', 'owner', 'type', 'immutable'):
            with self.subTest(scenario=scenario):
                version, obj = self.fixture_version()
                if scenario in ('uid', 'namespace'):
                    obj['metadata'][scenario] = 'unrelated'
                elif scenario in ('type', 'immutable'):
                    obj[scenario] = None
                else:
                    label={'secret-id':'platform.local/secret-id','version':'platform.local/secret-version','owner':'app.kubernetes.io/managed-by'}[scenario]
                    obj['metadata']['labels'][label]='unrelated'
                environment=Mock();environment.kube.return_value=obj
                with self.assertRaisesRegex(RuntimeError,'fixture repair refused'):
                    set_fixture_owner(environment,version,'sec_controlled','native-foreign-fixture','k8s-internal-platform')
                environment.kube.assert_called_once_with('/api/v1/namespaces/platform-apps/secrets/'+version['name'])

    def test_secret_fixture_repair_tests_uid_and_resource_version_before_patch(self):
        version,obj=self.fixture_version();environment=Mock();environment.kube.side_effect=[obj,{}]
        set_fixture_owner(environment,version,'sec_controlled','native-foreign-fixture','k8s-internal-platform')
        environment.kube.assert_called_with('/api/v1/namespaces/platform-apps/secrets/'+version['name'],method='PATCH',
            content_type='application/json-patch+json',body=[
                {'op':'test','path':'/metadata/uid','value':'controlled-uid'},
                {'op':'test','path':'/metadata/resourceVersion','value':'42'},
                {'op':'replace','path':'/metadata/labels/app.kubernetes.io~1managed-by','value':'k8s-internal-platform'}])

    def test_secret_fixture_concurrent_change_is_not_retried_without_guards(self):
        version,obj=self.fixture_version();environment=Mock()
        environment.kube.side_effect=[obj,urllib.error.HTTPError('/fixture',409,'conflict',None,None)]
        with self.assertRaises(urllib.error.HTTPError):
            set_fixture_owner(environment,version,'sec_controlled','native-foreign-fixture','k8s-internal-platform')
        self.assertEqual(environment.kube.call_count,2)

    def test_diagnostic_cleanup_refuses_reused_or_unrecorded_identity(self):
        for identity in ('original-uid', None):
            environment = Mock()
            environment.kube.return_value = {'metadata': {'uid': 'replacement-uid', 'resourceVersion': '42'}}
            with self.assertRaisesRegex(RuntimeError, 'identity changed'):
                delete_fixture(environment, '/owned-fixture', identity)
            environment.kube.assert_called_once_with('/owned-fixture')

    def test_diagnostic_cleanup_uses_uid_and_resource_version_preconditions(self):
        environment = Mock()
        environment.kube.side_effect = [{'metadata': {'uid': 'owned-uid', 'resourceVersion': '42'}}, {}]
        delete_fixture(environment, '/owned-fixture', 'owned-uid')
        self.assertEqual(environment.kube.call_count, 2)
        environment.kube.assert_called_with('/owned-fixture', method='DELETE',
            body={'preconditions': {'uid': 'owned-uid', 'resourceVersion': '42'}})

    def test_diagnostic_cleanup_accepts_already_removed_fixture(self):
        environment = Mock()
        environment.kube.side_effect = urllib.error.HTTPError('/owned-fixture', 404, 'missing', None, None)
        delete_fixture(environment, '/owned-fixture', 'owned-uid')
        environment.kube.assert_called_once_with('/owned-fixture')

    def test_interrupted_first_deployment_retains_private_cleanup_identity(self):
        with tempfile.TemporaryDirectory() as folder:
            environment = Native.__new__(Native)
            environment.state = {}
            environment.password = 'controlled-private-state'
            environment.state_path = Path(folder)/'access.json'
            environment.client = Mock()
            environment.client.request.return_value = (202, {'applicationId':'owned-app','operationId':'owned-operation'})
            environment.client.poll.side_effect = RuntimeError('controlled interruption')
            with self.assertRaisesRegex(RuntimeError, 'controlled interruption'):
                environment.blocked_first_removal('example.com/app@sha256:fixture')
            self.assertEqual(json.loads(environment.state_path.read_text())['blockedTestApp'], 'owned-app')
            self.assertEqual(environment.state_path.stat().st_mode & 0o777, 0o600)

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
