# Kubernetes internal application platform

A self-hosted application platform built with Go, Kubernetes/K3s, Helm and a
Svelte web interface. The platform and managed applications run in one cluster.
The broker stores application revisions and operations in SQLite on a persistent
volume; application changes are executed by its durable worker.
[Implementation status and next work](docs/status.md) separates verified behavior
from known defects and planned capabilities.

## Install a release on Linux

Release assets are created through **Actions → Release → Run workflow**. Select
`branch` (default `main`) and a version; the selected branch's HEAD is captured,
validated and built before its tag and GitHub release are published. The
[release guide](release/README.md) describes manual publication and the optional
prerelease setting. Normal releases are the default. Published assets
are listed on the [GitHub Releases page](https://github.com/piotrek1459/k8s-internal-platform/releases).

The [installation guide](docs/installation.md) describes ARM64/AMD64 bundles,
the dedicated Linux installer and initial SSH access. Its `v0.1.12` commands
are version examples; the downloaded guide matches the selected release.

Published v0.1.12 passed reference-Pi installation, trusted LAN UI/API, defaults,
lifecycle, repeated rollout continuity, steady load, unhealthy-update restoration
and same-version/reboot retention. Its private controller routes through stable
Kubernetes Service IPs. The [native report](tests/experiments/2026-11-27-v0.1.12-native.md)
retains exact source/image identities, release CI and raw observations.
The [preceding evaluation](tests/experiments/2026-11-26-core-evaluation.md) and
[routing study](tests/experiments/2026-11-27-rollout-routing.md) preserve calibration,
controls and the earlier failed v0.1.11 trace. The [human timed walkthrough](tests/evaluation/HUMAN.md)
is the final M4 requirement in [status](docs/status.md).

Private HTTPS requires the [configuration and trust setup](docs/private-lan.md).
Public hosting, application volumes, verified backup restoration and cross-version
upgrades retain separate acceptance boundaries.

## Local startup

Prerequisites: a running Linux Docker engine (Docker Desktop on macOS), k3d,
kubectl, Node.js 22 with npm, Python 3, curl, tar and shasum. Go is required for
host-side development checks; the broker is compiled inside Docker for startup.
The pinned Helm CLI is downloaded automatically when necessary.

From the repository root:

```bash
./scripts/platform.sh up
```

This command creates or starts `platform-local`, builds the UI, broker and demo
images, publishes to a local registry, installs/upgrades the platform, checks
readiness, and starts a foreground UI port-forward. Its output identifies the
appropriate browser URL:

- First installation: **http://127.0.0.1:8080/setup**.
- Existing administrator: **http://127.0.0.1:8080/login**.

Keep the forwarding terminal open. For first-time setup, retrieve the bootstrap
token in another terminal:

```bash
./scripts/platform.sh token
```

The token belongs in the setup form. The administrator password selected in that
form is used for subsequent sign-in. Ctrl+C stops the forward; cluster data
remains available.

The default environment uses registry port **5002** and Kubernetes API port
**6446**, matching the manually established `platform-local` installation. An
existing cluster is reused only when its repository kubeconfig selects the
expected local endpoint. Existing installations without matching registry
configuration are rejected with instructions rather than recreated.

## Build and operate

One script provides the local workflow:

| Command | Effect |
| --- | --- |
| `./scripts/platform.sh build` | Build the embedded UI, broker and v1/v2/v3 demo images; no cluster or registry required. |
| `./scripts/platform.sh up` | Build, publish, install/upgrade and forward the UI. |
| `./scripts/platform.sh up --no-ui` | Perform startup without occupying a forwarding terminal. |
| `./scripts/platform.sh ui` | Reconnect the UI forward after a broker restart. |
| `./scripts/platform.sh image v1` | Print the published demo's exact image reference for the UI/API. |
| `./scripts/platform.sh demo v2` | Rebuild and publish demo v2 after source changes. |
| `./scripts/platform.sh forward APP_ID` | Access an internal application at `http://127.0.0.1:8081`. |
| `./scripts/platform.sh status` | Inspect cluster, broker and managed workload state. |
| `./scripts/platform.sh logs` | Follow broker logs. |
| `./scripts/platform.sh stop` | Stop the local cluster and registry while retaining their data. |

The default `up` command includes the build. A separate `build` invocation is
useful for preparing images without installing anything. Registry uploads remain
on the local machine. The v3 demo deliberately fails readiness and is intended
for controlled failure experiments.

## UI and API guides

- [UI walkthrough](docs/user/README.md): setup, deployment, application access,
  v1-to-v2 updates, scaling, revision history and troubleshooting.
- [API walkthrough and endpoint reference](docs/user/api-reference.md): cookie
  authentication, executable curl examples, asynchronous operations, updates,
  rollback and deletion.
- [Contributor guide](docs/contributor/README.md): build targets, checks, local
  configuration and script responsibilities.
- [Script inventory](scripts/README.md): local operations, thesis building and test/release entry points.

## Verification

`python3 -B tests/run.py unit` selects all fast suites. Required checks run on
every PR. Real-cluster E2E workflows run at 02:00 UTC nightly, manually and before
release publication; they build the platform and verify API/browser lifecycles,
failure locks and packaged images. The [test guide](tests/README.md) explains
prerequisites, reports and known acceptance gaps.

## Current scope

The implementation provides a local HTTP application workflow, immutable image
references, operation tracking, revision history, restart and verified restoration.
Private registry passwords are stored in Kubernetes Secrets. The UI defaults to
Internal exposure; the Linux installer provides SSH/loopback management access.
[Implementation status](docs/status.md) records reference-host results and the
remaining recovery boundaries. Public HTTPS, application data volumes, verified
backup/restore and cross-version platform upgrades remain separate work.
The [audited implementation plan](docs/milestones.md) marks verified work and
separates remaining acceptance. Outstanding M0–M4 core work is tracked in
[issue #16](https://github.com/piotrek1459/k8s-internal-platform/issues/16); it is
not replaced by later milestones.

## Repository

| Directory | Purpose |
| --- | --- |
| `broker/` | Go API, authentication, durable worker, store, Helm and cluster adapters. |
| `web/` | Svelte administrator UI, embedded into the broker during build. |
| `api/` | OpenAPI contract. |
| `deploy/helm/` | Platform and managed application charts. |
| `docs/` | Design, user and contributor documentation. |
| `tests/` | Common runner, regressions, real-cluster/browser workflows and recorded experiments. |
| `installer/`, `release/` | Native host installer, OCI bundles and release metadata. |
| `examples/` | Healthy and deliberately unhealthy demonstration applications. |
| `scripts/` | Local build/startup, shared paths and thesis build. |
| `book/` | Engineering thesis LaTeX source. |
| `infra/`, `observability/` | Planned infrastructure and monitoring work. |
