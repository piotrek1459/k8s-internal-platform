#!/usr/bin/env bash
# Local platform workflow. Requires Bash 3.2+; all cluster commands select the
# repository-local kubeconfig rather than the caller's default Kubernetes context.
set -euo pipefail
source "$(dirname "$0")/lib/dev-common.sh"

usage() {
    cat <<'HELP'
Usage: ./scripts/platform.sh COMMAND

  build              Build the UI, broker, and v1/v2/v3 demo images locally.
  up [--no-ui]       Start the cluster, build, push, install, and open a UI forward.
  ui                 Forward the platform to http://127.0.0.1:8080 (foreground).
  demo v1|v2|v3      Build and push one demo version to the local registry.
  image v1|v2|v3     Print a published demo's cluster-visible digest reference.
  forward APP_ID     Forward one application to http://127.0.0.1:8081.
  token              Print the first-run bootstrap token, if setup is available.
  status             Show the selected cluster's platform and application state.
  logs               Follow broker logs.
  stop               Stop the selected cluster and registry; retain their data.
  tools              Install the pinned Helm CLI into .local/bin/.

Without a command, 'up' is used. Foreground forwards stop with Ctrl+C.
Defaults: platform-local cluster; registry port 5002; API port 6446.
Optional environment settings:
  PLATFORM_CLUSTER_NAME, PLATFORM_REGISTRY_PORT, PLATFORM_API_PORT,
  PLATFORM_UI_PORT, PLATFORM_APP_PORT

Advanced build targets: build-ui; build-demo [native|multi].
CI checks and the historical M0/M1 experiment remain separate scripts.
HELP
}

fail() { printf 'Error: %s\n' "$*" >&2; exit 1; }

