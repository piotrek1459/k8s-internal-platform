# Private rollout routing correction — 2026-11-27

The private controller in current source forwards to Kubernetes Service ClusterIPs
instead of rebuilding a Pod-address upstream list for each replacement.
[PR #25](https://github.com/piotrek1459/k8s-internal-platform/pull/25) contains the
setting and stronger rollout regression. The [machine summary](2026-11-27-rollout-routing.json)
and hashed raw artifacts retain every controlled result.
**A following manual release and published Pi validation remain required.**
The human walkthrough remains last; no human measurements are inferred here.

## Observed failure and mechanism

The preceding unmodified [v0.1.11 validation](2026-11-27-v0.1.11-native.md) retained
373/375 successful requests during application update and later broker replacement.
The 502 and timeout occurred before the broker outage. That failed trace is unchanged.

The Pi was inspected before this study: original application version 7 and HTTPS
v1 were healthy, and the private proxy reported pinned Traefik 3.7.19 on ARM64.
The broker binary and embedded application chart remained published v0.1.11.
Temporary proxy arguments enabled a loopback-only observation API, selected access
log fields and the controlled provider/routing options. Deployment UID, resource
version and expected-template guards protected each change and restoration.

The first instrumented default-policy run passed 550/550 requests. Its simultaneous
Pod/EndpointSlice watch, process log and 100 ms proxy-configuration samples showed:

- Kubernetes marked the old endpoint terminating and not Ready.
- The proxy took approximately **4.682 seconds** to publish its fenced state.
- The old process started shutdown approximately **0.210 seconds** after that observation.

This run did not fail, but showed little margin within the five-second hook.
Instrumentation can affect timing. The configuration observations have sampling/collection delay; these values are
not an exact internal propagation bound. No equivalent simultaneous trace exists
for the original failed v0.1.11 run, so its exact internal timing remains unproven.

To isolate the failure mechanism, endpoint-provider refresh was deliberately
delayed to **15 seconds** in both routing policies. The Pod-address control lost
90/550 requests. Proxy access logs independently identify **90 failed responses
targeting the stopped old Pod address** while the replacement was available.
The configuration trace retained that old upstream after process shutdown.
This reproduces the shutdown/routing race without changing the application image,
shutdown hook or request timeout.

## Controlled comparisons

Each case used a fresh owned Go demo fixture, one replica, the published resource
defaults and five-second hook. Two separate verified HTTPS clients each sent
5 requests/s for 55 seconds: one reused connections, the other requested connection
closure. Both used normal CA/hostname verification and a three-second timeout.
Preparation and route convergence preceded the traffic interval. Client scheduling
and complete raw request counts were checked; no failed requests were excluded.

| Case | Proxy endpoint delay | Routing | Actions | Successful requests |
| --- | --- | --- | --- | --- |
| Instrumented default | Default | Pod addresses | Update | 550/550 |
| Delayed control | 15 s | Pod addresses | Update | **460/550** |
| Delayed candidate | 15 s | Service ClusterIP | Update | **550/550** |
| Default candidate | Default | Service ClusterIP | Update, rollback, restart | **550/550** |
| Delayed candidate cycle | 15 s | Service ClusterIP | Update, rollback, restart | **550/550** |

The candidate passed **1,650/1,650** requests across its three cases. Each control
client observed one HTTP 502 and 44 timeouts; scheduling remained valid. Access
logs explain the corresponding server-side failures rather than attributing them
to client saturation. The final candidate case records the actual Service UID/
specification; every sampled upstream equals its ClusterIP and Service port,
despite the Pod and EndpointSlice changes.

These are controlled native experiments with temporary proxy options. The imposed
15-second delay is a fault condition, not the production default. The default
instrumented case is retained as a pass; the original v0.1.11 trace and delayed
control remain failures. The controlled result establishes the stale-backend
failure mechanism and the candidate's independence from endpoint-provider refresh
in this sample; it does not prove every internal cause of the earlier uninstrumented
trace or provide a general availability guarantee.

## Source change and regression

The dedicated private controller sets
`--providers.kubernetesingress.nativelbbydefault=true`. Its stable upstream is the
Service ClusterIP and Kubernetes selects available Pod endpoints. This removes
the proxy backend-list refresh from the application replacement path. The option
is defined in the [Traefik provider documentation](https://doc.traefik.io/traefik/reference/install-configuration/providers/kubernetes/kubernetes-ingress/)
and [pinned implementation](https://github.com/traefik/traefik/blob/v3.7.19/pkg/provider/kubernetes/ingress/kubernetes.go).

The application hook remains five seconds. Existing backend connections may serve
the prior version during draining. Service balancing operates on connections;
proxy connection reuse can distribute traffic unevenly across replicas. This
change does not introduce an application-request retry policy or establish results
for arbitrary images, long requests or node failure.

The source private HTTPS regression delays endpoint refresh to 15 seconds, observes
actual update and rollback traffic, waits until the previous Pod UIDs are absent
and includes a later observation interval. Every failed response remains fatal.
The previous observer ended shortly after rollback readiness and could miss the
last old process's shutdown. Unit regressions distinguish a terminating Pod from
an absent Pod and reject retirement timeout. Chart checks verify native routing
within the private class/namespace scope and refuse leaked observation/test flags.

Full [source E2E run 45993196980](https://github.com/piotrek1459/k8s-internal-platform/actions/runs/45993196980)
passed **27 tests, zero failures and zero skips** at
`826b92e45cf5730c52a1e1b37d3159c1fff5fc21`. Its downloaded artifact SHA256,
JUnit and rollout trace were verified: **116/116 actual HTTPS requests succeeded**
through the delayed-provider update/rollback and old-Pod retirement interval.
The stored artifact digest and selected results are retained in `source-ci/`.
Local Python regressions passed **104 tests**; chart rendering, negative cases and
shell syntax passed. Five fast CI checks passed on the source implementation head.
Published-release acceptance remains separate.

## Restoration and remaining work

Every fixture was deleted through the tracked API and waited for complete Pod
removal. The original private Deployment UID/template and application/version/
revision/access identities were restored and verified, with actual HTTPS v1.
The temporary loopback API, input credentials, SSH master and sleep inhibitor were
removed. One healthy owned LAN demo remains on the original published installation.
Passwords, cookie material and private keys are excluded from these artifacts.

The next sequence is administrator merge/manual release, validation of that exact
published release on the Pi, then the [human Helm/UI walkthrough](../evaluation/HUMAN.md).
Issue #16 and M0–M4 remain open until those criteria pass. Application volumes,
public hosting, upgrades and extended monitoring remain separate M5–M8 work.
