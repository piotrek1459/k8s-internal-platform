"""Credential-safe management client for explicit native evaluation fixtures."""
import http.cookiejar
import json
from pathlib import Path
import secrets
import re
import socket
import ssl
import time
import urllib.error
import urllib.parse
import urllib.request


class API:
    def __init__(self, url, state, ca, address, expected_version):
        parsed = urllib.parse.urlsplit(url)
        if parsed.scheme != 'https' or not parsed.hostname or parsed.username or parsed.password or parsed.query or parsed.fragment:
            raise ValueError('Use a verified HTTPS management URL without embedded credentials')
        self.url = url.rstrip('/')
        self.password = json.loads(Path(state).read_text())['password']
        self.context = ssl.create_default_context(cafile=str(ca))
        self.cookies = http.cookiejar.CookieJar()
        self.http = urllib.request.build_opener(urllib.request.ProxyHandler({}), urllib.request.HTTPSHandler(context=self.context), urllib.request.HTTPCookieProcessor(self.cookies))
        self.expected_version = expected_version
        self.owned = {}
        self.original_dns = socket.getaddrinfo
        self.management_host = parsed.hostname
        self.application_domain = None
        self.address = address
        socket.getaddrinfo = self.resolve

    def resolve(self, host, port, *args, **kwargs):
        selected = host == self.management_host or (isinstance(host, str) and self.application_domain and host.endswith('.' + self.application_domain))
        return self.original_dns(self.address if selected else host, port, *args, **kwargs)

    def request(self, method, path, body=None, key=None):
        headers = {'Content-Type': 'application/json', 'X-Requested-With': 'XMLHttpRequest'}
        if key:
            headers['Idempotency-Key'] = key
        req = urllib.request.Request(self.url + '/api/v1' + path, method=method,
            data=None if body is None else json.dumps(body).encode(), headers=headers)
        try:
            response = self.http.open(req, timeout=30)
        except urllib.error.HTTPError as error:
            response = error
        with response:
            raw = response.read((1 << 20) + 1)
            if len(raw) > 1 << 20:
                raise RuntimeError("Management response exceeded the observation bound")
            try:
                result = json.loads(raw) if raw else None
            except json.JSONDecodeError:
                raise RuntimeError('Management response was not JSON') from None
            return response.code, result

    def get(self, path):
        code, body = self.request('GET', path)
        if code != 200:
            raise RuntimeError(f'Management GET failed ({code}) at {path}')
        return body

    def login(self):
        code, _ = self.request('POST', '/auth/session', {'password': self.password})
        if code != 204:
            raise RuntimeError(f'Evaluation login failed ({code}); credentials omitted')
        system = self.get('/system')
        if system['version'] != self.expected_version or system['architecture'] != 'arm64' or not system['clusterReachable']:
            raise ValueError('Installed release/architecture/cluster differs from the reference plan')
        self.application_domain = (system.get('privateNetwork') or {}).get('applicationDomain')
        return system

    def poll(self, operation, expected='succeeded', budget=390):
        start = time.monotonic()
        states = []
        while time.monotonic() - start < budget:
            value = self.get('/operations/' + operation)
            if not states or states[-1]['state'] != value['state']:
                states.append({'seconds': round(time.monotonic() - start, 6), 'state': value['state']})
            if value['state'] in ('succeeded', 'failed', 'needs_attention'):
                if value['state'] != expected:
                    raise RuntimeError('Evaluation operation reached ' + value['state'])
                return value, states
            time.sleep(.25)
        raise TimeoutError('Evaluation operation exceeded its declared budget')

    def create(self, spec):
        if not re.fullmatch(r"eval-[0-9a-f]{12}-[a-z0-9-]+", spec.get("name", "")):
            raise ValueError("A unique evaluation fixture name is required")
        code, body = self.request('POST', '/apps', spec, secrets.token_hex(16))
        if code != 202:
            raise RuntimeError(f'Evaluation application was not accepted ({code})')
        self.owned[body['applicationId']] = spec['name']
        return body

    def mutate(self, app, method, suffix='', **fields):
        current = self.get('/apps/' + app)
        if not current['name'].startswith('eval-') or self.owned.get(app) != current['name']:
            raise ValueError('Refused mutation of a non-evaluation application')
        code, body = self.request(method, '/apps/' + app + suffix, {'version': current['version'], **fields}, secrets.token_hex(16))
        if code != 202:
            raise RuntimeError(f'Evaluation mutation was not accepted ({code})')
        return body

    def close(self):
        try:
            self.request('DELETE', '/auth/session')
        finally:
            self.password = ''
            self.cookies.clear()
            socket.getaddrinfo = self.original_dns
