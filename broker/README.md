# Platform broker

The Go broker: the management API, a durable SQLite operation queue, a single
worker, a status observer, and in-process Helm and Kubernetes integration. It runs
as one in-cluster workload installed by the [platform chart](../deploy/helm/platform).

## Layout

```text
cmd/platform/        executable entry point and composition root
internal/platform/   domain types, error codes, and port interfaces
internal/store/      SQLite schema, migrations, transactional persistence
internal/applications/ pure validation and lifecycle policy
internal/operations/ durable queue worker and startup recovery
internal/httpapi/    HTTP handlers, auth/CSRF, response envelope
internal/auth/       argon2id passwords, setup token, sessions
internal/cluster/    client-go observer, immutable Secrets, Helm action config
internal/helm/       embedded application chart and values mapping
internal/registry/   image inspection and architecture validation
internal/audit/      sanitized audit diffs
integration/         gated k3d integration suite (//go:build integration)
```

## Developing

```bash
./scripts/check-broker.sh    # gofmt, go vet, hermetic unit/handler tests
go -C broker test ./...      # the same test suite directly
```

The default test suite is hermetic (temporary SQLite, in-memory Helm storage,
client-go fake, in-process OCI registry). The real-cluster integration test is
gated behind the `integration` build tag and `PLATFORM_INTEGRATION=1`; see
[tests/experiments/2026-10-26-m2-broker-install.md](../tests/experiments/2026-10-26-m2-broker-install.md).

## Contract and rules

The API and data model are defined in [api-and-data.md](../docs/api-and-data.md)
and `api/openapi.yaml`; execution and recovery rules are in
[operations.md](../docs/operations.md). The administrator web UI (M3) and the
reliable-lifecycle operations — failed-rollout restoration, explicit rollback,
drift reapply (M4) — build on this backend.
