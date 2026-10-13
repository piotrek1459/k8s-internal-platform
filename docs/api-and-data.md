# Application contract, data model, and API

Part of the [application platform plan](app-plan.md). This document defines the
contract between the administrator and the platform: what an application
specifies, how the platform persists it, and the HTTP API. System shape is in
[architecture.md](architecture.md); networking and secrets handling are in
[networking-and-security.md](networking-and-security.md); execution and recovery
are in [operations.md](operations.md#deployment-execution-recovery-and-reconciliation).

## Application contract

### What "give it an image" means

The shortest supported path should be:

1. Paste an image reference.
2. Confirm the suggested application name and listening port.
3. Supply required configuration, if any.
4. Choose an available visibility mode and click **Deploy**.

The platform remembers installation-level networking and registry settings.
It provides defaults for deployment resources and health checks. It cannot infer
every application's required secrets, database, data directory, or correct health
endpoint. Image metadata such as `EXPOSE` is only a hint, not proof that a service
is running or published. [Dockerfile reference](https://docs.docker.com/reference/dockerfile/#expose)

### Required and optional input

| Field | Default / behavior |
| --- | --- |
| Name | Suggest a name from the image; administrator can edit it. Validate uniqueness. |
| Image | Existing OCI image reference. Retain the supplied tag and resolved digest. |
| Registry credential | Optional for public images; select or create one for private images. |
| Container port | Suggest from image metadata where possible; require confirmation. |
| Workload profile | HTTP service first; private TCP service added with stateful support. |
| Exposure | Internal, LAN, or public, depending on installed capabilities. |
| Environment | Optional non-sensitive key/value settings. |
| Secrets | Write-only input or references to stored secret versions. |
| Replicas | One initially; manual changes for supported stateless apps. |
| Resources | A tested default request/limit preset, editable within host policy. |
| Readiness | Optional HTTP path; otherwise a documented TCP readiness fallback. |
| Startup check | Configurable startup allowance for applications that need it. |
| Liveness | Optional; enable only with a suitable health endpoint. |
| Storage | Off by default; one retained volume and mount path in the initial persistent profile. |

A compatible application must listen on `0.0.0.0` inside the container, support
the host's Linux CPU architecture, and get runtime settings from the configured
environment or files. It should log to stdout/stderr and handle termination.
Document supported non-root execution and volume permissions; show a useful
compatibility error for an image that needs unsupported privileges.

Inspect the registry manifest before deployment, including authentication and
architecture where possible. Deploy by digest so an existing revision does not
change when a tag moves. If the registry cannot be checked, show a retryable
validation error instead of claiming the image is compatible.
[Kubernetes images](https://kubernetes.io/docs/concepts/containers/images/)

Build platform and demonstration images for `linux/arm64` and `linux/amd64`.
The ARM64 variant covers the reference Pi; the AMD64 variant covers the rented
server target. Do not rely on transparent emulation on the Pi.
[Docker multi-platform builds](https://docs.docker.com/build/building/multi-platform/)

### Health and status

Keep these states separate in the API and UI:

- **Operation:** queued, running, recovering, succeeded, failed, or needs attention.
- **Workload:** deploying, updating, ready, degraded, stopped, failed, or unknown.
- **Exposure:** not requested, waiting for configuration, route ready, TLS ready,
  verified reachable, or verification failed.

A successful Helm command or a running pod is not sufficient proof that the
application can serve a request. Check desired versus ready replicas, rollout
generation, and service endpoints. Record observation time and make stale status
visible when the Kubernetes API cannot be reached.

Readiness controls whether a pod receives service traffic; liveness can restart
the container. A startup probe can protect slow startup from premature health
failures. A TCP readiness fallback proves less than an application-specific HTTP
check. [Kubernetes probe behavior](https://kubernetes.io/docs/tasks/configure-pod-container/configure-liveness-readiness-probes/)

### Scaling and persistent storage

For stateless applications, support manual replicas including zero. Zero means
**Stopped**; it is not automatic scale-to-zero or wake-on-request. Show requested
and ready replica counts and scheduling/resource errors. More replicas on one
host do not create additional hardware capacity or protect against host failure.

Applications sharing local files, in-memory sessions, or background task ownership
need an application-level scaling design. The UI must not imply that any image
can safely run with several copies.

For the first persistent profile:

- Allow replicas zero or one only, with `Recreate` updates and visible expected downtime.
- Support one PVC, one mount path, and the tested local storage class.
- Keep the PVC outside the workload Helm release and retain it on app deletion.
- Allow explicit reuse through `storage.existingVolumeId` when creating an app.
  Validate ownership, storage class, mount path, and that no other app uses it;
  never attach it to two app records silently.
- Display retained volumes and their association with deleted applications.
- Make volume deletion a separate operation, with an in-use check and explicit confirmation.
- Defer resizing, moving storage classes, and distributed storage.
- Explain that code rollback does not reverse file changes or database migrations.

`ReadWriteOnce` is a single-node access mode, not a guarantee of one pod writing.
Enforce the one-instance rule in the platform. Local-path data stays on its node;
it is not replicated storage. [Kubernetes persistent volumes](https://kubernetes.io/docs/concepts/storage/persistent-volumes/),
[K3s storage](https://docs.k3s.io/add-ons/storage)

The default local-path provisioner does not enforce the requested PVC size as a
capacity limit. Label requested capacity separately from measured usage and make
the shared disk limit visible. Hard per-app disk quotas are outside the first
release. [Local-path provisioner limitations](https://github.com/rancher/local-path-provisioner#cons)

If the future app needs PostgreSQL, introduce it as an explicitly configured
private dependency using the persistent profile or a documented external service.
Define its credentials, backups, and migration procedure. This does not require
building a general managed-database product.

## Platform data model and traceability

SQLite is the recommended first database because one broker instance serves one
administrator. Persist it on a volume, use schema migrations, transactions,
foreign keys, and a tested backup procedure. Scaling the broker itself requires
a new coordination/storage design; do not increase its replicas in this version.

| Entity | Important data |
| --- | --- |
| Administrator | ID, password hash, creation/change timestamps. |
| Session | Hashed session identifier, administrator ID, expiry/revocation data. |
| Machine token | Hash, app/action scopes, issuer, expiry/revocation metadata; added with API automation. |
| Application | Stable ID/name, optimistic version, effective desired revision, last successful revision, active operation, deletion state. |
| App revision | Requested candidate, then frozen execution spec: image reference/digest, chart version/artifact digest, secret versions, author/time, result. |
| Operation | App/type, source and target revision, state/stage, idempotency key and request hash, timestamps/deadline, recovery outcome. |
| Operation event | Ordered progress entries with sanitized diagnostics and request correlation. |
| Audit event | Actor, action, target, time, outcome, request/operation ID, safe configuration diff. |
| Registry credential metadata | Registry, label, credential Secret reference/version, verification status, usage. |
| Secret metadata | Owner, name/key metadata, immutable Kubernetes Secret reference/version; no plaintext value. |
| Volume | PVC identity, requested size, mount association, retained/orphaned state, deletion status. |
| Settings | Validated non-secret installation preferences and capability references. |

Audit data answers **who requested what and when**. Operation history records
what the platform did. Application logs show what the workload emitted. Metrics
measure behavior over time. Distributed traces are a separate optional capability;
a database audit trail does not require a tracing stack.

Store configuration snapshots and history in SQLite. Use Kubernetes and Helm to
observe what is actually running. Cached status must include `observedAt` and
must not be presented as fresh while the cluster is unavailable.

Retention should bound stored operation details and revision history. Preserve
versions needed for supported rollback and describe how long history remains
available. The local audit table provides traceability, not tamper-proof evidence
against the machine administrator.

## API contract

Use JSON over HTTP under `/api/v1`. Define the contract in `api/openapi.yaml`
before implementing each group of endpoints; generate frontend types if useful.
The following is a proposed contract, not an implemented interface.

### Endpoints

| Method and path | Purpose | Stage |
| --- | --- | --- |
| `GET /healthz` | Process health, without sensitive information. | First service |
| `GET /readyz` | Ability to serve the API; report cluster connectivity separately. | First service |
| `GET /api/v1/setup/status` | Whether initial admin setup is available. | Setup |
| `POST /api/v1/setup` | Consume the single-use bootstrap token and create the admin. | Setup |
| `POST /api/v1/auth/session` | Authenticate and establish a session. | Setup |
| `DELETE /api/v1/auth/session` | Revoke the current session. | Setup |
| `GET /api/v1/auth/me` | Current administrator/session summary. | Setup |
| `GET /api/v1/tokens` | Machine-token metadata and scopes; never token values. | Automation |
| `POST /api/v1/tokens` | Create an app-scoped automation token; show its value once. | Automation |
| `DELETE /api/v1/tokens/{id}` | Revoke an automation token. | Automation |
| `GET /api/v1/system` | Version, host architecture, capabilities, capacity, component health. | Core |
| `GET /api/v1/settings` | Supported non-secret platform settings. | Core |
| `PATCH /api/v1/settings` | Change supported preferences; host-level changes stay in installer tooling. | Later core |
| `POST /api/v1/images/inspect` | Check an image reference/credential; report architecture, digest, suggested ports. | Deployment |
| `GET /api/v1/apps` | Paginated applications with summarized status. | Deployment |
| `POST /api/v1/apps` | Persist application intent and queue installation. | Deployment |
| `GET /api/v1/apps/{id}` | Configuration, observed status, active operation, and version. | Deployment |
| `PATCH /api/v1/apps/{id}` | Image/configuration/resources/exposure/replica change. | Lifecycle |
| `POST /api/v1/apps/{id}/restart` | Queue a restart through a chart revision. | Lifecycle |
| `POST /api/v1/apps/{id}/reapply` | Explicitly restore managed resources from the recorded spec/digest after drift. | Lifecycle |
| `GET /api/v1/apps/{id}/revisions` | Revision history and rollback eligibility. | Lifecycle |
| `POST /api/v1/apps/{id}/rollback` | Queue restoration of a selected revision. | Lifecycle |
| `DELETE /api/v1/apps/{id}` | Queue workload deletion and retain data by default. | Lifecycle |
| `GET /api/v1/apps/{id}/logs` | Bounded recent logs, optionally a selected pod/previous container. | Diagnostics |
| `GET /api/v1/apps/{id}/events` | Recent cluster diagnostics and operation progress. | Diagnostics |
| `GET /api/v1/operations/{id}` | Durable progress, outcome, and recovery details. | Deployment |
| `GET /api/v1/audit-events` | Paginated administrator action history. | Traceability |
| `GET /api/v1/registry-credentials` | Credential metadata and usage, never passwords/tokens. | Private images |
| `POST /api/v1/registry-credentials` | Create a credential securely. | Private images |
| `PUT /api/v1/registry-credentials/{id}` | Rotate credential material with reference-aware behavior. | Private images |
| `DELETE /api/v1/registry-credentials/{id}` | Delete only when no active/retained dependency needs it. | Private images |
| `GET /api/v1/secrets` | Secret metadata; values are never returned. | Configuration |
| `POST /api/v1/secrets` | Create a versioned secret before referencing it in an app. | Configuration |
| `POST /api/v1/secrets/{id}/versions` | Add an immutable replacement version. | Configuration |
| `DELETE /api/v1/secrets/{id}` | Delete only after checking active and rollback references. | Configuration |
| `GET /api/v1/volumes` | Attached and retained data volumes. | Persistence |
| `DELETE /api/v1/volumes/{id}` | Explicit, tracked deletion of an unused retained volume. | Persistence |
| `GET /metrics` | Private platform metrics for the monitoring profile. | Observability |

Secrets created during a form that is later abandoned need visible unused status
and reference-aware cleanup. Do not hide plaintext inside an operation payload.
Do not add arbitrary exec, kubectl, manifest upload, or operation cancellation
endpoints to the first release.

### Request and response conventions

- Authenticate all management endpoints except minimal health/setup status and
  the credential-protected setup/session entry points.
- Use server-side sessions with HttpOnly cookies, SameSite policy, CSRF defenses,
  expiry, and revocation. Use Secure cookies on HTTPS. Plain HTTP management is
  restricted to local development or authenticated loopback bootstrap, never LAN login.
- Later automation uses scoped bearer tokens, checked through the same authorization
  rules; it must not reuse the administrator's browser password or session.
- Use `Idempotency-Key` on lifecycle mutations. The same key/body returns the
  original result; the same key with different input returns a conflict.
- Return an ETag for the application version; require `If-Match` for changes to
  an existing application, including rollback, restart, and deletion.
- Persist accepted changes before returning `202 Accepted` and an operation URL.
  The request's disconnection must not cancel an accepted deployment.
- Return `400` for malformed input, `401/403` for access errors, `404` for missing
  resources, `409` for conflicting active work, `412` for a stale version, and
  `422` for an unsupported specification. Use `503` for temporary unavailable
  dependencies before acceptance; record failures after acceptance on the operation.
- Bound request sizes, registry calls, log reads, query ranges, and list pagination.
- Return stable error codes, readable messages, affected fields, and request IDs.

### Example: create an application

The image, hostnames, IDs, and resource numbers below are illustrative. Resource
defaults must be measured on the reference host before being adopted.

```http
POST /api/v1/apps
Content-Type: application/json
Idempotency-Key: deploy-notes-001
```

```json
{
  "name": "notes",
  "image": "ghcr.io/example/notes:1.0.0",
  "registryCredentialId": "reg_01",
  "containerPort": 8080,
  "profile": "http",
  "replicas": 1,
  "environment": {"APP_ENV": "production"},
  "secretRefs": [
    {"environmentVariable": "APP_SECRET", "secretId": "sec_01", "version": 1}
  ],
  "resources": {
    "cpuRequest": "100m",
    "cpuLimit": "500m",
    "memoryRequest": "128Mi",
    "memoryLimit": "256Mi"
  },
  "health": {"readinessPath": "/readyz"},
  "exposure": {"mode": "lan", "hostname": "notes.pi.home.arpa"},
  "storage": null
}
```

```http
HTTP/1.1 202 Accepted
Location: /api/v1/operations/op_01
```

```json
{
  "applicationId": "app_01",
  "operationId": "op_01",
  "state": "queued"
}
```

The worker resolves and records the digest before changing the cluster. For public
images, `registryCredentialId` can be omitted. The effective image digest and chart
version become part of the immutable revision used for execution and later recovery.

Image inspection in the form is an advisory preview unless the client submits the
resolved `image@sha256:...` reference. Display the actual execution digest once the
worker freezes it. Only initial creation, a changed image selection, or an explicit
request to resolve that image again can select a new digest.

For persistent creation, `storage` takes either a new-volume specification such
as `{"size":"1Gi","mountPath":"/data"}` or a reuse specification such as
`{"existingVolumeId":"vol_01","mountPath":"/data"}`. The server rejects both
forms being supplied together. Attaching an existing volume requires the
single-instance persistent profile and an unused compatible retained volume.

### Example: scale or update through the same mutation path

```http
PATCH /api/v1/apps/app_01
Content-Type: application/json
If-Match: "app-3"
Idempotency-Key: scale-notes-002
```

```json
{"replicas": 2}
```

Changing `image` uses this same endpoint. Both actions create a revision and an
operation, pass through Helm, and update the configuration used by later upgrades.
Do not scale directly through Kubernetes while leaving stale replicas in Helm values.

### Example: observed state during an update

```json
{
  "id": "app_01",
  "version": 4,
  "desiredRevision": 4,
  "lastSuccessfulRevision": 3,
  "activeOperationId": "op_02",
  "status": {
    "phase": "updating",
    "desiredReplicas": 2,
    "readyReplicas": 1,
    "workloadReady": false,
    "routeReady": true,
    "tlsReady": true,
    "externalReachability": "not-verified",
    "observedAt": "2026-10-13T12:00:00Z"
  }
}
```

Example failure body: `{"error":{"code":"IMAGE_ARCH_UNSUPPORTED","message":
"This image has no linux/arm64 variant.","field":"image","requestId":"req_01"}}`.
