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

## Published v0.1.2 native result

Release: [v0.1.2](https://github.com/piotrek1459/k8s-internal-platform/releases/tag/v0.1.2).
Source: `446ee0794a6cbd462edb9f83220e9754a8c75916` (merged PR #14).
The [manual release run](https://github.com/piotrek1459/k8s-internal-platform/actions/runs/45068352693)
completed successfully. The release contains both architecture bundles,
`install.sh`, `INSTALL.md`, `release.json` and `SHA256SUMS`.

The published installer, guide and manifest checksums matched on the Pi. The
installer refused the existing `0.1.1` installation before any upgrade mutation,
as designed. API and Kubernetes checks confirmed that the baseline application
and metadata volume were the only test workloads. A private cold copy of the
stopped baseline metadata/control-plane state was retained with mode `0600`;
its restoration was not tested. Only that session-owned installation was reset.
This was a fresh install, not a supported cross-version upgrade.

The published command `sudo bash install.sh --version v0.1.2` then exited `0`.
The installer downloaded/imported its ARM64 bundle, installed the platform and
established readiness without Docker, Go or Node.js on the target. Observed pins:
K3s `v1.36.5+k3s1`, Helm `v3.22.0`; kernel `6.18.60+rpt-rpi-2712`.
The active cgroup controllers included `memory`. Kubeconfig and installed release
metadata had mode `0600`. The broker image reference was:

```text
platform.local/platform-broker@sha256:6ac0bab04c2471336e4f771721da04c7321f59df0ac43ac35c82b1eea868711e
```

| Check | Observed result |
| --- | --- |
| Browser setup | Root opened setup; an invalid token displayed the nested API message; account creation and login succeeded. |
| Initial deployment | Bundled v1 deployed by digest; camelCase status and a real HTTP version response matched. |
| Update and scale | v2 update and scale 2/0/1 succeeded. |
| Restart | Pod identity changed while the image digest remained fixed. |
| Failed update | Deliberately unhealthy v3 failed; the v2 snapshot and live resources were restored before unlock. An HTTP request during failure also returned v2. This was an availability observation, not a continuous zero-downtime measurement. |
| Explicit rollback | The original successful v1 snapshot was restored. |
| Deletion/history | Deletion completed and revision history remained readable. The native harness initially asserted immediate Pod deletion; the assertion was corrected to allow bounded Kubernetes garbage collection. A repeated deletion passed with the corrected check. |
| Same-version reinstall | The published installer exited `0` again. The original session, administrator, application, revision/history, PVC UID and v1 HTTP response were unchanged. |
| Host reboot | K3s was active/enabled without manually starting it; the node and broker recovered. The pre-reboot session, account, app/history, PVC UID and v1 HTTP response passed comparison. |

Native results are retained privately under `.local/pi-acceptance/native-v012/`,
with additional reinstall/reboot identity comparisons. The generated administrator
password is stored only in `.local/pi-acceptance/native-access.json` (mode `0600`).
Raw credentials, cookies, kubeconfigs and bootstrap logs are excluded from tracked
reports. A healthy v1 demonstration application remains installed.

The boot flags had already been enabled during the earlier host-preparation
attempt. This run therefore establishes native installation/reboot with enabled
cgroups; the new `--prepare-host` transformation has automated regressions but
was not invoked against disabled cgroups on this run.

## Additional image-compatibility finding

A browser deployment of `docker.io/nginxinc/nginx-unprivileged:stable-alpine`
passed registry inspection and ARM64 resolution, then failed startup because
`mkdir /tmp/proxy_temp` encountered a read-only filesystem. The frozen image was:

```text
index.docker.io/nginxinc/nginx-unprivileged@sha256:f5116ed4606f3eb015dd4a8e28cb9b351476dc0ad4bc6e9f93b19cec130cdf03
```

A controlled patch of that test Deployment added only a `/tmp` emptyDir with a
64Mi size limit and filesystem group `2000`/`OnRootMismatch`. The image then
became Ready and returned the Nginx welcome page over HTTP 200. The root
filesystem remained read-only. The experimental application was deleted after
verification. This is evidence for the proposed chart fix, not an unmodified
`v0.1.2` browser-deployment pass.

The canonical and embedded application charts are updated to `0.1.1` with these
settings and a rendered-manifest regression. The fix requires a subsequent
published platform release and a fresh native compatibility check. The published
`v0.1.2` assets remain unchanged. Native AMD64, public HTTPS, application data
volumes, backup restoration and cross-version upgrades are outside this run.
