#!/usr/bin/env bash
set -euo pipefail
source "$(dirname "$0")/lib/dev-common.sh"
source "$PLATFORM_ROOT/scripts/lib/transcript.sh"
require_helm
require_local_cluster
require_command docker
require_command k3d
require_command curl
require_command python3

run_id="$(date -u +%Y%m%dT%H%M%SZ)-$$"
results="$PLATFORM_ROOT/tests/experiments/runs/$run_id"
mkdir -p "$results"
start_transcript "$results/transcript.log"
printf 'Local K3s lifecycle experiment: %s\n' "$run_id"
printf 'Results: %s\n' "$results"
printf 'Environment: local Docker/K3s; these are not Raspberry Pi measurements.\n'

run() {
    printf '\n$'
    printf ' %q' "$@"
    printf '\n'
    "$@"
}

diagnostics() {
    platform_kubectl -n "$PLATFORM_NAMESPACE" get deployment,pod,service,ingress,endpointslices -o yaml > "$results/resources.yaml" 2>&1 || true
    platform_kubectl -n "$PLATFORM_NAMESPACE" get events --sort-by=.metadata.creationTimestamp > "$results/events.txt" 2>&1 || true
    platform_helm history demo --namespace "$PLATFORM_NAMESPACE" -o json > "$results/helm-history.json" 2>&1 || true
}
finish() {
    local status=$?
    trap - EXIT
    diagnostics
    if [[ "$status" == 0 ]]; then
        printf '\nPASS: deployment, update, failed rollout, rollback, non-default port, and route isolation.\n'
    else
        printf '\nFAIL: diagnostics retained at %s.\n' "$results" >&2
    fi
    finish_transcript "$status" || status=$?
    printf '%s\n' "$status" > "$results/exit-code.txt"
    exit "$status"
}
trap finish EXIT

git -C "$PLATFORM_ROOT" rev-parse HEAD > "$results/commit.txt"
git -C "$PLATFORM_ROOT" status --short > "$results/worktree-status.txt"
git -C "$PLATFORM_ROOT" diff --binary > "$results/tracked-changes.patch"
# Capture implementation sources, including untracked files, without kubeconfig
# or unrelated credentials. The archive makes an uncommitted run reproducible.
tar -czf "$results/implementation.tar.gz" -C "$PLATFORM_ROOT" \
    examples/http-demo deploy/helm/application dev scripts/lib tests/test_local_helpers.py \
    scripts/setup-dev-tools.sh scripts/dev-cluster.sh scripts/build-demo.sh \
    scripts/check-chart.sh scripts/test-local.sh
platform_kubectl version -o json > "$results/kubernetes-version.json"
platform_kubectl get nodes -o json > "$results/nodes.json"
docker version > "$results/docker-version.txt"
docker info --format '{{json .}}' | python3 -c 'import json,sys; d=json.load(sys.stdin); print(json.dumps({k:d.get(k) for k in ("OperatingSystem","OSType","Architecture","NCPU","MemTotal","Driver")},indent=2))' > "$results/docker-host.json"
k3d version > "$results/k3d-version.txt"
"$PLATFORM_HELM" version > "$results/helm-version.txt"
df -h "$PLATFORM_ROOT" > "$results/host-disk.txt"
printf 'scenario,duration_seconds\n' > "$results/timings.csv"

run "$PLATFORM_ROOT/scripts/check-chart.sh"
for version in v1 v2 v3; do
    docker image inspect "platform-demo:$version" --format '{{.Id}} {{.Os}}/{{.Architecture}}' > "$results/image-$version.txt"
done
run k3d image import platform-demo:v1 platform-demo:v2 platform-demo:v3 --cluster "$PLATFORM_CLUSTER"
"$PLATFORM_HELM" template demo "$PLATFORM_CHART" --namespace "$PLATFORM_NAMESPACE" --values "$PLATFORM_VALUES" > "$results/rendered.yaml"
run platform_kubectl apply --dry-run=server -f "$results/rendered.yaml"

