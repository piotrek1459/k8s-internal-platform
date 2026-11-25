"""Evidence handling and acceptance rules for explicit reference-host experiments."""
import hashlib
import json
import math
import os
from pathlib import Path
import re


def canonical_hash(value):
    return hashlib.sha256(json.dumps(value, sort_keys=True, separators=(',', ':')).encode()).hexdigest()


def load_plan(path):
    value = json.loads(Path(path).read_text())
    if value.get('schemaVersion') != 1 or not re.fullmatch(r'\d+\.\d+\.\d+', value.get('referenceRelease', '')):
        raise ValueError('Unsupported evaluation plan or reference release')
    for key in ('repetitions', 'sampleIntervalSeconds', 'idleSeconds', 'loadedSeconds', 'warmupSeconds',
                'requestRatePerSecond', 'requestTimeoutSeconds', 'healthyDeadlineSeconds',
                'helmFailureTimeoutSeconds', 'operationDeadlineSeconds'):
        if type(value.get(key)) is not int or value[key] < 1:
            raise ValueError('Plan requires a positive integer: ' + key)
    if len(value.get('methodOrder', [])) != value['repetitions'] or any(sorted(row) != ['api', 'helm'] for row in value['methodOrder']):
        raise ValueError('Each repetition must compare both methods exactly once')
    if value['operationDeadlineSeconds'] <= value['helmFailureTimeoutSeconds']:
        raise ValueError('Operation budget must include failure and restoration')
    if value.get('cacheConditions', {}).get('globalPruneAllowed') is not False:
        raise ValueError('Evaluation must prohibit shared-store pruning')
    return value


def percentile(values, percent):
    """Nearest-rank percentile; retain n/range because three trials are descriptive."""
    if not values or not 0 < percent <= 100:
        raise ValueError('A nonempty sample and percentile in (0, 100] are required')
    if any(isinstance(v, bool) or not isinstance(v, (int, float)) or not math.isfinite(v) or v < 0 for v in values):
        raise ValueError('Measurements must be finite nonnegative numbers')
    ordered = sorted(values)
    return ordered[max(0, math.ceil(len(ordered) * percent / 100) - 1)]


def describe(values):
    if not values:
        return {'count': 0, 'minimum': None, 'median': None, 'p95': None, 'maximum': None}
    if any(isinstance(v, bool) or not isinstance(v, (int, float)) or not math.isfinite(v) or v < 0 for v in values):
        raise ValueError('Measurements must be finite nonnegative numbers')
    ordered = sorted(values)
    middle = len(ordered) // 2
    median = ordered[middle] if len(ordered) % 2 else (ordered[middle - 1] + ordered[middle]) / 2
    return {'count': len(ordered), 'minimum': ordered[0], 'median': median,
            'p95': percentile(ordered, 95), 'maximum': ordered[-1]}


def write_json(path, value):
    path = Path(path)
    path.parent.mkdir(mode=0o700, parents=True, exist_ok=True)
    temporary = path.with_suffix(path.suffix + '.tmp')
    fd = os.open(temporary, os.O_WRONLY | os.O_CREAT | os.O_TRUNC, 0o600)
    with os.fdopen(fd, 'w') as file:
        json.dump(value, file, indent=2)
        file.write('\n')
    os.replace(temporary, path)


def require_owned_release(run_id, release, recorded_uid, live):
    if not re.fullmatch(r'[0-9a-f]{12}', run_id) or not release.startswith('eval-' + run_id + '-'):
        raise ValueError('Refused release outside this evaluation run')
    meta = live.get('metadata', {})
    labels = meta.get('labels', {})
    if not recorded_uid or meta.get('uid') != recorded_uid or labels.get('name') != release or labels.get('owner') != 'helm':
        raise ValueError('Recorded Helm identity changed; cleanup refused')


def derive_reference_preset(steady_cpu_millicores, lifetime_memory_bytes):
    """Predeclared headroom rule; reference HTTP workloads only, not generic sizing."""
    if not math.isfinite(steady_cpu_millicores) or not math.isfinite(lifetime_memory_bytes) or steady_cpu_millicores <= 0 or lifetime_memory_bytes <= 0:
        raise ValueError('Positive observed CPU and memory evidence is required')
    request_cpu = max(25, math.ceil(steady_cpu_millicores * 2 / 25) * 25)
    limit_cpu = max(250, math.ceil(request_cpu * 2 / 250) * 250)
    request_mib = max(32, math.ceil(lifetime_memory_bytes * 2 / (16 * 2**20)) * 16)
    limit_mib = max(128, math.ceil(request_mib * 2 / 64) * 64)
    return {'cpuRequest': f'{request_cpu}m', 'cpuLimit': f'{limit_cpu}m',
            'memoryRequest': f'{request_mib}Mi', 'memoryLimit': f'{limit_mib}Mi'}


def lifecycle_acceptance(timings, plan, include_failure=True):
    missing = []
    failed = []
    limits = {'deploy-v1': plan['targets']['cachedDeploySeconds'],
              'update-v2': plan['targets']['cachedUpdateSeconds']}
    if include_failure:
        limits['unhealthy-v3-and-restoration'] = plan['helmFailureTimeoutSeconds'] + plan['targets']['recoveryAfterFailureSeconds']
    for method in ('api', 'helm'):
        for stage, limit in limits.items():
            rows = [r for r in timings if r['method'] == method and r['stage'] == stage]
            if len(rows) != plan['repetitions'] or {r['repetition'] for r in rows} != set(range(1, plan['repetitions'] + 1)):
                missing.append({'method': method, 'stage': stage})
            for row in rows:
                if not math.isfinite(row['seconds']) or row['seconds'] < 0 or row['seconds'] > limit:
                    failed.append({'method': method, 'stage': stage, 'repetition': row['repetition'],
                                   'seconds': row['seconds'], 'targetSeconds': limit})
    return {'passed': not missing and not failed, 'missingScenarios': missing, 'failedTargets': failed}


def content_state(descriptors, present):
    """Classify all recorded native content, never infer coldness from a missing tag."""
    if not descriptors or not any(x['kind'] == 'layer' for x in descriptors):
        raise ValueError('Native manifest and layer evidence are required')
    missing = [x for x in descriptors if x['digest'] not in present]
    layers = [x for x in descriptors if x['kind'] == 'layer']
    missing_layers = [x for x in layers if x['digest'] not in present]
    condition = 'fully-cold-content' if len(missing) == len(descriptors) else 'partial-cache' if missing else 'all-content-cached'
    return {'condition': condition, 'missingContent': missing,
            'missingLayerBytes': sum(x['size'] for x in missing_layers),
            'totalLayerBytes': sum(x['size'] for x in layers),
            'presentDescriptors': len(descriptors) - len(missing), 'totalDescriptors': len(descriptors)}


def require_application(run_id, app_id, recorded, live):
    if not re.fullmatch(r'[0-9a-f]{12}', run_id) or not recorded['name'].startswith('eval-' + run_id + '-'):
        raise ValueError('Invalid recorded evaluation identity')
    if live['id'] != app_id or live['name'] != recorded['name'] or live['activeOperationId']:
        raise ValueError('Application identity changed or an operation remains active; inspect before cleanup')
