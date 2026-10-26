# Helm deployments

The [application chart](application/README.md) implements the M1 stateless HTTP
profile. It supports manual deployment of the demonstration image before broker
implementation, with private routing, health probes, resource settings, stable
ownership labels, and existing secret references.

The [platform chart](platform/README.md) installs the M2 broker and its metadata
volume in `platform-system`, with a service account and scoped RBAC granting
workload management in `platform-apps` and read-only cluster reads for capacity
and volumes.

Committed values contain publishable example configuration only. Application
secrets and registry credentials are created outside the application chart;
values contain references rather than secret material.
