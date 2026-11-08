# v0.0.1 preview release verification

Date: 2026-11-08. Tagged source:
`090e3bf789ad6c6969df4ee1b52077569bb0f5ef` (`v0.0.1`).

This report separates automated containerized-cluster evidence from native host
installation acceptance. The preview does not complete M4 or M7.

## Fast checks

Local verification and PR CI passed broker formatting/vet/tests, demo
formatting/vet/tests, Svelte/TypeScript checks, ESLint, production bundling,
full OpenAPI validation, chart positive/negative/render/synchronization checks,
and shell syntax. The source has 98 ordinary broker test functions, five demo
Go tests and 28 Python regression methods; table-driven subcases are additional.
Actionlint v1.7.12 passed with external shellcheck/pyflakes disabled.

New regressions cover setup/login/logout mutation headers, interleaved bootstrap
logs, diagnostic token redaction, native OCI architecture/version/content
identity, index normalization, release assembly/checksums and installer guards.
Go tests remain alongside packages; orchestration and helper tooling are grouped
under `tests/` and selected through `tests/run.py`.

## Local real-cluster evidence

The full local workflow verified browser bootstrap/login, authentication/CSRF,
invalid requests, idempotency, API deployment/update/scale 2-0-1/rollback/deletion,
browser deployment/update/progress refresh/history rollback/deletion, immutable
configuration Secret rollback (including actual contents), missing images,
Service reapply, and sessions/history/application traffic through broker Pod
replacement. An unhealthy rollout retained the previous serving workload and a
`needs_attention` lock through broker replacement, rejecting conflicting rollback.

The initial package runs exposed two distinct OCI index-wrapping behaviors and
an incorrect Service forward port. A diagnostic run was manually re-imported to
establish the containerd cause; that run is not counted as unattended package
acceptance. After correction, a separate unattended native ARM64 package run
built OCI archives, checked/imported recorded digest references, started the
packaged broker on a fresh K3s node without a registry, deployed/updated a demo,
verified its HTTP response, deleted it and cleaned owned resources.

The bootstrap parser failure, CSRF gap and asynchronous browser-assertion race
were corrected with focused regression or fresh-cluster browser verification.
The final focused browser run passed both workflows. Local artifacts are ignored
under `.local/e2e/`; current checks are retained for diagnosis rather than treated
as reference-host measurements. No test-owned containers remained after cleanup.

## GitHub release gate

[Release run #42510734382](https://github.com/piotrek1459/k8s-internal-platform/actions/runs/42510734382)
validates the tagged source, repeats fast checks, runs the full E2E suite without
local intervention, and builds both ARM64 and AMD64 release bundles. Publication
requires every gate and both packages; source identity and compatibility pins
are included in `release.json`, and `SHA256SUMS` covers the published assets.

The release run completed successfully. Its E2E JUnit report contains 13 cases:
11 passed, zero failed, and two explicit known-gap skips (`STATUS-1`, `RESTART-1`).
Both native architecture packaging jobs and the publication job passed.

The [published preview](https://github.com/piotrek1459/k8s-internal-platform/releases/tag/v0.0.1)
contains six assets: two native bundles, `install.sh`, `INSTALL.md`, `release.json`
and `SHA256SUMS`. All six were downloaded publicly after publication. Asset hashes
and every `SHA256SUMS` record matched; both bundles' native OCI identities matched
their metadata, the source commit matched the tag, and downloaded installer syntax
and `--help` passed. The standalone guide's links resolve to version-pinned source
documentation. Publication did not require GHCR credentials.

## Remaining acceptance boundaries

- `STATUS-1`: exported workload status uses PascalCase while the contract/UI use
  camelCase. Its precise known signature is an explicit E2E skip.
- `RESTART-1`: an already-frozen revision misses the new restart token and can
  finish without Pod replacement. Its precise known signature is an E2E skip.
- Failed-rollout restoration, interrupted-target verification, complete chart
  snapshots, private-registry credential provisioning, UI Secret/exposure
  integration, application volumes and public HTTPS remain separate work.
- The native Linux/systemd installer has not been executed on the reference Pi.
  Native boot, storage/cgroup compatibility, host reboot persistence, LAN access
  and rented-server acceptance remain pending. Containerized K3s and cross-build
  evidence do not establish those results.

The E2E workflow is configured for 02:00 UTC daily and manual execution. Those
triggers become available after the workflow is merged into the default branch;
release reuse runs directly from the tagged commit. CI artifacts are sanitized
and retained for seven days. Repeatable thesis measurements still require the
predefined hardware, repetitions and measurement conditions in the evaluation
plan.
