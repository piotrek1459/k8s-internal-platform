# Helm deployments

The [platform chart](platform/README.md) installs the Go broker and embedded UI,
metadata PVC, Service, service account and scoped RBAC. The
[application chart](application/README.md) defines each managed stateless HTTP
workload. The broker uses an embedded copy of that chart; synchronization checks
compare it with the canonical source.

Current local installation uses `./scripts/platform.sh up`. The separate
[manual Helm baseline](../../docs/local-development.md#manual-helm-baseline)
remains an executable comparison workflow. It does not install the platform.

Committed values contain publishable configuration. Configuration Secrets are
created separately, and chart values carry references. Private registry
credential provisioning is incomplete even though the workload chart accepts
image-pull Secret names. [Implementation status](../../docs/status.md) records
recovery, networking, storage and release boundaries.
