# Linux installation — v0.0.1 preview

The initial release targets a dedicated, single-node Linux host: Raspberry Pi 5
with 8 GB RAM and 64-bit Raspberry Pi OS, or an AMD64 Linux server with systemd.
The platform and its managed applications run in the same K3s cluster. The
installer provisions K3s, imports native OCI images and installs the platform
chart. Docker, Go and Node.js are not required on the target host.

This release is a preview. Native Raspberry Pi installation and reboot acceptance
remain pending. Public HTTPS, application volumes, private-registry credentials,
verified failed-rollout restoration and cross-version upgrades are incomplete.
The [implementation status](status.md) records the remaining boundaries. Existing
unrelated K3s installations are refused; this installer is for a dedicated host.

## 1. Prepare the host

A 64-bit OS, systemd, enabled memory cgroups, at least 2 GiB RAM and 6 GiB free
space under `/var/lib` are required. The reference Pi has 32 GB microSD storage;
adequate free space must remain available for images, SQLite and K3s. An SSD is
preferable for sustained database use. Internet access is needed for GitHub
release assets, the pinned K3s distribution, Helm and K3s system images.

On Raspberry Pi OS:

```bash
uname -m                         # aarch64
cat /proc/1/comm                  # systemd
free -h
df -h /var/lib
```

