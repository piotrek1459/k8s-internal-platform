#!/usr/bin/env python3
"""Create a checksummed native installation bundle from built OCI images."""
import argparse
import hashlib
import io
import json
from pathlib import Path
import re
import subprocess
import tarfile
import tempfile

ROOT = Path(__file__).resolve().parents[1]
REPOSITORIES = {"platform": "platform.local/platform-broker", "v1": "platform.local/http-demo", "v2": "platform.local/http-demo", "v3": "platform.local/http-demo"}


def image_digest(path, architecture, version):
    with tarfile.open(path) as archive:
        def read(name):
            member = archive.getmember(name)
            if not member.isfile():
                raise ValueError("OCI metadata must be a regular file")
            return archive.extractfile(member).read()

        def blob(digest):
            if not re.fullmatch(r"sha256:[0-9a-f]{64}", digest):
                raise ValueError("Unsupported OCI digest")
            raw = read("blobs/sha256/" + digest.split(":")[1])
            if "sha256:" + hashlib.sha256(raw).hexdigest() != digest:
                raise ValueError("OCI metadata checksum mismatch")
            return json.loads(raw)

        def walk(descriptor, depth=0):
            if depth > 8:
                raise ValueError("OCI index nesting exceeds limit")
            document = blob(descriptor["digest"])
            if "manifests" in document:
                result = []
                for child in document["manifests"]:
                    result.extend(walk(child, depth + 1))
                return result
            config = blob(document["config"]["digest"])
            if config.get("os") == "linux" and config.get("architecture") == architecture:
                label = config.get("config", {}).get("Labels", {}).get("org.opencontainers.image.version")
                if label != version:
                    raise ValueError(f"Image version label {label!r} does not match {version!r}")
                return [descriptor["digest"]]
            return []

        candidates = []
        for descriptor in json.loads(read("index.json"))["manifests"]:
            candidates.extend(walk(descriptor))
        if len(set(candidates)) != 1:
            raise ValueError("Expected exactly one matching native image")
        return candidates[0]


def versions():
    return dict(line.split("=", 1) for line in (ROOT / "dev/versions.env").read_text().splitlines() if line and not line.startswith("#"))


def package(version, architecture, images, output, helm):
    version = version.removeprefix("v")
    if not re.fullmatch(r"(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)", version):
        raise ValueError("Version must be a three-part release number")
    if architecture not in ("arm64", "amd64"):
        raise ValueError("Unsupported release architecture")
    pins = versions()
    expected = "rancher/k3s:" + pins["K3S_VERSION"].replace("+", "-")
    if pins["K3S_IMAGE"] != expected:
        raise ValueError("Native K3s and local image pins differ")
    references = {key: REPOSITORIES[key] + "@" + image_digest(path, architecture, version if key == "platform" else key) for key, path in images.items()}
    if set(references) != set(REPOSITORIES):
        raise ValueError("The platform and all three demo images are required")
    output.mkdir(parents=True, exist_ok=True)
    destination = output / f"platform-v{version}-linux-{architecture}.tar.gz"
    manifest = {"version": version, "architecture": architecture, "images": references, "k3s": pins["K3S_VERSION"], "helm": pins["HELM_VERSION"]}
    environment = {"PLATFORM_VERSION": version, "PLATFORM_ARCH": architecture, "K3S_VERSION": pins["K3S_VERSION"], "HELM_VERSION": pins["HELM_VERSION"], "PLATFORM_IMAGE": references["platform"], **{f"DEMO_{key.upper()}_IMAGE": references[key] for key in ("v1", "v2", "v3")}}
    with tempfile.TemporaryDirectory(prefix="platform-package-") as directory:
        staging = Path(directory)
        subprocess.run([str(helm), "package", str(ROOT / "deploy/helm/platform"), "--version", version, "--app-version", version, "--destination", str(staging)], check=True, stdout=subprocess.DEVNULL)
        with tarfile.open(destination, "w:gz") as archive:
            def content(name, raw):
                entry = tarfile.TarInfo(name)
                entry.size = len(raw); entry.mode = 0o644
                archive.addfile(entry, io.BytesIO(raw))
            content("release.env", "".join(f"{key}={value}\n" for key, value in environment.items()).encode())
            content("release.json", (json.dumps(manifest, indent=2) + "\n").encode())
            archive.add(staging / f"platform-{version}.tgz", arcname="platform-chart.tgz")
            archive.add(ROOT / "dev/namespace.yaml", arcname="namespace.yaml")
            archive.add(images["platform"], arcname="platform-image.oci.tar")
            for key in ("v1", "v2", "v3"):
                archive.add(images[key], arcname=f"demo-{key}.oci.tar")
    digest = file_hash(destination)
    destination.with_suffix(destination.suffix + ".sha256").write_text(f"{digest}  {destination.name}\n")
    return destination


def file_hash(path):
    digest = hashlib.sha256()
    with path.open("rb") as file:
        for chunk in iter(lambda: file.read(1024 * 1024), b""):
            digest.update(chunk)
    return digest.hexdigest()


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--version", required=True)
    parser.add_argument("--arch", required=True, choices=("amd64", "arm64"))
    parser.add_argument("--platform", type=Path, required=True)
    for key in ("v1", "v2", "v3"):
        parser.add_argument("--" + key, type=Path, required=True)
    parser.add_argument("--output", type=Path, required=True)
    parser.add_argument("--helm", type=Path, default=ROOT / ".local/bin/helm")
    args = parser.parse_args()
    images = {key: getattr(args, key) for key in REPOSITORIES}
    print(package(args.version, args.arch, images, args.output, args.helm))


if __name__ == "__main__":
    main()
