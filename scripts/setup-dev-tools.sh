#!/usr/bin/env bash
set -euo pipefail
source "$(dirname "$0")/lib/dev-common.sh"

require_command curl
require_command tar
require_command shasum
case "$(uname -s)" in
    Darwin) platform_os=darwin ;;
    Linux) platform_os=linux ;;
    *) printf 'Supported development systems: macOS and Linux.\n' >&2; exit 1 ;;
esac
case "$(uname -m)" in
    arm64|aarch64) platform_arch=arm64 ;;
    x86_64|amd64) platform_arch=amd64 ;;
    *) printf 'Supported development architectures: ARM64 and AMD64.\n' >&2; exit 1 ;;
esac

mkdir -p "$PLATFORM_LOCAL/bin"
if [[ -x "$PLATFORM_HELM" ]] && [[ "$("$PLATFORM_HELM" version --template '{{.Version}}')" == "$HELM_VERSION" ]]; then
    printf 'Helm %s is already available at %s.\n' "$HELM_VERSION" "$PLATFORM_HELM"
    exit 0
fi

download_dir="$(mktemp -d "$PLATFORM_LOCAL/helm-download.XXXXXX")"
trap 'rm -rf "$download_dir"' EXIT
archive="helm-${HELM_VERSION}-${platform_os}-${platform_arch}.tar.gz"
curl --proto '=https' --tlsv1.2 -fsSL --retry 3 "https://get.helm.sh/$archive" -o "$download_dir/$archive"
curl --proto '=https' --tlsv1.2 -fsSL --retry 3 "https://get.helm.sh/$archive.sha256sum" -o "$download_dir/checksum"
(cd "$download_dir" && shasum -a 256 -c checksum)
tar -xzf "$download_dir/$archive" -C "$download_dir"
install -m 0755 "$download_dir/$platform_os-$platform_arch/helm" "$PLATFORM_HELM"
require_helm
printf 'Installed Helm %s at %s.\n' "$HELM_VERSION" "$PLATFORM_HELM"
