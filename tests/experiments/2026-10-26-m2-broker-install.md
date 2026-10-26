# M2 — Manual platform installation and local verification

This note records the documented procedure for installing the broker on the local
K3s development cluster and exercising it, satisfying the M2 acceptance item for a
recorded manual platform installation and access procedure. It is a local
development procedure; reference-host (Raspberry Pi) and rented-server acceptance
remain M7 activities.

## Components built in M2

- `broker/` — the Go broker: management API, durable SQLite operation queue,
  single worker with freeze-before-mutation, startup recovery with
  `NeedsAttention` blocking, in-process Helm adapter, cluster observer, image
  inspection, and versioned immutable Kubernetes Secrets.
- `deploy/helm/platform/` — the platform chart: a single-replica `Recreate`
  Deployment with a metadata PVC, a private ClusterIP Service, a service account,
  and scoped RBAC (full workload CRUD in `platform-apps`, read-only cluster reads
  for capacity and volumes; no cluster-admin, no `kube-system` access).
- `broker/Dockerfile` — a multi-stage, non-root, distroless image built for
  `linux/arm64` and `linux/amd64`.

## Prerequisites

Docker running; k3d, kubectl, Go 1.25; the local toolchain from
[local-development.md](../../docs/local-development.md). Helm is used in-process by
the broker, so the Helm CLI is only needed for chart linting.

## Hermetic verification (no cluster)

```bash
./scripts/check-broker.sh      # gofmt, go vet, and the broker unit/handler tests
./scripts/check-api.sh         # OpenAPI contract validation
./scripts/check-chart.sh       # application + platform chart lint/render (needs the local Helm)
```

The broker test suite is fully hermetic: the store runs on temporary SQLite, the
Helm adapter installs against in-memory release storage with a fake Kubernetes
client, the cluster observer uses the client-go fake clientset, and image
inspection runs against an in-process OCI registry. No cluster is required.

## Manual broker installation on the local cluster

1. Start the local cluster and build the demonstration image:

   ```bash
   ./scripts/setup-dev-tools.sh
   ./scripts/dev-cluster.sh up
   ./scripts/build-demo.sh native
   ```

2. Build and import the broker image into the cluster:

   ```bash
   docker build -t platform-broker:dev broker
   k3d image import platform-broker:dev --cluster platform-dev
   ```

3. Create the platform namespace and install the chart. The broker deploys
   applications into `platform-apps` (created by `dev/namespace.yaml`) using its
   own service account; no administrator kubeconfig is mounted.

   ```bash
   kubectl --kubeconfig .local/kubeconfig create namespace platform-system
   .local/bin/helm --kubeconfig .local/kubeconfig install platform deploy/helm/platform \
     --namespace platform-system \
     --set image.repository=platform-broker --set image.tag=dev --set image.pullPolicy=Never \
     --wait --timeout 120s
   ```

4. Read the single-use bootstrap token from the broker logs, then reach the
   private API over an authenticated port-forward (management stays private; there
   is no LAN or public route before M3):

   ```bash
   kubectl --kubeconfig .local/kubeconfig -n platform-system logs deploy/platform | grep -A1 'Bootstrap token'
   kubectl --kubeconfig .local/kubeconfig -n platform-system port-forward svc/platform 8080:80 &
   curl -fsS http://127.0.0.1:8080/readyz
   ```

## Gated integration test

The real-cluster integration test drives the broker's Helm adapter and observer
against k3d. It is gated so the default `go test` stays hermetic:

```bash
./scripts/dev-cluster.sh up
./scripts/build-demo.sh native
k3d image import platform-demo:v1 --cluster platform-dev
KUBECONFIG=.local/kubeconfig PLATFORM_INTEGRATION=1 \
  go -C broker test -tags integration ./integration/... -run TestBrokerDeploysThroughHelm -v
```

The test installs the demonstration image through the broker's Helm adapter into
`platform-apps` and polls the cluster observer until the workload is Ready, then
uninstalls it. Supply `PLATFORM_DEMO_IMAGE` with the imported image's
digest-pinned reference, because the adapter deploys by digest.

## Recorded component versions

| Component | Version | Source |
| --- | --- | --- |
| Go toolchain | 1.25 | `broker/go.mod` |
| Helm SDK | v3.21.0 | `broker/go.mod` (Helm 3 storage format, same as the v3.22 CLI in `dev/versions.env`) |
| client-go / api / apimachinery | v0.33.4+ | `broker/go.mod` |
| go-containerregistry | v0.20.6 | `broker/go.mod` |
| modernc.org/sqlite | v1.39.0 | `broker/go.mod` (pure Go, no CGO) |

## Outstanding (not part of M2)

Administrator web UI (M3), failed-rollout restoration and explicit rollback (M4),
application persistence and backup (M5), public HTTPS (M6), installer automation
and release artifacts (M7), and the full observability profile (M8).
