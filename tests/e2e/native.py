"""Opt-in acceptance of an installed release through SSH loopback forwards.

Only applications created by this run are mutated. Administrator credentials are
kept in a mode-0600 local state file, never in reports or command arguments.
"""
import argparse
import json
import os
from pathlib import Path
import re
import secrets
import time
import urllib.parse
import urllib.request

from support import Client


class Native:
    def __init__(self, args):
        self.args = args
        self.url = args.url.rstrip('/')
        self.kubernetes = args.kubernetes_url.rstrip('/')
        for url in (self.url, self.kubernetes):
            if urllib.parse.urlsplit(url).hostname not in ('127.0.0.1', 'localhost', '::1'):
                raise ValueError('Native acceptance requires local SSH forwards')
        self.state_path = args.state
        self.state = json.loads(args.state.read_text()) if args.state.exists() else {}
        self.password = self.state.get('password') or secrets.token_urlsafe(24)
        self.client = Client(self)
        self.records = []
        args.artifacts.mkdir(parents=True, exist_ok=True)

    def save(self):
        self.state_path.parent.mkdir(parents=True, exist_ok=True)
        fd = os.open(self.state_path, os.O_WRONLY | os.O_CREAT | os.O_TRUNC, 0o600)
        with os.fdopen(fd, 'w') as file:
            os.chmod(self.state_path, 0o600)
            json.dump({**self.state, 'password': self.password}, file)

    def kube(self, path):
        with urllib.request.urlopen(self.kubernetes + path, timeout=20) as response:
            return json.load(response)

    def record(self, text):
        self.records.append(text)
        (self.args.artifacts / 'native-results.json').write_text(json.dumps({'release': self.args.version, 'checks': self.records}, indent=2)+'\n')
        print(text, flush=True)

    def pods(self, app):
        selector = urllib.parse.quote('platform.local/app-id='+app)
        return self.kube('/api/v1/namespaces/platform-apps/pods?labelSelector='+selector)['items']

    def pod_uids(self, app):
        return sorted(p['metadata']['uid'] for p in self.pods(app) if not p['metadata'].get('deletionTimestamp'))

    def wait_for_pod_deletion(self, app):
        # Helm waits for the Deployment to disappear; Kubernetes garbage
        # collection may still be terminating its Pods after operation success.
        deadline = time.monotonic() + 60
        while self.pods(app):
            if time.monotonic() >= deadline:
                raise AssertionError('Deleted application Pods remain after 60 seconds')
            time.sleep(1)

    def http(self, app, expected):
        selector = urllib.parse.quote('platform.local/app-id='+app)
        services = self.kube('/api/v1/namespaces/platform-apps/services?labelSelector='+selector)['items']
        assert len(services) == 1, 'Expected exactly one owned Service'
        path = '/api/v1/namespaces/platform-apps/services/'+services[0]['metadata']['name']+':80/proxy/version'
        body = self.kube(path)
        assert body['version'] == expected, 'Application version differs from expected'

    def mutate(self, app, method, suffix='', **changes):
        current = self.client.get('/apps/'+app)
        return self.client.mutate(method, '/apps/'+app+suffix, {'version':current['version'], **changes})

    def browser_setup(self):
        if not self.args.setup:
            raise ValueError('First administrator creation requires --setup on a fresh test installation')
        pods = self.kube('/api/v1/namespaces/platform-system/pods')['items']
        pod = next(p['metadata']['name'] for p in pods if p['metadata']['labels'].get('app.kubernetes.io/name') == 'platform' and not p['metadata'].get('deletionTimestamp'))
        with urllib.request.urlopen(self.kubernetes+'/api/v1/namespaces/platform-system/pods/'+pod+'/log', timeout=20) as response:
            logs = response.read().decode()
        token = re.search(r'Bootstrap token[^\n]*\n\s+([A-Za-z0-9_-]+)', logs)
        if token is None: raise RuntimeError('No current unused bootstrap token is available')
        from playwright.sync_api import sync_playwright
        with sync_playwright() as playwright:
            browser = playwright.chromium.launch()
            try:
                page = browser.new_page();page.set_default_timeout(30000)
                page.goto(self.url+'/');page.wait_for_url('**/setup')
                page.get_by_label('Bootstrap token').fill('invalid-token')
                page.get_by_label('Administrator password', exact=True).fill(self.password)
                page.get_by_label('Confirm password').fill(self.password)
                with page.expect_response(lambda r:r.url.endswith('/api/v1/setup') and r.request.method == 'POST') as response:
                    page.get_by_role('button',name='Create account').click()
                assert response.value.status == 401
                message = response.value.json()['error']['message']
                page.get_by_text(message, exact=True).wait_for()
                page.get_by_label('Bootstrap token').fill(token.group(1))
                with page.expect_response(lambda r:r.url.endswith('/api/v1/setup') and r.request.method == 'POST') as response:
                    page.get_by_role('button',name='Create account').click()
                assert response.value.status == 201
                self.save()
                page.wait_for_url('**/login')
                page.get_by_label('Password',exact=True).fill(self.password)
                page.get_by_role('button',name='Sign in',exact=True).click()
                page.wait_for_url('**/apps')
            finally: browser.close()
        self.record('Browser: root opens setup; nested token error displayed; administrator created and login succeeded')

    def browser_public_image(self):
        """Verify unmodified release compatibility with a registry image."""
        from playwright.sync_api import sync_playwright
        with sync_playwright() as playwright:
            browser = playwright.chromium.launch()
            try:
                page = browser.new_page(); page.set_default_timeout(30000)
                page.goto(self.url+'/login')
                page.get_by_label('Password', exact=True).fill(self.password)
                page.get_by_role('button', name='Sign in', exact=True).click()
                page.wait_for_url('**/apps')
                page.goto(self.url+'/apps/new')
                page.get_by_label('Image reference').fill(self.args.public_image)
                with page.expect_response(lambda r:r.url.endswith('/api/v1/images/inspect') and r.request.method=='POST') as inspection:
                    page.get_by_role('button', name='Inspect image', exact=True).click()
                assert inspection.value.status == 200, 'Public image inspection failed'
                page.get_by_role('button', name='Next').click()
                page.get_by_label('Name', exact=True).fill('pi-nginx-'+secrets.token_hex(3))
                page.get_by_label('Container port').fill('8080')
                page.get_by_label('Readiness path').fill('/')
                for _ in range(3):
                    page.get_by_role('button', name='Next').click()
                with page.expect_response(lambda r:r.url.endswith('/api/v1/apps') and r.request.method=='POST') as response:
                    page.get_by_role('button', name='Deploy', exact=True).click()
                assert response.value.status == 202
                accepted = response.value.json(); app = accepted['applicationId']
                self.state['publicTestApp'] = app; self.save()
                self.client.poll(accepted['operationId'])
                page.goto(self.url+'/apps/'+app)
                page.get_by_text('ready', exact=True).wait_for()
                selector = urllib.parse.quote('platform.local/app-id='+app)
                deployments = self.kube('/apis/apps/v1/namespaces/platform-apps/deployments?labelSelector='+selector)['items']
                assert len(deployments) == 1
                pod = deployments[0]['spec']['template']['spec']
                container = pod['containers'][0]
                assert pod['securityContext']['fsGroup'] == 2000
                assert container['securityContext']['readOnlyRootFilesystem'] is True
                assert container['securityContext']['allowPrivilegeEscalation'] is False
                assert any(v.get('emptyDir', {}).get('sizeLimit') == '64Mi' for v in pod['volumes'])
                assert any(m['mountPath'] == '/tmp' for m in container['volumeMounts'])
                services = self.kube('/api/v1/namespaces/platform-apps/services?labelSelector='+selector)['items']
                assert len(services) == 1
                path = '/api/v1/namespaces/platform-apps/services/'+services[0]['metadata']['name']+':80/proxy/'
                with urllib.request.urlopen(self.kubernetes+path, timeout=20) as response:
                    assert response.status == 200 and b'Welcome to nginx!' in response.read()
                revision = self.client.get('/apps/'+app+'/revisions')['items'][0]
                (self.args.artifacts/'public-image.json').write_text(json.dumps({
                    'release': self.args.version, 'image': self.args.public_image,
                    'frozenImage': revision['frozenImage'], 'httpStatus': 200,
                    'readOnlyRootFilesystem': True, 'temporaryVolume': '64Mi',
                }, indent=2)+'\n')
                self.record('Browser: unprivileged Nginx deployed without patches; Ready, HTTP 200 and writable /tmp with container hardening verified')
                self.mutate(app, 'DELETE'); self.wait_for_pod_deletion(app)
                del self.state['publicTestApp']; self.save()
                self.record('Public-image acceptance application removed; retained lifecycle application preserved')
            finally:
                browser.close()

    def blocked_first_removal(self, image):
        code, accepted = self.client.request('POST', '/apps', {
            'name': 'pi-blocked-'+secrets.token_hex(3), 'image': image,
            'containerPort': 8080, 'profile': 'http',
            'health': {'startupPath': '/healthz', 'readinessPath': '/readyz'},
            'exposure': {'mode': 'internal'},
        }, {'Idempotency-Key': secrets.token_hex(16)})
        assert code == 202
        app = accepted['applicationId']
        self.state['blockedTestApp'] = app; self.save()
        operation = self.client.poll(accepted['operationId'], terminal='needs_attention')
        current = self.client.get('/apps/'+app)
        assert not current['lastSuccessfulRevision'] and current['activeOperationId'] == operation['id']
        frozen = self.client.get('/apps/'+app+'/revisions')['items'][0]['frozenImage']
        code, _ = self.client.request('PATCH', '/apps/'+app,
            {'version': current['version'], 'replicas': 2}, {'Idempotency-Key': secrets.token_hex(16)})
        assert code == 409
        from playwright.sync_api import sync_playwright
        with sync_playwright() as playwright:
            browser = playwright.chromium.launch()
            try:
                page = browser.new_page(); page.set_default_timeout(30000)
                page.goto(self.url+'/login'); page.get_by_label('Password', exact=True).fill(self.password)
                page.get_by_role('button', name='Sign in', exact=True).click(); page.wait_for_url('**/apps')
                page.goto(self.url+'/apps/'+app)
                page.get_by_text('The first deployment has no successful revision to restore.', exact=False).wait_for()
                page.on('dialog', lambda dialog: dialog.accept())
                with page.expect_response(lambda r:r.url.endswith('/api/v1/apps/'+app) and r.request.method=='DELETE') as response:
                    page.get_by_role('button', name='Delete', exact=True).click()
                assert response.value.status == 202
                self.client.poll(response.value.json()['operationId'])
            finally:
                browser.close()
        self.wait_for_pod_deletion(app)
        original = self.client.get('/operations/'+operation['id'])
        assert original['state'] == 'failed' and original['stage'] == 'removal_requested'
        assert self.client.get('/apps/'+app+'/revisions')['items'][-1]['frozenImage'] == frozen
        assert app not in [item['id'] for item in self.client.get('/apps')['items']]
        del self.state['blockedTestApp']; self.save()
        self.record('Published release: failed first deployment retained its lock/409; browser deletion removed the workload and retained failed operation/frozen history')

    def run(self):
        if self.client.get('/setup/status')['setupAvailable']:
            self.browser_setup()
        elif not self.state.get('password'):
            raise RuntimeError('Existing administrator has no matching acceptance state; installation was not changed')
        self.client.login()
        system = self.client.get('/system')
        assert system['version'] == self.args.version.removeprefix('v'), 'Installed version does not match requested release'
        assert system['clusterReachable'] and system['architecture'] == 'arm64'
        self.record('Native ARM64 system reports the expected release and a reachable cluster')
        if self.args.resume:
            app = self.state['retainedApp']
            current = self.client.get('/apps/'+app)
            assert current['lastSuccessfulRevision'] == self.state['retainedRevision']
            self.http(app, 'v1')
            self.record('After host reboot: administrator, retained application, revision history and HTTP response recovered')
            if self.args.public_image:
                self.browser_public_image()
            return
        images = json.loads(self.args.release_manifest.read_text())['targets']['arm64']['images']
        accepted,_ = self.client.mutate('POST','/apps',{'name':'pi-acceptance-'+secrets.token_hex(3),'image':images['v1'],'containerPort':8080,'profile':'http','health':{'startupPath':'/healthz','readinessPath':'/readyz'},'exposure':{'mode':'internal'}})
        app = accepted['applicationId'];self.state['retainedApp']=app;self.save()
        first = self.client.get('/apps/'+app);original = first['lastSuccessfulRevision']
        assert first['status']['phase'] == 'ready' and first['status']['readyReplicas'] == 1
        self.http(app,'v1');self.record('Digest-pinned bundled v1 deployed; camelCase status and actual HTTP response verified')
        self.mutate(app,'PATCH',image=images['v2']);self.http(app,'v2')
        for replicas in (2,0,1): self.mutate(app,'PATCH',replicas=replicas)
        self.record('Healthy v2 update and manual scaling to 2, 0 and 1 completed')
        before = self.pod_uids(app)
        accepted,_ = self.mutate(app,'POST','/restart')
        deadline=time.monotonic()+60
        while self.pod_uids(app)==before and time.monotonic()<deadline:time.sleep(1)
        assert self.pod_uids(app)!=before, 'Restart did not replace the Pod'
        assert self.client.get('/apps/'+app+'/revisions')['items'][0]['frozenImage']==images['v2']
        self.record('Restart replaced the Pod while retaining the exact image digest')
        current=self.client.get('/apps/'+app)
        _,operation=self.client.mutate('PATCH','/apps/'+app,{'version':current['version'],'image':images['v3']},terminal='failed')
        restored=self.client.get('/apps/'+app)
        assert operation['stage']=='restored' and not restored['activeOperationId']
        assert restored['desiredRevision']==current['lastSuccessfulRevision']
        self.http(app,'v2');self.record('Unhealthy v3 failed; v2 release and live workload restored, lock released after verification')
        self.mutate(app,'POST','/rollback',revisionId=original);self.http(app,'v1')
        self.record('Explicit rollback restored the original successful v1 snapshot')
        # Exercise deletion separately, retaining one healthy application for reboot.
        deleted,_=self.client.mutate('POST','/apps',{'name':'pi-delete-'+secrets.token_hex(3),'image':images['v1'],'containerPort':8080,'profile':'http','exposure':{'mode':'internal'}})
        self.mutate(deleted['applicationId'],'DELETE')
        assert self.client.get('/apps/'+deleted['applicationId']+'/revisions')['items']
        self.wait_for_pod_deletion(deleted['applicationId'])
        self.record('Application deletion removed Pods and retained revision history')
        self.state['retainedRevision']=self.client.get('/apps/'+app)['lastSuccessfulRevision'];self.save()
        self.record('One healthy test application retained for same-version reinstall and host-reboot verification')
        if self.args.blocked_first_removal:
            self.blocked_first_removal(images['v3'])
        if self.args.public_image:
            self.browser_public_image()


def main():
    parser=argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--url',required=True)
    parser.add_argument('--kubernetes-url',required=True)
    parser.add_argument('--version',required=True)
    parser.add_argument('--release-manifest',type=Path,required=True)
    parser.add_argument('--state',type=Path,required=True)
    parser.add_argument('--artifacts',type=Path,required=True)
    parser.add_argument('--setup',action='store_true')
    parser.add_argument('--resume',action='store_true')
    parser.add_argument('--public-image', help='Opt-in unprivileged Nginx image for the browser temporary-files compatibility check')
    parser.add_argument('--blocked-first-removal', action='store_true', help='Exercise a failed first deployment and guarded browser removal (adds a deliberate Helm timeout)')
    Native(parser.parse_args()).run()

if __name__=='__main__':main()
