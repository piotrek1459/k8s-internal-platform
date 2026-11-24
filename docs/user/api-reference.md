# API walkthrough and endpoint reference

The API uses JSON under `/api/v1`. Management requests require a session cookie;
mutations additionally require `X-Requested-With`. Application mutations return
202 and an operation ID. Completion must be checked through the operation API.
[Implementation status](../status.md) records known contract, recovery and
credential gaps; this walkthrough uses the verified healthy Internal path.

The examples use the default local environment and a separate application with a unique
`api-demo-<timestamp>` name. Commands are executed from the repository root in **Bash**. The UI
and curl use separate sessions, even when browser login has already succeeded.

## 1. Start the platform and establish shell helpers

Keep a management forward running in another terminal:

```bash
./scripts/platform.sh up
# For an installation that is already running: ./scripts/platform.sh ui
```

In the API terminal:

```bash
bash
set -o pipefail
umask 077
mkdir -p .local/api
chmod 700 .local/api
PLATFORM_API=http://127.0.0.1:8080/api/v1
PLATFORM_COOKIES="$PWD/.local/api/cookies.txt"

api() {
  curl --fail-with-body --silent --show-error \
    --cookie "$PLATFORM_COOKIES" --cookie-jar "$PLATFORM_COOKIES" \
    --header 'Content-Type: application/json' \
    --header 'X-Requested-With: XMLHttpRequest' "$@"
}
```

Cookie files under `.local/` are ignored by Git. The helper preserves JSON error
bodies and returns a nonzero exit status for failed HTTP requests.

## 2. Complete setup once, then log in

Check setup availability:

```bash
api "$PLATFORM_API/setup/status"
```

If `setupAvailable` is true and no administrator has been created, run this
one-time step. The password is read from the terminal without entering it in
shell history:

```bash
BOOTSTRAP_TOKEN="$(./scripts/platform.sh token)"
python3 -c 'import getpass,json; print(json.dumps({"password":getpass.getpass("New administrator password (12–256 characters): ")}))' \
  | api --request POST \
      --header "Authorization: Bearer $BOOTSTRAP_TOKEN" \
      --data-binary @- "$PLATFORM_API/setup"
unset BOOTSTRAP_TOKEN
```

An account created through the browser skips that step. Login uses the
administrator password chosen during setup:

```bash
python3 -c 'import getpass,json; print(json.dumps({"password":getpass.getpass("Administrator password: ")}))' \
  | api --request POST --data-binary @- "$PLATFORM_API/auth/session"

api "$PLATFORM_API/auth/me"
api "$PLATFORM_API/system"
```

Successful login returns HTTP 204 with no body and writes the session cookie.
`/auth/me` then reports the administrator ID and session expiry. A setup token
expires one hour after broker startup; it is not a login password. Token renewal
before account creation is described in the [UI guide](README.md).

## 3. Define operation submission and polling

The helpers below expose the request body and idempotency key for retries while
keeping the repetitive curl options in one place. Polling stops on success,
failure, `needs_attention`, or timeout.

```bash
poll_operation() {
  local operation_id="$1" attempt state
  for ((attempt=0; attempt<120; attempt++)); do
    api "$PLATFORM_API/operations/$operation_id" > .local/api/operation.json || return 1
    state="$(python3 -c 'import json; print(json.load(open(".local/api/operation.json"))["state"])')" || return 1
    printf '%s: %s\n' "$operation_id" "$state"
    case "$state" in
      succeeded) return 0 ;;
      failed|needs_attention)
        python3 -m json.tool < .local/api/operation.json
        return 1 ;;
    esac
    sleep 2
  done
  printf 'Operation polling timed out; inspect the recorded operation.\n' >&2
  return 1
}

mutate() {
  REQUEST_METHOD="$1"
  REQUEST_PATH="$2"
  printf '%s' "$3" > .local/api/request.json
  REQUEST_KEY="$(python3 -c 'import uuid; print(uuid.uuid4())')" || return 1
  api --request "$REQUEST_METHOD" \
    --header "Idempotency-Key: $REQUEST_KEY" \
    --data-binary @.local/api/request.json \
    "$PLATFORM_API$REQUEST_PATH" > .local/api/accepted.json || {
      cat .local/api/accepted.json
      return 1
    }
  cat .local/api/accepted.json
  OPERATION_ID="$(python3 -c 'import json; print(json.load(open(".local/api/accepted.json"))["operationId"])')" || return 1
  poll_operation "$OPERATION_ID"
}
```

Each new `mutate` call generates a new idempotency key. If a connection fails
and the request's acceptance is uncertain, retry the existing request with its
existing key and body instead of calling `mutate` again:

