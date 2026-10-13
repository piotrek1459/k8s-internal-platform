# Milestones, workflows, tests, and open decisions

Part of the [application platform plan](app-plan.md). This is the **execution
document**: the ordered milestones with per-milestone task checklists and risk
notes, the end-to-end workflows they must satisfy, the evaluation plan for the
thesis, the still-open decisions, and the intended source layout. For design
detail, see [architecture.md](architecture.md),
[api-and-data.md](api-and-data.md),
[networking-and-security.md](networking-and-security.md), and
[operations.md](operations.md).

> **Scope banner.** Full scope (M0–M8) remains the target. **M0–M4 is the
> thesis-sufficient core**: it demonstrates a trustworthy application lifecycle on
> a real host and is enough to write and defend the thesis. **M5–M8 are stretch**:
> persistence, public access, distribution, and the full evaluation profile. Each
> milestone below carries a *Risk / cut-point* note marking the safe place to stop
> if time runs short.

Complete one working workflow at a time. Capture evidence while implementing.
Installer work begins with a documented manual procedure in M0 and is automated
incrementally. Basic tests and logs are part of M1–M4, not postponed to M8.
Each checklist item names a code-level **done:** criterion so completion is
checkable, not subjective.

## M0 — Constraints and baseline (core)

**Goal:** prove the target host can deploy a demo via Helm, and record the
baseline manual procedure and resource costs.

**Risk / cut-point:** low risk; this is mostly measurement and manual commands.
The main danger is skipping it and discovering host/arch problems later. Do not
proceed to code before a LAN request to the demo succeeds. Safe stop: a working
manual Helm deploy on the host with recorded steps.

