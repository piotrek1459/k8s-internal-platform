"""External crash barriers on an owned disposable cluster; no broker test hooks."""
import base64
import contextlib
import hashlib
import json
import secrets
import socket
import subprocess
import tempfile
import time
import urllib.error
import urllib.request
from pathlib import Path

from support import ROOT, free_ports
from resilience import control_secret, fixture_images, move_tag, release_identity


@contextlib.contextmanager
def pod_forward(environment, namespace, name, remote_port):
    port = free_ports(1)[0]
    process = subprocess.Popen(['kubectl', '--kubeconfig', str(environment.config),
        '--context', 'k3d-'+environment.name, '-n', namespace, 'port-forward',
        '--address', '127.0.0.1', 'pod/'+name, f'{port}:{remote_port}'],
        stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, start_new_session=True)
    environment.forwards.append(process)
    try:
        deadline = time.monotonic()+30
        while time.monotonic() < deadline:
            if process.poll() is not None:
                raise AssertionError('Owned fixture forward exited')
            with socket.socket() as connection:
                if connection.connect_ex(('127.0.0.1', port)) == 0:
                    break
            time.sleep(.1)
        else:
            raise AssertionError('Owned fixture forward did not become ready')
        yield f'http://127.0.0.1:{port}'
    finally:
        environment.stop_forward(process)


