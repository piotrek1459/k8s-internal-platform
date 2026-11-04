# k8s-internal-platform

An internal application platform built on Kubernetes. Administrators deploy and
operate containerised HTTP applications through a browser UI or REST API, without
writing Kubernetes YAML or running Helm commands directly.

Built as an engineering thesis project at Silesian University of Technology.

## What is built

Milestones M0–M4 are complete:

| Milestone | Outcome |
|---|---|
| M0 | K3s baseline, application Helm chart, local lifecycle test |
| M1 | Multi-architecture demo image, ARM64/AMD64 CI artifacts |
| M2 | Go broker: durable operation queue, SQLite, in-cluster Helm, auth, image inspection, secrets |
| M3 | Embedded Svelte 5 SPA: six screens, wizard deploy flow, operation progress |
| M4 | Full lifecycle API: update, restart, reapply, rollback, delete; revision history; operation page |

The platform runs as a single binary inside the cluster. It embeds the admin UI
as static assets, manages a SQLite database on a PVC, and drives Helm to install
and update applications in a dedicated namespace.

## Repository layout

```
api/                OpenAPI specification (api/openapi.yaml)
book/               LaTeX thesis document
broker/             Go broker source
  cmd/platform/     main binary
  internal/
    applications/   deployment policy (validation, factory methods)
    auth/           session authentication
    cluster/        Kubernetes client wrapper
    helm/           Helm adapter
    httpapi/        HTTP handlers and router
    operations/     worker, observer, recovery
    platform/       domain types and port interfaces
    registry/       image inspection
    store/          SQLite persistence
deploy/helm/
  application/      Helm chart for managed applications
  platform/         Helm chart for the broker itself
docs/               Design documents and guides
  user/             User-facing guides
  contributor/      Contributor and development guides
examples/           Demo application source
infra/              Terraform (planned)
observability/      Monitoring and logging (planned)
scripts/            Helper scripts
tests/              Test scenarios and experiment evidence
web/                Svelte 5 SPA source
```

## Prerequisites

| Tool | Purpose |
|---|---|
| Docker Desktop | Container builds and k3d nodes |
| k3d ≥ 5.8 | Local Kubernetes cluster |
| kubectl | Cluster commands |
| Helm ≥ 3.17 | Chart operations |
| Go ≥ 1.25 | Broker development |
| Node.js 22 + npm | UI development |
| Python 3 | OpenAPI validation script |

Install dev tools (Helm, pinned versions):

```bash
./scripts/setup-dev-tools.sh
```

## Quick start

```bash
# 1. Start the local cluster, build everything, deploy the broker
./scripts/dev-env.sh up

# 2. Open the UI
open http://127.0.0.1:8080

# 3. Complete one-time setup using the bootstrap token printed by the script
#    (or retrieve it later):
./scripts/dev-env.sh token
```

Follow the setup wizard in the browser to create an admin password, then log in.

To deploy the bundled demo application, get its image reference:

```bash
docker image inspect --format '{{index .RepoDigests 0}}' localhost:5001/platform-demo:v1 \
  | sed 's|localhost:5001|k3d-platform-registry:5001|'
```

Paste the resulting digest reference into the **Image reference** field in the
New Application wizard. Use container port `8080` and profile `http`.

Tear everything down:

```bash
./scripts/dev-env.sh down
```

## Documentation

- [User guide](docs/user/README.md) — UI walkthrough, application lifecycle, operations
- [API reference](docs/user/api-reference.md) — all endpoints, request/response shapes
- [Contributor guide](docs/contributor/README.md) — development setup, checks, CI
- [Architecture](docs/architecture.md) — system design and component layout
- [Local development (M0–M1)](docs/local-development.md) — baseline K3s workflow