configure() {
    LOCAL_CLUSTER="${PLATFORM_CLUSTER_NAME:-platform-local}"
    [[ ${#LOCAL_CLUSTER} -le 40 && "$LOCAL_CLUSTER" =~ ^[a-z0-9]([a-z0-9-]*[a-z0-9])?$ ]] ||
        fail 'PLATFORM_CLUSTER_NAME must be a lowercase DNS label of at most 40 characters.'
    LOCAL_REGISTRY_PORT="${PLATFORM_REGISTRY_PORT:-5002}"
    LOCAL_API_PORT="${PLATFORM_API_PORT:-6446}"
    LOCAL_UI_PORT="${PLATFORM_UI_PORT:-8080}"
    LOCAL_APP_PORT="${PLATFORM_APP_PORT:-8081}"
    local port seen=' '
    for port in "$LOCAL_REGISTRY_PORT" "$LOCAL_API_PORT" "$LOCAL_UI_PORT" "$LOCAL_APP_PORT"; do
        [[ ${#port} -le 5 && "$port" =~ ^[0-9]+$ ]] || fail 'Ports must be decimal integers from 1 to 65535.'
        (( 10#$port >= 1 && 10#$port <= 65535 )) || fail 'Ports must be between 1 and 65535.'
        [[ "$port" == "$((10#$port))" ]] || fail 'Ports must not contain leading zeroes.'
        case "$seen" in *" $port "*) fail 'The API, registry, UI and application forwards need distinct ports.' ;; esac
        seen="$seen$port "
    done
    LOCAL_STATE="$PLATFORM_LOCAL/$LOCAL_CLUSTER"
    LOCAL_KUBECONFIG="$PLATFORM_LOCAL/$LOCAL_CLUSTER.kubeconfig"
    LOCAL_CONTEXT="k3d-$LOCAL_CLUSTER"
    LOCAL_REGISTRY_NAME="$LOCAL_CLUSTER-registry"
    LOCAL_REGISTRY_CONTAINER="k3d-$LOCAL_REGISTRY_NAME"
    LOCAL_REGISTRY_HOST="localhost:$LOCAL_REGISTRY_PORT"
    LOCAL_REGISTRY_CLUSTER="$LOCAL_REGISTRY_CONTAINER:$LOCAL_REGISTRY_PORT"
    LOCAL_REGISTRY_ENDPOINT="$LOCAL_REGISTRY_CONTAINER:5000"
}

local_kubectl() {
    kubectl --kubeconfig "$LOCAL_KUBECONFIG" --context "$LOCAL_CONTEXT" "$@"
}

local_helm() {
    "$PLATFORM_HELM" --kubeconfig "$LOCAL_KUBECONFIG" --kube-context "$LOCAL_CONTEXT" "$@"
}

require_docker() {
    require_command docker
    [[ "$(docker info --format '{{.OSType}}')" == linux ]] || fail 'A running Linux Docker engine is required.'
}

require_target() {
    require_command kubectl
    [[ -f "$LOCAL_KUBECONFIG" ]] || fail "No kubeconfig for $LOCAL_CLUSTER. Run ./scripts/platform.sh up."
    local server
    server="$(local_kubectl config view --minify -o jsonpath='{.clusters[0].cluster.server}')"
    [[ "$server" == "https://127.0.0.1:$LOCAL_API_PORT" ]] || fail "Unexpected local API endpoint: $server"
}

install_tools() (
    require_command curl
    require_command tar
    require_command shasum
    if [[ -x "$PLATFORM_HELM" ]] && [[ "$("$PLATFORM_HELM" version --template '{{.Version}}')" == "$HELM_VERSION" ]]; then
        return
    fi
    local os arch download archive
    case "$(uname -s)" in Darwin) os=darwin ;; Linux) os=linux ;; *) fail 'Only macOS and Linux are supported.' ;; esac
    case "$(uname -m)" in arm64|aarch64) arch=arm64 ;; x86_64|amd64) arch=amd64 ;; *) fail 'Only ARM64 and AMD64 are supported.' ;; esac
    mkdir -p "$PLATFORM_LOCAL/bin"
    download="$(mktemp -d "$PLATFORM_LOCAL/helm-download.XXXXXX")"
    trap 'rm -rf "$download"' EXIT
    archive="helm-${HELM_VERSION}-${os}-${arch}.tar.gz"
    curl --proto '=https' --tlsv1.2 -fsSL --retry 3 "https://get.helm.sh/$archive" -o "$download/$archive"
    curl --proto '=https' --tlsv1.2 -fsSL --retry 3 "https://get.helm.sh/$archive.sha256sum" -o "$download/checksum"
    (cd "$download" && shasum -a 256 -c checksum)
    tar -xzf "$download/$archive" -C "$download"
    install -m 0755 "$download/$os-$arch/helm" "$PLATFORM_HELM"
    require_helm
)

build_ui() {
    require_command node
    require_command npm
    printf '\n==> Building and embedding the administrator UI\n'
    npm --prefix "$PLATFORM_ROOT/web" ci
    npm --prefix "$PLATFORM_ROOT/web" run build
    local embedded="$PLATFORM_ROOT/broker/internal/httpapi/ui_dist"
    find "$embedded" -not -name README.txt -not -path "$embedded" -delete
    cp -R "$PLATFORM_ROOT/web/build/." "$embedded/"
}

build_demo_version() {
    local version="$1" unhealthy=false
    case "$version" in v1|v2) ;; v3) unhealthy=true ;; *) fail 'Demo version must be v1, v2 or v3.' ;; esac
    docker build --provenance=false --build-arg "VERSION=$version" --build-arg "UNHEALTHY=$unhealthy" \
        --tag "platform-demo:$version" "$PLATFORM_ROOT/examples/http-demo"
}