# Only the fixed demonstration release in the owned disposable cluster is reset.
run platform_helm uninstall demo --namespace "$PLATFORM_NAMESPACE" --ignore-not-found --wait --timeout 120s

assert_version() {
    local expected="$1"
    local response
    for ((attempt=0; attempt<30; attempt++)); do
        response="$(curl --noproxy '*' -fsS --max-time 3 -H 'Host: demo.localhost' http://127.0.0.1:8082/version 2>/dev/null)" || response='{}'
        if printf '%s' "$response" | python3 -c 'import json,sys; sys.exit(0 if json.load(sys.stdin).get("version") == sys.argv[1] else 1)' "$expected" 2>/dev/null; then
            printf 'Verified private route: %s\n' "$response"
            printf '%s\n' "$response" >> "$results/responses.jsonl"
            return 0
        fi
        sleep 2
    done
    printf 'Expected version %s through the private route. Last response: %s\n' "$expected" "$response" >&2
    return 1
}

started=$SECONDS
run platform_helm install demo "$PLATFORM_CHART" --namespace "$PLATFORM_NAMESPACE" --values "$PLATFORM_VALUES" --wait --timeout 120s
assert_version v1
printf 'install_v1,%s\n' "$((SECONDS-started))" >> "$results/timings.csv"

started=$SECONDS
run platform_helm upgrade demo "$PLATFORM_CHART" --namespace "$PLATFORM_NAMESPACE" --values "$PLATFORM_VALUES" --set image.tag=v2 --wait --timeout 120s
assert_version v2
printf 'upgrade_v2,%s\n' "$((SECONDS-started))" >> "$results/timings.csv"
previous_revision="$(platform_helm status demo --namespace "$PLATFORM_NAMESPACE" -o json | python3 -c 'import json,sys; print(json.load(sys.stdin)["version"])')"

started=$SECONDS
if run platform_helm upgrade demo "$PLATFORM_CHART" --namespace "$PLATFORM_NAMESPACE" --values "$PLATFORM_VALUES" --set image.tag=v3 --wait --timeout 30s; then
    printf 'The deliberately unhealthy v3 upgrade unexpectedly succeeded.\n' >&2
    exit 1
fi
printf 'failed_upgrade_v3,%s\n' "$((SECONDS-started))" >> "$results/timings.csv"
platform_helm status demo --namespace "$PLATFORM_NAMESPACE" -o json > "$results/failed-release.json"
python3 -c 'import json,sys; d=json.load(open(sys.argv[1])); assert d["info"]["status"] == "failed", d["info"]["status"]' "$results/failed-release.json"
platform_kubectl -n "$PLATFORM_NAMESPACE" get pods -l app.kubernetes.io/instance=demo -o json > "$results/failed-pods.json"
python3 - "$results/failed-pods.json" <<'PY'
import json, sys
pods = json.load(open(sys.argv[1]))['items']
bad = [p for p in pods if p['spec']['containers'][0]['image'].endswith(':v3')]
assert bad, 'No v3 pod exists; failure may be unrelated to readiness'
assert all(any(s.get('state', {}).get('running') for s in p['status'].get('containerStatuses', [])) for p in bad), 'The v3 process must be running'
assert all(all(s.get('started') is True and s.get('restartCount') == 0 for s in p['status'].get('containerStatuses', [])) for p in bad), 'Startup must succeed without container restarts'
assert all(not any(c['type'] == 'Ready' and c['status'] == 'True' for c in p['status'].get('conditions', [])) for p in bad), 'v3 must remain unready'
PY
platform_kubectl -n "$PLATFORM_NAMESPACE" get events -o json > "$results/failed-events.json"
python3 - "$results/failed-pods.json" "$results/failed-events.json" <<'PY'
import json, sys
pods = json.load(open(sys.argv[1]))['items']
bad_uids = {p['metadata']['uid'] for p in pods if p['spec']['containers'][0]['image'].endswith(':v3')}
events = json.load(open(sys.argv[2]))['items']
assert any(e.get('involvedObject', {}).get('uid') in bad_uids and 'Readiness probe failed' in e.get('message', '') and '503' in e.get('message', '') for e in events), 'Expected evidence of an HTTP 503 readiness failure on v3'
PY
platform_kubectl -n "$PLATFORM_NAMESPACE" describe pods -l app.kubernetes.io/instance=demo > "$results/failed-pod-diagnostics.txt"
platform_kubectl -n "$PLATFORM_NAMESPACE" logs -l app.kubernetes.io/instance=demo --all-containers --tail=100 --prefix > "$results/application-logs.txt"
assert_version v2

