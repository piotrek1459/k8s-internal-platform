# Implementation status and next work

Updated: 2026-11-24. This document distinguishes implemented behavior, executed
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
| Installation | Published v0.1.7 passed fresh Raspberry Pi installation, lifecycle, same-version rerun and reboot. The installer provisions pinned K3s/Helm and native OCI digests; same-version reruns retain state. | Verified owned v0.1.5 test state was privately archived/reset before fresh v0.1.7. This was not a cross-version upgrade or backup restoration. Unrelated installations remain protected. |
| Authentication | Single-administrator setup, expiring bootstrap token, Argon2id password hashing, hashed sessions, logout and mutation-header checks. Root navigation selects setup on a fresh installation. | Login throttling/password recovery remain outstanding; the new LAN mode awaits published Pi acceptance. |
| UI/API | CamelCase workload status, nested error messages, object-shaped Secret versions, Secret-reference submission, lifecycle/history screens and recovery retry. | Released diagnostics include bounded application logs/events and drift inspection; the complete configuration editor remains planned. LAN selection is enabled only when private ingress is explicitly configured. |
| Application lifecycle | Digest-pinned deployment, update, scale 2/0/1, restart, reapply, rollback and deletion with retained history. | Replicas on one node do not provide host availability. Application data volumes remain M5 scope. |
| Temporary files | A Pod-local `/tmp` emptyDir, introduced in chart `0.1.1` and retained in `0.1.2`, provides temporary storage with a 64Mi size limit and non-root filesystem-group access. | Published `v0.1.3` passed unpatched browser deployment of unprivileged Nginx and real HTTP verification on the Pi. The volume is ephemeral; it does not provide M5 persistence. |
| Execution snapshots | Image digest, exact Secret references, chart version, embedded chart content digest, canonical values hash and stable restart token are recorded before apply. | A different chart artifact is refused; old chart retrieval and rollback across chart changes are not implemented. See the M7 boundary and core acceptance tracker. |
| Recovery | Interrupted success requires target release identity, matching chart/values and healthy matching live resources. Failed updates attempt verified restoration of the last successful revision. | Ambiguous/pending state remains blocked. A blocked first create without a successful predecessor supports tracked removal with retained evidence; other recovery retry reuses the existing operation. |
| Configuration Secrets | From v0.1.6: immutable versions, exact references, durable deletion reservations, UID/ownership-guarded physical removal and retry. Published v0.1.7 verified browser creation/versioning, reference refusal, deleted-history cleanup and partial retry across actual broker replacement on the Pi. | Complete negative-path disclosure and historical orphan collection remain separate. Original v0.1.7 session/PVC/history survived the native retry and reinstall/reboot checks. |
| Registry credentials | Immutable Kubernetes Docker configuration Secrets support inspection and kubelet pulls. SQLite contains metadata only. Credentials are registry-scoped and deletion is guarded by active/rollback references. | Password creation does not itself verify registry access. Credential rotation and external registry-provider variations require separate acceptance. |
| Diagnostics | Published v0.1.7 verified bounded owned logs/events, managed-Secret masking, escaped text and field-only drift on the Pi. Explicit Reapply restored Service ports/HTTP/intent while retaining UID/address; fixture Secret cleanup passed. | Current-resource reads are not a durable archive. Audit browsing, capacity, metrics and the full disclosure/failure matrix remain open. |
| Networking | Default SSH/ClusterIP access; source optional private LAN HTTPS binds a specific IPv4 address, enforces source CIDRs, uses an existing TLS Secret and exposes configured app URLs with Secure cookies. | The next published release and real second-device Pi/DNS/trust checks remain required. Built-in Traefik/ServiceLB stay disabled; the opt-in private gateway is separate. Public routing remains M6 scope. |
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
Published v0.1.7 passed native diagnostics/drift and the Secret cleanup correction
introduced in v0.1.6. Browser/API reference refusal, actual value/metadata removal
after dependent-app deletion, partial refusal/reservation across broker replacement
and guarded browser retry all passed. The original v0.1.7 administrator/session/
app/revision/history/PVC identity and HTTP survived reinstall/reboot. The
[native report](../tests/experiments/2026-11-23-v0.1.7-native.md) records source/image
identity and evidence. The preceding v0.1.5 failure remains in its
[dated report](../tests/experiments/2026-11-23-v0.1.5-secret-lifecycle.md);
v0.1.6 itself was not installed natively.
Core acceptance still missing includes native private LAN HTTPS and parts of the real-cluster
crash/browser/security/evaluation matrix. These remain M0–M4 obligations in
[issue #16](https://github.com/piotrek1459/k8s-internal-platform/issues/16), alongside
the [audited milestone checklists](milestones.md). Public HTTPS, application
persistence, verified backup/restore, private registry rotation and login
throttling retain their separately recorded boundaries.

## Verification available

- The [release compatibility follow-up](../tests/experiments/2026-11-24-private-lan.md#release-verification-compatibility-correction)
  records the failed v0.1.8 verification at `40f467a`: OpenSSL 3.0.13 returned
  success for a wrong hostname while the earlier PR job used Ubuntu 26.04.
  Corrected certificate verification passed **81 Python regressions and syntax**
  on OpenSSL 3.0.13/Ubuntu 24.04 and OpenSSL 3.6.2. PR Python checks now match the
  release runner. No release was published; a new manual dispatch must select the
  fixed branch HEAD before native LAN acceptance.
- The [private LAN source report](../tests/experiments/2026-11-24-private-lan.md)
  records a focused 2/0/0 workflow and full clean-source **21/0/0** acceptance at
  `ddec3aa`, with downloaded JUnit/strict summary and artifact digest verified.
  Actual TLS/browser/update/rollback, cookie/CORS, source/route/Host/SNI and IPv6
  checks passed. The read-only native bundle-capability guard added afterward is
  covered by the 80-case Python suite; runtime/chart/E2E code is unchanged.
  Native LAN criteria await merge, manual release and second-device Pi acceptance.
- The [published v0.1.7 native report](../tests/experiments/2026-11-23-v0.1.7-native.md)
  verifies fresh installation, lifecycle, diagnostics/drift, physical Secret cleanup,
  browser retry across replacement, same-version reinstall and reboot with original
  identities/HTTP preserved. Temporary administrative access was closed; one healthy
  owned demo remains. The test-state archive passed gzip integrity, not restoration.
- The [core resilience report](../tests/experiments/2026-11-23-core-resilience.md)
  records disposable-cluster acceptance while reference-host SSH was unavailable:
  actual workload-origin protected requests, lost committed response
  with real tag movement, and browser refresh during a running operation. The
  focused local run passed 4 tests with zero failures/skips. The full
  [clean-source GitHub run](https://github.com/piotrek1459/k8s-internal-platform/actions/runs/45220381729)
  passed **20 tests, 0 failures, 0 skips** at `0a616fb`, with downloaded JUnit and
  the strict summary verified. These tests do not establish native Pi acceptance.
- Pull requests run uncached Go regressions, UI type checking/lint/build, OpenAPI
  validation, Python/shell checks and the chart lint/render/negative/synchronization
  matrix through `tests/run.py`. The fast jobs do not create a cluster.
- The real-cluster API/browser/bundle suite runs nightly at **02:00 UTC**, manually,
  and as a release gate. Status serialization and Pod replacement are required
  assertions. Unhealthy updates must restore the previous snapshot before unlock.
  Authenticated registry inspection and actual kubelet image pulls are covered.
  The [earlier 16-case source run](../tests/experiments/2026-11-23-v0.1.4-and-diagnostics.md#executed-source-verification)
  includes bounded logs/events, hostile-text escaping and owned drift repair.
  The [earlier 15-case source run](../tests/experiments/2026-11-23-core-acceptance.md#migration-and-blocked-deployment-follow-up)
  also verifies runtime SIGKILL during a pending first install, durable blocking,
  independent queued browser work and supported blocked-first-deployment removal.
- `tests/e2e/native.py` exercises a published ARM64 release on an explicitly
  selected installation through loopback SSH forwards. It creates an administrator
  only with `--setup`, retains private local credentials and one owned test app,
  supports post-reboot verification with `--resume`, and optionally checks the
  unprivileged Nginx browser workflow with `--public-image` and owned diagnostics/
  drift/physical Secret cleanup with `--diagnostics`. The separate two-phase
  `--secret-cleanup-phase hold|resume` verifies native browser guards and partial
  cleanup across an operator-performed broker replacement.
- The [v0.1.5 report](../tests/experiments/2026-11-23-v0.1.5-secret-lifecycle.md)
  distinguishes published Pi lifecycle/diagnostics/reinstall/reboot success from
  failed v0.1.5 Secret cleanup and the subsequent source correction. Focused source
  browser/cleanup acceptance passed 5 tests with zero failures/skips. The full
  [clean-source GitHub run](https://github.com/piotrek1459/k8s-internal-platform/actions/runs/45194804590)
  passed **17 tests, 0 failures, 0 skips** at `8804ced`, with downloaded JUnit and
  the strict summary verified. The correction introduced in v0.1.6 subsequently
  passed native v0.1.7 verification; the intermediate v0.1.6 release was not installed.
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

1. Merge the source-accepted [private LAN implementation](private-lan.md),
   publish a release manually and validate that unmodified release
   on the Pi from another LAN device, including DNS/trust, Secure cookies and the
   TLS/SSH fallback. No later implementation PR starts before this validation.
2. Complete the remaining core reliability/security matrix in
   [issue #16](https://github.com/piotrek1459/k8s-internal-platform/issues/16):
   deterministic worker crash barriers, failed restoration/recovery retry, expected
   route absence before lifting the interim access-transition guard,
   and comprehensive negative-path Secret disclosure acceptance.
3. Define repetitions/workload/targets before reference-Pi resource calibration
   and manual-Helm versus UI/API evaluation. Separate cold pulls from cached images;
   the existing successful functional runs do not provide a performance dataset.
4. Proceed to application volumes/verified backup restoration (M5), public hosting
   (M6) and the remaining upgrade/observability work under their separate criteria
   after the applicable core acceptance is complete.

The [milestone checklists](milestones.md) retain the complete project plan.
