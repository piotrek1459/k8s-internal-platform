# Verification guide

All repository checks are selected through `python3 -B tests/run.py` from the
repository root. Test tooling and fixtures are grouped under `tests/`. Go tests
remain beside their packages so that unexported behavior can be tested without
changing production interfaces solely for test placement.

| Location | Responsibility | Execution |
| --- | --- | --- |
| `broker/internal/**/*_test.go` | Domain, SQLite, worker, HTTP/auth, registry, Kubernetes and Helm adapter regressions. | Every PR. |
| `examples/http-demo/*_test.go` | Demo configuration, HTTP and logging regressions. | Every PR. |
| `tests/regression/` | CLI, rollback evidence, transcript, OCI packaging and installer guards. | Every PR. |
| `tests/checks/` | Broker/UI/API/chart checks and OpenAPI helper. | Every PR. |
| `broker/integration/` | Gated store/worker/Helm test using a prepared real cluster. | Explicit local invocation. |
| `tests/integration/` | Owned baseline cluster and direct Helm lifecycle experiment. | Explicit local invocation. |
| `tests/e2e/` | Fresh platform build, API/browser workflows and release-bundle installation. | Nightly, manual and release gate. |
| `tests/experiments/` | Dated curated evidence and historical reports. | Documentation. |

## Fast regression checks on every pull request

Prerequisites: Go matching the module files, Node.js 22/npm, Python 3.10+, Bash,
curl and tar. Docker and a cluster are not required. Helm is installed under
`.local/bin/` using `dev/versions.env`.

```bash
python3 -m venv .local/checks-venv
.local/checks-venv/bin/python -m pip install -r tests/requirements.txt
PATH="$PWD/.local/checks-venv/bin:$PATH" python3 -B tests/run.py unit
```

Individual suites use `--suite broker`, `demo`, `python`, `ui`, `api`, `chart` or
`syntax`; the option may be repeated. For example:

```bash
python3 -B tests/run.py unit --suite python --suite syntax
```

| Workflow | Required checks |
| --- | --- |
| `broker.yml` | Broker/demo formatting, vet and uncached Go regression tests. |
| `api-chart.yml` | Python regression tests, shell syntax, full OpenAPI validation, chart lint/render/negative cases and embedded chart synchronization. |
| `ui.yml` | Dependency installation, Svelte/TypeScript checks, ESLint and production build. |

These workflows trigger on every PR without path filters and on pushes to
`main`. Existing regression coverage includes transaction rollback, immutable
image reuse, stale versions/observations, Secret deletion guards, registry
timeouts and mutation-header enforcement. Python cases cover CLI validation,
registry-specific digest selection, evidence integrity and safe release inputs.
Release-controller regressions check source-branch HEAD selection, version input,
existing tag/release conflicts, normal/prerelease selection, draft retries,
checksum failures and the order of remote publication steps without creating a
release. Progress regressions verify visible heartbeats, private-command silence
and redacted partial output on timeout. UI lint/build is
distinguished from browser acceptance.

## End-to-end workflows

The E2E suite requires a running Linux Docker engine (Docker Desktop on macOS),
k3d matching `dev/versions.env`, kubectl, Node.js 22/npm and Python 3.10+. Host Go
is not required: platform compilation runs in Docker. Native OCI packaging uses
an owned Buildx builder and additional temporary disk space. Linux CI installs
Chromium system dependencies with `--with-deps`.

```bash
python3 -m venv .local/e2e-venv
.local/e2e-venv/bin/python -m pip install -r tests/e2e/requirements.txt
.local/e2e-venv/bin/python -m playwright install chromium
PATH="$PWD/.local/e2e-venv/bin:$PATH" python3 -B tests/run.py e2e \
  --artifacts .local/e2e/run
```

The suite builds the actual UI, broker and demos; installs an isolated K3s
platform; and drives the running UI/API. Each run selects a random cluster name,
loopback ports, administrator password and dedicated kubeconfig. Tests verify
actual Kubernetes resources, Pod identities and application HTTP responses.
Owned forwards, clusters, registry data and the temporary builder are cleaned
in `finally` blocks. Existing installations and the default kubeconfig are not
selected. A forced machine shutdown or SIGKILL can bypass cleanup; artifact logs
identify the owned `platform-e2e-*` resources for inspection.

