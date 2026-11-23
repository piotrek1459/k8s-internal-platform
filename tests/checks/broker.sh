#!/usr/bin/env bash
# Verify the Go broker: formatting, static analysis, and the hermetic unit and
# handler tests. The gated k3d integration suite is run separately (see
# broker/integration and docs/local-development.md). Pins the Go toolchain to the
# module's go directive so dependency version skew is caught early.
set -euo pipefail
source "$(dirname "$0")/../../scripts/lib/dev-common.sh"
require_command go

cd "$PLATFORM_ROOT/broker"

# gofmt: fail if any file is not formatted.
unformatted="$(gofmt -l . || true)"
if [[ -n "$unformatted" ]]; then
    printf 'These files are not gofmt-clean:\n%s\n' "$unformatted" >&2
    exit 1
fi

GOTOOLCHAIN=local go vet ./...
GOTOOLCHAIN=local go test -count=1 ./...
printf 'Broker formatting, vet, and tests passed.\n'

cd "$PLATFORM_ROOT/tests/e2e/fixtures/workload"
unformatted="$(gofmt -l .)"
[[ -z "$unformatted" ]] || { printf 'Unformatted E2E fixture files:\n%s\n' "$unformatted"; exit 1; }
GOTOOLCHAIN=local go vet ./...
GOTOOLCHAIN=local go test -count=1 ./...
printf 'E2E workload fixture formatting, vet, and tests passed.\n'
