# Core resilience and workload boundary acceptance

Date: 2026-11-23. Follow-up to [issue #16](https://github.com/piotrek1459/k8s-internal-platform/issues/16).

## Published baseline and native boundary

[PR #19](https://github.com/piotrek1459/k8s-internal-platform/pull/19) is merged and
published in normal release [v0.1.6](https://github.com/piotrek1459/k8s-internal-platform/releases/tag/v0.1.6),
source `cdfe951719ff1903badf01c84ef1bde8007c3589`. It includes guarded, retryable
configuration Secret cleanup and browser configuration acceptance. Its preceding
source suite passed [17 tests with zero failures/skips](https://github.com/piotrek1459/k8s-internal-platform/actions/runs/45194804590).

Native v0.1.6 installation/cleanup acceptance remains pending: reference-host SSH
is currently unavailable. No SSH attempt or native v0.1.6 result is claimed.
The last native run remains v0.1.5, with successful lifecycle/diagnostics/reinstall/
reboot checks and failed Secret cleanup recorded in the
[earlier report](2026-11-23-v0.1.5-secret-lifecycle.md).

## New disposable-cluster workflows

The following checks run against the actual broker/UI, SQLite queue, Helm SDK,
registry and Kubernetes resources in an owned K3s cluster. A test workload holds
readiness until authenticated explicit release; the running operation, pending
Helm release, frozen image and running/unready container establish the barrier.
The fixture is not included in published platform/demo bundles.

| Workflow | Verified invariant |
| --- | --- |
| Lost acceptance response | A loopback proxy reads the backend's complete 202 after durable acceptance, then closes the client connection without returning headers/body. A failed pre-acceptance request cannot satisfy this fixture. Cookies and raw bodies remain in memory. |
| Real moved registry tag | The owned tag initially resolves to fixture v1. After snapshot freezing, it is republished as v2 and inspection confirms a different digest. Original-key replay preserves application/operation/revision identity, frozen digest and actual Helm chart/values hashes. |
| Completion and later retry | Explicit readiness release completes the original operation and real HTTP reports fixture v1. Pod specification uses the frozen digest. Replay after broker replacement and explicit Reapply keep that digest/chart/values identity despite the moved tag. |
| Running browser refresh | The browser submits once, reloads/navigates during an observed running operation, then resumes persisted progress. Explicit readiness release completes the same operation/revision, followed by successful reload and real HTTP. |
| Workload-origin authorization | The actual managed Pod reaches platform health, receives 401 for protected management reads/mutations, and receives a TLS-verified Kubernetes Unauthorized/Forbidden Status for cross-namespace Secret reads and Namespace creation. Application/Secret counts stay unchanged and the attempted Namespace is absent. |
| Service-account boundary | The ordinary application Pod uses its namespace's default service account with automount disabled and no projected token. The process verifies the standard token file is absent. Network/TLS failure cannot masquerade as an authorization refusal. |

Raw Helm release data is read privately and hashed in memory. Only sanitized
status classifications and chart/values hashes enter uploaded step artifacts.
The readiness control token is an immutable configuration Secret, not ordinary
environment plaintext. Owned temporary image tags, registry data and clusters
are cleaned at completion.

## Executed verification

The focused local ARM64-container run passed **4 tests, 0 failures, 0 skips** in
42.931 seconds of case execution, excluding initial platform build/setup.
It includes bootstrap and all three new workflow cases. Go broker/demo/fixture
formatting, vet/tests and 70 Python regressions passed. UI type checking/lint/build,
OpenAPI, chart matrix and shell checks passed after serial execution.

An initial local UI check failed because E2E startup and the UI check concurrently
ran `npm ci` against the same checkout. The affected UI/API/chart/syntax checks
were rerun serially and passed. That failed attempt is not accepted as green.
GitHub jobs use separate workspaces. The clean-source [GitHub E2E run](https://github.com/piotrek1459/k8s-internal-platform/actions/runs/45220381729)
passed **20 tests, 0 failures, 0 skips** at
`0a616fbc1c5705862cd9a68530523a7b3a803468`. Downloaded JUnit and the strict
zero-skips summary were verified. Recorded case time totalled 729.013 seconds,
including 36.762 seconds for workload-origin acceptance (with fixture image
build/push), 10.427 seconds for lost-response/moved-tag retries and 9.299 seconds
for running browser refresh. All five fast GitHub checks passed. The final
evidence update changes documentation only.

The managed workload received HTTP 200 from platform health, HTTP 401 from all
three protected management requests and HTTP 401 / Kubernetes Status Unauthorized
from both protected Kubernetes requests. All six transport checks succeeded and
the service-account token was absent. The [machine-readable summary](2026-11-23-core-resilience.json)
retains these sanitized results.

## Remaining acceptance

The moved-tag case verifies API replay, completed-operation restart and explicit
Reapply. It does not substitute for the remaining deterministic worker crash
barriers before/after external apply or before terminal SQL recording. Failed
restoration/retry and comprehensive negative-path disclosure also remain open.
Private LAN HTTPS/DNS/trust, native v0.1.6 checks, reference resource calibration
and controlled repeated evaluation require their own evidence. This run is not
a performance dataset or proof of every production security boundary.
