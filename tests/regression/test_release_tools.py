"""Release integrity and installer guard regressions; no root or cluster needed."""
import hashlib
import importlib.util
import io
import json
import os
from pathlib import Path
import subprocess
import sys
import tarfile
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[2]
spec = importlib.util.spec_from_file_location("packaging", ROOT / "release/package.py")
packaging = importlib.util.module_from_spec(spec); spec.loader.exec_module(packaging)
manifest_spec = importlib.util.spec_from_file_location("release_manifest", ROOT / "release/manifest.py")
manifest = importlib.util.module_from_spec(manifest_spec); manifest_spec.loader.exec_module(manifest)


def image(path, arch="arm64", version="0.0.1", corrupt=False):
    blobs = {}
    def blob(value):
        raw = json.dumps(value).encode(); digest = hashlib.sha256(raw).hexdigest()
        blobs["blobs/sha256/" + digest] = raw
        return "sha256:" + digest
    config = blob({"os": "linux", "architecture": arch, "config": {"Labels": {"org.opencontainers.image.version": version}}})
    manifest = blob({"schemaVersion": 2, "config": {"digest": config}, "layers": []})
    blobs["index.json"] = json.dumps({"manifests": [{"digest": manifest}]}).encode()
    if corrupt: blobs["blobs/sha256/" + config.split(":")[1]] = b"{}"
    with tarfile.open(path, "w") as archive:
        for name, raw in blobs.items():
            item = tarfile.TarInfo(name); item.size = len(raw); archive.addfile(item, io.BytesIO(raw))
    return manifest


