# Script inventory

## Current local workflow

**`platform.sh` is the entry point for building and running the application.**
Run `./scripts/platform.sh --help` for commands, or follow the root README.
Foreground forwarding makes process ownership explicit; stopping a forward does
not delete its application or cluster data. No broad process-name matching or
automatic cluster deletion is used by the current workflow.

## Compatibility wrappers

| Old command | Current implementation |
| --- | --- |
| `dev-env.sh up` | `platform.sh up` |
| `dev-env.sh status`, `token` | Corresponding platform command. |
| `dev-env.sh down` | `platform.sh stop`; data is retained. |
| `build-ui.sh` | `platform.sh build-ui` |
| `build-demo.sh native` | `platform.sh build-demo native`; images remain local. |
| `build-demo.sh multi` | `platform.sh build-demo multi` |
| `setup-dev-tools.sh` | `platform.sh tools` |

The wrappers contain no second build or startup implementation. New documentation
and local workflows use `platform.sh` directly.

## Checks and other deliverables

- `check-broker.sh`, `check-ui.sh`, `check-api.sh`, `check-chart.sh`: focused checks
  used during development and CI.
- `build-book.sh`: independent thesis LaTeX/Docker build.
- `dev-cluster.sh`, `test-local.sh`: historical M0/M1 manual Helm baseline in
  `platform-dev`; separate from the administrator platform in `platform-local`.
- `lib/`: shared paths, OpenAPI validation, transcript handling and experiment
  assertions. These are supporting code, not additional startup commands.

The baseline scripts remain available to reproduce thesis experiments. Their
local cluster and port conventions are recorded in `docs/local-development.md`.
