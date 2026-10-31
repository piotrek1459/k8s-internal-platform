#!/usr/bin/env bash
# Shared paths and explicit cluster selection for local development commands.

PLATFORM_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
PLATFORM_LOCAL="$PLATFORM_ROOT/.local"
PLATFORM_CLUSTER=platform-dev
PLATFORM_CONTEXT="k3d-$PLATFORM_CLUSTER"
PLATFORM_KUBECONFIG="$PLATFORM_LOCAL/kubeconfig"
PLATFORM_NAMESPACE=platform-apps
PLATFORM_CHART="$PLATFORM_ROOT/deploy/helm/application"
PLATFORM_VALUES="$PLATFORM_CHART/examples/local.yaml"
source "$PLATFORM_ROOT/dev/versions.env"
PLATFORM_HELM="$PLATFORM_LOCAL/bin/helm"
PLATFORM_REGISTRY_NAME=platform-registry
PLATFORM_REGISTRY="k3d-$PLATFORM_REGISTRY_NAME:5001"
# The registry is reachable from the Mac host on localhost:5001 (Docker maps the
# container's port 5000 to host port 5001). The cluster nodes use the DNS name
# k3d-platform-registry:5001 injected by k3d. Push with PLATFORM_REGISTRY_HOST;
# reference images in specs with PLATFORM_REGISTRY.
PLATFORM_REGISTRY_HOST="localhost:5001"

require_command() {
    command -v "$1" >/dev/null 2>&1 || { printf 'Required command is unavailable: %s\n' "$1" >&2; exit 1; }
}

require_helm() {
    if [[ ! -x "$PLATFORM_HELM" ]]; then
        printf 'Helm is unavailable. Run ./scripts/setup-dev-tools.sh first.\n' >&2
        exit 1
    fi
    local actual
    actual="$("$PLATFORM_HELM" version --template '{{.Version}}')"
    [[ "$actual" == "$HELM_VERSION" ]] || { printf 'Expected Helm %s; found %s.\n' "$HELM_VERSION" "$actual" >&2; exit 1; }
}

require_local_cluster() {
    require_command kubectl
    if [[ ! -f "$PLATFORM_LOCAL/cluster-owned" || ! -f "$PLATFORM_KUBECONFIG" ]]; then
        printf 'The managed local cluster is unavailable. Run ./scripts/dev-cluster.sh up.\n' >&2
        exit 1
    fi
    local server
    server="$(kubectl --kubeconfig "$PLATFORM_KUBECONFIG" --context "$PLATFORM_CONTEXT" config view --minify -o jsonpath='{.clusters[0].cluster.server}')"
    [[ "$server" == 'https://127.0.0.1:6445' ]] || { printf 'Unexpected local API endpoint: %s\n' "$server" >&2; exit 1; }
}

platform_kubectl() {
    kubectl --kubeconfig "$PLATFORM_KUBECONFIG" --context "$PLATFORM_CONTEXT" "$@"
}

platform_helm() {
    "$PLATFORM_HELM" --kubeconfig "$PLATFORM_KUBECONFIG" --kube-context "$PLATFORM_CONTEXT" "$@"
}
