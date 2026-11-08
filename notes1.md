# Presentation 1 — speaker notes

Audience file: `presentation1.pptx`. Reviewed: 2026-11-08.
This deck and its speaker text form a presentation snapshot.
Current capabilities and operating procedures are maintained
in [implementation status](docs/status.md) and the [release guide](release/README.md).
The deck has **20 slides**: 19 presented slides and a references appendix.
The target schedule is **13 minutes 15 seconds**, within the required 10–15
minute window. Approximately 2,100 words require a steady speaking pace and
brief pauses; timing remains subject to rehearsal. Slide 20 is an appendix and
has no allocated speaking time. Updated notes are embedded in Presenter View.

| Slide | Topic | Target | Cumulative |
| --- | --- | --- | --- |
| 1 | Title and central idea | 25 s | 0:25 |
| 2 | Motivation — an image is only the start | 40 s | 1:05 |
| 3 | The problem and operator effort | 40 s | 1:45 |
| 4 | Use cases and visibility | 45 s | 2:30 |
| 5 | Why not just use the cloud? | 50 s | 3:20 |
| 6 | State of the art — overview | 50 s | 4:10 |
| 7 | State of the art — comparison matrix | 45 s | 4:55 |
| 8 | What is different / planned contribution | 45 s | 5:40 |
| 9 | Target user experience | 40 s | 6:20 |
| 10 | Single-cluster architecture | 55 s | 7:15 |
| 11 | Deployment and recovery workflow | 50 s | 8:05 |
| 12 | Network exposure and responsibilities | 40 s | 8:45 |
| 13 | Current implementation and next stages | 50 s | 9:35 |
| 14 | Evaluation method | 45 s | 10:20 |
| 15 | Example workloads | 35 s | 10:55 |
| 16 | Example — deploying an AI agent service | 40 s | 11:35 |
| 17 | Example — why self-host agents | 30 s | 12:05 |
| 18 | Scope and limitations | 45 s | 12:50 |
| 19 | Closing message / goal | 25 s | 13:15 |
| 20 | Reference appendix | — | — |

The examples on slides 15–17 can be shortened during rehearsal. The factual
implementation boundary is maintained in [status.md](docs/status.md); target
architecture and recovery diagrams do not establish completed acceptance.

## Slide 1 — Title and central idea

This project concerns the design and implementation of a Kubernetes platform for
containerized application deployment. Its central idea is to make applications
easier to operate on infrastructure controlled by an administrator — a small
Raspberry Pi, a rented Linux server, or an organisation's internal environment.
The talk covers the problem, why not simply use a public cloud, how existing
solutions compare, the proposed architecture, what is already built, and how the
result will be evaluated.

## Slide 2 — Motivation

A container image packages an application, but it does not complete its
deployment. The operator still has to configure the listening port, supply
variables and credentials, arrange network access, and decide how health is
checked. The work continues after the first successful start: an update can fail,
storage can fill, and the previous version may need restoring. The objective is a
consistent workflow for these routine operations, while infrastructure control
stays with the administrator.

## Slide 3 — The problem, quantified

Before proposing a solution it helps to see where the manual effort actually
goes. Deploying an image on Kubernetes by hand is not one action; it is a
repeated sequence. The administrator prepares supporting objects — a namespace, an image-pull
secret, configuration and secrets. The workflow defines several manifests: Deployment,
Service, Ingress, probes, resource limits. The resources are applied and rollout status, events and logs are inspected to
judge success. And when an update fails, you
diagnose it and roll back by hand, with nothing recording what happened. This
effort scales with every application and every update, and because it is manual
repeatability and auditability require explicit automation and recordkeeping. The container ecosystem removed
packaging friction but not this operational friction [3]. That gap is the target.

## Slide 4 — Use cases and visibility

