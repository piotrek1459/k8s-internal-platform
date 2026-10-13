# Networking and security

Part of the [application platform plan](app-plan.md). This document covers
visibility modes, network isolation (LAN first, public access later), credential
management, data protection, and operational safety. The architecture is defined in
[architecture.md](architecture.md); the API and data model are in
[api-and-data.md](api-and-data.md); installation and recovery are described in
[operations.md](operations.md).

## Networking: LAN first, public access later

### Visibility and authentication

| Mode | Intended users | Access path |
| --- | --- | --- |
| Internal | Other applications in the cluster. | ClusterIP Service and internal service name; no external route. |
| LAN | People on the administrator's LAN or VPN. | Private ingress listener with enforced network restrictions. |
| Public | Internet visitors. | Selected public ingress mode and a configured hostname. |

The management UI/API remains private by default. Authentication of visitors to
a deployed application remains the application's responsibility. Public
application access does not give visitors access to platform administration.

### LAN milestone

The UI uses the private management endpoint, and LAN applications use explicitly
configured private routes. These may share a restricted private listener; public
traffic uses a separate entrypoint as described in [architecture.md](architecture.md#management-requests-and-application-traffic).
A local domain, such as `pi.home.arpa`, provides application hostnames such as
`notes.pi.home.arpa`. These names are illustrative; their DNS records are not created
automatically by Kubernetes.

For an early developer demonstration, an explicit hosts-file entry is acceptable.
For a repeatable installation, the documentation must specify the required local
DNS configuration on the router or local resolver, preferably with a wildcard.
The setup interface must identify any outstanding DNS configuration and certificate
trust requirements. Kubernetes cluster DNS is not assumed to be accessible from
client devices on the LAN.

Normal administration requires HTTPS over the private listener, with either a
trusted domain certificate or a locally trusted CA. The initial SSH-forwarded
bootstrap remains a fallback. A self-signed certificate requires deliberate
browser trust setup. [cert-manager private certificate guidance](https://cert-manager.io/docs/configuration/selfsigned/)

### Public mode decision

The public access mode is selected after successful validation of the LAN
workflow. Selection depends on the deployment environment:

- **Home Raspberry Pi:** CGNAT, unavailable inbound port forwarding, or a changing
  public address may constrain direct ingress. An outbound tunnel is an option
  when inbound connectivity is unavailable.
- **Rented internet-facing server (VPS or dedicated server):** a stable public
  address and permitted inbound connections on ports 80 and 443 support direct
  ingress without a tunnel provider dependency.

| Option | Required setup | Benefit | Constraint |
| --- | --- | --- | --- |
| Direct ingress | Domain, reachable address, DNS, router/firewall access, certificate issuance. | Public traffic reaches the operator's own ingress directly. Applicable to a rented server with a public IP. | Upstream CGNAT, blocked ports, and changing addresses can prevent access, particularly in home networks. |
| Outbound tunnel | Domain/provider account, a configured named tunnel, outbound connectivity. | Useful when inbound port forwarding is unavailable, e.g. a home Pi behind CGNAT. | Adds a dependency on the tunnel provider and its account/service. |

The initial public-access implementation must support and test one selected
mode. An adapter boundary permits a second mode to be added subsequently.
Selection requires confirmation of the target host's inbound connectivity.
Domain acquisition is not a prerequisite for the initial LAN workflow.

Direct ingress requires ports 80 and 443 to reach the public listener, suitable DNS
records, and cert-manager for certificate lifecycle management. HTTP-01 validation
requires incoming port 80; DNS-01 supports wildcard certificates and requires DNS
credentials. Certificate issuance must be validated against the ACME staging
service before production issuance. [Let's Encrypt challenge types](https://letsencrypt.org/docs/challenge-types/),
[cert-manager ACME](https://cert-manager.io/docs/configuration/acme/)

The Cloudflare adapter requires a named tunnel, explicit public-hostname routes,
and rejection of unmatched hostnames. A wildcard is acceptable only when it points
to an ingress dedicated to public applications. Administrator and internal routes
must be excluded. Edge HTTPS and the local tunnel-to-service connection are distinct
TLS boundaries; the actual origin protocol must be documented. This mode keeps
computation on the host while depending on Cloudflare for public traffic.
[Cloudflare tunnel setup](https://developers.cloudflare.com/tunnel/get-started/)

### Isolation and verification

A private hostname on the same publicly reachable listener is not private:
an external caller can supply that hostname in Host/SNI. Separate listeners and
effective firewall/source restrictions are required. A different port alone is
not sufficient on an internet-facing machine. On a publicly addressed server,
private listeners require explicit firewall or source restrictions wherever
upstream network controls do not already prevent external access.

Every generated ingress route must explicitly select its permitted entrypoint:
private applications use the private listener, and public applications use the
public listener. Controller defaults that attach routes to all entrypoints must
not be used. The installer keeps public entrypoints disabled, unexposed, or blocked
until public networking setup succeeds. The isolation policy must operate
independently of any NAT provided by the surrounding network.

The implemented restrictions must be tested with actual traffic, including IPv6
and forged private Host/SNI requests. Proxy headers must be accepted only from
explicitly trusted proxies; client-supplied forwarding headers must not bypass
access restrictions.
[Traefik entrypoint selection](https://doc.traefik.io/traefik/v3.5/reference/routing-configuration/kubernetes/ingress/)

Workload health, DNS, routing, TLS, and external reachability require separate
status reporting.
A check from the host or the same LAN cannot prove reachability from the Internet;
the final public-access acceptance test therefore requires an external client.
A router's hairpin behavior can also make LAN and external results differ.
Diagnostic checks must be restricted to managed destinations to prevent the API
from becoming an arbitrary network probe.

## Credentials, data, and operational safety

Authentication and restricted runtime defaults are requirements of the initial
design, including the LAN deployment stages.

- One local administrator account, securely hashed password, login throttling,
  expiring sessions, and host-local password recovery.
- Bootstrap registration closes after first use; no default published password.
- UI/API and cluster administration endpoints stay outside public app routing.
- Workloads run with restricted privileges: no host networking, privileged mode,
  arbitrary host mounts, or service-account token by default.
- Resource policies and network rules separate workloads from management.
  Required DNS, ingress, registry, and selected dependency paths are explicitly allowed.
- Registry destinations and health-check targets are validated; private registries
  are allowed only through administrator-configured trust rules. Link-local,
  metadata, loopback, and cluster management endpoints must be protected from
  arbitrary proxying.
- External operations use structured SDK calls; shell interpolation of image names
  or other input is prohibited.
- Log and event access is restricted to resources owned by the requested application.

Secret material is stored in separately managed Kubernetes Secrets. SQLite, audit
diffs, logs, regular responses, and Helm values contain only references and
metadata. Helm stores release values, so passing plaintext through chart values
would make them part of release history.
[Helm storage backends](https://helm.sh/docs/topics/advanced/#storage-backends)

Application configuration secrets use immutable versions; retained revisions
reference their versions. Secret versioning is a prerequisite for configuration
rollback in the core prototype. Registry credentials require separate rotation and
reference rules because old revoked credentials must not be restored accidentally
during code rollback. The documented rollback behavior must identify limitations
introduced by changes to external credentials.

K3s secret encryption must be enabled, and backups containing credentials must be
protected. Base64 representation is not encryption, and the machine administrator
still controls the host and cluster. [Kubernetes secret practices](https://kubernetes.io/docs/concepts/security/secrets-good-practices/),
[K3s secrets encryption](https://docs.k3s.io/security/secrets-encryption)

### Backup and restore

The backup design comprises three coordinated tasks:

1. Platform database, settings, secret references/material, and version manifest.
2. K3s datastore and the server token required by its restore procedure.
3. Application volumes and application-specific database backups.

SQLite backups require a consistent backup API or a controlled stopped-service
procedure; copying a live database file alone is insufficient. Databases used by
deployed applications require their supported backup methods. At least one backup
copy must be stored outside the host's primary storage, including outside the
microSD card for the Raspberry Pi reference installation. Recovery validation must
include restoration to replacement storage.
[SQLite backup API](https://www.sqlite.org/backup.html),
[K3s backup and restore](https://docs.k3s.io/datastore/backup-restore)

The supported recovery procedure and measured recovery time must be recorded.
Application data protection requires separate validation from image rollback
and K3s datastore backup.
