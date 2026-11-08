# Administrator UI walkthrough

Commands are executed from the repository root. The default workflow runs in the
`platform-local` cluster, with the management UI on port 8080 and an application
forward on port 8081. Installation, rebuilds and local access are managed through
`scripts/platform.sh`.

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

The UI blocks conflicting actions while an operation is active. Accepted changes
also carry the application version, preventing stale forms from overwriting a
newer accepted configuration.

## Secrets and credentials

The Secrets screen accepts write-only values and records immutable versions.
Use the single `value` key for the current environment-variable mapping profile.
An application's reference selects the secret and exact version. Values must not
be placed in ordinary environment fields or source control.

Registry credential endpoints currently provide metadata CRUD; private registry
authentication and image-pull provisioning are not complete end to end. The local
registry used by this walkthrough does not require a credential.

## Diagnostics

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

The restart token is not yet assigned correctly for an already-frozen revision;
a Restart operation can report success without replacing its Pods. Pod identity
must be checked when evaluating restart behaviour.

Failed Helm mutations retain `NeedsAttention` when their result is unresolved.
Automatic restoration after an unhealthy update remains incomplete; another
rollback request cannot bypass an active operation's lock. Demo v3 deliberately
fails readiness and should be reserved for a disposable failure experiment.

The initial UI offers a limited update form. A non-empty environment text area
replaces the complete ordinary environment map. Public HTTPS and application data
volumes are outside this local walkthrough. The existing creation form's LAN
mode also requires UI/API alignment; Internal is the supported guide path.

## Stop and resume

Stop active forwards with Ctrl+C, then stop the selected local environment:

```bash
./scripts/platform.sh stop
```

Cluster and registry data remain available. `./scripts/platform.sh up` starts,
rebuilds and upgrades the installation on the next run. Stopping is not a backup;
important state requires a separately verified backup procedure.
