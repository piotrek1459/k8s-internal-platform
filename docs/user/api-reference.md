# API walkthrough and endpoint reference

The API uses JSON under `/api/v1`. Management requests require a session cookie;
mutations additionally require `X-Requested-With`. Application mutations return
202 and an operation ID. Completion must be checked through the operation API.

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
| `POST /apps/{id}/restart` | `{version}`; current implementation has the restart limitation below. |
| `POST /apps/{id}/reapply` | `{version}`; reapply the desired configuration. |
| `POST /apps/{id}/rollback` | `{version, revisionId}`; target must be a successful revision of this app. |
| `GET /apps/{id}/revisions` | Revision history, newest first. |
| `GET /operations/{id}` | State, stages, events and recovery outcome. |
| `GET /secrets` | Metadata and versions; no plaintext values. |
| `POST /secrets` | `{name, data:{value:...}}`; create immutable version 1. |
| `POST /secrets/{id}/versions` | `{data:{value:...}}`; create a new immutable version. |
| `DELETE /secrets/{id}` | Rejected when protected revisions reference the secret. |
| `GET /registry-credentials` | Credential metadata. |
| `POST /registry-credentials` | `{label, registry, username, password}`; current storage is metadata only. |
| `DELETE /registry-credentials/{id}` | Delete metadata. |

All application mutations require `Idempotency-Key`; mutations of existing apps
also require the current `version` in the JSON body. The implementation currently
uses that body field for its optimistic check. Complete request schemas are in
[`api/openapi.yaml`](../../api/openapi.yaml).

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

`needs_attention` retains the mutation lock. The current backend does not yet
perform complete automatic restoration after unhealthy updates; a new rollback
cannot bypass that lock. Restart can report success without replacing Pods until
the recorded restart-token issue is corrected. Private registry authentication is
not complete end to end. Public exposure and application data volumes remain
outside the supported local walkthrough. Demo v3 is intentionally unhealthy.

Logout after the walkthrough:

```bash
api --request DELETE "$PLATFORM_API/auth/session"
```
