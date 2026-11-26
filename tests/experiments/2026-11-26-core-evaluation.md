# Reference core evaluation — 2026-11-25/26

Published **v0.1.10**, source `c9caa4782d4d3a1d63093be06abf2d199d3b7e3f`,
was measured on the reference Raspberry Pi. The runtime was not rebuilt or patched.
The [machine summary](2026-11-26-core-evaluation.json) retains artifact hashes,
per-run results, original/excluded attempts and the measured preset decision.
The [protocol](../../docs/evaluation.md) and [operator walkthrough](../evaluation/HUMAN.md)
separate system measurements from the still-pending human trial. **M0–M4 is not
declared complete by this report.** Published-default validation and the human record remain required.

## Environment and collection

The Pi 5 has nominal 8 GB RAM and 32 GB microSD storage, 64-bit Raspberry Pi OS/
Debian 13.7, kernel 6.18.60+rpt-rpi-2712, K3s v1.36.5+k3s1 and Helm v3.22.0.
The platform, K3s add-ons and one retained healthy LAN demo remained installed.
No extended monitoring stack was added. Actual host/swap/disk/temperature facts,
boot identity, release image digests and chart input identities are retained.

The lifecycle series completed on 25 November before the administrator powered
off the Pi. Resume inspection on 26 November confirmed automatic K3s/platform
recovery, unchanged Deployment/application/revision identities and actual LAN HTTP v1.
The lifecycle result had no remaining owned fixtures and was retained.

The plan was committed before collection (`a36bc44`); the bounded-client amendment
was committed before its replacement resource series (`0ad2a93`). Request rate,
duration and acceptance thresholds did not change. Independent joined-data
validation checks timestamps, identities, counter continuity and request counts.

## Cached API versus manual Helm system time

Both methods ran on the Pi with identical released images and stored chart,
matching non-identity workload values, probes/resources and actual Service-HTTP
completion. Application/release IDs differ by owned fixture. Three repetitions alternated
method order. Authentication/preparation preceded timing. These durations exclude
human decisions and do not measure usability.

| Operation | API median (range), s | Manual Helm median (range), s |
| --- | --- | --- |
| Deploy v1 | 4.724 (4.682–4.812) | 4.551 (4.549–4.612) |
| Update v2 | 4.955 (4.950–4.965) | 4.547 (4.537–4.678) |
| Unhealthy v3 and verified restoration | 301.902 (301.704–302.091) | 302.837 (302.823–302.860) |

All six deployments and six healthy updates met the 30-second targets. Both failure
policies waited 300 seconds. The platform restored automatically; the manual baseline
used an explicitly scripted rollback after Helm failure. Backend stage timestamps
and manual detection/rollback observations remain in the raw lifecycle records.
The platform does not materially reduce Kubernetes rollout time in this sample;
the administrator-interaction comparison requires the human trial.

Deletion observations remain raw evidence but use the original collector completion
condition: tracked completion and absence of nonterminating Pods. They are not a
bound on dependent Pod garbage collection. The current collector waits for all owned
Pods, including terminating Pods. Three-sample percentiles are descriptive maxima.

## Idle and loaded resources

Each new single-replica fixture had 30 seconds idle, 10 seconds warmup, 60 seconds
at 20 requests/s and a bounded cooldown; a separate workstation verified HTTPS
and expected content. All six final joined traces passed: **7,200/7,200 steady
requests succeeded**. No successful response is substituted for an unsent/error observation.

| Case | Idle CPU median, m | Loaded CPU median/max, m | Lifetime peak, MiB | Loaded request p95, ms |
| --- | --- | --- | --- | --- |
| go-1 | 0.00 | 6.05/7.67 | 10.92 | 83.06 |
| nginx-1 | 0.00 | 4.52/5.12 | 9.67 | 80.68 |
| nginx-2 | 0.00 | 4.55/5.39 | 9.81 | 82.14 |
| go-2 | 0.00 | 6.09/8.06 | 10.44 | 81.66 |
| go-3 | 0.00 | 6.16/7.49 | 10.38 | 81.45 |
| nginx-3 | 0.00 | 4.65/5.27 | 9.66 | 80.58 |

