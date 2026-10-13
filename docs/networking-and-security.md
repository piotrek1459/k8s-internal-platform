# Networking and security

Part of the [application platform plan](app-plan.md). This document covers
visibility modes and network isolation (LAN first, public access later) and the
credentials/data/operational-safety rules. System shape is in
[architecture.md](architecture.md); the API and data model are in
[api-and-data.md](api-and-data.md); install and recovery are in
[operations.md](operations.md).

## Networking: LAN first, public access later

### Separate visibility from authentication

| Mode | Intended users | Access path |
| --- | --- | --- |
| Internal | Other applications in the cluster. | ClusterIP Service and internal service name; no external route. |
| LAN | People on the administrator's LAN or VPN. | Private ingress listener with enforced network restrictions. |
| Public | Internet visitors. | Selected public ingress mode and a configured hostname. |

The management UI/API remains private by default. Authentication of visitors to
a deployed app is that app's responsibility. Making an app public does not give
visitors access to platform administration.

### LAN milestone

Use the private management endpoint for the UI and explicit private routes for
LAN apps. These may share a restricted private listener; public traffic uses a
separate entrypoint as described in [architecture.md](architecture.md#management-requests-and-application-traffic).
Configure a local domain, for example `pi.home.arpa`, and route an app as
`notes.pi.home.arpa`. These are illustrative names, not records created
automatically by Kubernetes.

For an early developer demonstration, an explicit hosts-file entry is acceptable.
For a repeatable installation, document one-time local DNS configuration on the
router/local resolver, ideally with a wildcard. Show exact missing DNS/trust steps
in setup. Do not assume Kubernetes DNS is visible to laptops on the LAN.

Normal administration should use HTTPS over the private listener, with either a
trusted domain certificate or a locally trusted CA. The initial SSH-forwarded
bootstrap remains a fallback. A self-signed certificate requires deliberate
browser trust setup. [cert-manager private certificate guidance](https://cert-manager.io/docs/configuration/selfsigned/)

### Public mode decision

Decide the public access mode after the LAN workflow works. The right choice
depends heavily on **where the platform is hosted**:

- **Pi at home:** often behind CGNAT with no controllable inbound ports and a
  changing address. An outbound tunnel is frequently the only reliable option.
- **Rented internet server (VPS/dedicated):** usually has a stable public IP and
  direct inbound 80/443, so **direct ingress** is the natural choice and no tunnel
  dependency is needed. This is a key reason the platform targets rented servers,
  not only the Pi.

| Option | Required setup | Benefit | Constraint |
| --- | --- | --- | --- |
| Direct ingress | Domain, reachable address, DNS, router/firewall access, certificate issuance. | Public traffic reaches the operator's own ingress directly. Natural fit for a rented server with a public IP. | Upstream CGNAT, blocked ports, and changing addresses can prevent access (common on a home Pi). |
| Outbound tunnel | Domain/provider account, a configured named tunnel, outbound connectivity. | Useful when inbound port forwarding is unavailable, e.g. a home Pi behind CGNAT. | Adds a dependency on the tunnel provider and its account/service. |

Implement and test **one** public mode for the initial release. Keep an adapter
boundary so the other can follow. Choosing the host (home Pi vs rented server)
largely decides which mode to implement first. Do not build both before learning
whether the chosen host allows inbound access. Buying the domain is not an early dependency.

For direct ingress, route 80/443 to the public listener, configure DNS, and use
cert-manager for certificate lifecycle. HTTP-01 validation requires incoming port
80; DNS-01 can support wildcard certificates and requires DNS credentials. Test
against the ACME staging service first. [Let's Encrypt challenge types](https://letsencrypt.org/docs/challenge-types/),
[cert-manager ACME](https://cert-manager.io/docs/configuration/acme/)

For a Cloudflare adapter, use a named tunnel and explicit public-hostname routes
with an unmatched-host rejection. A wildcard is acceptable only when it points
to an ingress dedicated to public apps. Keep administrator/internal routes out
of it. Edge HTTPS and the local tunnel-to-service connection are different TLS
boundaries; document the actual origin protocol. This mode keeps computation on
the host while depending on Cloudflare for public traffic.
[Cloudflare tunnel setup](https://developers.cloudflare.com/tunnel/get-started/)

### Isolation and verification

A private hostname on the same publicly reachable listener is not private:
an external caller can supply that hostname in Host/SNI. Separate listeners and
effective firewall/source restrictions are required. A different port alone is
not sufficient on an Internet-facing machine. **This matters even more on a
rented server, which is internet-facing by default** — unlike a home Pi that may
be shielded by NAT, a VPS exposes every open port to the Internet immediately.

Every generated ingress route must explicitly select its permitted entrypoint:
private applications use the private listener, and public applications use the
public listener. Never rely on a controller default that attaches routes to all
entrypoints. The installer keeps public entrypoints disabled/unexposed or blocked
until public networking setup succeeds; home-router NAT is not the isolation policy,
and on a rented server there is no NAT to fall back on.

Test the implemented restrictions with actual traffic, including IPv6 and
forged private Host/SNI requests. Configure trusted proxy headers carefully;
client-supplied forwarding headers must not bypass access restrictions.
[Traefik entrypoint selection](https://doc.traefik.io/traefik/v3.5/reference/routing-configuration/kubernetes/ingress/)

Track workload health, DNS, routing, TLS, and external reachability separately.
A check from the host or the same LAN cannot prove reachability from the Internet;
use an outside client for the final acceptance test. A router's hairpin behavior
can also make LAN and external results differ. Restrict diagnostic checks to
managed destinations so the API does not become an arbitrary network probe.

## Credentials, data, and operational safety

Public applications make authentication and sensible runtime defaults part of
the initial design, not an optional final polish step.

- One local administrator account, securely hashed password, login throttling,
  expiring sessions, and host-local password recovery.
- Bootstrap registration closes after first use; no default published password.
- UI/API and cluster administration endpoints stay outside public app routing.
- Workloads run with restricted privileges: no host networking, privileged mode,
  arbitrary host mounts, or service-account token by default.
- Apply resource policies and network rules separating workloads from management.
  Explicitly allow needed DNS, ingress, registry, and selected dependency paths.
- Validate registry destinations and health-check targets; private registries are
  allowed only through administrator-configured trust rules. Protect link-local,
  metadata, loopback, and cluster management endpoints from arbitrary proxying.
- Use structured SDK calls, not shell interpolation of image names or input.
- Bind log/event access to the requested application's owned resources.

Store secret material in separately managed Kubernetes Secrets. SQLite, audit
diffs, logs, regular responses, and Helm values contain only references and
metadata. Helm stores release values, so passing plaintext through chart values
would make them part of release history.
[Helm storage backends](https://helm.sh/docs/topics/advanced/#storage-backends)

Use immutable versions for application configuration secrets; retained revisions
reference their versions. Registry credentials require separate rotation and
reference rules because old revoked credentials must not be restored accidentally
during code rollback. Explain rollback limitations when external credentials change.

Enable K3s secret encryption and protect backups containing credentials. Base64
representation is not encryption, and the machine administrator still controls
the host and cluster. [Kubernetes secret practices](https://kubernetes.io/docs/concepts/security/secrets-good-practices/),
[K3s secrets encryption](https://docs.k3s.io/security/secrets-encryption)

### Backup and restore

Treat backup as three coordinated tasks:

1. Platform database, settings, secret references/material, and version manifest.
2. K3s datastore and the server token required by its restore procedure.
3. Application volumes and application-specific database backups.

Back up SQLite using a consistent backup API or a controlled stopped-service
procedure, not by blindly copying a live database file. For databases inside
applications, use their supported backup methods. Keep at least one copy off the
host's primary storage (for the Pi, off the microSD) and test restoration to
replacement storage.
[SQLite backup API](https://www.sqlite.org/backup.html),
[K3s backup and restore](https://docs.k3s.io/datastore/backup-restore)

Record the supported recovery procedure and measured recovery time. Neither a
successful image rollback nor a K3s datastore backup proves app data is protected.
