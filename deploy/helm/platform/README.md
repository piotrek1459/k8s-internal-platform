# Platform chart

Installs the broker (M2): the management API, durable operation worker, and status
observer, as a single in-cluster workload. Install it into `platform-system`; it
manages applications in `platform-apps` through scoped RBAC.

## What it creates

- A service account and RBAC: a namespaced Role in `appsNamespace` granting
  workload CRUD plus Secret access (Helm release storage and immutable
  application secrets), and a read-only ClusterRole for `nodes`,
  `persistentvolumes`, and `storageclasses` (capacity and volume reporting). No
  cluster-admin, no namespace or RBAC management, nothing in `kube-system`.
- A single-replica Deployment with a `Recreate` strategy, so only one process
  ever writes the SQLite metadata file. The container runs non-root with a
  read-only root filesystem, dropped capabilities, and the runtime-default seccomp
  profile; its service-account token is mounted so it can reach the Kubernetes API.
- A metadata `PersistentVolumeClaim` for the SQLite database.
- A private ClusterIP Service for the management API. There is no LAN or public
  route in M2; management access is an authenticated port-forward until M3.

## Values

| Value | Behaviour |
| --- | --- |
| `image.repository` | Broker image repository (required). |
| `image.tag` / `image.digest` | Supply one; releases should pin a digest. |
| `image.pullPolicy` | `IfNotPresent` by default; `Never` for locally imported images. |
| `appsNamespace` | Namespace the broker manages; must exist. Defaults to `platform-apps`. |
| `persistence.size` | Metadata volume size. |
| `persistence.storageClassName` | Empty uses the cluster default (local-path on K3s). |
| `resources` | Broker container requests/limits. |
| `service.port` | Private management Service port. |
| `secureCookies` | Set Secure on session cookies (enable behind HTTPS). |

## Installation

See the recorded procedure in
[tests/experiments/2026-10-26-m2-broker-install.md](../../../tests/experiments/2026-10-26-m2-broker-install.md).
The broker prints a single-use bootstrap token on first start; the setup endpoint
consumes it once to create the administrator account.
