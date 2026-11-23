# Verification guide

Fast, integration and disposable E2E checks are selected through
`python3 -B tests/run.py` from the repository root. Native release acceptance is
an explicit opt-in command documented below. Test tooling and fixtures are grouped under `tests/`. Go tests
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
- Unhealthy rollout with verified restoration of the prior snapshot before
  mutation unlock, plus preserved previous application traffic.
- Native OCI/chart packaging and startup on a second fresh node without a registry.
- An observed pending first install interrupted with runtime SIGKILL on the owned
  k3d node: unchanged Helm evidence, durable lock/409 across broker replacement,
  independent queued browser work without resubmission, and tracked first-deployment
  removal with retained history. This is one interruption barrier; the remaining
  crash/restoration matrix is tracked separately.

Status serialization and application restart are required assertions; no known-gap
skips remain. The runner and CI summary require a non-empty suite with zero
failures and zero skips; expected-failure waivers also fail acceptance. Missing
or inconsistent JUnit evidence fails the summary. Its source commit identifies
what was actually tested, so a historical summary cannot establish current behavior. Authenticated registry inspection and real kubelet pulls use only
the suite's owned registry and test credentials. Pending/unknown recovery and
failed restoration retain durable locks in focused regression tests. Public HTTPS
and native AMD64 installation remain separate acceptance boundaries.

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

## Native release acceptance

This is an explicit, stateful test against a dedicated installed ARM64 release;
it is not part of scheduled disposable CI. Management and Kubernetes proxy URLs
must use loopback SSH forwards. On the Pi, start the documented management
port-forward and an additional temporary root-readable Kubernetes proxy:

```bash
pkubectl proxy --address=127.0.0.1 --port=18001
```

Forward that proxy over SSH as well. The proxy grants Kubernetes administrative
access: keep both listeners on loopback and stop the temporary proxy after testing.
Download the release's `release.json`, then run from the checkout with the pinned
E2E dependencies and Chromium installed:

```bash
python3 tests/e2e/native.py --url http://127.0.0.1:8080 \
  --kubernetes-url http://127.0.0.1:18001 --version v0.1.3 \
  --release-manifest .local/pi/release.json --state .local/pi/access.json \
  --artifacts .local/pi/acceptance --setup
```

`--setup` explicitly authorizes first administrator creation on this test
installation. An existing account without the matching private state file is
refused. The state file has mode `0600` and contains the generated administrator
password and owned retained application ID; never commit or publish it. Reports
contain check descriptions and release identity, not credentials. The command
retains one healthy owned application for same-version reinstall and reboot checks.
For unprivileged Nginx compatibility, add
`--public-image docker.io/nginxinc/nginx-unprivileged:stable-alpine`. This optional
browser check uses port `8080`, readiness `/`, validates the temporary volume and
container hardening, checks actual HTTP, and removes only its own test application.
It requires reachable public-registry inspection/pulling.
After reboot, restart the forwards and run the same command with `--resume`
instead of `--setup`. Cleanup of an installation requires separate explicit
operator action; the test never uninstalls K3s or deletes unrelated applications.

The [v0.1.3 report](experiments/2026-11-23-core-acceptance.md) records the native
lifecycle, unpatched public-image deployment, reinstall/reboot and direct Helm
baseline. The [earlier Pi report](experiments/2026-11-22-raspberry-pi.md) retains
its original version-specific observations.

The native deletion check allows up to 60 seconds for Pod garbage collection
after the tracked delete operation completes. Deployment deletion and dependent
Pod deletion are asynchronous. A remaining Pod after that deadline fails the check.

## Core milestone audit

The [M0–M4 checklists](../docs/milestones.md) distinguish completed implementation
from remaining acceptance and missing functionality. Outstanding core work is
tracked in [issue #16](https://github.com/piotrek1459/k8s-internal-platform/issues/16).
The old standalone nightly run on commit `076d1fc` predates PR #14 and its two
conditional skips. Historical reports retain their original results; current
status/restart tests are mandatory and the updated policy rejects any skipped
E2E workflow. Secret-reading test commands are private; known secret plaintext
and base64-encoded values are redacted from retained diagnostics.
