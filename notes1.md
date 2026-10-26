# Presentation 1 — speaker notes

Audience file: `presentation1.pptx`.
The 12 main slides are paced for **13 minutes**. Slide 13 is a reference appendix. Timing guidance is not displayed on the audience slides. Advance slides manually.
Rehearse once with a timer. The script contains approximately 1,400 words; use the per-slide targets and brief pauses around diagrams to keep the talk within 10–15 minutes. These notes are also embedded in PowerPoint Presenter View.

| Slide | Topic | Target | Cumulative |
| --- | --- | --- | --- |
| 1 | Title and central idea | 35 s | 0:35 |
| 2 | Problem and objective | 65 s | 1:40 |
| 3 | Use cases and visibility | 70 s | 2:50 |
| 4 | State of the art | 85 s | 4:15 |
| 5 | What is different | 75 s | 5:30 |
| 6 | Target user experience | 65 s | 6:35 |
| 7 | Single-cluster architecture | 80 s | 7:55 |
| 8 | Deployment and recovery workflow | 70 s | 9:05 |
| 9 | Network exposure and responsibilities | 60 s | 10:05 |
| 10 | Current implementation and next stages | 65 s | 11:10 |
| 11 | Evaluation method | 70 s | 12:20 |
| 12 | Closing message | 40 s | 13:00 |

## Slide 1 — Title and central idea

This project concerns the design and implementation of a Kubernetes platform for containerized application deployment. Its central idea is to make applications easier to operate on infrastructure controlled by an administrator. That infrastructure could be a small Raspberry Pi, a rented Linux server, or an organisation's internal environment. The presentation explains the problem, compares existing approaches, and describes the proposed architecture and its evaluation.

## Slide 2 — Problem and objective

A container image packages an application, but it does not complete its deployment. The operator still has to configure the listening port, supply variables and credentials, arrange network access, and decide how health will be checked. The work continues after the first successful start: an update can fail, storage can fill, and the previous application version may need to be restored. These tasks become repetitive when they are performed through individual commands and configuration files. The objective is therefore to provide a consistent workflow for routine application operations. Infrastructure control remains with the administrator, while the platform handles the supported deployment steps and presents the outcome in an understandable form.

## Slide 3 — Use cases and visibility

Three settings illustrate the intended use. The first is an institution that wants applications to run within infrastructure it controls. A bank's internal development or testing network is an example: an internal tool may have no reason to be publicly reachable. This is a potential pilot scenario, rather than a claim that the prototype is ready for production banking. The second setting is personal hardware. The reference device is a Raspberry Pi 5 with eight gigabytes of memory, thirty-two gigabytes of storage, and a sixty-four-bit operating system. It could host private home applications or an explicitly published service. The third setting is a Linux VPS. Renting the host does not require publishing every application. Hosting location and network visibility are separate decisions, and management access should remain restricted in each case.

## Slide 4 — State of the art

The existing landscape already contains strong solutions. Kubernetes supplies the orchestration mechanisms: desired state, workload scheduling, service discovery, scaling, and recovery. K3s packages Kubernetes and supporting components for smaller installations. Coolify focuses on delivering applications to servers controlled by the operator, including deployments from prebuilt images. Portainer provides visual management of container environments and already supports form-based Kubernetes application deployment. OpenShift provides a broader enterprise application platform built around Kubernetes. These products overlap with the proposed project's features, so self-hosting or a graphical deployment form cannot be presented as a new invention. The relevant question is how much of the operating environment is exposed to the administrator and which workflow the platform deliberately supports. This thesis selects a narrower scope: a small installation, one administrator, and a controlled application lifecycle that can be implemented and evaluated in detail.

Sources:

- https://kubernetes.io/docs/concepts/overview/
- https://docs.k3s.io/
- https://coolify.io/docs/core/what-is-coolify
- https://docs.portainer.io/user/kubernetes/applications/add
- https://www.redhat.com/en/technologies/cloud-computing/openshift/container-platform

## Slide 5 — What is different

The differentiation is the combination of a deliberately limited scope and a measurable implementation. The initial product serves one administrator operating one cluster. Applications begin as existing container images, which avoids adding a source-code build service to the first version. Routine actions are expressed as application operations rather than requiring the administrator to edit Kubernetes resources. Each operation is intended to retain its inputs, status, and resulting revision, so recovery can be verified rather than inferred from a successful command. The Raspberry Pi provides a concrete constraint for evaluating memory, storage, and deployment behaviour. Existing platforms offer overlapping capabilities; the thesis contribution is to implement this particular design, explain its trade-offs, and measure whether it improves the selected workflow compared with the same deployment performed manually.

## Slide 6 — Target user experience

