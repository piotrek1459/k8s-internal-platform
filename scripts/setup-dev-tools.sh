#!/usr/bin/env bash
# Compatibility entry point; implementation lives in platform.sh.
set -euo pipefail
exec "$(dirname "$0")/platform.sh" tools "$@"
