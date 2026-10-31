#!/usr/bin/env bash
set -euo pipefail
source "$(dirname "$0")/lib/dev-common.sh"
require_command docker
mode="${1:-native}"
mkdir -p "$PLATFORM_LOCAL/images"

case "$mode" in
    native)
        docker_arch="$(docker version --format '{{.Server.Arch}}')"
        case "$docker_arch" in arm64|amd64) ;; *) printf 'Unsupported Docker architecture: %s\n' "$docker_arch" >&2; exit 1 ;; esac
        build_options=(--platform "linux/$docker_arch" --load)
        ;;
    multi)
        # OCI archives stay local; no registry upload is performed.
        if ! docker buildx inspect platform-demo-builder >/dev/null 2>&1; then
            docker buildx create --name platform-demo-builder --driver docker-container >/dev/null
        fi
        build_options=(--builder platform-demo-builder --platform linux/arm64,linux/amd64)
        ;;
    *) printf 'Usage: %s [native|multi]\n' "$0" >&2; exit 2 ;;
esac

for version in v1 v2 v3; do
    unhealthy=false
    [[ "$version" != v3 ]] || unhealthy=true
    version_options=("${build_options[@]}")
    if [[ "$mode" == multi ]]; then
        version_options+=(--output "type=oci,dest=$PLATFORM_LOCAL/images/$version.oci.tar")
    fi
    docker buildx build "${version_options[@]}" \
        --provenance=false --build-arg "VERSION=$version" --build-arg "UNHEALTHY=$unhealthy" \
        --tag "platform-demo:$version" \
        --metadata-file "$PLATFORM_LOCAL/images/$version-$mode.json" \
        "$PLATFORM_ROOT/examples/http-demo"
    if [[ "$mode" == native ]]; then
        # Tag and push via the host-side port (localhost:5001). Docker records the
        # digest under this address; callers swap it for the cluster-visible name
        # (k3d-platform-registry:5001) when building image references.
        docker tag "platform-demo:$version" "localhost:5001/platform-demo:$version"
        docker push "localhost:5001/platform-demo:$version"
    fi
done
if [[ "$mode" == native ]]; then
    printf 'Completed native demonstration image builds and pushed to %s.\n' "$PLATFORM_REGISTRY"
else
    printf 'Completed %s demonstration image builds. No images were published.\n' "$mode"
fi
