#!/usr/bin/env python3
"""Guarded public-image pull/cache observations; no content deletion or global prune."""
import argparse
import hashlib
import json
from pathlib import Path
import re
import time
import urllib.error
from run import Experiment
from common import write_json, content_state


def graph(host, image):
    digest = image.split('@')[1]
    descriptors = []
    def document(selected, kind):
        raw = host.command([host.k3s, 'ctr', '-n', 'k8s.io', 'content', 'get', selected])
        if len(raw) > 1 << 20:
            raise ValueError('Oversized image metadata')
        descriptors.append({'digest': selected, 'size': len(raw.encode()), 'kind': kind})
        return json.loads(raw)
    value = document(digest, 'index-or-manifest')
    if 'manifests' in value:
        matches = [x for x in value['manifests'] if x.get('platform', {}).get('architecture') == 'arm64'
                   and x.get('platform', {}).get('os') == 'linux']
        if len(matches) != 1:
            raise ValueError('No unique native manifest for pull experiment')
        value = document(matches[0]['digest'], 'manifest')
    descriptors.append({**value['config'], 'kind': 'config'})
    descriptors.extend({**layer, 'kind': 'layer'} for layer in value['layers'])
    return descriptors


def snapshot_chains(host, descriptors):
    config = next(x for x in descriptors if x['kind'] == 'config')
    raw = host.command([host.k3s, 'ctr', '-n', 'k8s.io', 'content', 'get', config['digest']])
    diffs = json.loads(raw)['rootfs']['diff_ids']
    if not diffs or any(not re.fullmatch(r'sha256:[0-9a-f]{64}', d) for d in diffs):
        raise ValueError('Native uncompressed layer identities are missing')
    chain = diffs[0]
    result = [chain]
    for diff in diffs[1:]:
        chain = 'sha256:' + hashlib.sha256((chain + ' ' + diff).encode()).hexdigest()
        result.append(chain)
    return result


def snapshot_inventory(exp):
    # The retained reference demo verifies the actual native runtime snapshotter.
    retained = next(iter(exp.original_apps))
    containers = exp.host.containers(retained)
    info = json.loads(exp.host.command([exp.host.k3s, 'ctr', '-n', 'k8s.io', 'containers', 'info', containers[0]['containerID']]))
    if info.get('Snapshotter') != 'overlayfs':
        raise ValueError('This reference protocol requires the verified native overlayfs snapshotter')
    listing = exp.host.command([exp.host.k3s, 'ctr', '-n', 'k8s.io', 'snapshots', '--snapshotter', 'overlayfs', 'list'])
    lines = listing.splitlines()
    if not lines or lines[0].split()[0] != 'KEY':
        raise ValueError('Unexpected native snapshot inventory format')
    return {line.split()[0] for line in lines[1:] if line.strip()}


def assert_unused(exp, image, selected, descriptors):
    digest = image.split('@')[1]
    image_id = selected['id']
    identities = {x['digest'] for x in descriptors if x['kind'] != 'layer'} | {digest, image_id}
    aliases = set(selected.get('repoTags', []) + selected.get('repoDigests', []))
    pods = exp.host.kube('get', 'pods', '--all-namespaces')['items']
    for pod in pods:
        spec = pod['spec']
        for container in spec.get('containers', []) + spec.get('initContainers', []) + spec.get('ephemeralContainers', []):
            if container['image'] in aliases or any(identity in container['image'] for identity in identities):
                raise ValueError('Selected image is referenced by a live Pod')
        status = pod.get('status', {})
        for container in status.get('containerStatuses', []) + status.get('initContainerStatuses', []):
            if any(identity in container.get('imageID', '') for identity in identities):
                raise ValueError('Selected image config is used by a live container')
    for app in exp.api.get('/apps')['items']:
        detail = exp.api.get('/apps/' + app['id'])
        if detail['activeOperationId']:
            raise ValueError('Active application operation prevents cache reset')
        revisions = exp.api.get('/apps/' + app['id'] + '/revisions')['items']
        # Check every retained revision of each undeleted app, including rollback candidates.
        if any(identity in json.dumps(revisions) for identity in identities | aliases):
            raise ValueError('Selected image is retained by an application revision')