class Barrier:
    """Admission/SQLite fixture in platform-system, absent from released images."""
    def __init__(self, environment):
        if not environment.owns_cluster or not environment.name.startswith('platform-e2e-'):
            raise RuntimeError('Crash barriers require an owned disposable E2E cluster')
        self.e = environment
        self.name = 'operation-barrier-'+secrets.token_hex(4)
        self.token = secrets.token_urlsafe(32)
        environment.secret_values.append(self.token)
        self.directory = tempfile.TemporaryDirectory(prefix='owned-platform-barrier-')
        self.folder = Path(self.directory.name)
        self.forward = None
        self.resources = []

    def create(self, obj):
        metadata = obj['metadata']
        metadata.setdefault('labels', {})['platform.local/e2e-owner'] = self.e.name
        file = self.folder/'resource.json'
        file.write_text(json.dumps(obj)); file.chmod(0o600)
        self.e.kubectl('create', '-f', file)
        namespace = metadata.get('namespace')
        options = ['-n', namespace] if namespace else []
        created = json.loads(self.e.kubectl(*options, 'get', obj['kind'], metadata['name'], '-o', 'json').stdout)
        self.resources.append((obj['kind'], namespace, metadata['name'], created['metadata']['uid']))

    def __enter__(self):
        try:
            if not hasattr(self.e, 'barrier_image'):
                port = self.e.env['PLATFORM_REGISTRY_PORT']
                local = f'localhost:{port}/operation-barrier:latest'
                self.e.fixture_image_tags.append(local)
                self.e.run('docker', 'build', '-f', ROOT/'broker/integration/fixtures/barrier/Dockerfile',
                    '-t', local, ROOT/'broker', timeout=900)
                self.e.run('docker', 'push', local)
                self.e.barrier_image = f'k3d-{self.e.name}-registry:{port}/operation-barrier:latest'
            hostname = self.name+'.platform-system.svc'
            self.e.run('openssl', 'req', '-x509', '-newkey', 'rsa:2048', '-nodes', '-days', '2',
                '-subj', '/CN='+hostname, '-addext', 'basicConstraints=critical,CA:TRUE',
                '-addext', 'subjectAltName=DNS:'+hostname,
                '-keyout', self.folder/'tls.key', '-out', self.folder/'tls.crt', private=True)
            certificate = base64.b64encode((self.folder/'tls.crt').read_bytes()).decode()
            self.create({'apiVersion':'v1','kind':'Secret','metadata':{'name':self.name,'namespace':'platform-system'},
                'type':'kubernetes.io/tls','data':{'tls.crt':certificate,
                    'tls.key':base64.b64encode((self.folder/'tls.key').read_bytes()).decode(),
                    'control':base64.b64encode(self.token.encode()).decode()}})
            labels = {'app':self.name}
            self.create({'apiVersion':'v1','kind':'Service','metadata':{'name':self.name,'namespace':'platform-system'},
                'spec':{'selector':labels,'ports':[{'name':'admission','port':443,'targetPort':8443}]}})
            self.create({'apiVersion':'v1','kind':'Pod','metadata':{'name':self.name,'namespace':'platform-system','labels':labels},
                'spec':{'automountServiceAccountToken':False,'securityContext':{'runAsNonRoot':True,'runAsUser':65532,'runAsGroup':65532,'fsGroup':65532},
                    'containers':[{'name':'barrier','image':self.e.barrier_image,
                        'securityContext':{'allowPrivilegeEscalation':False,'readOnlyRootFilesystem':True,'capabilities':{'drop':['ALL']}},
                        'resources':{'requests':{'cpu':'25m','memory':'32Mi'},'limits':{'cpu':'500m','memory':'384Mi'}},
                        'env':[{'name':'FIXTURE_CONTROL_TOKEN','valueFrom':{'secretKeyRef':{'name':self.name,'key':'control'}}},
                            {'name':'FIXTURE_OWNED_CLUSTER','value':self.e.name}],
                        'volumeMounts':[{'name':'metadata','mountPath':'/var/lib/platform'},{'name':'tls','mountPath':'/certs','readOnly':True}],
                        'readinessProbe':{'httpGet':{'path':'/healthz','port':8080},'periodSeconds':1}}],
                    'volumes':[{'name':'metadata','persistentVolumeClaim':{'claimName':'platform-metadata'}},{'name':'tls','secret':{'secretName':self.name}}]}})
            self.e.kubectl('-n','platform-system','wait','--for=condition=Ready','pod/'+self.name,'--timeout=120s')
            self.forward = pod_forward(self.e, 'platform-system', self.name, 8080)
            self.url = self.forward.__enter__()
            self.create({'apiVersion':'admissionregistration.k8s.io/v1','kind':'ValidatingWebhookConfiguration',
                'metadata':{'name':self.name},'webhooks':[{'name':self.name+'.platform.test','admissionReviewVersions':['v1'],
                    'sideEffects':'None','failurePolicy':'Fail','timeoutSeconds':30,
                    'namespaceSelector':{'matchLabels':{'kubernetes.io/metadata.name':'platform-apps'}},
                    'clientConfig':{'caBundle':certificate,'service':{'name':self.name,'namespace':'platform-system','path':'/admit','port':443}},
                    'rules':[{'operations':['CREATE','UPDATE'],'apiGroups':[''],'apiVersions':['v1'],'resources':['secrets'],'scope':'Namespaced'},
                        {'operations':['CREATE','UPDATE'],'apiGroups':['apps'],'apiVersions':['v1'],'resources':['deployments'],'scope':'Namespaced'}]}]})
            # Admission registration is asynchronous. Prove a request reaches the
            # fixture before treating its next hit as a deterministic barrier.
            self.configure('deny-secret', message='controlled-admission-ready')
            deadline = time.monotonic()+30
            while time.monotonic() < deadline:
                probe = 'admission-probe-'+secrets.token_hex(4)
                obj = {'apiVersion':'v1','kind':'Secret','metadata':{'name':probe,'namespace':'platform-apps'},'type':'Opaque','stringData':{'value':'probe'}}
                file = self.folder/'probe.json';file.write_text(json.dumps(obj));file.chmod(0o600)
                result = self.e.kubectl('create','-f',file,check=False)
                if result.returncode and self.status()['lastName'] == probe:
                    break
                if not result.returncode:
                    self.e.kubectl('-n','platform-apps','delete','secret',probe)
                time.sleep(.2)
            else:
                raise AssertionError('Admission fixture was not positively observed')
            self.configure('allow')
            return self
        except BaseException:
            self.__exit__(None,None,None)
            raise

    def request(self, method, path, body=None):
        req = urllib.request.Request(self.url+path, method=method,
            data=None if body is None else json.dumps(body).encode(),
            headers={'Content-Type':'application/json','X-Fixture-Control':self.token})
        with urllib.request.urlopen(req, timeout=10) as response:
            raw = response.read()
            return json.loads(raw) if raw else None

    def configure(self, mode, **options):
        self.request('POST','/configure',{'mode':mode,**options})

    def status(self):
        return self.request('GET','/status')

    def state(self, operation):
        return self.request('POST','/state',{'operation':operation})

    def wait(self, accepted):
        deadline = time.monotonic()+20
        while time.monotonic()<deadline:
            status = self.status()
            if status['waiting'] > 0 and accepted['applicationId'].replace('_','-') in status['lastName']:
                return self.state(accepted['operationId'])
            time.sleep(.1)
        raise AssertionError('Expected admission crash barrier was not reached')

    def __exit__(self, *_):
        if self.forward is not None:
            with contextlib.suppress(Exception):
                self.configure('allow');self.request('POST','/lock',{'held':False})
            self.forward.__exit__(None,None,None)
        for kind, namespace, name, uid in reversed(self.resources):
            options = ['-n', namespace] if namespace else []
            result = self.e.kubectl(*options,'get',kind,name,'-o','json',check=False)
            if result.returncode:
                continue
            obj = json.loads(result.stdout)
            if obj['metadata']['uid'] != uid or obj['metadata']['labels'].get('platform.local/e2e-owner') != self.e.name:
                raise AssertionError('Refused cleanup of changed barrier fixture identity')
            self.e.kubectl(*options,'delete',kind,name,'--wait=true','--timeout=60s')
        self.directory.cleanup()


