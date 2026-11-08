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

## Checks and acceptance

```bash
python3 -m venv .local/checks-venv
.local/checks-venv/bin/python -m pip install -r tests/requirements.txt
PATH="$PWD/.local/checks-venv/bin:$PATH" python3 -B tests/run.py unit
python3 -B tests/run.py unit --suite broker --suite python
```

Every PR runs uncached broker/demo Go regressions, UI checks, full OpenAPI
validation, shell/Python tests and the chart matrix. Go tests remain beside
packages; orchestration, helper regressions, integration scripts and browser
workflows are grouped under `tests/`. The [test guide](../../tests/README.md)
records dependencies, exact commands, workflow triggers and acceptance limits.

The full E2E suite builds an isolated platform and drives its API and browser,
checks real Pods and HTTP responses, tests release bundles on a second node,
and cleans owned resources. It runs at 02:00 UTC nightly, manually, and before
release publication. Documented status/restart gaps remain explicit acceptance
skips rather than silently passing checks. Scheduled/manual execution requires
the workflow to be merged into the default branch.

## Release distribution

`release/version.txt` identifies the source version. `release/package.py`
validates native OCI architecture/version metadata and creates the chart/image
bundle. `release/manifest.py` assembles source identity, compatibility metadata
and checksums. `.github/workflows/release.yml` gates publication on fast checks,
E2E and ARM64/AMD64 bundles, then publishes a preview from an immutable tag.
Published assets are not replaced; a new version is required for corrections.

The [native installation guide](../installation.md) describes `installer/install.sh`.
The initial release uses GitHub-hosted OCI bundles instead of requiring a public
container registry. Image digests remain immutable and are imported directly
into K3s. Native Pi acceptance and cross-version upgrade/recovery remain pending.

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
[script inventory](../../scripts/README.md) identifies local operations, thesis building and the separate test/release entry points.

The [local development guide](../local-development.md) documents the gated broker
integration test and baseline commands. Historical reports identify their dated
scope; old raw output was removed during cleanup. New evaluation runs must retain
their own evidence.
