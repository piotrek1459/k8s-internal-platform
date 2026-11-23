"""Two-phase published-release Secret acceptance on a dedicated native install.

Only recorded fixture identities are changed. The operator replaces the broker
and reconnects its loopback forward between phases. Values remain in memory;
resource/session identities are retained privately for interrupted inspection.
"""
import base64
import http.cookiejar
import json
import os
import secrets
import urllib.error

from support import Client


PREFIX = '/api/v1/namespaces/'
APPS = PREFIX+'platform-apps/secrets/'


def set_fixture_owner(native, version, secret_id, expected_owner, owner):
    obj = native.kube(APPS+version['name'])
    metadata = obj['metadata']
    labels = metadata.get('labels', {})
    if metadata['uid'] != version['uid'] or metadata['namespace'] != 'platform-apps' or \
            labels.get('platform.local/secret-id') != secret_id.replace('_', '-') or \
            labels.get('platform.local/secret-version') != str(version['version']) or \
            labels.get('app.kubernetes.io/managed-by') != expected_owner or \
            obj.get('type') != 'Opaque' or obj.get('immutable') is not True:
        raise RuntimeError('Controlled Secret identity/ownership changed; fixture repair refused')
    native.kube(APPS+version['name'], method='PATCH', content_type='application/json-patch+json', body=[
        {'op':'test', 'path':'/metadata/uid', 'value':version['uid']},
        {'op':'test', 'path':'/metadata/resourceVersion', 'value':metadata['resourceVersion']},
        {'op':'replace', 'path':'/metadata/labels/app.kubernetes.io~1managed-by', 'value':owner},
    ])


def absent(native, version):
    try:
        native.kube(APPS+version['name'])
    except urllib.error.HTTPError as error:
        assert error.code == 404
        error.close()
    else:
        raise AssertionError('Controlled Secret value still exists')


def broker_identity(native):
    pods = native.kube(PREFIX+'platform-system/pods')['items']
    ready = [p for p in pods if p['metadata'].get('labels', {}).get('app.kubernetes.io/name') == 'platform'
        and not p['metadata'].get('deletionTimestamp')
        and p.get('status', {}).get('containerStatuses', [{}])[0].get('ready')]
    assert len(ready) == 1, 'Expected one ready broker for the replacement check'
    pvc = native.kube(PREFIX+'platform-system/persistentvolumeclaims/platform-metadata')
    return {'container': ready[0]['status']['containerStatuses'][0]['containerID'], 'pvc': pvc['metadata']['uid']}


