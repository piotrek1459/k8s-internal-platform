# Platform broker

Placeholder for the Go module: an API for deploying applications, retrieving
status, updating deployments, rolling back, and removing deployments.

Once implementation begins, `cmd/broker/` can hold the entry point and `internal/`
can contain API handlers and Kubernetes/Helm operations. The API contract and
state storage approach remain design decisions for the author.
