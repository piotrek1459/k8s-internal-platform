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