```bash
api --request "$REQUEST_METHOD" \
  --header "Idempotency-Key: $REQUEST_KEY" \
  --data-binary @.local/api/request.json \
  "$PLATFORM_API$REQUEST_PATH"
```

An identical replay returns the original operation. Reusing a key for a different
application, action or body returns a conflict. Poll the returned operation before
starting the next mutation.

## 4. Inspect and deploy demo v1

Startup publishes the demonstration images. Get the immutable v1 reference:

```bash
IMAGE_V1="$(./scripts/platform.sh image v1)"

python3 -c 'import json,sys; print(json.dumps({"image":sys.argv[1]}))' "$IMAGE_V1" \
  | api --request POST --data-binary @- "$PLATFORM_API/images/inspect"
```

Create a new application:

```bash
APP_NAME="api-demo-$(date +%s)"
CREATE_BODY="$(python3 -c 'import json,sys; print(json.dumps({"name":sys.argv[2],"image":sys.argv[1],"profile":"http","containerPort":8080,"replicas":1,"health":{"readinessPath":"/readyz"},"exposure":{"mode":"internal"}}))' "$IMAGE_V1" "$APP_NAME")"
mutate POST /apps "$CREATE_BODY"
```

Continue after the operation succeeds:

```bash
APP_ID="$(python3 -c 'import json; print(json.load(open(".local/api/accepted.json"))["applicationId"])')"
api "$PLATFORM_API/apps/$APP_ID"
api "$PLATFORM_API/apps"
REVISION_V1="$(api "$PLATFORM_API/apps/$APP_ID" | python3 -c 'import json,sys; print(json.load(sys.stdin)["lastSuccessfulRevision"])')"
printf 'Application ID: %s\n' "$APP_ID"
```

In a separate terminal, forward the application using that printed ID:

```bash
./scripts/platform.sh forward app_REPLACE_WITH_APPLICATION_ID
```

Then request **http://127.0.0.1:8081/version** in a browser or another terminal.
The expected version is v1.

## 5. Update to v2 and inspect history

Rebuild v2 when the demo source has changed; otherwise the image published by
startup is already available:

```bash
./scripts/platform.sh demo v2
IMAGE_V2="$(./scripts/platform.sh image v2)"
APP_VERSION="$(api "$PLATFORM_API/apps/$APP_ID" | python3 -c 'import json,sys; print(json.load(sys.stdin)["version"])')"
UPDATE_BODY="$(python3 -c 'import json,sys; print(json.dumps({"version":int(sys.argv[1]),"image":sys.argv[2]}))' "$APP_VERSION" "$IMAGE_V2")"
mutate PATCH "/apps/$APP_ID" "$UPDATE_BODY"

api "$PLATFORM_API/apps/$APP_ID/revisions"
```

The `version` is the application's concurrency counter, not its software version
or the Helm revision. Fetch it again before each new mutation. Changes omitted
from PATCH retain their current configuration. A supplied environment map
replaces the complete map.

Reconnect the application forward after Pod replacement. `/version` should now
return v2 with a new Pod hostname. History is newest-first and includes
`frozenImage` when an execution snapshot exists.

## 6. Change replicas, roll back and delete the test app

Scale to two replicas:

```bash
APP_VERSION="$(api "$PLATFORM_API/apps/$APP_ID" | python3 -c 'import json,sys; print(json.load(sys.stdin)["version"])')"
mutate PATCH "/apps/$APP_ID" "{\"version\":$APP_VERSION,\"replicas\":2}"
api "$PLATFORM_API/apps/$APP_ID"
```

A replica-only update preserves the recorded image digest. Zero replicas stops
the workload; one restores it. Single-host replica counts do not provide
host-level availability.

Roll back to the successful v1 revision saved earlier:

```bash
APP_VERSION="$(api "$PLATFORM_API/apps/$APP_ID" | python3 -c 'import json,sys; print(json.load(sys.stdin)["version"])')"
ROLLBACK_BODY="$(python3 -c 'import json,sys; print(json.dumps({"version":int(sys.argv[1]),"revisionId":sys.argv[2]}))' "$APP_VERSION" "$REVISION_V1")"
mutate POST "/apps/$APP_ID/rollback" "$ROLLBACK_BODY"
```

Rollback creates a new revision using the selected successful snapshot, including
its image digest and secret version references. It does not restore external
application data. Reconnect the forward before checking the HTTP response.

The following removes only the application created above:

