# Platform chart

Installs the Go broker, embedded static UI, durable operation worker and status
observer as one workload in `platform-system`. Managed applications reside in
`platform-apps`. [Implementation status](../../../docs/status.md) records the
remaining correctness and product requirements.

## Resources and permissions

- A service account, namespaced application-management Role and RoleBinding.
  Secret access supports Helm release storage and immutable configuration Secrets.
- Read-only cluster access to nodes, PersistentVolumes and StorageClasses. These
  permissions do not imply that capacity/volume reporting is implemented.
- One broker replica with `Recreate` replacement and a metadata PVC. The container
  runs non-root with a read-only root filesystem, dropped capabilities and the
  runtime-default seccomp profile. Its service-account token enables in-cluster
  access; no administrator kubeconfig is mounted.
- A ClusterIP management Service, with SSH loopback access by default.
- Optional private LAN gateway and management Ingress. An explicit private IPv4
  bind address, mandatory source CIDRs, existing TLS Secret and `platform-private`
  IngressClass supply HTTPS; Secure cookies are enabled automatically. See the
  [LAN guide](../../../docs/private-lan.md) for DNS/trust, permissions, renewal and
  the published-release acceptance boundary.

The source private controller uses native Kubernetes load balancing: its upstream
is each Service's stable ClusterIP. Kubernetes handles Pod endpoint changes during
rollout; the proxy does not need a new backend-address configuration for every Pod
replacement. The application shutdown hook still permits endpoint convergence and
in-flight completion. Connection reuse can distribute traffic unevenly across
replicas; native Service routing does not promise fairness per HTTP request.
Published v0.1.11 used Pod-address routing and failed native rollout continuity.
The source correction requires its following manual release and Pi acceptance.

## Values

| Value | Behavior |
| --- | --- |
| `image.repository` | Broker image repository. |
| `image.tag` / `image.digest` | Supply one; published installations should use an immutable digest. |
| `image.pullPolicy` | `IfNotPresent` by default; `Never` is available for imported local images. |
| `appsNamespace` | Existing managed namespace; defaults to `platform-apps`. |
| `persistence.size` | Metadata volume request; defaults to 1 GiB. |
| `persistence.storageClassName` | Empty uses the cluster default, such as K3s local-path. |
| `resources` | Broker requests and limits; reference-host measurements remain outstanding. |
| `service.port` | Management Service port, 80 by default. |
| `secureCookies` | Enables Secure session cookies; private LAN mode always enables them. |
| `privateLAN.enabled` | Installs the private gateway, class and management route; false by default. |
| `privateLAN.address` / `httpsPort` | Explicit RFC1918 IPv4 bind address and port 1024–65535, default 8443. No public/IPv6 listener is installed. |
| `privateLAN.allowedCIDRs` | Required RFC1918 source networks, matched against the connection peer. Forwarded headers are not trusted. |
| `privateLAN.managementHostname` / `applicationDomain` | Separate management name and wildcard application domain. App hostnames must be one label beneath the domain. |
| `privateLAN.tlsSecretName` | Existing TLS Secret in the platform namespace; its key is mounted, never supplied in chart values. |
| `privateLAN.image` | Pinned Traefik v3.7.19 multi-architecture digest; first use pulls from Docker Hub. |
| `privateIngressEnabled` | Legacy operator-managed ingress capability flag; does not itself install HTTPS. |
| `insecureRegistries` | Explicit registry addresses using plain HTTP for local development. |
| `registryAliases` | Comma-separated inspection address rewrites, such as published identity to local registry container port 5000. |

## Installation

The supported local source workflow builds UI assets before the broker image and
supplies registry aliases automatically:

```bash
./scripts/platform.sh up
```

For direct chart installation, a compatible cluster, managed namespace, built
image and reachable registry are prerequisites. The following is a chart-level
example; variables must be set to an existing installation's values:

```bash
PLATFORM_CONFIG=/absolute/path/to/cluster.kubeconfig
BROKER_REPOSITORY=REGISTRY/REPOSITORY
BROKER_DIGEST=sha256:REPLACE_WITH_IMAGE_DIGEST
./scripts/platform.sh tools
kubectl --kubeconfig "$PLATFORM_CONFIG" apply -f dev/namespace.yaml
.local/bin/helm --kubeconfig "$PLATFORM_CONFIG" upgrade --install platform deploy/helm/platform \
  --namespace platform-system --create-namespace \
  --set-string "image.repository=$BROKER_REPOSITORY" \
  --set-string "image.digest=$BROKER_DIGEST" \
  --wait --timeout 180s
```

The shared namespace manifest assumes the pinned K3s/Pod Security version.
The [native installer](../../../docs/installation.md) applies it and installs
this chart from a verified native OCI bundle, with `image.pullPolicy=Never` for
the imported broker. Public routing and production network-policy acceptance
remain separate requirements.
A local HTTP registry also requires the documented alias/HTTP settings and node
registry configuration; the unified launcher supplies them for its own cluster.

On first start the broker prints a one-hour, single-use bootstrap token. The
[UI guide](../../../docs/user/README.md) separates account creation from login.
Upgrades retain the metadata PVC. The chart does not implement a retained-data
uninstall policy: the PVC is an ordinary Helm resource, and uninstall can delete
it and its backing data according to storage policy. Production backup/restore
and safe uninstall procedures remain planned; platform deletion and application
deletion are separate actions.
