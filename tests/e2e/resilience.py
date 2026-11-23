"""Deterministic application gates and a lost-response boundary on owned clusters.

The response proxy observes the backend's committed 202, then closes the client
connection without delivering any headers/body. Cookies and bodies stay in memory.
"""
import contextlib
import base64
import gzip
import hashlib
import http.client
import http.server
import json
import socket
import secrets
import threading
import time
import urllib.error
import urllib.parse
import urllib.request

from support import ROOT


def drop_accepted_response(client, body, key):
    origin = urllib.parse.urlsplit(client.environment.url)
    if origin.scheme != 'http' or origin.hostname not in ('127.0.0.1', 'localhost', '::1'):
        raise ValueError('The lost-response fixture requires an owned loopback origin')
    completed = threading.Event()
    outcome = {}

    class Handler(http.server.BaseHTTPRequestHandler):
        def log_message(self, *_):
            pass

        def do_POST(self):
            self.connection.settimeout(25)
            connection = None
            try:
                if self.path != '/api/v1/apps':
                    raise ValueError('Unexpected fixture path')
                length = int(self.headers.get('Content-Length', '0'))
                if not 0 < length <= 1048576:
                    raise ValueError('Invalid fixture body size')
                raw = self.rfile.read(length)
                headers = {name: self.headers[name] for name in
                    ('Content-Type', 'Cookie', 'Idempotency-Key', 'X-Requested-With') if name in self.headers}
                connection = http.client.HTTPConnection(origin.hostname, origin.port, timeout=20)
                connection.request('POST', '/api/v1/apps', raw, headers)
                response = connection.getresponse()
                outcome['status'] = response.status
                # Reading the backend response establishes the post-commit
                # barrier. Nothing from it is sent to the submitting client.
                outcome['accepted'] = json.loads(response.read(65537))
            except Exception:
                outcome['fixtureError'] = True
            finally:
                if connection:
                    connection.close()
                completed.set()
                self.close_connection = True
                with contextlib.suppress(OSError):
                    self.connection.shutdown(socket.SHUT_RDWR)
                self.connection.close()

    server = http.server.ThreadingHTTPServer(('127.0.0.1', 0), Handler)
    server.daemon_threads = True
    thread = threading.Thread(target=server.serve_forever, daemon=True)
    thread.start()
    lost = False
    try:
        request = urllib.request.Request(f'http://127.0.0.1:{server.server_port}/api/v1/apps',
            data=json.dumps(body).encode(), method='POST', headers={
                'Content-Type': 'application/json', 'X-Requested-With': 'XMLHttpRequest', 'Idempotency-Key': key})
        try:
            with client.http.open(request, timeout=25):
                pass
        except (http.client.RemoteDisconnected, ConnectionResetError, urllib.error.URLError):
            lost = True
        if not completed.wait(5) or outcome.get('fixtureError'):
            raise AssertionError('Lost-response fixture did not reach a verified backend response')
        if outcome.get('status') != 202 or not lost:
            raise AssertionError('Lost-response fixture did not discard a durably accepted 202')
        accepted = outcome['accepted']
        if not all(isinstance(accepted.get(field), str) and accepted[field] for field in ('applicationId', 'operationId')):
            raise AssertionError('Accepted response omitted durable operation/application identity')
        return accepted
    finally:
        server.shutdown()
        server.server_close()
        thread.join(timeout=5)


def fixture_images(environment):
    if hasattr(environment, 'workload_fixture_images'):
        return environment.workload_fixture_images
    if not environment.owns_cluster:
        raise RuntimeError('Workload fixture images require an owned disposable cluster')
    port = environment.env['PLATFORM_REGISTRY_PORT']
    images = {}
    for version in ('v1', 'v2'):
        local = f'localhost:{port}/workflow-fixture:{version}'
        environment.fixture_image_tags.append(local)
        environment.run('docker', 'build', '--build-arg', 'VERSION=fixture-'+version,
            '-t', local, ROOT/'tests/e2e/fixtures/workload')
        environment.run('docker', 'push', local)
        images[version] = f'k3d-{environment.name}-registry:{port}/workflow-fixture:{version}'
    environment.workload_fixture_images = images
    return images