```bash
APP_VERSION="$(api "$PLATFORM_API/apps/$APP_ID" | python3 -c 'import json,sys; print(json.load(sys.stdin)["version"])')"
mutate DELETE "/apps/$APP_ID" "{\"version\":$APP_VERSION}"
api "$PLATFORM_API/apps"
api "$PLATFORM_API/apps/$APP_ID/revisions"
```

The app disappears from the active list; stored revision history remains.

## Endpoint reference

| Method and path | Input / result |
| --- | --- |
| `GET /healthz`, `GET /readyz` | Unauthenticated process/readiness diagnostics outside `/api/v1`. |
| `GET /setup/status` | Unauthenticated `setupAvailable` boolean. |
| `POST /setup` | Bearer bootstrap token and `{password}`; creates the administrator once. |
| `POST /auth/session` | `{password}`; sets session cookie, returns 204. |
| `DELETE /auth/session` | Revokes session and clears cookie. |
| `GET /auth/me` | Administrator identity and session expiry. |
| `GET /system` | Version, architecture, cluster reachability and capabilities. |
| `POST /images/inspect` | `{image, registryCredentialId?}`; digest, architectures and suggested port. |
| `POST /apps` | Create specification; requires Idempotency-Key, returns 202. |
| `GET /apps`, `GET /apps/{id}` | Active list or application details; detail response includes an ETag. |
| `PATCH /apps/{id}` | `{version, image?, replicas?, environment?, ...}`; tracked update. |
| `DELETE /apps/{id}` | `{version}`; tracked deletion. |
| `POST /apps/{id}/restart` | `{version}`; retain image digest and replace Pods. |
| `POST /apps/{id}/reapply` | `{version}`; reapply the desired configuration. |
| `POST /apps/{id}/rollback` | `{version, revisionId}`; target must be a successful revision of this app. |
| `GET /apps/{id}/revisions` | Revision history, newest first. |
| `GET /operations/{id}` | State, stages, events and recovery outcome. |
| `GET /secrets` | Metadata, versions and deletion-pending state; no plaintext values. |
| `POST /secrets` | `{name, data:{value:...}}`; create immutable version 1. |
| `POST /secrets/{id}/versions` | `{data:{value:...}}`; create a new immutable version. |
| `DELETE /secrets/{id}` | Refuse live protected references; remove recorded Kubernetes versions, then metadata. |
| `GET /registry-credentials` | Credential metadata. |
| `POST /registry-credentials` | `{label, registry, username, password}`; password stored in a Kubernetes Docker configuration Secret. |
| `DELETE /registry-credentials/{id}` | Delete credential and its Kubernetes Secret; reject protected references. |

All application mutations require `Idempotency-Key`; mutations of existing apps
also require the current `version` in the JSON body. The implementation uses that body field for its optimistic check; If-Match is
not enforced. Request and camelCase response schemas are in
[`api/openapi.yaml`](../../api/openapi.yaml).

Secret deletion in v0.1.6 first reserves cleanup durably. A reference
from an active application's pending/successful revision or nonterminal operation
returns `SECRET_IN_USE`/409. Deleted-application history remains available without
blocking removal. New application references and new versions are refused while
`deleting: true`. Each Kubernetes removal verifies the recorded UID, deterministic
name, namespace and ownership, and uses UID/resource-version preconditions.
Missing versions are already removed; changed ownership is refused. A partial
cleanup returns 409/503 and retains metadata/identities, so repeating DELETE can
finish after diagnosis. A 204 establishes removal of all recorded versions and
metadata. Versions orphaned by earlier metadata-only deletion are not guessed or
collected automatically. This corrected cleanup is included from v0.1.6 and passed native v0.1.7
physical cleanup/browser retry acceptance. The intermediate v0.1.6 release was
not tested natively.

`POST /apps/{id}/recover` is a recovery retry, not a new mutation specification.
It accepts `{version}`, reuses the existing blocked operation ID and increments
the application version. It does not require a new Idempotency-Key. A repeated
submission with the old version is rejected. Pending or ambiguous cluster state
remains blocked. Failed updates with a prior complete successful snapshot attempt
verified restoration automatically and expose the outcome in operation events.

## Application diagnostics

The administrator session is required. Responses use `Cache-Control: no-store`.
Unknown or repeated query parameters are rejected.

| Route | Parameters and limits |
| --- | --- |
| `GET /apps/{id}/logs` | `pod` optional; `previous=false`; `tailLines=100` (1–500); `sinceSeconds=3600` (1–86400). Application container only; no namespace, container, exec or follow option. |
| `GET /apps/{id}/events` | `limit=50` (1–100); retained events for current owned resource UIDs. |
| `GET /apps/{id}/drift` | No parameters; frozen revision, observation time, active-operation flag and field-path differences. |

