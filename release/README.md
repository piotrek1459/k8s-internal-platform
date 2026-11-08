# Release maintenance

The initial distribution is a preview. The source version is stored in
`release/version.txt`; the platform chart and broker OCI version label match it.
The release notes state supported behavior and known limitations.

## Publish

1. Update the version file, platform chart version/appVersion, installation
   examples and release notes in a reviewed change.
2. Run `python3 -B tests/run.py unit` and the E2E suite described in
   [tests/README.md](../tests/README.md).
3. Create and push an immutable `vMAJOR.MINOR.PATCH` tag at the intended source
   commit. `.github/workflows/release.yml` validates the tag against the version
   file, runs all fast checks and E2E, and builds both native bundles.
4. Publication creates a draft, uploads complete assets and checksums, then
   publishes the prerelease. A published version is never overwritten. A failed
   draft can be repaired by rerunning its workflow before publication.

`workflow_dispatch` can rebuild an existing unpublished tag after the workflow
is merged into the default branch. Reusable E2E executes the same source commit.
A tag-triggered release can be produced from a reviewed feature commit before
merge; scheduled E2E becomes active only after merging the workflow into `main`.

## Artifacts

Each `platform-vVERSION-linux-ARCH.tar.gz` contains a platform chart, namespace
manifest, platform/demo OCI archives, and `release.env`/`release.json`. Packaging
validates native architecture, immutable manifest/config checksums and version
labels. `release/manifest.py` requires both architecture bundles and records the
source commit and compatibility pins. `SHA256SUMS` covers bundles, installer,
installation guide and top-level metadata.

GitHub hosts the OCI bundles; GHCR visibility or credentials are not required.
The installer imports image digests into containerd. This distribution is not an
air-gapped package: initial K3s/Helm/system images still need outbound downloads.
The [installation guide](../docs/installation.md) describes the dedicated-host
boundary, same-version reruns and deferred native-host validation.
