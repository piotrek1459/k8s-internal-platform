# Core evaluation protocol

This protocol supplies M1 resource calibration and the repeated M4 system
comparison. It follows [published v0.1.10 native acceptance](../tests/experiments/2026-11-25-v0.1.10-native.md).
The [machine plan](../tests/evaluation/plan.json) is fixed and hashed before a
measurement run. A failed target remains a failed result; changing the plan
requires a new run identity. Earlier functional timings are excluded from this
dataset.

## Controlled conditions

The reference is the Pi 5/8 GB/32 GB microSD host with pinned native K3s/Helm,
the lightweight platform and one retained healthy LAN demo. Additional monitoring
stacks remain disabled. The source/release/image/chart identities, actual OS,
memory/swap, free disk, temperatures, background workload and cache conditions
are retained. Measurements use only new owned fixtures. Existing applications,
accounts, history and installation data are preserved.

Three repetitions alternate API/manual Helm order. Both methods use the same
released demo image, first-party chart, values, probes, resource settings and
readiness/actual-HTTP completion condition. Application/release IDs and LAN
hostnames differ by owned fixture; the other workload settings match. The manual chart is copied from the
release's exact Git source; stored Helm chart/configuration identities are checked
against the API deployment. Baseline preparation and authentication precede timing.
Both methods run on the Pi to avoid attributing SSH/client startup overhead to one
method. Gateway request observations remain separate from backend readiness.

## Recorded measurements

- Cached v1 deployment, v2 update, unhealthy-v3 detection/restoration and deletion
  are repeated for both methods. Manual rollback is explicitly scripted after
  Helm failure; platform compensation is automatic. Both failure waits use 300 s.
- Idle and loaded CPU, memory/current and lifetime peak, throttling, OOM/restart
  counters, disk headroom and temperature are retained. The [Linux cgroup v2 interface](https://docs.kernel.org/admin-guide/cgroup-v2.html)
  defines CPU usage in microseconds and memory.peak as a lifetime maximum; it is
  not a peak confined to the load interval. Raw cgroup counters are
  preferred to coarse metrics-server samples for short-lived peaks. Samples are
  timestamped; missing/stale samples are not treated as zero usage.
- Fixed-rate HTTP observations retain every status/error/latency and the request
  schedule. Availability during update and broker unavailability is recorded
  separately from steady-state load; first successful requests alone cannot
  establish uninterrupted service. The earlier route-removal and request-timeout
  observations are explicit reproducibility cases.
- Public-image pull-required and cached runs are separated. Removing an image
  reference does not prove all layers are absent. Image/content inventory, actual
  pull evidence and shared/cached content are retained; partial-cache pulls must
  not be labelled fully cold. A shared-store/global prune is prohibited. Any
  optional reset targets only the exact unused owned benchmark image after live
  Pod/application/history checks, with explicit opt-in.

The resource-collector amendment is fixed before its replacement series: 32
bounded outstanding requests replace four after client saturation caused three
unsent requests. The invalid attempt is retained. The rate, duration and acceptance
targets are unchanged; all resource scenarios are repeated under the amended plan.

The collection candidate was 25m/32Mi requests and 250m/128Mi limits. The
[retained results](../tests/experiments/2026-11-26-core-evaluation.md) derive the
source reference preset of 25m/48Mi requests with unchanged limits. Collection
plans retain their original candidate values; they are not rewritten after measurement. A resulting reference profile must retain
headroom above observed peaks, satisfy zero OOM/restarts and state the tested
workloads/rate. Two small HTTP applications cannot justify sizing arbitrary apps.
The measured defaults and hook presence passed [published v0.1.11 checks](../tests/experiments/2026-11-27-v0.1.11-native.md)
under a separately retained plan fixed before collection. Application-rollout HTTP
failed its zero-error target (373/375); this remains a failed result requiring
diagnosis/correction. The original calibration plan/results are unchanged; human
measurements remain pending.

## Targets and interpretation

The machine plan defines targets before collection: cached deployment/update
within 30 s, verified restoration within 60 s after the common failure deadline,
100% steady-state request success and p95 below 500 ms at 20 requests/s, no OOM/
container restart, temperature below 80 °C and at least 6 GiB free disk. These
are prototype acceptance targets, not guarantees or measured conclusions.

Raw per-run values precede aggregates; sample counts and range accompany medians
and percentiles. With three repetitions, results are descriptive for this host,
not a population estimate. Network/cache conditions, client overhead, background
load, thermal throttling and missing evidence limit interpretation. Human time
and scripted execution time are never combined.

## Human timed walkthrough

The administrator agreed to perform a short timed walkthrough. System benchmarks
can proceed independently; human active operator time remains pending until the
completed record is supplied. The procedure and recording template are maintained
alongside the evaluation tooling. The human trial compares equivalent prepared
Helm and UI/API tasks, states its operator-step definition and pauses active time
during passive deployment waits. No participant observation or usability result
is inferred from browser automation.