Three settings illustrate the intended use. First, an institution that wants
applications on infrastructure it controls — a bank's internal development or
test network, where an internal tool may have no reason to be publicly reachable;
a pilot scenario, not a claim of production banking readiness. Second, personal
hardware: the reference Raspberry Pi 5 with 8 GB RAM, 32 GB storage and a 64-bit
OS, hosting private home tools or a published service. Third, a rented Linux VPS —
the same application model on a machine with a public IP. Hosting location and
network visibility are separate decisions; management access stays restricted in
every case.

## Slide 5 — Why not just use the cloud?

A fair question for any self-hosting project. The honest answer is that managed
cloud is excellent for many workloads; this project targets a different operating
point, on four axes. **Economics:** managed Kubernetes carries recurring
per-cluster control-plane fees, data-egress charges and per-service minimums,
while a single small always-on application runs on fixed, low-cost hardware the
operator already owns. **Data sovereignty:** keeping applications and data on
controlled infrastructure is the enabling condition for the institutional
internal-network case, where public reachability may be neither needed nor
permitted. **Portability:** one application model spans a Pi, a VPS and
institutional hardware without re-architecting around one provider's managed
services [5, 11]. **Transparency:** a fully visible stack is what makes the
thesis's reproducible, measured evaluation possible [4, 9]. The point is not that
cloud is wrong — it is the right tool at larger scale; this work studies the
small, operator-controlled case.

## Slide 6 — State of the art (overview)

The landscape already contains strong solutions. Kubernetes supplies the
orchestration mechanisms — desired state, scheduling, service discovery, scaling,
recovery [1]. K3s packages Kubernetes and supporting components for smaller
installations [2]. Coolify focuses on delivering applications to
operator-controlled hosts, including from prebuilt images [3]. Portainer provides
visual management of container environments with form-based Kubernetes deployment
[4]. OpenShift is a broad enterprise application platform built on Kubernetes [5].
These overlap with the project, so self-hosting and a deployment form are not new
inventions.

## Slide 7 — State of the art (comparison matrix)

This matrix makes the positioning explicit. Every listed tool supports
self-hosting, and several offer a deployment UI — so neither distinguishes this
work. The distinguishing combination, in the highlighted row, is three things
together: a deliberately narrow single-administrator scope, an explicit
modest-hardware target, and — crucially for a thesis — a measured comparison
against the manual baseline, which none of the others set out to provide. Overlap
is expected and intended; the contribution is the specific combination and its
evaluation.

## Slide 8 — What is different / planned contribution

The differentiation is a deliberately limited scope joined to a measurable
implementation. The first product serves one administrator operating one cluster.
Applications begin as existing container images, which avoids adding a build
service to the first version. Routine actions are expressed as application
operations rather than hand-edited Kubernetes resources. Each operation retains
its inputs, status and resulting revision, so recovery can be verified rather than
inferred from a successful command. The Raspberry Pi gives a concrete constraint
for evaluating memory, storage and deployment behaviour.

## Slide 9 — Target user experience

The intended experience begins with installation on a supported Linux host. After
administrator setup, the main input is a container image reference with the
application's port, health endpoint and configuration. The platform validates the
request, applies the supported chart, and observes whether the workload becomes
ready. The result is an application URL and an understandable status, with logs
and deployment history for investigation. The same interface then supports
updates, manual replica changes for compatible applications, and recovery. This
reduces repeated infrastructure work within an explicitly supported application
contract — it does not remove application-specific responsibilities such as
choosing suitable images or managing database migrations.

## Slide 10 — Single-cluster architecture

Both the platform and the deployed applications run in the same K3s cluster. The
reference topology is one Linux host, so that machine runs the control plane and
the workloads. The platform lives in its own namespace: a Go process serving the
UI and API, executing deployment operations through the Helm library, and storing
metadata in SQLite on persistent storage. Managed applications occupy another
namespace, a separate release per application. The blue path is management — the
broker asks the Kubernetes API to create or change workloads and observes their
state. The green path is application traffic — users reach a permitted ingress
route, then the Service and Pods; the broker is not in that path. Existing
applications keep running if the broker is unavailable, provided their
dependencies are healthy. Namespaces are not physical isolation, and a single-host
failure affects the whole installation.

