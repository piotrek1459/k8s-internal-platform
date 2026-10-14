# Tests and experiments

The initial integration test is [scripts/test-local.sh](../scripts/test-local.sh).
It verifies the actual Helm/K3s application lifecycle and private ingress
entrypoint selection. Demonstration unit tests reside alongside the Go source in
[examples/http-demo](../examples/http-demo/).

Each local run records its transcript, implementation archive, versions, image
identities, manifests, diagnostics, response assertions, and resource snapshots
under `experiments/runs/`. Raw output is ignored by Git. Curated reports identify
the environment and limitations separately from implementation acceptance.

The [local development guide](../docs/local-development.md) defines the execution
procedure. The [evaluation plan](../docs/milestones.md#tests-and-thesis-evaluation)
defines the required core evidence and later extensions.
