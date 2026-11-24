# Administrator UI walkthrough

Commands are executed from the repository root. The default workflow runs in the
`platform-local` cluster, with the management UI on port 8080 and an application
forward on port 8081. Installation, rebuilds and local access are managed through
`scripts/platform.sh`. [Implementation status](../status.md) records the full
capability and verification boundaries. The installation may be absent after
cleanup; startup creates a fresh one when necessary.

## 1. Start the platform

```bash
./scripts/platform.sh up
```

The command builds and installs the platform, then remains in a foreground
port-forward. Keep that terminal open. A second terminal is used for the commands
below. A later rebuild uses the same command and preserves the cluster's metadata
volume. Ctrl+C stops forwarding; `./scripts/platform.sh ui` reconnects it.

For startup without a forward:

```bash
./scripts/platform.sh up --no-ui
./scripts/platform.sh ui
```

## 2. Create the administrator account or sign in

For the first installation, open **http://127.0.0.1:8080/setup** and retrieve the
current token in a second terminal:

```bash
./scripts/platform.sh token
```

Complete all three fields: Bootstrap token, Administrator password, and Confirm
password. The password must contain 12–256 characters. Select **Create account**,
then sign in at **http://127.0.0.1:8080/login** using the chosen password.

| Credential | Purpose |
| --- | --- |
| Bootstrap token | One-time account creation through `/setup`; expires one hour after broker startup. |
| Administrator password | Login after the account has been created. |
| Session cookie | Browser authentication following login; expires after 12 hours or logout. |

If only a Sign in form is visible before account creation, navigate explicitly
to `/setup`. The current root route can lead to login before first-time setup.
A consumed token is no longer needed.

If setup has not completed and the token has expired, restart only the broker:

```bash
kubectl --kubeconfig .local/platform-local.kubeconfig \
  --context k3d-platform-local -n platform-system \
  rollout restart deployment/platform
kubectl --kubeconfig .local/platform-local.kubeconfig \
  --context k3d-platform-local -n platform-system \
  rollout status deployment/platform --timeout=180s
```

Reconnect using `./scripts/platform.sh ui` and retrieve the new token. The
metadata volume is retained. An already-created account continues to use its
administrator password. A token does not reset that password.

## 3. Deploy demo v1

Print the image reference:

```bash
./scripts/platform.sh image v1
```

Copy the complete `k3d-platform-local-registry:5002/platform-demo@sha256:...`
reference. The digest selects the exact image built and published by startup.

Open **Applications → New Application** and complete the wizard:

| Field | Value |
| --- | --- |
| Image reference | Complete output of `platform.sh image v1`. |
| Registry credential | None for the local development registry. |
| Name | `ui-demo`, or another unused lowercase name. |
| Container port | `8080`. |
| Readiness path | `/readyz`. |
| Exposure | **Internal**. |
| Environment and secret references | Empty for this demo. |

Inspect the image, review the settings and submit. Deployment is asynchronous:
request acceptance creates a queued operation; completion is reported separately.
Wait for **succeeded**. An API, registry or workload error is investigated through
the operation events and broker diagnostics.

## 4. Access the application

Copy the application's opaque ID from its detail page, then run in another
terminal, replacing the example ID:

```bash
./scripts/platform.sh forward app_REPLACE_WITH_APPLICATION_ID
```

Keep the forward running and open **http://127.0.0.1:8081/version**. The response
should contain `"version":"v1"` and the responding Pod's hostname.
`http://127.0.0.1:8081/readyz` should return HTTP 200.

Internal exposure creates a cluster Service without an ingress route. The local
forward provides browser access without configuring DNS or public networking.

## 5. Update to demo v2

Startup already publishes v2. If the demo source has changed, rebuild and publish
that version first:

```bash
./scripts/platform.sh demo v2
./scripts/platform.sh image v2
```

Open the application and select **Update**. Replace the image reference with the
v2 digest, keep Replicas at 1, leave the environment text area empty to preserve
existing values, and select **Apply update**. Wait for **succeeded**.

The application forward may stop when its selected Pod is replaced. Stop the old
forward with Ctrl+C, rerun `platform.sh forward APP_ID`, and check `/version`.
The result should contain `"version":"v2"` and a new Pod hostname.

The demo version is embedded by the Docker build's `VERSION` argument. Image
references select built artifacts; they do not modify the application's version
at runtime. The **History** tab displays the prior and current successful
revisions and their frozen image references.

## 6. Scale, roll back, reapply or delete

- **Scale:** select Update, change Replicas, and apply. Two replicas run two Pods
  on the same host; zero stops the workload. Restore one before requesting HTTP
  access. Single-host replicas do not provide host-level high availability.
- **Rollback:** open History and select Roll back on a prior successful revision.
  Wait for its operation, reconnect the application forward, and verify the
  returned version. Rolling back creates a new history entry. It does not undo
  external database or application-data changes.
- **Reapply:** reapplies the recorded desired workload configuration. It is an
  explicit action for supported drift correction.
- **Delete:** select Delete and confirm. Wait for completion; the workload is
  removed and the application disappears from the active list. Recorded history
  remains stored. Use a separate test application for deletion experiments.

The UI blocks conflicting actions while an operation is active. A blocked first
deployment with no successful revision offers **Delete** as an explicit removal
path. Its failed deployment history is retained, and the application remains
locked until the tracked cleanup succeeds. This exception does not apply to
running work or unresolved updates of previously successful applications.
Accepted changes
also carry the application version, preventing stale forms from overwriting a
newer accepted configuration.

## Secrets and credentials