The intended experience begins with installation on a supported Linux host. After administrator setup, the main input is a container image reference together with the application's port, health endpoint, and required configuration. The platform validates that request, applies the supported application chart, and observes whether the workload becomes ready. The result is an application address and an understandable status, with logs and deployment history available for investigation. The same interface then supports updates, manual replica changes for compatible applications, and recovery. This workflow is planned product behaviour. It does not remove application-specific responsibilities such as choosing suitable images, managing database migrations, or providing sufficient hardware. It reduces repeated infrastructure work within the explicitly supported application contract.

## Slide 7 — Single-cluster architecture

Both the platform and the deployed applications run in the same K3s cluster. The reference topology contains one Linux host, so that machine runs the control plane as well as the workloads. The platform resides in its own namespace. Its Go process serves the user interface and API, executes deployment operations through the Helm library, and stores metadata in SQLite on persistent storage. Managed applications occupy another namespace, with a separate set of resources for each release. The blue path represents management: the broker asks the Kubernetes API to create or change workloads and observes their state. The green path represents application traffic: users reach a permitted ingress route, then the application Service and Pods. The broker is not in this traffic path. Existing applications should continue during broker unavailability if their dependencies remain healthy. However, namespaces do not provide physical isolation, and a failure of the single host affects the shared installation.

## Slide 8 — Deployment and recovery workflow

A deployment is modelled as an operation with a recorded outcome. The administrator submits an image and configuration. Validation checks whether the request is supported, and the operation is persisted before execution begins. The worker applies the desired resources and observes the resulting workload rather than treating request acceptance as completion. The version sequence illustrates why this matters. Version one is deployed, version two is a healthy update, and version three deliberately fails readiness. Healthy version-two Pods can continue serving while that update fails. Recovery must therefore verify more than an HTTP response containing version two. It must also confirm the restored desired state, the release outcome, and removal of the failed Pods. That distinction is already reflected in the local baseline tests; automating the complete workflow through the broker remains the next implementation stage.

## Slide 9 — Network exposure and responsibilities

The operating model begins with private access. Administrators connect through a restricted network path, such as a LAN or VPN, and internal applications can remain private. Public access is an additional choice for a selected application, with an explicit hostname and HTTPS configuration. The management interface should remain outside that public routing path. This separation matters on a home network and on a VPS with a public address. It requires actual network restrictions, not merely a private-looking hostname. The planned controls also include scoped permissions, protected credentials, and backup and recovery procedures. An institution would have further requirements, including identity integration, availability objectives, and its own security approval. The project can demonstrate the design principles, while production institutional adoption requires additional validation.

## Slide 10 — Current implementation and next stages

The project already has a verified local foundation. A small Go application provides healthy versions and a deliberately unhealthy update. A reusable Helm chart deploys the application, and the test workflow exercises installation, a healthy update, a failed rollout, explicit rollback, and route isolation. The most recent run also verified deployment on a non-default application port and strengthened the recovery assertions. This evidence comes from a local Docker-based K3s cluster. It does not establish Raspberry Pi performance or completion of the full platform. The next stages are the Go API, durable deployment operations, administrator authentication, and the web interface. Application persistence, public HTTPS, installation releases, and validation on the target hardware follow the planned sequence.

## Slide 11 — Evaluation method

Evaluation should establish whether the platform improves the selected workflow, rather than relying on the existence of a user interface. The comparison baseline is the same application deployed manually with Helm on the same hardware. Operator effort is measured using a defined step count and active operator time. Deployment measurements record the time to readiness, separating cached images from downloads. Failure experiments measure detection and recovery, including whether the desired version and healthy workload state are actually restored. CPU, memory, and disk use indicate the cost of running the platform alongside applications. Runs should be repeated, with component versions, configuration, hardware, and cache conditions recorded. Numerical acceptance targets will be established before the relevant evaluation runs. Existing single-run local results provide functional evidence, while the Raspberry Pi measurements are still to be collected.

## Slide 12 — Closing message

The goal is a practical application platform that makes supported deployment tasks easier while keeping infrastructure decisions with the administrator. It should support both private applications and deliberately published services, across small personal hardware and suitable Linux servers. Its value will be assessed through an implemented workflow and reproducible measurements. The immediate next step is to automate the verified lifecycle through the Go broker and administrator interface, then evaluate the complete experience on the reference hardware.

## Slide 13 — Reference appendix

Reference slide for questions and source checking; no additional speaking time is allocated.

Sources:

- https://kubernetes.io/docs/concepts/overview/
- https://docs.k3s.io/
- https://coolify.io/docs/core/what-is-coolify
- https://docs.portainer.io/user/kubernetes/applications/add
- https://www.redhat.com/en/technologies/cloud-computing/openshift/container-platform
- https://docs.k3s.io/installation/requirements
- https://github.com/piotrek1459/k8s-internal-platform
