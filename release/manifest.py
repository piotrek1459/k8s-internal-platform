#!/usr/bin/env python3
"""Assemble release metadata and checksums after both native bundles exist."""
import argparse
import hashlib
import json
from pathlib import Path
import re
import tarfile


def assemble(directory, version, commit, prerelease=False):
    version = version.removeprefix("v")
    if not re.fullmatch(r"[0-9]+\.[0-9]+\.[0-9]+", version) or not re.fullmatch(r"[0-9a-f]{40}", commit):
        raise ValueError("Invalid release identity")
    targets = {}
    for architecture in ("amd64", "arm64"):
        bundle = directory / f"platform-v{version}-linux-{architecture}.tar.gz"
        with tarfile.open(bundle) as archive:
            metadata = json.load(archive.extractfile("release.json"))
        if metadata["version"] != version or metadata["architecture"] != architecture:
            raise ValueError("Bundle identity differs from release identity")
        targets[architecture] = metadata
    # The downloaded guide has no adjacent docs/ tree. Pin its relative source
    # links to this release rather than leaving them broken outside a checkout.
    source_version = (Path(__file__).resolve().parent / "version.txt").read_text().strip()
    installer = directory / "install.sh"
    installer.write_text(installer.read_text().replace("v" + source_version, "v" + version))
    guide = directory / "INSTALL.md"
    guide_text = guide.read_text()
    # Documentation examples may advance independently of the source build's
    # fallback version. Derive their version from the guide's own declaration.
    declaration = re.search(r"^The examples below use `(v[0-9]+\.[0-9]+\.[0-9]+)`\.", guide_text, re.MULTILINE)
    example = declaration[1] if declaration else "v" + source_version
    selected = "v" + version
    # Render runnable examples while retaining historical acceptance statements.
    guide_text = guide_text.replace(
        "The examples below use `" + example + "`.",
        "The examples below use `" + selected + "`.")
    guide_text = re.sub(r"(^```[^\n]*\n)(.*?)(^```[ \t]*$)",
                        lambda match: match[1] + match[2].replace(example, selected) + match[3],
                        guide_text, flags=re.MULTILINE | re.DOTALL)
    guide.write_text(guide_text)
    guide = directory / "INSTALL.md"
    prefix = "https://github.com/piotrek1459/k8s-internal-platform/blob/v" + version + "/docs/"
    guide.write_text(re.sub(r"\]\((?!https?://|#)([^)]+)\)",
                           lambda match: "](" + prefix + match.group(1) + ")", guide.read_text()))
    (directory / "release.json").write_text(json.dumps({"version": version, "commit": commit, "preview": prerelease, "targets": targets}, indent=2) + "\n")
    files = [directory / f"platform-v{version}-linux-{arch}.tar.gz" for arch in targets]
    files += [directory / name for name in ("install.sh", "INSTALL.md", "release.json")]
    records = []
    for path in files:
        digest = hashlib.sha256()
        with path.open("rb") as stream:
            for chunk in iter(lambda: stream.read(1024*1024), b""): digest.update(chunk)
        records.append(digest.hexdigest() + "  " + path.name)
    (directory / "SHA256SUMS").write_text("\n".join(records) + "\n")


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--directory", type=Path, required=True)
    parser.add_argument("--version", required=True)
    parser.add_argument("--commit", required=True)
    parser.add_argument("--prerelease", action="store_true")
    args = parser.parse_args()
    assemble(args.directory, args.version, args.commit, args.prerelease)
