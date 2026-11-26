#!/usr/bin/env python3
"""Validate supplied human trial data without inventing missing observations."""
import argparse
import csv
import math
from pathlib import Path
from common import describe, write_json


def summarize(path=None):
    if path is None:
        return {'status': 'pending', 'trials': 0, 'humanOperatorTimeMeasured': False, 'pairedTrialsComplete': False}
    with Path(path).open(newline='') as file:
        rows = list(csv.DictReader(file))
    if not rows:
        return {'status': 'pending', 'trials': 0, 'humanOperatorTimeMeasured': False, 'pairedTrialsComplete': False}
    groups = {}
    for row in rows:
        if row['method'] not in ('helm', 'ui') or row['success'] not in ('true', 'false') or row['comparable'] not in ('true', 'false'):
            raise ValueError('Unknown human method/success value')
        active = float(row['active_seconds'])
        elapsed = float(row['elapsed_seconds'])
        steps = int(row['operator_steps'])
        if not math.isfinite(active) or not math.isfinite(elapsed) or not 0 <= active <= elapsed or elapsed <= 0 or steps <= 0:
            raise ValueError('Human trial requires finite real active/elapsed time and positive step count')
        key = (row['trial_id'], row['participant_label'], row['release'], row['task'])
        group = groups.setdefault(key, {})
        if row['method'] in group or row['order'] not in ('1', '2'):
            raise ValueError('Duplicate method or invalid human trial order')
        group[row['method']] = {'active': active, 'elapsed': elapsed, 'steps': steps,
                                'success': row['success'] == 'true', 'order': row['order'], 'comparable': row['comparable'] == 'true'}
    paired = all(set(group) == {'helm', 'ui'} and group['helm']['order'] != group['ui']['order'] for group in groups.values())
    comparable = all(row['comparable'] for group in groups.values() for row in group.values())
    return {'status': 'incomparable' if not comparable else 'recorded' if paired else 'incomplete',
            'comparableConditionsAttested': comparable, 'trials': len(groups),
            'humanOperatorTimeMeasured': True, 'pairedTrialsComplete': paired,
            'allTasksSucceeded': all(row['success'] for group in groups.values() for row in group.values()),
            'methods': {method: {metric: describe([group[method][metric] for group in groups.values() if method in group])
                                 for metric in ('active', 'elapsed', 'steps')} for method in ('helm', 'ui')},
            'scope': 'Supplied administrator case study; no population/usability generalization'}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--input', type=Path)
    parser.add_argument('--output', type=Path, required=True)
    args = parser.parse_args()
    result = summarize(args.input)
    write_json(args.output, result)
    print(result['status'])


if __name__ == '__main__':
    main()
