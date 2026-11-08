#!/usr/bin/env bash
# Initial, single-node Linux installation. No application data is deleted.
set -euo pipefail
REPOSITORY=piotrek1459/k8s-internal-platform
STATE_ROOT=/etc/k8s-internal-platform
TOOL_ROOT=/usr/local/lib/k8s-internal-platform
KUBECONFIG_PATH=/etc/rancher/k3s/k3s.yaml
WORK=

fail() { printf 'Error: %s\n' "$*" >&2; exit 1; }
usage() {
    cat <<'HELP'
Usage: sudo bash install.sh --version v0.0.1 [--check]
       sudo bash install.sh --version v0.0.1 --bundle FILE --checksum FILE

Installs the pinned K3s release and imports the platform/demo OCI images.
Requires 64-bit Linux, systemd, memory cgroups, 2 GiB RAM and 6 GiB free disk.
--check performs read-only host checks without downloading or installing.
An unrelated K3s installation is never replaced. Same-version reruns retain data;
upgrading a different installed platform version is not supported by this preview.
HELP
}
cleanup() {
    if [[ -n "$WORK" && "$WORK" == /tmp/platform-install.* && -d "$WORK" ]]; then
        find "$WORK" -depth -delete
    fi
}
architecture() {
    case "$1" in aarch64|arm64) printf 'arm64\n' ;; x86_64|amd64) printf 'amd64\n' ;; *) fail 'Only 64-bit ARM64 and AMD64 are supported; 32-bit Raspberry Pi OS is unsupported.' ;; esac
}
preflight() {
    [[ "$(uname -s)" == Linux ]] || fail 'The host installer requires Linux.'
    [[ "$(id -u)" == 0 ]] || fail 'Run the installer with sudo.'
    for command in curl tar sha256sum awk grep systemctl install df sort uniq; do
        command -v "$command" >/dev/null || fail "Missing required host tool: $command"
    done
    [[ "$(cat /proc/1/comm)" == systemd ]] || fail 'A systemd-based host is required.'
    ARCH="$(architecture "$(uname -m)")"
    if [[ -f /sys/fs/cgroup/cgroup.controllers ]]; then
        grep -qw memory /sys/fs/cgroup/cgroup.controllers || fail 'Memory cgroups are unavailable. On Raspberry Pi OS, enable cgroup_memory=1 cgroup_enable=memory in the single-line /boot/firmware/cmdline.txt (older OS: /boot/cmdline.txt), reboot manually, then rerun.'
    else
        awk '$1=="memory" && $4==1 {found=1} END {exit !found}' /proc/cgroups || fail 'The memory cgroup controller must be enabled before installing K3s.'
    fi
    local memory disk
    memory="$(awk '/^MemTotal:/ {print $2}' /proc/meminfo)"
    disk="$(df -Pk /var/lib | awk 'END {print $4}')"
    (( memory >= 2097152 )) || fail 'This preview requires at least 2 GiB RAM.'
    (( disk >= 6291456 )) || fail 'At least 6 GiB free space under /var/lib is required.'
    if [[ -e "$STATE_ROOT/owner" ]]; then
        [[ "$(cat "$STATE_ROOT/owner")" == "$REPOSITORY" ]] || fail 'The installation marker belongs to another installation.'
        if [[ -f "$STATE_ROOT/release.env" ]]; then
            local installed
            installed="$(awk -F= '$1=="PLATFORM_VERSION" {print $2}' "$STATE_ROOT/release.env")"
            [[ "$installed" == "$REQUESTED_VERSION" ]] || fail "Installed version $installed differs; cross-version upgrades require a verified backup/upgrade procedure."
        fi
    elif [[ -e /usr/local/bin/k3s || -e /etc/rancher/k3s || -e /var/lib/rancher/k3s || -e /etc/systemd/system/k3s.service || -e "$STATE_ROOT" || -e "$TOOL_ROOT" ]]; then
        fail 'Existing K3s/platform state is not owned by this installer; no files were changed.'
    fi
    printf 'Host checks passed: linux/%s.\n' "$ARCH"
}
download() {
    curl --proto '=https' --tlsv1.2 -fsSL --retry 3 --connect-timeout 15 --max-time 900 --max-filesize 1073741824 "$1" -o "$2"
}
verify_bundle() {
    local bundle="$1" checksum="$2" expected="platform-v$REQUESTED_VERSION-linux-$ARCH.tar.gz" record name
    [[ -f "$bundle" && -f "$checksum" ]] || fail 'Bundle and checksum files are required.'
    record="$(awk -v name="$expected" '$2==name {print $1}' "$checksum")"
    [[ "$record" =~ ^[0-9a-f]{64}$ ]] || fail 'Expected exactly one release bundle SHA-256 record.'
    [[ "$(sha256sum "$bundle" | awk '{print $1}')" == "$record" ]] || fail 'Bundle SHA-256 does not match; installation was not started.'
    tar -tzf "$bundle" > "$WORK/members"
    [[ -z "$(sort "$WORK/members" | uniq -d)" ]] || fail 'Duplicate bundle members are not allowed.'
    while IFS= read -r name; do
        case "$name" in release.env|release.json|platform-chart.tgz|namespace.yaml|platform-image.oci.tar|demo-v1.oci.tar|demo-v2.oci.tar|demo-v3.oci.tar) ;; *) fail "Unexpected bundle member: $name" ;; esac
    done < "$WORK/members"
    mkdir "$WORK/bundle"
    tar --no-same-owner --no-same-permissions -xzf "$bundle" -C "$WORK/bundle"
    for name in release.env release.json platform-chart.tgz namespace.yaml platform-image.oci.tar demo-v1.oci.tar demo-v2.oci.tar demo-v3.oci.tar; do
        [[ -f "$WORK/bundle/$name" && ! -L "$WORK/bundle/$name" ]] || fail "Missing/non-regular bundle file: $name"
    done
}
load_metadata() {
    local key value
    while IFS='=' read -r key value; do
        case "$key" in PLATFORM_VERSION|PLATFORM_ARCH|K3S_VERSION|HELM_VERSION|PLATFORM_IMAGE|DEMO_V1_IMAGE|DEMO_V2_IMAGE|DEMO_V3_IMAGE) ;; *) fail 'Unexpected release metadata key.' ;; esac
        [[ "$value" =~ ^[A-Za-z0-9._@:+/-]+$ ]] || fail 'Invalid release metadata value.'
        printf -v "$key" '%s' "$value"
    done < "$WORK/bundle/release.env"
    [[ "${PLATFORM_VERSION:-}" == "$REQUESTED_VERSION" && "${PLATFORM_ARCH:-}" == "$ARCH" ]] || fail 'Release version or architecture does not match this installation.'
    [[ "${K3S_VERSION:-}" =~ ^v[0-9]+\.[0-9]+\.[0-9]+\+k3s[0-9]+$ ]] || fail 'Invalid K3s version pin.'
    [[ "${HELM_VERSION:-}" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]] || fail 'Invalid Helm version pin.'
    for key in PLATFORM_IMAGE DEMO_V1_IMAGE DEMO_V2_IMAGE DEMO_V3_IMAGE; do
        value="${!key:-}"
        [[ "$value" =~ ^platform\.local/[a-z0-9-]+@sha256:[0-9a-f]{64}$ ]] || fail 'Invalid bundled image digest reference.'
    done
}
install_helm() {
    local archive="helm-$HELM_VERSION-linux-$ARCH.tar.gz"
    download "https://get.helm.sh/$archive" "$WORK/$archive"
    download "https://get.helm.sh/$archive.sha256sum" "$WORK/helm.sha256"
    (cd "$WORK" && sha256sum -c helm.sha256)
    tar -xzf "$WORK/$archive" -C "$WORK"
    install -d -m 0755 "$TOOL_ROOT"
    install -m 0755 "$WORK/linux-$ARCH/helm" "$TOOL_ROOT/helm"
}
install_k3s() {
    if [[ -x /usr/local/bin/k3s ]]; then
        [[ "$(/usr/local/bin/k3s --version | awk 'NR==1 {print $3}')" == "$K3S_VERSION" ]] || fail 'Existing managed K3s version differs from the release pin.'
        systemctl start k3s
    else
        install -d -m 0700 "$STATE_ROOT" /etc/rancher/k3s
        printf '%s\n' "$REPOSITORY" > "$STATE_ROOT/owner"
        cat > /etc/rancher/k3s/config.yaml <<'CONFIG'
write-kubeconfig-mode: "0600"
secrets-encryption: true
disable:
  - traefik
  - servicelb
CONFIG
        chmod 0600 /etc/rancher/k3s/config.yaml
        download "https://raw.githubusercontent.com/k3s-io/k3s/$K3S_VERSION/install.sh" "$WORK/k3s-install.sh"
        INSTALL_K3S_VERSION="$K3S_VERSION" INSTALL_K3S_EXEC=server sh "$WORK/k3s-install.sh"
    fi
    /usr/local/bin/k3s kubectl --kubeconfig "$KUBECONFIG_PATH" wait --for=condition=Ready nodes --all --timeout=180s
}
install_platform() {
    local key image archive
    for key in PLATFORM_IMAGE DEMO_V1_IMAGE DEMO_V2_IMAGE DEMO_V3_IMAGE; do
        image="${!key}"
        case "$key" in PLATFORM_IMAGE) archive=platform-image.oci.tar ;; DEMO_V1_IMAGE) archive=demo-v1.oci.tar ;; DEMO_V2_IMAGE) archive=demo-v2.oci.tar ;; DEMO_V3_IMAGE) archive=demo-v3.oci.tar ;; esac
        /usr/local/bin/k3s ctr --namespace k8s.io images import --digests --base-name "${image%@*}" "$WORK/bundle/$archive"
    done
    /usr/local/bin/k3s kubectl --kubeconfig "$KUBECONFIG_PATH" apply -f "$WORK/bundle/namespace.yaml"
    "$TOOL_ROOT/helm" --kubeconfig "$KUBECONFIG_PATH" upgrade --install platform "$WORK/bundle/platform-chart.tgz" \
        --namespace platform-system --create-namespace \
        --set-string "image.repository=${PLATFORM_IMAGE%@*}" --set-string "image.digest=${PLATFORM_IMAGE#*@}" \
        --set image.pullPolicy=Never --wait --timeout 180s
    local attempt response
    for ((attempt=0; attempt<30; attempt++)); do
        response="$(/usr/local/bin/k3s kubectl --kubeconfig "$KUBECONFIG_PATH" --request-timeout=10s get --raw /api/v1/namespaces/platform-system/services/platform:80/proxy/readyz 2>/dev/null)" || response='{}'
        if grep -Eq '"status":[[:space:]]*"ready"' <<< "$response" && grep -Eq '"cluster":[[:space:]]*"available"' <<< "$response"; then
            install -m 0600 "$WORK/bundle/release.env" "$STATE_ROOT/release.env"
            printf 'Installed platform v%s. Metadata is retained across same-version reruns.\n' "$PLATFORM_VERSION"
            printf 'On the Pi: sudo k3s kubectl -n platform-system port-forward --address 127.0.0.1 service/platform 8080:80\n'
            printf 'Use an SSH local forward and open http://127.0.0.1:8080/setup. Retrieve the token from current broker logs.\n'
            return
        fi
        sleep 2
    done
    fail 'Platform readiness was not established. Inspect: sudo k3s kubectl -n platform-system logs deployment/platform'
}
main() {
    REQUESTED_VERSION=; local bundle= checksum= check=false
    while [[ $# -gt 0 ]]; do
        case "$1" in
            -h|--help) usage; return ;;
            --check) check=true; shift ;;
            --version|--bundle|--checksum)
                [[ $# -ge 2 ]] || fail "Missing value for $1"
                case "$1" in --version) REQUESTED_VERSION="${2#v}" ;; --bundle) bundle="$2" ;; --checksum) checksum="$2" ;; esac
                shift 2 ;;
            *) fail "Unknown argument: $1" ;;
        esac
    done
    [[ "$REQUESTED_VERSION" =~ ^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$ ]] || fail 'Supply --version vMAJOR.MINOR.PATCH.'
    [[ ( -z "$bundle" && -z "$checksum" ) || ( -n "$bundle" && -n "$checksum" ) ]] || fail '--bundle and --checksum must be supplied together.'
    preflight
    [[ "$check" != true ]] || return
    WORK="$(mktemp -d /tmp/platform-install.XXXXXX)"
    trap cleanup EXIT
    if [[ -z "$bundle" ]]; then
        local filename="platform-v$REQUESTED_VERSION-linux-$ARCH.tar.gz"
        bundle="$WORK/$filename"; checksum="$WORK/SHA256SUMS"
        download "https://github.com/$REPOSITORY/releases/download/v$REQUESTED_VERSION/$filename" "$bundle"
        download "https://github.com/$REPOSITORY/releases/download/v$REQUESTED_VERSION/SHA256SUMS" "$checksum"
    fi
    verify_bundle "$bundle" "$checksum"
    load_metadata
    install -d -m 0700 "$STATE_ROOT"
    printf '%s\n' "$REPOSITORY" > "$STATE_ROOT/owner"
    install_helm
    install_k3s
    install_platform
}
if [[ "${BASH_SOURCE[0]}" == "$0" ]]; then main "$@"; fi