CPU values are per-container one-second counter deltas. Memory peaks cover the
entire container lifetime, including startup, rather than only the load interval.
Throttling counters, OOM/restarts, raw host CPU/memory/swap, temperature and disk
are retained. Whole-host memory is MemTotal minus MemAvailable, not broker RSS.
Host measurements include K3s, the retained app and collectors; no isolated broker
overhead or generic minimum host size is inferred.

## Cold and cached public-image pulls

Only the exact unused predeclared Nginx image was removed through CRI. Live Pods
and every retained revision of undeleted applications were checked. Shared content
was not explicitly deleted or globally pruned. Native compressed descriptors and
overlayfs chain IDs were inventoried before each pull; absent references alone
cannot establish coldness. Each result includes actual native image identity,
Kubernetes pull events, HTTPS controls and startup memory peaks.

| Repetition | Requested condition | Observed content/snapshot state | Pod Ready, s | Actual HTTPS, s |
| --- | --- | --- | --- | --- |
| 1 | pull-required | fully-cold-content-and-snapshots | 8.611 | 8.628 |
| 1 | cached | all-content-cached | 4.478 | 7.181 |
| 2 | pull-required | fully-cold-content-and-snapshots | 8.695 | 8.712 |
| 2 | cached | all-content-cached | 4.790 | 5.570 |
| 3 | pull-required | fully-cold-content-and-snapshots | 8.555 | 8.650 |
| 3 | cached | all-content-cached | 4.660 | 5.001 |

These public-image timings are separate from the cached bundled-demo comparison.
They include API acceptance/inspection, readiness and their stated HTTP checks.

## Request continuity and preset decision

The 75-second app trace observed **367/375 successful requests**
during v1→v2 update and a UID-guarded deletion/replacement of the broker Pod.
The management readiness trace observed 358/375 successful requests;
its deliberate broker outage is reported separately from steady-state availability.
The original authenticated session worked after the replacement. Timeline, statuses,
errors and client schedules are retained; this finite sample is not an availability guarantee.

All eight app errors occurred around the application rollout, before broker
replacement. During the recorded 3.772-second broker replacement window,
all 18 sampled app requests succeeded. This supports the shared-cluster
broker-independence requirement; it does not pass the separate rollout trace.

### Rollout correction in source chart 0.1.3

A separate direct-Helm native comparison used identical images, explicit 48Mi
requests, probes, private ingress class and workload brief. Both cases performed
three v1/v2 rollouts during a 75-second, five-request-per-second client trace.
The published chart control returned **344/375 successes (31 errors)**; the
source chart with five-second native preStop sleep returned **375/375 successes**.
Live candidate Pods retained the intended sleep action through each update.
Stored chart identities, values, timelines and all requests remain in the dataset.

