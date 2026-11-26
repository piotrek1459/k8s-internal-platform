#!/usr/bin/env python3
"""Recompute reference resource/request results from complete raw observations."""
import argparse
import hashlib
import gzip
import json
from pathlib import Path
import urllib.parse
from common import describe, derive_reference_preset, load_plan, write_json
from load import summarize


def evidence_bytes(path):
    path = Path(path)
    return path.read_bytes() if path.exists() else gzip.decompress(Path(str(path) + '.gz').read_bytes())


def rows(path):
    return [json.loads(line) for line in evidence_bytes(path).decode().splitlines() if line.strip()]


def analyze(host_folder, client_folder, fixture, plan, clock_offset=0):
    host_folder, client_folder = Path(host_folder), Path(client_folder)
    before = json.loads((host_folder / 'identity-before.json').read_text())
    host_result = json.loads((host_folder / 'summary.json').read_text())
    client = json.loads((client_folder / 'summary.json').read_text())
    samples = rows(host_folder / 'samples.jsonl')
    requests = rows(client_folder / 'requests.jsonl')
    if before['appID'] != fixture['appID'] or host_result['appID'] != fixture['appID']:
        raise ValueError('Fixture and sampled application identities differ')
    if urllib.parse.urlsplit(client['url']).netloc != urllib.parse.urlsplit(fixture['access']['url']).netloc:
        raise ValueError('Load client targeted another application hostname')
    if not host_result['evidenceValid'] or not samples or len(samples) != host_result['sampleCount']:
        raise ValueError('Host evidence is incomplete or container identity changed')
    # The initial collector/plan revision fixed four slots without this JSON field.
    if client.get('maximumOutstandingRequests', 4) != plan.get('maximumOutstandingRequests', 4):
        raise ValueError('Client concurrency differs from the selected plan revision')
    expected = plan['requestRatePerSecond'] * plan['loadedSeconds']
    total = plan['requestRatePerSecond'] * (plan['loadedSeconds'] + plan['warmupSeconds'])
    request_result = summarize(requests, plan['targets']['loadedP95Milliseconds'], expected, total)
    if not request_result['complete']:
        raise ValueError('Request observations are incomplete')
    for field, key in (('ratePerSecond', 'requestRatePerSecond'), ('durationSeconds', 'loadedSeconds'), ('warmupSeconds', 'warmupSeconds')):
        if client[field] != plan[key]:
            raise ValueError('Client schedule differs from the fixed plan')
    steady = [r for r in requests if not r['warmup']]
    loaded_start = min(r['scheduledEpochSeconds'] for r in steady) + clock_offset
    loaded_end = max(r['scheduledEpochSeconds'] for r in steady) + 1 / plan['requestRatePerSecond'] + clock_offset
    idle_start = samples[0]['epochSeconds']
    idle_end = idle_start + plan['idleSeconds']
    if loaded_start < idle_end or samples[-1]['epochSeconds'] < loaded_end:
        raise ValueError('Host samples do not cover the prescribed idle and loaded windows')
    identities = {c['containerID'] for c in before['containers']}
    phase = {'idle': [], 'loaded': []}
    host_phase = {'idle': [], 'loaded': []}
    memory_peak = 0
    maximum_oom = 0
    for sample in samples:
        if {c['containerID'] for c in sample['containers']} != identities or any(c.get('missing') for c in sample['containers']):
            raise ValueError('Missing or changed sampled container')
        for item in sample['containers']:
            memory_peak = max(memory_peak, item['memoryPeakBytes'])
            maximum_oom = max(maximum_oom, item['memoryEvents']['oom_kill'])
    for previous, current in zip(samples, samples[1:]):
        dt = current['epochSeconds'] - previous['epochSeconds']
        if not .5 * plan['sampleIntervalSeconds'] <= dt <= 2 * plan['sampleIntervalSeconds']:
            raise ValueError('Missing/stale host sampling interval')
        selected = 'idle' if previous['epochSeconds'] >= idle_start and current['epochSeconds'] <= idle_end else 'loaded' if previous['epochSeconds'] >= loaded_start and current['epochSeconds'] <= loaded_end else None
        if selected is None:
            continue
        old_cpu = list(map(int, previous['hostCPU'].split()[1:9]))
        new_cpu = list(map(int, current['hostCPU'].split()[1:9]))
        delta = [new - old for old, new in zip(old_cpu, new_cpu)]
        if len(delta) != 8 or min(delta) < 0 or sum(delta) <= 0:
            raise ValueError('Invalid host CPU counters')
        memory = current['hostMemoryBytes']
        host_phase[selected].append({
            'cpuBusyPercentAcrossAllCores': 100 * (sum(delta) - delta[3] - delta[4]) / sum(delta),
            'unavailableMemoryBytes': memory['MemTotal'] - memory['MemAvailable'],
            'usedSwapBytes': memory['SwapTotal'] - memory['SwapFree']})
        old = {c['containerID']: c for c in previous['containers']}
        for item in current['containers']:
            baseline = old[item['containerID']]
            cpu = item['CPU']['usage_usec'] - baseline['CPU']['usage_usec']
            throttled = item['CPU']['throttled_usec'] - baseline['CPU']['throttled_usec']
            periods = item['CPU']['nr_periods'] - baseline['CPU']['nr_periods']
            limited = item['CPU']['nr_throttled'] - baseline['CPU']['nr_throttled']
            if min(cpu, throttled, periods, limited) < 0:
                raise ValueError('Container counters reset during sampling')
            phase[selected].append({'cpuMillicores': cpu / dt / 1000, 'memoryCurrentBytes': item['memoryCurrentBytes'],
                                    'throttledMicroseconds': throttled, 'throttledPeriods': limited, 'periods': periods})
    for name, seconds in (('idle', plan['idleSeconds']), ('loaded', plan['loadedSeconds'])):
        minimum = max(1, int(seconds / plan['sampleIntervalSeconds']) - 2) * len(identities)
        if len(phase[name]) < minimum:
            raise ValueError('Insufficient ' + name + ' observations')
    temperatures = [s['temperatureCelsius'] for s in samples]
    if any(t is None for t in temperatures):
        raise ValueError('Reference host temperature evidence is missing')
    restarts = max(c['restartCount'] for c in before['containers'] + host_result['containersAfter'])
    max_cpu = max(row['cpuMillicores'] for row in phase['loaded'])
    disk_free = min(s['freeDiskBytes'] for s in samples)
    targets = plan['targets']
    checks = {'requests': request_result['targetsPassed'], 'OOMKills': maximum_oom <= targets['maximumOOMKills'],
              'restarts': restarts <= targets['maximumContainerRestarts'],
              'temperature': max(temperatures) < targets['temperatureCeilingCelsius'],
              'diskHeadroom': disk_free >= targets['minimumFreeDiskBytes']}
    return {'appID': fixture['appID'], 'image': fixture['image'], 'clockOffsetSecondsHostMinusClient': clock_offset,
            'targetsPassed': all(checks.values()), 'targetChecks': checks, 'requests': request_result,
            'phases': {name: {'cpuMillicores': describe([r['cpuMillicores'] for r in data]),
                             'memoryCurrentBytes': describe([r['memoryCurrentBytes'] for r in data]),
                             'throttledMicroseconds': sum(r['throttledMicroseconds'] for r in data),
                             'throttledPeriods': sum(r['throttledPeriods'] for r in data),
                             'periods': sum(r['periods'] for r in data)} for name, data in phase.items()},
            'hostPhases': {name: {metric: describe([r[metric] for r in data]) for metric in
                ('cpuBusyPercentAcrossAllCores', 'unavailableMemoryBytes', 'usedSwapBytes')}
                for name, data in host_phase.items()},
            'hostScope': 'Whole host including K3s, platform, retained demo and collectors; not broker-only RSS',
            'maximumLifetimeContainerMemoryPeakBytes': memory_peak, 'maximumOOMKills': maximum_oom,
            'maximumRestartCount': restarts, 'maximumTemperatureCelsius': max(temperatures),
            'minimumFreeDiskBytes': disk_free, 'derivedReferencePreset': derive_reference_preset(max_cpu, memory_peak),
            'memoryPeakScope': 'Container lifetime including startup; no peak counter reset', 'humanOperatorTime': None,
            'inputSHA256': {str(path): hashlib.sha256(evidence_bytes(path)).hexdigest() for path in
                [host_folder / 'identity-before.json', host_folder / 'samples.jsonl', host_folder / 'summary.json',
                 client_folder / 'requests.jsonl', client_folder / 'summary.json']}}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--host', type=Path, required=True)
    parser.add_argument('--client', type=Path, required=True)
    parser.add_argument('--fixture', type=Path, required=True)
    parser.add_argument('--plan', type=Path, required=True)
    parser.add_argument('--clock-offset-seconds', type=float, default=0, help='Measured host epoch minus client epoch')
    parser.add_argument('--output', type=Path, required=True)
    args = parser.parse_args()
    result = analyze(args.host, args.client, json.loads(args.fixture.read_text()), load_plan(args.plan), args.clock_offset_seconds)
    write_json(args.output, result)
    print(json.dumps({'targetsPassed': result['targetsPassed'], 'derivedReferencePreset': result['derivedReferencePreset']}))
    return 0 if result['targetsPassed'] else 1


if __name__ == '__main__':
    raise SystemExit(main())
