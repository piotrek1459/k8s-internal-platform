#!/usr/bin/env bash
# Initial, single-node Linux installation. No application data is deleted.
set -euo pipefail
REPOSITORY=piotrek1459/k8s-internal-platform
STATE_ROOT=/etc/k8s-internal-platform
TOOL_ROOT=/usr/local/lib/k8s-internal-platform
KUBECONFIG_PATH=/etc/rancher/k3s/k3s.yaml
WORK=
LAN_CONFIG=

fail() { printf 'Error: %s\n' "$*" >&2; exit 1; }
usage() {
    cat <<'HELP'
Usage: sudo bash install.sh --version v0.1.2 [--check] [--prepare-host]
       sudo bash install.sh --version v0.1.2 --bundle FILE --checksum FILE
       sudo bash install.sh --version v0.1.2 --lan-config FILE

Installs the pinned K3s release and imports the platform/demo OCI images.
Requires 64-bit Linux, systemd, memory cgroups, 2 GiB RAM and 6 GiB free disk.
--check performs read-only host checks without downloading or installing.
--prepare-host may enable Raspberry Pi memory cgroups in the boot command line;
a manual reboot is required (exit status 20). No reboot is performed automatically.
An unrelated K3s installation is never replaced. Same-version reruns retain data;
upgrading a different installed platform version is not supported by this installer.
--lan-config enables private HTTPS on an existing same-version installation.
Requires Python 3, OpenSSL and an existing TLS Secret; see the LAN setup guide.
The validated configuration is retained on subsequent same-version reruns.
HELP
}

# Pure validation/conversion. JSON is parsed as data, never sourced or evaluated.
lan_values() {
    python3 - "$1" <<'PY'
import ipaddress,json,re,sys
try:
    with open(sys.argv[1]) as stream: config=json.load(stream)
    required={'address','httpsPort','allowedCIDRs','managementHostname','applicationDomain','tlsSecretName'}
    if not isinstance(config,dict) or set(config)!=required: raise ValueError('expected exactly the documented LAN fields')
    private=[ipaddress.ip_network(value) for value in ('10.0.0.0/8','172.16.0.0/12','192.168.0.0/16')]
    if not isinstance(config['address'],str): raise ValueError('listener address must be a string')
    address=ipaddress.ip_address(config['address'])
    if address.version!=4 or not any(address in network for network in private): raise ValueError('listener must bind a specific RFC1918 IPv4 address')
    if type(config['httpsPort']) is not int or not 1024<=config['httpsPort']<=65535: raise ValueError('HTTPS port must be between 1024 and 65535')
    ranges=config['allowedCIDRs']
    if not isinstance(ranges,list) or not ranges or len(ranges)!=len(set(ranges)): raise ValueError('non-empty unique source CIDRs are required')
    for value in ranges:
        if not isinstance(value,str): raise ValueError('source CIDRs must be strings')
        network=ipaddress.ip_network(value,strict=True)
        if network.version!=4 or not any(network.subnet_of(parent) for parent in private): raise ValueError('source CIDRs must be RFC1918 IPv4 subnets')
    label=r'[a-z0-9](?:[-a-z0-9]{0,61}[a-z0-9])?'
    for field in ('managementHostname','applicationDomain'):
        name=config[field]
        if not isinstance(name,str) or len(name)>253 or not re.fullmatch(label+r'(?:\.'+label+r')+',name): raise ValueError('valid lowercase DNS names are required')
    host,domain=config['managementHostname'],config['applicationDomain']
    if host==domain or host.endswith('.'+domain): raise ValueError('management hostname must be outside the application domain')
    if not isinstance(config['tlsSecretName'],str) or not re.fullmatch(label,config['tlsSecretName']): raise ValueError('TLS Secret name must be a DNS label')
    print(json.dumps({'privateLAN':{'enabled':True,**config}},sort_keys=True))
except (ValueError,TypeError,KeyError,OSError) as error:
    sys.exit('Invalid LAN configuration: '+str(error))
PY
}

check_lan_address() {
    python3 - "$LAN_CONFIG" <<'PY'
import json,subprocess,sys
with open(sys.argv[1]) as stream: address=json.load(stream)['address']
try: interfaces=json.loads(subprocess.check_output(['ip','-j','-4','address','show']))
except (OSError,subprocess.CalledProcessError,ValueError): sys.exit('LAN setup requires iproute2 IPv4 address inspection')
if not any(item.get('local')==address for interface in interfaces for item in interface.get('addr_info',[])):
    sys.exit('Configured private listener address is not assigned to this host')
PY
}