def frozen_identity(state):
    value = state['frozen']
    if not all(value.get(k) for k in ('image','chartVersion','chart','values','frozenAt')):
        raise AssertionError('No complete durable execution snapshot at the crash barrier')
    return value


def submit(env, image, secret, **options):
    body = {'name':'crash-matrix-'+secrets.token_hex(4),'image':image,'containerPort':8080,'profile':'http',
        'health':{'readinessPath':'/readyz'},'exposure':{'mode':'internal'},
        'secretRefs':[{'environmentVariable':'FIXTURE_CONTROL_TOKEN','secretId':secret['id'],'version':1}],**options}
    code, accepted = env.client.request('POST','/apps',body,{'Idempotency-Key':secrets.token_hex(16)})
    if code != 202:
        raise AssertionError('Crash-matrix application was not accepted')
    return accepted


def release_candidate(environment, app, image, token):
    deadline = time.monotonic()+90
    while time.monotonic()<deadline:
        candidates = [p for p in environment.resource('pods',app) if not p['metadata'].get('deletionTimestamp')
            and p['spec']['containers'][0]['image']==image and any(s.get('state',{}).get('running') for s in p.get('status',{}).get('containerStatuses',[]))]
        if len(candidates)==1:
            break
        time.sleep(.2)
    else:
        raise AssertionError('No unique running candidate Pod for readiness control')
    with pod_forward(environment,'platform-apps',candidates[0]['metadata']['name'],8080) as url:
        req=urllib.request.Request(url+'/control/ready',method='POST',data=b'',headers={'X-Fixture-Control':token})
        with urllib.request.urlopen(req,timeout=10) as response:
            if response.status!=204:
                raise AssertionError('Candidate readiness was not released')


def helm_records(environment, app):
    return json.loads(environment.kubectl('-n','platform-apps','get','secrets',
        '-l','owner=helm,name=app-'+app.replace('_','-'),'-o','json').stdout)['items']


