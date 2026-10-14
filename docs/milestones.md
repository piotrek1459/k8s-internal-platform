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

Implementation proceeds through complete, verifiable workflows. Evidence is
collected throughout development. Installation begins with a documented manual
procedure in M0, includes manual platform installation in M2, and is automated
incrementally. Basic tests, logs, and the minimum evaluation are part of M0–M4.
Each checklist item specifies an acceptance criterion. Unchecked items represent
planned work rather than verified capabilities.

The first implementation uses a local K3s development cluster and locally stored
images. The [local development guide](local-development.md) defines this workflow.
Local verification provides implementation evidence but does not satisfy the
reference Pi, second-device LAN, published-image, or hardware-calibration criteria.
The milestone checklists retain those outstanding acceptance requirements.

## Implementation readiness and entry criteria

The documented architecture and initial application contract are sufficiently
defined for implementation of M0 and M1 to begin. No additional architecture
decision is required before development of the demonstration application and
initial chart. This assessment concerns planning readiness; no milestone is
declared complete and no host measurements or runtime tests are implied.

| Implementation stage | Entry criteria |
| --- | --- |
| M0 and initial M1 development | The reference hardware, HTTP workload profile, shared-cluster topology, and LAN-first scope are defined. Host access, tool installation, and version selection are performed as M0 tasks. |
| M2 backend implementation | M0/M1 implementation evidence demonstrates deployment, update, and diagnosed failure on the selected development cluster. Initial development uses local K3s; Pi acceptance and resource calibration remain required before reference-host support is claimed. The initial OpenAPI contract, schema, and pinned Helm SDK version are recorded before their implementation. |
| M3 administrator interface | M2 provides durable operations, an installed platform workload, versioned secret references, and the initial API. Authentication and private management access are implemented before LAN administration is enabled. |
| M4 lifecycle and core evaluation | M2/M3 are complete; immutable configuration references and the recovery-blocking invariant are available. Numerical evaluation targets and measurement procedures are recorded before evaluation runs. |
| M5–M8 extensions | The core prototype and its minimum evaluation are complete. Each extension requires the specific storage, networking, or distribution decisions identified below. |

## M0 — Constraints and baseline (core)

**Objective:** verification that the reference host can deploy the demonstration
application through Helm, with recorded procedures and resource measurements.

**Risk and completion boundary:** low implementation risk. Hardware,
architecture, or networking incompatibilities must be identified before backend
integration. Demonstration application and chart development form part of this
milestone. Completion requires a successful LAN request and recorded manual
deployment and recovery procedures.

**Tasks:**

