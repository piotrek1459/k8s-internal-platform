#!/usr/bin/env python3
"""Read-only Linux/cgroup sampling on the explicitly selected native host."""
import argparse
import json
import os
from pathlib import Path
import platform
import re
import shutil
import subprocess
import time

from common import write_json


def counters(path):
    return {key: int(value) for key, value in (line.split() for line in Path(path).read_text().splitlines())}


def memory_info():
    wanted = {'MemTotal', 'MemAvailable', 'SwapTotal', 'SwapFree'}
    return {line.split(':')[0]: int(line.split()[1]) * 1024
            for line in Path('/proc/meminfo').read_text().splitlines() if line.split(':')[0] in wanted}


class Host:
    def __init__(self, kubeconfig='/etc/rancher/k3s/k3s.yaml', tools='/usr/local/lib/k8s-internal-platform'):
        self.kubeconfig = str(kubeconfig)
        self.kubectl = str(Path(tools) / 'kubectl')
        self.helm = str(Path(tools) / 'helm')
        self.k3s = '/usr/local/bin/k3s'

    def command(self, args, timeout=30):
        result = subprocess.run(list(map(str, args)), capture_output=True, text=True, timeout=timeout)
        if result.returncode:
            raise RuntimeError('Evaluation command failed: ' + Path(str(args[0])).name)
        return result.stdout

    def kube(self, *args):
        return json.loads(self.command([self.kubectl, '--kubeconfig', self.kubeconfig, *args, '-o', 'json']))

    def raw(self, path):
        return json.loads(self.command([self.kubectl, '--kubeconfig', self.kubeconfig, 'get', '--raw', path]))

    def app_pods(self, app_id, include_deleting=False):
        pods = self.kube('-n', 'platform-apps', 'get', 'pods', '-l', 'platform.local/app-id=' + app_id)['items']
        return [p for p in pods if include_deleting or not p['metadata'].get('deletionTimestamp')]

    def containers(self, app_id):
        result = []
        for pod in self.app_pods(app_id):
            for status in pod.get('status', {}).get('containerStatuses', []):
                identity = status.get('containerID', '').removeprefix('containerd://')
                if not re.fullmatch(r'[0-9a-f]{64}', identity):
                    raise ValueError('No running container identity for sampling')
                inspected = json.loads(self.command([self.k3s, 'crictl', 'inspect', identity]))
                pid = int(inspected['info']['pid'])
                if pid <= 1:
                    raise ValueError('Invalid native container process identity')
                lines = Path(f'/proc/{pid}/cgroup').read_text().splitlines()
                relative = next(line.split('::', 1)[1] for line in lines if line.startswith('0::'))
                directory = (Path('/sys/fs/cgroup') / relative.lstrip('/')).resolve()
                directory.relative_to(Path('/sys/fs/cgroup'))
                if not (directory / 'memory.peak').is_file():
                    raise ValueError('Evaluation requires readable cgroup v2 lifetime memory peaks')
                result.append({'podUID': pod['metadata']['uid'], 'containerID': identity,
                               'imageID': status['imageID'], 'restartCount': status['restartCount'],
                               'cgroup': str(directory)})
        if not result:
            raise ValueError('No owned application containers found')
        return result

    def facts(self):
        return {'machine': platform.machine(), 'kernel': platform.release(),
                'bootID': Path('/proc/sys/kernel/random/boot_id').read_text().strip(),
                'OS': Path('/etc/os-release').read_text(), 'memory': memory_info(),
                'disk': dict(zip(('total', 'used', 'free'), shutil.disk_usage('/var/lib/rancher/k3s'))),
                'Kubernetes': self.kube('version'),
                'Helm': self.command([self.helm, 'version', '--short']).strip()}


def sample(host, app_id, output, seconds=110, interval=1):
    if platform.system() != 'Linux' or not 1 <= seconds <= 900 or not 0.1 <= interval <= 10:
        raise ValueError('Use Linux and bounded sampling duration/interval')
    output = Path(output)
    if output.exists() and any(output.iterdir()):
        raise ValueError('Refused to overwrite an existing host-sampling attempt')
    output.mkdir(mode=0o700, parents=True, exist_ok=True)
    selected = host.containers(app_id)
    write_json(output / 'identity-before.json', {'appID': app_id, 'containers': selected, 'host': host.facts()})
    started = time.monotonic()
    wall = time.time()
    write_json(output / 'ready.json', {'startedEpochSeconds': wall, 'seconds': seconds, 'interval': interval})
    rows = 0
    missing = 0
    raw = output / 'samples.jsonl'
    with raw.open('w') as file:
        while time.monotonic() - started < seconds:
            stamp = time.time()
            record = {'epochSeconds': stamp, 'elapsedSeconds': round(time.monotonic() - started, 6),
                      'hostMemoryBytes': memory_info(), 'hostCPU': Path('/proc/stat').read_text().splitlines()[0],
                      'freeDiskBytes': shutil.disk_usage('/var/lib/rancher/k3s').free, 'containers': []}
            thermal = Path('/sys/class/thermal/thermal_zone0/temp')
            record['temperatureCelsius'] = int(thermal.read_text()) / 1000 if thermal.is_file() else None
            for item in selected:
                directory = Path(item['cgroup'])
                try:
                    data = {'containerID': item['containerID'], 'podUID': item['podUID'],
                            'memoryCurrentBytes': int((directory / 'memory.current').read_text()),
                            'memoryPeakBytes': int((directory / 'memory.peak').read_text()),
                            'memoryEvents': counters(directory / 'memory.events'),
                            'CPU': counters(directory / 'cpu.stat')}
                except (OSError, ValueError):
                    data = {'containerID': item['containerID'], 'missing': True}
                    missing += 1
                record['containers'].append(data)
            file.write(json.dumps(record) + '\n')
            file.flush()
            rows += 1
            remaining = started + rows * interval - time.monotonic()
            if remaining > 0:
                time.sleep(remaining)
    raw.chmod(0o600)
    after = host.containers(app_id)
    stable = {(x['podUID'], x['containerID']) for x in selected} == {(x['podUID'], x['containerID']) for x in after}
    result = {'appID': app_id, 'startedEpochSeconds': wall, 'finishedEpochSeconds': time.time(),
              'sampleCount': rows, 'missingSamples': missing, 'stableContainerIdentity': stable,
              'containersAfter': after, 'evidenceValid': rows > 0 and missing == 0 and stable}
    write_json(output / 'summary.json', result)
    return result


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--app-id', required=True)
    parser.add_argument('--output', type=Path, required=True)
    parser.add_argument('--seconds', type=int, default=110)
    parser.add_argument('--interval', type=float, default=1)
    parser.add_argument('--kubeconfig', default='/etc/rancher/k3s/k3s.yaml')
    parser.add_argument('--tools', default='/usr/local/lib/k8s-internal-platform')
    args = parser.parse_args()
    result = sample(Host(args.kubeconfig, args.tools), args.app_id, args.output, args.seconds, args.interval)
    print(json.dumps(result, indent=2))
    return 0 if result['evidenceValid'] else 1


if __name__ == '__main__':
    raise SystemExit(main())
