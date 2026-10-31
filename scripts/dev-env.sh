#!/usr/bin/env bash
# Spin up a complete local development environment: k3d cluster with a local
# registry, broker image built and installed via the platform chart, and a
# port-forward so the management API is reachable at http://127.0.0.1:8080.
#
# Usage:
#   ./scripts/dev-env.sh up      — start everything (idempotent)
#   ./scripts/dev-env.sh down    — tear everything down
#   ./scripts/dev-env.sh status  — show cluster + broker state
#   ./scripts/dev-env.sh token   — print the bootstrap token from broker logs
set -euo pipefail
source "$(dirname "$0")/lib/dev-common.sh"

require_command docker
require_command k3d
require_command kubectl
require_helm

BROKER_IMAGE="$PLATFORM_REGISTRY/platform-broker:dev"
PLATFORM_SYSTEM_NS=platform-system
PLATFORM_CHART_DIR="$PLATFORM_ROOT/deploy/helm/platform"

_broker_running() {
    platform_kubectl -n "$PLATFORM_SYSTEM_NS" get deployment platform --no-headers 2>/dev/null \
        | awk '{print $2}' | grep -q '^[1-9]' 2>/dev/null
}

cmd_up() {
    printf '==> Starting cluster and registry...\n'
    "$PLATFORM_ROOT/scripts/dev-cluster.sh" up

    printf '\n==> Building demo images and pushing to local registry...\n'
    "$PLATFORM_ROOT/scripts/build-demo.sh" native

    printf '\n==> Building broker image...\n'
    docker build -t "$BROKER_IMAGE" "$PLATFORM_ROOT/broker"
    docker push "${BROKER_IMAGE/$PLATFORM_REGISTRY/localhost:5001}"

    printf '\n==> Installing platform chart...\n'
    PUSH_REF="${BROKER_IMAGE/$PLATFORM_REGISTRY/localhost:5001}"
    RAW_DIGEST=$(docker image inspect --format '{{index .RepoDigests 0}}' "$PUSH_REF" 2>/dev/null || \
                 docker image inspect --format '{{index .RepoDigests 0}}' "$BROKER_IMAGE" 2>/dev/null)
    # Swap the push host for the cluster-visible registry name so k3d nodes can pull it.
    DIGEST="${RAW_DIGEST/localhost:5001/$PLATFORM_REGISTRY}"
    REPO="${DIGEST%%@*}"
    SHA="${DIGEST##*@}"

    if platform_helm -n "$PLATFORM_SYSTEM_NS" status platform >/dev/null 2>&1; then
        printf '    Platform release already exists — upgrading.\n'
        platform_helm upgrade platform "$PLATFORM_CHART_DIR" \
            --namespace "$PLATFORM_SYSTEM_NS" --create-namespace \
            --set "image.repository=$REPO" \
            --set "image.digest=$SHA" \
            --set "image.pullPolicy=Always" \
            --wait --timeout 120s
    else
        platform_kubectl create namespace "$PLATFORM_SYSTEM_NS" 2>/dev/null || true
        platform_helm install platform "$PLATFORM_CHART_DIR" \
            --namespace "$PLATFORM_SYSTEM_NS" \
            --set "image.repository=$REPO" \
            --set "image.digest=$SHA" \
            --set "image.pullPolicy=Always" \
            --wait --timeout 120s
    fi

    printf '\n==> Broker is up. Starting port-forward to http://127.0.0.1:8080 ...\n'
    # Kill any stale port-forward for this service.
    pkill -f "port-forward svc/platform 8080" 2>/dev/null || true
    kubectl --kubeconfig "$PLATFORM_KUBECONFIG" --context "$PLATFORM_CONTEXT" \
        -n "$PLATFORM_SYSTEM_NS" port-forward svc/platform 8080:80 \
        >/dev/null 2>&1 &
    PF_PID=$!
    sleep 2
    if ! kill -0 "$PF_PID" 2>/dev/null; then
        printf 'Port-forward failed to start.\n' >&2; exit 1
    fi
    printf '    Port-forward PID %d running in background.\n' "$PF_PID"

    printf '\n==> Waiting for /readyz ...\n'
    for ((i=0; i<30; i++)); do
        if curl -fsS http://127.0.0.1:8080/readyz >/dev/null 2>&1; then
            printf '    /readyz OK\n'; break
        fi
        sleep 2
    done

    printf '\n==> Bootstrap token (one-time use):\n'
    platform_kubectl -n "$PLATFORM_SYSTEM_NS" logs deploy/platform 2>/dev/null \
        | grep -A1 'Bootstrap token' | tail -1 || printf '    (token already consumed or not yet printed — check logs manually)\n'

    printf '\n==> Dev environment ready.\n'
    printf '    API:       http://127.0.0.1:8080\n'
    printf '    Kubeconfig: %s\n' "$PLATFORM_KUBECONFIG"
    printf '\n    Quick start:\n'
    printf '      # 1. Set up admin account (one-time):\n'
    printf '      TOKEN=<token-from-above>\n'
    printf '      curl -fsS -X POST http://127.0.0.1:8080/api/v1/setup \\\n'
    printf '        -H "Content-Type: application/json" \\\n'
    printf '        -H "X-Requested-With: XMLHttpRequest" \\\n'
    printf '        -d "{\"token\":\"$TOKEN\",\"password\":\"changeme123456\"}" -c /tmp/platform-cookies.txt\n'
    printf '\n      # 2. Login (creates a session cookie):\n'
    printf '      curl -fsS -X POST http://127.0.0.1:8080/api/v1/session \\\n'
    printf '        -H "Content-Type: application/json" \\\n'
    printf '        -H "X-Requested-With: XMLHttpRequest" \\\n'
    printf '        -d '"'"'{"password":"changeme123456"}'"'"' -c /tmp/platform-cookies.txt -b /tmp/platform-cookies.txt\n'
    printf '\n      # 3. Deploy the demo app:\n'
    printf '      DEMO_IMAGE=$(docker image inspect --format '"'"'{{index .RepoDigests 0}}'"'"' k3d-platform-registry:5001/platform-demo:v1)\n'
    printf '      curl -fsS -X POST http://127.0.0.1:8080/api/v1/applications \\\n'
    printf '        -H "Content-Type: application/json" \\\n'
    printf '        -H "X-Requested-With: XMLHttpRequest" \\\n'
    printf '        -b /tmp/platform-cookies.txt \\\n'
    printf '        -d "{\"name\":\"demo\",\"image\":\"$DEMO_IMAGE\",\"containerPort\":8080,\"profile\":\"http\",\"exposure\":{\"mode\":\"internal\"}}"\n'
}

