# Helm deployments

The [application chart](application/README.md) implements the M1 stateless HTTP
profile. It supports manual deployment of the demonstration image before broker
implementation, with private routing, health probes, resource settings, stable
ownership labels, and existing secret references.

The platform chart is planned for M2. It will install the broker and its metadata
volume in `platform-system`, with scoped permissions for workloads in
`platform-apps`.

Committed values contain publishable example configuration only. Application
secrets and registry credentials are created outside the application chart;
values contain references rather than secret material.
