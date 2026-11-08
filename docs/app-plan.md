# Application platform plan (index)

Updated: 2026-11-08. Status: local core prototype implemented in part;
UI/API alignment, reliable recovery and reference-host acceptance remain open. This document accompanies
[project-outline.md](project-outline.md), which preserves the original thesis
scope and supplied university requirements. The plan records target requirements and does not contain thesis chapters.
[Implementation status and next work](status.md) records current capabilities,
verification evidence and known gaps.

The broker, Svelte UI, application/platform charts and unified local workflow are
implemented. The healthy local API lifecycle has been verified. The
[local development guide](local-development.md) covers current execution and the
separate manual Helm baseline. No M0–M4 milestone is declared fully accepted on
the reference hardware.

This file is the **index**. The detailed plan is split across focused documents:

| Document | Covers |
| --- | --- |
| [status.md](status.md) | Current implementation, known limitations, verification boundaries and immediate next work. |
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
cluster.** K3s is the selected Kubernetes distribution; local development uses k3d. The product gives the
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
| Initial networking | Current development uses loopback forwarding. LAN/HTTPS is the next deployment target; domain acquisition remains deferred. |
| Final networking | Public application access on the host through a connection method selected after network validation: direct ingress or an outbound tunnel. |
| Distribution | Installable GitHub releases for both ARM64 and AMD64. |
| Release initiation | Manual dispatch with a source branch (default `main`) and version. The selected branch HEAD is pinned for validation/builds; successful gates create the tag and release. PR, branch and tag pushes do not publish. |
| Working language | English for project documentation, code, and UI. |

The implemented choices are K3s/k3d, a Go modular monolith, SQLite metadata,
first-party Helm charts, and a Svelte 5/SvelteKit/TypeScript/Tailwind static UI.
The [UI decision](decisions/m3-ui-stack.md) supersedes the original React proposal.
Reference-host resource presets, public access mode and production installation
procedures still require validation.

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

The next implementation work aligns UI/API contracts, corrects restart and
recovery semantics, completes private image support, and extends core failure
acceptance. Manual releases and nightly/API/browser acceptance are already implemented.
The [status report](status.md#next-implementation-order) defines that order.
[Milestone acceptance](milestones.md#implementation-readiness-and-entry-criteria)
continues to require evidence for hardware, networking, failure recovery and
evaluation; existing code alone does not satisfy those requirements.