def check_secret_cleanup(native, phase):
    from playwright.sync_api import sync_playwright

    key = 'secretCleanupResources'
    cookie_path = native.state_path.with_name('secret-cleanup.cookies')
    client = native.client

    def metadata(secret_id):
        return next(item for item in client.get('/secrets')['items'] if item['id'] == secret_id)

    def login(page):
        page.goto(native.url+'/login')
        page.get_by_label('Password', exact=True).fill(native.password)
        page.get_by_role('button', name='Sign in', exact=True).click()
        page.wait_for_url('**/apps')

    if phase == 'hold':
        if native.state.get(key) or cookie_path.exists():
            raise RuntimeError('An earlier Secret-cleanup fixture requires private-state inspection')
        resources = {'stage':'creating', 'name':'native-cleanup-'+secrets.token_hex(3), 'secret':None, 'app':None, 'versions':[]}
        native.state[key] = resources
        native.save()
        values = [secrets.token_urlsafe(24), secrets.token_urlsafe(24)]
        with sync_playwright() as playwright:
            browser = playwright.chromium.launch()
            try:
                page = browser.new_page(); page.set_default_timeout(30000)
                login(page); page.goto(native.url+'/secrets')
                page.get_by_placeholder('Name', exact=True).fill(resources['name'])
                page.get_by_placeholder('Value (write-only)', exact=True).fill(values[0])
                with page.expect_response(lambda r:r.url.endswith('/api/v1/secrets') and r.request.method=='POST') as response:
                    page.get_by_role('button', name='Create', exact=True).click()
                assert response.value.status == 201
                secret = response.value.json(); resources['secret'] = secret['id']; native.save()
                assert all(value not in json.dumps(secret) for value in values), 'Creation disclosed a controlled value'
                row = page.get_by_text(resources['name'], exact=True).locator('../..')
                row.get_by_role('button', name='Add version', exact=True).click()
                page.get_by_placeholder('New value (write-only)', exact=True).fill(values[1])
                with page.expect_response(lambda r:r.url.endswith('/api/v1/secrets/'+secret['id']+'/versions') and r.request.method=='POST') as response:
                    page.get_by_role('button', name='Add', exact=True).click()
                assert response.value.status == 201 and response.value.json()['version'] == 2
                page.get_by_text('Versions: 1, 2', exact=True).wait_for()
                assert all(value not in page.locator('body').inner_text() for value in values), 'Secret page displayed a controlled value'
                for number in (1, 2):
                    name = 'app-secret-'+secret['id'].replace('_', '-')+'-v'+str(number)
                    obj = native.kube(APPS+name)
                    assert obj['immutable'] and base64.b64decode(obj['data']['value']).decode() == values[number-1]
                    resources['versions'].append({'name':name, 'uid':obj['metadata']['uid'], 'version':number})
                    native.save()
                image = json.loads(native.args.release_manifest.read_text())['targets']['arm64']['images']['v1']
                code, accepted = client.request('POST', '/apps', {'name':'native-secret-ref-'+secrets.token_hex(3),
                    'image':image, 'containerPort':8080, 'profile':'http', 'health':{'readinessPath':'/readyz'},
                    'secretRefs':[{'environmentVariable':'NATIVE_SECRET', 'secretId':secret['id'], 'version':2}],
                    'exposure':{'mode':'internal'}}, {'Idempotency-Key':secrets.token_hex(16)})
                assert code == 202
                resources['app'] = accepted['applicationId']; native.save()
                client.poll(accepted['operationId']); native.http(resources['app'], 'v1')
                code, body = client.request('DELETE', '/secrets/'+secret['id'])
                assert code == 409 and body['error']['code'] == 'SECRET_IN_USE'
                assert not metadata(secret['id'])['deleting']
                page.on('dialog', lambda dialog:dialog.accept())
                with page.expect_response(lambda r:r.url.endswith('/api/v1/secrets/'+secret['id']) and r.request.method=='DELETE') as response:
                    row.get_by_role('button', name='Delete', exact=True).click()
                assert response.value.status == 409
                page.get_by_text(response.value.json()['error']['message'], exact=True).wait_for()
                native.http(resources['app'], 'v1')
            finally:
                browser.close()
        native.record('Native Secret browser creation/versioning is write-only; exact Kubernetes values verified privately; referenced API/browser deletion refused with continued HTTP v1')
        native.mutate(resources['app'], 'DELETE'); native.wait_for_pod_deletion(resources['app'])
        assert client.get('/apps/'+resources['app']+'/revisions')['items'], 'Deletion removed revision evidence'
        version = resources['versions'][1]
        set_fixture_owner(native, version, resources['secret'], 'k8s-internal-platform', 'native-foreign-fixture')
        code, body = client.request('DELETE', '/secrets/'+resources['secret'])
        assert code == 409 and body['error']['code'] == 'CONFLICT'
        absent(native, resources['versions'][0])
        meta = metadata(resources['secret']); assert meta['deleting'] and len(meta['versions']) == 2
        assert client.request('POST', '/secrets/'+resources['secret']+'/versions', {'data':{'value':values[0]}})[0] == 409
        before = [item['id'] for item in client.get('/apps')['items']]
        code, _ = client.request('POST', '/apps', {'name':'native-secret-rejected-'+secrets.token_hex(3),
            'image':image, 'containerPort':8080, 'profile':'http', 'exposure':{'mode':'internal'},
            'secretRefs':[{'environmentVariable':'NATIVE_SECRET', 'secretId':resources['secret'], 'version':2}]},
            {'Idempotency-Key':secrets.token_hex(16)})
        assert code == 400 and [item['id'] for item in client.get('/apps')['items']] == before
        resources['broker'] = broker_identity(native)
        resources['administrator'] = client.get('/auth/me')['administratorId']
        resources['stage'] = 'held'; native.save()
        jar = http.cookiejar.MozillaCookieJar(str(cookie_path))
        for cookie in client.cookies: jar.set_cookie(cookie)
        jar.save(ignore_discard=True, ignore_expires=True); os.chmod(cookie_path, 0o600)
        native.record('Native partial cleanup removed v1, refused foreign-owned v2, retained metadata/reservation/history and rejected new versions/references; replace broker and reconnect before resume')
        return

    resources = native.state.get(key)
    if not resources or resources.get('stage') != 'held':
        raise RuntimeError('No completed hold phase is available for replacement verification')
    current = broker_identity(native)
    assert current['container'] != resources['broker']['container'], 'Broker process was not replaced between phases'
    assert current['pvc'] == resources['broker']['pvc'], 'Broker metadata PVC identity changed'
    original = Client(native)
    jar = http.cookiejar.MozillaCookieJar(str(cookie_path)); jar.load(ignore_discard=True, ignore_expires=True)
    for cookie in jar: original.cookies.set_cookie(cookie)
    assert original.get('/auth/me')['administratorId'] == resources['administrator'], 'Original session did not survive replacement'
    client = original
    assert metadata(resources['secret'])['deleting']
    absent(native, resources['versions'][0])
    with sync_playwright() as playwright:
        browser = playwright.chromium.launch()
        try:
            page = browser.new_page(); page.set_default_timeout(30000)
            login(page); page.goto(native.url+'/secrets')
            page.get_by_text('Deletion is pending. Retry deletion after resolving the reported error.', exact=True).wait_for()
            row = page.get_by_text(resources['name'], exact=True).locator('../..')
            assert row.get_by_role('button', name='Add version', exact=True).is_disabled()
            page.on('dialog', lambda dialog:dialog.accept())
            with page.expect_response(lambda r:r.url.endswith('/api/v1/secrets/'+resources['secret']) and r.request.method=='DELETE') as response:
                row.get_by_role('button', name='Retry deletion', exact=True).click()
            assert response.value.status == 409 and metadata(resources['secret'])['deleting']
            set_fixture_owner(native, resources['versions'][1], resources['secret'], 'native-foreign-fixture', 'k8s-internal-platform')
            with page.expect_response(lambda r:r.url.endswith('/api/v1/secrets/'+resources['secret']) and r.request.method=='DELETE') as response:
                row.get_by_role('button', name='Retry deletion', exact=True).click()
            assert response.value.status == 204
            page.get_by_text(resources['name'], exact=True).wait_for(state='detached')
        finally:
            browser.close()
    assert all(item['id'] != resources['secret'] for item in client.get('/secrets')['items'])
    for version in resources['versions']: absent(native, version)
    assert client.get('/apps/'+resources['app']+'/revisions')['items']
    assert client.request('DELETE', '/auth/session')[0] == 204
    cookie_path.unlink(); del native.state[key]; native.save()
    native.record('Native replacement retained original session/PVC/deletion reservation; browser retry refused unresolved ownership, then removed all versions/metadata after guarded fixture repair; revision history retained')