Log text is limited to 64 KiB. Event text is limited to 4096 bytes per field and
64 KiB overall. Discovery is limited to 50 objects per resource kind and 50 event
source UIDs, with at most 20 events retained per bounded source read. `truncated`
identifies incomplete output. Kubernetes retention and bounded source reads can
omit events. Previous logs belong to a retained container instance in the same
Pod; deleted Pods are not archived. Pod controller UIDs are checked before and
after reading to reject a reused Pod name.

Known managed configuration and registry credentials are masked in memory,
including plaintext, standard/URL base64 with or without padding, JSON-escaped
strings and URL-query encoding. Overlapping matches are merged, masking precedes
clipping, and a truncated trailing secret prefix is covered. Applications must
avoid logging credentials: arbitrary application-defined secrets and custom
transformations are outside this policy.

Secret inventory is bounded to 100 objects per category, aggregate redaction
inputs to 512 KiB, unique patterns to 2048 and match processing to a fixed budget.
Missing/incomplete inventory fails closed. Two diagnostic requests may execute
concurrently; HTTP reads have a ten-second deadline. Namespace-wide inventory
limits can make diagnostics unavailable even when the selected app is small.
Values are not copied to SQLite, Helm, broker logs or retained test artifacts by
the diagnostics read path.

Drift reports `in_sync` or `drifted` for owned Deployment, Service and Ingress
fields, without expected/actual values. Readiness and allocated addresses are
separate observations. Missing snapshots or an intent change during inspection
produce a conflict. Inspection performs no correction. Explicit Reapply repairs
supported owned fields, including the ClusterIP Service port list under UID and
resource-version guards, while preserving its identity/address. Ownership checks
also protect restoration and uninstall. Pending Helm records are not cleared.

These source endpoints require a release containing the diagnostics changes;
the published v0.1.4 Pi acceptance covers the earlier lifecycle features.

## Errors and operating limits

Errors use the nested envelope:

```json
{"error":{"code":"VERSION_CONFLICT","message":"...","field":"...","requestId":"req_..."}}
```

| HTTP status | Meaning / response |
| --- | --- |
| 400 | Invalid request input. |
| 401 | Missing, invalid or expired session/credential. |
| 403 | Forbidden request, including a missing mutation CSRF header. |
| 409 | Active operation, secret in use, or conflicting idempotency reuse. |
| 412 | Stale application version; fetch current details before a new mutation. |
| 422 | Unsupported application/image specification. |
| 503 | A required dependency is unavailable. |

`needs_attention` retains the mutation lock when restoration or target identity
cannot be verified. A new rollback cannot bypass it. Recovery retry reuses the
blocked operation after diagnosis; a still-pending Helm transaction remains
blocked. Failed updates with a complete prior snapshot attempt verified automatic
restoration. Restart requires a changed Pod identity and retains the image digest.

For a blocked first deployment with no successful predecessor, explicit
`DELETE /apps/{id}` provides a removal path. Submit the current `version` and a
new `Idempotency-Key`. Acceptance records the original create operation as failed
with stage `removal_requested`, retains its frozen snapshot and history, and
transfers the lock to queued cleanup atomically. The application leaves the active
list only after uninstall succeeds. A failed uninstall remains blocked and can
use recovery retry. This exception does not permit deletion to bypass running
work or an unresolved update of a previously successful application.

Public exposure and application data volumes remain outside this walkthrough.
Demo v3 deliberately fails readiness and should be used only for failure testing.

Logout after the walkthrough:

```bash
api --request DELETE "$PLATFORM_API/auth/session"
```

## Configured private network access

With the optional LAN mode, `GET /system` includes `privateNetwork` configuration
(`managementHostname`, `applicationDomain`, `httpsPort`). `GET /apps/{id}` adds
`access` with mode/revision ID and a configured LAN hostname/HTTPS URL. Access
uses the last successful revision until a candidate succeeds; an initial app
without a successful revision uses its desired revision. These fields do not
establish DNS, certificate trust or external reachability.

LAN creation/PATCH requires a single-label hostname beneath the configured app
domain. Management/outside/nested names return 400, and another live app's
pending/successful rollback reservation returns 409 atomically before acceptance.
Deleting the owning app releases the reservation. In a configured LAN installation,
PATCH changing access mode or hostname returns 422; image/replica changes continue
to work. Route transitions require failed-mutation absence verification and remain
part of the core recovery follow-up. Internal exposure creates no
Ingress. See the [private LAN setup and HTTPS/SSH fallback](../private-lan.md).
This source capability is not in published v0.1.7; native acceptance awaits the
next release.
