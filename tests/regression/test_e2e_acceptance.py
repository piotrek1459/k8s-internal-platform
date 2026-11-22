"""Skipped or absent workflows cannot satisfy nightly or release acceptance."""
import io
from pathlib import Path
import sys
import tempfile
import unittest

sys.path.insert(0, str(Path(__file__).resolve().parents[1] / 'e2e'))
from acceptance import acceptance_successful, summarize


class E2EAcceptanceTests(unittest.TestCase):
    def test_runner_rejects_a_successful_result_with_skips_or_no_tests(self):
        result = unittest.TestResult()
        self.assertFalse(acceptance_successful(result))
        result.testsRun = 1
        self.assertTrue(acceptance_successful(result))
        result.skipped.append(('test', 'previously known defect'))
        self.assertTrue(result.wasSuccessful())
        self.assertFalse(acceptance_successful(result))

    def test_runner_rejects_failed_cases(self):
        result = unittest.TestResult(); result.testsRun = 1
        result.failures.append(('test', 'failure'))
        self.assertFalse(acceptance_successful(result))
        result.failures.clear()
        result.expectedFailures.append(('test', 'expected failure'))
        self.assertFalse(acceptance_successful(result))

    def test_summary_requires_complete_zero_skip_evidence_and_reports_source(self):
        cases = (
            ('<testsuite tests="1" failures="0" skipped="0"><testcase name="ready"/></testsuite>', True),
            ('<testsuite tests="1" failures="0" skipped="1"><testcase><skipped/></testcase></testsuite>', False),
            ('<testsuite tests="1" failures="1" skipped="0"><testcase><failure/></testcase></testsuite>', False),
            ('<testsuite tests="1" failures="0" skipped="0"><testcase><skipped/></testcase></testsuite>', False),
            ('<testsuite tests="1" failures="0" skipped="0"/>', False),
            ('<testsuite tests="0" failures="0" skipped="0"/>', False),
            ('invalid XML', False),
            (None, False),
        )
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / 'junit.xml'
            for xml, expected in cases:
                with self.subTest(xml=xml):
                    if path.exists(): path.unlink()
                    if xml is not None: path.write_text(xml)
                    output = io.StringIO()
                    self.assertEqual(summarize(path, 'a' * 40, output), expected)
                    self.assertIn('a' * 40, output.getvalue())
