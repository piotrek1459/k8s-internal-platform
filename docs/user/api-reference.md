# API reference

The broker exposes a JSON REST API at `/api/v1/`. All endpoints require the
`X-Requested-With: XMLHttpRequest` header (CSRF protection). Authenticated
endpoints require a valid session cookie obtained via `POST /api/v1/auth/session`.

Mutation endpoints (`POST`, `PATCH`, `DELETE`) require an `Idempotency-Key` header.
Submit the same key with the same body to safely retry — the platform returns the
original result without re-executing.

All error responses follow:

```json
{ "error": { "code": "ERROR_CODE", "message": "human description", "requestId": "req_..." } }
```

---

## Authentication

### POST /api/v1/setup/status
Returns whether initial setup has been completed.

```json
{ "setupAvailable": true }
```

### POST /api/v1/setup
Complete one-time setup. Requires `Authorization: Bearer <bootstrap-token>` header.

**Body:**
```json
{ "password": "your-admin-password" }
```

### POST /api/v1/auth/session
Log in. Creates a session cookie.

**Body:**
```json
{ "password": "your-admin-password" }
```

### DELETE /api/v1/auth/session
Log out. Clears the session cookie.

### GET /api/v1/auth/me
Returns the current session.

```json
{ "administratorId": "adm_...", "sessionExpiresAt": "2026-11-01T10:00:00Z" }
```

---

## System

### GET /api/v1/system
Platform metadata and cluster status.

```json
{
  "version": "dev",
  "architecture": "arm64",
  "clusterReachable": true,
  "capabilities": ["secrets", "registry-credentials"]
}
```

### GET /healthz
Liveness probe. Always returns `200 { "status": "ok" }`.

### GET /readyz
Readiness probe. Returns `200` with cluster connectivity status.

```json
{ "status": "ready", "cluster": "available" }
```

---

## Applications

### GET /api/v1/apps
List all non-deleted applications.

Query params: `limit` (default 50), `offset` (default 0).

```json
{
  "items": [
    { "id": "app_...", "name": "demo", "version": 3, "phase": "ready" }
  ]
}
```

### POST /api/v1/apps
Create and deploy a new application. Returns `202 Accepted`.

**Headers:** `Idempotency-Key: <uuid>`

**Body:**
```json
{
  "name": "demo",
  "image": "ghcr.io/example/app@sha256:...",
  "registryCredentialId": "cred_...",
  "containerPort": 8080,
  "profile": "http",
  "replicas": 1,
  "environment": { "LOG_LEVEL": "info" },
  "secretRefs": [
    { "environmentVariable": "DB_PASSWORD", "secretId": "sec_...", "version": 1 }
  ],
  "resources": {
    "cpuRequest": "50m", "cpuLimit": "500m",
    "memoryRequest": "64Mi", "memoryLimit": "256Mi"
  },
  "health": { "readinessPath": "/health" },
  "exposure": { "mode": "internal" }
}
```

**Response:**
```json
{
  "applicationId": "app_...",
  "operationId": "op_...",
  "state": "queued",
  "deduplicated": false
}
```

### GET /api/v1/apps/{id}
Get full application detail including current status.

```json
{
  "id": "app_...",
  "name": "demo",
  "version": 3,
  "desiredRevision": "rev_...",
  "lastSuccessfulRevision": "rev_...",
  "activeOperationId": "",
  "status": {
    "phase": "ready",
    "desiredReplicas": 1,
    "readyReplicas": 1,
    "workloadReady": true,
    "routeReady": false,
    "observedAt": "2026-10-31T22:00:00Z",
    "stale": false
  }
}
```

### PATCH /api/v1/apps/{id}
Update application configuration. All fields optional — only supplied fields change.
Returns `202 Accepted`.

**Headers:** `Idempotency-Key: <uuid>`

**Body:**
```json
{
  "image": "ghcr.io/example/app@sha256:...",
  "replicas": 2,
  "environment": { "LOG_LEVEL": "debug" },
  "version": 3
}
```

### DELETE /api/v1/apps/{id}
Delete the application (uninstalls Helm release). Returns `202 Accepted`.

**Headers:** `Idempotency-Key: <uuid>`

**Body:**
```json
{ "version": 3 }
```

### POST /api/v1/apps/{id}/restart
Rolling restart with current spec. Returns `202 Accepted`.

**Headers:** `Idempotency-Key: <uuid>`

**Body:**
```json
{ "version": 3 }
```

### POST /api/v1/apps/{id}/reapply
Re-run Helm with current spec (repairs drift). Returns `202 Accepted`.

**Headers:** `Idempotency-Key: <uuid>`

**Body:**
```json
{ "version": 3 }
```

### POST /api/v1/apps/{id}/rollback
Roll back to a specific succeeded revision. Returns `202 Accepted`.

**Headers:** `Idempotency-Key: <uuid>`

**Body:**
```json
{ "revisionId": "rev_...", "version": 3 }
```

The target revision must belong to this application and have `result: succeeded`.

### GET /api/v1/apps/{id}/revisions
List all revisions for an application (newest first).

```json
{
  "items": [
    {
      "id": "rev_...",
      "number": 3,
      "result": "succeeded",
      "frozenAt": "2026-10-31T20:00:00Z",
      "createdAt": "2026-10-31T20:00:00Z",
      "spec": { "image": "ghcr.io/example/app@sha256:...", "replicas": 1 }
    }
  ]
}
```

---

## Operations

### GET /api/v1/operations/{id}
Get operation state and event log.

```json
{
  "id": "op_...",
  "applicationId": "app_...",
  "type": "update",
  "state": "succeeded",
  "stage": "apply",
  "recoveryOutcome": "",
  "events": [
    { "at": "2026-10-31T20:00:01Z", "stage": "freeze", "message": "image digest resolved" },
    { "at": "2026-10-31T20:00:02Z", "stage": "apply", "message": "helm upgrade succeeded" }
  ]
}
```

---

## Images

### POST /api/v1/images/inspect
Inspect an image reference: resolve digest, list architectures, detect port.

**Body:**
```json
{ "image": "ghcr.io/example/app:latest", "registryCredentialId": "cred_..." }
```

**Response:**
```json
{ "digest": "sha256:...", "architectures": ["arm64", "amd64"], "suggestedPort": 8080 }
```

---

## Secrets

### GET /api/v1/secrets
List all secrets (values are never returned).

### POST /api/v1/secrets
Create a secret.

**Body:** `{ "name": "db-password", "data": { "value": "secret123" } }`

### POST /api/v1/secrets/{id}/versions
Add a new version to an existing secret.

**Body:** `{ "data": { "value": "newsecret456" } }`

### DELETE /api/v1/secrets/{id}
Delete a secret and all its versions.

---

## Registry credentials

### GET /api/v1/registry-credentials
List stored credentials (passwords are never returned).

### POST /api/v1/registry-credentials
Store a credential.

**Body:**
```json
{ "label": "GitHub", "registry": "ghcr.io", "username": "myuser", "password": "ghp_..." }
```

### DELETE /api/v1/registry-credentials/{id}
Delete a stored credential.
