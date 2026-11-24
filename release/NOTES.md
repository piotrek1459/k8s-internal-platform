Kubernetes internal application platform.

The platform and managed applications share one single-node K3s cluster. The
release includes a Go broker with an embedded Svelte UI, a durable SQLite queue,
digest-pinned application deployment, update, scaling, restart, rollback,
configuration Secrets and private registry authentication stored in Kubernetes.

Download `install.sh`, `SHA256SUMS` and `INSTALL.md` to begin. Native OCI bundles
are provided for dedicated ARM64/AMD64 Linux/systemd hosts. Docker, Go, Node.js
and registry accounts are not required on the target. K3s/Helm are pinned and
`release.json` records source, architecture and image digests.

Raspberry Pi memory-cgroup preparation is available through the explicit
`--prepare-host` option. The boot configuration is backed up and a manual reboot
is required; normal host checks remain read-only. Reference-host installation
results and remaining acceptance boundaries are recorded in `docs/status.md`.

Status responses use camelCase, setup navigation detects a fresh installation,
errors preserve the server message and creation submits selected Secret versions.
Restart replaces Pods while retaining the image digest. Execution snapshots record
chart and values identity. Failed updates attempt verified restoration before
unlocking; ambiguous state remains `needs_attention` and permits a recovery retry.
Restoration now clears failed-candidate fields and verifies the full managed
resource set, including absence of an Internal application's route. Guarded retry
restores the successful predecessor without re-executing an already failed
candidate. Access-mode and LAN hostname changes use tracked revisions; failed
transitions retain their lock until the previous state is verified. Pending Helm
records and foreign ownership remain protected.
A different historical chart artifact is refused rather than silently substituted.

A blocked first deployment without a successful predecessor supports explicit
tracked removal while retaining its failed operation and frozen history.
Application details provide bounded, ownership-checked logs and Kubernetes events,
with managed-Secret masking and escaped browser text. Read-only drift inspection
reports owned workload/route field paths without values or automatic correction.
Explicit Reapply repairs owned ClusterIP Service port lists under UID and resource
version guards while preserving identity/address. Pending transactions and foreign
ownership remain blocked; ownership checks also cover restoration and uninstall.
Diagnostic redaction and retention limits are documented in the API guide.
Upstream Helm/registry/Secret error bodies and dynamic SDK debug arguments are
excluded from broker errors/logs. New failed Helm release descriptions are
sanitized before storage without rewriting historical evidence. Deterministic
external admission/SQLite barriers test four worker crash boundaries, failed
restoration/retry and controlled disclosure paths in disposable clusters; these
fixtures are absent from the released broker image.

Configuration Secret deletion reserves cleanup durably, removes recorded immutable
Kubernetes versions under UID/resource-version and ownership guards, then removes
metadata. Active/rollback/in-flight references remain protected; retained history
of deleted applications no longer blocks cleanup. Partial cleanup remains visible
and retryable across broker replacement, and rejects new versions/references.
The browser provides an explicit Retry deletion action. Credential and Secret
management acceptance covers real browser creation, version selection, invalid
private credentials, kubelet pulls, deletion guards and physical Secret removal.

Application chart `0.1.1` provides a Pod-local `/tmp` emptyDir with a 64Mi
size limit and a filesystem group for non-root image users. This permits common
images such as unprivileged Nginx to create temporary files while the root
filesystem stays read-only. Temporary files are discarded with their Pod.

Public HTTPS, application data volumes, verified backup/restore and cross-version
platform upgrades remain separate work. Administration uses SSH/loopback access;
private LAN ingress requires explicit configuration. Release publication requires
fast regressions, real-cluster API/browser/private-pull/bundle checks and both
architecture packages. Publication is manual from a selected branch HEAD.
Nightly E2E is scheduled at 02:00 UTC and also supports manual dispatch.

Optional private LAN HTTPS is implemented in this source: explicit private IPv4
binding, source CIDRs, an existing certificate/key Secret, Secure cookies and
configured app URLs. See [LAN configuration](../docs/private-lan.md) for DNS/trust,
renewal and the TLS/SSH fallback. The gateway image is pinned and pulled on first
use; it is not in the native OCI bundle. Published v0.1.8 passed reference-Pi
LAN trust/browser/API routing, isolation and reinstall/reboot retention. Published
v0.1.9 additionally passed corrected installer checks and fresh installation,
trusted LAN workflows, same-version retention and reboot. New
release installers require their own native verification.
Application chart 0.1.2 adds the dedicated private class. Existing frozen chart
identities are preserved; historical-chart retrieval and cross-version upgrades
remain unsupported.

Successful read-only installer `--check` now exits 0 while failed preflight retains
its nonzero status. This corrects v0.1.8 returning 1 after printing a successful
host check. The unmodified published v0.1.9 installer passed fresh/installed/LAN/
reinstalled/rebooted preflight checks and the native lifecycle/persistence gate.
The new recovery/security and access-transition behavior requires separate native
validation after manual publication. Resource calibration and controlled core
evaluation remain outstanding; the native report retains unresolved rollout-adjacent
browser timeouts without claiming a confirmed cause or uninterrupted service.
