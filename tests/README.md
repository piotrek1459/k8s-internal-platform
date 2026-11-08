# Tests and experiments

## Automated checks

| Entry point | Coverage | External runtime |
| --- | --- | --- |
| `scripts/check-broker.sh` | Broker formatting, vet and ordinary Go tests: validation, auth, store, handlers, worker, adapters and recovery classifications. | Temporary SQLite and fake/in-process dependencies; no cluster. |
| `scripts/check-ui.sh` | Svelte/TypeScript checks, ESLint and production build. | Node/npm; no browser acceptance. |
| `scripts/check-api.sh` | YAML structure and local references; optional full OpenAPI validation. | Python with PyYAML; optional full validator. |
| `scripts/check-chart.sh` | Both charts, positive/negative values and embedded copy synchronization. | Pinned local Helm CLI; no cluster. |
| Demo Go tests | Configuration, version/health responses and sanitized request logs. | Go; no cluster. |
| Python helper tests | Rollback evidence, transcript completion, CLI validation and registry-specific digest selection. | Python and Bash; no cluster or Docker daemon. |

The Python checks run from the repository root:

```bash
python3 -B -m unittest discover -s tests -p 'test_*.py' -v
```

[test_local_helpers.py](test_local_helpers.py) contains nine test methods;
[test_platform_cli.py](test_platform_cli.py) contains six. Table-driven cases are
additional. Broker tests are alongside their packages; demo tests are under
[examples/http-demo](../examples/http-demo/).

## Regression tests on every pull request

Regression tests are part of the ordinary suites rather than a separate test
framework or directory. They preserve expected behavior and previously fixed
failure cases. The workflows trigger on every `pull_request` without path or
branch filters, and on pushes to `main`.

| Suite and location | Example protected behavior | PR workflow |
| --- | --- | --- |
| `broker/internal/**/*_test.go` | Frozen image reuse, transaction rollback, secret deletion guards, stale observations and registry timeouts. | `broker.yml`, broker job. |
| `examples/http-demo/*_test.go` | Health/readiness separation, configuration validation and request-log sanitization. | `broker.yml`, demo job. |
| `tests/test_local_helpers.py` | Incomplete rollback evidence is rejected; transcript/logging failures preserve exit status. | `api-chart.yml`, Python regression step. |
| `tests/test_platform_cli.py` | Invalid CLI input is rejected and the selected registry's digest is used. | `api-chart.yml`, Python regression step. |
| `scripts/check-chart.sh` | Invalid values/probe configurations are rejected; both charts render and the embedded chart stays synchronized. | `api-chart.yml`, chart job. |

Both Go jobs use `go test -count=1 ./...`, ensuring tests execute instead of
reusing cached results. Compiled broker dependencies can still be cached. The
chart job installs Helm through `platform.sh tools`, using the same version pin
as local checks. UI type-check/lint/build also runs on every PR, but is not a
browser regression suite. Known gaps listed in [status](../docs/status.md) still
require new tests and fixes; executing existing suites does not cover them all.

## Real-cluster verification

- `broker/integration/` contains one test behind the `integration` build tag and
  `PLATFORM_INTEGRATION=1`. It deploys through the store/worker/Helm path and
  reopens the database after completed work. It is not a crash-injection test.
- [scripts/test-local.sh](../scripts/test-local.sh) exercises the separate manual
  Helm baseline, including failed readiness, explicit rollback and route isolation.
- The 2026-11-08 healthy API lifecycle was executed against an isolated cluster;
  its driver was temporary local tooling, not a committed automated suite.

Execution procedures are in the [local development guide](../docs/local-development.md).
GitHub Actions runs broker/demo Go suites, UI checks, structural API validation,
shell/Python regressions and the full local chart matrix. Browser and real K3s
acceptance remain separate from the default PR workflows.

## Historical reports

| Report | Scope |
| --- | --- |
| [2026-10-14 local baseline](experiments/2026-10-14-local-m0-m1.md) | Manual Helm lifecycle, timing snapshots and cross-build packaging. |
| [2026-10-26 review verification](experiments/2026-10-26-pr6-review-fixes.md) | Stronger rollback assertions and transcript handling. |
| [2026-10-26 backend snapshot](experiments/2026-10-26-m2-broker-install.md) | M2 implementation and installation/test procedure boundaries. |
| [2026-10-31 UI verification plan](experiments/2026-10-31-m3-admin-ui.md) | UI implementation snapshot and manual acceptance checklist. |
| [2026-11-08 local workflow](experiments/2026-11-08-local-workflow.md) | Fresh setup, healthy API lifecycle and preserved state through stop/rebuild. |

The reports describe dated observations or procedures, not blanket milestone
acceptance. Earlier ignored raw output and temporary test environments were
removed during cleanup. New baseline runs create evidence under
`experiments/runs/`; source code and curated reports remain tracked.

The [current status](../docs/status.md) records known gaps. The
[evaluation plan](../docs/milestones.md#tests-and-thesis-evaluation) defines the
remaining functional, failure, hardware and measurement requirements.
