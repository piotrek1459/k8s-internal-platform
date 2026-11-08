# Decision: M3 UI stack

Date: 2026-10-31. Reviewed: 2026-11-08. Status: accepted and implemented.

## Context

The original plan considered React/TypeScript. The selected interface requires
application forms, image inspection, polled operation progress and history, with
static assets served by the Go broker. A separate production Node.js server is
not required. Screen-level extension and a small deployment footprint informed
the choice.

## Decision

The implementation uses Svelte 5, SvelteKit in static SPA mode, TypeScript and
Tailwind CSS. No general component library is required. The packages and lockfile
under `web/` record the build dependencies.

## Rationale and alternatives

File-based routes and reactive components support the form/progress workflow.
Static output can be embedded into the existing Go executable. These are design
reasons for the selected implementation, not measured performance comparisons.

React/TypeScript and Vue were considered viable alternatives. Plain HTML or
server-rendered forms would require a different approach to the current client
workflow. A large component framework was deferred until concrete screens require
it. No bundle-size or framework-performance claim is used as acceptance evidence.

## Build and runtime

- `./scripts/platform.sh build-ui` installs dependencies, builds `web/build/` and
  copies assets into `broker/internal/httpapi/ui_dist/`.
- `platform.sh build` and `platform.sh up` include that preparation before the
  broker image build. Generated assets are ignored by Git.
- `broker/internal/httpapi/ui.go` embeds those copied assets and serves the SPA.
  Specific API routes take precedence; non-API client routes fall back to
  `index.html`.
- Node.js 22/npm are build/development prerequisites. No Node.js runtime is needed
  by the deployed broker container.
- The Vite development server uses port 5173 and proxies `/api/` to the running
  broker forward on 8080. It is a development path, not the production server.

## Verification boundary

`./tests/checks/ui.sh` runs Svelte/TypeScript checks, ESLint and a production
build. These checks do not validate live response shapes or browser workflows.
Manual frontend types and Go responses currently disagree in several places;
[implementation status](../status.md#known-correctness-and-integration-gaps)
records those issues and the need for contract/browser acceptance tests.
