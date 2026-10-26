# Application Helm chart

This chart implements the M1 stateless HTTP workload profile. Each release
contains a Deployment, a ClusterIP Service, and an optional private Ingress.
Persistent volumes, public routes, arbitrary manifests, and secret creation are
outside this version's scope. The chart version is independent of the application
image version.

## Prerequisites and identity

The target namespace must already exist. The platform uses `platform-apps`;
isolated test namespaces are also permitted. Installation does not create a
namespace, ingress controller, registry credential, or application secret.

`appId` is required and remains unchanged for the lifetime of a release. It is
recorded as `platform.local/app-id` on every rendered resource and the pod
template. The Helm release name determines the resource names. Deployment and
Service selectors contain both the application ID and release name, preventing
selection of another release's pods. Changing the application ID is a new
application operation; Kubernetes rejects changes to an existing Deployment's
immutable selector.

The chart uses stable Kubernetes APIs and declares Kubernetes 1.30 or later as
its API compatibility floor. This range is not evidence of testing on every
release. The local development scripts pin the K3s and Helm versions used for
integration verification.

## Local installation

The local image must be imported into the test cluster before installation. The
example values select `platform-demo:v1`, disable registry pulls, and route
`demo.localhost` through the `private` Traefik entrypoint. The repository's local
development workflow supplies the cluster and listener configuration.

From the repository root, chart validation and installation use:

```sh
helm lint deploy/helm/application --strict \
  --values deploy/helm/application/examples/local.yaml

helm template demo deploy/helm/application \
  --namespace platform-apps \
  --values deploy/helm/application/examples/local.yaml \
  | kubectl apply --dry-run=server -f -

helm upgrade --install demo deploy/helm/application \
  --namespace platform-apps \
  --values deploy/helm/application/examples/local.yaml \
  --wait --timeout 180s
```

The initial values are intentionally incomplete: an application ID, image
repository, and tag or digest must be supplied. Consequently, validation without
an application-specific values file is expected to fail.

## Values contract

| Value | Behavior |
| --- | --- |
| `appId` | Required stable Kubernetes label value, up to 63 characters. |
| `image.repository` | Required image repository without a tag or digest. |
| `image.tag`, `image.digest` | Exactly one is required. Digests use `sha256:` followed by 64 lowercase hexadecimal characters. |
| `image.pullPolicy` | `IfNotPresent` by default; local examples use `Never`. |
| `replicas` | Non-negative integer; defaults to one. Zero stops the workload while retaining the Service and route. |
| `containerPort` | Integer in the range 1–65535; defaults to 8080. Must match the application's listener; does not configure the process. The Service exposes port 80. |
| `env` | Map of non-sensitive environment names to string values. |
| `secretRefs` | Existing secret references of the form `{name, secretName, key}`. The name is the environment variable name. |
| `imagePullSecrets` | Existing registry secret references of the form `{name}`. |
| `resources.requests` | Provisional defaults: `cpu: 25m`, `memory: 32Mi`. |
| `resources.limits` | Provisional defaults: `cpu: 250m`, `memory: 128Mi`. |
| `probes.readiness` | Required enabled readiness check; defaults to HTTP `/readyz`. |
| `probes.startup` | Enabled by default; HTTP `/healthz`, two-second period, 30 failures allowed. |
| `probes.liveness` | Disabled by default; HTTP `/healthz` when enabled. |
| `ingress.enabled` | Disabled by default. Without ingress, only the ClusterIP Service is created. |
| `ingress.hostname` | Required DNS hostname when ingress is enabled. Wildcards are unsupported. |
| `ingress.className` | Fixed to `traefik`. |
| `ingress.entrypoint` | Fixed to `private`; default public listeners cannot be selected. |
| `ingress.tls` | Optional existing TLS secret; `enabled: true` requires `secretName`. |