check_lan_chart() {
    "$TOOL_ROOT/helm" show values "$WORK/bundle/platform-chart.tgz" | grep '^privateLAN:' >/dev/null ||
        fail 'This release bundle does not support private LAN HTTPS; use a matching installer and LAN-capable release.'
}

check_lan_certificate() {
    local secret hostname domain
    read -r secret hostname domain < <(python3 - "$WORK/lan-values.json" <<'PY'
import json,sys
with open(sys.argv[1]) as stream: config=json.load(stream)['privateLAN']
print(config['tlsSecretName'],config['managementHostname'],config['applicationDomain'])
PY
)
    "$TOOL_ROOT/kubectl" --kubeconfig "$KUBECONFIG_PATH" -n platform-system get secret "$secret" -o json > "$WORK/tls-secret.json" || fail 'Create the documented TLS Secret before enabling LAN HTTPS.'
    python3 - "$WORK" <<'PY'
import base64,json,pathlib,sys
folder=pathlib.Path(sys.argv[1])
with (folder/'tls-secret.json').open() as stream: secret=json.load(stream)
if secret.get('type')!='kubernetes.io/tls': sys.exit('LAN certificate requires a kubernetes.io/tls Secret')
for key in ('tls.crt','tls.key'):
    try: value=base64.b64decode(secret['data'][key],validate=True)
    except (KeyError,ValueError): sys.exit('TLS Secret is missing valid certificate/key data')
    path=folder/key;path.write_bytes(value);path.chmod(0o600)
PY
    openssl x509 -in "$WORK/tls.crt" -noout -checkend 86400 >/dev/null || fail 'LAN certificate must remain valid for at least 24 hours.'
    openssl x509 -in "$WORK/tls.crt" -noout -startdate > "$WORK/tls-start"
    python3 - "$WORK/tls-start" <<'PY'
import pathlib,ssl,sys,time
start=pathlib.Path(sys.argv[1]).read_text().split('=',1)[1].strip()
if ssl.cert_time_to_seconds(start)>time.time(): sys.exit('LAN certificate is not valid yet')
PY
    openssl x509 -in "$WORK/tls.crt" -noout -checkhost "$hostname" >/dev/null || fail 'LAN certificate does not cover the management hostname.'
    # Require the wildcard itself, rather than accepting only one test hostname.
    openssl x509 -in "$WORK/tls.crt" -noout -ext subjectAltName > "$WORK/tls-san"
    python3 - "$WORK/tls-san" "$domain" <<'PY'
import pathlib,re,sys
names=re.findall(r'DNS:([^,\s]+)',pathlib.Path(sys.argv[1]).read_text())
if '*.'+sys.argv[2] not in names: sys.exit('LAN certificate must cover the exact wildcard application domain')
PY
    openssl x509 -in "$WORK/tls.crt" -noout -pubkey | openssl pkey -pubin -outform DER > "$WORK/cert-public.der"
    openssl pkey -in "$WORK/tls.key" -passin pass: -pubout -outform DER > "$WORK/key-public.der" || fail 'LAN key must be readable without a passphrase.'
    cmp -s "$WORK/cert-public.der" "$WORK/key-public.der" || fail 'LAN certificate and key do not match.'
}
cleanup() {
    if [[ -n "$WORK" && "$WORK" == /tmp/platform-install.* && -d "$WORK" ]]; then
        find "$WORK" -depth -delete
    fi
}
architecture() {
    case "$1" in aarch64|arm64) printf 'arm64\n' ;; x86_64|amd64) printf 'amd64\n' ;; *) fail 'Only 64-bit ARM64 and AMD64 are supported; 32-bit Raspberry Pi OS is unsupported.' ;; esac
}
# Pure transformation, also exercised by regression tests. Keep the required
# kernel arguments on the existing single command line.
memory_cgroup_cmdline() {
    awk '
        NF { lines++; for (i=1; i<=NF; i++) {
            if ($i != "cgroup_disable=memory" && $i != "cgroup_memory=0" && $i != "cgroup_memory=1" && $i != "cgroup_enable=memory") {
                printf "%s%s", separator, $i; separator=" "
            }
        }}
        END { if (lines != 1) exit 1; printf "%scgroup_memory=1 cgroup_enable=memory\n", separator }
    ' "$1"
}
prepare_memory_cgroup() {
    [[ "$PREPARE_HOST" == true ]] || fail 'Memory cgroups are unavailable. On Raspberry Pi OS, rerun with --prepare-host to back up and update the boot command line, then reboot manually. See INSTALL.md.'
    [[ -r /proc/device-tree/model ]] && grep -q 'Raspberry Pi' /proc/device-tree/model || fail '--prepare-host only supports Raspberry Pi boot configuration.'
    local path=/boot/firmware/cmdline.txt updated backup
    [[ -f "$path" ]] || path=/boot/cmdline.txt
    [[ -f "$path" && ! -L "$path" ]] || fail 'A regular Raspberry Pi boot command-line file was not found.'
    updated="$(memory_cgroup_cmdline "$path")" || fail 'The boot command line must contain exactly one non-empty line; no files were changed.'
    backup="$path.platform-backup-$(date -u +%Y%m%dT%H%M%S)-$$"
    cp -p "$path" "$backup"
    printf '%s\n' "$updated" > "$path"
    printf 'Memory cgroup boot settings saved. Backup: %s\nReboot the host, then rerun the installer without --prepare-host.\n' "$backup"
    exit 20
}
preflight() {
    [[ "$(uname -s)" == Linux ]] || fail 'The host installer requires Linux.'
    [[ "$(id -u)" == 0 ]] || fail 'Run the installer with sudo.'
    for command in curl tar sha256sum awk grep systemctl install df sort uniq ln; do
        command -v "$command" >/dev/null || fail "Missing required host tool: $command"
    done
    [[ "$(cat /proc/1/comm)" == systemd ]] || fail 'A systemd-based host is required.'
    ARCH="$(architecture "$(uname -m)")"
    local memory disk
    memory="$(awk '/^MemTotal:/ {print $2}' /proc/meminfo)"
    disk="$(df -Pk /var/lib | awk 'END {print $4}')"
    (( memory >= 2097152 )) || fail 'Installation requires at least 2 GiB RAM.'
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
    if [[ -f /sys/fs/cgroup/cgroup.controllers ]]; then
        grep -qw memory /sys/fs/cgroup/cgroup.controllers || prepare_memory_cgroup
    else
        awk '$1=="memory" && $4==1 {found=1} END {exit !found}' /proc/cgroups || prepare_memory_cgroup
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
    # Explicit multicall names avoid dependence on global tool aliases.
    ln -sfn /usr/local/bin/k3s "$TOOL_ROOT/kubectl"
    ln -sfn /usr/local/bin/k3s "$TOOL_ROOT/ctr"
    local attempt
    for ((attempt=0; attempt<60; attempt++)); do
        if "$TOOL_ROOT/kubectl" --kubeconfig "$KUBECONFIG_PATH" --request-timeout=5s get nodes -o name 2>/dev/null | grep -q '^node/'; then
            "$TOOL_ROOT/kubectl" --kubeconfig "$KUBECONFIG_PATH" wait --for=condition=Ready nodes --all --timeout=180s
            return
        fi
        sleep 2
    done
    fail 'K3s did not register a node; inspect sudo journalctl -u k3s.'
}
install_platform() {
    local key image archive
    local lan_options=()
    if [[ -n "$LAN_CONFIG" ]]; then
        check_lan_chart
        lan_values "$LAN_CONFIG" > "$WORK/lan-values.json"
        check_lan_certificate
        lan_options=(--values "$WORK/lan-values.json" --atomic --cleanup-on-fail)
    fi
    for key in PLATFORM_IMAGE DEMO_V1_IMAGE DEMO_V2_IMAGE DEMO_V3_IMAGE; do
        image="${!key}"
        case "$key" in PLATFORM_IMAGE) archive=platform-image.oci.tar ;; DEMO_V1_IMAGE) archive=demo-v1.oci.tar ;; DEMO_V2_IMAGE) archive=demo-v2.oci.tar ;; DEMO_V3_IMAGE) archive=demo-v3.oci.tar ;; esac
        "$TOOL_ROOT/ctr" --namespace k8s.io images import --local --digests --base-name "${image%@*}" "$WORK/bundle/$archive"
    done
    "$TOOL_ROOT/kubectl" --kubeconfig "$KUBECONFIG_PATH" apply -f "$WORK/bundle/namespace.yaml"
    "$TOOL_ROOT/helm" --kubeconfig "$KUBECONFIG_PATH" upgrade --install platform "$WORK/bundle/platform-chart.tgz" \
        --namespace platform-system --create-namespace \
        --set-string "image.repository=${PLATFORM_IMAGE%@*}" --set-string "image.digest=${PLATFORM_IMAGE#*@}" \
        --set image.pullPolicy=Never "${lan_options[@]}" --wait --timeout 180s
    local attempt response
    for ((attempt=0; attempt<30; attempt++)); do
        response="$("$TOOL_ROOT/kubectl" --kubeconfig "$KUBECONFIG_PATH" --request-timeout=10s get --raw /api/v1/namespaces/platform-system/services/platform:80/proxy/readyz 2>/dev/null)" || response='{}'
        if grep -Eq '"status":[[:space:]]*"ready"' <<< "$response" && grep -Eq '"cluster":[[:space:]]*"available"' <<< "$response"; then
            install -m 0600 "$WORK/bundle/release.env" "$STATE_ROOT/release.env"
            if [[ -n "$LAN_CONFIG" ]]; then
                python3 - "$WORK/lan-values.json" "$WORK/lan-config.json" <<'PY'
