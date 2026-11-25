# Core recovery and disclosure verification — 2026-11-24

This source cycle follows the completed
[published v0.1.9 native gate](2026-11-24-v0.1.9-native.md). It implements the
remaining failure-path work in
[issue #16](https://github.com/piotrek1459/k8s-internal-platform/issues/16).
Source-cluster acceptance does not establish native release acceptance.

## Runtime change

Failed updates restore a frozen successful predecessor. Preparation resets the
owned Deployment template/replicas/strategy, preserves Service UID/address while
repairing its port list, and restores the canonical route or deletes it for an
Internal target. UID/resource-version guards and full Helm ownership prevent
repair of foreign/concurrently changed objects. Target verification requires
observed healthy replicas, exact managed fields and the expected resource set,
including route absence. Failed preparation/apply/verification retains the lock.
Terminating Deployments, Services and Ingresses are refused even if their fields
and stale Ready counters still match. A focused regression reproduced all three
false positives before the deletion-timestamp guard was added.

Supported version-guarded retry restores an already failed candidate's predecessor
directly, preserving that candidate's snapshot and failed Helm evidence. It does
not execute the known failed candidate again. Pending/unknown Helm records remain
blocked, and unavailable historical chart artifacts are still refused. Internal/
LAN and LAN hostname changes can therefore use the tracked revision path; the
interim 422 guard is retained in published v0.1.9, not in this new source.

Upstream registry/Secret/Helm failure bodies and dynamic SDK debug arguments are
excluded from broker messages/logs. The pinned SDK also persists raw admission
errors in failed descriptions before returning its error. A storage-driver wrapper
sanitizes new failed writes without mutating the SDK's in-memory release or
rewriting existing records. Error codes remain classified; successful descriptions,
chart/config identities, operation history and failed-release evidence are retained.

## Executed focused verification

The host was macOS/ARM64 with Linux Docker, pinned K3s/k3d/Helm and Chromium from
the existing E2E dependency set. Runs created their own random clusters, registries,
fixtures and loopback access; finalizers removed those resources.

- `focused-02`: **5 tests, 0 failures, 0 skips**, 101.684 s. Browser bootstrap
  and all four actual runtime SIGKILL barriers. The final SQL case blocks an
  actual writer transaction, observes the deployed target while SQL remains
  nonterminal, then verifies completion and independent queued work once.
- `focused-04`: **3 tests, 0 failures, 0 skips**, 60.365 s. Bootstrap, initial
  failed restoration/verification case and expanded disclosure/storage checks.
- `focused-05`: **4 tests, 0 failures, 0 skips**, 173.232 s. Bootstrap, all three
  failed restoration transitions, controlled disclosure failures and real private
  HTTPS mode/hostname transitions. Healthy predecessor HTTP is insufficient to
  unlock while verification is deliberately refused. Recovery preserves the
  original failed Helm UID/payload and frozen candidate values; candidate-only
  environment/Secret/pull fields are removed. Source HTTPS checks use independent
  CA/hostname verification and the existing controlled browser leaf pin.
- Full broker/demo formatting, vet and uncached regressions passed locally.
  Python/syntax/OpenAPI checks passed with **86 Python tests** and full validation
  of 24 paths/41 schemas using `tests/requirements.txt` in an isolated venv.

These focused runs preceded the clean committed branch. They identify their
selected cases and are not a substitute for the complete CI suite. A subsequent
Dockerfile relocation colocates the external fixture source/build definition;
the full committed-source run must include that layout and the terminating-resource
guard. The initial [CI run](https://github.com/piotrek1459/k8s-internal-platform/actions/runs/45453312193)
was explicitly cancelled as superseded after the additional regression exposed
that guard; it is not recorded as passing acceptance.

## Failed attempts retained

`focused-01` failed on a fixture argument-name collision before submission.
`focused-03` reached restored runtime state but used a faulty assertion comparing
failed Helm descriptions to platform revision IDs. Helm rewrites those descriptions;
the corrected check counts the failed candidate's frozen values hash in decoded
Helm configs. The storage privacy gap discovered during that investigation was
fixed and explicitly exercised by real denied Deployment writes. An initial
OpenAPI command lacked PyYAML in its interpreter; the required checks were rerun
successfully using the documented isolated environment. These failed commands
are not counted as passing evidence.

## Clean committed-source acceptance

The [full GitHub run](https://github.com/piotrek1459/k8s-internal-platform/actions/runs/45453842119) passed **27 tests, 0 failures, 0 skips**
at exact source `97b81c6859924fa9bef1aaec340d3db22acbc9bd`. All six new failure cases are present
in downloaded JUnit; its counts/individual outcomes and the strict non-empty,
zero-failure/zero-skip summary were independently checked. All five fast PR
checks passed at that same runtime source.

Downloaded artifact `14950677120` matched GitHub's SHA-256 digest
`8f81102270c25b81cceb57b7833b07fd8d3cf066988d2dd3ca01c8d0fc13da45`. JUnit SHA-256:
`bb4cf8e3e79b3fee5dd770fb0bd119492324d8f3187d69a23e1a717340836370`. The [machine summary](2026-11-24-core-recovery.json)
records exact run/source/case identities, fast-check URLs, superseded/failed
attempts and private evidence hashes. Local evidence remains under
`.local/core-recovery/`; cookies, credentials and raw Secret resources are excluded.
Subsequent documentation records this result without changing the tested runtime.
This is source acceptance, not native release or performance acceptance.

## Remaining acceptance

After administrator merge and manual publication, the unmodified release requires
native Pi installation/lifecycle/reinstall/reboot and controlled failure/retry/
access-transition checks. The external fixture is for disposable clusters, not
installation on the Pi. Native fault injection must remain explicitly owned and
bounded. The v0.1.9 rollout-adjacent browser timeout observation remains unconfirmed
and needs reproducible availability measurement; this source does not claim to
fix it or establish uninterrupted service. Resource calibration and repeated
manual-Helm/UI/API evaluation remain open in M0/M1/M4. Issue #16 remains open.

The native cycle must retain the following observations separately:

| Phase | Required evidence |
| --- | --- |
| Published identity | Unmodified installer/bundle checksums, source and live ARM64 image identity; read-only preflight exits; ownership inventory before any authorized test-state reset. |
| Healthy baseline | Setup/login, native lifecycle, actual trusted app HTTPS and retained operation/revision identities. |
| Failed restoration | An owned successful app, an actually applied unhealthy candidate and a bounded guarded denial of restoration; failed candidate/frozen evidence and 409 conflicts remain after actual broker replacement. |
| Failed verification | Restore permission permits apply while a guarded verification denial retains the lock despite healthy predecessor HTTP; replacement preserves that block. |
| Supported retry | Restore the exact recorded permissions under UID/resource-version checks; version-guarded recovery keeps the operation ID, preserves failed evidence and unlocks only after verifying the predecessor. Candidate-only env/Secret/pull fields are absent. |
| Access changes | Successful and failed Internal/LAN/hostname changes, effective API URLs, original route/absence and actual trusted HTTPS after convergence. |
| Disclosure | Controlled markers/encodings excluded from native ordinary/diagnostic responses, raw broker logs and read-only SQLite/Helm evidence; sensitive commands/files remain private. |
| Retention and cleanup | Same-version installer and actual reboot retain original identities/sessions/config; owned faults/temporary permissions/fixtures are removed and administrative access is closed. |

Fault controls must have cleanup recorded before activation. Unknown resource or
permission identity requires inspection rather than forced repair. No SQL rows
or Helm release records may be edited to resolve a lock. An unrelated/user app
in the ownership inventory requires a non-destructive validation plan instead
of resetting that installation. The four exhaustive external crash barriers
remain mandatory disposable E2E, not a reason to install the test webhook/PVC
fixture on the reference host.

## Subsequent published validation

The [v0.1.10 native report](2026-11-25-v0.1.10-native.md) completes the next
release's recovery/security/retention gate. This dated report preserves its own
original scope and failed observations; it does not establish measured resource
presets, human operator time or uninterrupted request availability.