def check_crash_boundary(case, stage):
    from resilience import wait_pending
    e=case.e;images=fixture_images(e);token,secret=control_secret(e)
    with Barrier(e) as gate:
        image=move_tag(e,'v1') if stage=='before-helm' else images['v1']
        if stage!='pending':
            gate.configure('hold-create' if stage=='before-helm' else 'hold-deployed')
        accepted=submit(e,image,secret,environment={'FIXTURE_START_READY':'false' if stage=='pending' else 'true'})
        app=accepted['applicationId'];op=accepted['operationId']
        if stage=='pending':
            wait_pending(e,app);before=gate.state(op)
        else:
            before=gate.wait(accepted)
        frozen=frozen_identity(before)
        case.assertEqual(json.loads(frozen['secretRefs']),[{'EnvironmentVariable':'FIXTURE_CONTROL_TOKEN','SecretID':secret['id'],'Version':1}])
        before_records=helm_records(e,app)
        if stage=='before-helm':
            case.assertEqual(before_records,[])
            case.assertEqual(e.resource('deployments,services,ingresses',app),[])
            move_tag(e,'v2')
            code,inspection=e.client.request('POST','/images/inspect',{'image':image})
            case.assertEqual(code,200);case.assertFalse(frozen['image'].endswith('@'+inspection['digest']))
        else:
            case.assertEqual(len(before_records),1)
            case.assertEqual(before_records[0]['metadata']['labels']['status'],'pending-install')
            if stage=='after-apply':
                e.wait_replicas(app,1);e.http_version(app,'fixture-v1')
        new_value=secrets.token_urlsafe(24);e.secret_values.append(new_value)
        code,_=e.client.request('POST','/secrets/'+secret['id']+'/versions',{'data':{'value':new_value}})
        case.assertEqual(code,201)
        e.crash_broker(after_kill=lambda:gate.configure('allow'))
        if stage=='before-helm':
            e.client.poll(op);e.wait_replicas(app,1);e.http_version(app,'fixture-v1')
            case.assertEqual(len(helm_records(e,app)),1)
            case.assertEqual(len(e.client.get('/apps/'+app+'/revisions')['items']),1)
        else:
            e.client.poll(op,terminal='needs_attention')
            after_records=helm_records(e,app)
            case.assertTrue(after_records[0]['metadata']['uid']==before_records[0]['metadata']['uid'] and after_records[0]['data']==before_records[0]['data'],'Pending Helm evidence was modified')
            current=e.client.get('/apps/'+app)
            case.assertEqual(current['activeOperationId'],op)
            case.assertEqual(e.client.request('PATCH','/apps/'+app,{'version':current['version'],'replicas':2},{'Idempotency-Key':secrets.token_hex(16)})[0],409)
            e.restart_broker()
            case.assertEqual(e.client.get('/operations/'+op)['state'],'needs_attention')
        case.assertEqual(frozen_identity(gate.state(op)),frozen)
        # Tracked first-deployment removal is the supported pending resolution;
        # no SQL rows or Helm storage records are edited by the fixture.
        e.delete(app)
        case.assertTrue(e.client.get('/apps/'+app+'/revisions')['items'])
        case.assertEqual(e.client.request('DELETE','/secrets/'+secret['id'])[0],204)
        e.record('Crash barrier '+stage+': exact frozen image/Secret/chart/values retained; verified completion or durable block; tracked cleanup',frozen=frozen)