**Tasks:**
- [ ] Record host details (model, arch, OS, free disk after OS+K3s+images) — done: written into `tests/experiments/`.
- [ ] Decide demo app port and readiness endpoint — done: documented in `examples/`.
- [ ] Build a tiny ARM64 (and AMD64) HTTP demo returning a version string, logging to stdout — done: image pushed to a registry, both arches present in manifest.
- [ ] Manually install pinned K3s on the host — done: `kubectl get nodes` shows Ready; K3s version recorded.
- [ ] Write the smallest application Helm chart and `helm install` the demo manually — done: pod Ready, LAN request succeeds from another device.
- [ ] Record exact install / update / failed-update / recovery commands and outputs — done: reproducible transcript in `tests/experiments/`.
- [ ] Define the minimal image contract (listen on 0.0.0.0, arch, config via env) — done: captured in [api-and-data.md](api-and-data.md#application-contract).

**Completion evidence:** LAN request succeeds; idle resources and manual steps recorded; a failure example is understood.

## M1 — Application chart (core)

**Goal:** a versioned, first-party application Helm chart producing workload,
service, probes, resource policy, ownership labels, and a private route.

**Risk / cut-point:** moderate. Probe and resource defaults must be measured, not
guessed. Safe stop: a chart that installs/updates a healthy image and produces a
diagnosable failure for an unhealthy one.

**Tasks:**
- [ ] Chart renders Deployment + Service for the HTTP profile — done: `helm template` output validates with `kubeval`/`helm lint`.
- [ ] Readiness/liveness/startup probes wired from values — done: unhealthy image stays NotReady and is visible in `kubectl describe`.
- [ ] Resource requests/limits preset applied; measured on the host — done: values recorded as versioned defaults, not placeholders.
- [ ] Ownership labels + stable app ID on every resource — done: `kubectl get all -l app-id=<id>` returns only that app's resources.
- [ ] Private route generated with explicit entrypoint selection — done: route reachable on the private listener, absent on any public one.
- [ ] Real install + update of the demo on K3s — done: `helm upgrade` to v2 succeeds and old pods are replaced.

**Completion evidence:** template checks pass and a real install/update works on K3s; an unhealthy image yields a diagnosable failure.

## M2 — Durable backend (core)

**Goal:** the Go service with a durable operation queue that can deploy an app
through Helm, survive restarts, and observe real status.

**Risk / cut-point:** **highest-risk core milestone.** The durable-queue +
crash-recovery design (see [execution and recovery](operations.md#deployment-execution-recovery-and-reconciliation))
is where subtle bugs live. Timeline danger: over-engineering reconciliation.
Safe stop: deploy + status + operation-survives-disconnect working for the HTTP
profile; defer rollback/drift to M4. Cut here if needed and still have a
demonstrable platform.

**Tasks:**
- [ ] `api/openapi.yaml` for health, system, images/inspect, apps (create/get/list), operations — done: spec validates; frontend types generated if used.
- [ ] SQLite migrations for administrator/application/revision/operation/audit — done: migrate up and down tested in CI.
- [ ] Transactional persist of request + candidate revision + operation + audit — done: a unit test proves all-or-nothing on failure.
- [ ] Image inspection resolves digest + architecture before mutation — done: wrong-arch image rejected with `IMAGE_ARCH_UNSUPPORTED`.
- [ ] Single worker executes queued ops serially; freezes execution digest — done: integration test shows a moved tag does not change a frozen revision.
- [ ] Helm adapter installs/upgrades the M1 chart from recorded values — done: real release created in `platform-apps` on K3s.
- [ ] Status observer records `observedAt`, ready vs desired replicas, endpoints — done: stale status surfaced when the API is unreachable.
- [ ] `202 Accepted` + operation URL returned before execution; `Idempotency-Key` honored — done: duplicate request returns the original operation, not a second app.

**Completion evidence:** the API deploys an app; an operation survives client disconnection; a duplicate request does not create another app.

## M3 — Administrator UI (core)

**Goal:** a browser UI for setup/login, listing, creating, detail, logs, and
history, including image inspection and the private registry flow.

**Risk / cut-point:** moderate; UI scope creep is the danger. Build only the
screens in [architecture.md](architecture.md#ui-structure-and-user-experience).
Safe stop: deploy a public and a private image from the browser with working
auth; defer polish and SSE.

**Tasks:**
- [ ] Setup wizard consumes the single-use bootstrap token and creates the admin — done: token is single-use and expires; second use rejected.
- [ ] Session login/logout with HttpOnly+SameSite cookies and CSRF defense — done: unauthenticated mutation returns 401; CSRF test passes.
- [ ] Applications list + create form (image-first, defaults, collapsed advanced) — done: a deploy from the form reaches Ready and shows a URL.
- [ ] Application detail: status, logs, events, revision history — done: logs render as escaped plain text with bounded history.
- [ ] Registry credential add/verify and private-image deploy — done: a private image deploys; an invalid credential shows a clear error.
- [ ] Durable operation progress survives refresh/navigation — done: reloading mid-deploy re-fetches state from the API.

**Completion evidence:** an administrator deploys public and private images from the browser; access controls are exercised.

## M4 — Reliable lifecycle (core)

**Goal:** update, failure recovery, rollback, restart, manual replicas, deletion,
crash recovery, and drift reporting.

**Risk / cut-point:** high; this is where correctness is proven. Timeline danger:
chasing every ambiguous-recovery edge case. Safe stop (and **end of thesis-core**):
good v2 updates, bad v3 recovers verifiably, interrupted work is recovered or
clearly blocked. **This is the recommended minimum viable thesis.**

**Tasks:**
- [ ] `PATCH /apps/{id}` drives image/config/resources/exposure/replica changes through one revision+Helm path — done: scaling and image change both create tracked revisions.
- [ ] Failed-rollout detection + controlled restore of previous config — done: bad v3 leaves operation Failed with recovery outcome "previous version restored".
- [ ] Explicit rollback to a selected successful revision — done: rollback creates a new revision reusing the old digest; data untouched (and documented as such).
- [ ] Restart and manual replicas incl. zero (=Stopped) — done: zero shows Stopped; persistent profile rejects >1 with a clear error.
- [ ] Delete app retains data by default — done: workload/routes gone, volume + history retained and visible.
- [ ] Crash recovery inspects nonterminal operations on startup — done: broker killed mid-upgrade recovers or enters Needs-attention; no blind Helm-lock deletion.
- [ ] Drift reporting + `POST /apps/{id}/reapply` — done: a manually deleted resource is reported, and reapply restores it without clobbering operator edits silently.

**Completion evidence:** good v2 works; bad v3 gives a verified recovery outcome; interrupted work is recovered or clearly blocked.

## M5 — Persistence (stretch)

**Goal:** the single-volume persistent profile, retained volumes, versioned
secrets, and platform/data backup and restore.

**Risk / cut-point:** moderate–high; data-loss risk makes testing essential.
Safe stop: data survives app replacement/reboot and a restore succeeds once.
May reorder with M6 once the author's app is designed (its persistence needs
decide the profile).

**Tasks:**
- [ ] Single PVC + mount path profile, PVC kept outside the workload release — done: deleting the app retains the PVC.
- [ ] `storage.existingVolumeId` reuse with ownership/class/mount validation — done: attaching a volume already in use is rejected.
- [ ] Versioned, immutable secrets referenced by revision — done: a rollback references the correct secret version, not plaintext in Helm values.
- [ ] Separate, confirmed, in-use-checked volume deletion — done: deleting an attached volume is refused.
- [ ] Platform SQLite backup via backup API (not live file copy) + restore — done: restore to replacement storage reproduces state.
- [ ] Documented three-part backup (platform / K3s datastore / app data) with measured recovery time — done: a full restore rehearsal is recorded in `tests/experiments/`.

**Completion evidence:** data survives app replacement/reboot; app deletion preserves it; the restore procedure succeeds.

## M6 — Public application (stretch)

**Goal:** choose one public ingress mode, configure domain/TLS, and publish the
author's separate app over HTTPS.

**Risk / cut-point:** high and partly outside your control (ISP/CGNAT, DNS, TLS).
This is where the **rented server pays off**: a VPS with a public IP makes direct
ingress straightforward, whereas a home Pi behind CGNAT likely needs a tunnel —
see [public mode decision](networking-and-security.md#public-mode-decision).
Timeline danger: fighting a home network. Safe stop: one mode verified from an
outside network with private/management routes proven unreachable.

**Tasks:**
- [ ] Decide host + mode: rented-server direct ingress, or Pi tunnel, based on the actual network — done: decision and rationale recorded.
- [ ] Public listener is separate from private and firewalled-off until configured — done: forged private Host/SNI to the public listener is rejected (tested).
- [ ] cert-manager (direct ingress) tested against ACME staging first, then production — done: valid certificate issued; renewal path documented.
- [ ] Publish the author's app and route it on the public hostname — done: HTTPS works from an outside network (verified with an external client, not the LAN).
- [ ] Management + private apps confirmed unreachable via the public endpoint — done: external request to a management route fails.

**Completion evidence:** HTTPS works from outside; private-app/management route rejection is tested.

## M7 — Repeatable distribution (stretch)

**Goal:** installer runnable from a checkout, a clean-install rehearsal, platform
upgrade and recovery, ARM64/AMD64 artifacts, and GitHub release packaging. Also
introduces the scoped-token endpoints and a sample CI deployment through an
approved private-network path.

**Risk / cut-point:** moderate; mostly engineering discipline. Timeline danger:
gold-plating the installer framework. Safe stop: a clean supported host installs
from the documented procedure alone. **Do the clean-install rehearsal on a rented
AMD64 server** — it validates portability beyond the Pi and gives a real
external-reachability test for M6.

**Tasks:**
- [ ] Installer runs the full preflight → K3s → namespaces/RBAC → chart flow from a checkout — done: a fresh host reaches a working setup wizard.
- [ ] Clean-install rehearsal on a rented AMD64 server — done: platform runs on the VPS using only the documented steps.
- [ ] Platform upgrade (backup → validate → upgrade chart) and failure recovery — done: an upgrade and a rollback-with-restore are both exercised.
- [ ] ARM64 + AMD64 images and installer assets published for a tagged release — done: GHCR has both arches with recorded digests; checksums published.
- [ ] Scoped machine-token endpoints (`/tokens`) stored hashed and revocable — done: a token limited to one app cannot touch another.
- [ ] Sample CI deploy calls the API over a private path and polls the operation — done: a registry push + API call deploys without opening admin to the Internet.

**Completion evidence:** a clean supported host can be installed using only the documented procedure.

## M8 — Evaluation and observability (stretch)

**Goal:** add and tune the full observability profile; collect functional,
performance, and failure results; final usability pass.

**Risk / cut-point:** low technical risk, high time cost. Measure, don't polish.
Safe stop: reproducible results tied to commits/hardware/versions for the
scenarios you actually implemented.

**Tasks:**
- [ ] Full profile (Prometheus/Grafana/Loki/Alloy) with bounded retention, overhead measured — done: added CPU/RAM/disk recorded with and without it.
- [ ] Core dashboards (deploy outcome/duration, API errors/latency, restarts, ready replicas, disk) — done: dashboards render against real data.
- [ ] Alerts for repeated failed deployments and sustained resource pressure — done: an induced failure fires the alert.
- [ ] Functional/performance/failure results collected per the [evaluation plan](#tests-and-thesis-evaluation) — done: raw results stored with environment metadata.
- [ ] Platform-vs-manual-baseline comparison measured — done: step count, hands-on time, and timings recorded under a stated step definition.

**Completion evidence:** reproducible results tied to commits, hardware, versions, and configuration.

## First concrete development session

1. Record host details and free disk; decide the demo port and readiness endpoint.
2. Build a tiny ARM64/AMD64-capable HTTP demo with a version response and stdout logs.
3. Create the smallest application Helm chart and deploy it manually on the host.
4. Record the exact install, update, failed update, and recovery commands/results.
5. Define only the API/model needed to automate this proven path.

Avoid spending the first week on a large dashboard, a general installer framework,
or the full monitoring stack before proving that the target host can deploy the demo.

## Example end-to-end workflows

These are the scenarios the milestones must satisfy; they double as manual test scripts.

### A. First LAN deployment

1. Install on the host, create the admin account, and complete private access/DNS setup.
2. Build a small demonstration app on the development machine; publish its image.
3. Paste its reference in the UI, confirm port and readiness endpoint, and deploy.
4. Watch durable progress; open the resulting LAN URL from another device.
5. Open logs and history, restart the broker, and confirm the app keeps serving.

### B. Private registry deployment

1. Add a narrowly scoped registry pull credential in the UI.
2. Select it when entering a private image reference.
3. Validate architecture and permissions; show a clear error for invalid credentials.
4. Deploy using a referenced image-pull Secret. Confirm secrets do not appear in
   history, normal API responses, or rendered Helm values.
5. Rotate credentials and verify a subsequent image pull before retiring the old version.

### C. Update and failure recovery

1. Publish version 2 of the demonstration app.
2. Choose **Update**, inspect the change summary, and follow readiness progress.
3. Confirm that history records the old/new digests and the successful result.
4. Publish a deliberately unhealthy version 3 and attempt an update.
5. Confirm an intelligible failure, preserved diagnostics, and the recorded outcome
   of restoring version 2. Reboot/restart recovery is tested separately.

### D. Scale a suitable application

1. Set a stateless demonstration app to two instances.
2. Validate the resource policy; apply through the same revision/Helm flow.
3. Show two ready instances and observe requests reaching them.
4. Set zero instances and display **Stopped**; restore one instance afterwards.
5. Attempt two instances for the persistent profile and verify a clear rejection.

### E. Publish the author's separate application

1. Specify that app's port, architecture, configuration, data/dependency needs,
   health checks, and migration procedure.
2. Publish its image to the chosen registry and prove the LAN workflow first.
3. Buy/configure the domain and choose direct ingress or a named tunnel based on
   the actual host (rented server favours direct ingress). Configure this once at
   the platform level.
4. Select **Public**, choose an allowed hostname, and apply the tracked change.
5. Verify HTTPS from an outside network. Confirm administration and private apps
   remain unreachable through the public endpoint.
6. Exercise an update and recovery scenario with that app, then document backup
   restoration if it stores persistent data.

### F. Delete an application safely

1. Show the app and its attached storage before accepting deletion.
2. Remove external routing and workload resources, then mark the app deleted.
3. Keep its history and retained volume visible according to retention policy.
4. Allow a later explicit volume deletion only after validating it is unused.

## Tests and thesis evaluation

| Area | Cases to cover |
| --- | --- |
| Shared cluster architecture | Platform and demo app run in the same cluster/on the reference node; app traffic still succeeds while the broker is stopped; host-side recovery restores management; workloads cannot reach protected management endpoints. |
| Validation | Invalid image/name/port, unsupported architecture, missing secret/credential, forbidden exposure mode. |
| Deployment | Public/private registry pulls, wrong credentials, nonexistent tag, registry unavailable, app binding to localhost. |
| Lifecycle | Valid update, failed readiness, recovery failure, rollback, restart, scale 0/1/2, persistent scaling rejection. |
| Durable state | Browser disconnect, duplicate request, stale app version, broker restart before/after Helm change, ambiguous pending release. |
| Capacity | Insufficient memory, unschedulable replicas, large image pull, disk pressure, monitoring retention limits. |
| Persistence | App restart, host reboot, app deletion with retained data, separate volume deletion, backup restore. |
| Networking | LAN DNS failure, invalid public DNS/TLS, selected public access path, private Host/SNI isolation, IPv6 policy. |
| Authentication | Setup token expiry/reuse, login/logout, session expiry, unauthenticated mutations, CSRF, secret non-disclosure. |
| Distribution | Repeat installer, partial failure, clean install (on the rented server), supported upgrade, documented recovery. |
| Usability | Deploy/update/recover using UI instructions only; clear actions for image/port/health failures. |

Use unit tests for domain policies and operation transitions; integration tests
must exercise actual Helm/Kubernetes behavior. Browser tests should cover the
few important administrator journeys. Rehearse on disposable local clusters
where useful, but retain host/K3s runs as evidence for the advertised targets.

Compare the platform with the same manual Helm deployment baseline. Measure:

- Administrator steps and hands-on time, under a stated definition of a step.
- Deployment/update time, separating cold image pulls from cached runs.
- Failure detection and verified recovery time.
- Idle and loaded CPU/RAM/disk use, with and without the full observability profile.
- Request success/latency under a fixed workload and supported replica counts.
- Reproducibility across repeated deployments and one clean installation.

Choose numerical targets after M0 measurements, before the final evaluation.
Record commit, component versions, image digests, hardware, storage, network,
cache state, repetitions, and raw results. Run load generation from another
machine where possible. Do not claim high availability from replicas on one host
or general performance conclusions from a single convenient run.

## Decisions still open

These do not block the first LAN deployment:

| Decision | When needed | Recommended next action |
| --- | --- | --- |
| Future app database/uploads/jobs | Before its persistence design. | Write its deployment contract when the app idea is chosen. |
| Direct public ingress or tunnel | M6. | Pick based on host: rented server → direct ingress; home Pi → check CGNAT/router before choosing. |
| Primary production host | Before M6/M7. | Pi is reference/starter; decide whether the rented server becomes the long-term home before investing in its public setup. |
| Domain and DNS provider | M6. | Choose after the public access mode; no purchase needed for M0–M4. |
| LAN resolver and private certificate trust | M0 developer setup, polished in M3/M7. | Use explicit host mapping first; document a repeatable private DNS/TLS setup. |
| SSD upgrade | Before sustained Pi use with valuable data. | Measure the microSD installation, retain backups, then decide; a rented server with SSD avoids this. |
| Exact library/component versions | At each implementation milestone. | Pin tested versions and maintain a compatibility manifest; avoid floating latest dependencies. |
| UI stack | Before M3. | Use the proposed React/TypeScript client unless there is a preferred alternative. |
| Terraform's concrete role | Before final infrastructure evaluation. | **Candidate: provision the rented VPS + DNS** as the reproducible infrastructure use case, or record why the host installer is the appropriate mechanism instead. |

The immediate focus is a trustworthy application lifecycle on the actual host:
deploy, observe, update, recover, and preserve the administrator's state and data.

## Proposed source layout

Create these files/directories when the relevant milestone starts; the following
tree describes the intended layout, not code already present in the repository.

```text
api/
  openapi.yaml                 public management API contract
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
examples/                     healthy/unhealthy demo versions
tests/
  integration/                actual Kubernetes/Helm behavior
  e2e/                        browser-driven user workflows
  experiments/                repeatable measurement procedures/results
docs/
  project-outline.md          thesis context and original scope
  app-plan.md                 plan index
  architecture.md             system shape, components, cluster layout, UI
  api-and-data.md             application contract, data model, API
  networking-and-security.md  visibility modes, isolation, secrets, backup
  operations.md               install, execution/recovery, observability
  milestones.md               this execution document
  decisions/                  small records of important implementation choices
infra/terraform/              optional reproducible VM/cloud/network setup
book/                         thesis source
```

Terraform does not need to install software on an already prepared host. Use a
repeatable installer for that. Its concrete candidate use case is provisioning a
reproducible rented VPS and its DNS records (see the open-decisions table);
document that choice rather than forcing it into every install.
