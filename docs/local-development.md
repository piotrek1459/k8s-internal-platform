# Local development and verification

Updated: 2026-11-08. The Go broker, Svelte administrator UI and healthy local
application lifecycle are implemented. The [status report](status.md) distinguishes
verified behavior from recovery, UI integration, hardware and release work that
remains outstanding.

## Current platform workflow

The prerequisites and normal startup are defined in the [root README](../README.md)
and [contributor guide](contributor/README.md). From the repository root:

```bash
./scripts/platform.sh up
```

This creates or reuses `platform-local`, builds the embedded UI, broker and demo
images, publishes them locally, installs/upgrades the platform and starts a
foreground management forward. Initial setup is at `http://127.0.0.1:8080/setup`;
subsequent login uses `/login`. The default registry and Kubernetes API ports are
5002 and 6446. A separate application forward uses 8081.

The [UI guide](user/README.md) and [API guide](user/api-reference.md) contain the
supported Internal application workflow. Full rebuilds use the same `up` command.
Ctrl+C stops a forward. `platform.sh stop` retains installation data; it does not
perform a reset.

## Checks without a cluster

```bash
./scripts/platform.sh tools
./scripts/check-broker.sh
./scripts/check-ui.sh
./scripts/check-chart.sh
python3 -B -m unittest discover -s tests -p 'test_*.py' -v
(cd examples/http-demo && GOTOOLCHAIN=local go test -race ./... && GOTOOLCHAIN=local go vet ./...)
```

API validation requires PyYAML. Full OpenAPI validation additionally uses the
optional `openapi-spec-validator` dependency. The contributor guide documents an
isolated environment for those checks.

The broker suite uses temporary SQLite, fake Kubernetes clients, in-memory Helm
storage and an in-process registry. UI checks type-check, lint and build; they do
not drive a browser. The default CI does not run real-cluster integration. The
[test guide](../tests/README.md) records these distinctions.

## Real-cluster broker integration

The gated Go test drives the actual durable store, worker, Helm adapter and
Kubernetes observer. It deploys one application, reopens its temporary database
and verifies that the completed operation and application survive. It does not
exercise HTTP authentication or interruption during an unfinished upgrade.

The following example creates a separate disposable installation. The selected
ports must be unused. Settings apply to every command in this shell:

```bash
export PLATFORM_CLUSTER_NAME=platform-integration
export PLATFORM_REGISTRY_PORT=5003
export PLATFORM_API_PORT=6447
export PLATFORM_UI_PORT=18080
export PLATFORM_APP_PORT=18081
./scripts/platform.sh up --no-ui

KUBECONFIG="$PWD/.local/platform-integration.kubeconfig" \
  PLATFORM_INTEGRATION=1 \
  PLATFORM_DEMO_IMAGE="$(./scripts/platform.sh image v1)" \
  go -C broker test -tags integration ./integration/... -v
```

The test uses a digest reference published by the selected local registry. An
image's local Docker ID or a tag alone does not provide that registry identity.
A failed test can leave its release for inspection; the disposable installation
can then be removed explicitly. The following removes only this example's
installation and its data:

```bash
k3d registry delete k3d-platform-integration-registry
KUBECONFIG="$PWD/.local/platform-integration.kubeconfig" \
  k3d cluster delete platform-integration
rm -rf .local/platform-integration
rm -f .local/platform-integration.kubeconfig
unset PLATFORM_CLUSTER_NAME PLATFORM_REGISTRY_PORT PLATFORM_API_PORT PLATFORM_UI_PORT PLATFORM_APP_PORT
```

This procedure is separate from the latest healthy API lifecycle experiment,
which is described in its [historical report](../tests/experiments/2026-11-08-local-workflow.md).
That experiment is not yet a committed, automatically executed end-to-end suite.

## Manual Helm baseline

The baseline remains useful for comparison with the platform. It operates on the
disposable `platform-dev` cluster, the fixed demonstration release `demo`, and
`.local/kubeconfig`. It does not install the administrator platform.

```bash
./scripts/platform.sh tools
./scripts/platform.sh build-demo native
./scripts/dev-cluster.sh up
./scripts/test-local.sh
```

Native images are built locally, then imported by the experiment. `test-local.sh`
resets only the demonstration release in the owned baseline cluster. It verifies
healthy v1 installation, v2 replacement, deliberately unhealthy v3 readiness,
continued v2 traffic, explicit Helm rollback, a non-default application port and
private-entrypoint route isolation. This is manual Helm recovery evidence, not
proof of automatic broker restoration.

| Baseline address | Purpose |
| --- | --- |
| `127.0.0.1:6445` | Kubernetes API. |
| `127.0.0.1:8082` | Traefik private test entrypoint. |
| `127.0.0.1:8083` | Separate listener for the negative routing test. |

The baseline has a separate registry convention on port 5001. Application test
images are imported directly, and baseline addresses must not be substituted for
the current platform's registry or kubeconfig. The API and test HTTP listeners
use loopback; this experiment does not establish production LAN, TLS, firewall
or IPv6 isolation.

```bash
curl --noproxy '*' --resolve demo.localhost:8082:127.0.0.1   http://demo.localhost:8082/version
./scripts/dev-cluster.sh status
```

Successful completion leaves the baseline demo at v2 on container port 9090,
with the same external URL. The chart reference documents why both
`containerPort` and the process's `PORT` setting must change.

Baseline cleanup deletes its cluster and data:

```bash
./scripts/dev-cluster.sh down
```

## Multi-architecture artifacts and evidence

```bash
./scripts/platform.sh build-demo multi
```

This produces Linux ARM64/AMD64 OCI archives and metadata under `.local/images/`
using the named `platform-demo-builder` builder. It does not publish a release.
Cross-compilation and image packaging do not establish native execution on a Pi
or AMD64 server. The builder can be stopped with
`docker buildx stop platform-demo-builder`.

New baseline runs write transcripts, source archives, manifests, versions,
responses, timings and diagnostics under `tests/experiments/runs/`. Raw output is
ignored by Git and must be retained separately when used as thesis evidence.
Earlier raw runs and local installations were removed during cleanup; tracked
reports preserve their historical observations. Numerical targets, repetitions,
cache conditions and reference-host measurements remain required by the
[evaluation plan](milestones.md#tests-and-thesis-evaluation).