import json,sys
with open(sys.argv[1]) as stream: config=json.load(stream)['privateLAN']
del config['enabled']
with open(sys.argv[2],'w') as stream: json.dump(config,stream,sort_keys=True)
PY
                install -m 0600 "$WORK/lan-config.json" "$STATE_ROOT/private-lan.json"
                printf 'Private HTTPS configured. Verify local DNS and certificate trust using the LAN setup guide.\n'
            fi
            printf 'Installed platform v%s. Metadata is retained across same-version reruns.\n' "$PLATFORM_VERSION"
            printf 'On the Pi: sudo %s/kubectl --kubeconfig %s -n platform-system port-forward --address 127.0.0.1 service/platform 8080:80\n' "$TOOL_ROOT" "$KUBECONFIG_PATH"
            if [[ -z "$LAN_CONFIG" ]]; then
                printf 'Use an SSH local forward and open http://127.0.0.1:8080/setup. Retrieve the token from current broker logs.\n'
            else
                printf 'Secure cookies are enabled. Use HTTPS, including the documented TLS/SSH fallback; plain-HTTP session forwarding is not the LAN fallback.\n'
            fi
            return
        fi
        sleep 2
    done
    fail 'Platform readiness was not established. Inspect broker logs with the installer-owned kubectl documented in INSTALL.md.'
}
main() {
    REQUESTED_VERSION=; PREPARE_HOST=false; local bundle= checksum= check=false
    while [[ $# -gt 0 ]]; do
        case "$1" in
            -h|--help) usage; return ;;
            --check) check=true; shift ;;
            --prepare-host) PREPARE_HOST=true; shift ;;
            --version|--bundle|--checksum|--lan-config)
                [[ $# -ge 2 ]] || fail "Missing value for $1"
                case "$1" in --version) REQUESTED_VERSION="${2#v}" ;; --bundle) bundle="$2" ;; --checksum) checksum="$2" ;; --lan-config) LAN_CONFIG="$2" ;; esac
                shift 2 ;;
            *) fail "Unknown argument: $1" ;;
        esac
    done
    [[ "$REQUESTED_VERSION" =~ ^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$ ]] || fail 'Supply --version vMAJOR.MINOR.PATCH.'
    [[ ( -z "$bundle" && -z "$checksum" ) || ( -n "$bundle" && -n "$checksum" ) ]] || fail '--bundle and --checksum must be supplied together.'
    [[ "$check" != true || "$PREPARE_HOST" != true ]] || fail '--check is read-only and cannot be combined with --prepare-host.'
    if [[ -z "$LAN_CONFIG" && -f "$STATE_ROOT/private-lan.json" ]]; then LAN_CONFIG="$STATE_ROOT/private-lan.json"; fi
    if [[ -n "$LAN_CONFIG" ]]; then
        command -v python3 >/dev/null && command -v openssl >/dev/null || fail 'Private LAN setup requires Python 3 and OpenSSL.'
        lan_values "$LAN_CONFIG" >/dev/null
        check_lan_address
        if [[ -f "$STATE_ROOT/private-lan.json" ]]; then
            [[ "$(lan_values "$LAN_CONFIG")" == "$(lan_values "$STATE_ROOT/private-lan.json")" ]] || fail 'Changing installed LAN settings is not supported; existing configuration was retained.'
        fi
    fi
    preflight
    [[ "$check" != true ]] || return
    [[ -z "$LAN_CONFIG" || -x "$TOOL_ROOT/kubectl" ]] || fail 'Install without LAN first, then create the TLS Secret and rerun with --lan-config.'
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
