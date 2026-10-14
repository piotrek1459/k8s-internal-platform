#!/usr/bin/env bash
set -euo pipefail
source "$(dirname "$0")/lib/dev-common.sh"

require_command docker
require_command k3d
require_command kubectl
require_command python3
action="${1:-up}"

case "$action" in
    up)
        docker info >/dev/null
        mkdir -p "$PLATFORM_LOCAL"
        if k3d cluster list -o json | python3 -c 'import json,sys; sys.exit(0 if any(x["name"] == "platform-dev" for x in json.load(sys.stdin)) else 1)'; then
            [[ -f "$PLATFORM_LOCAL/cluster-owned" ]] || { printf 'Refusing to reuse an unrecognized platform-dev cluster.\n' >&2; exit 1; }
            KUBECONFIG="$PLATFORM_KUBECONFIG" k3d cluster start "$PLATFORM_CLUSTER" --wait --timeout 180s
        else
            KUBECONFIG="$PLATFORM_KUBECONFIG" k3d cluster create "$PLATFORM_CLUSTER" \
                --servers 1 --agents 0 --image "$K3S_IMAGE" \
                --k3s-arg '--secrets-encryption@server:0' \
                --api-port 127.0.0.1:6445 \
                --port '127.0.0.1:8082:8082@loadbalancer' \
                --port '127.0.0.1:8083:80@loadbalancer' \
                --kubeconfig-update-default=false --kubeconfig-switch-context=false \
                --timeout 180s --wait
            printf '%s\n' "$PLATFORM_CLUSTER" > "$PLATFORM_LOCAL/cluster-owned"
        fi
        (umask 077; k3d kubeconfig get "$PLATFORM_CLUSTER" > "$PLATFORM_KUBECONFIG")
        require_local_cluster
        platform_kubectl wait --for=condition=Ready nodes --all --timeout=120s
        platform_kubectl apply -f "$PLATFORM_ROOT/dev/namespace.yaml"
        platform_kubectl apply -f "$PLATFORM_ROOT/dev/traefik-private.yaml"
        # HelmChartConfig reconciliation is asynchronous. Wait for the private
        # entrypoint to appear in the Deployment before observing its rollout.
        private_configured=false
        for ((attempt=0; attempt<90; attempt++)); do
            if platform_kubectl -n kube-system get deployment traefik -o json 2>/dev/null | python3 -c 'import json,sys; d=json.load(sys.stdin); sys.exit(0 if any("entryPoints.private.address" in arg for c in d["spec"]["template"]["spec"]["containers"] for arg in c.get("args", [])) else 1)' 2>/dev/null; then
                private_configured=true
                break
            fi
            sleep 2
        done
        [[ "$private_configured" == true ]] || { printf 'Traefik private entrypoint did not become available.\n' >&2; exit 1; }
        platform_kubectl -n kube-system rollout status deployment/traefik --timeout=120s
        printf 'Local cluster ready. Private HTTP: 127.0.0.1:8082. Negative-test listener: 127.0.0.1:8083.\n'
        printf 'Kubeconfig: %s (the default context was not changed).\n' "$PLATFORM_KUBECONFIG"
        ;;
    down)
        [[ -f "$PLATFORM_LOCAL/cluster-owned" ]] || { printf 'No owned local cluster is recorded.\n' >&2; exit 1; }
        KUBECONFIG="$PLATFORM_KUBECONFIG" k3d cluster delete "$PLATFORM_CLUSTER"
        rm -f "$PLATFORM_LOCAL/cluster-owned" "$PLATFORM_KUBECONFIG"
        ;;
    status)
        require_local_cluster
        platform_kubectl get nodes
        platform_kubectl -n "$PLATFORM_NAMESPACE" get deployment,pod,service,ingress
        ;;
    *) printf 'Usage: %s [up|down|status]\n' "$0" >&2; exit 2 ;;
esac