The Secrets screen accepts write-only values and records immutable versions.
The API supports the single `value` key for the environment-variable mapping
profile. The creation wizard submits the selected Secret and exact version.
The Secrets screen supports creation and **Add version**; values are cleared after
submission and are never returned by reads. Values must not be placed in ordinary
environment fields or source control.

Release v0.1.6 removes the recorded Kubernetes versions before completing Secret
deletion. Active, rollback-eligible and in-flight references return HTTP 409.
Retained history of a deleted application does not keep a live dependency.
If cleanup is interrupted or an object's identity/ownership has changed, the
Secret remains marked **Deletion is pending**. New versions and deployment
references are refused; after the underlying error is resolved, **Retry deletion**
continues cleanup using the recorded identities. The platform does not adopt a
replacement object. The v0.1.5 metadata-only deletion and
deleted-history defects are corrected in v0.1.6. Source real-cluster acceptance
and published v0.1.7 Pi cleanup/retry verification passed; the [native report](../../tests/experiments/2026-11-23-v0.1.7-native.md)
records the physical version removal and retained history/session/PVC checks. See [status](../status.md).

Stored registry credentials support authenticated image inspection and actual
kubelet pulls. Passwords remain in Kubernetes; ordinary responses contain
metadata. In **Registries**, **Add credential** records a registry hostname,
username and password/token. Select its label in the application's image step.
An incorrect password is reported during **Inspect image**, before deployment.
Creation alone does not establish that the credential works. Referenced
credentials cannot be removed; after dependent applications are deleted,
**Remove** deletes the credential and its Kubernetes authentication Secret.
The local demonstration registry does not require a credential.

## Diagnostics

Application details → **Diagnostics** provides three explicit reads:

- **Read logs:** newest active owned Pod by default, or a selected owned Pod.
  Recent lines are limited to 100 or 500 and the range to the last hour or day.
  **Previous container** reads a prior container instance in that same Pod when
  retained by Kubernetes; it does not retrieve deleted Pods or an archived release.
- **Read events:** retained Kubernetes events for current owned resource UIDs.
  These are separate from the application's durable operation events.
- **Inspect drift:** compares owned workload/route fields with the frozen desired
  execution snapshot. Differences identify fields without returning their values.
  Active operations can still be converging; inspect again after completion.

Logs and events are bounded and displayed as escaped text. Managed configuration
and registry Secret values and supported encodings are masked before clipping.
A failed Secret inventory read fails the diagnostic request. Arbitrary
application-defined secrets or custom transformations cannot be recognized;
applications must avoid logging credentials or other sensitive content.
The [API guide](api-reference.md#application-diagnostics) records exact limits.

Drift inspection changes no resources. **Reapply** remains a separate action.
For an owned ClusterIP Service, explicit Reapply repairs the desired port list
under UID/resource-version guards before Helm applies the frozen workload.
Service identity and allocated addresses are retained. Foreign ownership or an
unresolved Helm transaction blocks repair. Unsupported fields, external resources
and the unavailable historical-chart case still require diagnosis.

Development broker diagnostics remain available through:

```bash
./scripts/platform.sh status
./scripts/platform.sh logs
```

| Symptom | Action |
| --- | --- |
| Incorrect password when entering a token | Use `/setup` for account creation; `/login` uses the selected password. |
| Token unavailable | Check whether the account already exists or the first-hour token expired. |
| UI connection lost after rebuild | Reconnect with `platform.sh ui`. |
| Application connection lost after update | Reconnect with `platform.sh forward APP_ID`. |
| Port already in use | Stop the earlier forward or select `PLATFORM_UI_PORT` / `PLATFORM_APP_PORT`. |
| Image inspection fails | Confirm `image v1` prints a published reference; inspect broker logs and registry state. |
| VERSION_CONFLICT / HTTP 412 | Reload the application before submitting a new action. |
| NeedsAttention / HTTP 409 | Read operation diagnostics; the unresolved operation retains its mutation lock. |

## Current limitations

Status fields and nested errors follow the API contract. Restart retains the
recorded image digest and replaces Pods using a stable per-operation token.
After a failed update, the previous successful snapshot is restored and verified
before its operation is marked failed and the application is unlocked. An
unresolved result remains `NeedsAttention`; **Retry recovery** reuses the blocked
operation after its underlying cause has been addressed. A pending Helm release
or unavailable historical chart can still require host-side diagnosis.

The update form remains limited. A non-empty environment text area replaces the
complete ordinary environment map. Creation supports versioned Secret references.
LAN selection requires an explicitly configured private ingress controller.
The source implementation adds an optional [private HTTPS installation](../private-lan.md)
with separate management/application names, local DNS/trust instructions and
source restrictions; it requires the next release and native validation. The
wizard accepts one application label beneath the configured domain, and details
show the persisted HTTPS URL after reload. Internal access and initial SSH
bootstrap remain available.
Public HTTPS, application volumes, a complete configuration editor and durable
searchable log storage remain planned. Application diagnostics and drift inspection
are included in published v0.1.5. The reference Pi verified masking, ownership
checks, escaped text and explicit Service repair. The earlier v0.1.5 native run exposed
Secret-deletion defects; that failure remains retained as evidence. Published
v0.1.7 subsequently passed physical cleanup and retry across broker replacement.

## Stop and resume

Stop active forwards with Ctrl+C, then stop the selected local environment:

```bash
./scripts/platform.sh stop
```

Cluster and registry data remain available. `./scripts/platform.sh up` starts,
rebuilds and upgrades the installation on the next run. Stopping is not a backup;
important state requires a separately verified backup procedure.
