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

Public HTTPS, application data volumes, verified backup/restore and cross-version
platform upgrades remain separate work. Administration uses SSH/loopback access;
private LAN ingress requires explicit configuration. Release publication requires
fast regressions, real-cluster API/browser/private-pull/bundle checks and both
architecture packages. Publication is manual from a selected branch HEAD.
Nightly E2E is scheduled at 02:00 UTC and also supports manual dispatch.
