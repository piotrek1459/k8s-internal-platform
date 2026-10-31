# Decision: M3 UI stack

**Date:** 2026-10-31
**Milestone:** M3 — Administrator UI
**Status:** Accepted

## Context

`docs/milestones.md` lists "UI stack" as an open decision required before M3, noting
React + TypeScript as the proposed approach subject to a recorded decision. The
architecture specifies "built static assets served by the Go service" with no separate
Node.js runtime in production.

The user's top priority for the UI is: **easy to extend with new features** as the
platform grows through M4–M8, and minimalistic with low ceremony.

## Decision

**Svelte 5 + SvelteKit (SPA / adapter-static mode) + Tailwind CSS + TypeScript.**

No component library is included as a hard dependency. Web components (SAP UI5,
Flowbite, etc.) can be added per-screen in later milestones without framework conflict.

## Rationale

| Criterion | React | Svelte 5 | Decision weight |
|---|---|---|---|
| Runtime bundle size | ~45 KB (React + ReactDOM) | ~10–15 KB | Low — bandwidth not the concern |
| Boilerplate per component | High (hooks, providers, context wiring) | Low (one `.svelte` file, reactive by default) | **High** — every new M4–M8 screen is cheaper |
| Adding a new screen/route | New file + manual route registration | New file in `src/routes/` — auto-picked up by SvelteKit | **High** — directly addresses extensibility goal |
| TypeScript support | First-class | First-class | Equal |
| Web component interop | Works | Works (Svelte compiles to standard custom elements) | Equal |
| Tailwind CSS | Works well | Works well | Equal |
| Node.js in production | Not required (static build) | Not required (adapter-static) | Equal |
| Learning curve | Moderate — requires understanding hooks, closure traps, re-render model | Lower — template syntax, explicit reactivity runes | Moderate |

### Why not UI5 / SAP Web Components as the base framework

UI5 is an enterprise design system (~300 KB CSS/JS) with an opinionated layout shell.
At M3 scope (6–8 screens, single admin) the overhead is disproportionate. The
extensibility benefit of UI5 — consistent wide table layouts, complex form fields, a
design language familiar to SAP enterprise users — is not relevant for this tool.
Keeping the base layer bare (Svelte + Tailwind) allows a specific UI5 widget to be
imported where it genuinely fits later, without the whole design system as a constraint.

### Why not plain HTML + HTMX

HTMX would work for the simple read/list screens, but the 6-step New Application form
with inline image inspection, polled operation progress, and conditional step
visibility is complex enough that a reactive framework pays for itself. Svelte 5's
reactivity runes keep that complexity local to the component.

## Constraints

- No SSR. The Go binary embeds `web/dist/` and serves it as static assets under `/`.
  Requests to unknown paths fall back to `index.html` (SPA history-mode routing).
- No Node.js runtime in the broker container. `npm run build` runs at build time only.
- Vite dev server (port 5173) proxies `/api/` to the broker at `:8080` for local
  iteration without a Go rebuild.

## Alternatives considered

| Option | Reason not chosen |
|---|---|
| React + TypeScript | More boilerplate per component; larger bundle; same capability for this scope |
| Vue 3 | Comparable to Svelte but less ergonomic for server-embedded static builds |
| Plain HTML + vanilla JS | No type safety; form-heavy flows become hard to maintain |
| HTMX | Insufficient for complex multi-step form with reactive validation |
| SAP UI5 as base | Oversized for M3 scope; adds ~300 KB; constrains layout choices for M4–M8 |

## Consequences

- `web/` directory added at repo root following the layout in `docs/milestones.md`.
- `scripts/build-ui.sh` builds the UI; `scripts/check-ui.sh` type-checks and lints.
- `broker/internal/httpapi/ui.go` embeds `web/dist/` and registers the SPA handler.
- The platform Helm chart requires no changes — static assets ship inside the binary.
- Introducing a component library (e.g., a specific UI5 table) in a later milestone is
  straightforward: add the package, import the component, no framework migration needed.
