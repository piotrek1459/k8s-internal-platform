# Contributor guide

Everything you need to develop, test, and ship changes to the platform.

## Contents

- [Repository structure](#repository-structure)
- [Development environment](#development-environment)
- [Running checks](#running-checks)
- [CI workflows](#ci-workflows)
- [Broker development](#broker-development)
- [UI development](#ui-development)
- [Adding an API endpoint](#adding-an-api-endpoint)
- [Configuration reference](#configuration-reference)
- [Scripts reference](#scripts-reference)

---

## Repository structure

```
broker/             Go source (single binary: API + embedded UI + worker)
web/                Svelte 5 SPA (built and embedded into the broker)
api/                OpenAPI specification
deploy/helm/        Helm charts
scripts/            Helper scripts (check-*, dev-*, build-*)
.github/workflows/  CI: broker.yml, ui.yml, api-chart.yml
```

The broker embeds the compiled UI from `broker/internal/httpapi/ui_dist/`. Run
`./scripts/build-ui.sh` to rebuild and copy the assets before building the broker
image.

---

## Development environment

**Prerequisites:** Docker Desktop, k3d, kubectl, Helm, Go ≥ 1.25, Node 22, Python 3.

```bash
./scripts/setup-dev-tools.sh   # downloads pinned Helm into .local/bin/
./scripts/dev-env.sh up        # cluster + registry + broker deployed + port-forward
```

The dev environment creates a k3d cluster (`platform-dev`) with a local registry
(`k3d-platform-registry`, exposed as `localhost:5001` on the host). The broker is
installed via the platform Helm chart with a values override that configures the
k3d registry for insecure (plain HTTP) access.

To rebuild and redeploy after a Go or UI change:

```bash
./scripts/build-ui.sh          # rebuild SPA and copy assets into broker/
docker build -t localhost:5001/platform-broker:dev broker/
docker push localhost:5001/platform-broker:dev
DIGEST=$(docker image inspect --format '{{index .RepoDigests 0}}' localhost:5001/platform-broker:dev \
  | sed 's|localhost:5001|k3d-platform-registry:5001|')
REPO="${DIGEST%%@*}"; SHA="${DIGEST##*@}"
# write a values override then helm upgrade (see dev-env.sh cmd_up for the full snippet)
```

For iterating on the broker only (no UI change), skip `build-ui.sh`.

---

## Running checks

All checks are hermetic — no running cluster required.

```bash
./scripts/check-broker.sh   # gofmt + go vet + go test ./...
./scripts/check-ui.sh       # svelte-check + eslint + vite build
./scripts/check-api.sh      # OpenAPI structural validation
./scripts/check-chart.sh    # helm lint + helm template (both charts)
```

Run all four before opening a PR. CI runs the same checks.

---

## CI workflows

Three GitHub Actions workflows run on every PR and on push to `main`:

### `.github/workflows/broker.yml`
- `gofmt -l` — fails if any file is not formatted
- `go vet ./...`
- `go test ./...`

Working directory: `broker/`. Uses `actions/setup-go@v5` with `go-version-file`.

### `.github/workflows/ui.yml`
- `npm ci`
- `npm run check` (svelte-check)
- `npm run lint` (eslint)
- `npm run build` (vite)

Working directory: `web/`. Uses `actions/setup-node@v4` with npm cache on
`web/package-lock.json`. The lock file must be committed.

### `.github/workflows/api-chart.yml`
- Installs `pyyaml` then runs `python3 scripts/lib/validate_openapi.py api/openapi.yaml`
- `helm lint` + `helm template` for the application chart and the platform chart

---

## Broker development

### Package layout

| Package | Responsibility |
|---|---|
| `platform` | Domain types, port interfaces (Store, ImageInspector, etc.) |
| `applications` | Pure policy: input validation, operation factory methods |
| `store` | SQLite persistence (accept, reads, operations, lifecycle) |
| `httpapi` | HTTP handlers, router, middleware |
| `operations` | Worker (execute), observer (status), recovery (restart scan) |
| `helm` | Helm adapter (install, upgrade, uninstall) |
| `cluster` | Kubernetes client (Secrets, status reads, Ping) |
| `registry` | Image inspection via go-containerregistry |
| `auth` | Session authentication |
| `audit` | Append-only event log |

### Adding a new operation type

1. Add the type constant to `platform/entities.go`.
2. Add a factory method to `applications/lifecycle.go` (mirrors existing methods).
3. The worker dispatches by type in `operations/worker.go` — add a case if a new
   `execute*` function is needed (most types reuse `executeApply`).
4. Add the HTTP handler in `httpapi/mutations.go` and register it in
   `httpapi/mutations.go:registerApplicationMutations`.
5. Add the path to `api/openapi.yaml`.
6. Add unit tests in `applications/applications_test.go` and handler tests in
   `httpapi/applications_test.go` (extend `fakeStore` if new Store methods are needed).

### Store interface

All persistence goes through `platform.Store`. To add a method:

1. Declare it in `platform/ports.go`.
2. Implement it in the appropriate `store/` file (reads, operations, lifecycle).
3. Add a stub to the `fakeStore` in `httpapi/applications_test.go`.

### Formatting

The CI gate is strict: `gofmt -l .` must produce no output. Run
`gofmt -w ./...` from the `broker/` directory before committing.

---

## UI development

The UI is a Svelte 5 SPA in `web/`. It is built with Vite and served by the broker
as embedded static files.

```bash
cd web
npm install
npm run dev        # dev server at http://localhost:5173 (proxies /api to the broker)
npm run check      # svelte-check type checking
npm run lint       # eslint
npm run build      # production build into web/build/
```

The dev server (`vite.config.ts`) proxies `/api` and `/readyz` to
`http://127.0.0.1:8080`, so you need the broker running (`dev-env.sh up`) for API
calls to work. UI-only changes (styling, layout) don't need the broker.

**Key conventions:**

- Svelte 5 runes throughout: `$state`, `$derived`, `$props`, `onMount`.
- `$app/state` (not `$app/stores`) for page and navigation.
- `requireAuth()` returns `void` — do not test its return value.
- `crypto.randomUUID()` is a browser global — no import needed.
- API client is `web/src/lib/api.ts`; types are `web/src/lib/types.ts`.

After a production build, copy the assets into the broker:

```bash
./scripts/build-ui.sh
```

This runs `npm run build` and copies `web/build/` into
`broker/internal/httpapi/ui_dist/`. The broker embeds this directory with
`//go:embed`. Commit both the web source changes and the updated `ui_dist/`.

---

## Adding an API endpoint

1. **Handler** — add to the appropriate `httpapi/*.go` file (mutations, reads, etc.).
2. **Registration** — call `r.mux.HandleFunc(...)` inside the relevant `register*` function.
3. **OpenAPI** — add the path and schemas to `api/openapi.yaml`.
4. **API client** — add a function to `web/src/lib/api.ts`.
5. **Types** — update `web/src/lib/types.ts` if new response shapes are introduced.
6. **Tests** — handler tests in `httpapi/applications_test.go`; extend `fakeStore` if needed.

---

## Configuration reference

### Broker environment variables

| Variable | Default | Description |
|---|---|---|
| `PLATFORM_LISTEN` | `0.0.0.0:8080` | TCP address to listen on |
| `PLATFORM_DB` | `/var/lib/platform/platform.db` | SQLite database path |
| `PLATFORM_APPS_NAMESPACE` | `platform-apps` | Kubernetes namespace for managed apps |
| `PLATFORM_SECURE_COOKIES` | `false` | Set `Secure` flag on session cookies (enable behind HTTPS) |
| `PLATFORM_INSECURE_REGISTRIES` | _(empty)_ | Comma-separated `host:port` list for plain-HTTP registries |
| `PLATFORM_REGISTRY_ALIASES` | _(empty)_ | Comma-separated `from=to` pairs to rewrite registry addresses before inspection |

`PLATFORM_INSECURE_REGISTRIES` and `PLATFORM_REGISTRY_ALIASES` are used together
for k3d development where the registry is exposed on host port 5001 but listens on
container port 5000 internally:

```
PLATFORM_INSECURE_REGISTRIES=k3d-platform-registry:5000
PLATFORM_REGISTRY_ALIASES=k3d-platform-registry:5001=k3d-platform-registry:5000
```

### Platform Helm chart values (`deploy/helm/platform/values.yaml`)

| Value | Default | Description |
|---|---|---|
| `image.repository` | `ghcr.io/piotrek1459/k8s-internal-platform` | Broker image |
| `image.tag` | _(empty)_ | Image tag (use `image.digest` for production) |
| `image.digest` | _(empty)_ | Image digest (takes precedence over tag) |
| `image.pullPolicy` | `IfNotPresent` | Kubernetes pull policy |
| `appsNamespace` | `platform-apps` | Namespace for managed applications |
| `persistence.size` | `1Gi` | PVC size for SQLite |
| `persistence.storageClassName` | _(empty)_ | Storage class (empty = cluster default) |
| `resources.requests.cpu` | `50m` | CPU request |
| `resources.requests.memory` | `64Mi` | Memory request |
| `resources.limits.cpu` | `500m` | CPU limit |
| `resources.limits.memory` | `256Mi` | Memory limit |
| `service.port` | `80` | ClusterIP service port |
| `secureCookies` | `false` | Enable `Secure` cookie flag |
| `insecureRegistries` | _(empty)_ | Plain-HTTP registry hosts (see env var above) |
| `registryAliases` | _(empty)_ | Registry address rewrites (see env var above) |

---

## Scripts reference

| Script | Description |
|---|---|
| `./scripts/setup-dev-tools.sh` | Download pinned Helm binary into `.local/bin/` |
| `./scripts/dev-env.sh up` | Start cluster, build everything, deploy broker, start port-forward |
| `./scripts/dev-env.sh down` | Tear down cluster and stop port-forward |
| `./scripts/dev-env.sh status` | Show cluster and broker pod status |
| `./scripts/dev-env.sh token` | Print the bootstrap token from broker logs |
| `./scripts/dev-cluster.sh up` | Start k3d cluster and registry only |
| `./scripts/dev-cluster.sh down` | Delete k3d cluster and registry |
| `./scripts/build-ui.sh` | Build SPA and copy assets into `broker/internal/httpapi/ui_dist/` |
| `./scripts/build-demo.sh native` | Build demo app image for the host architecture |
| `./scripts/build-demo.sh multi` | Build multi-arch OCI archives into `.local/images/` |
| `./scripts/check-broker.sh` | gofmt + go vet + go test |
| `./scripts/check-ui.sh` | svelte-check + eslint + vite build |
| `./scripts/check-api.sh` | OpenAPI structural validation |
| `./scripts/check-chart.sh` | Helm lint + template (both charts) |
| `./scripts/test-local.sh` | Full K3s lifecycle integration test (requires cluster) |
| `./scripts/build-book.sh` | Build LaTeX thesis PDF |
