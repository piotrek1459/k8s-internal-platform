"""Require non-empty, failure-free and skip-free E2E acceptance evidence."""
import argparse
from pathlib import Path
import subprocess
import xml.etree.ElementTree as XML


def acceptance_successful(result):
    return result.testsRun > 0 and result.wasSuccessful() and not result.skipped and not result.expectedFailures


def summarize(junit, source_commit, output):
    output.write(f"Source commit: `{source_commit}`\n\n")
    try:
        suite = XML.parse(junit).getroot()
        cases = suite.findall('testcase')
        tests = int(suite.attrib['tests'])
        failures = int(suite.get('failures', '0')) + int(suite.get('errors', '0'))
        skipped = int(suite.get('skipped', '0'))
        failed_cases = sum(case.find('failure') is not None or case.find('error') is not None for case in cases)
        skipped_cases = sum(case.find('skipped') is not None for case in cases)
        consistent = (suite.tag == 'testsuite' and tests == len(cases)
                      and failures == failed_cases and skipped == skipped_cases)
    except (OSError, XML.ParseError, KeyError, ValueError):
        output.write('Acceptance failed: the E2E report is missing or invalid.\n')
        return False
    output.write(f"Tests: {tests}; failures: {failures}; skipped: {skipped}\n\n")
    accepted = consistent and tests > 0 and failures == 0 and skipped == 0
    output.write('Acceptance: **passed**.\n' if accepted else
                 'Acceptance: **failed**. A consistent, non-empty report with zero failures and zero skips is required.\n')
    return accepted


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--junit', type=Path, required=True)
    parser.add_argument('--summary', type=Path, required=True)
    args = parser.parse_args()
    source = subprocess.check_output(['git', 'rev-parse', 'HEAD'], text=True).strip()
    with args.summary.open('a') as output:
        return 0 if summarize(args.junit, source, output) else 1


if __name__ == '__main__':
    raise SystemExit(main())