started=$SECONDS
run platform_helm rollback demo "$previous_revision" --namespace "$PLATFORM_NAMESPACE" --wait --timeout 120s
run platform_kubectl -n "$PLATFORM_NAMESPACE" rollout status deployment/demo --timeout=120s
rollback_verified=false
# Helm can finish before terminating Pods disappear. Collect bounded, repeated
# snapshots rather than interpreting a surviving v2 HTTP response as recovery.
for ((attempt=0; attempt<30; attempt++)); do
    platform_helm status demo --namespace "$PLATFORM_NAMESPACE" -o json > "$results/rollback-release.json"
    platform_kubectl -n "$PLATFORM_NAMESPACE" get deployment demo -o json > "$results/rollback-deployment.json"
    platform_kubectl -n "$PLATFORM_NAMESPACE" get replicasets -l app.kubernetes.io/instance=demo -o json > "$results/rollback-replicasets.json"
    platform_kubectl -n "$PLATFORM_NAMESPACE" get pods -l app.kubernetes.io/instance=demo -o json > "$results/rollback-pods.json"
    if python3 "$PLATFORM_ROOT/scripts/lib/assert_rollback.py" "$results"; then
        rollback_verified=true
        break
    fi
    sleep 2
done
[[ "$rollback_verified" == true ]] || { printf 'Rollback state verification timed out.\n' >&2; exit 1; }
assert_version v2
printf 'rollback_v2,%s\n' "$((SECONDS-started))" >> "$results/timings.csv"

started=$SECONDS
run platform_helm upgrade demo "$PLATFORM_CHART" --namespace "$PLATFORM_NAMESPACE" \
    --values "$PLATFORM_VALUES" --values "$PLATFORM_CHART/examples/local-port-9090.yaml" \
    --set image.tag=v2 --wait --timeout 120s
run platform_kubectl -n "$PLATFORM_NAMESPACE" rollout status deployment/demo --timeout=120s
assert_version v2
platform_kubectl -n "$PLATFORM_NAMESPACE" get deployment demo -o json > "$results/port-9090-deployment.json"
printf 'port_9090,%s\n' "$((SECONDS-started))" >> "$results/timings.csv"

code="$(curl --noproxy '*' -sS --max-time 5 -o "$results/wrong-listener-body.txt" -w '%{http_code}' -H 'Host: demo.localhost' http://127.0.0.1:8083/version)"
[[ "$code" == 404 ]] || { printf 'Private route unexpectedly reachable on other listener: HTTP %s\n' "$code" >&2; exit 1; }
printf 'Forged private Host on other listener: HTTP %s (expected 404).\n' "$code"

platform_kubectl top nodes > "$results/node-usage.txt" 2>&1 || printf 'Node metrics were unavailable.\n'
platform_kubectl -n "$PLATFORM_NAMESPACE" top pods > "$results/pod-usage.txt" 2>&1 || printf 'Pod metrics were unavailable.\n'
docker stats --no-stream k3d-platform-dev-server-0 k3d-platform-dev-serverlb > "$results/container-usage.txt"
printf 'version_2_ready=true\nrollback_state_verified=true\nport_9090_ready=true\nprivate_route_rejected_on_other_listener=true\n' > "$results/assertions.txt"
