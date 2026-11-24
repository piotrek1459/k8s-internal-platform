"""Actual private TLS traffic on the owned node; native LAN acceptance is separate."""
import base64
import hashlib
from http.cookies import SimpleCookie
import json
from pathlib import Path
import secrets
import socket
import ssl
import subprocess
import tempfile
import time

from support import Client, ROOT, free_ports


class TLSClient(Client):
    """Keep cookie/body material out of command diagnostics and test artifacts."""
    def __init__(self, environment, folder, port):
        super().__init__(environment)
        self.folder, self.port, self.session = folder, port, ''

    def fetch(self, path, host='platform.home.arpa', method='GET', body=None, headers=None, trusted=True):
        response_headers = self.folder/'response.headers'
        args=['curl','--noproxy','*','--silent','--show-error','--connect-timeout','5','--max-time','30',
            '--resolve',f'{host}:{self.port}:127.0.0.1','--request',method,
            '--dump-header',str(response_headers),'--write-out','\n%{http_code}']
        if trusted: args += ['--cacert',str(self.folder/'ca.crt')]
        for key,value in (headers or {}).items(): args += ['--header',key+': '+value]
        if body is not None: args += ['--header','Content-Type: application/json','--data-binary',json.dumps(body)]
        result=self.environment.run(*args,f'https://{host}:{self.port}'+path,private=True,check=False)
        if result.returncode: return result.returncode,None,None
        text,code=result.stdout.rsplit('\n',1)
        return 0,int(code),(text,response_headers.read_text())

    def request(self, method, path, body=None, headers=None, csrf=True):
        fields={**(headers or {})}
        if csrf: fields['X-Requested-With']='XMLHttpRequest'
        if self.session: fields['Cookie']=self.session
        error,code,response=self.fetch('/api/v1'+path,method=method,body=body,headers=fields)
        if error: raise RuntimeError('Private HTTPS request failed with curl status '+str(error))
        text,response_headers=response
        for line in response_headers.splitlines():
            if line.lower().startswith('set-cookie:'):
                parsed=SimpleCookie();parsed.load(line.split(':',1)[1].strip())
                cookie=next(iter(parsed.values()))
                assert cookie['secure'] and cookie['httponly'] and cookie['samesite']=='Strict' and not cookie['domain']
                self.session=cookie.key+'='+cookie.value
                self.environment.secret_values.append(cookie.value)
        return code,json.loads(text) if text else None


def verify_public_listener_refusal(case):
    """A reachable independent controller must not serve forged private routes."""
    e=case.e
    for service_port,port in zip((80,443),free_ports(2)):
        process=subprocess.Popen(['kubectl','--kubeconfig',str(e.config),'--context','k3d-'+e.name,
            '-n','kube-system','port-forward','--address','127.0.0.1','service/traefik',f'{port}:{service_port}'],
            stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL,start_new_session=True)
        e.forwards.append(process)
        try:
            for _ in range(60):
                if process.poll() is not None: raise AssertionError('Independent listener forward exited')
                with socket.socket() as connection:
                    if connection.connect_ex(('127.0.0.1',port))==0: break
                time.sleep(1)
            else: raise AssertionError('Independent listener was not reachable')
            for host in ('platform.home.arpa','demo.apps.home.arpa'):
                scheme='https' if service_port==443 else 'http'
                args=['curl','--noproxy','*','--silent','--show-error','--connect-timeout','5','--max-time','15',
                    '--resolve',f'{host}:{port}:127.0.0.1','--write-out','\n%{http_code}']
                # This is the unrelated controller's generated certificate, not
                # the private CA path. Require an actual HTTP refusal after TLS.
                if service_port==443: args+=['--insecure']
                result=e.run(*args,f'{scheme}://{host}:{port}/healthz',private=True)
                case.assertEqual(int(result.stdout.rsplit('\n',1)[1]),404,'Independent listener served a forged private hostname/SNI')
        finally:e.stop_forward(process)


