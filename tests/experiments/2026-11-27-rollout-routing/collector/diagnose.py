"""Owned native fixture and temporary, identity-guarded proxy observations."""
import json, os, secrets, subprocess, sys, threading, time, urllib.request
from pathlib import Path
sys.path.insert(0, 'tools')
from api import API
from host import Host

root = Path.cwd(); host = Host(); before = json.loads((root/'before.json').read_text())
images = json.loads((root/'release.json').read_text())['targets']['arm64']['images']
api = API('https://platform.home.arpa:8443', root/'access.json', root/'ca.crt', '192.168.68.227', '0.1.11')
case=sys.argv[1] if len(sys.argv)>1 else 'diagnosis-01'
policy=sys.argv[2] if len(sys.argv)>2 else 'control'
delay=int(sys.argv[3]) if len(sys.argv)>3 else 0
folder = root/case; folder.mkdir(mode=0o700)
(folder/'collector-source.py').write_bytes(Path(__file__).read_bytes())
exchange = root/'exchange'; exchange.mkdir(mode=0o700, exist_ok=True); os.chown(exchange, 1000, 1000)
def write(path, value): path.write_text(json.dumps(value, indent=2)+'\n')
def proxy(): return host.kube('-n','platform-system','get','deployment','platform-private')
def patch_template(expected, replacement):
    live = proxy()
    assert live['metadata']['uid'] == before['privateUID'] and live['spec']['template'] == expected
    patch = [{'op':'test','path':'/metadata/uid','value':live['metadata']['uid']},
             {'op':'test','path':'/metadata/resourceVersion','value':live['metadata']['resourceVersion']},
             {'op':'replace','path':'/spec/template','value':replacement}]
    host.command([host.kubectl,'--kubeconfig',host.kubeconfig,'-n','platform-system','patch','deployment','platform-private','--type=json','-p',json.dumps(patch)])
    host.command([host.kubectl,'--kubeconfig',host.kubeconfig,'-n','platform-system','rollout','status','deployment/platform-private','--timeout=120s'],timeout=130)
    deadline=time.monotonic()+90
    while True:
        try:
            with api.http.open('https://platform.home.arpa:8443/readyz',timeout=3) as r:assert json.load(r)['status']=='ready'
            break
        except Exception:
            if time.monotonic()>deadline:raise
            time.sleep(.2)
    return proxy()['spec']['template']

processes=[]; threads=[]; stop=threading.Event(); app=None; changed=None
def watch(kind, selector):
    p=subprocess.Popen([host.kubectl,'--kubeconfig',host.kubeconfig,'-n','platform-apps','get',kind,'-l',selector,'--watch','--output-watch-events','-o','json'],stdout=subprocess.PIPE,stderr=subprocess.PIPE,text=True)
    processes.append(p)
    def read():
        decoder=json.JSONDecoder();buffer=''
        with (folder/(kind+'.jsonl')).open('w') as out:
            while True:
                char=p.stdout.read(1)
                if not char:break
                buffer+=char
                try:event,end=decoder.raw_decode(buffer.lstrip())
                except json.JSONDecodeError:continue
                buffer=buffer.lstrip()[end:];obj=event['object'];m=obj['metadata']
                record={'epochSeconds':time.time(),'type':event['type'],'name':m['name'],'uid':m['uid'],'resourceVersion':m['resourceVersion'],'deletionTimestamp':m.get('deletionTimestamp')}
                if kind=='pods':record.update(status=obj.get('status'),lifecycle=obj['spec']['containers'][0].get('lifecycle'))
                else:record.update(endpoints=obj['endpoints'],ports=obj['ports'])
                out.write(json.dumps(record)+'\n');out.flush()
    t=threading.Thread(target=read,daemon=True);t.start();threads.append(t)
def logs(pod):
    p=subprocess.Popen([host.kubectl,'--kubeconfig',host.kubeconfig,'-n','platform-apps','logs','-f',pod],stdout=(folder/'old-process.jsonl').open('w'),stderr=subprocess.DEVNULL)
    processes.append(p)
def config():
    with (folder/'proxy-services.jsonl').open('w') as out:
        while not stop.wait(.1):
            try:
                with urllib.request.urlopen('http://127.0.0.1:19080/api/http/services',timeout=2) as r:services=json.load(r)
                selected=[s for s in services if ('app-app-'+app.removeprefix('app_')) in s['name']]
                out.write(json.dumps({'epochSeconds':time.time(),'services':selected})+'\n');out.flush()
            except Exception as e:out.write(json.dumps({'epochSeconds':time.time(),'error':type(e).__name__})+'\n');out.flush()
