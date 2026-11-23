# Implementation status and next work

Updated: 2026-11-23. This document distinguishes implemented behavior, executed
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
| Installation | Published `v0.1.5` passed fresh Raspberry Pi installation, lifecycle, same-version rerun and reboot checks. Dedicated Linux/systemd installer provisions pinned K3s/Helm, imports native OCI digests and installs the platform. Same-version reruns retain state. Optional Pi boot preparation backs up the command line and requires a manual reboot. | Its native diagnostics cleanup exposed Secret-deletion defects described below; the complete native command failed. Unrelated clusters and cross-version upgrades are refused. |
| Authentication | Single-administrator setup, expiring bootstrap token, Argon2id password hashing, hashed sessions, logout and mutation-header checks. Root navigation selects setup on a fresh installation. | Login throttling, password recovery and normal LAN HTTPS administration remain outstanding. |
| UI/API | CamelCase workload status, nested error messages, object-shaped Secret versions, Secret-reference submission, lifecycle/history screens and recovery retry. | Source includes bounded application diagnostics and drift inspection; the complete configuration editor remains planned. LAN selection is enabled only when private ingress is explicitly configured. |
| Application lifecycle | Digest-pinned deployment, update, scale 2/0/1, restart, reapply, rollback and deletion with retained history. | Replicas on one node do not provide host availability. Application data volumes remain M5 scope. |
| Temporary files | Application chart `0.1.1` supplies a Pod-local `/tmp` emptyDir with a 64Mi size limit and non-root filesystem-group access. | Published `v0.1.3` passed unpatched browser deployment of unprivileged Nginx and real HTTP verification on the Pi. The volume is ephemeral; it does not provide M5 persistence. |
| Execution snapshots | Image digest, exact Secret references, chart version, embedded chart content digest, canonical values hash and stable restart token are recorded before apply. | A different chart artifact is refused; old chart retrieval and rollback across chart changes are not implemented. See the M7 boundary and core acceptance tracker. |
| Recovery | Interrupted success requires target release identity, matching chart/values and healthy matching live resources. Failed updates attempt verified restoration of the last successful revision. | Ambiguous/pending state remains blocked. A blocked first create without a successful predecessor supports tracked removal with retained evidence; other recovery retry reuses the existing operation. |
| Configuration Secrets | Immutable Kubernetes versions, metadata-only SQLite, exact version references and protected dependencies. Candidate source reserves deletion durably, removes recorded versions under UID/ownership guards and supports partial-cleanup retry. | Published v0.1.5 leaves values after metadata-only deletion and incorrectly protects deleted-application history. The correction requires merge, release and native confirmation. Ordinary responses never return values. |
| Registry credentials | Immutable Kubernetes Docker configuration Secrets support inspection and kubelet pulls. SQLite contains metadata only. Credentials are registry-scoped and deletion is guarded by active/rollback references. | Password creation does not itself verify registry access. Credential rotation and external registry-provider variations require separate acceptance. |
| Diagnostics | Published v0.1.5 verified bounded owned-workload logs/events, managed-Secret masking, escaped browser text and field-path drift on the Pi. Explicit Reapply restored Service ports, HTTP and intent while retaining UID/address. | Diagnostic fixture Secret cleanup failed separately. Current-resource reads are not a durable archive; audit browsing, capacity, metrics and the complete disclosure/failure matrix remain open. |
| Networking | ClusterIP management and applications with SSH loopback forwarding. Private ingress is an explicit optional capability. | The native installer disables Traefik/ServiceLB. LAN DNS/HTTPS and public routing/tunnels remain separate work. |
| Persistence | A platform PVC retains SQLite, accounts, sessions, revisions and history. | Application volumes, off-device backups and verified data restoration remain M5 scope. |

## Known correctness and integration gaps

The previous status/restart acceptance skips have been removed. E2E acceptance
requires a non-empty suite with zero failures, zero skips and no expected-failure
waivers; its summary records the tested source commit. Recovery no
longer accepts an arbitrary deployed Helm release as proof that a target was
applied. Ambiguous state deliberately retains `needs_attention` and the mutation
lock; a new rollback cannot bypass that lock. After addressing the underlying
cause, `POST /apps/{id}/recover` or **Retry recovery** requeues the existing
blocked operation under the application-version guard.

Published `v0.1.3` resolves the Nginx temporary-files gap observed in `v0.1.2`.
The unmodified release passed the native browser workflow, same-version rerun and
host reboot. The [new report](../tests/experiments/2026-11-23-core-acceptance.md)
records those results and the direct Helm baseline on the reference Pi.

