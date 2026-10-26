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
"$PLATFORM_HELM" template demo "$PLATFORM_CHART" --namespace "$PLATFORM_NAMESPACE" --values "$PLATFORM_VALUES" \
    --values "$PLATFORM_CHART/examples/local-port-9090.yaml" >/dev/null

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
for probe in readiness startup liveness; do
    # null removes the default path during Helm's values merge. Merely omitting
    # the override would retain the default and would not test a missing path.
    "$PLATFORM_HELM" template demo "$PLATFORM_CHART" --values "$PLATFORM_VALUES" \
        --set "probes.$probe.enabled=true,probes.$probe.type=tcp,probes.$probe.path=null" >/dev/null
    "$PLATFORM_HELM" template demo "$PLATFORM_CHART" --values "$PLATFORM_VALUES" \
        --set "probes.$probe.enabled=true,probes.$probe.type=tcp" \
        --set-string "probes.$probe.path=ignored-for-tcp" >/dev/null
    expect_invalid "$probe HTTP probe without a path" \
        --set "probes.$probe.enabled=true,probes.$probe.path=null"
    expect_invalid "$probe HTTP probe with a relative path" \
        --set "probes.$probe.enabled=true" --set-string "probes.$probe.path=readyz"
    expect_invalid "$probe HTTP probe with an empty path" \
        --set "probes.$probe.enabled=true" --set-string "probes.$probe.path="
done
printf 'Chart rendering and input validation passed.\n'

# The broker embeds a copy of this chart (broker/internal/helm/chart) so it ships
# as a single binary. Verify the embedded copy matches the canonical chart for the
# files they share; the broker copy intentionally omits examples/ and README.md.
embedded="$PLATFORM_ROOT/broker/internal/helm/chart"
if [[ -d "$embedded" ]]; then
    for rel in Chart.yaml values.yaml values.schema.json .helmignore \
        templates/_helpers.tpl templates/deployment.yaml templates/ingress.yaml templates/service.yaml; do
        if ! diff -q "$PLATFORM_CHART/$rel" "$embedded/$rel" >/dev/null 2>&1; then
            printf 'Embedded broker chart is out of sync with the canonical chart: %s\n' "$rel" >&2
            printf 'Re-copy deploy/helm/application into broker/internal/helm/chart.\n' >&2
            exit 1
        fi
    done
    printf 'Embedded broker chart matches the canonical chart.\n'
fi