## Slide 11 — Deployment and recovery workflow

A deployment is an operation with a persisted request and recorded outcome.
Validation checks the supported configuration, then the worker freezes the image
reference before applying resources through Helm. Kubernetes creates the Pods
and reports readiness. The demo provides healthy v1/v2 and deliberately unhealthy
v3. In the manual baseline, healthy v2 kept serving during the failed v3 rollout;
explicit Helm rollback then verified restoration and removal of failed Pods.
The broker's healthy lifecycle is verified separately. Automatic restoration after
an unhealthy update remains incomplete: an unresolved mutation currently enters
NeedsAttention and retains its lock. A successful HTTP response alone cannot
prove that the intended revision was restored.

## Slide 12 — Network exposure and responsibilities

The operating model begins with private access. Administrators connect through a
restricted path — a LAN or VPN — and internal applications can stay private.
Public access is an additional choice for a selected application, with an explicit
hostname and HTTPS; the management interface stays out of that public path. This
separation matters on a home network and on a VPS with a public address, and it
requires real network restrictions, not merely a private-looking hostname. The
planned controls include scoped permissions, protected credentials, and backup
and recovery. Institutional adoption would add identity integration, availability
objectives and security review.

## Slide 13 — Current implementation and next stages

The local prototype includes the Go demo, application and platform Helm charts,
a durable Go broker, and a Svelte administrator UI. The API lifecycle has been
verified on Docker-based K3s: setup, deployment, update, manual scaling, rollback
and deletion. Stopping and rebuilding retained the administrator session and
application history. These are local functional observations, not Pi performance
or complete M0–M4 acceptance. Current work must align UI/API fields, correct
restart, verify interrupted and failed-rollout recovery, and complete private
registry authentication. Native Pi/LAN evaluation follows. Application volumes,
public HTTPS, production installation, releases and monitoring remain later
capabilities. The current status and verification reports distinguish code that
exists from behavior that has been accepted.

## Slide 14 — Evaluation method

Evaluation should establish whether the platform improves the selected workflow,
not merely that a UI exists. The baseline is the same application deployed
manually with Helm on the same hardware. Operator effort is measured with a
defined step count and active operator time. Deployment measurements record time
to readiness, separating cached images from downloads. Failure experiments measure
detection and recovery, including whether the desired version and healthy state
are actually restored. CPU, memory and disk use indicate the cost of running the
platform alongside applications [9]. Runs are repeated, with versions,
configuration, hardware and cache state recorded; numerical targets are set before
the relevant runs.

## Slide 15 — Example workloads

To make the value concrete, here is what an administrator would actually put on
the platform. The current contract is a compatible stateless Linux HTTP image with a suitable
port, health behavior and restricted runtime requirements. Candidate uses include: a self-hosted AI agent or LLM-tooling
service kept on controlled infrastructure so prompts and data stay in-house; an
internal web tool such as a dashboard or wiki, private on the LAN; an automation
or webhook endpoint reacting to Git, CI or chat events; a monitoring or status
page; a stateless API backend; or a deliberately published personal site. The
common thread is an operator who wants the service on infrastructure they control,
with a managed lifecycle instead of hand-written manifests. The next slide walks
through one of these end to end.

## Slide 16 — Example: deploying an AI agent service

Here is one example end to end: deploying a self-hosted AI agent service. Step
one, describe it — paste the agent's container image, confirm the listening port
and the health endpoint. Step two, configure — set non-sensitive environment
values, and add the model or provider API key as a versioned secret, which the
platform stores write-only so it never appears in history, logs or responses. Step
three, deploy — the platform freezes the image digest, applies the chart and
watches readiness, accepting the request first and verifying the workload
afterwards. Step four, operate — you get a private LAN URL, live logs and status,
and a tracked update to a new agent version with recorded history and recovery.
The result is a controlled agent service with a managed lifecycle, and the data
stays on the operator's own cluster.

