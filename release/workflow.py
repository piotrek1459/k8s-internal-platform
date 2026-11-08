#!/usr/bin/env python3
"""Validate a dispatched release and publish only its tested source commit."""
import argparse
import json
import os
from pathlib import Path
import re
import subprocess


VERSION = r"v?(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)"


def normalize_version(value):
    if not re.fullmatch(VERSION, value):
        raise ValueError("Use a version such as v0.0.2 (MAJOR.MINOR.PATCH).")
    return "v" + value.removeprefix("v")


def validate_sha(value):
    if not re.fullmatch(r"[0-9a-f]{40}", value):
        raise ValueError("The source must be a full commit SHA.")
    return value


def command(*args, cwd=None):
    result = subprocess.run(args, capture_output=True, text=True, cwd=cwd)
    if result.returncode:
        raise RuntimeError(result.stderr.strip() or "Command failed: " + args[0])
    return result.stdout.strip()


def api(path, method="GET", fields=None):
    # Preserve HTTP errors: authentication, rate limits and network failures
    # must never be interpreted as permission to create a new release.
    args = ["gh", "api", "--include", "--method", method,
            "repos/" + os.environ["GH_REPO"] + "/" + path]
    for key, value in (fields or {}).items():
        args += ["--raw-field", key + "=" + value]
    result = subprocess.run(args, capture_output=True, text=True)
    headers, _, body = result.stdout.partition("\n\n")
    status = re.match(r"HTTP/\S+\s+(\d{3})", headers)
    code = int(status.group(1)) if status else None
    if result.returncode or code not in (200, 201):
        raise RuntimeError(f"GitHub API {method} {path} failed (HTTP {code}): " + result.stderr.strip())
    return json.loads(body)


def remote_tag(version):
    name = "refs/tags/" + version
    lines = command("git", "ls-remote", "origin", name, name + "^{}").splitlines()
    refs = dict(line.split()[::-1] for line in lines)
    return refs.get(name + "^{}", refs.get(name))


def find_release(version):
    # The listing includes authenticated drafts, including a failed publication
    # whose tag has not yet been attached. Inspect every page for reused versions.
    page = 1
    while True:
        releases = api(f"releases?per_page=100&page={page}")
        for release in releases:
            if release["tag_name"] == version:
                return release
        if len(releases) < 100:
            return None
        page += 1


def check_remote_identity(version, sha):
    target = remote_tag(version)
    if target is not None and target != sha:
        raise ValueError("The version tag already points to another commit; choose a new version.")
    release = find_release(version)
    if release is not None:
        if not release["draft"]:
            raise ValueError("This release is already published; choose a new version.")
        if release["target_commitish"] != sha:
            raise ValueError("The draft belongs to another source commit; rerun its original workflow or choose a new version.")
    return release


def prepare(version, branch):
    version = normalize_version(version)
    command("git", "check-ref-format", "--branch", branch)
    name = "refs/heads/" + branch
    lines = command("git", "ls-remote", "--heads", "origin", name).splitlines()
    refs = dict(line.split()[::-1] for line in lines)
    if name not in refs:
        raise ValueError("The specified source branch does not exist: " + branch)
    # Resolve branch HEAD once. Subsequent jobs check out this SHA even if the
    # branch receives more commits while validation is running.
    sha = validate_sha(refs[name])
    check_remote_identity(version, sha)
    return {"version": version, "app_version": version[1:], "sha": sha, "branch": branch}


def publish(directory, version, sha, prerelease=False):
    version = normalize_version(version)
    sha = validate_sha(sha)
    assets = [directory / f"platform-{version}-linux-{arch}.tar.gz" for arch in ("arm64", "amd64")]
    assets += [directory / name for name in ("install.sh", "INSTALL.md", "release.json", "SHA256SUMS")]
    if any(not path.is_file() for path in assets):
        raise ValueError("All six release assets are required before creating a tag.")
    metadata = json.loads((directory / "release.json").read_text())
    if metadata["version"] != version[1:] or metadata["commit"] != sha:
        raise ValueError("Release assets differ from the tested version/source commit.")
    if metadata.get("preview") is not prerelease:
        raise ValueError("Release metadata does not match the selected prerelease setting.")
    command("sha256sum", "--check", "SHA256SUMS", cwd=directory)
    existing = check_remote_identity(version, sha)
    if remote_tag(version) is None:
        try:
            api("git/refs", "POST", {"ref": "refs/tags/" + version, "sha": sha})
        except RuntimeError:
            # Another actor may have created the same tag between checks. Only
            # an identical target is acceptable; a different target is refused.
            if remote_tag(version) != sha:
                raise
    if remote_tag(version) != sha:
        raise ValueError("The release tag does not match the tested source commit.")
    title = version + " — preview" if prerelease else version
    if existing is None:
        kind = ["--prerelease"] if prerelease else []
        command("gh", "release", "create", version, "--verify-tag", "--target", sha,
                "--draft", *kind, "--title", title, "--notes-file", "release/NOTES.md")
    # Recheck a created/reused draft before replacing any assets.
    check_remote_identity(version, sha)
    command("gh", "release", "upload", version, *map(str, assets), "--clobber")
    if remote_tag(version) != sha:
        raise ValueError("The release tag changed before publication.")
    command("gh", "release", "edit", version, "--draft=false",
            "--prerelease=" + str(prerelease).lower(), "--latest=" + str(not prerelease).lower(),
            "--title", title)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("command", choices=("prepare", "publish"))
    parser.add_argument("--version", required=True)
    parser.add_argument("--sha")
    parser.add_argument("--branch", default="main")
    parser.add_argument("--directory", type=Path, default=Path("dist"))
    parser.add_argument("--prerelease", action="store_true")
    args = parser.parse_args()
    if args.command == "prepare":
        outputs = prepare(args.version, args.branch)
        print(f"Release {outputs['version']} from {outputs['branch']} at {outputs['sha']}")
        with open(os.environ["GITHUB_OUTPUT"], "a") as output:
            for key, value in outputs.items():
                output.write(f"{key}={value}\n")
    else:
        if not args.sha:
            parser.error("--sha is required for publication")
        publish(args.directory, args.version, args.sha, args.prerelease)
        print("Published " + normalize_version(args.version) + " at " + args.sha)


if __name__ == "__main__":
    try:
        main()
    except (ValueError, RuntimeError) as error:
        raise SystemExit(str(error))
