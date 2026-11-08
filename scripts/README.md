# Operational scripts

`scripts/platform.sh` is the local build and execution entry point. It builds the
UI, broker and demos, manages the selected k3d installation and provides
foreground forwards. `stop` retains installation data. No compatibility aliases
or reset wrappers are provided.

| Command | Purpose |
| --- | --- |
| `./scripts/platform.sh up` | Build, publish locally, install/upgrade and forward the UI. |
| `./scripts/platform.sh build` | Build UI, broker and native demos without a cluster. |
| `./scripts/platform.sh build-ui` | Build and embed only the UI. |
| `./scripts/platform.sh build-demo native` | Build native demo images. |
| `./scripts/platform.sh build-demo multi` | Create ARM64/AMD64 demo OCI archives under `.local/images/`. |
| `./scripts/platform.sh tools` | Install pinned Helm under `.local/bin/`. |
| `./scripts/platform.sh --help` | List all local operational commands. |

`scripts/build-book.sh` builds the separate LaTeX thesis with Docker.
`scripts/lib/dev-common.sh` provides shared repository paths and pinned versions.

Verification is grouped under `tests/` and selected through
`python3 -B tests/run.py`; the [test guide](../tests/README.md) describes suites,
prerequisites and CI. The direct Helm baseline lives in `tests/integration/`.
Native host installation lives in `installer/install.sh`; OCI packaging and
manual release validation, packaging and publication live in `release/`. The
[release guide](../release/README.md) is the publication procedure. The [installation guide](../docs/installation.md)
describes release installation and the [contributor guide](../docs/contributor/README.md)
describes source development.