If the installer reports missing memory cgroups, follow the
[K3s Raspberry Pi requirements](https://docs.k3s.io/installation/requirements?os=pi).
Add `cgroup_memory=1 cgroup_enable=memory` to the existing single line in
`/boot/firmware/cmdline.txt` (older OS: `/boot/cmdline.txt`), then reboot manually.
The installer does not edit boot files or reboot the host.

## 2. Download and inspect the installer

Run on the Pi/server as the normal SSH user:

```bash
mkdir -p ~/platform-v0.0.1
cd ~/platform-v0.0.1
curl --proto '=https' --tlsv1.2 -fLO https://github.com/piotrek1459/k8s-internal-platform/releases/download/v0.0.1/install.sh
curl --proto '=https' --tlsv1.2 -fLO https://github.com/piotrek1459/k8s-internal-platform/releases/download/v0.0.1/SHA256SUMS
awk '$2 == "install.sh"' SHA256SUMS | sha256sum -c -
less install.sh
sudo bash install.sh --version v0.0.1 --check
```

The checksum detects a damaged download. Both files are obtained from the same
GitHub release; this is not an independent signature or offline trust root.
`--check` performs read-only host checks without installation or downloads.

## 3. Install

```bash
sudo bash install.sh --version v0.0.1
```

The architecture is detected automatically. The matching bundle is downloaded and
verified before installation. It contains the platform OCI image, three demo OCI
images, the platform chart, application namespace configuration and compatibility
metadata. Images are imported into K3s containerd and referenced by digest; no
registry account is required. The broker uses `imagePullPolicy: Never` for its
bundled image. Traefik and ServiceLB are disabled; the management Service is
ClusterIP. K3s Secret encryption is enabled, and the kubeconfig has mode `0600`.

Important locations:

| Location | Purpose |
| --- | --- |
| `/etc/k8s-internal-platform/owner` | Installer ownership marker. |
| `/etc/k8s-internal-platform/release.env` | Installed version, pins and demo digest references; mode `0600`. |
| `/usr/local/lib/k8s-internal-platform/` | Pinned Helm and installer-owned K3s kubectl/ctr aliases. |
| `/etc/rancher/k3s/config.yaml` | K3s configuration. |
| `/etc/rancher/k3s/k3s.yaml` | Root-readable cluster credentials. |
| Platform PVC | SQLite metadata, accounts, sessions and operation history. |

Same-version reruns preserve the account and application history. An installation
with another recorded platform version is refused. Failed partial installations
retain their ownership marker for diagnosis and a same-version retry. No automatic
uninstall, host reset or application-data deletion is performed.

A bundle downloaded separately can be used with:

```bash
sudo bash install.sh --version v0.0.1 \
  --bundle platform-v0.0.1-linux-arm64.tar.gz --checksum SHA256SUMS
```

Use `linux-amd64` on AMD64. This option still downloads K3s and Helm as needed;
it is not an air-gapped installer.

## 4. Open the UI over SSH

Define the installer-owned Kubernetes command in each Pi/server shell:

```bash
pkubectl() {
  sudo /usr/local/lib/k8s-internal-platform/kubectl \
    --kubeconfig /etc/rancher/k3s/k3s.yaml "$@"
}
```

Keep this command running on the Pi/server:

```bash
pkubectl -n platform-system port-forward --address 127.0.0.1 service/platform 8080:80
```

In another terminal on the workstation, substitute the actual SSH destination:

```bash
ssh -N -L 8080:127.0.0.1:8080 USER@PI_HOST
```

Open **http://127.0.0.1:8080/setup** locally. Retrieve the bootstrap token privately
on the Pi/server:

```bash
pkubectl -n platform-system logs deployment/platform | grep -A 3 'Bootstrap token'
```

The bootstrap token is used once to create the administrator. Subsequent sign-in
uses the chosen password at `/login`. Do not publish the token, logs containing
it, passwords or the kubeconfig. If an unused token expires, restart the broker
Pod and read the current Pod's logs before retrying setup.

Health and system information can be inspected through the SSH tunnel:

```bash
curl -fsS http://127.0.0.1:8080/readyz
```

The `/api/v1/system` endpoint requires an authenticated session; authentication
and all mutation headers are documented in the [API guide](user/api-reference.md).

## 5. Deploy the bundled demo

Read the exact demo image reference on the server:

```bash
sudo awk -F= '$1 == "DEMO_V1_IMAGE" {print $2}' /etc/k8s-internal-platform/release.env
```

The bundled canonical name is local-only and has no registry endpoint. The
current UI wizard requires successful registry inspection, so the bundled demo
must be created through the [API walkthrough](user/api-reference.md), using the
`DEMO_V1_IMAGE` value as `image`. Set the HTTP profile, port `8080`, readiness path
`/readyz`, one replica and Internal exposure. A digest-pinned request bypasses
registry resolution; the imported image is already available on the node.

For UI deployment, supply an image available in a reachable public registry and
built for the host architecture. Inspect it, select its actual container port
and readiness path, then deploy with Internal exposure. Public image availability
and public application exposure are separate settings. Operation success and an
actual HTTP response must both be verified.

Find the application's Service and forward it on the server:

```bash
pkubectl -n platform-apps get services
pkubectl -n platform-apps port-forward --address 127.0.0.1 service/APP_SERVICE 8081:8080
```

Add a second workstation tunnel, then open the application:

```bash
ssh -N -L 8081:127.0.0.1:8081 USER@PI_HOST
curl -fsS http://127.0.0.1:8081/version
```

Use `DEMO_V2_IMAGE` for a healthy update. The v3 image deliberately fails readiness
and can leave a durable `needs_attention` lock; it is reserved for disposable
failure tests. The UI's status casing and application restart defects are
recorded limitations. Successful operation polling and an actual HTTP response
are stronger evidence than the current status badge.

Public applications require a reachable registry and an image built for the
host architecture. Public domain routing is a separate planned capability.
The [UI guide](user/README.md) and [API guide](user/api-reference.md) describe the
application lifecycle; their `platform.sh` commands apply to local k3d development
rather than native host installation.

## 6. Validate the reference Pi

Record the host model, OS, architecture, free space and installed pins. Confirm
setup, a v1 deployment, v2 update, scale 2/0/1, rollback, deletion and retained
history. Reboot the host and confirm that K3s, the broker, login and a retained
application recover. Native host results remain pending until this procedure is
executed; the disposable k3d acceptance suite does not establish Pi acceptance.

For diagnosis:

```bash
sudo systemctl status k3s --no-pager
pkubectl get nodes
pkubectl -n platform-system get pods,pvc
pkubectl -n platform-system logs deployment/platform
pkubectl -n platform-apps get deployments,pods,services
```

Logs can contain a setup token before administrator creation. Review and redact
locally before sharing. Recovery, backup and removal procedures must be reviewed
before retaining valuable application data on this preview.
