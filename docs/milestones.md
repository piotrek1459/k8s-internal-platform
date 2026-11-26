# Milestones, workflows, tests, and open decisions

Part of the [application platform plan](app-plan.md). This execution plan defines
the milestone sequence, acceptance criteria, implementation risks, verification
workflows, evaluation requirements, outstanding decisions, and proposed source
layout. The design is specified in [architecture.md](architecture.md),
[api-and-data.md](api-and-data.md),
[networking-and-security.md](networking-and-security.md), and
[operations.md](operations.md).

> **Scope.** M0–M8 remains the full project target. **M0–M4 defines the core
> prototype**, including a required evaluation of the implemented application
> lifecycle. **M5–M8 defines the planned extensions** for application persistence,
> public hosting, distribution, and extended evaluation and observability. These
> extensions are required for the corresponding complete product capabilities.
> Core prototype completion does not establish academic sufficiency; the thesis
> scope remains subject to the applicable requirements and supervisory agreement.

Implementation proceeds through verifiable workflows. A checked item means the
stated behavior has evidence at the named scope: unit/handler, disposable real
cluster, browser or reference Pi. It does not automatically accept a broader
hardware, security or evaluation requirement. Compound items have been separated
where implementation is complete but further acceptance remains outstanding.
The M0–M4 core remains incomplete; accepted requirements and remaining work are
identified separately below.

## Implementation readiness and entry criteria

