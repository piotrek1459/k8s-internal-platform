# Implementation status and next work

Updated: 2026-11-08. Source baseline: merged PR #10 (`e68570c`).
This document records implementation and verification boundaries. The detailed
plans describe target requirements; a planned requirement is not evidence of
completion. Runtime installations and generated files are not part of the source
checkout.

## Current implementation

The local prototype runs the platform and managed applications in one K3s
cluster. A single Go process serves a static Svelte UI and JSON API, executes a
SQLite-backed operation queue, observes Kubernetes workloads, and applies an
embedded first-party chart through the Helm SDK.

| Area | Implemented and recorded evidence | Remaining boundary |
| --- | --- | --- |
| Local workflow | `scripts/platform.sh` builds the UI, broker and demos, creates/reuses a k3d cluster and registry, installs the platform and provides foreground forwards. Fresh installation and stop/rebuild were verified locally. | A production Linux/K3s installer and published releases are not implemented. |
| Application chart | Deployment, Service, probes, resource settings, ownership labels, Secret references and optional private ingress. Local manual lifecycle and route-isolation reports are retained. | Resource defaults are provisional; reference-Pi and second-device LAN acceptance remain outstanding. |
| Durable backend | SQLite migrations, transactional acceptance, idempotency, application version checks, a serial worker, status observer, operation history and audit records. | Interrupted-operation target verification and complete execution snapshots require further work. |
| Authentication | Single-administrator setup, expiring bootstrap token, Argon2id password hashes, hashed sessions, logout and mutation-header checks. | Login throttling, password recovery and normal private HTTPS administration remain outstanding. |
| Administrator UI | Setup/login, application creation, operation polling, update/scale, history, rollback, deletion, Secret and credential screens. | UI/API contract mismatches remain; browser workflows are not covered by an automated acceptance suite. |
| Healthy lifecycle | Local API deployment of v1, update to v2, scaling to 2/0/1, rollback to v1 and deletion with retained history were verified. | Restart, unhealthy-rollout restoration and crash recovery are not accepted as complete. |
| Configuration Secrets | Immutable Kubernetes Secret creation/versioning, metadata-only SQLite records, explicit version references and deletion guards. | UI application-reference submission and real-cluster configuration rollback need verification. |
| Private images | Credential metadata endpoints and image-inspector authentication interfaces exist. | Password persistence, credential resolution and image-pull Secret provisioning are incomplete. |
| Diagnostics | Structured broker logs, operation events and cached workload observations. | Application log/event endpoints, audit browsing, metrics, capacity reporting and retention automation remain planned. |
| Persistence | The platform has a metadata PVC. | Application volumes, retained-volume operations and verified backup/restore belong to M5. |
| Networking | Current development access uses loopback forwarding and Internal application exposure. | LAN HTTPS, public routing/TLS and direct-ingress/tunnel acceptance remain outstanding. |
| Portability | The demo Dockerfile supports ARM64/AMD64 cross-builds; earlier local OCI packaging is recorded. | Native Raspberry Pi/AMD64 acceptance and published multi-architecture platform artifacts remain outstanding. |

The current supported walkthrough uses an anonymous local registry, the stateless
HTTP profile, port 8080 and readiness path `/readyz`. A public image registry and
public application exposure are separate concepts. Replica changes are manual;
several replicas on one host do not provide host-level availability.

## Known correctness and integration gaps

These findings are based on source inspection. Existing passing checks do not
establish acceptance for the affected paths.

1. **UI/API response shapes:** application detail status serializes exported Go
   names such as `Phase`, while the schema and UI expect `phase`. The UI reads
   errors at the top level although the server returns an `error` envelope.
   Secret versions are objects in responses but numbers in the UI type.
2. **Creation form:** LAN uses `private` in the UI instead of backend `lan` and
   lacks the required hostname input. The optional Secret-reference controls are
   not included in the submitted application body. Initial root navigation can
   lead to login before setup; the current guide opens `/setup` explicitly.