build_demos() {
    local mode="${1:-native}" version unhealthy
    case "$mode" in
        native) for version in v1 v2 v3; do build_demo_version "$version"; done ;;
        multi)
            mkdir -p "$PLATFORM_LOCAL/images"
            if ! docker buildx inspect platform-demo-builder >/dev/null 2>&1; then
                docker buildx create --name platform-demo-builder --driver docker-container >/dev/null
            fi
            for version in v1 v2 v3; do
                unhealthy=false
                [[ "$version" != v3 ]] || unhealthy=true
                docker buildx build --builder platform-demo-builder --platform linux/arm64,linux/amd64 \
                    --provenance=false --build-arg "VERSION=$version" --build-arg "UNHEALTHY=$unhealthy" \
                    --tag "platform-demo:$version" --output "type=oci,dest=$PLATFORM_LOCAL/images/$version.oci.tar" \
                    --metadata-file "$PLATFORM_LOCAL/images/$version-multi.json" \
                    "$PLATFORM_ROOT/examples/http-demo"
            done ;;
        *) fail 'Build mode must be native or multi.' ;;
    esac
}

build_all() {
    require_docker
    build_ui
    printf '\n==> Building the broker image\n'
    docker build --provenance=false --tag platform-broker:dev "$PLATFORM_ROOT/broker"
    build_demos native
    printf '\nBuilt platform-broker:dev and platform-demo:v1/v2/v3. v3 deliberately fails readiness.\n'
}

ensure_registry() {
    local info
    if info="$(docker inspect "$LOCAL_REGISTRY_CONTAINER" 2>/dev/null)"; then
        # Validate existing ownership and published address before starting it.
        printf '%s' "$info" | python3 -c '
import json,sys
d=json.load(sys.stdin)[0]
assert d["Config"].get("Labels",{}).get("k3d.role")=="registry", "Existing container is not a k3d registry"
p=d["HostConfig"]["PortBindings"].get("5000/tcp",[])
assert any(x["HostIp"]=="127.0.0.1" and x["HostPort"]==sys.argv[1] for x in p), "Registry binding differs from the requested loopback port"
' "$LOCAL_REGISTRY_PORT"
        docker start "$LOCAL_REGISTRY_CONTAINER" >/dev/null
    else
        k3d registry create "$LOCAL_REGISTRY_NAME" --port "127.0.0.1:$LOCAL_REGISTRY_PORT"
    fi
}

ensure_cluster() {
    local exists
    exists="$(k3d cluster list -o json | python3 -c 'import json,sys; print("yes" if any(x["name"]==sys.argv[1] for x in json.load(sys.stdin)) else "no")' "$LOCAL_CLUSTER")"
    if [[ "$exists" == yes ]]; then
        # The manually created platform-local installation is reusable when its
        # repository kubeconfig selects exactly the expected local cluster/API.
        require_target
        ensure_registry
        k3d cluster start "$LOCAL_CLUSTER" --wait --timeout 180s
        if ! docker exec "k3d-$LOCAL_CLUSTER-server-0" sh -c \
            'test -f /etc/rancher/k3s/registries.yaml && grep -Fq -- "$1" /etc/rancher/k3s/registries.yaml' sh "$LOCAL_REGISTRY_CLUSTER"; then
            fail 'Existing cluster has no matching registry configuration. Select a fresh PLATFORM_CLUSTER_NAME and unused API/registry ports; no cluster was deleted.'
        fi
    else
        ensure_registry
        k3d cluster create "$LOCAL_CLUSTER" --servers 1 --agents 0 --image "$K3S_IMAGE" \
            --registry-use "$LOCAL_REGISTRY_CLUSTER" --k3s-arg '--secrets-encryption@server:0' \
            --api-port "127.0.0.1:$LOCAL_API_PORT" \
            --kubeconfig-update-default=false --kubeconfig-switch-context=false --wait --timeout 180s
    fi
    mkdir -p "$LOCAL_STATE"
    (umask 077; k3d kubeconfig get "$LOCAL_CLUSTER" > "$LOCAL_KUBECONFIG")
    chmod 600 "$LOCAL_KUBECONFIG"
    require_target
    local_kubectl wait --for=condition=Ready nodes --all --timeout=120s
    local_kubectl apply -f "$PLATFORM_ROOT/dev/namespace.yaml"
}