class ReleaseIntegrityTests(unittest.TestCase):
    def test_release_manifest_requires_both_correctly_identified_architectures(self):
        with tempfile.TemporaryDirectory() as directory:
            folder = Path(directory)
            for name in ("install.sh", "INSTALL.md"):
                (folder / name).write_text("[API guide](user/api-reference.md)" if name == "INSTALL.md" else "fixture")
            for arch in ("amd64", "arm64"):
                path = folder / ("platform-v0.0.1-linux-" + arch + ".tar.gz")
                raw = json.dumps({"version": "0.0.1", "architecture": arch}).encode()
                with tarfile.open(path, "w:gz") as archive:
                    entry = tarfile.TarInfo("release.json"); entry.size = len(raw)
                    archive.addfile(entry, io.BytesIO(raw))
                if arch == "amd64":
                    with self.assertRaises(FileNotFoundError):
                        manifest.assemble(folder, "0.0.1", "a" * 40)
            manifest.assemble(folder, "v0.0.1", "a" * 40)
            self.assertIn("/blob/v0.0.1/docs/user/api-reference.md", (folder / "INSTALL.md").read_text())
            metadata = json.loads((folder / "release.json").read_text())
            self.assertEqual(metadata["commit"], "a" * 40)
            self.assertEqual(set(metadata["targets"]), {"amd64", "arm64"})
            records = (folder / "SHA256SUMS").read_text().splitlines()
            self.assertEqual(len(records), 5)
            for record in records:
                digest, name = record.split()
                self.assertEqual(digest, hashlib.sha256((folder / name).read_bytes()).hexdigest())

    def test_dispatch_version_updates_assets_without_a_source_version_bump(self):
        fallback = (ROOT / "release/version.txt").read_text()
        with tempfile.TemporaryDirectory() as directory:
            folder = Path(directory)
            (folder / "install.sh").write_text("Usage: --version v0.0.1")
            (folder / "INSTALL.md").write_text("Install v0.0.1: [API](user/api-reference.md)")
            for arch in ("amd64", "arm64"):
                raw = json.dumps({"version": "0.0.2", "architecture": arch}).encode()
                with tarfile.open(folder / ("platform-v0.0.2-linux-" + arch + ".tar.gz"), "w:gz") as archive:
                    entry = tarfile.TarInfo("release.json"); entry.size = len(raw)
                    archive.addfile(entry, io.BytesIO(raw))
            manifest.assemble(folder, "v0.0.2", "a" * 40)
            self.assertIn("--version v0.0.2", (folder / "install.sh").read_text())
            guide = (folder / "INSTALL.md").read_text()
            self.assertIn("Install v0.0.2", guide)
            self.assertIn("/blob/v0.0.2/docs/", guide)
            self.assertNotIn("v0.0.1", guide)
        self.assertEqual((ROOT / "release/version.txt").read_text(), fallback)

    def test_native_digest_and_image_version_are_verified(self):
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "image.tar"
            digest = image(path)
            self.assertEqual(packaging.image_digest(path, "arm64", "0.0.1"), digest)
            for arch, version in (("amd64", "0.0.1"), ("arm64", "0.0.2")):
                with self.subTest(arch=arch, version=version), self.assertRaises(ValueError):
                    packaging.image_digest(path, arch, version)

    def test_wrapped_oci_index_is_normalized_to_the_recorded_native_digest(self):
        with tempfile.TemporaryDirectory() as directory:
            folder = Path(directory); source = folder / "source.tar"
            digest = image(source)
            wrapper = json.dumps({"schemaVersion": 2, "manifests": [{"digest": digest}]}).encode()
            wrapped_digest = "sha256:" + hashlib.sha256(wrapper).hexdigest()
            wrapped = folder / "wrapped.tar"
            with tarfile.open(source) as original, tarfile.open(wrapped, "w") as archive:
                for member in original.getmembers():
                    if member.name != "index.json": archive.addfile(member, original.extractfile(member))
                for name, raw in (("index.json", json.dumps({"manifests": [{"digest": wrapped_digest}]}).encode()),
                                  ("blobs/sha256/" + wrapped_digest.split(":")[1], wrapper)):
                    member = tarfile.TarInfo(name); member.size = len(raw); archive.addfile(member, io.BytesIO(raw))
            native = packaging.image_digest(wrapped, "arm64", "0.0.1")
            output = folder / "normalized.tar"
            packaging.native_archive(wrapped, output, native, "arm64", "platform.local/platform-broker:0.0.1")
            with tarfile.open(output) as archive:
                index = json.load(archive.extractfile("index.json"))
                self.assertEqual(index["manifests"][0]["digest"], digest)
                self.assertEqual(index["manifests"][0]["platform"]["architecture"], "arm64")
                self.assertEqual(len(index["manifests"]), 1)
                self.assertNotIn("blobs/sha256/" + wrapped_digest.split(":")[1], archive.getnames())

    def test_corrupt_oci_metadata_is_rejected(self):
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "image.tar"; image(path, corrupt=True)
            with self.assertRaisesRegex(ValueError, "checksum"):
                packaging.image_digest(path, "arm64", "0.0.1")

    def test_bundle_contains_pins_images_and_chart(self):
        with tempfile.TemporaryDirectory() as directory:
            folder = Path(directory); images = {}
            for key in ("platform", "v1", "v2", "v3"):
                path = folder / (key + ".tar"); image(path, version="0.0.1" if key == "platform" else key); images[key] = path
            helm = folder / "helm"
            helm.write_text('#!/usr/bin/env python3\nimport pathlib,sys\na=sys.argv\np=pathlib.Path(a[a.index("--destination")+1])/ ("platform-"+a[a.index("--version")+1]+".tgz")\np.write_bytes(b"chart-fixture")\n')
            helm.chmod(0o755)
            bundle = packaging.package("0.0.1", "arm64", images, folder / "out", helm)
            with tarfile.open(bundle) as archive:
                self.assertEqual(set(archive.getnames()), {"release.env", "release.json", "namespace.yaml", "platform-chart.tgz", "platform-image.oci.tar", "demo-v1.oci.tar", "demo-v2.oci.tar", "demo-v3.oci.tar"})
                metadata = archive.extractfile("release.env").read().decode()
                self.assertIn("PLATFORM_VERSION=0.0.1", metadata)
                self.assertIn("PLATFORM_IMAGE=platform.local/platform-broker@sha256:", metadata)