def check_terminal_sql_boundary(case):
    from resilience import wait_pending
    e=case.e;images=fixture_images(e);token,secret=control_secret(e)
    with Barrier(e) as gate:
        accepted=submit(e,images['v1'],secret,environment={'FIXTURE_START_READY':'true'})
        app=accepted['applicationId'];e.client.poll(accepted['operationId']);e.wait_replicas(app,1)
        current=e.client.get('/apps/'+app);previous=current['lastSuccessfulRevision']
        code,accepted=e.client.request('PATCH','/apps/'+app,{'version':current['version'],'image':images['v2'],'environment':{}},{'Idempotency-Key':secrets.token_hex(16)})
        case.assertEqual(code,202);op=accepted['operationId']
        deadline=time.monotonic()+90
        while time.monotonic()<deadline:
            records=helm_records(e,app)
            if records and max(records,key=lambda r:int(r['metadata']['labels']['version']))['metadata']['labels']['status']=='pending-upgrade':
                break
            time.sleep(.2)
        else:raise AssertionError('Candidate upgrade did not reach its pending barrier')
        frozen=frozen_identity(gate.state(op))
        code,queued=e.client.request('POST','/apps',{'name':'sql-queued-'+secrets.token_hex(4),'image':e.image('v1'),'profile':'http','containerPort':8080,'exposure':{'mode':'internal'}},{'Idempotency-Key':secrets.token_hex(16)})
        case.assertEqual(code,202);case.assertEqual(e.client.get('/operations/'+queued['operationId'])['state'],'queued')
        gate.request('POST','/lock',{'held':True})
        try:
            release_candidate(e,app,frozen['image'],token)
            deadline=time.monotonic()+90
            while time.monotonic()<deadline:
                records=helm_records(e,app);latest=max(records,key=lambda r:int(r['metadata']['labels']['version']))
                if latest['metadata']['labels']['status']=='deployed':break
                time.sleep(.1)
            else:raise AssertionError('Real Helm target did not finish before SQL recording')
            e.wait_replicas(app,1)
            state=gate.state(op)
            case.assertEqual(state['state'],'running');case.assertEqual(state['result'],'pending')
            case.assertEqual(state['lastSuccessfulRevision'],previous);case.assertEqual(state['activeOperation'],op)
            case.assertTrue(gate.status()['locked']);identity=release_identity(e,app)
            e.crash_broker(after_kill=lambda:gate.request('POST','/lock',{'held':False}))
        finally:
            gate.request('POST','/lock',{'held':False})
        e.client.poll(op);e.client.poll(queued['operationId'])
        case.assertEqual(frozen_identity(gate.state(op)),frozen)
        case.assertEqual(release_identity(e,app),identity)
        case.assertEqual(len(e.client.get('/apps/'+app+'/revisions')['items']),2)
        case.assertEqual(len(e.client.get('/apps/'+queued['applicationId']+'/revisions')['items']),1)
        e.http_version(app,'fixture-v2');e.http_version(queued['applicationId'],'v1')
        e.delete(app);e.delete(queued['applicationId'])
        case.assertEqual(e.client.request('DELETE','/secrets/'+secret['id'])[0],204)
        e.record('Actual runtime SIGKILL after deployed Helm target and before terminal SQL: original target recovered once, frozen identity retained, independent queued app completed once')


@contextlib.contextmanager
def legacy_private_ingress(environment):
    """Enable API private routes without exposing a host listener in this case."""
    e=environment
    e.management.__exit__(None,None,None)
    e.run(ROOT/'.local/bin/helm','--kubeconfig',e.config,'--kube-context','k3d-'+e.name,
        'upgrade','platform',ROOT/'deploy/helm/platform','-n','platform-system','--reuse-values',
        '--set','privateIngressEnabled=true','--wait','--timeout','180s')
    e.start_management()
    try:
        yield
    finally:
        e.management.__exit__(None,None,None)
        e.run(ROOT/'.local/bin/helm','--kubeconfig',e.config,'--kube-context','k3d-'+e.name,
            'upgrade','platform',ROOT/'deploy/helm/platform','-n','platform-system','--reuse-values',
            '--set','privateIngressEnabled=false','--wait','--timeout','180s')
        e.start_management()


class RoleRestriction:
    """UID/version-guarded edits to one Role in an owned disposable cluster."""
    def __init__(self, environment):
        if not environment.owns_cluster:
            raise RuntimeError('Role fault requires an owned disposable cluster')
        self.e=environment
        self.original=json.loads(environment.kubectl('-n','platform-apps','get','role','platform-apps','-o','json').stdout)
        self.current=self.original['rules']

    def set(self, *, deployment_get=True, ingress_list=True):
        rules=json.loads(json.dumps(self.original['rules']))
        for rule in rules:
            if 'apps' in rule.get('apiGroups',[]) and 'deployments' in rule.get('resources',[]) and not deployment_get:
                rule['verbs']=[v for v in rule['verbs'] if v!='get']
            if 'networking.k8s.io' in rule.get('apiGroups',[]) and 'ingresses' in rule.get('resources',[]) and not ingress_list:
                rule['verbs']=[v for v in rule['verbs'] if v!='list']
        live=json.loads(self.e.kubectl('-n','platform-apps','get','role','platform-apps','-o','json').stdout)
        if live['metadata']['uid']!=self.original['metadata']['uid'] or live['rules']!=self.current:
            raise AssertionError('Role identity/rules changed outside the controlled fixture; repair refused')
        patch=[{'op':'test','path':'/metadata/uid','value':live['metadata']['uid']},
            {'op':'test','path':'/metadata/resourceVersion','value':live['metadata']['resourceVersion']},
            {'op':'replace','path':'/rules','value':rules}]
        with tempfile.TemporaryDirectory(prefix='owned-role-fault-') as folder:
            file=Path(folder)/'patch.json';file.write_text(json.dumps(patch));file.chmod(0o600)
            self.e.kubectl('-n','platform-apps','patch','role','platform-apps','--type=json','--patch-file',file)
        self.current=rules

    def restore(self):
        self.set()