Each probe accepts `type: http` or `type: tcp`, `path`, `initialDelaySeconds`,
`periodSeconds`, `timeoutSeconds`, and `failureThreshold`. An HTTP probe requires
a path beginning with `/`. A TCP probe permits an omitted path, ignores any supplied
path, and verifies only that the port accepts connections. An application without
an HTTP health endpoint requires both readiness and startup to use TCP, or startup
to be disabled. Optional liveness requires a suitable application endpoint.

The values schema rejects unknown properties, invalid selectors and ports,
negative replicas, disabled readiness, and public ingress entrypoints. Duplicate
environment names across `env` and `secretRefs` are rejected during rendering.
Kubernetes server-side validation remains required for cross-field constraints
such as resource requests exceeding limits.

Resource defaults are demonstration settings rather than a measured Raspberry Pi
preset. Reference-host measurements and suitable capacity for a surge pod remain
M0/M1 completion requirements.

## Image selection and configuration

`containerPort` declares the named `http` port used by the Service and probes.
The application process must independently listen on that same port. The chart
does not inject a `PORT` variable because container images use different
configuration mechanisms.

The demonstration image recognises `PORT`. A deployment on port 9090 therefore
requires both values:

```yaml
containerPort: 9090
env:
  PORT: "9090"
```

The [local port overlay](examples/local-port-9090.yaml) supplies this pair and is
applied after `examples/local.yaml`. The local lifecycle test verifies this
configuration through the Service, probes, and private Ingress. Changing only
`containerPort` leaves the demonstration process on 8080 and causes probe failures.
Other images require their own documented listener configuration.

Tagged images support the local M0/M1 experiment. A broker-managed revision must
use a resolved immutable digest, with `image.tag` empty. For example:

```yaml
appId: app_01
image:
  repository: registry.example.com/team/application
  tag: ""
  digest: sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
```

The digest above is illustrative. Image existence, architecture, registry access,
and compatibility are verified outside chart rendering. Both local demonstration
architectures are built by the example workflow; the chart itself does not select
an architecture.

Secret references resolve in the release namespace and are required at runtime.
Plaintext secrets must not be supplied through `env` or other values because Helm
stores release values. Immutable secret versions are provisioned outside this
chart, and the M2 broker will retain their references in application revisions.
The chart does not verify secret immutability. A reference example is:

```yaml
env:
  APP_ENV: development
secretRefs:
  - name: APP_SECRET
    secretName: app-01-secret-v1
    key: value
imagePullSecrets:
  - name: registry-credentials
```

## Rollout and execution constraints

Rolling updates use `maxSurge: 1` and `maxUnavailable: 0`. During an unhealthy
upgrade, a healthy old replica remains eligible for traffic while the new pod
fails readiness. No automatic rollback is configured by the chart; a timed-out
Helm operation requires recorded diagnostics and an explicit recovery action.
The Deployment progress deadline is 180 seconds. Probe allowances must remain
compatible with this deadline. Node failure, resource exhaustion, and an unhealthy
old application can still interrupt availability.

Pods run with `runAsNonRoot`, a read-only root filesystem, no added Linux
capabilities, no privilege escalation, and the runtime-default seccomp profile.
Service account token mounting and automatic Service environment variables are
disabled. The image must declare a numeric non-root `USER`, listen on the
configured port and `0.0.0.0`, and function without writable root filesystem
directories. This initial profile provides no writable volume. The demonstration
image satisfies these requirements.

Private entrypoint selection is explicit but does not establish a network access
boundary by itself. The installation must restrict the private listener through
bind addresses, port mappings, or firewall rules. The local test listener is
restricted to host loopback. A forged private Host header on the default public
listener must not reach the application. TLS references are supported, but
certificate issuance and trust configuration are installation responsibilities.

Relevant semantics are documented in the [Kubernetes Deployment reference](https://kubernetes.io/docs/concepts/workloads/controllers/deployment/),
[Helm values schema reference](https://helm.sh/docs/topics/charts/#schema-files),
and [Traefik Kubernetes Ingress reference](https://doc.traefik.io/traefik/reference/routing-configuration/kubernetes/ingress/).