push_image() {
    local image="$1" version="$2"
    docker tag "$image:$version" "$LOCAL_REGISTRY_HOST/$image:$version"
    docker push "$LOCAL_REGISTRY_HOST/$image:$version"
}

published_digest() {
    local image="$1" version="$2"
    docker image inspect "$LOCAL_REGISTRY_HOST/$image:$version" --format '{{json .RepoDigests}}' | \
        python3 -c 'import json,sys; prefix=sys.argv[1]+"@"; refs=[r for r in json.load(sys.stdin) if r.startswith(prefix)]; assert refs,"Image has not been published to the selected local registry"; print(refs[0].split("@",1)[1])' "$LOCAL_REGISTRY_HOST/$image"
}

demo_reference() {
    case "$1" in v1|v2|v3) ;; *) fail 'Demo version must be v1, v2 or v3.' ;; esac
    local digest
    digest="$(published_digest platform-demo "$1")"
    printf '%s/platform-demo@%s\n' "$LOCAL_REGISTRY_CLUSTER" "$digest"
}

service_json() {
    local_kubectl --request-timeout=10s get --raw \
        "/api/v1/namespaces/platform-system/services/platform:80/proxy/$1"
}

wait_for_api() {
    local attempt response
    for ((attempt=0; attempt<30; attempt++)); do
        response="$(service_json readyz 2>/dev/null)" || response='{}'
        if printf '%s' "$response" | python3 -c 'import json,sys; d=json.load(sys.stdin); sys.exit(0 if d.get("status")=="ready" and d.get("cluster")=="available" else 1)' 2>/dev/null; then
            return
        fi
        sleep 2
    done
    fail 'Broker readiness timed out. Inspect ./scripts/platform.sh logs and status.'
}

install_platform() {
    local version digest
    for version in v1 v2 v3; do push_image platform-demo "$version"; done
    push_image platform-broker dev
    digest="$(published_digest platform-broker dev)"
    cat > "$LOCAL_STATE/values.yaml" <<YAML
insecureRegistries: "$LOCAL_REGISTRY_CLUSTER,$LOCAL_REGISTRY_ENDPOINT"
registryAliases: "$LOCAL_REGISTRY_CLUSTER=$LOCAL_REGISTRY_ENDPOINT"
YAML
    local_helm upgrade --install platform "$PLATFORM_ROOT/deploy/helm/platform" \
        --namespace platform-system --create-namespace --values "$LOCAL_STATE/values.yaml" \
        --set-string "image.repository=$LOCAL_REGISTRY_CLUSTER/platform-broker" \
        --set-string "image.digest=$digest" --wait --timeout 180s
    wait_for_api
    printf '\nPlatform ready in %s. Demo v1 image:\n' "$LOCAL_CLUSTER"
    demo_reference v1
}

forward_ui() {
    require_target
    local page=login
    if service_json api/v1/setup/status | python3 -c 'import json,sys; sys.exit(0 if json.load(sys.stdin)["setupAvailable"] else 1)'; then
        page=setup
        printf 'Retrieve the one-hour bootstrap token in another terminal: ./scripts/platform.sh token\n'
    fi
    printf 'Open http://127.0.0.1:%s/%s\nKeep this terminal running; Ctrl+C stops only the forward.\n' "$LOCAL_UI_PORT" "$page"
    printf 'If the port is occupied, stop the previous forward or set PLATFORM_UI_PORT.\n'
    local_kubectl -n platform-system port-forward --address 127.0.0.1 service/platform "$LOCAL_UI_PORT:80"
}

print_token() {
    require_target
    if ! service_json api/v1/setup/status | python3 -c 'import json,sys; sys.exit(0 if json.load(sys.stdin)["setupAvailable"] else 1)'; then
        fail 'Setup is closed or its one-hour token has expired. Use /login after account creation; token renewal is described in docs/user/README.md.'
    fi
    local_kubectl -n platform-system logs deployment/platform | python3 -c '
import re,sys
m=re.search(r"Bootstrap token[^\n]*\n\s*([A-Za-z0-9_-]+)",sys.stdin.read())
if not m: sys.exit("No generated token in current pod logs; check the installer-provided token.")
print(m.group(1))'
}

