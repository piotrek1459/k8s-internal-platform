# Manual release tooling verification

Date: 2026-11-08.

This report records validation of manual publication from a selected branch HEAD
and distinguishes containerized package checks from native host acceptance.

## Workflow

The Release workflow has only `workflow_dispatch`, with `branch` (default `main`)
and `version` inputs. Preparation resolves the specified remote branch HEAD once.
Validation, E2E, builds and publication use that SHA. Successful gates permit tag
creation and preview publication. PR, branch and tag pushes do not publish releases.

The full publication workflow has not been dispatched. Manual publication is an
administrator action after the workflow is available on the default branch.

## Verification performed

- All 45 Python regression tests, shell syntax checks and actionlint v1.7.12
  passed. Actionlint ran with external shellcheck/pyflakes disabled.
- Release-controller regressions cover branch HEAD selection, input validation,
  published-version/tag conflicts, draft retries, checksum failures, incomplete
  assets and publication ordering without remote writes.
- A read-only preparation run with the official checksum-verified GitHub CLI
  resolved the actual `main` HEAD and authenticated release API responses. It
  created no tags or releases.
- A local package run supplied version `0.0.2` while the development fallback
  remained `0.0.1`. It built and imported native ARM64 OCI archives, started the
  packaged broker on a fresh K3s node without a registry, verified version and
  architecture, deployed/updated a demo, checked its HTTP response and deleted it.
- Installer/guide version rendering, native OCI identity and final `SHA256SUMS`
  assembly have focused regressions. Owned test resources were cleaned up.

## Acceptance boundaries

- `STATUS-1`: workload status uses PascalCase while the contract/UI use camelCase.
  Its precise known signature produces an explicit E2E skip.
- `RESTART-1`: an already-frozen revision can miss the new restart token and finish
  without Pod replacement. Its precise known signature produces an E2E skip.
- Failed-rollout restoration, interrupted-target verification, complete chart
  snapshots, private-registry credentials, UI Secret/exposure integration,
  application volumes and public HTTPS remain separate work.
- The native Linux/systemd installer has not been executed on the reference Pi.
  Native boot, storage/cgroup compatibility, reboot persistence, LAN access and
  rented-server acceptance remain pending. Containerized checks do not establish
  those results.

E2E is configured for 02:00 UTC daily, manual execution and release validation.
Scheduled/manual execution requires the workflow on the default branch. CI
artifacts are sanitized and retained for seven days. Thesis measurements require
the predefined hardware, repetitions and conditions in the evaluation plan.
