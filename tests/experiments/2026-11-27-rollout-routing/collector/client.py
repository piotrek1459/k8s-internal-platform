import json,sys,time,threading,http.client
from pathlib import Path
sys.path.insert(0,'tests/evaluation')
from load import run
from remote import command,upload
p=Path('.local/rollout-v0111')
if len(sys.argv)>1 and sys.argv[1]=='worker':
    policy=sys.argv[2];job=json.loads(sys.argv[3])
    if policy=='fresh':
        original=http.client.HTTPSConnection.request
        def request(self,method,url,body=None,headers=None,*,encode_chunked=False):
            return original(self,method,url,body=body,headers={**(headers or {}),'Connection':'close'},encode_chunked=encode_chunked)
        http.client.HTTPSConnection.request=request
    result=run(job['url'],p/('client-'+job['jobID'])/policy,Path('.local/pi-acceptance/native-v018/pki/ca.crt'),'192.168.68.227',rate=job['rate'],seconds=job['seconds'],warmup=0,expected_version=('v1','v2'))
    upload(p/('client-'+job['jobID'])/policy/'summary.json','platform-rollout-v0111/exchange/'+job['jobID']+'-'+policy+'-summary.json')
    print(policy,result['successful'],result['requests'],flush=True)
    sys.exit(0)
expected=sys.argv[1] if len(sys.argv)>1 else 'diagnosis-01'
deadline=time.monotonic()+300
while True:
    r=command('cat platform-rollout-v0111/exchange/job.json 2>/dev/null')
    if r.returncode==0:
        job=json.loads(r.stdout)
        if job['jobID']==expected:break
    if time.monotonic()>deadline:raise TimeoutError('No diagnostic job')
    time.sleep(1)
delay=job['startEpochSeconds']-time.time()
if delay>0:time.sleep(delay)
assert time.time()-job['startEpochSeconds']<2
# Separate processes preserve independent connection policies and DNS overrides.
import subprocess
children=[]
for policy in ('pooled','fresh'):
    children.append(subprocess.Popen([sys.executable,str(p/'client.py'),'worker',policy,json.dumps(job)]))
for c in children:assert c.wait()==0
