# External recovery fixture

This test-only executable provides deterministic barriers for
`tests/e2e/recovery_matrix.py`. Its Go source and Dockerfile are colocated under
the broker module to reuse the pinned Kubernetes/SQLite dependencies. Docker's
build context is `broker/`; workstation `.local/` credentials are excluded.
The platform Dockerfile builds only `cmd/platform`, so this fixture is absent
from native release images and adds no production broker endpoint or pause flag.

The Python controller refuses any cluster that it did not create with the
`platform-e2e-` prefix. It records labels and UIDs for an ephemeral TLS Secret,
Service, non-root Pod and namespace-scoped admission configuration. Controls
require a random in-memory token through an owned loopback port-forward. The
fixture has no mounted Kubernetes service-account credentials. It does mount the
test metadata PVC and therefore must never be installed on a real platform.

The admission endpoint observes or rejects only application-namespace Secret
and Deployment writes. Held requests are denied when released, timed out or
cancelled; a request from a killed broker cannot later commit successfully.
Readiness controls belong to the separate workload fixture. SQL control holds
an actual `BEGIN IMMEDIATE` transaction without modifying rows, allowing Helm
to finish while terminal platform recording is blocked. The fixture exposes
selected operation/snapshot metadata and bounded database/WAL/SHM scans with
positive controls; it never returns scanned content or Secret values.

Required cases cover:

- Broker runtime SIGKILL before Helm mutation, during pending storage, after
  cluster apply, and after deployed Helm but before terminal SQL.
- Frozen image, exact Secret references, chart content/version, values hash,
  timestamps and operation identity across replacement and actual tag movement.
- Refused restoration and refused verification across broker replacement for
  Internal/LAN/hostname changes, followed by supported guarded recovery.
- Controlled raw/base64/authentication values absent from ordinary responses,
  diagnostics, raw logs, actual SQLite/WAL/SHM and Helm release payloads after
  real admission failures. This finite matrix does not prove arbitrary output safe.

Admission/RBAC registration and application rollout are asynchronous: each case
positively observes its barrier before killing a broker or interpreting a denial.
RBAC changes use UID/resource-version tests. Cleanup releases SQL/admission
holds, removes the webhook before its Pod, validates recorded fixture identities,
and deletes the owned disposable cluster in the suite's finalizer. An external
machine shutdown can bypass finalizers; retained artifacts identify owned
`platform-e2e-*` resources for inspection. Production Helm records and SQL rows
are never manually edited to make recovery succeed.

Execution and acceptance policy are in the [verification guide](../../../../tests/README.md).
