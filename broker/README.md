# Platform broker

Placeholder for the Go module: an API for deploying applications, retrieving
status, updating deployments, rolling back, and removing deployments.

M2 introduces `cmd/platform/` and internal modules for HTTP handling, durable
operations, persistence, and Kubernetes/Helm integration. The planned contract and
storage model are defined in [api-and-data.md](../docs/api-and-data.md), with
execution and recovery rules in [operations.md](../docs/operations.md).
