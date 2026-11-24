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
the full committed-source run must include that layout.

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

The full CI run, exact source, downloaded artifact digest, JUnit and strict
summary will be recorded here before the PR is marked ready. All 27 workflows
must pass without failures, skips or expected-failure waivers; all five fast PR
checks must also pass. Local evidence remains private under `.local/core-recovery/`.

## Remaining acceptance

After administrator merge and manual publication, the unmodified release requires
native Pi installation/lifecycle/reinstall/reboot and controlled failure/retry/
access-transition checks. The external fixture is for disposable clusters, not
installation on the Pi. Native fault injection must remain explicitly owned and
bounded. The v0.1.9 rollout-adjacent browser timeout observation remains unconfirmed
and needs reproducible availability measurement; this source does not claim to
fix it or establish uninterrupted service. Resource calibration and repeated
manual-Helm/UI/API evaluation remain open in M0/M1/M4. Issue #16 remains open.
