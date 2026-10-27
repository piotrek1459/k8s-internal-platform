#!/usr/bin/env bash
# Validate the management API contract in api/openapi.yaml.
#
# Runs the offline structural validator (YAML parse, required top-level fields,
# local $ref resolution) and, when openapi-spec-validator is installed, the full
# OpenAPI specification validation. Mirrors the convention of check-chart.sh.
set -euo pipefail
source "$(dirname "$0")/lib/dev-common.sh"
require_command python3

spec="$PLATFORM_ROOT/api/openapi.yaml"
python3 "$PLATFORM_ROOT/scripts/lib/validate_openapi.py" "$spec"