def run(exp, allow_reset):
    if not allow_reset:
        raise ValueError('Unused image-reference reset requires explicit --allow-remove-unused-reference')
    image = exp.plan['publicImage']
    if not re.fullmatch(r'index\.docker\.io/nginxinc/nginx-unprivileged@sha256:[0-9a-f]{64}', image):
        raise ValueError('Only the predeclared immutable public benchmark image is permitted')
    descriptors = graph(exp.host, image)
    chains = snapshot_chains(exp.host, descriptors)
    snapshot_inventory(exp)
    timings = []
    for repetition in range(1, exp.plan['repetitions'] + 1):
        images = json.loads(exp.host.command([exp.host.k3s, 'crictl', 'images', '-o', 'json']))['images']
        matching = [row for row in images if any(image.split('@')[1] in ref for ref in row.get('repoDigests', []))]
        if len(matching) != 1:
            raise ValueError('A unique previously benchmarked native public image is required')
        selected = matching[0]
        assert_unused(exp, image, selected, descriptors)
        write_json(exp.folder / f'cache-before-{repetition}.json', {'selectedImage': selected, 'descriptors': descriptors, 'nativeChainIDs': chains})
        exp.host.command([exp.host.k3s, 'crictl', 'rmi', selected['id']])
        # This removes only the unused selected CRI image. No ctr content rm/prune is used.
        time.sleep(5)
        for condition in ('pull-required', 'cached'):
            present = set(exp.host.command([exp.host.k3s, 'ctr', '-n', 'k8s.io', 'content', 'list', '-q']).splitlines())
            inventory = content_state(descriptors, present, chains, snapshot_inventory(exp))
            write_json(exp.folder / f'content-{repetition}-{condition}.json', inventory)
            start = time.monotonic()
            app, accepted = exp.api_create(f'pull-{repetition}-{condition}', image=image, lan=True, public=True)
            exp.api.poll(accepted['operationId'], budget=exp.plan['operationDeadlineSeconds'])
            pods = exp.host.app_pods(app)
            if len(pods) != 1 or not pods[0]['status']['containerStatuses'][0]['ready']:
                raise ValueError('Public image did not produce one ready native container')
            row = {'repetition': repetition, 'requestedCondition': condition, 'content': inventory,
                   'readySeconds': time.monotonic() - start, 'appID': app,
                   'actualImageID': pods[0]['status']['containerStatuses'][0]['imageID']}
            current = exp.api.get('/apps/' + app)
            deadline = time.monotonic() + 30
            probes = []
            while True:
                try:
                    with exp.api.http.open(current['access']['url'], timeout=3) as response:
                        status, body = response.code, response.read((1 << 20) + 1)
                    healthy = status == 200 and len(body) <= 1 << 20 and b'Welcome to nginx!' in body
                except urllib.error.HTTPError as error:
                    status, healthy = error.code, False
                    error.close()
                probes.append({'seconds': time.monotonic() - start, 'status': status, 'expectedContent': healthy})
                if healthy:
                    break
                if time.monotonic() >= deadline:
                    raise TimeoutError('Public-image HTTPS did not converge to the expected content')
                time.sleep(.2)
            row['endToEndHTTPSSeconds'] = time.monotonic() - start
            row['HTTPSProbes'] = probes
            # Capture startup peaks even when fresh unpacking is absent from steady-state runs.
            row['nativeContainerResources'] = []
            for container in exp.host.containers(app):
                directory = Path(container['cgroup'])
                row['nativeContainerResources'].append({**container,
                    'memoryPeakBytes': int((directory / 'memory.peak').read_text()),
                    'memoryEvents': dict(line.split() for line in (directory / 'memory.events').read_text().splitlines()),
                    'CPU': dict(line.split() for line in (directory / 'cpu.stat').read_text().splitlines())})
            events = exp.host.kube('-n', 'platform-apps', 'get', 'events', '--field-selector', 'involvedObject.uid=' + pods[0]['metadata']['uid'])['items']
            row['pullEvents'] = [{'reason': e.get('reason'), 'firstTimestamp': e.get('firstTimestamp'),
                                  'lastTimestamp': e.get('lastTimestamp'), 'count': e.get('count'), 'message': e.get('message')} for e in events]
            timings.append(row);write_json(exp.folder / 'pull-timings.json', timings)
            exp.api_delete(app)
            print(repetition, condition, inventory['condition'], round(row['readySeconds'], 3), flush=True)
    write_json(exp.folder / 'summary.json', {'runs': timings, 'originalApplicationsRetained': exp.application_identities() == exp.original_apps,
               'humanOperatorTime': None, 'globalPruneUsed': False})


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    for name in ('plan', 'release-manifest', 'state', 'ca', 'chart', 'output'):
        parser.add_argument('--' + name, type=Path, required=True)
    for name in ('management-url', 'address'):
        parser.add_argument('--' + name, required=True)
    parser.add_argument('--kubeconfig', default='/etc/rancher/k3s/k3s.yaml')
    parser.add_argument('--tools', default='/usr/local/lib/k8s-internal-platform')
    parser.add_argument('--allow-remove-unused-reference', action='store_true')
    args = parser.parse_args()
    if not args.allow_remove_unused_reference:
        parser.error('The explicitly scoped reset opt-in is required')
    experiment = Experiment(args)
    try:
        run(experiment, args.allow_remove_unused_reference)
    finally:
        experiment.close()


if __name__ == '__main__':
    main()
