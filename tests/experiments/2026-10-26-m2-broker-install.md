# M2 backend implementation and installation snapshot — 2026-10-26

## Historical scope

This document records the M2 implementation snapshot and the installation/test
procedure documented at that stage. It is not evidence that every M2 acceptance
criterion, private-image deployment or interrupted recovery passed. The current
[status report](../../docs/status.md) identifies remaining work.

The original standalone installation commands have been superseded by the
[local development guide](../../docs/local-development.md) and
[platform chart reference](../../deploy/helm/platform/README.md). Those documents
use the current single entry point and registry identity. No historical cluster
is required to remain installed.

## Components introduced

- The Go broker: JSON API, SQLite metadata and operation queue, serial worker,
  image digest freezing, startup recovery classifications, workload observer,
  image inspector and versioned immutable configuration Secrets.
- The platform chart: one broker replica with `Recreate`, metadata PVC, ClusterIP
  Service, service account and scoped application-namespace permissions.
- A non-root, distroless broker Dockerfile. Current release publication and native
  ARM64/AMD64 host acceptance remain separate requirements.

## Verification method

The ordinary broker suite uses temporary SQLite, in-memory Helm storage, fake
Kubernetes clients and an in-process registry. Chart lint/render and API
validation are separate checks. This does not require a running cluster.

The gated integration test accepts an operation through the real store, executes
it through the worker/Helm adapter against K3s, observes readiness, then closes
and reopens the temporary database. It verifies that the completed operation and
application records survive. It does not inject a crash during an upgrade or
exercise browser authentication. The current command is documented under
[real-cluster integration](../../docs/local-development.md#real-cluster-broker-integration).

## Current follow-up and dependency sources

The UI and healthy lifecycle operations were subsequently implemented. Restart,
exact interrupted-target verification, automatic failed-rollout restoration and
private credential use remain incomplete. Application data, public HTTPS,
production installation/releases and monitoring remain planned.

Current dependency versions are recorded in `broker/go.mod`/`go.sum`, local tool
versions in `dev/versions.env`, and image builders in the Dockerfiles. They
supersede the early version table in this historical procedure. Later local
acceptance evidence is indexed in the [test guide](../README.md#historical-reports).