try:
    api.login();original={x['id']:api.get('/apps/'+x['id'])['version'] for x in api.get('/apps')['items']}
    initial=proxy()['spec']['template'];assert initial==before['privateTemplate']
    template=json.loads(json.dumps(initial));args=template['spec']['containers'][0]['args']
    assert all(not a.startswith('--api.insecure') for a in args)
    args+=['--api.insecure=true','--entrypoints.traefik.address=127.0.0.1:19080',
           '--accesslog=true','--accesslog.format=json','--accesslog.fields.defaultmode=drop',
           '--accesslog.fields.headers.defaultmode=drop',
           *['--accesslog.fields.names.'+name+'=keep' for name in ('StartUTC','RequestHost','ServiceAddr','ServiceName','DownstreamStatus','OriginStatus','Duration')]]
    if policy=='native':args.append('--providers.kubernetesingress.nativelbbydefault=true')
    if delay:args.append('--providers.kubernetesingress.throttleduration='+str(delay)+'s')
    changed=patch_template(initial,template)
    log=subprocess.Popen([host.kubectl,'--kubeconfig',host.kubeconfig,'-n','platform-system','logs','-f','deployment/platform-private'],stdout=(folder/'proxy-access.jsonl').open('w'),stderr=subprocess.DEVNULL)
    processes.append(log)
    write(folder/'proxy-control.json',{'uid':before['privateUID'],'initialTemplate':initial,'observedTemplate':changed})
    nonce=secrets.token_hex(6);name='eval-'+nonce+'-diagnose';hostname=name+'.apps.home.arpa'
    spec={'name':name,'image':images['v1'],'containerPort':8080,'replicas':1,'profile':'http',
          'health':{'startupPath':'/healthz','readinessPath':'/readyz'},'exposure':{'mode':'lan','hostname':hostname}}
    accepted=api.create(spec);app=accepted['applicationId'];write(folder/'owned.json',{'id':app,'name':name})
    api.poll(accepted['operationId'])
    url=api.get('/apps/'+app)['access']['url']+'version'
    deadline=time.monotonic()+60
    while True:
        try:
            with api.http.open(url,timeout=3) as r:assert json.load(r)['version']=='v1'
            break
        except Exception:
            if time.monotonic()>deadline:raise
            time.sleep(.2)
    service=host.kube('-n','platform-apps','get','services','-l','platform.local/app-id='+app)['items'][0]
    pods=host.app_pods(app);assert len(pods)==1
    write(folder/'fixture.json',{'appID':app,'service':service['metadata']['name'],'serviceUID':service['metadata']['uid'],'serviceSpec':service['spec'],'oldPod':pods[0]['metadata']['name'],'url':url,'oldPodUID':pods[0]['metadata']['uid'],'oldPodIP':pods[0]['status']['podIP']})
    watch('pods','platform.local/app-id='+app)
    watch('endpointslices','kubernetes.io/service-name='+service['metadata']['name']);logs(pods[0]['metadata']['name'])
    thread=threading.Thread(target=config,daemon=True);thread.start();threads.append(thread)
    start=time.time()+12;job={'jobID':case,'url':url,'startEpochSeconds':start,'seconds':55,'rate':5}
    write(exchange/'job.json',job);os.chown(exchange/'job.json',1000,1000)
    time.sleep(max(0,start+10-time.time()));submitted=time.time()
    accepted=api.mutate(app,'PATCH',image=images['v2']);api.poll(accepted['operationId'])
    timeline={'updateSubmitted':submitted,'updateReady':time.time(),'operationID':accepted['operationId'],'steps':[]}
    write(folder/'timeline.json',timeline)
    if len(sys.argv)>4 and sys.argv[4]=='cycle':
        first=api.get('/apps/'+app+'/revisions')['items'][-1]['id']
        for offset,action in ((25,'rollback'),(40,'restart')):
            time.sleep(max(0,start+offset-time.time()));entry={'action':action,'submitted':time.time()}
            if action=='rollback':accepted=api.mutate(app,'POST','/rollback',revisionId=first)
            else:accepted=api.mutate(app,'POST','/restart')
            api.poll(accepted['operationId']);entry.update(ready=time.time(),operationID=accepted['operationId'])
            timeline['steps'].append(entry);write(folder/'timeline.json',timeline)
    time.sleep(max(0,start+60-time.time()))
    for name in ('pooled','fresh'):
        receipt=exchange/(case+'-'+name+'-summary.json');assert receipt.exists(),'Missing separate-client trace'
        write(folder/(name+'-summary.json'),json.loads(receipt.read_text()))
    print('DIAGNOSIS_COMPLETE',flush=True)
finally:
    stop.set()
    for p in processes:
        if p.poll() is None:p.terminate()
    for p in processes:
        try:p.wait(timeout=5)
        except subprocess.TimeoutExpired:p.kill();p.wait()
    for t in threads:t.join(timeout=3)
    if app:
        current=api.get('/apps/'+app)
        if current['activeOperationId']:raise RuntimeError('Owned fixture locked; explicit recovery required')
        accepted=api.mutate(app,'DELETE');api.poll(accepted['operationId'])
        end=time.monotonic()+60
        while host.app_pods(app,include_deleting=True) and time.monotonic()<end:time.sleep(.5)
        assert not host.app_pods(app,include_deleting=True)
    if changed:patch_template(changed,initial)
    after={x['id']:api.get('/apps/'+x['id'])['version'] for x in api.get('/apps')['items']}
    assert after==original
    write(root/(case+'-cleanup.json'),{'privateTemplateRestored':proxy()['spec']['template']==before['privateTemplate'],'originalApplicationsUnchanged':True,'ownedFixtureDeleted':bool(app)})
    api.close()