class DiagnosticRedactionTests(unittest.TestCase):
    def test_e2e_build_version_uses_the_release_input(self):
        from unittest.mock import patch
        sys.path.insert(0, str(ROOT / "tests/e2e"))
        from support import Environment
        with tempfile.TemporaryDirectory() as folder, patch.dict(os.environ, PLATFORM_RELEASE_VERSION="0.0.2"):
            environment = Environment(Path(folder))
            self.assertEqual(environment.version, "0.0.2")
            self.assertEqual(environment.env["PLATFORM_BUILD_VERSION"], "0.0.2")

    def test_interleaved_bootstrap_output_is_redacted_before_token_is_known(self):
        sys.path.insert(0, str(ROOT / "tests/e2e"))
        from support import Environment
        with tempfile.TemporaryDirectory() as folder:
            environment = Environment(Path(folder))
            token = "a" * 32
            logs = 'Bootstrap token (single use):\n{"msg":"setup_available"}\n  ' + token + '\n'
            self.assertNotIn(token, environment.redact(logs))
            self.assertIn("[REDACTED]", environment.redact(logs))


class InstallerGuards(unittest.TestCase):
    def cli(self, *args):
        return subprocess.run(["bash",str(ROOT / "installer/install.sh"),*args],text=True,capture_output=True,timeout=10)

    def helper(self, command, directory, *args):
        return subprocess.run(["bash", "-c", 'source "$1"; WORK="$2"; REQUESTED_VERSION=0.0.1; ARCH=arm64; '+command, "installer-test",str(ROOT / "installer/install.sh"),str(directory),*map(str,args)],text=True,capture_output=True,timeout=10)

    def test_help_does_not_require_root_or_linux(self):
        result = self.cli("--help")
        self.assertEqual(result.returncode, 0); self.assertIn("--check", result.stdout)

    def test_invalid_arguments_are_rejected_before_host_changes(self):
        for args in (("--version", "0.0.1;invalid"), ("--version", "v0.0.1", "--bundle", "missing"), ("--unknown",)):
            with self.subTest(args=args): self.assertNotEqual(self.cli(*args).returncode, 0)

    def test_32_bit_raspberry_pi_is_rejected(self):
        with tempfile.TemporaryDirectory() as folder:
            result = self.helper("architecture armv7l", folder)
            self.assertNotEqual(result.returncode, 0); self.assertIn("32-bit", result.stderr)

    def test_bad_checksum_stops_before_archive_extraction(self):
        with tempfile.TemporaryDirectory() as directory:
            folder = Path(directory); bundle = folder / "bundle.tar.gz"; bundle.write_bytes(b"not an archive")
            checksum = folder / "SHA256SUMS"; checksum.write_text("a" * 64 + "  platform-v0.0.1-linux-arm64.tar.gz\n")
            result = self.helper('verify_bundle "$3" "$4"', folder, bundle, checksum)
            self.assertNotEqual(result.returncode, 0); self.assertIn("SHA-256", result.stderr)
            self.assertFalse((folder / "bundle").exists())

    def test_traversal_member_is_rejected(self):
        with tempfile.TemporaryDirectory() as directory:
            folder = Path(directory); bundle = folder / "bundle.tar.gz"
            with tarfile.open(bundle, "w:gz") as archive:
                entry = tarfile.TarInfo("../escaped");entry.size=1;archive.addfile(entry,io.BytesIO(b"x"))
            checksum=folder/"SHA256SUMS";checksum.write_text(hashlib.sha256(bundle.read_bytes()).hexdigest()+"  platform-v0.0.1-linux-arm64.tar.gz\n")
            result=self.helper('verify_bundle "$3" "$4"',folder,bundle,checksum)
            self.assertNotEqual(result.returncode,0);self.assertIn("Unexpected bundle member",result.stderr)

    def test_metadata_is_not_executed_as_shell(self):
        with tempfile.TemporaryDirectory() as directory:
            folder=Path(directory);(folder/"bundle").mkdir();probe=folder/"executed"
            (folder/"bundle/release.env").write_text('PLATFORM_VERSION=$(touch '+str(probe)+')\n')
            result=self.helper("load_metadata",folder)
            self.assertNotEqual(result.returncode,0);self.assertFalse(probe.exists())


if __name__ == "__main__": unittest.main()