def check_failed_restoration(case):
    for start, target in (({'mode':'internal'},{'mode':'lan','hostname':'failed.apps.home.arpa'}),
            ({'mode':'lan','hostname':'original.apps.home.arpa'},{'mode':'internal'}),
            ({'mode':'lan','hostname':'original.apps.home.arpa'},{'mode':'lan','hostname':'renamed.apps.home.arpa'})):
        _check_failed_restoration_transition(case, start, target)


def _check_failed_restoration_transition(case, start, target):
    e=case.e;images=fixture_images(e);token,secret=control_secret(e)
    with legacy_private_ingress(e), Barrier(e) as gate:
        restriction=RoleRestriction(e)
        original_pvc=json.loads(e.kubectl('-n','platform-system','get','pvc','platform-metadata','-o','json').stdout)['metadata']['uid']
        accepted=submit(e,images['v1'],secret,environment={'FIXTURE_START_READY':'true'},exposure=start)
        app=accepted['applicationId'];e.client.poll(accepted['operationId']);e.wait_replicas(app,1)
        original=e.client.get('/apps/'+app)['lastSuccessfulRevision']
        password=secrets.token_urlsafe(24);e.secret_values.append(password)
        code,credential=e.client.request('POST','/registry-credentials',{'label':'restoration-'+secrets.token_hex(4),
            'registry':images['v2'].split('/')[0],'username':'controlled-user','password':password})
        case.assertEqual(code,201)
        try:
            current=e.client.get('/apps/'+app)
            code,accepted=e.client.request('PATCH','/apps/'+app,{'version':current['version'],'image':images['v2'],
                'environment':{'CANDIDATE_ONLY':'removed-after-restoration'},'registryCredentialId':credential['id'],
                'secretRefs':[{'environmentVariable':'FIXTURE_CONTROL_TOKEN','secretId':secret['id'],'version':1},
                    {'environmentVariable':'CANDIDATE_SECRET','secretId':secret['id'],'version':1}],
                'exposure':target},{'Idempotency-Key':secrets.token_hex(16)})
            case.assertEqual(code,202);op=accepted['operationId']
            deadline=time.monotonic()+90
            while time.monotonic()<deadline:
                routes=e.resource('ingresses',app)
                state=gate.state(op)
                candidates=[p for p in e.resource('pods',app) if p['spec']['containers'][0]['image']==state['frozen']['image'] and any(s.get('state',{}).get('running') and not s.get('ready') for s in p.get('status',{}).get('containerStatuses',[]))]
                if candidates and len(routes)==(1 if target['mode']=='lan' else 0) and (not routes or routes[0]['spec']['rules'][0]['host']==target['hostname']):
                    break
                time.sleep(.2)
            else:raise AssertionError('Failed candidate did not actually introduce its route and unready workload')
            frozen=frozen_identity(state)
            # Refuse SDK readiness reads to fail the running Helm wait, and keep
            # restoration preparation blocked without changing SQL or Helm data.
            restriction.set(deployment_get=False,ingress_list=False)
            e.client.poll(op,terminal='needs_attention')
            failed_records=helm_records(e,app)
            failed_candidate=next(r for r in failed_records if r['metadata']['labels']['status']=='failed')
            before=gate.state(op);case.assertEqual(before['result'],'failed')
            case.assertEqual(e.client.get('/apps/'+app)['activeOperationId'],op)
            e.restart_broker()
            case.assertEqual(e.client.get('/operations/'+op)['state'],'needs_attention')
            current=e.client.get('/apps/'+app)
            case.assertEqual(e.client.request('PATCH','/apps/'+app,{'version':current['version'],'replicas':2},{'Idempotency-Key':secrets.token_hex(16)})[0],409)
            # Preparation/apply can now complete, but strict absence verification
            # cannot enumerate routes. Real healthy resources must remain locked.
            restriction.set(deployment_get=True,ingress_list=False)
            retry,_=e.client.mutate('POST','/apps/'+app+'/recover',{'version':current['version']},terminal='needs_attention')
            case.assertEqual(retry['operationId'],op)
            restored_routes=e.resource('ingresses',app)
            case.assertEqual(len(restored_routes),1 if start['mode']=='lan' else 0)
            if restored_routes:case.assertEqual(restored_routes[0]['spec']['rules'][0]['host'],start['hostname'])
            restored=e.resource('deployments',app)[0]['spec']['template']['spec']
            variables={v['name'] for v in restored['containers'][0].get('env',[])}
            case.assertFalse({'CANDIDATE_ONLY','CANDIDATE_SECRET'} & variables)
            case.assertEqual(restored.get('imagePullSecrets',[]),[])
            e.http_version(app,'fixture-v1')
            e.restart_broker()
            case.assertEqual(e.client.get('/operations/'+op)['state'],'needs_attention')
            restriction.restore()
            current=e.client.get('/apps/'+app)
            retry,_=e.client.mutate('POST','/apps/'+app+'/recover',{'version':current['version']},terminal='failed')
            case.assertEqual(retry['operationId'],op)
            final=e.client.get('/apps/'+app)
            case.assertFalse(final['activeOperationId']);case.assertEqual(final['lastSuccessfulRevision'],original)
            case.assertEqual(final['desiredRevision'],original)
            case.assertEqual(e.client.get('/operations/'+op)['stage'],'restored')
            case.assertEqual(frozen_identity(gate.state(op)),frozen)
            case.assertEqual(gate.state(op)['result'],'failed')
            records=helm_records(e,app)
            retained=next(r for r in records if r['metadata']['name']==failed_candidate['metadata']['name'])
            case.assertTrue(retained['metadata']['uid']==failed_candidate['metadata']['uid'] and retained['data']==failed_candidate['data'],'Failed Helm candidate evidence changed during recovery')
            import gzip
            configurations=[json.loads(gzip.decompress(base64.b64decode(base64.b64decode(r['data']['release']))))['config'] for r in records]
            hashes=[hashlib.sha256(json.dumps(v,sort_keys=True,separators=(',',':')).encode()).hexdigest() for v in configurations]
            case.assertEqual(hashes.count(frozen['values']),1,'Retry executed the failed candidate again')
            case.assertEqual(len(e.client.get('/apps/'+app+'/revisions')['items']),2)
            case.assertEqual(json.loads(e.kubectl('-n','platform-system','get','pvc','platform-metadata','-o','json').stdout)['metadata']['uid'],original_pvc)
            e.http_version(app,'fixture-v1')
            e.delete(app)
            case.assertEqual(e.client.request('DELETE','/secrets/'+secret['id'])[0],204)
            case.assertEqual(e.client.request('DELETE','/registry-credentials/'+credential['id'])[0],204)
            e.record('Real failed restoration/verification across broker replacements; guarded retry restored route/absence and removed candidate fields without reexecution',transition={'from':start,'to':target})
        finally:
            restriction.restore()