The result is consistent with asynchronous endpoint/proxy removal while a process
starts shutdown. The [native sleep action](https://kubernetes.io/docs/concepts/containers/container-lifecycle-hooks/)
lets the old process continue serving briefly before its stop signal. The hook
requires no command inside the image and leaves 25 seconds of the 30-second
grace period for application shutdown. Source chart 0.1.3 requires Kubernetes
1.34+, where [KEP-3960](https://github.com/kubernetes/enhancements/blob/master/keps/sig-node/3960-pod-lifecycle-sleep-action/kep.yaml)
is stable; the tested K3s pin is unchanged. This finite comparison supports the
reference fix rather than a universal zero-downtime claim.

This is a **source application-chart experiment on the native cluster**. The
v0.1.10 platform binary and its embedded chart were not replaced. Source CI and
the following manual-release/native API validation remain separate gates.

The predeclared headroom rule uses maximum steady CPU **8.060m**
and conservative maximum observed lifetime memory **17.156 MiB**
(including retained preliminary and cold-start observations). It derives:

- Requests: **25m CPU / 48Mi memory**.
- Limits: **250m CPU / 128Mi memory**.

This is a small stateless HTTP reference profile. Arbitrary images, databases,
higher rates and longer runs require separate sizing. Limits may censor demand;
startup throttling and page-cache accounting must be read with the raw counters.
The explicit profile passed native Go and Nginx checks with two Ready replicas
and actual HTTPS for each workload. One initial single-probe check returned 404;
that failed helper attempt is retained. The corrected verifier records bounded
route convergence. Subsequent published-default validation retains separate evidence; existing platform upgrades are not established by these results.

## Excluded attempts and remaining gates

- The first resource attempt had no matching workstation trace before shutdown.
- The initial four-slot series dropped three unsent Nginx requests during a latency
  burst. All sent requests succeeded, but the generator was invalid. That entire
  series was excluded from the final loaded comparison; its raw observations remain.
- One replacement Go run began after the Pi sampling window. Joined validation
  refused it. The matching first three cases were retained and the remaining cases
  used new identities, late-start guards and temporary workstation idle-sleep inhibition.
  The exact cause of the workstation delay is not claimed.
- A direct-Helm diagnostic setup initially selected `traefik` instead of the
  installed `platform-private` ingress class. Its 404 setup failure is excluded;
  the manual walkthrough/template and controlled comparison now match the platform.
  A second diagnostic helper stopped on a three-second read timeout; its partial
  control trace is retained separately from the completed three-update comparison.
- Earlier route-convergence and post-cleanup login-timeout observations remain
  historical, cause-unconfirmed cases. Passing these controlled traces does not
  retrospectively turn those attempts into passes or establish their cause.

The human active-time/step record remains pending. The administrator must merge
and manually release this PR, followed by published native/default validation and
completion of the [short timed walkthrough](../evaluation/HUMAN.md). Issue #16
remains open until the applicable evidence is complete. M5–M8 remain separate.

## Source verification and final native state

[Full source CI](https://github.com/piotrek1459/k8s-internal-platform/actions/runs/45832187594)
passed **27 tests, 0 failures, 0 skips** at
`29216455cfeae52eef803f9987008ecb844d8bb2`. The downloaded artifact digest was
verified against the GitHub API, then JUnit and the new private-LAN request trace
were inspected: **56/56 verified requests** succeeded across API update/rollback.
The trace uses normal CA/hostname validation and retains failures on error.
All five fast checks passed on that runtime source. Later documentation/data
commits retain the same broker/chart/UI/runtime code; their fast checks include
recomputation/integrity regressions for this dataset.

Final native audit found the unchanged v0.1.10 platform Deployment/image and
original LAN demo/revision serving v1. All cluster Pods were Ready; no benchmark
apps/releases, application volumes, configuration Secret metadata or registry
credentials remained. The temporary native credential bundle/copy was removed,
SSH control connection closed, and the bounded workstation idle inhibitor stopped
or expired. Canonical administrator state remains private for the next acceptance
cycle. This cleanup does not delete the working platform or its retained history.

## Recompute the committed dataset

The machine summary lists SHA-256 for stored and expanded artifacts. Exact
collector/orchestration source snapshots are retained without credentials or
SSH/CA private keys; imported library source and runtime identity remain versioned
in the repository and per-run metadata. Deterministic
gzip retains raw JSONL without embedding credentials. For example:

```bash
python3 tests/evaluation/analyze.py \
  --host tests/experiments/2026-11-26-core-evaluation/resources/go-1/host \
  --client tests/experiments/2026-11-26-core-evaluation/resources/go-1/client \
  --fixture tests/experiments/2026-11-26-core-evaluation/resources/go-1/fixture.json \
  --plan tests/experiments/2026-11-26-core-evaluation/resource-plan.json \
  --clock-offset-seconds 0.017563820 \
  --output .local/recomputed-go-1.json
```