Updated: 2026-11-25. The audit includes merged changes through PR #23,
published `v0.1.10`, its [recovery/native report](../tests/experiments/2026-11-25-v0.1.10-native.md),
the [v0.1.8 LAN native report](../tests/experiments/2026-11-24-v0.1.8-native.md),
the preceding [v0.1.7 native report](../tests/experiments/2026-11-23-v0.1.7-native.md),
the [source resilience report](../tests/experiments/2026-11-23-core-resilience.md),
the [private LAN source acceptance](../tests/experiments/2026-11-24-private-lan.md)
and the current regression/E2E suites. Remaining work is tracked in
[issue #16](https://github.com/piotrek1459/k8s-internal-platform/issues/16).
Older dated reports remain evidence only for their recorded source and scope.

| Milestone | Completed implementation/evidence | Remaining acceptance or implementation |
| --- | --- | --- |
| M0 | Reference hardware/OS/storage records, pinned native K3s, published ARM64/AMD64 demo bundles, reference-Pi manual Helm functional baseline and v0.1.8 separate-workstation LAN access. | Human operator-active-time comparison and final published profile/rollout validation. The published v0.1.9 installer correction is accepted. |
| M1 | Workload/Service/ownership/probe/private-entrypoint chart, healthy/unhealthy workflows, v0.1.3 temporary-files compatibility and v0.1.8 reference-LAN routing. | Published source-chart 0.1.3 profile/draining validation; explicit native profile and source-chart comparison are retained. |
| M2 | Validated API, atomic migrations/acceptance, serial worker, frozen snapshots, configuration Secrets and native persistence; source crash/restoration/disclosure matrix. | No open listed M2 acceptance item; request-availability evaluation remains M4. |
| M3 | Released setup/auth/forms/history/Secret and credential management/logs/events; source queued/running refresh, native v0.1.7 Secret cleanup/retry and v0.1.8 LAN HTTPS/browser/URL acceptance. | No open listed M3 acceptance item. The installer gate passed in v0.1.9; broader failure/security work remains under M2/M4. |
| M4 | Healthy lifecycle, verified unhealthy-update restoration, blocked first-deployment removal, drift/repair and source interruption/failed-compensation matrix. | Published profile/draining validation and the agreed human active-time trial; repeated system data is retained. |
| M5–M8 | Native installer/release distribution portions of M7 are implemented and the Pi core install/reinstall/reboot run passed. | Application data/backup, public hosting, supported upgrades/automation and extended monitoring retain their separate requirements. |

The two former status/restart E2E skips are removed: both are required assertions.
Current E2E acceptance requires a non-empty run with zero failures and zero skips.
An older workflow summary is not evidence for the current source; the summary
records the tested commit. Public image distribution through native OCI release
bundles is accepted; publishing platform images to a registry is optional.

## M0 — Constraints and baseline (core)

**Objective:** a verified reference environment and reproducible manual Helm
baseline for the later platform comparison. The installer now performs pinned
K3s provisioning; repeating its installation steps manually is unnecessary.
Manual application deployment remains relevant to the operator-effort baseline.

**Tasks:**

- [x] Reference host model, architecture, OS, RAM, storage and post-install free disk are recorded. **Evidence:** [Pi report](../tests/experiments/2026-11-22-raspberry-pi.md).
- [x] The demo listening address, port, readiness/version endpoints, configuration and logging contract are documented. **Evidence:** `examples/http-demo/`, its Go tests and [application contract](api-and-data.md#application-contract).
- [x] Healthy v1/v2 and deliberately unhealthy v3 are distributed as native ARM64/AMD64 OCI images. **Evidence:** both published release bundles and `release.json`; a registry account is not required.
- [x] Pinned K3s provisioning on the Pi reaches a Ready node and records version/configuration. **Evidence:** the published `v0.1.2` installer and reboot run. This replaces the earlier manual-K3s-install method, not the manual Helm comparison.
- [x] A local manual Helm install/update/unhealthy-update/explicit-recovery procedure and results are recorded. **Evidence:** [local baseline](../tests/experiments/2026-10-14-local-m0-m1.md) and `tests/integration/helm-lifecycle.sh`. The dated raw local run was removed; its curated report is historical evidence.
- [x] Repeat and retain the manual Helm functional baseline on the reference Pi with current pins/images and raw results. **Evidence:** [v0.1.3 Pi baseline](../tests/experiments/2026-11-23-core-acceptance.md#direct-helm-baseline-on-the-pi), exact commands, source/archive hashes, values, responses and history. Operator active time remains M4 work; the current repeated backend comparison is retained in the [evaluation](../tests/experiments/2026-11-26-core-evaluation.md).
- [x] Verify application access from another LAN device through the intended private route. **Evidence:** published v0.1.8, direct workstation hosts/CA trust, normal browser and actual app HTTPS, source/header/SNI/IPv6 isolation and reinstall/reboot retention. [Native report](../tests/experiments/2026-11-24-v0.1.8-native.md). Router wildcard DNS was not automated.
- [x] Publish and verify the installer preflight exit-status correction. **Evidence:** unmodified v0.1.9 installer, all read-only preflight variants, fresh installation, normal trusted LAN browser/API flows, original session/identity/config retention across same-version rerun and actual reboot. [Native report](../tests/experiments/2026-11-24-v0.1.9-native.md). The v0.1.8 failure remains historical evidence.

**Completion boundary:** host installation and reference-LAN runtime access are
verified, including the corrected published v0.1.9 installer. Controlled
measurements/operator comparison supporting M1/M4 remain outstanding.

## M1 — Application chart (core)

**Objective:** the first-party stateless HTTP chart, including workload, service,
probes, resources, ownership and private routes.

**Tasks:**

- [x] Deployment and Service render and validate against K3s. **Evidence:** shared chart lint/render/negative matrix, historical server-side dry-run and current real-cluster E2E installation.
- [x] Readiness, startup and optional liveness probes are configurable; startup/readiness API paths are independent. **Evidence:** chart matrix, probe-mapping regressions and the unhealthy-v3/native restoration workflow.
- [x] Replace provisional requests/limits with a measured reference-Pi preset in source. **Evidence:** the [controlled dataset](../tests/experiments/2026-11-26-core-evaluation.md) fixes targets/methods before collection, retains six complete load trials, cold/cached pulls, CPU and lifetime/startup peaks and derives 25m/48Mi requests with 250m/128Mi limits. Explicit native Go/Nginx two-replica checks passed. Published-default validation remains a separate gate below.
- [x] Managed resources have stable application ownership and release selectors. **Evidence:** rendered/installed Deployment, Service and Ingress label/selector checks.
- [x] Private routes explicitly select the private entrypoint. **Evidence:** isolated local listener/forged-Host baseline checks. Published v0.1.8 also passed reference-LAN routing and isolation under M0/M3.
- [x] Healthy install/update and unhealthy failure diagnostics operate on K3s. **Evidence:** disposable real-cluster and published `v0.1.2` Pi workflows.
- [x] Source chart 0.1.1 supplies a size-limited temporary `/tmp` volume and non-root filesystem-group access while retaining the read-only root filesystem. **Evidence:** rendered-manifest regression, full source E2E and the controlled native Nginx experiment.
- [x] Verify temporary files using a published installer/release without patching a Deployment manually. **Evidence:** unmodified `v0.1.3` Nginx browser deployment reached Ready and HTTP 200; [native report](../tests/experiments/2026-11-23-core-acceptance.md).

**Completion boundary:** source chart behavior and published compatibility are
verified, including native LAN routing. Resource calibration and explicit native
profile checks are retained. Source chart 0.1.3 adds native shutdown delay and
requires Kubernetes 1.34+; publication/default/draining validation remains open.

## M2 — Durable backend (core)

**Objective:** in-cluster Go API, transactional queue/state, immutable execution
snapshots, configuration Secrets, Helm integration and observed status.

**Tasks:**

- [x] Health/system/image/application/operation/Secret routes have a validated OpenAPI contract and aligned supported response shapes. **Evidence:** full schema validation, handler contract tests and browser workflows. Frontend types are maintained manually, not generated.
- [x] SQLite migrations create the metadata schema, reopen idempotently and enforce foreign keys. **Evidence:** `TestMigrateCreatesSchema`, `TestMigrateIsIdempotent`, `TestForeignKeysEnforced` in `broker/internal/store/db_test.go`.
- [x] Inject failing migration statements and ledger insertion, reopen the database and verify no partial schema/data/ledger changes. **Evidence:** `TestMigrationFailureRollsBackSchemaDataAndLedger` also verifies corrected retry/idempotence. Backup restoration and cross-version upgrade support remain M5/M7.
- [x] Application, candidate revision, operation, first event and audit acceptance is atomic. **Evidence:** store atomicity and rollback-on-failure regressions.
- [x] The platform chart provides Service/PVC/service account and scoped RBAC; the broker deploys into `platform-apps` through in-cluster credentials. **Evidence:** local real-cluster and native release installation.
- [x] Verify protected management and Kubernetes refusal from an actual managed application. **Evidence:** `test_03_workload_management_boundaries` requires positive platform reachability, verified Kubernetes TLS/Status refusals, absent service-account token and unchanged resource counts. This is the ordinary uncredentialed workload boundary; broader disclosure/attack cases remain open.
- [x] The single-replica/Recreate platform configuration preserves completed application state, history and sessions through broker replacement; Pi reinstall/reboot preserves the PVC identity. **Evidence:** E2E broker replacement and native identity/session comparisons.
- [x] Interrupt a real pending first install with broker runtime SIGKILL; preserve Helm evidence and durable blocking across replacement, while independent queued work completes once. **Evidence:** `test_95_pending_first_install_and_queued_browser_resume`; [report](../tests/experiments/2026-11-23-core-acceptance.md).
- [x] Complete disposable-cluster before-Helm, pending, after-apply and before-terminal-SQL interruption barriers with full frozen Secret/chart/values identity checks. **Evidence:** required `test_32`–`test_35`, actual runtime SIGKILL and observed external barriers; [source report](../tests/experiments/2026-11-24-core-recovery.md). Native acceptance of the new paths is tracked separately below.
- [x] Manual chart/development access and published native installation procedures are recorded. **Evidence:** local development and Linux installation guides.
- [x] Configuration Secret creation/versioning, immutable Kubernetes values, metadata-only records, exact version references and deletion guards operate. **Evidence:** Secret handler/store tests, browser reference submission and real-cluster configuration rollback.
- [x] Complete the controlled disposable-cluster negative-path disclosure matrix. **Evidence:** `test_37` scans actual SQLite/WAL/SHM with a positive control, decoded Helm payloads including failed descriptions, ordinary/diagnostic responses, raw broker logs and sanitized artifacts after real Secret/Helm/Deployment admission errors. Raw/base64/authentication forms are absent; [source report](../tests/experiments/2026-11-24-core-recovery.md). This finite matrix is not a proof against arbitrary disclosure; published native validation is recorded below.
- [x] Image inspection resolves digests and Linux architectures; incompatible images map to `IMAGE_ARCH_UNSUPPORTED`. **Evidence:** registry and handler tests, native ARM64 inspection and public/private real-cluster pulls.
- [x] Serial claiming and one-shot snapshot freezing preserve recorded image, chart/version/content digest, values hash, Secret versions and restart identity. **Evidence:** store/worker/snapshot regressions and lifecycle E2E.
- [x] Verify real moved-tag API replay, completed-operation broker replacement and explicit Reapply retain the committed digest. **Evidence:** `test_07_lost_response_and_moved_tag_retries` confirms the registry tag actually changes, with one operation/application/revision, matching chart/values hashes and real HTTP v1. Mid-flight worker-crash variants remain under the interruption matrix above.
- [x] The Helm adapter installs recorded values and the observer caches time/replicas/endpoints with stale status on read failures. **Evidence:** adapter/observer tests and real K3s operations.
- [x] Acceptance returns `202`/Location and ordinary identical retries preserve the operation/application identity. **Evidence:** handler/store regressions and E2E idempotency assertions.
- [x] Drop a response after durable acceptance and verify retry/completion without another operation/application. **Evidence:** the loopback fixture reads the backend's committed 202, closes the client connection without delivering it, and requires identical-key replay of the same IDs. A rejected backend response cannot satisfy the fixture. See the [source report](../tests/experiments/2026-11-23-core-resilience.md).

**Completion boundary:** the normal durable backend is implemented and tested.
The source worker-crash/restoration/security matrix has disposable-cluster evidence.
Published v0.1.10 also passed the native failure/retry/disclosure gate;
[report](../tests/experiments/2026-11-25-v0.1.10-native.md). Controlled evaluation remains M4.

## M3 — Administrator UI (core)

**Objective:** authenticated administration with clear forms/status/progress,
configuration/history and diagnostics over a private management path.

**Tasks:**

- [x] Single-use expiring bootstrap setup creates the administrator; root navigation selects setup on a fresh installation. **Evidence:** auth service/handler tests, browser setup and native invalid-token/error checks.
- [x] Login/logout, HttpOnly/SameSite sessions, expiry/revocation and CSRF/mutation checks operate. **Evidence:** auth regressions/E2E; the original native session survived reinstall/reboot.
- [x] Verify private LAN HTTPS through a published release. **Evidence:** v0.1.8 on the Pi, separate-workstation normal browser CA trust/hosts resolution, Secure host-only cookies, source/listener isolation, initial SSH bootstrap and verified TLS/SSH fallback. [Native report](../tests/experiments/2026-11-24-v0.1.8-native.md). The installer correction passed native v0.1.9 acceptance under M0; M6 public hosting is separate.
- [x] Application list, image inspection, supported creation fields and Ready/status views operate for Internal exposure. **Evidence:** real browser lifecycle and API response contract checks. Source defaults use the measured M1 reference profile; published-default validation remains open.
- [x] Verify the configured LAN application URL through the published private route. **Evidence:** v0.1.8 browser wizard deployment, URL retained after reload, actual separate-workstation HTTPS Nginx/demo content and API update/scale/restart/rollback; hostname and transition guards passed. [Native report](../tests/experiments/2026-11-24-v0.1.8-native.md).
- [x] Secret metadata/version objects and application Secret-reference submission use the API contract. **Evidence:** write-only API tests, browser reference selection/submission and exact-version rollback.
- [x] Verify browser Secret creation/version management and credential creation/selection/private-image/invalid-password workflows. **Evidence:** focused real-cluster browser lifecycle/private-registry cases verify exact Secret contents privately, actual private HTTP, deletion guards and physical Kubernetes cleanup. The full 17-case CI suite passed without failures/skips; release/native boundaries remain recorded in the [report](../tests/experiments/2026-11-23-v0.1.5-secret-lifecycle.md).
- [x] Natively verify the Secret cleanup correction introduced in v0.1.6. **Evidence:** published v0.1.7 passed referenced API/browser refusal, physical version removal after dependent-app deletion and controlled partial cleanup across actual broker replacement. Original session/PVC and reservation survived; unresolved ownership was refused, guarded fixture repair enabled browser retry, values/metadata were removed and history remained. Original v0.1.7 identities/HTTP also survived reinstall/reboot. [Report](../tests/experiments/2026-11-23-v0.1.7-native.md). v0.1.6 itself was not run natively.
- [x] Application details include observed status, revision history and operation progress/events. **Evidence:** handler contracts and browser update/history/rollback workflows.
- [x] Add bounded application log/event reads and escaped browser rendering. **Evidence:** handler/ownership/redaction regressions and the focused real-cluster fixture test; namespace/UID chains, cross-app/copied-label rejection, real sensitive fixture masking and hostile-text escaping are verified. Full disclosure acceptance remains separately open.
- [x] Registry credentials are stored in Kubernetes and support real private inspection/pulls; ordinary responses remain metadata-only and invalid credentials fail. **Evidence:** registry/credential regressions and authenticated-registry E2E. Full private browser flow remains above.
- [x] A completed operation can be reloaded from its persisted state. **Evidence:** browser operation-page refresh.
- [x] Refresh/navigate while an operation is deliberately queued behind a pending install, then recover its persisted progress after broker replacement without resubmission. **Evidence:** browser submission count, one revision and actual HTTP checks in `test_95_pending_first_install_and_queued_browser_resume`.
- [x] Verify refresh during a deliberately running operation as a separate browser case. **Evidence:** `test_12_browser_running_operation_resume` observes the running/pending/unready barrier, reloads/navigates, explicitly releases readiness and verifies completion/reload with one submission and revision.

**Completion boundary:** setup/authentication, lifecycle, diagnostics and the
specified queued/running browser refresh and Secret/credential paths have
evidence. Published v0.1.8 also passed normal private LAN HTTPS and application-URL
acceptance. The separate installer correction passed published v0.1.9 native acceptance in M0.

## M4 — Reliable lifecycle and core evaluation (core)

**Objective:** tracked lifecycle, verified restoration/rollback, durable blocking,
crash recovery, private LAN administration and measured evaluation of the core prototype.
Application data volumes remain outside this milestone.

**Tasks:**

- [x] Image/configuration/resource/available exposure/replica changes share the tracked revision/Helm path; configuration-only updates retain the image digest. **Evidence:** mutation/worker tests and API lifecycle E2E.
- [x] Unhealthy-v3 rollout failure restores and verifies the prior successful v2 snapshot before recording the failed operation and releasing its lock. **Evidence:** real-cluster E2E and the published `v0.1.2` native run.
- [x] Unverified restoration retains a failed candidate, nonterminal `needs_attention` and conflict guards; version-guarded recovery retry retains operation identity. **Evidence:** store/worker/recovery unit regressions.
- [x] Provide supported removal of a blocked first deployment without a successful predecessor. **Evidence:** atomic store rejection/rollback/replay regressions and real browser deletion after runtime SIGKILL, retained blocking and a subsequent broker replacement. Failed candidate/snapshot/history remain recorded.
- [x] Verify failed restoration and failed verification across actual broker restarts in a disposable cluster. **Evidence:** `test_36` retains failed/frozen evidence and 409 guards for Internal/LAN/hostname transitions, clears candidate-only fields and verifies the prior route/absence before supported retry unlocks. The interim 422 guard is removed in source; [report](../tests/experiments/2026-11-24-core-recovery.md). Published native acceptance is recorded below.
- [x] Supported rollback creates a new revision preserving the selected image, exact Secret versions and available chart/values identity. Unavailable chart artifacts are refused rather than substituted. **Evidence:** native rollback, configuration-version E2E and chart/snapshot regressions. Historical-chart retrieval across platform upgrades remains M7 scope.
- [x] Restart replaces Pods with a stable operation token and fixed image digest; stateless scale 2/0/1 reaches its expected states. **Evidence:** mandatory status/restart assertions, API E2E and the native workflow. There are no acceptance skips for these paths.
- [x] Tracked stateless deletion removes Helm-managed workload/routing resources and retains history. **Evidence:** API/browser/native deletion checks. Dependent Pod garbage collection is asynchronous and has a bounded native acceptance check.
- [x] Startup recovery classifies queued/interrupted/pending/unknown states and verifies target release/chart/values/live identity before success. **Evidence:** positive/negative recovery and live-verifier regressions.
- [x] Complete the disposable real-cluster interruption matrix around Helm and terminal SQL recording. **Evidence:** `test_32`–`test_35` require exact target completion or durable blocking, stable frozen identity and unchanged pending evidence without deleting Helm records; [report](../tests/experiments/2026-11-24-core-recovery.md).
- [x] Validate the recovery/security PR through its unmodified published release on the reference Pi. **Evidence:** v0.1.10 native lifecycle, all three forced restoration/verification failures with six replacements and unchanged failed/frozen evidence, trusted browser recovery, controlled disclosure/file scans, tracked access transitions, original session/identity retention through reinstall/reboot and guarded cleanup. [Report](../tests/experiments/2026-11-25-v0.1.10-native.md). Request-timeout/route-convergence observations remain in the evaluation scope below.
- [x] Explicit reapply recreates a manually deleted Service from recorded state without changing its image. **Evidence:** real-cluster Service-deletion/reapply E2E.
- [x] Report owned workload/route drift against frozen intent without automatic overwrite or configuration values. **Evidence:** field/ownership/route unit matrix and real-cluster image/env/resources/probe/replica/Service-port drift, plus explicit Reapply and HTTP verification. Guarded Service-list repair preserves identity/address; pending/foreign ownership remains blocked. See the [source report](../tests/experiments/2026-11-23-v0.1.4-and-diagnostics.md).
- [ ] Complete the [minimum core evaluation](#minimum-core-evaluation-m4) against the reference manual baseline. **Acceptance:** predefined targets/repetitions, operator steps/active time, timing/resource measurements and environment/raw results are retained. The [repeated system dataset](../tests/experiments/2026-11-26-core-evaluation.md) is retained; human active time/steps and final published-default/draining validation remain open in issue #16.

**Completion boundary:** supported healthy lifecycle and failed-update restoration
are verified. The new failure/interruption matrix has source-cluster evidence;
published v0.1.10 native acceptance is complete. The repeated system/resource
dataset and native source-chart drain comparison are retained. Published new
default/draining validation and the human timed walkthrough remain open. M5–M8 do not replace these core obligations.

## M5 — Application persistence and backup (extension)

**Objective:** the single-volume persistent profile, retained-volume lifecycle,
and platform and application backup and restore procedures. Platform metadata
persistence and immutable application secrets are already required by M2.

**Risk and completion boundary:** moderate to high risk due to potential data
loss. Completion requires data preservation through replacement and reboot,
retention after application deletion, and a successful restoration. M5 may be
reordered with M6 when the separate application's storage requirements permit.

**Tasks:**

- [ ] The persistent profile supports one PVC and mount path, with the PVC managed outside the workload release. **Acceptance:** data survives application replacement and host reboot.
- [ ] Persistent applications use the specified single-instance update policy. **Acceptance:** replicas zero and one are supported, values above one are rejected, and expected update downtime is documented.
- [ ] Application deletion preserves associated volumes and history. **Acceptance:** the workload and routes are removed while the retained volume remains visible with its previous association.
- [ ] `storage.existingVolumeId` supports explicit reuse with ownership, class, and mount validation. **Acceptance:** an in-use volume cannot be attached to another application.
- [ ] Volume deletion is separate, confirmed, and subject to an in-use check. **Acceptance:** deletion of an attached volume is refused.
- [ ] Platform SQLite backup uses a consistent backup API or controlled stopped-service procedure. **Acceptance:** restoration to replacement storage reproduces the recorded platform state.
- [ ] A three-part backup procedure covers the platform, K3s datastore, and application data. **Acceptance:** a complete restoration rehearsal and measured recovery time are recorded in `tests/experiments/`.

**Completion evidence:** verified persistent-profile restrictions, retained data
after application deletion, explicit volume reuse, and successful restoration.

## M6 — Public application (extension)

**Objective:** configuration of one public ingress mode, domain and TLS, and
publication of the separate application over HTTPS.

**Risk and completion boundary:** high risk due to external dependencies such as
ISP routing, CGNAT, DNS, and certificate issuance. The selected mode must follow
the actual host's connectivity constraints, as specified in the
[public mode decision](networking-and-security.md#public-mode-decision).
Completion requires external verification of public access and rejection of
management and private application routes.

**Tasks:**

- [ ] The host and public access mode are selected from measured network capabilities. **Acceptance:** the direct-ingress or tunnel decision and its rationale are recorded.
- [ ] Public and private listeners are separated; public access remains blocked until configured. **Acceptance:** forged private Host/SNI requests to the public listener are rejected.
- [ ] The selected TLS path is configured and documented. **Acceptance:** direct ingress uses cert-manager with ACME staging verification before production; tunnel mode verifies provider-managed HTTPS and documents the origin connection. Certificate renewal responsibilities are recorded for the selected mode.
- [ ] The separate application is published at its allowed public hostname. **Acceptance:** HTTPS succeeds from an external client outside the host's LAN.
- [ ] Management and private applications remain inaccessible through the public endpoint. **Acceptance:** external requests to their routes fail, including the supported IPv6 path.

**Completion evidence:** externally verified HTTPS access to the application and
verified rejection of private and management traffic.

## M7 — Repeatable distribution (extension)

**Objective:** a published-release installer usable without a checkout, clean-install verification,
platform upgrade and recovery, ARM64/AMD64 release artifacts, and GitHub release
packaging. Scoped automation tokens and a sample CI deployment use an approved
private network path.

**Risk and completion boundary:** moderate risk, mainly from installation and
upgrade compatibility across supported hosts. Automation builds on the manual
procedures established in M0 and M2. Completion requires installation from the
documented procedure on a clean rented AMD64 server, verifying portability beyond
the Pi reference host. Public-mode acceptance is repeated where the selected
host and networking mode differ from M6.

**Tasks:**

- [x] The installer implements preflight checks, K3s setup, namespaces, RBAC, and platform chart installation. **Acceptance:** a fresh reference Pi reaches the setup wizard using the published installer. Native `v0.1.2` installation, same-version rerun and reboot evidence is recorded in the [Pi report](../tests/experiments/2026-11-22-raspberry-pi.md).
- [ ] A clean installation is performed on a rented AMD64 server. **Acceptance:** the platform operates using only the documented installation procedure.
- [ ] Platform upgrade includes backup, compatibility validation, chart upgrade, and failure recovery. **Acceptance:** both a successful upgrade and recovery through rollback with state restoration are exercised.
- [x] Manually dispatched releases publish ARM64 and AMD64 images and installation assets. **Acceptance:** both native OCI image bundles, installer assets, recorded digests and published checksums are available on GitHub Releases. Distribution uses direct containerd import; registry publishing is optional.
- [ ] Machine-token endpoints provide hashed storage, application/action scopes, expiry, and revocation. **Acceptance:** a token scoped to one application cannot access another.
- [ ] A sample CI deployment invokes the API over a private path and polls the operation. **Acceptance:** an image publication followed by an authorized API call completes deployment without public exposure of administration.

**Completion evidence:** reproducible installation on the supported targets,
tested platform upgrade and recovery, and scoped deployment automation.

## M8 — Extended evaluation and observability (extension)

**Objective:** configuration and measurement of the full observability profile,
evaluation of the implemented extensions, and assessment of the final user
workflows. The minimum core evaluation is already required by M4.

**Risk and completion boundary:** moderate integration and capacity risk, with
substantial measurement effort. Completion requires reproducible results tied to
commits, hardware, component versions, and configuration. Monitoring overhead is
evaluated against the lightweight core profile.

**Tasks:**

- [ ] Prometheus, Grafana, Loki, and Alloy operate with bounded retention. **Acceptance:** additional CPU, RAM, and disk use are measured against the core profile.
- [ ] Dashboards cover deployment outcomes and duration, API errors and latency, restarts, ready replicas, and disk capacity. **Acceptance:** each dashboard displays measured platform data.
- [ ] Alerts cover repeated deployment failures and sustained resource pressure. **Acceptance:** controlled failure scenarios trigger the expected alerts.
- [ ] Functional, performance, and failure evaluation covers the implemented extensions. **Acceptance:** raw results and environment metadata are recorded according to the [evaluation plan](#tests-and-thesis-evaluation).
- [ ] The manual-baseline comparison is repeated for extended workflows where applicable. **Acceptance:** operator steps, active time, and deployment timings use the same definitions as the core evaluation.

**Completion evidence:** reproducible extension results and measured
observability overhead, supplementing the mandatory core evaluation.

## Core execution and evidence order

The LAN/installer cycle passed published v0.1.9 native acceptance; temporary-files
compatibility passed in v0.1.3.

1. Published v0.1.10 completed the native failure/recovery/security gate in issue #16.
   The final resource/evaluation PR follows, then manual release/native validation.
2. The fixed-plan system/resource dataset, derived profile, explicit native profile
   checks and old/source-chart request comparison are retained. Complete source
   checks, then merge/manual release and validate the published defaults/draining.
3. Perform the agreed human walkthrough; retain actual steps and active/elapsed
   time. Reconcile final native/data evidence before closing issue #16.

The demo, charts, API, queue, UI and release tooling already exist. This order
does not require recreating that foundation. M5–M8 remain separate extensions;
their implementation must not be used to label unfinished core criteria complete.

## Example end-to-end workflows

These workflows define verification scenarios for the corresponding milestones
and provide the basis for manual test procedures.

### A. First LAN deployment

1. The platform is installed, the administrator account is created, and private access and DNS are configured.
2. A demonstration image is built on the development machine and published.
3. Its reference, port, and readiness endpoint are submitted through the UI.
4. The persisted operation reaches completion and the LAN URL is accessed from another device.
5. Logs and history are inspected; application traffic is verified while the broker is restarted.

### B. Private registry deployment

1. A registry pull credential with the required scope is added through the UI.
2. The credential is selected for a private image reference.
3. Architecture and access are validated; invalid credentials produce a specific error.
4. Deployment uses a referenced image-pull Secret. Secret values are verified absent from history, ordinary API responses, and rendered Helm values.
5. Credential rotation is followed by a verified image pull before retirement of the previous credential.

### C. Update and failure recovery

1. Version 2 of the demonstration application is published.
2. An update is submitted after review of the configuration changes; readiness progress is observed.
3. History records the previous and new digests and the successful outcome.
4. An update to the deliberately unhealthy version 3 is submitted.
5. Failure diagnostics and verified restoration of version 2 are recorded. A separate recovery-failure scenario verifies `NeedsAttention` and continued rejection of conflicting mutations after broker restart.

### D. Scaling a supported application

1. A stateless demonstration application is configured for two instances.
2. Resource policy validation precedes execution through the revision and Helm workflow.
3. Two ready instances are reported and requests are observed reaching both.
4. Zero instances produces Stopped; one instance is subsequently restored.
5. Following M5 implementation, a two-instance request for the persistent profile is rejected with a specific compatibility error.

### E. Publication of the separate application

1. The application's port, architecture, configuration, dependencies, storage, health checks, and migration procedure are specified.
2. Its image is published and the LAN workflow is verified.
3. A domain and DNS are configured for the selected direct-ingress or tunnel mode. Installation-level public networking is configured once.
4. An update selects Public exposure and an allowed hostname through the tracked mutation workflow.
5. HTTPS is verified externally; management and private application routes remain inaccessible through the public endpoint.
6. Update and recovery are exercised. Where persistent data is used, backup restoration is documented and verified.

### F. Application deletion

1. The application and any attached storage are displayed before deletion is accepted.
2. Owned routing and workload resources are removed before the application is marked deleted.
3. History remains visible according to retention policy. Following M5 implementation, associated volumes are retained and displayed with their previous association.
4. M5 volume deletion requires a separate confirmed operation and verification that the volume is unused.

## Tests and thesis evaluation

### Minimum core evaluation (M4)

Core completion requires functional, failure, and performance evidence for the
implemented M0–M4 workflows. The required dataset includes:

- Deployment from public and private registries, valid and unhealthy updates, rollback with exact secret versions, restart, stateless scaling, and deletion.
- Duplicate-request and client-disconnection handling, platform pod replacement with retained state, and interrupted or failed recovery with durable mutation blocking.
- Authentication, secret non-disclosure, private route isolation, and continued application traffic during broker unavailability.
- A comparison with the M0 manual Helm baseline using a stated definition of an operator step and recorded active operator time.
- Deployment and update duration with cold and cached pulls distinguished, failure detection and verified recovery time, and idle and loaded resource use.

Measurements use recorded procedures, numerical acceptance targets, repetitions,
and raw results. Full Prometheus/Grafana/Loki deployment is not a prerequisite:
timestamps, structured logs, API observations, and recorded host measurements
provide the initial evidence. M8 extends this dataset to later capabilities and
measures the additional observability profile.

### Verification coverage

| Area | Cases to cover | Stage |
| --- | --- | --- |
| Shared cluster architecture | Broker and application share the reference cluster; application traffic continues during broker unavailability; host-side recovery restores management; workloads cannot access protected management endpoints. | M2–M4 |
| Validation | Invalid image, name, port, architecture, secret or credential reference, and unavailable exposure mode. | M2–M4 |
| Deployment | Public/private image pulls, invalid credentials, nonexistent tag, unavailable registry, and application binding only to localhost. | M2–M4 |
| Lifecycle | Valid update, failed readiness, failed recovery, rollback with secret versions, restart, stateless scaling to 0/1/2, and deletion. | M4 |
| Durable state | Client disconnection, duplicate request, stale application version, broker restart around a Helm mutation, and ambiguous pending release. | M2–M4 |
| Capacity | Insufficient memory, unschedulable replicas, large image pulls, and disk pressure; monitoring retention limits in the extended profile. | M1–M4; M8 extension |
| Persistence | Platform state survives pod replacement; application restart/reboot, persistent scaling rejection, retained volumes, volume deletion, and backup restoration follow with M5. | M2; M5 extension |
| Networking | Private routing and DNS failure; public DNS/TLS, external access, forged private Host/SNI, and applicable IPv6 restrictions follow with M6. | M1–M4; M6 extension |
| Authentication | Setup token expiry/reuse, login/logout, session expiry, unauthenticated mutations, CSRF, and secret non-disclosure. | M3–M4 |
| Distribution | Repeated installation, partial failure, clean rented-server installation, supported upgrade, and recovery. | M7 extension |
| Usability | Deployment, update, and recovery through the documented UI, with actionable image, port, and health errors. | M3–M4; M8 extension |

Unit tests cover domain policies and operation transitions. Integration tests
exercise actual Helm and Kubernetes behavior. Browser tests cover the principal
administrator workflows. Disposable local clusters may support development;
reference-host K3s runs provide evidence for the advertised deployment targets.

Evaluation compares the platform with the same manual Helm deployment baseline.
The full measurement set comprises:

- Operator steps and active operator time under a stated step definition.
- Deployment and update duration, with cold image pulls and cached runs separated.
- Failure detection and verified recovery time.
- Idle and loaded CPU, RAM, and disk use; M8 adds comparison with the full observability profile.
- Request success rate and latency under a fixed workload and supported replica counts.
- Reproducibility across repeated deployments; M7 adds clean-install verification.

Numerical targets are established after M0 measurements and before the relevant
evaluation runs. Records include commit, component versions, image digests,
hardware, storage, network, cache state, repetitions, and raw results. Load is
generated from another machine where practical. Results are limited to the
measured conditions; single-host replicas do not establish high availability.

## Decisions still open

The following decisions have defined implementation deadlines. Initial version
selection and local access configuration are part of M0; later product decisions
do not prevent the first implementation increment.

| Decision | When required | Resolution approach |
| --- | --- | --- |
| Separate application database, uploads, and jobs | Before its persistence design. | A deployment contract is recorded when the application requirements are defined. |
| Direct public ingress or tunnel | M6. | Selection is based on measured reachability, router/firewall control, CGNAT, and provider requirements. |
| Primary production host | Before M6/M7. | The Pi remains the reference host; the production host is selected before its public network configuration. |
| Domain and DNS provider | M6. | Selection follows the public access mode; purchase is not required for M0–M4. |
| LAN resolver and private certificate trust | Initial configuration in M0; normal administration in M3; repeatable setup in M7. | Explicit host mapping is acceptable initially; private DNS and HTTPS trust procedures are documented before LAN administration. |
| SSD upgrade | Before sustained Pi operation with valuable data. | The decision uses measured microSD capacity and reliability requirements, with off-device backups retained. |
| Library and component versions | Partly recorded. | Go/JS dependencies and local versions are pinned in module/lock/version files. Local and CI chart checks share the Helm version pin. The release compatibility manifest records source, K3s/Helm pins and native image digests. Native Pi acceptance is recorded separately; native AMD64 and cross-version upgrades remain outstanding. |
| UI stack | Decided and implemented. | Svelte 5/SvelteKit static SPA, TypeScript and Tailwind; see the accepted [decision](decisions/m3-ui-stack.md). |
| Terraform scope | Before infrastructure evaluation. | Rented VPS and DNS provisioning is the candidate use case; an alternative boundary with the host installer requires a recorded rationale. |

The initial objective is a verified application lifecycle on the reference host,
with durable platform state, observed outcomes, and defined recovery behavior.

## Current and planned source layout

The broker, UI, API, charts, scripts, docs and thesis directories exist. Entries
marked planned identify work that is not implemented; placeholder directories do
not establish capability. The ordinary Go tests reside alongside package source.

```text
api/
  openapi.yaml                 management API contract
broker/
  cmd/platform/                executable entry point
  internal/httpapi/            HTTP handlers and response mapping
  internal/auth/               administrator sessions and setup
  internal/applications/       validation, revisions, lifecycle policies
  internal/operations/         durable queue, execution, recovery
  internal/store/              SQLite queries and migrations
  internal/helm/               chart/release adapter
  internal/cluster/            status, logs, events, Secret/PVC access
  internal/registry/           image inspection and credential use
  internal/networking/         planned dedicated mode adapters; validation currently lives in applications
  internal/audit/              sanitized activity records
  integration/                gated real-cluster Go test
web/
  src/                        UI screens and typed API client
deploy/helm/
  platform/                   broker/UI, RBAC, database volume
  application/                managed application workload chart
installer/                    dedicated Linux/K3s installer
release/                      manual branch-HEAD publisher, OCI bundles, source identity and pins
observability/                placeholder; monitoring profiles remain planned
examples/                     healthy/unhealthy demonstration versions
tests/
  run.py                      shared suite selector
  checks/                     fast checks and OpenAPI helper
  regression/                 CLI, evidence and release integrity regressions
  integration/                direct Helm baseline; gated Go test is broker/integration
  e2e/                        isolated build/API/browser/OCI-bundle workflows
  experiments/                repeatable measurement procedures/results
docs/
  project-outline.md          thesis context and original scope
  app-plan.md                 plan index
  status.md                   current capabilities, defects, evidence and next work
  architecture.md             system architecture, components, cluster layout, UI
  api-and-data.md             application contract, data model, API
  networking-and-security.md  visibility modes, isolation, secrets, backup
  operations.md               install, execution/recovery, observability
  milestones.md               implementation sequence and evaluation
  decisions/                  implementation decision records
infra/terraform/              optional reproducible VM/cloud/network setup
book/                         thesis source
```

Software installation on an existing host belongs to the repeatable installer.
Terraform's candidate responsibility is reproducible rented-server and DNS
provisioning. The selected boundary is documented in the relevant decision record.