Coverage includes:

- Invalid/valid browser bootstrap, login, logout, CSRF and consumed-token rejection.
- Invalid and unsupported requests without unintended application creation.
- API deployment, idempotent replay/conflict, update, stale versions, scale 2/0/1,
  rollback, deletion and retained history.
- Browser image inspection, wizard deployment, update, operation refresh, history
  rollback and deletion.
- Immutable configuration Secret versions, write-only responses, deletion guards
  and configuration rollback.
- Missing images, explicit reapply after Service deletion, session/history survival
  and continued application traffic through broker Pod replacement.
- Unhealthy rollout with preserved previous traffic, durable `needs_attention`
  state and rejection of conflicting mutations.
- Native OCI/chart packaging and startup on a second fresh node without a registry.

Two explicit known-gap probes cover status serialization (`STATUS-1`) and
application restart (`RESTART-1`). Only their precise currently documented defect
signatures produce skips; unrelated failures still fail the run. A skip does not
establish acceptance. Once fixed, these probes become ordinary passing tests.
Automatic failed-rollout restoration, interruption during Helm mutation, private
registries, native host bootstrap and public HTTPS remain outside current E2E
acceptance.

`.github/workflows/e2e.yml` runs at **02:00 UTC daily**, through
`workflow_dispatch`, and as a reusable release gate. Fast checks continue on
every PR. Scheduled/manual workflows become available after the workflow file
is merged into the default branch. GitHub scheduling may run later than the
requested time. Release publication waits for E2E success and both architectures.

Long non-private commands print their start, a heartbeat every 30 seconds and
their exit code. Raw command output remains captured in sanitized artifacts;
private commands remain silent. Initial UI/image builds and cluster installation
can therefore take several minutes without streaming compiler output. The
unhealthy-rollout case intentionally waits for Helm's failure timeout. A heartbeat
indicates that the command is still running; command and job timeout limits remain
enforced. On command timeout, partial non-private output is retained and redacted.

Results include JUnit XML, step summaries, sanitized logs and masked browser
screenshots. CI retains them for seven days. Cookies, kubeconfigs, Secret
resources, raw bootstrap output and browser network traces are not uploaded.
Reports distinguish accepted workflows from known gaps; green fast checks alone
do not establish whole-platform correctness.

## Prepared-cluster integration

```bash
PLATFORM_INTEGRATION=1 PLATFORM_DEMO_IMAGE=REGISTRY/IMAGE@sha256:DIGEST \
  KUBECONFIG=PATH python3 -B tests/run.py integration --suite broker
python3 -B tests/run.py integration --suite helm
```

These commands require explicit preparation described in the
[local development guide](../docs/local-development.md). The Helm baseline
compares direct chart behavior with platform behavior; it does not establish
broker crash recovery.

## Historical reports

| Report | Scope |
| --- | --- |
| [2026-10-14 local baseline](experiments/2026-10-14-local-m0-m1.md) | Manual Helm lifecycle, timing and cross-build packaging. |
| [2026-10-26 review verification](experiments/2026-10-26-pr6-review-fixes.md) | Rollback evidence and transcript handling. |
| [2026-10-26 backend snapshot](experiments/2026-10-26-m2-broker-install.md) | M2 implementation boundaries. |
| [2026-10-31 UI verification plan](experiments/2026-10-31-m3-admin-ui.md) | UI snapshot and manual checklist. |
| [2026-11-08 local workflow](experiments/2026-11-08-local-workflow.md) | Healthy lifecycle and stop/rebuild. |
| [2026-11-08 manual release tooling](experiments/2026-11-08-manual-release.md) | Branch HEAD resolution, publication guards, versioned package checks and native-host boundaries. |

These reports preserve dated observations. New thesis measurements require
retained evidence, predefined repetitions and reference-host conditions from the
[evaluation plan](../docs/milestones.md#tests-and-thesis-evaluation).
