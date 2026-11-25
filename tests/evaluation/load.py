#!/usr/bin/env python3
"""Bounded verified-HTTPS request observations from a separate client machine."""
import argparse
from concurrent.futures import ThreadPoolExecutor
import http.client
import ipaddress
import json
from pathlib import Path
import socket
import ssl
import threading
import time
import urllib.parse

from common import describe, write_json


def summarize(records, target_p95=500, expected_count=None, expected_total=None):
    steady = [r for r in records if not r['warmup']]
    successful = [r for r in steady if r['success']]
    client_invalid = any(r.get('error') == 'client_saturated' or r.get('scheduleDelaySeconds', 0) > 2 for r in records)
    total = expected_total if expected_total is not None else expected_count
    complete = (expected_count is not None and len(steady) == expected_count and len(records) == total
                and {r['sequence'] for r in records} == set(range(total)))
    latency = describe([r['latencyMilliseconds'] for r in successful])
    success_rate = len(successful) / len(steady) if steady else None
    return {'requests': len(steady), 'successful': len(successful), 'successRate': success_rate,
            'latencyMilliseconds': latency, 'clientSchedulingValid': not client_invalid,
            'expectedRequests': expected_count, 'complete': complete,
            'targetsPassed': complete and bool(steady) and not client_invalid and success_rate == 1 and latency['p95'] <= target_p95,
            'protocol': 'HTTP/1.1', 'humanOperatorTime': None}


def run(url, output, ca, address=None, rate=20, seconds=60, warmup=10, timeout=3,
        expected_version=None, expected_text=None):
    target = urllib.parse.urlsplit(url)
    if target.scheme != 'https' or not target.hostname or target.username or target.password or target.query or target.fragment:
        raise ValueError('Use an HTTPS application URL without credentials/query/fragment')
    if not 1 <= rate <= 100 or not 1 <= seconds <= 600 or not 0 <= warmup <= 60 or not 1 <= timeout <= 10:
        raise ValueError('Rate/duration/warmup/timeout exceed the bounded experiment range')
    if expected_version is None and expected_text is None:
        raise ValueError('An expected version or response text is required as a positive control')
    context = ssl.create_default_context(cafile=str(ca))
    original_dns = socket.getaddrinfo
    if address is not None:
        ipaddress.ip_address(address)
        def resolve(host, port, *args, **kwargs):
            return original_dns(address if host == target.hostname else host, port, *args, **kwargs)
        socket.getaddrinfo = resolve
    local = threading.local()
    connections = []
    lock = threading.Lock()
    permits = threading.BoundedSemaphore(4)
    records = []
    started = time.monotonic()
    wall_started = time.time()

    def request(sequence, scheduled):
        begin = time.monotonic()
        record = {'sequence': sequence, 'warmup': sequence < rate * warmup,
                  'scheduledEpochSeconds': wall_started + scheduled,
                  'startedEpochSeconds': time.time(),
                  'scheduleDelaySeconds': round(begin - started - scheduled, 6),
                  'status': None, 'success': False}
        try:
            if not hasattr(local, 'connection'):
                local.connection = http.client.HTTPSConnection(target.hostname, target.port or 443, timeout=timeout, context=context)
                with lock:
                    connections.append(local.connection)
            connection = local.connection
            connection.request('GET', target.path or '/', headers={'Accept': '*/*'})
            response = connection.getresponse()
            body = response.read((1 << 20) + 1)
            record['status'] = response.status
            if len(body) > 1 << 20:
                raise ValueError('Response exceeded the 1 MiB observation bound')
            valid = response.status == 200
            if expected_version is not None:
                data = json.loads(body)
                valid = valid and data.get('version') == expected_version
                record['applicationHostname'] = data.get('hostname')
            if expected_text is not None:
                valid = valid and expected_text.encode() in body
            record['success'] = valid
            if not valid:
                record['error'] = 'unexpected_response'
        except Exception as error:
            record['error'] = type(error).__name__
            if hasattr(local, 'connection'):
                local.connection.close()
                del local.connection
        finally:
            record['latencyMilliseconds'] = round((time.monotonic() - begin) * 1000, 6)
            with lock:
                records.append(record)
            permits.release()

    try:
        with ThreadPoolExecutor(max_workers=4) as executor:
            for sequence in range(rate * (seconds + warmup)):
                scheduled = sequence / rate
                delay = started + scheduled - time.monotonic()
                if delay > 0:
                    time.sleep(delay)
                if permits.acquire(blocking=False):
                    executor.submit(request, sequence, scheduled)
                else:
                    records.append({'sequence': sequence, 'warmup': sequence < rate * warmup,
                                    'scheduledEpochSeconds': wall_started + scheduled,
                                    'scheduleDelaySeconds': round(time.monotonic() - started - scheduled, 6),
                                    'status': None, 'success': False, 'error': 'client_saturated',
                                    'latencyMilliseconds': 0})
    finally:
        for connection in connections:
            connection.close()
        socket.getaddrinfo = original_dns
    records.sort(key=lambda item: item['sequence'])
    output = Path(output)
    output.mkdir(mode=0o700, parents=True, exist_ok=True)
    raw = output / 'requests.jsonl'
    with raw.open('w') as file:
        for record in records:
            file.write(json.dumps(record) + '\n')
    raw.chmod(0o600)
    result = {'url': url, 'addressOverride': address, 'certificateVerification': True,
              'ratePerSecond': rate, 'durationSeconds': seconds, 'warmupSeconds': warmup,
              'startedEpochSeconds': wall_started, 'finishedEpochSeconds': time.time(),
              **summarize(records, expected_count=rate * seconds, expected_total=rate * (seconds + warmup))}
    write_json(output / 'summary.json', result)
    return result


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--url', required=True)
    parser.add_argument('--ca', type=Path, required=True)
    parser.add_argument('--address', help='Explicit IP resolution for this hostname; TLS SNI/verification remain enabled')
    parser.add_argument('--output', type=Path, required=True)
    parser.add_argument('--rate', type=int, default=20)
    parser.add_argument('--seconds', type=int, default=60)
    parser.add_argument('--warmup', type=int, default=10)
    parser.add_argument('--timeout', type=int, default=3)
    expected = parser.add_mutually_exclusive_group(required=True)
    expected.add_argument('--version')
    expected.add_argument('--contains')
    args = parser.parse_args()
    result = run(args.url, args.output, args.ca, args.address, args.rate, args.seconds,
                 args.warmup, args.timeout, args.version, args.contains)
    print(json.dumps(result, indent=2))
    return 0 if result['targetsPassed'] else 1


if __name__ == '__main__':
    raise SystemExit(main())
