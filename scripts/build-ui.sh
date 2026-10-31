#!/usr/bin/env bash
# Build the admin UI and copy the static assets into the broker's embed directory.
# Run this before building the Go binary when you want the UI included.
#
# Usage:
#   ./scripts/build-ui.sh
set -euo pipefail
source "$(dirname "$0")/lib/dev-common.sh"
require_command node
require_command npm

UI_DIR="$PLATFORM_ROOT/web"
EMBED_DIR="$PLATFORM_ROOT/broker/internal/httpapi/ui_dist"

printf '==> Installing npm dependencies...\n'
npm --prefix "$UI_DIR" ci

printf '==> Building admin UI...\n'
npm --prefix "$UI_DIR" run build

printf '==> Copying build output to embed directory...\n'
# Remove everything except the README placeholder, then copy fresh build.
find "$EMBED_DIR" -not -name 'README.txt' -not -path "$EMBED_DIR" -delete
cp -r "$UI_DIR/build/." "$EMBED_DIR/"

printf '==> UI build complete. Assets at %s\n' "$EMBED_DIR"