3. **Restart:** a new restart revision carries an existing frozen specification.
   The worker returns it before assigning the new restart token, allowing success
   without Pod replacement.
4. **Interrupted recovery:** a Helm status of `deployed` plus a frozen candidate
   does not prove that the candidate was applied. An old deployed release can be
   mistaken for an interrupted target. Target identity and live state must be
   established before recording recovery success.
5. **Failed mutations:** Helm failures enter `needs_attention` and retain the
   application lock. Automatic verified restoration and an administrator recovery
   operation are not implemented. A new rollback cannot bypass the lock.
6. **Private credentials:** the current handler accepts a password but stores
   only metadata; the running inspectors have no credential resolver. The chart
   can refer to a pull Secret that the credential workflow has not provisioned.
7. **Execution snapshots:** chart version, chart artifact digest and values hash
   have model fields but are not populated by the worker. Rollback across chart
   upgrades therefore lacks the complete artifact guarantee in the design.
8. **Probe mapping:** the broker currently reuses a supplied readiness path for
   startup. The chart can express separate probes, but that distinction is not
   exposed correctly by the application API mapping. Unhealthy-rollout acceptance
   must verify startup and readiness independently.

## Verification available

- The ordinary broker suite has 97 top-level test functions using temporary
  SQLite, fake Kubernetes clients, in-memory Helm storage and an in-process
  registry. One additional real-cluster test is gated by the `integration` build
  tag and `PLATFORM_INTEGRATION=1`.
- The demo has five top-level Go tests. Fifteen Python test methods cover
  rollback evidence, transcript completion, command validation and digest
  selection. Table-driven subcases are additional to these counts.
- Every pull request runs broker/demo Go regression suites, UI type-check/lint/
  build, structural OpenAPI validation, shell/Python checks, and the full local
  chart lint/render/negative/synchronization matrix. Go tests use `-count=1`;
  local and CI chart checks share the Helm version pin. The default CI does not
  run the real-cluster test or an automated browser suite. Suite locations and
  workflow mapping are in the [test guide](../tests/README.md#regression-tests-on-every-pull-request).
- The [local lifecycle report](../tests/experiments/2026-11-08-local-workflow.md)
  records the latest full healthy API lifecycle and stop/rebuild test. Earlier
  [reports](../tests/README.md#historical-reports) cover the manual chart baseline
  and implementation snapshots. A procedure or unit test is distinguished from
  a real-cluster or browser acceptance result.

The earlier local clusters, registry data, review worktrees and ignored raw test
output were removed during cleanup. Curated tracked reports remain historical
records; their original raw paths are no longer available locally. New thesis
measurements require new retained evidence.

## Next implementation order

| Order | Scope | Acceptance |
| --- | --- | --- |
| 1 | Align API schemas, response serialization, UI errors, setup navigation, exposure fields and Secret submission. | Handler contract tests and browser workflows use the actual request/response shapes; unsupported paths are identified clearly. |
| 2 | Correct restart semantics. | Pod identity changes, the image digest remains fixed, and retries retain the same restart token. |
| 3 | Verify interrupted recovery and failed-rollout restoration; complete execution snapshots. | Interruption before/during/after Helm execution never produces false success. Restored state is verified before a lock is released; unresolved state remains blocked. |
| 4 | Complete private registry authentication and pull credentials. | A private image deploys; invalid credentials produce an actionable error without leaking values. |
| 5 | Automate real-cluster acceptance and perform reference-host evaluation. | Isolated runs verify resources and HTTP responses, clean their own resources, and retain evidence. Pi/LAN measurements use predefined targets and repetitions. |
| 6 | Implement M5–M8 capabilities. | Application data/backup, public HTTPS, installable releases and monitoring satisfy their separate acceptance criteria. |

The [milestone checklists](milestones.md) retain the full project scope. Local
implementation progress does not establish completion of reference-hardware,
public-hosting, release, or thesis-evaluation requirements.