def move_tag(environment, version):
    port = environment.env['PLATFORM_REGISTRY_PORT']
    target = f'localhost:{port}/workflow-fixture:retry'
    if target not in environment.fixture_image_tags:
        environment.fixture_image_tags.append(target)
    environment.run('docker', 'tag', f'localhost:{port}/workflow-fixture:{version}', target)
    environment.run('docker', 'push', target)
    return f'k3d-{environment.name}-registry:{port}/workflow-fixture:retry'


def wait_pending(environment, app):
    deadline = time.monotonic()+120
    while time.monotonic() < deadline:
        releases = json.loads(environment.kubectl('-n', 'platform-apps', 'get', 'secrets',
            '-l', 'owner=helm,name=app-'+app.replace('_', '-'), '-o', 'json').stdout)['items']
        pods = environment.resource('pods', app)
        operation = environment.client.get('/apps/'+app).get('activeOperationId')
        if operation and environment.client.get('/operations/'+operation)['state'] == 'running' and len(releases) == 1 and \
                releases[0]['metadata']['labels'].get('status') == 'pending-install' and any(
                    status.get('state', {}).get('running') and not status.get('ready')
                    for pod in pods for status in pod.get('status', {}).get('containerStatuses', [])):
            revision = environment.client.get('/apps/'+app+'/revisions')['items'][0]
            if revision.get('frozenImage'):
                return revision
        time.sleep(0.5)
    raise AssertionError('No real running operation / pending release / unready workload barrier was reached')


def release_gate(environment, app, token):
    with environment.forward('forward', app, port=environment.app_port):
        request = urllib.request.Request(f'http://127.0.0.1:{environment.app_port}/control/ready',
            method='POST', headers={'X-Fixture-Control': token}, data=b'')
        with urllib.request.urlopen(request, timeout=10) as response:
            if response.status != 204:
                raise AssertionError('Controlled workload readiness was not released')


def control_secret(environment):
    token = secrets.token_urlsafe(24)
    environment.secret_values.append(token)
    code, secret = environment.client.request('POST', '/secrets',
        {'name': 'fixture-control-'+secrets.token_hex(4), 'data': {'value': token}})
    if code != 201:
        raise AssertionError('Controlled fixture Secret was not created')
    return token, secret


def release_identity(environment, app):
    releases = json.loads(environment.kubectl('-n', 'platform-apps', 'get', 'secrets',
        '-l', 'owner=helm,name=app-'+app.replace('_', '-'), '-o', 'json').stdout)['items']
    if not releases:
        raise AssertionError('No recorded Helm release identity')
    latest = max(releases, key=lambda item: int(item['metadata']['labels']['version']))
    # Helm Secret data is Kubernetes base64 over Helm base64/gzip/JSON. Only
    # hashes leave memory; the release object is never retained as an artifact.
    payload = gzip.decompress(base64.b64decode(base64.b64decode(latest['data']['release'])))
    release = json.loads(payload)
    def digest(value):
        return hashlib.sha256(json.dumps(value, sort_keys=True, separators=(',', ':')).encode()).hexdigest()
    return {'chart': digest(release['chart']), 'values': digest(release['config'])}


def cleanup_fixture(environment, app, secret):
    environment.delete(app)
    if environment.client.request('DELETE', '/secrets/'+secret['id'])[0] != 204:
        raise AssertionError('Controlled fixture Secret cleanup did not complete')
    versions = json.loads(environment.kubectl('-n', 'platform-apps', 'get', 'secrets',
        '-l', 'platform.local/secret-id='+secret['id'].replace('_', '-'), '-o', 'json').stdout)['items']
    if versions:
        raise AssertionError('Controlled fixture Kubernetes values remain after deletion')
