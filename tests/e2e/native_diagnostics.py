"""Published-release diagnostics acceptance on the native runner's own fixtures.

Raw log/event fixtures remain in memory. Reports contain check descriptions only;
private state records resource identities before waiting, for interrupted cleanup.
"""
import base64
import datetime
import json
import secrets
import urllib.error
import urllib.parse
import urllib.request

from support import Client


NAMESPACE = '/api/v1/namespaces/platform-apps'
DEPLOYMENTS = '/apis/apps/v1/namespaces/platform-apps/deployments'


def delete_fixture(native, path, expected_uid):
    try:
        current = native.kube(path)['metadata']
        if not expected_uid or current['uid'] != expected_uid:
            raise RuntimeError('Diagnostic fixture identity changed; cleanup refused; inspect private state')
        native.kube(path, method='DELETE', body={'preconditions': {
            'uid': expected_uid, 'resourceVersion': current['resourceVersion']}})
    except urllib.error.HTTPError as error:
        if error.code != 404:
            raise
        error.close()


def check_diagnostics(native):
    from playwright.sync_api import sync_playwright

    if native.state.get('diagnosticResources'):
        raise RuntimeError('Interrupted diagnostic fixtures require inspection using private state before another run')
    images = json.loads(native.args.release_manifest.read_text())['targets']['arm64']['images']
    client = native.client
    resources = {'applications': [], 'events': [], 'eventUids': {}, 'pod': None, 'podUid': None, 'secret': None}
    native.state['diagnosticResources'] = resources
    native.save()
    value = 'native-diagnostic-'+secrets.token_hex(16)
    encoded = base64.b64encode(value.encode()).decode()

    def objects(kind, app):
        selector = urllib.parse.quote('platform.local/app-id='+app)
        path = DEPLOYMENTS if kind == 'deployments' else NAMESPACE+'/'+kind
        return native.kube(path+'?labelSelector='+selector)['items']

    def create_app(prefix, **extra):
        code, accepted = client.request('POST', '/apps', {
            'name': prefix+'-'+secrets.token_hex(3), 'image': images['v1'],
            'containerPort': 8080, 'profile': 'http',
            'health': {'readinessPath': '/readyz'}, 'exposure': {'mode': 'internal'}, **extra,
        }, {'Idempotency-Key': secrets.token_hex(16)})
        assert code == 202, 'Diagnostic fixture application was not accepted'
        app = accepted['applicationId']
        resources['applications'].append(app)
        native.save()
        client.poll(accepted['operationId'])
        return app

    def cleanup():
        if resources['pod']:
            delete_fixture(native, NAMESPACE+'/pods/'+resources['pod'], resources['podUid'])
        for event in resources['events']:
            delete_fixture(native, NAMESPACE+'/events/'+event, resources['eventUids'].get(event))
        for app in resources['applications']:
            current = client.get('/apps/'+app)
            if current.get('deletedAt'):
                continue
            client.mutate('DELETE', '/apps/'+app, {'version': current['version']})
            native.wait_for_pod_deletion(app)
        if resources['secret']:
            status = client.request('DELETE', '/secrets/'+resources['secret'])[0]
            resources['secretDeletionStatus'] = status
            native.save()
            assert status == 204, f'Native diagnostic Secret cleanup returned HTTP {status}; private state retains cleanup identities'
            try:
                native.kube(NAMESPACE+'/secrets/app-secret-'+resources['secret'].replace('_', '-')+'-v1')
            except urllib.error.HTTPError as error:
                assert error.code == 404
                error.close()
            else:
                raise AssertionError('Secret deletion reported success but retained the Kubernetes value')
        del native.state['diagnosticResources']
        native.save()

    try:
        code, secret = client.request('POST', '/secrets', {
            'name': 'native-diagnostic-'+secrets.token_hex(3), 'data': {'value': value}})
        assert code == 201, 'Diagnostic Secret was not created'
        resources['secret'] = secret['id']
        native.save()
        app = create_app('native-diagnostic', environment={'MODE': 'stable'},
            secretRefs=[{'environmentVariable': 'APP_SECRET', 'secretId': secret['id'], 'version': 1}])
        other = create_app('native-foreign')
        pod = next(p for p in native.pods(app) if not p['metadata'].get('deletionTimestamp'))
        foreign = next(p for p in native.pods(other) if not p['metadata'].get('deletionTimestamp'))
        service = objects('services', app)[0]
        identity = {key: service['metadata'][key] for key in ('name', 'uid')}
        address = {key: service['spec'].get(key) for key in ('clusterIP', 'clusterIPs', 'ipFamilies')}
        proxy = NAMESPACE+'/services/'+identity['name']+':80/proxy'
        try:
            native.kube(proxy+'/diagnostic/'+urllib.parse.quote(value+'/'+encoded, safe=''))
        except urllib.error.HTTPError as error:
            assert error.code == 404
            error.close()
        log_path = NAMESPACE+'/pods/'+pod['metadata']['name']+'/log?container=application&tailLines=100'
        # Kubernetes log responses are text, not JSON. Never retain this body.
        with urllib.request.urlopen(native.kubernetes+log_path, timeout=20) as response:
            raw = response.read().decode()
        assert value in raw and encoded in raw, 'Actual native workload log lacks the controlled fixture'
        logs = client.get('/apps/'+app+'/logs')
        assert value not in logs['text'] and encoded not in logs['text'], 'Managed Secret leaked in diagnostic logs'
        assert '[REDACTED]' in logs['text'] and len(logs['text'].encode()) <= 65536
        assert client.request('GET', '/apps/'+app+'/logs?pod='+foreign['metadata']['name'])[0] == 404
        assert client.request('GET', '/apps/'+app+'/logs?namespace=kube-system')[0] == 400
        assert client.request('GET', '/apps/'+app+'/logs?tailLines=501')[0] == 400
        assert Client(native).request('GET', '/apps/'+app+'/logs')[0] == 401

        payload = '<img src=x onerror="window.__diagnosticXss=1">'
        now = datetime.datetime.now(datetime.timezone.utc).isoformat().replace('+00:00', 'Z')
        for target, message, reason in ((pod, payload+' '+value+' '+encoded, 'NativeDiagnosticFixture'),
                (foreign, 'native-foreign-event', 'NativeForeignFixture')):
            name = 'native-diagnostic-'+secrets.token_hex(3)
            resources['events'].append(name)
            native.save()
            created = native.kube(NAMESPACE+'/events', method='POST', body={
                'apiVersion': 'v1', 'kind': 'Event', 'metadata': {'name': name, 'namespace': 'platform-apps'},
                'involvedObject': {'apiVersion': 'v1', 'kind': 'Pod', 'namespace': 'platform-apps',
                    'name': target['metadata']['name'], 'uid': target['metadata']['uid']},
                'type': 'Warning', 'reason': reason, 'message': message,
                'firstTimestamp': now, 'lastTimestamp': now, 'count': 1,
            })
            resources['eventUids'][name] = created['metadata']['uid']
            native.save()
        resources['pod'] = 'native-copied-labels-'+secrets.token_hex(3)
        native.save()
        created = native.kube(NAMESPACE+'/pods', method='POST', body={
            'apiVersion': 'v1', 'kind': 'Pod', 'metadata': {'name': resources['pod'],
                'namespace': 'platform-apps', 'labels': {'platform.local/app-id': app}},
            'spec': {'automountServiceAccountToken': False,
                'securityContext': {'runAsNonRoot': True, 'seccompProfile': {'type': 'RuntimeDefault'}},
                'containers': [{'name': 'application', 'image': images['v1'],
                    'resources': pod['spec']['containers'][0]['resources'],
                    'securityContext': {'allowPrivilegeEscalation': False, 'readOnlyRootFilesystem': True,
                        'capabilities': {'drop': ['ALL']}}}]},
        })
        resources['podUid'] = created['metadata']['uid']
        native.save()
        assert client.request('GET', '/apps/'+app+'/logs?pod='+resources['pod'])[0] == 404
        events = json.dumps(client.get('/apps/'+app+'/events'))
        assert value not in events and encoded not in events and 'native-foreign-event' not in events
        assert 'NativeDiagnosticFixture' in events
        assert client.get('/apps/'+app+'/drift')['state'] == 'in_sync'

        with sync_playwright() as playwright:
            browser = playwright.chromium.launch()
            try:
                page = browser.new_page()
                page.set_default_timeout(30000)
                page.goto(native.url+'/login')
                page.get_by_label('Password', exact=True).fill(native.password)
                page.get_by_role('button', name='Sign in', exact=True).click()
                page.wait_for_url('**/apps')
                page.goto(native.url+'/apps/'+app)
                page.get_by_role('button', name='Diagnostics', exact=True).click()
                page.get_by_role('button', name='Read logs', exact=True).click()
                page.locator('pre').wait_for()
                assert value not in page.locator('pre').inner_text()
                page.get_by_role('button', name='Read events', exact=True).click()
                page.get_by_text('NativeDiagnosticFixture', exact=False).wait_for()
                section = page.get_by_role('region', name='Workload events')
                assert payload in section.inner_text() and section.locator('img').count() == 0
                assert page.evaluate('window.__diagnosticXss') is None
                page.get_by_role('button', name='Inspect drift', exact=True).click()
                page.get_by_text('No owned configuration differences detected.', exact=True).wait_for()
            finally:
                browser.close()
        native.record('Native diagnostics: actual log/event fixtures masked; foreign/copied-label Pods denied; bounded/authenticated reads and escaped browser text verified')

        deployment = objects('deployments', app)[0]
        native.kube(DEPLOYMENTS+'/'+deployment['metadata']['name'], method='PATCH',
            content_type='application/strategic-merge-patch+json', body={
                'spec': {'replicas': 2, 'template': {'spec': {'containers': [{'name': 'application',
                    'image': images['v2'], 'env': [{'name': 'MODE', 'value': 'operator-change'}],
                    'resources': {'limits': {'memory': '256Mi'}},
                    'readinessProbe': {'httpGet': {'path': '/version'}},
                }]}}}})
        native.kube(NAMESPACE+'/services/'+identity['name'], method='PATCH',
            content_type='application/json-patch+json', body=[{'op': 'replace', 'path': '/spec/ports/0/port', 'value': 81}])
        drift = client.get('/apps/'+app+'/drift')
        assert drift['state'] == 'drifted'
        fields = {difference['field'] for difference in drift['differences']}
        for field in ('/spec/replicas', '/spec/ports', '/spec/template/spec/containers/application/image',
                '/spec/template/spec/containers/application/env', '/spec/template/spec/containers/application/resources',
                '/spec/template/spec/containers/application/readinessProbe'):
            assert field in fields, 'Expected owned configuration difference is missing'
        assert images['v2'] not in json.dumps(drift) and 'operator-change' not in json.dumps(drift)
        assert objects('services', app)[0]['spec']['ports'][0]['port'] == 81, 'Drift inspection mutated the Service'
        current = client.get('/apps/'+app)
        client.mutate('POST', '/apps/'+app+'/reapply', {'version': current['version']})
        native.http(app, 'v1')
        assert client.get('/apps/'+app+'/drift')['state'] == 'in_sync'
        repaired = objects('services', app)[0]
        assert repaired['metadata']['uid'] == identity['uid']
        assert {key: repaired['spec'].get(key) for key in address} == address
        assert len(repaired['spec']['ports']) == 1 and repaired['spec']['ports'][0]['port'] == 80
        native.record('Native drift: image/env/resources/probe/replica/Service differences reported without values or mutation; explicit Reapply restored HTTP v1/in_sync and retained Service UID/IP')
    except BaseException:
        try:
            cleanup()
        except Exception:
            native.record('Native diagnostic fixture cleanup is incomplete; private state retains resource identities; the original failure is preserved')
        raise
    else:
        cleanup()
    native.record('Native diagnostic applications, copied-label Pod, events and configuration Secret removed; original retained app preserved')
