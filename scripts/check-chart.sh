#!/usr/bin/env bash
set -euo pipefail
source "$(dirname "$0")/lib/dev-common.sh"
require_helm

"$PLATFORM_HELM" lint --strict "$PLATFORM_CHART" --values "$PLATFORM_VALUES"
"$PLATFORM_HELM" template demo "$PLATFORM_CHART" --namespace "$PLATFORM_NAMESPACE" --values "$PLATFORM_VALUES" >/dev/null
"$PLATFORM_HELM" template demo "$PLATFORM_CHART" --namespace "$PLATFORM_NAMESPACE" --values "$PLATFORM_VALUES" \
    --set replicas=0,ingress.enabled=false,probes.readiness.type=tcp >/dev/null
"$PLATFORM_HELM" template demo "$PLATFORM_CHART" --namespace "$PLATFORM_NAMESPACE" --values "$PLATFORM_VALUES" \
    --set-string image.tag= --set-string "image.digest=sha256:$(printf '%064d' 0)" >/dev/null

expect_invalid() {
    local label="$1"
    shift
    if "$PLATFORM_HELM" template demo "$PLATFORM_CHART" --values "$PLATFORM_VALUES" "$@" >/dev/null 2>&1; then
        printf 'Expected chart validation to reject %s.\n' "$label" >&2
        exit 1
    fi
    printf 'Rejected invalid configuration: %s.\n' "$label"
}
expect_invalid 'negative replicas' --set replicas=-1
expect_invalid 'out-of-range port' --set containerPort=65536
expect_invalid 'missing image selector' --set-string image.tag=
expect_invalid 'tag and digest together' --set-string "image.digest=sha256:$(printf '%064d' 0)"
expect_invalid 'missing application identity' --set-string appId=
expect_invalid 'public entrypoint' --set ingress.entrypoint=web
expect_invalid 'missing ingress hostname' --set-string ingress.hostname=
expect_invalid 'disabled readiness' --set probes.readiness.enabled=false
expect_invalid 'duplicated environment variable' --set-string env.TOKEN=example \
    --set-string 'secretRefs[0].name=TOKEN,secretRefs[0].secretName=existing,secretRefs[0].key=value'
printf 'Chart rendering and input validation passed.\n'