def observed_peer(environment, folder, port, node_address):
    """Measure the real connection peer; k3d port proxies can replace its IP."""
    context=ssl.create_default_context(cafile=str(folder/'ca.crt'))
    with socket.create_connection(('127.0.0.1',port),timeout=10) as raw:
        with context.wrap_socket(raw,server_hostname='platform.home.arpa'):
            node='k3d-'+environment.name+'-server-0'
            rows=environment.run('docker','exec',node,'cat','/proc/net/tcp',private=True).stdout.splitlines()[1:]
            peers=set()
            for row in rows:
                fields=row.split();local,remote=fields[1:3]
                local_address=socket.inet_ntoa(bytes.fromhex(local.split(':')[0])[::-1])
                if local_address==node_address and int(local.split(':')[1],16)==port and fields[3]=='01':
                    peers.add(socket.inet_ntoa(bytes.fromhex(remote.split(':')[0])[::-1]))
            if len(peers)!=1: raise AssertionError('Expected one observed peer on the owned TLS listener')
            return peers.pop()


def verify_private_lan(case):
    from playwright.sync_api import sync_playwright, expect
    e=case.e
    port=int(e.env['PLATFORM_LAN_TEST_PORT'])
    node=json.loads(e.run('docker','inspect','k3d-'+e.name+'-server-0',private=True).stdout)[0]
    network=node['NetworkSettings']['Networks']['k3d-'+e.name]
    address,gateway=network['IPAddress'],network['Gateway']
    app=None; internal=None; configured=False
    with tempfile.TemporaryDirectory(prefix='platform-private-tls-') as directory:
        folder=Path(directory)
        e.run('openssl','req','-x509','-newkey','rsa:2048','-nodes','-days','2','-subj','/CN=Owned E2E CA',
            '-keyout',folder/'ca.key','-out',folder/'ca.crt','-addext','basicConstraints=critical,CA:TRUE',private=True)
        e.run('openssl','req','-newkey','rsa:2048','-nodes','-subj','/CN=platform.home.arpa',
            '-keyout',folder/'tls.key','-out',folder/'tls.csr',private=True)
        (folder/'extensions').write_text('basicConstraints=critical,CA:FALSE\nkeyUsage=critical,digitalSignature,keyEncipherment\nextendedKeyUsage=serverAuth\nsubjectAltName=DNS:platform.home.arpa,DNS:*.apps.home.arpa\n')
        e.run('openssl','x509','-req','-in',folder/'tls.csr','-CA',folder/'ca.crt','-CAkey',folder/'ca.key',
            '-CAcreateserial','-out',folder/'tls.crt','-days','2','-extfile',folder/'extensions',private=True)
        secret={'apiVersion':'v1','kind':'Secret','metadata':{'name':'platform-private-e2e','namespace':'platform-system'},
            'type':'kubernetes.io/tls','data':{key:base64.b64encode((folder/key).read_bytes()).decode() for key in ('tls.crt','tls.key')}}
        (folder/'secret.json').write_text(json.dumps(secret));e.kubectl('create','-f',folder/'secret.json')
        values={'privateLAN':{'enabled':True,'address':address,'httpsPort':port,'allowedCIDRs':[gateway+'/32'],
            'managementHostname':'platform.home.arpa','applicationDomain':'apps.home.arpa','tlsSecretName':'platform-private-e2e'}}
        client=TLSClient(e,folder,port)

        def wait_http(expected):
            statuses=expected if isinstance(expected,tuple) else (expected,)
            deadline=time.monotonic()+90
            while time.monotonic()<deadline:
                error,code,_=client.fetch('/healthz')
                if not error and code in statuses: return
                time.sleep(1)
            raise AssertionError('Private TLS listener did not return expected health status '+str(expected))

        def wait_version(host, expected):
            # Ready Pods and ingress-provider endpoint observation are separate
            # asynchronous boundaries. Require actual HTTPS convergence too.
            deadline=time.monotonic()+60
            while time.monotonic()<deadline:
                error,code,response=client.fetch('/version',host=host)
                if not error and code==200 and json.loads(response[0])['version']==expected:return
                time.sleep(1)
            raise AssertionError(f'Private app did not serve {expected}; last curl status {error}, HTTP status {code}')

        def configure(settings, expected=200):
            (folder/'values.json').write_text(json.dumps(settings))
            e.management.__exit__(None,None,None)
            e.run(ROOT/'.local/bin/helm','--kubeconfig',e.config,'--kube-context','k3d-'+e.name,
                'upgrade','platform',ROOT/'deploy/helm/platform','--namespace','platform-system','--reuse-values',
                '--values',folder/'values.json','--atomic','--cleanup-on-fail','--wait','--timeout','180s',timeout=420)
            e.start_management()
            if settings['privateLAN']['enabled']: wait_http(expected)

        try:
            configured=True;configure(values,(200,403))
            peer=observed_peer(e,folder,port,address)
            values['privateLAN']['allowedCIDRs']=[peer+'/32']
            configure(values)
            e.record('Measured the actual owned TLS connection peer and applied an exact /32 source policy')
            # Positive verified TLS is required before trusting negative requests.
            case.assertEqual(client.fetch('/healthz')[1],200)
            case.assertEqual(client.fetch('/healthz',trusted=False)[0],60,'Untrusted CA was accepted')
            case.assertNotEqual(client.fetch('/healthz',host='unknown.home.arpa')[0],0,'Unknown SNI was accepted')
            case.assertEqual(client.request('GET','/system')[0],401)
            client.login()
            case.assertEqual(client.get('/system')['privateNetwork']['httpsPort'],port)
            for host in ('platform.home.arpa','apps.home.arpa','two.labels.apps.home.arpa','unrelated.example'):
                code,_=client.request('POST','/apps',{'name':'refused-'+secrets.token_hex(3),'image':e.image('v1'),
                    'containerPort':8080,'profile':'http','exposure':{'mode':'lan','hostname':host}}, {'Idempotency-Key':secrets.token_hex(16)})
                case.assertEqual(code,400)

            # Browser trusts only this controlled leaf's SPKI; OS trust is not changed.
            # Independent curl checks above validate the complete CA/hostname path.
            public=e.run('openssl','x509','-in',folder/'tls.crt','-pubkey','-noout',private=True).stdout
            (folder/'public.pem').write_text(public)
            e.run('openssl','pkey','-pubin','-in',folder/'public.pem','-outform','DER','-out',folder/'public.der',private=True)
            pin=base64.b64encode(hashlib.sha256((folder/'public.der').read_bytes()).digest()).decode()
            management=f'https://platform.home.arpa:{port}'
            hostname='demo.apps.home.arpa';app_url=f'https://{hostname}:{port}/'
            with sync_playwright() as playwright:
                browser=playwright.chromium.launch(args=['--no-proxy-server',
                    '--host-resolver-rules=MAP platform.home.arpa 127.0.0.1,MAP *.apps.home.arpa 127.0.0.1',
                    '--ignore-certificate-errors-spki-list='+pin])
                try:
                    context=browser.new_context();page=context.new_page();page.set_default_timeout(30000)
                    page.goto(management+'/login');page.get_by_label('Password',exact=True).fill(e.password)
                    page.get_by_role('button',name='Sign in',exact=True).click();page.wait_for_url('**/apps')
                    cookies=context.cookies(management)
                    case.assertTrue(cookies and all(c['secure'] and c['httpOnly'] and c['sameSite']=='Strict' for c in cookies))
                    case.assertEqual(context.cookies(app_url),[],'Management cookie leaked to application hostname')
                    page.goto(management+'/apps/new');page.get_by_label('Image reference').fill(e.image('v1'))
                    page.get_by_role('button',name='Inspect image',exact=True).click()
                    page.get_by_role('button',name='Next').click()
                    page.get_by_label('Name',exact=True).fill('private-browser-'+secrets.token_hex(3))
                    page.get_by_label('Readiness path').fill('/readyz');page.get_by_role('button',name='Next').click()
                    page.locator('input[name="exposure"][value="lan"]').check()
                    page.get_by_label('LAN hostname',exact=True).fill(hostname)
                    page.get_by_role('button',name='Next').click();page.get_by_role('button',name='Next').click()
                    accepted=case.click_mutation(page,page.get_by_role('button',name='Deploy',exact=True),'POST','/apps')
                    app=accepted['applicationId'];case.browser_success(page,accepted,navigates=False)
                    page.goto(management+'/apps/'+app);page.reload()
                    expect(page.get_by_role('link',name=app_url,exact=True)).to_have_attribute('href',app_url)
                    case.assertEqual(client.get('/apps/'+app)['access']['url'],app_url)
                    wait_version(hostname,'v1')
                    page.goto(app_url+'version');case.assertEqual(json.loads(page.locator('body').inner_text())['version'],'v1')
                finally:browser.close()

            before=len(client.get('/apps')['items'])
            code,_=client.request('POST','/apps',{'name':'duplicate-'+secrets.token_hex(3),'image':e.image('v1'),
                'containerPort':8080,'profile':'http','exposure':{'mode':'lan','hostname':hostname}}, {'Idempotency-Key':secrets.token_hex(16)})
            case.assertEqual(code,409);case.assertEqual(len(client.get('/apps')['items']),before)
            internal=e.create('private-no-route')
            for identity,exposure in ((internal,{'mode':'lan','hostname':'new.apps.home.arpa'}),
                (app,{'mode':'internal'}),(app,{'mode':'lan','hostname':'renamed.apps.home.arpa'})):
                current=client.get('/apps/'+identity)
                code,_=client.request('PATCH','/apps/'+identity,{'version':current['version'],'exposure':exposure},
                    {'Idempotency-Key':secrets.token_hex(16)})
                case.assertEqual(code,422)
                case.assertEqual(client.get('/apps/'+identity)['version'],current['version'])
            previous=client.get('/apps/'+app)['lastSuccessfulRevision']
            client.mutate('PATCH','/apps/'+app,{'version':client.get('/apps/'+app)['version'],'image':e.image('v2')})
            wait_version(hostname,'v2')
            client.mutate('POST','/apps/'+app+'/rollback',{'version':client.get('/apps/'+app)['version'],'revisionId':previous})
            wait_version(hostname,'v1')
            case.assertEqual(e.resource('ingresses',internal),[])
            case.assertEqual(client.fetch('/version',host='internal.apps.home.arpa')[1],404)
            verify_public_listener_refusal(case)
            with socket.socket(socket.AF_INET6,socket.SOCK_STREAM) as probe:
                probe.settimeout(2)
                case.assertNotEqual(probe.connect_ex(('::1',port)),0,'IPv6 reached the IPv4-only published listener')

            denied={'privateLAN':{**values['privateLAN'],'allowedCIDRs':['10.250.0.1/32']}}
            configure(denied,403)
            for host in ('platform.home.arpa',hostname):
                for headers in ({},{'X-Forwarded-For':peer,'X-Real-IP':peer,'Forwarded':'for='+peer+';proto=https'}):
                    case.assertEqual(client.fetch('/healthz',host=host,headers=headers)[1],403,'Forwarded headers bypassed source restriction')
            configure(values)
            client.request('DELETE','/auth/session')
            e.record('Private HTTPS: CA/hostname checks and pinned-browser LAN deployment/link/HTTP passed; Secure host-only cookies, reserved/duplicate hosts, internal-route absence, IPv6 refusal and forged-header source restrictions verified')
        finally:
            # Only this owned disposable cluster is changed; failures remain failures.
            if configured:
                configure({'privateLAN':{'enabled':False}})
            for owned in (app,internal):
                if owned:
                    current=e.client.get('/apps/'+owned)
                    if not current.get('activeOperationId'):e.delete(owned)
            e.kubectl('-n','platform-system','delete','secret','platform-private-e2e')
