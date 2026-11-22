"""Raspberry Pi boot preparation preserves a valid, idempotent command line."""
import pathlib
import subprocess
import tempfile
import unittest

ROOT = pathlib.Path(__file__).resolve().parents[2]


class MemoryCgroupTests(unittest.TestCase):
    def transform(self, text):
        with tempfile.TemporaryDirectory() as directory:
            path = pathlib.Path(directory) / 'cmdline.txt'
            path.write_text(text)
            return subprocess.run(['bash', '-c', 'source "$1"; memory_cgroup_cmdline "$2"', 'bash', str(ROOT / 'installer/install.sh'), str(path)], text=True, capture_output=True)

    def test_preserves_existing_arguments_and_replaces_conflicting_flags(self):
        result = self.transform('console=tty1 root=PARTUUID=test cgroup_disable=memory cgroup_memory=0 quiet\n')
        self.assertEqual(result.returncode, 0)
        self.assertEqual(result.stdout, 'console=tty1 root=PARTUUID=test quiet cgroup_memory=1 cgroup_enable=memory\n')

    def test_repeat_is_idempotent(self):
        line = 'console=tty1 cgroup_memory=1 cgroup_enable=memory\n'
        self.assertEqual(self.transform(line).stdout, line)

    def test_multiline_boot_configuration_is_refused(self):
        self.assertNotEqual(self.transform('console=tty1\nroot=test\n').returncode, 0)

    def test_read_only_check_cannot_prepare_host(self):
        result = subprocess.run(['bash', str(ROOT / 'installer/install.sh'), '--version', 'v0.1.2', '--check', '--prepare-host'], capture_output=True, text=True)
        self.assertNotEqual(result.returncode, 0)
        self.assertIn('read-only', result.stderr)
