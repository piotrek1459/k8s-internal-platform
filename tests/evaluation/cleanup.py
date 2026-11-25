#!/usr/bin/env python3
"""Delete only fixtures whose run identity was recorded by run.py."""
import argparse
import json
from pathlib import Path
from api import API
from common import require_owned_release, require_application, write_json
from host import Host
import subprocess


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--run', type=Path, required=True)
    parser.add_argument('--state', type=Path, required=True)
    parser.add_argument('--ca', type=Path, required=True)
    parser.add_argument('--management-url', required=True)
    parser.add_argument('--address', required=True)
    parser.add_argument('--kubeconfig', default='/etc/rancher/k3s/k3s.yaml')
    parser.add_argument('--tools', default='/usr/local/lib/k8s-internal-platform')
    args = parser.parse_args()
    marker = args.run / 'owned.json'
    owned = json.loads(marker.read_text())
    environment = json.loads((args.run / 'environment.json').read_text())
    if owned['runID'] != environment['runID']:
        raise ValueError('Run marker and environment differ')
    api = API(args.management_url, args.state, args.ca, args.address, environment['release']['version'])
    host = Host(args.kubeconfig, args.tools)
    try:
        api.login()
        for app, recorded in list(owned['applications'].items()):
            live = api.get('/apps/' + app)
            require_application(owned['runID'], app, recorded, live)
            api.owned[app] = recorded['name']
            accepted = api.mutate(app, 'DELETE')
            api.poll(accepted['operationId'])
            owned['applications'].pop(app)
            write_json(marker, owned)
        for release, recorded in list(owned['releases'].items()):
            live = host.kube('-n', 'platform-apps', 'get', 'secret', recorded['name'])
            require_owned_release(owned['runID'], release, recorded['uid'], live)
            result = subprocess.run([host.helm, '--kubeconfig', host.kubeconfig, 'uninstall', release,
                                     '-n', 'platform-apps', '--wait', '--timeout', '120s'], capture_output=True, timeout=150)
            if result.returncode:
                raise RuntimeError('Owned Helm cleanup failed; state retained')
            owned['releases'].pop(release)
            write_json(marker, owned)
        write_json(args.run / 'cleanup.json', {'ownedFixturesRemaining': owned, 'complete': True})
        print('Recorded evaluation fixtures deleted; application history retained')
    finally:
        api.close()


if __name__ == '__main__':
    main()
