# Manual releases

Release publication starts only through `workflow_dispatch`. The Release workflow
has no PR, branch-push or tag-push trigger. PR workflows run fast validation checks;
they do not create release tags, release bundles or GitHub Releases.

## Publish from a branch

The workflow definition must first be present on the default branch (`main`) for
GitHub to expose manual execution. After that, a release requires no version-bump
PR or pre-existing tag.

1. Open the repository's **Actions → Release → Run workflow**.
2. Keep **Use workflow from** set to `main` to use the installed workflow definition.
3. Set **branch** to the source branch to release; the default is `main`.
4. Enter **version**, for example `v0.1.1`.
5. Leave **prerelease** unchecked for a normal release, or check it for a prerelease.
6. Select **Run workflow**. On success, the workflow creates the version tag at the
   tested commit and publishes the complete release.

The source branch's HEAD is resolved once during preparation. Every validation,
E2E, build and publication job uses that exact SHA. Later commits on the branch
do not change the release already in progress. Tags cannot substitute for a
source branch.

The same operation through GitHub CLI is:

```bash
gh workflow run release.yml --ref main -f branch=main -f version=v0.1.1 -f prerelease=false
```

`--ref` selects the workflow definition; `branch` selects the application source.
GitHub's [manual workflow guide](https://docs.github.com/en/actions/how-tos/manage-workflow-runs/manually-run-a-workflow)
describes the UI and required repository access.

## Validation and publication

The workflow performs the following sequence:

1. Validate the version and source branch. Reject a published release, an existing
   tag at another commit, or a draft belonging to another commit.
2. Run all fast regression checks through `tests/run.py unit`.
3. Run the real-cluster API/browser/bundle E2E suite and build ARM64/AMD64 bundles.
4. Assemble and validate installer assets, compatibility/source metadata and checksums.
5. Create the tag at the tested SHA, create a draft, upload all six assets and
   publish the release. Failed tests/builds never reach tag creation or publication.

A failed upload can leave an unpublished draft and its tested tag. Rerun the
original workflow to retry that same commit. A fresh dispatch from a branch that
has since moved must use a new version. Existing published releases are refused.

`release/version.txt` is the development/test fallback version. The form input
supplies the released broker version, packaged chart version/appVersion, E2E
version, metadata and downloaded installation examples. Changing that fallback
file is not a prerequisite for publication. Release notes describe current
capabilities and limitations in `release/NOTES.md`.

Normal releases are the default and are marked Latest. Selecting `prerelease`
applies the prerelease label and does not mark that version Latest. This choice
is also recorded in `release.json` as the boolean `preview` field. The selection
controls publication labels; feature and native-host acceptance remains recorded
in [implementation status](../docs/status.md).

## Artifacts

Each `platform-vVERSION-linux-ARCH.tar.gz` contains a platform chart, namespace
manifest, platform/demo OCI archives, and `release.env`/`release.json`. Packaging
validates architecture, checksums and version labels. Normalized OCI archives and
containerd's `--local` import retain the exact recorded native digest references.

The six published assets are both native bundles, `install.sh`, `INSTALL.md`,
`release.json` and `SHA256SUMS`. The downloaded guide uses the selected version
and links to documentation at its release tag. `release.json` records the source
SHA and component pins. One final `SHA256SUMS` is assembled after both native
bundles are available and covers the bundles, installer, guide and metadata.

GitHub hosts the OCI bundles; GHCR credentials are not required. Initial K3s,
Helm and system-image downloads still require network access. The
[installation guide](../docs/installation.md) describes host requirements and
remaining native-host acceptance. Published versions are listed on the
[GitHub Releases page](https://github.com/piotrek1459/k8s-internal-platform/releases);
[implementation status](../docs/status.md) records current acceptance and availability.
