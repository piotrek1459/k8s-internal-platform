#!/usr/bin/env bash
# Hermetic UI checks: install deps, type-check with svelte-check, lint, build.
# Suitable for CI. Does not start a browser or require a running broker.
#
# Usage:
#   ./scripts/check-ui.sh
set -euo pipefail
source "$(dirname "$0")/lib/dev-common.sh"
require_command node
require_command npm

UI_DIR="$PLATFORM_ROOT/web"

printf '==> Installing npm dependencies...\n'
npm --prefix "$UI_DIR" ci

printf '==> Type-checking (svelte-check)...\n'
npm --prefix "$UI_DIR" run check

printf '==> Linting (eslint)...\n'
npm --prefix "$UI_DIR" run lint

printf '==> Building (vite build)...\n'
npm --prefix "$UI_DIR" run build

printf '==> UI checks passed.\n'