## Slide 17 — Example: why self-host agents

Agent workloads are worth calling out because they sharpen the very reasons behind
this project. First, data stays in-house: prompts, documents and tool inputs are
often sensitive or regulated, and self-hosting the agent avoids sending them to an
external platform — the same data-sovereignty argument from earlier. Second,
predictable cost: an always-on automation agent is a steady small workload that
suits fixed-cost hardware better than per-request cloud pricing. Third,
portability and composition: an agent, a vector store and a small web UI can run
as separate connected applications in one cluster, movable between a Pi, a VPS and
institutional hardware. Fourth, an honest boundary — local GPU model serving needs
more capable hardware than the reference Pi, so the platform targets agent and
tooling services that run as ordinary HTTP containers, not heavy model hosting.
This ties the concrete examples back to the thesis: a managed lifecycle for
controlled, measurable self-hosting.

## Slide 18 — Scope and limitations

A credible thesis states its limits. The prototype is explicitly not highly
available: one host runs the control plane and the workloads, so host or storage
failure affects everything, and replicas on one node are not fault tolerance. It
is not multi-tenant — one trusted administrator, not a security boundary against
hostile tenants. It is not a build service: applications start from existing
images. And it is not a managed cloud: no autoscaling or distributed storage, and
OS and K3s maintenance stay with the administrator. These boundaries are
deliberate; they match the target users and keep the design implementable and
measurable within the thesis.

## Slide 19 — Closing message

The goal is a practical application platform that makes supported deployment tasks
easier while keeping infrastructure decisions with the administrator, across small
personal hardware, a rented VPS and institutional infrastructure. Its value is
assessed through an implemented workflow and reproducible measurements. The
immediate next step is the administrator web interface and the reliable-lifecycle
operations on top of the durable backend, then evaluation on the reference
hardware.

## Slide 20 — Reference appendix

Reference slide for questions and source checking; no additional speaking time is
allocated.

### Primary sources (product documentation)

- [1] https://kubernetes.io/docs/concepts/overview/
- [2] https://docs.k3s.io/
- [3] https://coolify.io/docs/core/what-is-coolify
- [4] https://docs.portainer.io/user/kubernetes/applications/add
- [5] https://www.redhat.com/en/technologies/cloud-computing/openshift/container-platform
- [6] https://docs.k3s.io/installation/requirements
- [7] https://github.com/piotrek1459/k8s-internal-platform

### Peer-reviewed and research sources

- [8] Burns, Grant, Oppenheimer, Brewer, Wilkes. *Borg, Omega, and Kubernetes.*
  ACM Queue / Communications of the ACM, 2016. — the lineage and rationale of
  Kubernetes.
- [9] Felter, Ferreira, Rajamony, Rubio. *An Updated Performance Comparison of
  Virtual Machines and Linux Containers.* IEEE ISPASS, 2015. — container overhead
  versus virtual machines, supporting the modest-hardware argument.
- [10] Morabito. *Virtualization on Internet of Things Edge Devices With Container
  Technologies.* IEEE Access, 2017. — containers on constrained/edge devices such
  as the Raspberry Pi.
- [11] Kratzke, Quint. *Understanding cloud-native applications after 10 years of
  cloud computing — A systematic mapping study.* Journal of Systems and Software,
  2017. — cloud-native practice, portability and lock-in.
- [12] Bernstein. *Containers and Cloud: From LXC to Docker to Kubernetes.* IEEE
  Cloud Computing, 2014. — the container-to-orchestration progression.

Project documents: `docs/status.md` · `docs/app-plan.md` · `docs/architecture.md` ·
`docs/milestones.md` · dated verification reports and the broker/UI source.