Explicit deletion provides a resolution for a blocked first create with no
successful predecessor. It preserves the failed candidate and atomically
transfers its lock to tracked cleanup. Running work and unresolved updates remain
protected. Cross-version platform upgrades, unavailable historical chart artifacts,
pending Helm transactions and failed restoration retain their stated boundaries.
Diagnostics and owned-field drift reporting are released and verified on the Pi
in `v0.1.5`. The native run failed
at subsequent Secret cleanup: deleted-application history returned 409, and a
separate unused Secret probe returned 204 while leaving its Kubernetes value.
The [v0.1.5 report](../tests/experiments/2026-11-23-v0.1.5-secret-lifecycle.md)
records both failures, guarded fixture cleanup and unchanged original identities
through same-version reinstall/reboot. Candidate source corrects deletion and
adds browser creation/version/credential acceptance plus partial-cleanup retry
across broker replacement; native verification of that correction remains open.
Core work still missing includes normal private LAN HTTPS and parts of the real-cluster
crash/browser/security/evaluation matrix. These remain M0–M4 obligations in
[issue #16](https://github.com/piotrek1459/k8s-internal-platform/issues/16), alongside
the [audited milestone checklists](milestones.md). Public HTTPS, application
persistence, verified backup/restore, private registry rotation and login
throttling retain their separately recorded boundaries.

## Verification available

- Pull requests run uncached Go regressions, UI type checking/lint/build, OpenAPI
  validation, Python/shell checks and the chart lint/render/negative/synchronization
  matrix through `tests/run.py`. The fast jobs do not create a cluster.
- The real-cluster API/browser/bundle suite runs nightly at **02:00 UTC**, manually,
  and as a release gate. Status serialization and Pod replacement are required
  assertions. Unhealthy updates must restore the previous snapshot before unlock.
  Authenticated registry inspection and actual kubelet image pulls are covered.
  The [latest 16-case source run](../tests/experiments/2026-11-23-v0.1.4-and-diagnostics.md#executed-source-verification)
  includes bounded logs/events, hostile-text escaping and owned drift repair.
  The [earlier 15-case source run](../tests/experiments/2026-11-23-core-acceptance.md#migration-and-blocked-deployment-follow-up)
  also verifies runtime SIGKILL during a pending first install, durable blocking,
  independent queued browser work and supported blocked-first-deployment removal.
- `tests/e2e/native.py` exercises a published ARM64 release on an explicitly
  selected installation through loopback SSH forwards. It creates an administrator
  only with `--setup`, retains private local credentials and one owned test app,
  supports post-reboot verification with `--resume`, and optionally checks the
  unprivileged Nginx browser workflow with `--public-image` and owned diagnostics/
  drift/physical Secret cleanup with `--diagnostics`.
- The [v0.1.5 report](../tests/experiments/2026-11-23-v0.1.5-secret-lifecycle.md)
  distinguishes published Pi lifecycle/diagnostics/reinstall/reboot success from
  failed Secret cleanup and the candidate-source correction. Focused source
  browser/cleanup acceptance passed 5 tests with zero failures/skips. The full
  [clean-source GitHub run](https://github.com/piotrek1459/k8s-internal-platform/actions/runs/45194804590)
  passed **17 tests, 0 failures, 0 skips** at `8804ced`, with downloaded JUnit and
  the strict summary verified. The cleanup correction still requires publication
  and native confirmation.
- The [v0.1.4 and diagnostics report](../tests/experiments/2026-11-23-v0.1.4-and-diagnostics.md)
  records the published Pi lifecycle/removal/reinstall/reboot and separate source work.
- The [v0.1.3 reference acceptance](../tests/experiments/2026-11-23-core-acceptance.md)
  records fresh installation, unpatched Nginx, reinstall/reboot and a manual Helm baseline.
- The [earlier reference Pi report](../tests/experiments/2026-11-22-raspberry-pi.md)
  records installation attempts, host prerequisites and executed release acceptance.
  The [manual-release report](../tests/experiments/2026-11-08-manual-release.md)
  and earlier [local lifecycle report](../tests/experiments/2026-11-08-local-workflow.md)
  remain historical evidence for their recorded source versions.

Exact suite commands and locations are in the [test guide](../tests/README.md).
Evidence from containers does not establish systemd, native Pi boot or storage
reliability. Measurements must record release/source, host, pins and conditions.

## Next implementation order

1. Complete the remaining core work in
   [issue #16](https://github.com/piotrek1459/k8s-internal-platform/issues/16):
   private LAN administration, the remaining failure/browser/security
   matrix and the controlled evaluation. Release and natively verify the Secret
   cleanup correction before declaring that acceptance complete. The v0.1.5 Pi
   lifecycle/diagnostics/reinstall/reboot checks and direct functional Helm
   baseline have passed; the complete v0.1.5 native run failed at Secret cleanup.
2. Keep measured acceptance separate from implementation: a single cached-image
   baseline does not calibrate resource limits or satisfy the repeated M4 evaluation.
3. Continue M5–M8 under their separate acceptance criteria after the applicable
   core requirements are resolved.

The [milestone checklists](milestones.md) retain the complete project plan.
