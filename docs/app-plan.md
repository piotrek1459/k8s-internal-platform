# Application platform plan (index)

Updated: 2026-10-13. Status: proposed implementation plan, with confirmed
requirements recorded below. This document accompanies
[project-outline.md](project-outline.md), which preserves the original thesis
scope and supplied university requirements. This is a plan for building the
software; it does not contain thesis chapters or claim completed implementation.

This file is the **index**. The detailed plan is split across focused documents:

| Document | Covers |
| --- | --- |
| [architecture.md](architecture.md) | System architecture: deployment targets, scope and delivery sequence, cluster layout, components, traffic paths, databases, permissions, and UI structure. |
| [api-and-data.md](api-and-data.md) | Application contract (inputs, health, scaling, storage), platform data model, and the HTTP API with examples. |
| [networking-and-security.md](networking-and-security.md) | Visibility modes (LAN first, public later), network isolation and verification, credentials, secrets, and backup/restore. |
| [operations.md](operations.md) | Reference hardware and limits, installer workflow, upgrade/uninstall, CI/CD, deployment execution/recovery/reconciliation, and observability profiles. |
| [milestones.md](milestones.md) | Implementation milestones M0–M8, task checklists, risks and scope boundaries, end-to-end workflows, validation and evaluation, open decisions, and source layout. |

## Product goal

The platform shall support installation on an administrator-controlled Linux
machine. A web UI shall accept a container image and the required application
configuration, then initiate deployment on that infrastructure. The platform
shall manage deployment, configuration, updates, recovery, and supported scaling.
Public application access over HTTPS is a planned capability.

The reference outcome is the public deployment of a separate application from
its published container image on the host running the platform.

**The platform itself and every application it deploys run in the same Kubernetes
cluster.** K3s is the proposed Kubernetes distribution. The product gives the
administrator a managed application environment in an administrator-controlled
cluster, with application operations available through the platform UI/API after
installation.

The **Raspberry Pi 5 is the reference host for initial implementation**. A rented,
internet-facing Linux server (VPS or dedicated, typically AMD64) is a planned
additional deployment target. The architecture is independent of the host model;
target-specific validation remains required. See the
[deployment-targets note](architecture.md#deployment-targets-host-agnostic-by-design).

## Confirmed decisions

| Topic | Confirmed requirement |
| --- | --- |
| Deployment targets | Raspberry Pi 5 is the initial reference host; a rented internet-facing Linux server (VPS or dedicated, typically AMD64) is a planned additional target. |
| Reference host | Raspberry Pi 5, 8 GB RAM, 32 GB microSD, 64-bit Raspberry Pi OS. |
| Operator | One administrator: the person installing the platform. |
| Cluster topology | One shared cluster hosts both the platform and managed applications; initially one node runs the control plane and workloads. |
| Interface | A web UI is a core part of the product. |
| Application source | Public and private container images. |
| Operations | Application deployment, inspection, updates, scaling, and supporting recovery and lifecycle operations. |
| Traceability | Persistent configuration, deployment history, operations, and an audit trail. |
| Initial networking | Initial development and demonstration on the LAN; domain acquisition is deferred. |
| Final networking | Public application access on the host through a connection method selected after network validation: direct ingress or an outbound tunnel. |
| Distribution | Installable GitHub releases for both ARM64 and AMD64. |
| Working language | English for project documentation, code, and UI. |

The following are **proposed design choices**, subject to validation during the
initial implementation experiments: K3s, a Go modular monolith, a React/TypeScript
UI, SQLite for platform metadata, and a platform-owned Helm chart for applications.

The future application's own database, uploads, background jobs, and traffic
requirements are still unknown. They are separate from the platform's database.

## Scope at a glance

Full scope (M0–M8) remains the target. Milestones **M0–M4 define the core prototype**:
deployment, observation, updates, recovery, and durable platform state on the
reference host. This scope includes a required minimum evaluation of implemented
lifecycle operations: functional and failure scenarios, comparison with the
manual baseline, and measured deployment time and resource consumption. The
[minimum core evaluation](milestones.md#minimum-core-evaluation-m4) specifies the
required evidence.

Milestones **M5–M8 define planned extensions** for application data persistence, public
hosting, distribution, and extended observability and evaluation. These milestones
remain necessary for the applicable complete-product and release claims; they are
not prerequisites for starting the core prototype. Persistent platform metadata
and immutable application secret versions belong to the core. Application data
volumes are introduced in M5. Academic sufficiency must be assessed against the
agreed thesis requirements and cannot be inferred from a milestone label.

The specification is sufficiently defined to begin **M0 and M1**. Progression to
later milestones depends on the recorded acceptance evidence for their
prerequisites, as specified in the
[implementation entry criteria](milestones.md#implementation-readiness-and-entry-criteria). This readiness
assessment concerns the implementation plan; no implementation or runtime
validation is claimed.
