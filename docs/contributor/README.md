# Contributor guide

Current capabilities and open correctness work are recorded in
[implementation status](../status.md). The detailed plans include requirements
that are not implemented; the user guides document the supported local path.

## One local entry point

`scripts/platform.sh` contains the shared build and local execution workflow.
Defaults match the working `platform-local` environment. The default Kubernetes
context is never switched. Cluster commands explicitly select
`.local/platform-local.kubeconfig` and `k3d-platform-local`.

```bash
./scripts/platform.sh build          # UI + broker + native demo images only
./scripts/platform.sh up             # build + registry + cluster install + UI forward
./scripts/platform.sh up --no-ui     # installation without a long-running forward
./scripts/platform.sh ui             # foreground UI forward
./scripts/platform.sh demo v2        # build/publish one demo version
./scripts/platform.sh image v2       # exact digest for UI/API input
./scripts/platform.sh status
./scripts/platform.sh stop           # retain cluster, registry and metadata
```

`build` needs Docker and Node/npm. `up` additionally needs k3d, kubectl, Python 3,
curl, tar and shasum. The pinned Helm version from `dev/versions.env` is installed
automatically. The Go compiler for runtime image builds runs inside Docker.

The UI build performs `npm ci`, builds static assets and copies them into
`broker/internal/httpapi/ui_dist/` before compiling the broker image. Generated
assets remain ignored by Git. The standalone broker Dockerfile expects those
assets to have been generated when a UI is required.

Advanced targets:

```bash
./scripts/platform.sh build-ui
./scripts/platform.sh build-demo native  # local images only; no registry required
./scripts/platform.sh build-demo multi   # ARM64/AMD64 OCI archives under .local/images/
./scripts/platform.sh tools
```

## Local configuration

| Environment variable | Default |
| --- | --- |
| `PLATFORM_CLUSTER_NAME` | `platform-local` |
| `PLATFORM_REGISTRY_PORT` | `5002` |
| `PLATFORM_API_PORT` | `6446` |
| `PLATFORM_UI_PORT` | `8080` |
| `PLATFORM_APP_PORT` | `8081` |

The registry name derives from the cluster name: `k3d-<cluster>-registry`.
Registry, API and forwarded listeners bind to `127.0.0.1`. Container workloads
use the registry's container address; Docker publishes through localhost. The
broker chart receives an explicit alias from the published registry identity to
the container's HTTP port 5000.

A separate environment uses a different cluster name and unused ports. Export
these settings in every terminal interacting with that environment:

```bash
export PLATFORM_CLUSTER_NAME=platform-experiment
export PLATFORM_REGISTRY_PORT=5003
export PLATFORM_API_PORT=6447
export PLATFORM_UI_PORT=18080
export PLATFORM_APP_PORT=18081
./scripts/platform.sh up
```

An existing cluster requires its matching repository kubeconfig and expected
local API endpoint. A mismatched registry setup is reported as an error; the
script does not silently recreate a cluster. The historical `platform-dev`
baseline is separate from the default platform installation.

## Checks

```bash
./scripts/check-broker.sh
./scripts/check-ui.sh
./scripts/check-api.sh
./scripts/check-chart.sh
python3 -B -m unittest discover -s tests -p 'test_*.py' -v
```

The Go suite uses temporary SQLite and fake/in-process adapters. UI checks run
Svelte type checking, ESLint and production bundling. API validation requires
PyYAML; `openapi-spec-validator` enables full validation. Install those optional
check dependencies in a virtual environment:

```bash
python3 -m venv .local/checks-venv
.local/checks-venv/bin/python -m pip install pyyaml openapi-spec-validator
PATH="$PWD/.local/checks-venv/bin:$PATH" ./scripts/check-api.sh
```

GitHub Actions currently runs the broker, UI and API/chart workflows. Python CLI
regressions additionally check command validation and registry-specific digest
selection without requiring a Docker daemon. Real-cluster integration remains a
separate acceptance step; fake adapters do not verify actual Pod replacement.

## Development structure

| Directory/package | Responsibility |
| --- | --- |
| `broker/internal/platform` | Domain types and adapter interfaces. |
| `applications` | Input policy and operation request construction. |
| `store` | SQLite transactions, revisions, operations and metadata. |
| `operations` | Queue worker, status observation and recovery. |
| `httpapi` | HTTP handlers, authentication middleware and embedded UI. |
| `helm`, `cluster`, `registry` | External system adapters. |
| `web/src` | Svelte application and API client. |
| `api/openapi.yaml` | API contract. |

A lifecycle change should be tested through request construction, persistence,
worker execution and chart mapping, in addition to focused unit tests. Browser
checks should follow UI links and submit the relevant forms. An accepted API
request must be polled to completion before its workload result is evaluated.

## Supporting scripts

Compatibility wrappers have been removed. Build, startup, forwarding and tool
installation use `platform.sh` directly. The
[script inventory](../../scripts/README.md) identifies the focused checks, thesis
build and manual Helm baseline, each of which performs separate work.

The [local development guide](../local-development.md) documents the gated broker
integration test and baseline commands. Historical reports identify their dated
scope; old raw output was removed during cleanup. New evaluation runs must retain
their own evidence.
