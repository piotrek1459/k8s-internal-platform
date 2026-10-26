# Project outline

Title provided by the author: **Design and implementation of kubernetes platform for containerized application deployment**.

The current product and implementation plan is defined in [app-plan.md](app-plan.md).
It records the refined requirements: a Raspberry Pi reference installation, an
administrator web UI, public/private images, persistent deployment history, and
LAN-first development followed by public application hosting. This outline
preserves the original thesis context and distinguishes source-document
requirements from subsequent project decisions.

The platform and the applications it deploys share one Kubernetes/K3s cluster.
The reference Pi is initially both the control-plane node and the workload node;
the [cluster layout](architecture.md#one-cluster-hosting-the-platform-and-its-applications)
shows namespaces, traffic paths, and operational responsibilities.

This document provides repository planning context. It does not contain thesis
chapters or describe a completed implementation. The original scope is based on
the supplied `idea.pdf`. The longer title in that document does not replace the
title provided by the author.

## Scope described in `idea.pdf`

The source document describes a platform running in an organization's own
Kubernetes cluster to support application deployment by its team. Public hosting
for multiple independent clients is outside that original scope. The current
plan permits public access to administrator-deployed applications while retaining
a single administrator and a single managed cluster.

- A broker written in Go exposes an API for deploying a new application, updating
  it, retrieving its status, rolling it back, and deleting a deployment.
- Docker provides containerization, Kubernetes runs applications, and Helm
  provides deployment parameters and repeatable deployments.
- Infrastructure as Code provisions selected parts of the environment; the
  description identifies Terraform as a likely tool.
- CI/CD builds and verifies images, publishes artifacts, and calls the platform API.
  The original description leaves the choice between GitHub Actions and GitLab CI
  open. The current plan recommends GitHub Actions; implementation details are
  recorded in [operations.md](operations.md).
- The platform handles application configuration, ingress, secrets, and rollout verification.
- Prometheus, Grafana, and Loki provide metrics, visualization, alerts, and logs.
  Tracing is an optional extension mentioned in the description.
- The prototype is evaluated using sample applications, covering functionality,
  performance, and failure scenarios.

## Proposed repository structure

| Directory | Purpose |
| --- | --- |
| `book/` | Existing LaTeX template and thesis content written by the author. |
| `docs/` | The plan, followed by design decisions, diagrams, and instructions. |
| `broker/` | Future Go module containing the API and deployment operation logic. |
| `deploy/helm/` | Platform charts and a template for deployed applications. |
| `infra/terraform/` | Space for IaC if Terraform is ultimately selected. |
| `observability/` | Metrics and log collection configuration, dashboards, and alerts. |
| `examples/` | Small demonstration applications and their configuration. |
| `tests/` | Integration scenarios, experiments, and measurement results. |

The directories define implementation responsibilities. The initial HTTP
demonstration application, application chart, and local verification workflow are
implemented; the broker and full platform remain subsequent work. Current
execution instructions are provided in [local-development.md](local-development.md).

## Original work sequence

1. **Scope definition:** selection of the cluster environment, intended API users,
   minimum requirements, success criteria, and prototype boundaries.
2. **Manual deployment baseline:** deployment of a simple application using Helm
   and documentation of a repeatable procedure for comparison with the platform.
3. **Initial broker workflow:** application of the deployment chart and readiness
   observation through the API. The current asynchronous request and operation
   status contract is defined in [api-and-data.md](api-and-data.md).
4. **Lifecycle management:** updates, rollbacks, deletion, error handling,
   configuration rules, and controlled access to secrets.
5. **Automation and observability:** a reproducible environment, CI/CD, metrics
   and log collection, and selected dashboards and alerts.
6. **Prototype evaluation:** repeatable scenarios with recorded experimental
   conditions, raw results, and conclusions suitable for subsequent thesis work.

The current implementation order and acceptance criteria are defined in
[milestones.md](milestones.md). M0–M4 constitute the core prototype and include
basic evaluation. M5–M8 define planned product extensions and extended evaluation.
These implementation boundaries do not establish academic sufficiency.

## Original design questions and subsequent decisions

The initial design questions concerned the following areas:

- Demonstration environment and the scope of Infrastructure as Code provisioning.
- API inputs, user identification, and permission boundaries.
- Synchronous processing versus asynchronous operations with identifiers.
- Operation state storage and broker restart recovery.
- Restricted broker permissions and secret management outside Git.
- An API-only interface versus a web interface.

These questions arose during project planning and do not constitute additional
requirements from `idea.pdf`. The current plan specifies a Raspberry Pi reference
installation, asynchronous operations, persistent operation state, restricted
in-cluster permissions, and a web UI. Current decisions and remaining open items
are recorded in [app-plan.md](app-plan.md). A custom application CLI and compliance
with a particular Service Broker API standard remain outside the proposed initial
scope.

## Proposed verification plan

| Area | Example scenario | Evidence to collect |
| --- | --- | --- |
| Functionality | Installation, update, status retrieval, rollback, and deletion. | API requests, expected and actual results, resource state. |
| Repeatability | Deploying the same version several times with the same parameters. | Versions, configuration, and the result of each attempt. |
| Operator effort | Performing the same scenario manually and through the platform API. | An explicit definition of a step, step count, and operator time. |
| Performance | Measuring time to readiness for a fixed number of deployments. | Timings, cluster resources, load, and number of repetitions. |
| Failures | An invalid image or an application that fails readiness checks. | Operation state, logs, metrics, and recovery outcome. |

These scenarios are proposed evaluation methods. They do not represent completed
tests or verified platform capabilities. Acceptance thresholds must be defined
before measurements are taken.

## Requirements described in the supplied university document

The following list summarizes pages 1–3 of
`Inf_Wymagania_do-pracy-inzynierskiej-1.pdf`. It does not confirm that the university
regulations are current.

- The document lists: an abstract and keywords; an introduction with the objective,
  scope, and author's contribution; analysis of the subject and literature;
  requirements, tools, and use cases; an external specification; an internal
  specification; verification and validation; a summary and conclusions; a
  bibliography; a list of abbreviations and symbols; lists of figures and tables;
  and a list of supplementary files, where applicable.
- For a thesis by one author, it specifies at least **30 pages**, counted from the
  introduction to the end of the summary, and **5,000 words**, excluding tables,
  captions, code, and pseudocode.
- The bibliography must contain at least **5 entries** from the listed categories:
  technical documentation, books, specialist online sources, or research
  publications. Each entry must be cited in the text; online sources must include
  a URL and access date.
- The supplied template must be used, and figures and tables must be numbered and
  referenced. Figure captions go below figures, and table captions above tables.
  Abbreviations must be expanded on first use. The text should use an impersonal
  style and precise terminology.
- The document describes submitting the thesis text to APD together with materials
  needed to evaluate the work and reproduce the tests: code, test data, and required
  components. For projects involving software, it also lists a demonstration video
  packaged in a ZIP archive.

## Working mapping of project material to thesis sections

| Thesis section | Material collected during the project |
| --- | --- |
| Introduction and analysis | Objective, problem boundaries, sources, and comparison of existing solutions. |
| Requirements and tools | Use cases, acceptance criteria, and rationale for tool selection. |
| External specification | Installation, API usage, administration, security, and a usage example. |
| Internal specification | Architecture, data model, operation workflows, and key decisions. |
| Verification and validation | Scenarios from `tests/`, measurement conditions, results, and failure analysis. |
| Summary | Assessment of requirements met, prototype limitations, and future work. |

This table proposes an arrangement of project material; it does not represent
completed chapters. Thesis section numbering and titles must follow the existing
template in `book/`.