def check_disclosure(case):
    e=case.e
    value='disclosure-'+secrets.token_urlsafe(32)
    encoded=base64.b64encode(value.encode()).decode()
    e.secret_values.extend([value,'controlled-user:'+value])
    needles=[value,encoded,base64.b64encode(('controlled-user:'+value).encode()).decode()]
    with Barrier(e) as gate:
        code,secret=e.client.request('POST','/secrets',{'name':'disclosure-'+secrets.token_hex(4),'data':{'value':value}})
        case.assertEqual(code,201)
        gate.configure('deny-secret',message='controlled '+value+' '+encoded)
        code,body=e.client.request('POST','/secrets',{'name':'rejected-'+secrets.token_hex(4),'data':{'value':value}})
        case.assertEqual(code,503)
        case.assertFalse(any(v in json.dumps(body) for v in needles),'Secret admission response leaked controlled values')
        case.assertGreater(gate.status()['hits'],0)
        gate.configure('allow')
        image=e.image('v1')
        code,credential=e.client.request('POST','/registry-credentials',{'label':'disclosure-'+secrets.token_hex(4),'registry':image.split('/')[0],'username':'controlled-user','password':value})
        case.assertEqual(code,201)
        registry_secret=json.loads(e.kubectl('-n','platform-apps','get','secret','registry-'+credential['id'].replace('_','-'),'-o','json').stdout)
        config=registry_secret['data']['.dockerconfigjson']
        needles.append(config);e.secret_values.append(config)
        app=e.create('disclosure',registryCredentialId=credential['id'],secretRefs=[{'environmentVariable':'APP_SECRET','secretId':secret['id'],'version':1}])
        operations=[]
        for mode in ('deny-helm','deny-deployment'):
            gate.configure(mode,message='controlled '+value+' '+encoded)
            current=e.client.get('/apps/'+app)
            code,accepted=e.client.request('PATCH','/apps/'+app,{'version':current['version'],'environment':{'CHANGED':mode}},{'Idempotency-Key':secrets.token_hex(16)})
            case.assertEqual(code,202);op=accepted['operationId'];operations.append(op)
            e.client.poll(op,terminal='needs_attention')
            case.assertGreaterEqual(gate.status()['hits'],2,'Candidate and restoration denial paths were not exercised')
            gate.configure('allow')
            current=e.client.get('/apps/'+app)
            e.client.mutate('POST','/apps/'+app+'/recover',{'version':current['version']},terminal='failed')
        paths=['/system','/secrets','/registry-credentials','/apps','/apps/'+app,'/apps/'+app+'/revisions','/apps/'+app+'/logs','/apps/'+app+'/events','/apps/'+app+'/drift']
        paths.extend('/operations/'+op for op in operations)
        for path in paths:
            code,body=e.client.request('GET',path)
            case.assertEqual(code,200)
            case.assertFalse(any(v in json.dumps(body) for v in needles),'Ordinary/history/diagnostic response leaked controlled values')
        import gzip
        failed_records=0
        for record in helm_records(e,app):
            payload=gzip.decompress(base64.b64decode(base64.b64decode(record['data']['release'])))
            case.assertFalse(any(v.encode() in payload for v in needles),'Helm storage/values leaked controlled values')
            if record['metadata']['labels']['status']=='failed':failed_records+=1
        case.assertGreaterEqual(failed_records,1,'No persisted failed Helm release exercised failure-description storage')
        scan=gate.request('POST','/scan',{'needles':needles,'positive':secret['id']})
        case.assertTrue(scan['positiveFound']);case.assertGreaterEqual(scan['files'],2)
        case.assertFalse(scan['found'],'SQLite/WAL leaked controlled values')
        raw=e.kubectl('-n','platform-system','logs','deployment/platform','--tail=1000').stdout
        case.assertFalse(any(v in raw for v in needles),'Raw broker log leaked controlled values')
        e.diagnostics()
        for file in e.artifacts.rglob('*'):
            if file.is_file() and file.suffix in ('.json','.xml','.log','.txt'):
                data=file.read_bytes()
                case.assertFalse(any(v.encode() in data for v in needles),'Sanitized artifact leaked controlled values')
        e.delete(app)
        case.assertEqual(e.client.request('DELETE','/secrets/'+secret['id'])[0],204)
        case.assertEqual(e.client.request('DELETE','/registry-credentials/'+credential['id'])[0],204)
        e.record('Controlled config/registry raw/base64/auth/config-json absence verified across actual admission/Helm errors and failed release descriptions, SQLite/WAL positive scan, Helm values/history, ordinary/diagnostic responses, raw broker logs and sanitized artifacts')
