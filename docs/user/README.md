# User guide

This guide covers everything an administrator needs to operate the platform:
setting it up, deploying applications, and using the full application lifecycle.

## Contents

- [Setup and login](#setup-and-login)
- [Deploying an application](#deploying-an-application)
- [Application lifecycle](#application-lifecycle)
- [Revision history and rollback](#revision-history-and-rollback)
- [Secrets](#secrets)
- [Registry credentials](#registry-credentials)
- [Operations](#operations)

---

## Setup and login

The platform exposes a browser UI at `http://<host>:8080` (or whatever address
the port-forward or ingress is configured to). On first access you are redirected
to the setup page.

**One-time setup:**

1. Retrieve the bootstrap token from broker logs:
   ```bash
   ./scripts/dev-env.sh token
   # or: kubectl -n platform-system logs deploy/platform | grep -A1 "Bootstrap token"
   ```
2. Enter the token and choose an admin password (minimum 12 characters).
3. Log in with that password. The session is stored in a cookie and expires after
   24 hours.

**Subsequent logins:** navigate to `http://<host>:8080`, enter your password.

---

## Deploying an application

Click **New Application** in the left sidebar. The wizard has six steps:

### Step 1 — Image

Enter a fully qualified image reference. Both tag and digest forms are accepted:

```
ghcr.io/myorg/myapp:v1.2.3
ghcr.io/myorg/myapp@sha256:abc123...
k3d-platform-registry:5001/platform-demo@sha256:...   # local dev registry
```

Click **Inspect image**. The platform resolves the image digest and detects the
container port from `EXPOSE` metadata. If the registry requires authentication,
select a stored credential first (see [Registry credentials](#registry-credentials)).

### Step 2 — Application

- **Name**: a unique identifier for this application (letters, digits, hyphens).
- **Container port**: the port your container listens on.
- **Replicas**: number of pods (default 1).

### Step 3 — Access

Choose an exposure mode:

| Mode | Description |
|---|---|
| `internal` | ClusterIP only, no ingress. Reachable from within the cluster. |
| `lan` | Ingress on the private entrypoint (LAN/VPN only). Requires a hostname. |

`public` exposure is defined but not yet available in this version of the platform.

For `lan` mode, supply a hostname (e.g. `myapp.example.com`).

### Step 4 — Configuration

- **Environment variables**: plain key/value pairs injected as container env.
- **Secret references**: reference a stored secret version (see [Secrets](#secrets)).
- **Resources**: CPU and memory requests/limits.
- **Readiness path**: HTTP path for the readiness probe (e.g. `/health`).

### Step 5 — Review

Review the complete specification before submitting.

### Step 6 — Deploy

The platform accepts the deployment request and returns an operation ID. You are
redirected to the **Operation progress** page, which polls for live events until
the operation succeeds or fails.

A successful deploy means Helm has applied the chart and Kubernetes reports the
pod as ready.

---

## Application lifecycle

Open any application from the **Applications** list to reach its detail page.
The action bar at the top provides:

| Action | What it does |
|---|---|
| **Restart** | Re-deploys the current spec, triggering a rolling restart |
| **Reapply** | Re-runs Helm with the current spec; useful to repair drift |
| **Update** | Opens the update form to change image, config, replicas, etc. |
| **Delete** | Removes the application and all its Kubernetes resources |

All actions are disabled while another operation is in progress (`activeOperationId`
is set). Each action requires an **Idempotency-Key** (generated automatically by
the UI) and the current application **version** for optimistic concurrency.

After submitting any action you are navigated to the **Operation progress** page
(`/apps/{id}/operation?op={operationId}`).

### Update

The update form pre-fills with the current spec. Change only what you need — all
fields are optional. Submitting creates a new revision.

### Delete

A confirmation dialog is shown before deletion. After the operation succeeds the
application's Helm release is uninstalled and the application is marked deleted.

---

## Revision history and rollback

Click the **History** tab on the application detail page. The platform loads all
revisions and shows:

| Column | Description |
|---|---|
| # | Revision number (monotonically increasing) |
| Result | `pending`, `succeeded`, or `failed` |
| Image | Frozen image reference (digest) |
| Replicas | Replica count at deploy time |
| Created | Timestamp |

A **Roll back** button appears next to every `succeeded` revision that is not the
current desired revision. Rollback re-deploys the frozen spec of the target
revision — the same image digest, same configuration — without re-inspecting the
registry.

---

## Secrets

Navigate to **Secrets** in the sidebar.

- **Create secret**: enter a name and an initial value. The value is stored
  encrypted in the cluster as a Kubernetes Secret.
- **Add version**: append a new value to an existing secret. Previous versions
  remain available for rollback.
- **Reference in an application**: in the application configuration step, add a
  secret reference: choose the secret, the version, and the environment variable
  name it should be injected as.

Secret values are write-only — they cannot be read back through the API or UI.

---

## Registry credentials

Navigate to **Registry credentials** in the sidebar.

- **Create**: provide a label, registry hostname (e.g. `ghcr.io`), username, and
  password or personal access token.
- **Use**: select the credential on the Image step of the deploy wizard.
- **Delete**: removes the stored credential. Applications that referenced it will
  fail to pull on the next deploy.

---

## Operations

Every mutation (create, update, restart, reapply, rollback, delete) creates an
**operation** with a unique ID. Operations are durable: if the broker restarts
mid-operation it resumes or moves to `needs_attention`.

Operation states:

| State | Meaning |
|---|---|
| `queued` | Accepted, waiting for the worker |
| `running` | Worker is executing (Helm call in progress) |
| `succeeded` | Helm succeeded and the workload is ready |
| `failed` | Terminal failure; the platform did not modify the cluster |
| `needs_attention` | Helm ran but outcome is unverified; manual check required |
| `recovering` | Broker restarted mid-operation; re-evaluating |

Navigate to `/apps/{id}/operation?op={operationId}` to watch live events for any
operation. The Events tab on the application detail page links to the active
operation if one is running.
