"""Private listener validation and existing TLS Secret preflight, without root."""
import base64
import json
import os
from pathlib import Path
import subprocess
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[2]
INSTALLER = ROOT / 'installer/install.sh'


def config():
    return {'address':'192.168.68.227','httpsPort':8443,'allowedCIDRs':['192.168.68.0/24'],
        'managementHostname':'platform.home.arpa','applicationDomain':'apps.home.arpa','tlsSecretName':'platform-private-tls'}


class PrivateLANTests(unittest.TestCase):
    def convert(self, values):
        with tempfile.TemporaryDirectory() as directory:
            path=Path(directory)/'lan.json';path.write_text(json.dumps(values))
            return subprocess.run(['bash','-c','source "$1"; lan_values "$2"','bash',str(INSTALLER),str(path)],capture_output=True,text=True)

    def test_valid_configuration_is_data_not_shell(self):
        result=self.convert(config())
        self.assertEqual(result.returncode,0,result.stderr)
        self.assertEqual(json.loads(result.stdout),{'privateLAN':{'enabled':True,**config()}})

    def test_listener_address_must_exist_on_the_host(self):
        with tempfile.TemporaryDirectory() as directory:
            folder=Path(directory);path=folder/'config.json';tool=folder/'ip'
            tool.write_text('#!/bin/sh\nprintf \'%s\' \'[{"addr_info":[{"local":"192.168.68.227"}]}]\'\n');tool.chmod(0o755)
            for address,expected in (('192.168.68.227',0),('192.168.68.228',1)):
                path.write_text(json.dumps({**config(),'address':address}))
                result=subprocess.run(['bash','-c','source "$1"; LAN_CONFIG="$2"; check_lan_address','bash',str(INSTALLER),str(path)],
                    env={**os.environ,'PATH':str(folder)+os.pathsep+os.environ['PATH']},capture_output=True,text=True)
                self.assertEqual(result.returncode,expected,result.stderr)

    def test_unrestricted_public_invalid_and_overlapping_inputs_are_refused(self):
        changes=[{'address':value} for value in ('0.0.0.0','127.0.0.1','::','192.168.1.999','203.0.113.2','$(touch /tmp/unwanted)')]
        changes += [{'allowedCIDRs':value} for value in ([],['0.0.0.0/0'],['::/0'],['192.168.0.0/8'],['192.168.68.1/24'],['192.168.68.0/24']*2)]
        changes += [{'httpsPort':value} for value in (True,0,443,65536,'8443')]
        changes += [{'managementHostname':value} for value in ('apps.home.arpa','platform.apps.home.arpa','UPPER.home.arpa','platform..home.arpa','platform.home.arpa:8443')]
        changes += [{'applicationDomain':'* .home.arpa'},{'tlsSecretName':'../other'},{'extra':'unrecognized'}]
        for change in changes:
            with self.subTest(change=change):
                result=self.convert({**config(),**change})
                self.assertNotEqual(result.returncode,0)
                self.assertEqual(result.stdout,'')


class LANCertificateTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.temporary=tempfile.TemporaryDirectory();cls.folder=Path(cls.temporary.name)
        for name,sans in (('valid','DNS:platform.home.arpa,DNS:*.apps.home.arpa'),
            ('wrong-host','DNS:other.home.arpa,DNS:*.apps.home.arpa'),
            ('wrong-wildcard','DNS:platform.home.arpa,DNS:*.apps.home.arpa.evil')):
            subprocess.run(['openssl','req','-x509','-newkey','rsa:2048','-nodes','-days','2',
                '-keyout',str(cls.folder/(name+'.key')),'-out',str(cls.folder/(name+'.crt')),
                '-subj','/CN=controlled-test','-addext','subjectAltName='+sans],check=True,capture_output=True)

    @classmethod
    def tearDownClass(cls): cls.temporary.cleanup()

    def check(self, certificate='valid', key=None, secret_type='kubernetes.io/tls'):
        with tempfile.TemporaryDirectory() as directory:
            work=Path(directory);tool=work/'kubectl'
            tool.write_text('#!/bin/sh\ncat "$TLS_FIXTURE"\n');tool.chmod(0o755)
            secret={'type':secret_type,'data':{field:base64.b64encode((self.folder/(name+suffix)).read_bytes()).decode()
                for field,name,suffix in [('tls.crt',certificate,'.crt'),('tls.key',key or certificate,'.key')]}}
            fixture=work/'fixture.json';fixture.write_text(json.dumps(secret))
            (work/'lan-values.json').write_text(json.dumps({'privateLAN':{'enabled':True,**config()}}))
            command='source "$1"; WORK="$2"; TOOL_ROOT="$2"; check_lan_certificate'
            return subprocess.run(['bash','-c',command,'bash',str(INSTALLER),str(work)],
                env={**os.environ,'TLS_FIXTURE':str(fixture)},capture_output=True,text=True)

    def test_existing_matching_certificate_is_accepted(self):
        result=self.check();self.assertEqual(result.returncode,0,result.stderr)

    def test_wrong_hostname_wildcard_key_and_secret_type_are_refused(self):
        for changes in ({'certificate':'wrong-host'},{'certificate':'wrong-wildcard'},{'key':'wrong-host'},{'secret_type':'Opaque'}):
            with self.subTest(changes=changes):
                result=self.check(**changes);self.assertNotEqual(result.returncode,0)
                self.assertNotIn('BEGIN PRIVATE KEY',result.stdout+result.stderr)
