# Reference-Pi v0.1.3 and core recovery acceptance

Date: 2026-11-23. Follow-up to [issue #16](https://github.com/piotrek1459/k8s-internal-platform/issues/16)
and the [earlier Pi report](2026-11-22-raspberry-pi.md).

## Published release on the reference Pi

The normal GitHub release `v0.1.3` identifies source commit
`aea889af2a9154128c9edf9001bac17343a9c31c`. Its installer, guide and release
manifest passed their published checksum checks. The installer downloaded and
verified the native ARM64 bundle; no development checkout, Docker, Go or Node.js
was used on the Pi. K3s is `v1.36.5+k3s1`; Helm is `v3.22.0`.

The host is Raspberry Pi 5 Model B Rev 1.1 with 8 GB RAM, 32 GB microSD and
64-bit Raspberry Pi OS/Debian 13. The kernel is `6.18.60+rpt-rpi-2712`.
Post-test disk capacity was 29 GB total, 7.9 GB used and 20 GB available.
These observations do not calibrate workload resource limits.

Read-only preflight correctly refused replacing `v0.1.2`. Inventory established
that the existing installation contained only the previous session's acceptance
application and no configuration Secrets, registry credentials or additional
application volumes. A root-only stopped-state archive was retained and its gzip
integrity checked before that dedicated test installation was reset. Restoration
of the archive was not exercised; this is not backup/restore acceptance or a
supported cross-version upgrade procedure.

| Check against unmodified published v0.1.3 | Result |
| --- | --- |
| Fresh installer execution | Exit 0; K3s active/enabled, node Ready. |
| Browser setup and login | Root selected setup; invalid-token message displayed; account creation and login succeeded. |
| Bundled v1 deployment | Ready status and actual HTTP `/version` response verified. |
| Healthy v2 update and scale 2/0/1 | Successful operations and real workload state. |
| Restart | Pod UID changed; exact image digest retained. |
| Unhealthy v3 update | Candidate failed; v2 snapshot restored and verified before mutation unlock. |
| Explicit rollback | Original successful v1 snapshot and actual HTTP response restored. |
| Deletion | Workload removed, dependent Pod deletion awaited, revision history retained. |
| Public-image browser workflow | Unprivileged Nginx became Ready and returned its welcome page with HTTP 200; no Deployment patch. |
| Same-version installer rerun | Exit 0; original session/account/application/revision/history/PVC UID and HTTP response unchanged. |
| Host reboot | K3s recovered without a manual start; node Ready, memory cgroup active; original session and persisted identities unchanged. |

The Nginx input was `docker.io/nginxinc/nginx-unprivileged:stable-alpine`, port
`8080`, readiness `/`, Internal exposure. The recorded immutable image was:

```text
index.docker.io/nginxinc/nginx-unprivileged@sha256:f5116ed4606f3eb015dd4a8e28cb9b351476dc0ad4bc6e9f93b19cec130cdf03
```

The resulting Pod used filesystem group `2000`, an `emptyDir` mounted at `/tmp`
with size limit `64Mi`, a read-only root filesystem and privilege escalation
disabled. Its application was deleted after verification. One healthy lifecycle
demo remained for access and persistence checks. Temporary administrative
Kubernetes proxies and SSH forwards were stopped; the dedicated survival-check
session was revoked and its local cookie removed. The generated administrator
password remains only in the ignored mode-0600 acceptance state file.

The optional `--public-image` argument to `tests/e2e/native.py` retains this
browser compatibility check for subsequent native acceptance. The first attempt
at the new helper used an exact `Next` locator, whereas the UI's accessible
button name is `Next →`. That harness selector was corrected before the successful
run. No product change or Deployment patch was required for Nginx.

## Direct Helm baseline on the Pi

The application chart was extracted from tag `v0.1.3`, not copied from an
unrecorded worktree. SHA-256 of its source tar:

```text
790afda360372622c96da562927f42d6f43667c69fd271f7d230b3488eee2ae1
```

A new `platform-baseline` namespace enforced restricted Pod Security admission.
It was independent of the broker-managed namespace and held only this baseline.
Per-version values selected the exact ARM64 demo image in release `release.json`:

```json
{
  "appId": "manual-pi-baseline",
  "image": {
    "repository": "platform.local/http-demo",
    "tag": "",
    "digest": "<digest from targets.arm64.images.v1, v2 or v3>",
    "pullPolicy": "Never"
  }
}
```

The installer-owned Helm/kubectl tools and root-readable K3s kubeconfig were used:

```bash
export KUBECONFIG=/etc/rancher/k3s/k3s.yaml
export PATH=/usr/local/lib/k8s-internal-platform:$PATH
kubectl create namespace platform-baseline
kubectl label namespace platform-baseline pod-security.kubernetes.io/enforce=restricted
helm install manual-baseline deploy/helm/application -n platform-baseline -f v1.json --wait --timeout 120s
helm upgrade manual-baseline deploy/helm/application -n platform-baseline -f v2.json --wait --timeout 120s
# Expected failure: the v3 process runs but readiness returns HTTP 503.
helm upgrade manual-baseline deploy/helm/application -n platform-baseline -f v3.json --wait --timeout 30s
helm rollback manual-baseline 2 -n platform-baseline --wait --timeout 120s
helm uninstall manual-baseline -n platform-baseline --wait --timeout 120s
kubectl delete namespace platform-baseline --wait=true --timeout=120s
```

The actual response was read after each step through the Kubernetes Service
proxy at `/api/v1/namespaces/platform-baseline/services/manual-baseline:80/proxy/version`.
After the unhealthy update, Helm status was `failed`; v3 had started with zero
container restarts, was unready, and had recorded readiness HTTP 503 events.
The Service continued to return v2 in the recorded observation. Explicit rollback
to Helm revision 2 produced revision 4 with status `deployed` and HTTP v2.
The release and owned baseline namespace were removed afterward.

| One functional run, cached images | Observed duration |
| --- | --- |
| Install v1 and verify HTTP | 5 s |
| Update v2 and verify HTTP | 4 s |
| Deliberate unhealthy update | 31 s, expected exit 1 |
| Explicit rollback and verify HTTP | 2 s |

Raw transcript, exact values, version metadata, Pods/events, Helm history,
responses and timing CSV remain in the ignored native acceptance directory and
the Pi's release-specific baseline directory. The retained results archive SHA-256
is `94fd6577a96eb2fe25d44291170d5bab966466865edaf45ec001dd160e08f1eb`.
The [machine-readable summary](2026-11-23-core-acceptance.json) records release,
image identity and baseline observations without credentials.

This is one functional baseline with cached images. It is not a repeated
performance experiment, an operator-active-time comparison, a resource peak
measurement or normal LAN-ingress acceptance. The normal platform acceptance
also ran on the host during part of the baseline; no isolated performance
comparison is inferred from these durations.

## Migration and blocked-deployment follow-up

Migration regressions inject both an invalid SQL statement after DDL/data writes
and failure of migration-ledger insertion. The production transactional runner
is used with an instance-local fixture filesystem. After reopening the database,
the candidate table and ledger row are absent and existing data is unchanged.
A corrected migration then succeeds and can be retried idempotently. This does
not establish cross-version application upgrade or backup restoration support.

A blocked first create with no successful predecessor now supports explicit
version-guarded deletion. Supersession preserves the failed revision and frozen
snapshot and transfers the lock to tracked cleanup within the acceptance
transaction. Store regressions cover stale input, running/queued/recovering work,
a prior successful revision, a blocked update, duplicate deletion replay and a
failure after supersession that must roll the entire transaction back.

The disposable real-cluster follow-up uses an observed pending Helm install and
a running but deliberately unready v3 Pod as its interruption barrier. It sends
runtime SIGKILL to the broker container on the suite's owned k3d node. The
expected contract is retained pending evidence and a durable first-operation
block, independent queued-work execution, persisted browser progress without
another submission, and tracked removal of the blocked first deployment.
The final executed result is recorded below after verification.

Remaining issue #16 work includes the other interruption barriers, failed
restoration/recovery acceptance, moved-tag and disconnected-client cases,
comprehensive secret-disclosure and workload-origin checks, browser credential
and Secret management, diagnostics/drift, normal private LAN HTTPS and the
controlled reference-host evaluation. This report does not declare M0–M4 complete.