forward_app() {
    require_target
    [[ "$1" =~ ^[a-zA-Z0-9_-]+$ ]] || fail 'An application ID from the UI or API is required.'
    local service
    service="$(local_kubectl -n platform-apps get services -l "platform.local/app-id=$1" -o json | python3 -c 'import json,sys; a=json.load(sys.stdin)["items"]; assert len(a)==1,"Expected one Service for this application"; print(a[0]["metadata"]["name"])')"
    printf 'Application URL: http://127.0.0.1:%s/version\nReconnect this forward after a Pod-replacing update.\n' "$LOCAL_APP_PORT"
    local_kubectl -n platform-apps port-forward --address 127.0.0.1 "service/$service" "$LOCAL_APP_PORT:80"
}

main() {
    local command="${1:-up}"
    [[ $# == 0 ]] || shift
    case "$command" in help|-h|--help) usage; return ;; esac
    configure
    case "$command" in
        tools) [[ $# == 0 ]] || fail 'tools takes no arguments.'; install_tools ;;
        build) [[ $# == 0 ]] || fail 'build takes no arguments.'; build_all ;;
        build-ui) [[ $# == 0 ]] || fail 'build-ui takes no arguments.'; build_ui ;;
        build-demo) [[ $# -le 1 ]] || fail 'build-demo accepts native or multi.'; require_docker; build_demos "${1:-native}" ;;
        up)
            [[ $# == 0 || ( $# == 1 && "$1" == --no-ui ) ]] || fail 'Usage: platform.sh up [--no-ui]'
            require_command k3d; require_command kubectl; require_command python3
            require_command node; require_command npm; require_docker
            install_tools
            ensure_cluster
            build_all
            install_platform
            if [[ $# == 0 ]]; then forward_ui; fi ;;
        ui) [[ $# == 0 ]] || fail 'ui takes no arguments.'; require_command python3; forward_ui ;;
        token) [[ $# == 0 ]] || fail 'token takes no arguments.'; require_command python3; print_token ;;
        demo)
            [[ $# == 1 ]] || fail 'Usage: platform.sh demo v1|v2|v3'
            case "$1" in v1|v2|v3) ;; *) fail 'Demo version must be v1, v2 or v3.' ;; esac
            require_command python3; require_docker
            docker inspect "$LOCAL_REGISTRY_CONTAINER" >/dev/null 2>&1 || fail 'Run platform.sh up to create the local registry first.'
            build_demo_version "$1"; push_image platform-demo "$1"; demo_reference "$1" ;;
        image) [[ $# == 1 ]] || fail 'Usage: platform.sh image v1|v2|v3'; require_command docker; require_command python3; demo_reference "$1" ;;
        forward) [[ $# == 1 ]] || fail 'Usage: platform.sh forward APP_ID'; require_command python3; forward_app "$1" ;;
        status) [[ $# == 0 ]] || fail 'status takes no arguments.'; require_target; local_kubectl get nodes; local_kubectl -n platform-system get deployment,pod,service; local_kubectl -n platform-apps get deployment,pod,service ;;
        logs) [[ $# == 0 ]] || fail 'logs takes no arguments.'; require_target; local_kubectl -n platform-system logs deployment/platform --tail=100 --follow ;;
        stop) [[ $# == 0 ]] || fail 'stop takes no arguments.'; require_target; require_command k3d; require_docker; k3d cluster stop "$LOCAL_CLUSTER"; docker stop "$LOCAL_REGISTRY_CONTAINER" >/dev/null; printf 'Stopped %s; cluster and registry data retained.\n' "$LOCAL_CLUSTER" ;;
        *) usage >&2; fail "Unknown command: $command" ;;
    esac
}

if [[ "${BASH_SOURCE[0]}" == "$0" ]]; then main "$@"; fi
