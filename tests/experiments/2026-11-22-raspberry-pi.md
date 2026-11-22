# Reference Raspberry Pi installation and release acceptance

Date: 2026-11-22. Target: Raspberry Pi 5 Model B Rev 1.1, ARM64, 8 GB RAM,
32 GB microSD, Raspberry Pi OS/Debian 13 (Trixie). Initial root filesystem:
29 GB total, approximately 20 GB available. Docker, Go and Node.js are not
required on the target. Access uses SSH and loopback forwards.

## Initial published-release attempt

`v0.1.1` installer and SHA256SUMS were downloaded from GitHub Releases and the
installer checksum matched. Read-only preflight correctly refused the host:
`/sys/fs/cgroup/cgroup.controllers` did not contain `memory`. No K3s installation
was started by this rejected attempt.

The original `/boot/firmware/cmdline.txt` was backed up as
`cmdline.txt.platform-backup`. The existing command line was preserved and
`cgroup_memory=1 cgroup_enable=memory` was appended. After a manual reboot,
`memory` appeared in the active cgroup controllers and preflight passed.

The unmodified published `v0.1.1` installer then exited successfully. K3s
`v1.36.5+k3s1` reported a Ready node; Helm `v3.22.0` installed the platform.
`/readyz` returned `status=ready` and `cluster=available`; the authenticated system
endpoint reported platform `0.1.1` and `arm64`.

Executed baseline: administrator setup/login, bundled digest-pinned v1 deployment,
scale 2/0/1 and v2 update. All completed successfully. Application details
reproduced the PascalCase status defect in this release. A same-version installer rerun also exited successfully with the account and
application retained. The fixes and stricter acceptance belong to the subsequent
release, not to this baseline.

## Acceptance procedure

The final published release is downloaded and checksummed on the Pi. Only the
installation and test data created by this acceptance session may be reset.
Existing unrelated host state is not removed. The new release is installed using
its published installer, not a copied development binary.

The opt-in native acceptance command is documented in the [test guide](../README.md#native-release-acceptance).
It verifies browser setup/error messages/login, ARM64 version/cluster health,
application HTTP responses, status fields, updates, scaling, Pod-replacing restart,
unhealthy-update restoration, explicit rollback, deletion and retained history.
One owned healthy application is retained for same-version installer rerun and
host-reboot verification. Raw logs and administrator credentials are ignored local
artifacts; credentials and Kubernetes logs are not included in this report.

## Candidate verification

A disposable real-cluster run reproduced a false recovery block: Kubernetes
adds probe `successThreshold: 1`, whereas Helm stores the manifest without that
explicit default. The live verifier was corrected to normalize this API default
while retaining image, probe, generation, replica and Service checks. A focused
regression covers the defaulted object and rejects image/generation drift.
Direct verification of the restored live resources then succeeded.

The complete local API/browser/bundle suite subsequently passed: **14 tests,
0 failures, 0 skips**, 568.085 seconds of test execution, with build version
`0.1.2`. This includes browser setup and nested error rendering, Secret-reference
submission, healthy lifecycle/idempotency, authenticated registry inspection and
actual kubelet pull, configuration rollback, reapply after Service deletion,
broker replacement, verified unhealthy-update restoration, a native ARM64 OCI
bundle on a second fresh K3s node, camelCase status and Pod-replacing restart.

Fast verification also passed: broker formatting/vet/uncached tests, UI type
checking/lint/build, 55 Python regressions, complete OpenAPI validation, chart
matrix and shell syntax. The PR's five GitHub checks passed. Raw results are
ignored local artifacts under `.local/pi-acceptance/`; these development checks
are separate from the next published release's native-host acceptance.

Both disposable clusters, their registries and the temporary Buildx builder were
removed by the E2E runner. Temporary Pi management/Kubernetes proxies and SSH
forwards were stopped. The working `v0.1.1` installation and its owned baseline
application remain available for the subsequent published-release installation
cycle.

## Final release result

Final release installation, same-version rerun and reboot acceptance are pending
execution. Results below must be updated from observed output before acceptance
is recorded as complete. Native AMD64, public HTTPS, application volumes and
cross-version upgrades are outside this run.
