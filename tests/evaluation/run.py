#!/usr/bin/env python3
"""Explicit owned-fixture reference-host lifecycle/calibration experiments."""
import argparse
import base64
import copy
import gzip
import hashlib
import json
import os
from pathlib import Path
import re
import secrets
import subprocess
import time

from api import API
from common import canonical_hash, describe, load_plan, require_owned_release, write_json, lifecycle_acceptance
from host import Host


def decode_release(record):
    return json.loads(gzip.decompress(base64.b64decode(base64.b64decode(record['data']['release']))))


class Experiment:
    def __init__(self, args):
        self.args = args
        self.plan = load_plan(args.plan)
        self.release = json.loads(args.release_manifest.read_text())
        if self.release['version'] != self.plan['referenceRelease']:
            raise ValueError('Reference release and fixed plan differ')
        self.images = self.release['targets']['arm64']['images']
        if args.output.exists():
            raise ValueError('Refused existing experiment directory; retain each attempt separately')
        args.output.mkdir(mode=0o700, parents=True)
        self.folder = args.output
        self.run_id = secrets.token_hex(6)
        self.api = API(args.management_url, args.state, args.ca, args.address, self.release['version'])
        self.host = Host(args.kubeconfig, args.tools)
        self.owned = {'runID': self.run_id, 'applications': {}, 'releases': {}}
        self.timings = []
        self.base_values = None
        self.chart_identity = None
        try:
            self.system = self.api.login()
            deployment = self.host.kube('-n', 'platform-system', 'get', 'deployment', 'platform')
            if deployment['spec']['template']['spec']['containers'][0]['image'] != self.images['platform']:
                raise ValueError('Live platform image differs from the selected published release')
            if deployment['metadata'].get('annotations', {}).get('meta.helm.sh/release-name') != 'platform':
                raise ValueError('Platform installation ownership is not recorded')
            self.original_apps = self.application_identities()
            if any(self.api.get('/apps/' + x['id'])['activeOperationId'] for x in self.api.get('/apps')['items']):
                raise ValueError('An existing operation is active; experiment refused')
            self.environment = {'runID': self.run_id, 'plan': self.plan,
                'planSHA256': canonical_hash(self.plan), 'release': self.release,
                'platformDeploymentUID': deployment['metadata']['uid'], 'host': self.host.facts(),
                'originalApplications': self.original_apps, 'humanOperatorTime': None,
                'chartInputSHA256': self.chart_files()}
            write_json(self.folder / 'environment.json', self.environment)
            self.save_owned()
        except Exception:
            self.api.close()
            raise

    def application_identities(self):
        details = [self.api.get('/apps/' + row['id']) for row in self.api.get('/apps')['items']]
        return {row['id']: (row['version'], row['desiredRevision']) for row in details}

    def chart_files(self):
        root = self.args.chart.resolve()
        if not (root / 'Chart.yaml').is_file():
            raise ValueError('An explicit first-party chart directory is required')
        values = {}
        for path in sorted(root.rglob('*')):
            if path.is_symlink():
                raise ValueError('Chart inputs may not contain symbolic links')
            if path.is_file():
                values[str(path.relative_to(root))] = hashlib.sha256(path.read_bytes()).hexdigest()
        return values

    def save_owned(self):
        write_json(self.folder / 'owned.json', self.owned)

    def spec(self, suffix, image=None, lan=False, public=False, replicas=1):
        domain = self.api.application_domain
        if lan and not domain:
            raise ValueError('Load calibration requires configured private HTTPS')
        return {'name': f'eval-{self.run_id}-{suffix}', 'image': image or self.images['v1'],
            'containerPort': 8080, 'replicas': replicas, 'profile': 'http',
            'resources': self.plan['resourceCandidate'],
            'health': {'startupPath': '/' if public else '/healthz', 'readinessPath': '/' if public else '/readyz'},
            'exposure': {'mode': 'lan', 'hostname': f'eval-{self.run_id}-{suffix}.{domain}'} if lan else {'mode': 'internal'}}

    def api_create(self, suffix, image=None, lan=False, public=False, replicas=1):
        spec = self.spec(suffix, image, lan, public, replicas)
        accepted = self.api.create(spec)
        app = accepted['applicationId']
        self.owned['applications'][app] = {'name': spec['name'], 'operation': accepted['operationId']}
        self.save_owned()
        return app, accepted

    def release_record(self, release):
        rows = self.host.kube('-n', 'platform-apps', 'get', 'secrets', '-l', 'owner=helm,name=' + release)['items']
        if not rows:
            raise ValueError('No recorded Helm release evidence')
        return max(rows, key=lambda r: int(r['metadata']['labels']['version']))

    def record_timing(self, method, repetition, stage, started, **evidence):
        item = {'method': method, 'repetition': repetition, 'stage': stage,
                'seconds': round(time.monotonic() - started, 6), **evidence}
        self.timings.append(item)
        write_json(self.folder / 'timings.json', self.timings)
        print(f"{method} repetition {repetition} {stage}: {item['seconds']} s", flush=True)

    def http(self, app_id, expected, path='/version'):
        deadline = time.monotonic() + 60
        while time.monotonic() < deadline:
            services = self.host.kube('-n', 'platform-apps', 'get', 'services', '-l', 'platform.local/app-id=' + app_id)['items']
            if len(services) != 1:
                raise ValueError('No unique benchmark Service identity')
            route = '/api/v1/namespaces/platform-apps/services/' + services[0]['metadata']['name'] + ':80/proxy' + path
            try:
                value = self.host.raw(route)
                if value.get('version') == expected:
                    return value
            except (RuntimeError, json.JSONDecodeError):
                pass
            time.sleep(.25)
        raise TimeoutError('Actual benchmark Service HTTP did not converge')

    def prepare(self):
        inventory = self.host.command([self.host.k3s, "ctr", "-n", "k8s.io", "images", "list", "-q"]).splitlines()
        if any(self.images[v] not in inventory for v in ("v1", "v2", "v3")):
            raise ValueError("Exact bundled demo digests must already be cached")
        write_json(self.folder / "image-inventory.json", {"images": inventory, "cache": "cached"})
        app, accepted = self.api_create('prepare')
        self.api.poll(accepted['operationId'], budget=self.plan['operationDeadlineSeconds'])
        self.http(app, 'v1')
        release = decode_release(self.release_record('app-' + app.replace('_', '-')))
        self.base_values = release['config']
        self.chart_identity = release['chart']
        write_json(self.folder / 'reference-values.json', self.base_values)
        write_json(self.folder / 'reference-chart-identity.json', self.chart_identity)
        self.api_delete(app)

    def api_delete(self, app):
        accepted = self.api.mutate(app, 'DELETE')
        self.api.poll(accepted['operationId'], budget=self.plan['operationDeadlineSeconds'])
        self.owned['applications'].pop(app)
        self.save_owned()
        end = time.monotonic() + 60
        while self.host.app_pods(app) and time.monotonic() < end:
            time.sleep(.5)
        if self.host.app_pods(app):
            raise ValueError('Deleted benchmark Pods remain after the cleanup budget')

    def api_trial(self, repetition, failure=True):
        started = time.monotonic()
        app, accepted = self.api_create(f'api-{repetition}')
        operation, states = self.api.poll(accepted['operationId'], budget=self.plan['operationDeadlineSeconds'])
        self.http(app, 'v1')
        self.record_timing('api', repetition, 'deploy-v1', started, app=app, operation=operation['id'], states=states, cache='cached')
        record = decode_release(self.release_record('app-' + app.replace('_', '-')))
        if record['chart'] != self.chart_identity:
            raise ValueError('API chart identity changed during the comparison')
        started = time.monotonic()
        accepted = self.api.mutate(app, 'PATCH', image=self.images['v2'])
        self.api.poll(accepted['operationId'], budget=self.plan['operationDeadlineSeconds'])
        self.http(app, 'v2')
        self.record_timing('api', repetition, 'update-v2', started, operation=accepted['operationId'], cache='cached')
        if failure:
            started = time.monotonic()
            accepted = self.api.mutate(app, 'PATCH', image=self.images['v3'])
            result, states = self.api.poll(accepted['operationId'], expected='failed', budget=self.plan['operationDeadlineSeconds'])
            if result['stage'] != 'restored' or self.api.get('/apps/' + app)['activeOperationId']:
                raise ValueError('Platform failure did not verify restoration before unlocking')
            self.http(app, 'v2')
            self.record_timing('api', repetition, 'unhealthy-v3-and-restoration', started, operation=result['id'], states=states,
                failurePolicySeconds=self.plan['helmFailureTimeoutSeconds'], humanIntervention=False)
        started = time.monotonic()
        self.api_delete(app)
        self.record_timing('api', repetition, 'delete', started)

    def helm(self, *args, timeout=420):
        result = subprocess.run([self.host.helm, '--kubeconfig', self.host.kubeconfig, *map(str, args)],
                                capture_output=True, text=True, timeout=timeout)
        return result

    def manual_values(self, release, version):
        values = copy.deepcopy(self.base_values)
        values['appId'] = release
        values['image']['repository'], values['image']['digest'] = self.images[version].split('@')
        path = self.folder / (release + '-' + version + '.json')
        write_json(path, values)
        return path

    def remember_release(self, release):
        record = self.release_record(release)
        self.owned['releases'][release] = {'uid': record['metadata']['uid'], 'name': record['metadata']['name']}
        self.save_owned()
        return decode_release(record)

    def helm_delete(self, release):
        saved = self.owned['releases'][release]
        live = self.host.kube('-n', 'platform-apps', 'get', 'secret', saved['name'])
        require_owned_release(self.run_id, release, saved['uid'], live)
        result = self.helm('uninstall', release, '-n', 'platform-apps', '--wait', '--timeout', '120s')
        if result.returncode:
            raise RuntimeError('Owned manual release cleanup failed')
        self.owned['releases'].pop(release)
        self.save_owned()

    def helm_trial(self, repetition, failure=True):
        release = f'eval-{self.run_id}-helm-{repetition}'
        existing = self.host.kube('-n', 'platform-apps', 'get', 'secrets', '-l', 'owner=helm,name=' + release)['items']
        if existing:
            raise ValueError('Refused an existing manual release')
        started = time.monotonic()
        result = self.helm('install', release, self.args.chart, '-n', 'platform-apps', '-f', self.manual_values(release, 'v1'), '--wait', '--timeout', '300s')
        actual = self.remember_release(release)
        if result.returncode:
            raise RuntimeError('Manual Helm deployment failed; inspect owned release state')
        if actual['config'] != json.loads(self.manual_values(release, 'v1').read_text()):
            raise ValueError('Manual stored configuration differs from the matched values')
        if actual['chart'] != self.chart_identity:
            raise ValueError('Manual and platform stored charts differ; comparison invalid')
        self.http(release, 'v1')
        self.record_timing('helm', repetition, 'deploy-v1', started, release=release, chartIdentityEqual=True, cache='cached')
        started = time.monotonic()
        result = self.helm('upgrade', release, self.args.chart, '-n', 'platform-apps', '-f', self.manual_values(release, 'v2'), '--wait', '--timeout', '300s')
        if result.returncode:
            raise RuntimeError('Manual Helm v2 update failed')
        self.http(release, 'v2')
        self.record_timing('helm', repetition, 'update-v2', started, cache='cached')
        if failure:
            started = time.monotonic()
            result = self.helm('upgrade', release, self.args.chart, '-n', 'platform-apps', '-f', self.manual_values(release, 'v3'), '--wait', '--timeout', str(self.plan['helmFailureTimeoutSeconds']) + 's')
            if result.returncode == 0:
                raise ValueError('Unhealthy manual update unexpectedly succeeded')
            failed = decode_release(self.release_record(release))
            if failed['info']['status'] != 'failed':
                raise ValueError('Manual failure evidence is not a failed release')
            detected = time.monotonic() - started
            restored = self.helm('rollback', release, '2', '-n', 'platform-apps', '--wait', '--timeout', '120s')
            if restored.returncode:
                raise RuntimeError('Manual predecessor rollback failed')
            self.http(release, 'v2')
            self.record_timing('helm', repetition, 'unhealthy-v3-and-restoration', started,
                detectionSeconds=round(detected, 6), failurePolicySeconds=self.plan['helmFailureTimeoutSeconds'],
                humanIntervention='scripted manual rollback; human active time not measured')
        started = time.monotonic()
        self.helm_delete(release)
        self.record_timing('helm', repetition, 'delete', started)

    def lifecycle(self, include_failure=True):
        self.prepare()
        for number, methods in enumerate(self.plan['methodOrder'], 1):
            for method in methods:
                (self.api_trial if method == 'api' else self.helm_trial)(number, include_failure)
        after = self.application_identities()
        if after != self.original_apps:
            raise ValueError('Non-evaluation application identity/specification changed during the run')
        results = {}
        for method in ('api', 'helm'):
            results[method] = {stage: describe([r['seconds'] for r in self.timings if r['method'] == method and r['stage'] == stage])
                for stage in sorted({r['stage'] for r in self.timings})}
        acceptance = lifecycle_acceptance(self.timings, self.plan, include_failure)
        write_json(self.folder / 'summary.json', {'acceptance': acceptance, 'runID': self.run_id, 'planSHA256': canonical_hash(self.plan),
            'methods': results, 'failureScenariosIncluded': include_failure, 'humanOperatorTime': None,
            'humanTrialStatus': 'pending', 'coreEvaluationComplete': False, 'ownedFixturesRemaining': self.owned,
            'originalApplicationsRetained': True})
        return acceptance['passed']

    def close(self):
        self.api.close()


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('mode', choices=('lifecycle', 'healthy', 'load-fixture'))
    parser.add_argument('--plan', type=Path, required=True)
    parser.add_argument('--release-manifest', type=Path, required=True)
    parser.add_argument('--state', type=Path, required=True)
    parser.add_argument('--ca', type=Path, required=True)
    parser.add_argument('--management-url', required=True)
    parser.add_argument('--address', required=True)
    parser.add_argument('--chart', type=Path, required=True)
    parser.add_argument('--output', type=Path, required=True)
    parser.add_argument('--public-image', action='store_true')
    parser.add_argument('--replicas', type=int, choices=(1, 2), default=1)
    parser.add_argument('--kubeconfig', default='/etc/rancher/k3s/k3s.yaml')
    parser.add_argument('--tools', default='/usr/local/lib/k8s-internal-platform')
    args = parser.parse_args()
    experiment = Experiment(args)
    try:
        if args.mode == 'load-fixture':
            image = experiment.plan['publicImage'] if args.public_image else experiment.images['v1']
            app, accepted = experiment.api_create('load', image=image, lan=True, public=args.public_image, replicas=args.replicas)
            experiment.api.poll(accepted['operationId'], budget=experiment.plan['operationDeadlineSeconds'])
            current = experiment.api.get('/apps/' + app)
            write_json(experiment.folder / 'load-fixture.json', {'appID': app, 'access': current['access'], 'publicImage': args.public_image,
                'replicas': args.replicas, 'image': image, 'ownedName': current['name'], 'runID': experiment.run_id})
            print(json.dumps({'appID': app, 'access': current['access'], 'runID': experiment.run_id}), flush=True)
        else:
            if not experiment.lifecycle(args.mode == 'lifecycle'):
                raise SystemExit(1)
    finally:
        experiment.close()


if __name__ == '__main__':
    main()
