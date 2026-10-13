# Application platform plan (index)

Updated: 2026-10-13. Status: proposed implementation plan, with the author's
confirmed requirements recorded below. This document accompanies
[project-outline.md](project-outline.md), which preserves the original thesis
scope and supplied university requirements. This is a plan for building the
software; it does not contain thesis chapters or claim completed implementation.

This file is the **index**. The detailed plan is split across focused documents:

| Document | Covers |
| --- | --- |
| [architecture.md](architecture.md) | System shape: deployment targets, scope and order of delivery, cluster layout, components, traffic paths, databases, permissions, UI structure. |
| [api-and-data.md](api-and-data.md) | Application contract (inputs, health, scaling, storage), platform data model, and the HTTP API with examples. |
| [networking-and-security.md](networking-and-security.md) | Visibility modes (LAN first, public later), network isolation and verification, credentials, secrets, and backup/restore. |
| [operations.md](operations.md) | Reference hardware and limits, installer workflow, upgrade/uninstall, CI/CD, deployment execution/recovery/reconciliation, and observability profiles. |
| [milestones.md](milestones.md) | Execution document: milestones M0–M8 with task checklists and risk/cut-points, end-to-end workflows, test/evaluation plan, open decisions, and source layout. |

## Product goal

Build a platform that an administrator installs on their own Linux machine.
They open a straightforward web UI, supply a container image and the information
the application needs, and deploy it on their infrastructure. The platform
manages deployment, configuration, updates, recovery, and supported scaling.
An application can later be made publicly accessible over HTTPS.

The reference outcome is to build a separate application, publish its container
image, and run it publicly on the host running this platform.

**The platform itself and every application it deploys run in the same Kubernetes
cluster.** K3s is the proposed Kubernetes distribution. The product gives the
administrator a managed application environment on their own cluster: install
once, then operate applications through the platform UI/API.

The **Raspberry Pi 5 is the reference/starter host**, but the platform is not
Pi-specific: a rented, internet-facing Linux server (VPS/dedicated, typically
AMD64) is a supported later target for the same platform. See the
[deployment-targets note](architecture.md#deployment-targets-host-agnostic-by-design).

## Confirmed decisions

| Topic | Confirmed requirement |
| --- | --- |
| Deployment targets | Raspberry Pi 5 is the reference/starter host; a rented internet-facing Linux server (VPS/dedicated, typically AMD64) is a supported later target for the same platform. |
| Reference host | Raspberry Pi 5, 8 GB RAM, 32 GB microSD, 64-bit Raspberry Pi OS. |
| Operator | One administrator: the person installing the platform. |
| Cluster topology | One shared cluster hosts both the platform and managed applications; initially one node runs the control plane and workloads. |
| Interface | A web UI is a core part of the product. |
| Application source | Support public and private container images. |
| Operations | Deploy, inspect, update, and scale applications; add the lifecycle operations needed to make these reliable. |
| Traceability | Persist configuration, deployment history, operations, and an audit trail. |
| Initial networking | Develop and demonstrate on the LAN first. A domain will be purchased later. |
| Final networking | Support a publicly accessible application on the host. The public connection method is still to be selected (rented server favours direct ingress; home Pi may need a tunnel). |
| Distribution | Eventually provide installable GitHub releases for both ARM64 and AMD64. |
| Working language | English for project documentation, code, and UI. |

The following are **recommended design choices**, open to revision after the
first implementation experiments: K3s, a Go modular monolith, a React/TypeScript
UI, SQLite for platform metadata, and a platform-owned Helm chart for applications.

The future application's own database, uploads, background jobs, and traffic
requirements are still unknown. They are separate from the platform's database.

## Scope at a glance

Full scope (M0–M8) remains the target. **M0–M4 is the thesis-sufficient core**;
**M5–M8 are stretch** (persistence, public access, distribution, full evaluation).
Each milestone in [milestones.md](milestones.md) carries a risk/cut-point note
marking the safe place to stop if time runs short. The immediate focus is a
trustworthy application lifecycle on the actual host: deploy, observe, update,
recover, and preserve the administrator's state and data.
