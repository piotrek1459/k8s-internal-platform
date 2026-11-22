# Implementation status and next work

Updated: 2026-11-22. This document distinguishes implemented behavior, executed
verification and remaining scope. Plans and passing unit tests are not evidence
of acceptance on every deployment target.

## Current implementation

The platform and its applications run in one single-node K3s cluster. A Go
process serves the static Svelte UI and JSON API, stores metadata in SQLite,
executes a durable serial operation queue and manages applications through its
embedded first-party Helm chart.

| Area | Implemented behavior | Acceptance boundary |
| --- | --- | --- |
| Distribution | Manually dispatched releases pin a selected branch HEAD, run regression/E2E gates and publish ARM64/AMD64 OCI bundles, installer, guide and checksums. Normal publication is the default. | Native Raspberry Pi results are recorded separately; native AMD64 host acceptance and cross-version upgrades remain outstanding. |
| Installation | Dedicated Linux/systemd installer provisions pinned K3s/Helm, imports native OCI digests and installs the platform. Same-version reruns retain state. Optional Pi boot preparation backs up the command line and requires a manual reboot. | Unrelated clusters and cross-version upgrades are refused. The boot-preparation transformation has regressions; physical-host evidence must identify the tested installer release. |
| Authentication | Single-administrator setup, expiring bootstrap token, Argon2id password hashing, hashed sessions, logout and mutation-header checks. Root navigation selects setup on a fresh installation. | Login throttling, password recovery and normal LAN HTTPS administration remain outstanding. |
| UI/API | CamelCase workload status, nested error messages, object-shaped Secret versions, Secret-reference submission, lifecycle/history screens and recovery retry. | Application log browsing and the complete configuration editor remain planned. LAN selection is enabled only when private ingress is explicitly configured. |
| Application lifecycle | Digest-pinned deployment, update, scale 2/0/1, restart, reapply, rollback and deletion with retained history. | Replicas on one node do not provide host availability. Application data volumes remain M5 scope. |
| Execution snapshots | Image digest, exact Secret references, chart version, embedded chart content digest, canonical values hash and stable restart token are recorded before apply. | A different chart artifact is refused; old chart retrieval and rollback across chart changes are not implemented. |
| Recovery | Interrupted success requires target release identity, matching chart/values and healthy matching live resources. Failed updates attempt verified restoration of the last successful revision. | Pending Helm state, an unavailable cluster, missing snapshots or failed restoration remain blocked. Recovery retry reuses the existing operation; it does not blindly clear a lock. |
| Configuration Secrets | Immutable Kubernetes Secret versions, metadata-only SQLite storage, explicit version references and deletion guards. | Ordinary API responses never return values. |
| Registry credentials | Immutable Kubernetes Docker configuration Secrets support inspection and kubelet pulls. SQLite contains metadata only. Credentials are registry-scoped and deletion is guarded by active/rollback references. | Password creation does not itself verify registry access. Credential rotation and external registry-provider variations require separate acceptance. |
| Diagnostics | Structured broker logs, operation events and observed workload state. | Application log/event APIs, audit browsing, capacity reporting and metrics remain planned. |
| Networking | ClusterIP management and applications with SSH loopback forwarding. Private ingress is an explicit optional capability. | The native installer disables Traefik/ServiceLB. LAN DNS/HTTPS and public routing/tunnels remain separate work. |
| Persistence | A platform PVC retains SQLite, accounts, sessions, revisions and history. | Application volumes, off-device backups and verified data restoration remain M5 scope. |

## Known correctness and integration gaps

The previous status/restart acceptance skips have been removed. Recovery no
longer accepts an arbitrary deployed Helm release as proof that a target was
applied. Ambiguous state deliberately retains `needs_attention` and the mutation
lock; a new rollback cannot bypass that lock. After addressing the underlying
cause, `POST /apps/{id}/recover` or **Retry recovery** requeues the existing
blocked operation under the application-version guard.

Remaining boundaries are explicit: cross-version platform upgrades, unavailable
historical chart artifacts, first-install failure without a previous successful
revision, pending Helm transactions and failed restoration require diagnosis.
A retry does not promise that these conditions can be resolved automatically.
Public HTTPS, application persistence, backup/recovery, private registry rotation,
login throttling and application log browsing are not established by this increment.

## Verification available

- Pull requests run uncached Go regressions, UI type checking/lint/build, OpenAPI
  validation, Python/shell checks and the chart lint/render/negative/synchronization
  matrix through `tests/run.py`. The fast jobs do not create a cluster.
- The real-cluster API/browser/bundle suite runs nightly at **02:00 UTC**, manually,
  and as a release gate. Status serialization and Pod replacement are required
  assertions. Unhealthy updates must restore the previous snapshot before unlock.
  Authenticated registry inspection and actual kubelet image pulls are covered.
- `tests/e2e/native.py` exercises a published ARM64 release on an explicitly
  selected installation through loopback SSH forwards. It creates an administrator
  only with `--setup`, retains private local credentials and one owned test app,
  and supports post-reboot verification with `--resume`.
- The [reference Pi report](../tests/experiments/2026-11-22-raspberry-pi.md)
  records installation attempts, host prerequisites and executed release acceptance.
  The [manual-release report](../tests/experiments/2026-11-08-manual-release.md)
  and earlier [local lifecycle report](../tests/experiments/2026-11-08-local-workflow.md)
  remain historical evidence for their recorded source versions.

Exact suite commands and locations are in the [test guide](../tests/README.md).
Evidence from containers does not establish systemd, native Pi boot or storage
reliability. Measurements must record release/source, host, pins and conditions.

## Next implementation order

1. Complete and retain release-based native Pi installation, same-version rerun
   and reboot evidence; resolve failures before extending milestone scope.
2. Proceed to M5 application storage and verified backup/restore after the current
   defect and native-acceptance work is complete.
3. Address public HTTPS, supported platform upgrades and extended observability
   under their separate acceptance criteria.

The [milestone checklists](milestones.md) retain the complete project plan.
