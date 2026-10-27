# k8s-internal-platform

Engineering thesis project: **Design and implementation of kubernetes platform for containerized application deployment**.

The project aims to build a platform for deploying containerized applications on
Kubernetes, using a Go service broker, Helm, infrastructure as code, monitoring,
and logging. The current implementation includes an HTTP demonstration
application, an application Helm chart, local K3s deployment and recovery tests,
and the **M2 durable backend**: an in-cluster Go broker with a management API, a
durable SQLite operation queue, authentication, in-process Helm integration,
status observation, image inspection, and versioned secrets. The administrator UI
(M3) and reliable-lifecycle operations (M4) remain planned.

## Local application deployment

With Docker running and k3d, kubectl, Python 3, and curl available:

```bash
./scripts/setup-dev-tools.sh
./scripts/build-demo.sh native
./scripts/dev-cluster.sh up
./scripts/test-local.sh
```

The workflow uses a separate local K3s cluster, retains images locally, and
preserves the default Kubernetes context. Successful verification leaves v2 of
the demonstration application running at `http://demo.localhost:8082/version`.
The [local development guide](docs/local-development.md) describes prerequisites,
network boundaries, multi-architecture builds, evidence, and cleanup.

## Building the thesis


From the repository root, run:

```bash
./scripts/build-book.sh
```

On macOS, the script starts Docker Desktop if the engine is unavailable and waits
up to 120 seconds for it to become ready. It then builds the LaTeX image and
compiles `book/main.tex`. The first run requires internet access to download
TeX Live packages and may take several minutes. Later runs use Docker's cache.

Output: **[book/build/main.pdf](book/build/main.pdf)**.
Build log: `book/build/main.log`.
The script does not update the `book/main.pdf` supplied with the template.

You can also use `make -C book` or run the script by its absolute path from any
directory. Run the same command again after editing the thesis. `latexmk`
manages the pdfLaTeX and BibTeX passes, cross-references, and table of contents.

## Repository layout

```text
book/               LaTeX template and thesis content
broker/             Go service for managing deployments
deploy/helm/        Helm charts and deployment configuration
infra/terraform/    infrastructure definitions
observability/      monitoring and logging configuration
examples/           demonstration applications
tests/              test scenarios and experiments
docs/               project scope, plan, and decisions
docker/latex/       thesis build image
scripts/            helper scripts
```

The [project outline](docs/project-outline.md) describes the proposed scope and
its relationship to the supplied requirements. The broker, Terraform, and
observability directories remain placeholders for subsequent implementation.

The detailed [application platform plan](docs/app-plan.md) is an index linking
focused documents: [architecture](docs/architecture.md),
[API and data model](docs/api-and-data.md),
[networking and security](docs/networking-and-security.md),
[operations](docs/operations.md), and
[milestones](docs/milestones.md). It covers both the Raspberry Pi reference host
and the rented-server target, components, UI, API, deployment workflows, and
implementation milestones.
