# Platform broker

The implemented Go broker serves the management API and embedded administrator
UI, persists metadata and operations in SQLite, runs one serial worker and a
status observer, and integrates with Helm and Kubernetes in the same cluster.
The [platform chart](../deploy/helm/platform/README.md) installs it in
`platform-system` with scoped access to `platform-apps`.

The [status report](../docs/status.md) identifies incomplete UI/API integration,
restart, failed-rollout restoration, interrupted recovery and private images.
Existing endpoints and passing unit tests do not establish acceptance for those
paths.

## Layout

| Path | Responsibility |
| --- | --- |
| `cmd/platform/` | Executable and dependency wiring; startup recovery runs before serving. |
| `internal/platform/` | Domain types, errors and interfaces. |
| `internal/store/` | SQLite migrations and transactional persistence. |
| `internal/applications/` | Input validation, merging and lifecycle requests. |
| `internal/operations/` | Durable worker, observation and partial recovery. |
| `internal/httpapi/` | Handlers, authentication enforcement, response mapping and embedded UI. |
| `internal/auth/` | Argon2id passwords, bootstrap token and hashed sessions. |
| `internal/cluster/` | Kubernetes observer, immutable configuration Secrets and Helm configuration. |
| `internal/helm/` | Embedded application chart and values mapping. |
| `internal/registry/` | Image inspection and architecture checks; stored credential resolution is incomplete. |
| `internal/audit/` | Sanitized configuration differences. |
| `integration/` | Gated real-cluster deployment/completed-state persistence test. |

## Build and checks

Commands run from the repository root:

```bash
./scripts/platform.sh build
./tests/checks/broker.sh
go -C broker test ./...
```

The build command generates the UI before building the broker image. A direct
Go/Docker build without generated UI assets serves only the API. UI-only asset
preparation is available through `./scripts/platform.sh build-ui`.

The ordinary Go suite uses temporary SQLite, in-memory Helm storage, fake
Kubernetes clients and an in-process registry. It does not start a cluster. The
[local development guide](../docs/local-development.md#real-cluster-broker-integration)
defines execution with the `integration` build tag and `PLATFORM_INTEGRATION=1`.
The [test guide](../tests/README.md) distinguishes automated checks, procedures and
historical evidence.

## Contract and execution

The [API walkthrough](../docs/user/api-reference.md) documents current requests;
`api/openapi.yaml` is the schema contract. The
[data/API design](../docs/api-and-data.md) and
[execution requirements](../docs/operations.md#deployment-execution-recovery-and-reconciliation)
include planned behavior and explicitly identify current gaps. Application
metadata, accepted operations and audit records are persisted together before
202 is returned. Image/Secret references are frozen before mutation. No
transaction spans SQLite, Helm and Kubernetes; recovery must verify actual state.
