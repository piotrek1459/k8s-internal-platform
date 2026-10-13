# Application contract, data model, and API

Part of the [application platform plan](app-plan.md). This document defines the
contract between the administrator and the platform: application specifications,
persistence, and the HTTP API. System architecture is described in
[architecture.md](architecture.md); networking and secrets handling are in
[networking-and-security.md](networking-and-security.md); execution and recovery
are in [operations.md](operations.md#deployment-execution-recovery-and-reconciliation).

## Application contract

### Minimum deployment workflow

The minimum supported workflow consists of:

1. Entry of an image reference.
2. Confirmation of the suggested application name and listening port.
3. Provision of required configuration, where applicable.
4. Selection of an available visibility mode and submission of the deployment.

The platform retains installation-level networking and registry settings.
It provides defaults for deployment resources and health checks. It cannot infer
every application's required secrets, database, data directory, or correct health
endpoint. Image metadata such as `EXPOSE` is only a hint, not proof that a service
is running or published. [Dockerfile reference](https://docs.docker.com/reference/dockerfile/#expose)

### Required and optional input

| Field | Default / behavior |
| --- | --- |
| Name | Editable suggestion derived from the image; uniqueness is validated. |
| Image | Existing OCI image reference; the supplied reference and resolved digest are retained. |
| Registry credential | Optional for public images; selection or creation is required for private images. |
| Container port | Suggestion derived from image metadata where available; confirmation is required. |
| Workload profile | HTTP service first; private TCP service added with stateful support. |
| Exposure | Internal, LAN, or public, depending on installed capabilities. |
| Environment | Optional non-sensitive key/value settings. |
| Secrets | Write-only input or references to stored secret versions. |
| Replicas | One initially; manual changes for supported stateless applications. |
| Resources | A tested default request/limit preset, editable within host policy. |
| Readiness | Optional HTTP path; otherwise a documented TCP readiness fallback. |
| Startup check | Configurable startup allowance for applications that need it. |
| Liveness | Optional; activation requires a suitable health endpoint. |
| Storage | Off by default; one retained volume and mount path in the initial persistent profile. |

A compatible application must listen on `0.0.0.0` inside the container, support
the host's Linux CPU architecture, and obtain runtime settings from the configured
environment or files. It should log to stdout/stderr and handle termination.
Supported non-root execution and volume permissions must be documented. Images
requiring unsupported privileges must produce an actionable compatibility error.

Registry manifest inspection must precede deployment, including authentication
and architecture validation where possible. Deployment uses an immutable digest
so an existing revision does not change when a tag is reassigned. Unavailable
registry inspection produces a retryable validation error; compatibility remains
unverified until inspection succeeds.
[Kubernetes images](https://kubernetes.io/docs/concepts/containers/images/)

Platform and demonstration images must support `linux/arm64` and `linux/amd64`.
The ARM64 variant covers the reference Pi; the AMD64 variant covers the rented
server target. The Pi deployment requires native ARM64 images.
[Docker multi-platform builds](https://docs.docker.com/build/building/multi-platform/)

### Health and status

The API and UI distinguish the following state categories:

- **Operation:** queued, running, recovering, succeeded, failed, or needs attention
  (`NeedsAttention` in the state model; `needs_attention` in JSON).
- **Workload:** deploying, updating, ready, degraded, stopped, failed, or unknown.
- **Exposure:** not requested, waiting for configuration, route ready, TLS ready,
  verified reachable, or verification failed.

A successful Helm command or a running pod is not sufficient proof that the
application can serve a request. Readiness evaluation includes desired versus
ready replicas, rollout generation, and service endpoints. Status includes an
observation timestamp and an explicit stale indication when the Kubernetes API
cannot be reached.

`NeedsAttention` is a durable, nonterminal operation state for unverified recovery
or ambiguous cluster state. The application retains its active operation reference,
and conflicting mutations return `409` until a documented recovery procedure
verifies the resulting cluster state. A failed deployment attempt remains recorded
as a failed revision even while its operation requires recovery. The same rule
applies to failed compensation during normal execution and to recovery after a
broker restart.

Readiness controls whether a pod receives service traffic; liveness can restart
the container. A startup probe can protect slow startup from premature health
failures. A TCP readiness fallback proves less than an application-specific HTTP
check. [Kubernetes probe behavior](https://kubernetes.io/docs/tasks/configure-pod-container/configure-liveness-readiness-probes/)

### Scaling and persistent storage

Stateless applications support manual replica changes, including zero. Zero means
**Stopped**; automatic scale-to-zero and wake-on-request are outside this scope.
The UI displays requested and ready replica counts and scheduling/resource errors.
Additional replicas on a single host do not increase hardware capacity or
provide protection against host failure.

Applications sharing local files, in-memory sessions, or background task ownership
need an application-level scaling design. The UI must not imply that any image
can safely run with several copies.

The first persistent profile has the following requirements:

- Replica counts are restricted to zero or one, with `Recreate` updates and a
  visible indication of expected downtime.
- One PVC, one mount path, and the tested local storage class are supported.
- The PVC remains outside the workload Helm release and is retained on application deletion.
- Explicit reuse is available through `storage.existingVolumeId` during application
  creation. Validation covers ownership, storage class, mount path, and absence of
  an existing application association; concurrent attachment to two application
  records is prohibited.
- Retained volumes and their association with deleted applications remain visible.
- Volume deletion is a separate operation requiring an in-use check and explicit confirmation.
- Resizing, storage-class migration, and distributed storage are deferred.
- Rollback documentation states that code rollback does not reverse file changes
  or database migrations.

`ReadWriteOnce` is a single-node access mode, not a guarantee of one pod writing.
The platform enforces the one-instance rule. Local-path data remains on its node;
it is not replicated storage. [Kubernetes persistent volumes](https://kubernetes.io/docs/concepts/storage/persistent-volumes/),
[K3s storage](https://docs.k3s.io/add-ons/storage)

The default local-path provisioner does not enforce the requested PVC size as a
capacity limit. Requested capacity and measured usage must be distinguished, with
the shared disk limit visible. Hard per-application disk quotas are outside the first
release. [Local-path provisioner limitations](https://github.com/rancher/local-path-provisioner#cons)

If a future application requires PostgreSQL, it is introduced as an explicitly configured
private dependency using the persistent profile or a documented external service.
Its credentials, backups, and migration procedure must be defined. This does not require
building a general managed-database product.

## Platform data model and traceability

SQLite is the recommended first database because one broker instance serves one
administrator. Its database must reside on a persistent volume, with schema
migrations, transactions, foreign keys, and a tested backup procedure. The broker
is restricted to one replica; additional replicas require a revised coordination
and storage design.

| Entity | Important data |
| --- | --- |
| Administrator | ID, password hash, creation/change timestamps. |
| Session | Hashed session identifier, administrator ID, expiry/revocation data. |
| Machine token | Hash, app/action scopes, issuer, expiry/revocation metadata; added with API automation. |
| Application | Stable ID/name, optimistic version, effective desired revision, last successful revision, durable active operation reference, deletion state. |
| Application revision | Requested candidate, then frozen execution specification: image reference/digest, chart version/artifact digest, secret versions, actor/time, result. |
| Operation | Application/type, source and target revision, durable state/stage, idempotency key and request hash, timestamps/deadline, recovery outcome. |
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

Configuration snapshots and history are stored in SQLite. Kubernetes and Helm
provide the observed runtime state. Cached status must include `observedAt` and
must not be presented as fresh while the cluster is unavailable.

Retention bounds apply to stored operation details and revision history. Versions
required for supported rollback remain available, and the retention period must
be documented. Nonterminal operations and their recovery dependencies must not be
removed by retention. The local audit table provides traceability; it is not
tamper-proof against the machine administrator.

Versioned immutable application secrets and their metadata APIs belong to M2,
before private-image deployment and configuration rollback in M3/M4. Application
volumes belong to M5. The broker's own persistent SQLite volume belongs to M2 and
is independent of optional application-volume support.

## API contract

The API uses JSON over HTTP under `/api/v1`. The contract is defined in
`api/openapi.yaml` before implementation of each endpoint group. Frontend types
may be generated from this contract.
The following is a proposed contract, not an implemented interface.

### Endpoints

| Method and path | Purpose | Stage |
| --- | --- | --- |
| `GET /healthz` | Process health, without sensitive information. | First service |
| `GET /readyz` | API readiness, with cluster connectivity reported separately. | First service |
| `GET /api/v1/setup/status` | Availability of initial administrator setup. | Setup |
| `POST /api/v1/setup` | Administrator creation using the single-use bootstrap token. | Setup |
| `POST /api/v1/auth/session` | Authentication and session creation. | Setup |
| `DELETE /api/v1/auth/session` | Current session revocation. | Setup |
| `GET /api/v1/auth/me` | Current administrator/session summary. | Setup |
| `GET /api/v1/tokens` | Machine-token metadata and scopes; never token values. | Automation |
| `POST /api/v1/tokens` | Creation of an application-scoped automation token; its value is displayed once. | Automation |
| `DELETE /api/v1/tokens/{id}` | Automation token revocation. | Automation |
| `GET /api/v1/system` | Version, host architecture, capabilities, capacity, component health. | Core |
| `GET /api/v1/settings` | Supported non-secret platform settings. | Core |
| `PATCH /api/v1/settings` | Supported preference changes; host-level changes remain in installer tooling. | Later core |
| `POST /api/v1/images/inspect` | Image/credential inspection, reporting architecture, digest, and suggested ports. | Deployment |
| `GET /api/v1/apps` | Paginated applications with summarized status. | Deployment |
| `POST /api/v1/apps` | Persistence of application intent and queued installation. | Deployment |
| `GET /api/v1/apps/{id}` | Configuration, observed status, active operation, and version. | Deployment |
| `PATCH /api/v1/apps/{id}` | Image/configuration/resources/exposure/replica change. | Lifecycle |
| `POST /api/v1/apps/{id}/restart` | Queued restart through a chart revision. | Lifecycle |
| `POST /api/v1/apps/{id}/reapply` | Explicit restoration of managed resources from the recorded specification/digest after drift. | Lifecycle |
| `GET /api/v1/apps/{id}/revisions` | Revision history and rollback eligibility. | Lifecycle |
| `POST /api/v1/apps/{id}/rollback` | Queued restoration of a selected revision. | Lifecycle |
| `DELETE /api/v1/apps/{id}` | Queued workload deletion with data retention by default. | Lifecycle |
| `GET /api/v1/apps/{id}/logs` | Bounded recent logs, optionally a selected pod/previous container. | Diagnostics |
| `GET /api/v1/apps/{id}/events` | Recent cluster diagnostics and operation progress. | Diagnostics |
| `GET /api/v1/operations/{id}` | Durable progress, outcome, and recovery details. | Deployment |
| `GET /api/v1/audit-events` | Paginated administrator action history. | Traceability |
| `GET /api/v1/registry-credentials` | Credential metadata and usage, never passwords/tokens. | Private images |
| `POST /api/v1/registry-credentials` | Secure credential creation. | Private images |
| `PUT /api/v1/registry-credentials/{id}` | Credential rotation with reference-aware behavior. | Private images |
| `DELETE /api/v1/registry-credentials/{id}` | Credential deletion after verification that no active or retained dependency requires it. | Private images |
| `GET /api/v1/secrets` | Secret metadata; values are never returned. | Core configuration (M2) |
| `POST /api/v1/secrets` | Versioned secret creation before use in an application. | Core configuration (M2) |
| `POST /api/v1/secrets/{id}/versions` | Creation of an immutable replacement secret version. | Core configuration (M2) |
| `DELETE /api/v1/secrets/{id}` | Secret deletion after verification of active and rollback references. | Core configuration (M2) |
| `GET /api/v1/volumes` | Attached and retained data volumes. | Persistence |
| `DELETE /api/v1/volumes/{id}` | Explicit, tracked deletion of an unused retained volume. | Persistence |
| `GET /metrics` | Private platform metrics for the monitoring profile. | Observability |

Secrets created during an abandoned form submission require visible unused status
and reference-aware cleanup. Operation payloads must not contain plaintext secret
values. Arbitrary exec, kubectl, manifest upload, and operation cancellation
endpoints are outside the first release.

### Request and response conventions

- All management endpoints require authentication except minimal health/setup
  status and the credential-protected setup/session entry points.
- Server-side sessions use HttpOnly cookies, a SameSite policy, CSRF defenses,
  expiry, and revocation. HTTPS requires Secure cookies. Plain HTTP management is
  restricted to local development or authenticated loopback bootstrap; LAN login
  requires HTTPS.
- Later automation uses scoped bearer tokens, checked through the same authorization
  rules; it must not reuse the administrator's browser password or session.
- Lifecycle mutations require `Idempotency-Key`. The same key/body returns the
  original result; the same key with different input returns a conflict.
- Responses include an ETag for the application version; `If-Match` is required for changes to
  an existing application, including rollback, restart, and deletion.
- Accepted changes are persisted before `202 Accepted` and an operation URL are returned.
  The request's disconnection must not cancel an accepted deployment.
- Status codes are `400` for malformed input, `401/403` for access errors, `404` for missing
  resources, `409` for conflicting active work, `412` for a stale version, and
  `422` for an unsupported specification. `503` indicates temporarily unavailable
  dependencies before acceptance; failures after acceptance are recorded on the operation.
- `409` also applies when `NeedsAttention` retains the application's active operation.
  Timeout, process restart, and an unsuccessful recovery attempt do not release this block.
- Request sizes, registry calls, log reads, query ranges, and list pagination are bounded.
- Error responses include stable codes, readable messages, affected fields, and request IDs.

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
resolved `image@sha256:...` reference. The actual execution digest is displayed
after the worker freezes it. Only initial creation, a changed image selection, or an explicit
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
Direct scaling through Kubernetes without updating Helm values is unsupported.

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
