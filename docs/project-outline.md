# Project outline

Title provided by the author: **Design and implementation of kubernetes platform for containerized application deployment**.

This is a working plan for organizing the repository, not thesis content or a
completed implementation. The scope is based on the supplied `idea.pdf`. The
longer title in that document does not replace the title provided in the conversation.

## Scope described in `idea.pdf`

The platform is intended to run in the organization's own Kubernetes cluster and
help the team deploy applications. It is not intended as public hosting for multiple clients.

- A broker written in Go exposes an API for deploying a new application, updating
  it, retrieving its status, rolling it back, and deleting a deployment.
- Docker provides containerization, Kubernetes runs applications, and Helm
  provides deployment parameters and repeatable deployments.
- Infrastructure as Code provisions selected parts of the environment; the
  description identifies Terraform as a likely tool.
- CI/CD builds and verifies images, publishes artifacts, and calls the platform API.
  The choice between GitHub Actions and GitLab CI remains open.
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

At this stage, the directories define responsibilities. They do not contain a
runnable platform, a Go module, or placeholder infrastructure configuration.

## Proposed work sequence

1. **Refine the scope:** choose the cluster environment, intended API users, and
   minimum requirements; record the success criteria and prototype boundaries.
2. **Establish a manual deployment baseline:** deploy a simple application using
   Helm and document a repeatable procedure for later comparison with the platform.
3. **Implement the first broker workflow:** API → chart installation → readiness
   check → response with deployment status.
4. **Manage the lifecycle:** add updates, rollbacks, and deletion, along with error
   handling and agreed rules for configuration and access to secrets.
5. **Add automation and observability:** prepare a reproducible environment,
   CI/CD, metrics and log collection, and selected dashboards and alerts.
6. **Evaluate the prototype:** repeat the same scenarios, record experimental
   conditions, raw results, and conclusions; collect material for the thesis.

## Decisions for the author

- Where will the demonstration cluster run, and which parts will IaC provision?
- What input does the API accept, and how does it identify users and their permissions?
- Are operations synchronous, or do they return a task identifier?
- Where is operation state stored, and what happens when the broker restarts?
- How can broker permissions be restricted and secrets handled without storing them in Git?
- Is an API with documented request examples sufficient as the prototype's interface?

These are design questions, not additional requirements from `idea.pdf`. A web
interface, a custom CLI, and compliance with a particular Service Broker API
standard have not been decided.

## Proposed verification plan

| Area | Example scenario | Evidence to collect |
| --- | --- | --- |
| Functionality | Installation, update, status retrieval, rollback, and deletion. | API requests, expected and actual results, resource state. |
| Repeatability | Deploying the same version several times with the same parameters. | Versions, configuration, and the result of each attempt. |
| Operator effort | Performing the same scenario manually and through the platform API. | An explicit definition of a step, step count, and operator time. |
| Performance | Measuring time to readiness for a fixed number of deployments. | Timings, cluster resources, load, and number of repetitions. |
| Failures | An invalid image or an application that fails readiness checks. | Operation state, logs, metrics, and recovery outcome. |

These scenarios are proposed evaluation methods. They do not represent completed
tests or verified platform capabilities. Acceptance thresholds should be defined
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

This is a proposed arrangement of material, not completed chapters. The author
should align numbering and titles with the existing template in `book/` while
writing the thesis.
