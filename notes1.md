# Presentation 1 — speaker notes

Audience file: `presentation1.pptx`.
The deck has **20 slides** (19 content + a references appendix), paced for
**15–17 minutes**. Slide 20 is a reference appendix. Timing guidance is not shown
on the audience slides. Advance slides manually. Rehearse once with a timer; the
script is roughly 2,400 words, so keep a steady pace and brief pauses around the
diagrams to stay within the window. These notes are also embedded in PowerPoint
Presenter View.

| Slide | Topic | Target | Cumulative |
| --- | --- | --- | --- |
| 1 | Title and central idea | 30 s | 0:30 |
| 2 | Motivation — an image is only the start | 55 s | 1:25 |
| 3 | The problem, quantified | 55 s | 2:20 |
| 4 | Use cases and visibility | 60 s | 3:20 |
| 5 | Why not just use the cloud? | 70 s | 4:30 |
| 6 | State of the art — overview | 65 s | 5:35 |
| 7 | State of the art — comparison matrix | 60 s | 6:35 |
| 8 | What is different / planned contribution | 60 s | 7:35 |
| 9 | Target user experience | 55 s | 8:30 |
| 10 | Single-cluster architecture | 70 s | 9:40 |
| 11 | Deployment and recovery workflow | 65 s | 10:45 |
| 12 | Network exposure and responsibilities | 55 s | 11:40 |
| 13 | Current implementation and next stages | 60 s | 12:40 |
| 14 | Evaluation method | 60 s | 13:40 |
| 15 | Example workloads | 55 s | 14:35 |
| 16 | Example — deploying an AI agent service | 60 s | 15:35 |
| 17 | Example — why self-host agents | 50 s | 16:25 |
| 18 | Scope and limitations | 45 s | 17:10 |
| 19 | Closing message / goal | 30 s | 17:40 |
| 20 | Reference appendix | — | — |

If you need to land closer to 15 minutes, the examples section (slides 15–17) is
the natural place to trim: keep the walk-through (16) and summarise 15 and 17
briefly.

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
repeated sequence. You prepare supporting objects — a namespace, an image-pull
secret, configuration and secrets. You author several manifests: Deployment,
Service, Ingress, probes, resource limits. You apply them and then read rollout
status, events and logs yourself to judge success. And when an update fails, you
diagnose it and roll back by hand, with nothing recording what happened. This
effort scales with every application and every update, and because it is manual
it is neither reproducible nor auditable. The container ecosystem removed
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

A deployment is modelled as an operation with a recorded outcome. The
administrator submits an image and configuration; validation checks that the
request is supported; the operation is persisted before execution begins. The
worker applies the desired resources and observes the resulting workload rather
than treating acceptance as completion. The version sequence shows why this
matters: v1 deployed, v2 a healthy update, v3 deliberately failing readiness.
Healthy v2 Pods keep serving while the v3 update fails, so recovery must verify
more than an HTTP response — it confirms the restored desired state, the release
outcome and removal of the failed Pods. This distinction is reflected in the
local baseline tests, and the durable backend now records every such operation.

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

The project has a verified local foundation and now a durable backend. A small Go
application provides healthy versions and a deliberately unhealthy update; a
reusable Helm chart deploys it; and the test workflow exercises installation, a
healthy update, a failed rollout, explicit rollback and route isolation. Built on
that, the M2 backend adds the Go broker: a management API, a durable operation
queue, authentication, image inspection, versioned secrets, in-process Helm
integration and a status observer. This evidence comes from a local Docker-based
K3s cluster; it does not yet establish Raspberry Pi performance. The next stages
are the administrator web UI (M3) and the reliable-lifecycle operations — rollback
and recovery (M4) — followed by persistent data, public HTTPS, releases and Pi
validation.

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
the platform. The only requirement is a long-running HTTP service shipped as a
container image, so the range is wide: a self-hosted AI agent or LLM-tooling
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

Project documents: `app-plan.md` · `architecture.md` · `milestones.md` · local
verification reports and the M2 broker module.
