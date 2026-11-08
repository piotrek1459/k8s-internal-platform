# Script inventory

## Build and run

`scripts/platform.sh` is the local build and execution entry point. The former
compatibility wrappers have been removed; commands use this script directly.

| Command | Purpose |
| --- | --- |
| `./scripts/platform.sh up` | Build, publish locally, install/upgrade, check readiness and forward the UI. |
| `./scripts/platform.sh build` | Build the embedded UI, broker and native demos without a cluster. |
| `./scripts/platform.sh build-ui` | Build and embed only the UI. |
| `./scripts/platform.sh build-demo native` | Build native demo images without publishing. |
| `./scripts/platform.sh build-demo multi` | Create ARM64/AMD64 OCI archives and metadata under `.local/images/`. |
| `./scripts/platform.sh tools` | Install the pinned Helm CLI under `.local/bin/`. |
| `./scripts/platform.sh --help` | List forwarding, image, token, diagnostics and stop commands. |

Foreground forwarding stops with Ctrl+C. `stop` retains cluster and registry data;
there is no automatic reset or broad process-name termination.

## Verification and separate deliverables

| Script | Distinct responsibility |
| --- | --- |
| `check-broker.sh` | Broker formatting, vet and ordinary Go tests. |
| `check-ui.sh` | UI dependency installation, type checking, lint and production build. |
| `check-api.sh` | Structural API validation and optional full specification validation. |
| `check-chart.sh` | Chart lint/render, negative configuration cases and embedded chart synchronization. |
| `build-book.sh` | Thesis LaTeX/Docker build. |
| `dev-cluster.sh` | Disposable `platform-dev` cluster for the manual Helm baseline. |
| `test-local.sh` | Manual Helm lifecycle, diagnosed readiness failure, rollback and route-isolation experiment. |

The baseline scripts remain executable acceptance tooling; they do not build or
start the administrator platform. Their procedure is in
[local-development.md](../docs/local-development.md#manual-helm-baseline).

`lib/` contains shared paths, OpenAPI validation, transcript handling and experiment
assertions. The Python files are called by checks/tests and are not extra startup
entry points. The [contributor guide](../docs/contributor/README.md) describes the
current workflow; [implementation status](../docs/status.md) records its limits.
