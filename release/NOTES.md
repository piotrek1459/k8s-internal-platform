Kubernetes internal application platform.

The platform and managed applications run together in one single-node K3s
cluster. The release includes a Go broker with an embedded Svelte UI, a durable
SQLite operation queue, immutable image revisions, HTTP application deployment,
update, manual scaling, rollback, deletion and versioned configuration Secrets.

Download `install.sh`, `SHA256SUMS` and `INSTALL.md` to begin. Dedicated 64-bit
Linux/systemd hosts are supported by the installer design. Native OCI bundles
are supplied for ARM64 and AMD64; no registry account, Docker, Go or Node.js is
required on the target. K3s and Helm are pinned, and bundle image references use
validated native digests. `release.json` records source and compatibility pins.

Native Raspberry Pi installation is awaiting reference-host
validation. Administration uses SSH/loopback forwarding. Public HTTPS,
application volumes, private-registry credentials, verified failed-rollout
restoration and cross-version upgrades are incomplete. Application status
serialization and restart behavior have known defects. A failed rollout retains
an explicit `needs_attention` lock; automatic recovery is not claimed.

Release publication requires fast regression checks, real-cluster API/browser
workflows and both architecture packages. The E2E report explicitly identifies
the two known status/restart gaps instead of counting them as accepted behavior.
The E2E workflow is also configured for nightly execution at 02:00 UTC and manual
dispatch. Release publication is initiated manually from a specified branch HEAD.