cmd_down() {
    pkill -f "port-forward svc/platform 8080" 2>/dev/null || true
    "$PLATFORM_ROOT/scripts/dev-cluster.sh" down
}

cmd_status() {
    require_local_cluster
    printf '==> Nodes:\n'
    platform_kubectl get nodes
    printf '\n==> Platform system (%s):\n' "$PLATFORM_SYSTEM_NS"
    platform_kubectl -n "$PLATFORM_SYSTEM_NS" get deployment,pod,service 2>/dev/null || printf '    (namespace not yet created)\n'
    printf '\n==> Platform apps (%s):\n' "$PLATFORM_NAMESPACE"
    platform_kubectl -n "$PLATFORM_NAMESPACE" get deployment,pod,service,ingress 2>/dev/null || true
}

cmd_token() {
    require_local_cluster
    platform_kubectl -n "$PLATFORM_SYSTEM_NS" logs deploy/platform 2>/dev/null \
        | grep -A1 'Bootstrap token' | tail -1 \
        || printf '(token already consumed or broker not yet running)\n'
}

case "${1:-up}" in
    up)     cmd_up ;;
    down)   cmd_down ;;
    status) cmd_status ;;
    token)  cmd_token ;;
    *) printf 'Usage: %s [up|down|status|token]\n' "$0" >&2; exit 2 ;;
esac
