# Local M0–M1 development

The initial implementation comprises a versioned HTTP demonstration application,
an application Helm chart, and a local K3s lifecycle experiment. The local target
was selected for initial development; container images remain local. The broker,
administrator UI, and production installer are subsequent milestones.

## Prerequisites and versions

The workflow requires Docker with Buildx, k3d, kubectl, Bash, Python 3, curl, tar,
and shasum. Docker must be running. Go 1.25 or later is required for direct host
unit tests; container builds use the pinned Go 1.27.2 builder image.

Helm is downloaded from its official distribution endpoint, verified against its
published SHA-256 checksum, and installed under `.local/bin/helm`. The tested Helm
and K3s versions are recorded in [dev/versions.env](../dev/versions.env). The
demonstration [Dockerfile](../examples/http-demo/Dockerfile) pins the builder image
by digest. k3d 5.8.3 and kubectl 1.35.0 were used for the initial verification.

Version sources: [Go releases](https://go.dev/dl/),
[Helm releases](https://github.com/helm/helm/releases), and
[K3s releases](https://github.com/k3s-io/k3s/releases).

## Initial execution

The following commands are executed from the repository root:

```bash
./scripts/setup-dev-tools.sh
./scripts/build-demo.sh native
./scripts/dev-cluster.sh up
./scripts/test-local.sh
```

The cluster contains one K3s server and uses the repository-local kubeconfig
`.local/kubeconfig`. Commands explicitly select this cluster; the default
kubeconfig and current context remain unchanged. Creation refuses to reuse an
unrecognized cluster with the same name. Kubernetes Secret encryption is enabled.

The three local images are imported into the cluster without a registry upload:

| Image | Application version | Readiness | Liveness/startup |
| --- | --- | --- | --- |
| `platform-demo:v1` | `v1` | HTTP 200 | HTTP 200 |
| `platform-demo:v2` | `v2` | HTTP 200 | HTTP 200 |
| `platform-demo:v3` | `v3` | HTTP 503 | HTTP 200 |

The test resets only the `demo` Helm release in the owned local cluster, then
verifies installation, update, failed readiness, continued service from v2,
explicit Helm rollback, deployment on a non-default port, and private-route
isolation. Rollback assertions require a newer deployed Helm revision, a
converged v2 Deployment, a scaled-down failed ReplicaSet, and removal of v3 Pods.
Retained ReplicaSet history is permitted. The expected v3 timeout is an asserted
failure scenario. The final upgrade sets both `containerPort: 9090` and
`env.PORT: "9090"`; successful completion leaves v2 running on container port 9090.
The external URL remains unchanged because the Service and Ingress retain their
existing ports.

The application is accessible at `http://demo.localhost:8082/version`. An explicit
loopback mapping avoids dependence on local resolver behavior:

```bash
curl --noproxy '*' --resolve demo.localhost:8082:127.0.0.1 \
  http://demo.localhost:8082/version
```

## Local networking boundary

| Host address | Function |
| --- | --- |
| `127.0.0.1:6445` | Kubernetes API |
| `127.0.0.1:8082` | Traefik `private` entrypoint |
| `127.0.0.1:8083` | Traefik `web` entrypoint, retained for the negative routing test |

All published ports bind to IPv4 loopback. The development chart explicitly
selects the `private` entrypoint. A request carrying `Host: demo.localhost` on
port 8083 must return HTTP 404. This experiment verifies local entrypoint
selection; it does not establish production firewall, public TLS, IPv6, or LAN
isolation. Those require separate host/network acceptance tests.

The HTTP endpoint serves the demonstration workload only. It is not a management
login endpoint. Normal LAN administration requires the HTTPS and authentication
controls defined for M3.

## Verification commands

```bash
# Demonstration endpoint and configuration tests.
(cd examples/http-demo && go test -race ./... && go vet ./...)

# Chart rendering and rejection of unsupported configurations.
./scripts/check-chart.sh

# Rollback assertion and transcript completion regressions, without a cluster.
python3 -B -m unittest discover -s tests -p 'test_*.py' -v

# Actual K3s lifecycle and private-route checks.
./scripts/test-local.sh

# Current local workload state.
./scripts/dev-cluster.sh status
```

The chart supports non-sensitive environment settings, references to existing
Secrets, optional image-pull credentials, HTTP/TCP probes, replica counts, and
an optional private Ingress. The [chart reference](../deploy/helm/application/README.md)
defines the complete values contract. Resource settings and the local namespace
quota are provisional demonstration values; Raspberry Pi measurements remain
required before production defaults are selected.

## ARM64 and AMD64 build artifacts

```bash
./scripts/build-demo.sh multi
```

This command creates local OCI archives under `.local/images/` for all three
versions. Each archive contains Linux ARM64 and AMD64 images. The named
`platform-demo-builder` Buildx builder is used without changing the selected
builder. Native Docker images used by the local cluster remain separate from
these archives. Build metadata is retained alongside the artifacts. No external
registry publication occurs.

The builder can be stopped after archive generation:

```bash
docker buildx stop platform-demo-builder
```

An AMD64 build establishes compilation and image packaging; execution on a native
AMD64 host remains a separate portability test.

## Experiment evidence

Each lifecycle run creates `tests/experiments/runs/<UTC timestamp>-<process ID>/`.
The output includes command transcripts, exact source archives, worktree status,
component versions, image IDs, rendered manifests, failure diagnostics, Helm
history, responses, durations, and resource snapshots. Rollback state snapshots
and the final port-9090 Deployment are retained separately. The script drains and
waits for the transcript writer before recording its final exit status. A logger
failure fails an otherwise successful run; an existing test failure retains its
original exit status. These files are ignored by Git. The kubeconfig and Secret
values are excluded from the collected evidence.

The recorded durations describe the local scenario with imported images. They
exclude external image pulls and do not constitute a statistical performance
evaluation. Host measurements and the required repetitions are defined in the
[evaluation plan](milestones.md#tests-and-thesis-evaluation).
The [initial verification report](../tests/experiments/2026-10-14-local-m0-m1.md)
records the observed results and outstanding host-specific acceptance work.
The [PR #6 verification report](../tests/experiments/2026-10-26-pr6-review-fixes.md)
records the stronger rollback assertions, non-default port deployment, and
transcript completion checks added during review.

## Cleanup

```bash
./scripts/dev-cluster.sh down
```

Cleanup removes the owned local K3s cluster and its local kubeconfig. Images,
build caches, experiment output, and downloaded Helm remain available for reuse.
Other clusters and the default Kubernetes context are preserved.

Raspberry Pi installation, access from a second LAN device, hardware resource
measurements, and external image publication remain outside this local run.
