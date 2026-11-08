#!/usr/bin/env bash
# Compatibility entry point for the unified local platform workflow.
set -euo pipefail
command="${1:-up}"
[[ $# == 0 ]] || shift
if [[ "$command" == down ]]; then
    printf 'dev-env.sh down now stops the environment and retains its data.\n' >&2
    command=stop
fi
exec "$(dirname "$0")/platform.sh" "$command" "$@"