- [ ] Host characteristics are recorded, including model, architecture, OS, and free disk after installation of the OS, K3s, and images. **Acceptance:** records are stored in `tests/experiments/`.
- [ ] The demonstration application's port and readiness endpoint are specified. **Acceptance:** its deployment contract is documented in `examples/`.
- [ ] A minimal HTTP demonstration application exposes its version and writes logs to stdout. **Acceptance:** published images include ARM64 and AMD64 variants and healthy v1/v2 and intentionally unhealthy v3 versions.
- [ ] A pinned K3s release is installed manually on the host. **Acceptance:** the node reports Ready and the version and configuration are recorded.
- [ ] A minimal application Helm chart is installed manually. **Acceptance:** the application pod becomes Ready and a LAN request succeeds from another device.
- [ ] Installation, update, failed-update, and recovery commands and outputs are recorded. **Acceptance:** a reproducible transcript is stored in `tests/experiments/`.
- [ ] The minimum image contract specifies the listening address, architecture, and configuration mechanism. **Acceptance:** the contract is recorded in [api-and-data.md](api-and-data.md#application-contract).

**Completion evidence:** successful LAN access, measured idle resource use,
recorded manual steps, and a diagnosed deployment failure with recovery results.

## M1 — Application chart (core)

**Objective:** a versioned application Helm chart that defines the workload,
service, probes, resource policy, ownership labels, and private route.

**Risk and completion boundary:** moderate risk, primarily associated with probe
and resource defaults. Defaults require reference-host measurements. Completion
requires successful installation and update of a healthy image and a diagnosable
failure for an unhealthy image.

**Tasks:**

- [ ] The chart renders a Deployment and Service for the HTTP profile. **Acceptance:** `helm lint` passes and rendered resources pass server-side dry-run validation against the pinned K3s API.
- [ ] Readiness, optional liveness, and startup probes are configurable through chart values. **Acceptance:** the unhealthy image remains unready and diagnostic events identify the failing probe.
- [ ] Resource requests and limits use a measured preset. **Acceptance:** values and measurement conditions are recorded as versioned defaults.
- [ ] Every managed resource carries ownership labels and a stable application ID. **Acceptance:** rendered manifests and installed resource references identify the application consistently, including resources omitted by `kubectl get all`.
- [ ] Private routes explicitly select the permitted ingress entrypoint. **Acceptance:** the route is reachable through the private listener and unavailable through any public listener.
- [ ] Installation and update are verified on K3s. **Acceptance:** an upgrade to v2 replaces the previous pods and serves the expected version.

**Completion evidence:** successful chart validation, installation, and update
on K3s, together with diagnostics from an unhealthy image.

## M2 — Durable backend (core)

**Objective:** an in-cluster Go service with a durable operation queue, persistent
platform state, versioned configuration secrets, Helm integration, and observed
application status.

**Risk and completion boundary:** high risk due to durable state and external
mutation handling. The required [execution and recovery rules](operations.md#deployment-execution-recovery-and-reconciliation)
apply from the first operation. M2 verifies initial deployment, durable acceptance,
and state preservation after restart. Interrupted mutations that cannot yet be
recovered enter `NeedsAttention` and remain blocked. Automated lifecycle recovery
and drift handling are completed in M4.

**Tasks:**

- [ ] The initial `api/openapi.yaml` defines health, system, image inspection, application creation/retrieval/listing, operations, and versioned secret endpoints. **Acceptance:** the specification validates and generated client types, if used, match it.
- [ ] SQLite migrations define administrator, application, revision, operation, audit, and secret metadata. **Acceptance:** supported migration and schema recovery paths are exercised in CI.
- [ ] Request, candidate revision, operation, and audit records are persisted transactionally. **Acceptance:** a failure-injection test verifies all-or-nothing persistence.
- [ ] A minimal platform chart installs the broker in `platform-system` with a Service, metadata PVC, service account, and scoped RBAC for `platform-apps`. **Acceptance:** the broker deploys an application in the same cluster using its service account, without an administrator kubeconfig.
- [ ] Platform replacement permits only one active worker and preserves the SQLite volume. **Acceptance:** pod replacement retains application records, history, and queued work; unresolved interrupted mutations remain durably blocked.
- [ ] The manual platform installation and access procedure is recorded. **Acceptance:** the broker can be reinstalled from the documented chart and configuration; management access before M3 is limited to authenticated loopback forwarding.
- [ ] Application secret creation and versioning produce immutable Kubernetes Secrets with metadata-only database records and chart references. **Acceptance:** revisions retain exact secret versions; referenced versions cannot be deleted, and secret values are absent from SQLite, Helm values, logs, and ordinary responses.
- [ ] Image inspection resolves the digest and architecture before workload mutation. **Acceptance:** an incompatible image produces `IMAGE_ARCH_UNSUPPORTED`.
- [ ] A single worker executes queued operations serially and freezes each execution digest. **Acceptance:** an integration test verifies that a moved tag does not alter a frozen revision.
- [ ] The Helm adapter installs the M1 chart from recorded values. **Acceptance:** the broker creates a real release in `platform-apps` on K3s.
- [ ] The status observer records observation time, desired and ready replicas, and endpoints. **Acceptance:** cluster unavailability produces explicitly stale status.
- [ ] Accepted requests return `202 Accepted` and an operation URL, with lifecycle idempotency enforced. **Acceptance:** client disconnection does not discard work and duplicate requests return the original operation without creating another application.

**Completion evidence:** an in-cluster broker deploys an application, retains
state after pod replacement, preserves secret references, and handles duplicate
requests and disconnected clients without losing accepted work. Installer
automation remains part of M7.

## M3 — Administrator UI (core)

**Objective:** a browser interface for setup, authentication, application
creation and inspection, logs, history, configuration secrets, and private images.

**Risk and completion boundary:** moderate risk, primarily from interface scope
expansion. The initial interface is limited to the workflows defined in
[architecture.md](architecture.md#ui-structure-and-user-experience). Completion
requires authenticated deployment of public and private images. Interface
refinement and Server-Sent Events remain subsequent work.

**Tasks:**

- [ ] The setup wizard consumes an expiring, single-use bootstrap token and creates the administrator account. **Acceptance:** expired tokens and repeated token use are rejected.
- [ ] Session login and logout implement HttpOnly and SameSite cookies, expiry, revocation, and CSRF protection. **Acceptance:** unauthenticated mutations return `401` and CSRF verification passes.
- [ ] Private management access includes documented DNS and certificate trust configuration. **Acceptance:** normal LAN administration uses HTTPS; initial loopback bootstrap remains available through authenticated forwarding.
- [ ] The application list and creation form expose image input, measured defaults, and advanced configuration. **Acceptance:** a submitted application reaches Ready and displays its LAN URL.
- [ ] Configuration input uses the M2 secret endpoints and version references. **Acceptance:** secret values are accepted as write-only input and retained revisions identify the selected versions.
- [ ] Application details include status, bounded logs, events, and revision history. **Acceptance:** the supporting read endpoints are available and logs are rendered as escaped plain text.
- [ ] Registry credential creation and verification support private images. **Acceptance:** a private image deploys successfully and an invalid credential produces an actionable error.
- [ ] Operation progress is restored after refresh or navigation. **Acceptance:** reloading during deployment retrieves the persisted operation state.

**Completion evidence:** authenticated browser deployment of public and private
images, verified access controls, and persistent operation progress.

## M4 — Reliable lifecycle and core evaluation (core)

**Objective:** verified updates, failure recovery, rollback, restart, manual
replica changes, deletion, crash recovery, drift reporting, and evaluation of the
implemented core prototype.

**Risk and completion boundary:** high risk, particularly around interrupted
mutations and failed compensation. Completion requires verified update and
recovery outcomes, a durable block on unresolved recovery, and the
[minimum core evaluation](#minimum-core-evaluation-m4). Application volumes are
excluded from this milestone; their implementation and acceptance tests belong
to M5.

**Tasks:**

- [ ] `PATCH /api/v1/apps/{id}` applies image, configuration, resource, available exposure, and replica changes through one revision and Helm workflow. **Acceptance:** scaling and image changes both create tracked revisions; configuration-only changes preserve the recorded image digest.
- [ ] Failed rollout detection initiates controlled restoration of the previous configuration. **Acceptance:** an unhealthy v3 produces a `Failed` operation only after v2 restoration is verified, with the recovery outcome recorded.
- [ ] Unverified or failed recovery enters nonterminal `NeedsAttention`. **Acceptance:** the failed candidate remains recorded, conflicting mutations return `409` across broker restarts, and the block is cleared only after documented recovery verifies the resulting state.
- [ ] Explicit rollback creates a new revision from an eligible successful revision. **Acceptance:** the selected image digest, chart artifact, and immutable secret versions are restored; external data changes are explicitly outside rollback scope.
- [ ] Restart and manual replica changes support stateless applications, including zero replicas. **Acceptance:** zero replicas produces Stopped; restoration to one and scaling to two produce the expected ready counts. Persistent-profile requests remain unsupported until M5.
- [ ] Stateless application deletion removes owned workload and routing resources while preserving history. **Acceptance:** deletion completes only after those resources are absent; revisions and referenced secret versions remain subject to the documented retention policy.
- [ ] Startup recovery inspects nonterminal operations. **Acceptance:** interruption during an upgrade results in verified recovery or `NeedsAttention`, without blind deletion of Helm release records.
- [ ] Drift reporting and `POST /api/v1/apps/{id}/reapply` use the recorded specification. **Acceptance:** a manually deleted resource is reported and explicit reapplication restores it; operator changes are not overwritten automatically.
- [ ] The required core evaluation is completed against the M0 manual baseline. **Acceptance:** functional and failure outcomes, operator effort, deployment and recovery timings, and resource measurements are stored with raw results and environment metadata.

**Completion evidence:** successful v2 deployment, verified recovery from an
unhealthy v3, preserved secret-version semantics, tested recovery blocking, and
reproducible evaluation results for the core prototype.

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

**Objective:** an installer executable from a checkout, clean-install verification,
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

- [ ] The installer implements preflight checks, K3s setup, namespaces, RBAC, and platform chart installation. **Acceptance:** a fresh host reaches the setup wizard using the installer from a checkout.
- [ ] A clean installation is performed on a rented AMD64 server. **Acceptance:** the platform operates using only the documented installation procedure.
- [ ] Platform upgrade includes backup, compatibility validation, chart upgrade, and failure recovery. **Acceptance:** both a successful upgrade and recovery through rollback with state restoration are exercised.
- [ ] Tagged releases publish ARM64 and AMD64 images and installation assets. **Acceptance:** GHCR contains both image variants with recorded digests and published checksums.
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

## First implementation increment

1. Host details and available disk are recorded; the demonstration port and readiness endpoint are specified.
2. A minimal ARM64/AMD64 HTTP demonstration application provides a version response and stdout logs, with healthy and unhealthy image versions.
3. The pinned K3s installation and minimal application Helm chart establish a manual deployment on the host.
4. Installation, update, failed-update, and recovery commands and results are recorded.
5. The initial API and data model are specified for automation of the verified deployment path.

The initial increment excludes the full monitoring stack, distribution
automation, and expanded dashboard functionality. Its acceptance boundary is the
manual workflow and evidence required by M0/M1.

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
| Library and component versions | At the relevant milestone. | Tested versions are pinned and recorded in a compatibility manifest, including the Helm SDK major before M2. |
| UI stack | Before M3. | React and TypeScript remain the proposed implementation, subject to a recorded design decision. |
| Terraform scope | Before infrastructure evaluation. | Rented VPS and DNS provisioning is the candidate use case; an alternative boundary with the host installer requires a recorded rationale. |

The initial objective is a verified application lifecycle on the reference host,
with durable platform state, observed outcomes, and defined recovery behavior.

## Proposed source layout

The following directories and files are introduced at the relevant milestones.
The tree specifies the intended layout rather than existing implementation.

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
  internal/networking/         exposure validation and mode adapters
  internal/audit/              sanitized activity records
web/
  src/                        UI screens and typed API client
deploy/helm/
  platform/                   broker/UI, RBAC, database volume
  application/                managed application workload chart
installer/                    install, upgrade, diagnostics, recovery
observability/                profile values, dashboards, alert rules
examples/                     healthy/unhealthy demonstration versions
tests/
  integration/                actual Kubernetes/Helm behavior
  e2e/                        browser-driven user workflows
  experiments/                repeatable measurement procedures/results
docs/
  project-outline.md          thesis context and original scope
  app-plan.md                 plan index
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
