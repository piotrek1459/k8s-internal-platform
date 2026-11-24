# Private LAN HTTPS source acceptance

Recorded: 2026-11-24 (Europe/Warsaw). First sequential implementation cycle for
[issue #16](https://github.com/piotrek1459/k8s-internal-platform/issues/16).

## Implemented boundary

The optional gateway uses one explicit RFC1918 IPv4 bind address, mandatory
actual-peer CIDRs, a pinned multi-architecture Traefik v3.7.19 image, an existing
TLS Secret and the dedicated private IngressClass/entrypoint. Management and
application names are separate. Native setup validates configuration, local
address, certificate dates/SANs/key before Helm changes and retains successful
configuration across same-version reruns. Secure cookies are automatic. The
[LAN guide](../../docs/private-lan.md) records DNS, CA trust, renewal, image-pull
and TLS/SSH fallback requirements.

The API reserves app hostnames transactionally across live/rollback dependencies.
Details/API use the last successful revision for the configured HTTPS URL. Until
failed-mutation route absence is verified, access mode/hostname changes are
refused with 422; normal image/replica changes remain supported. The source
workflow verifies actual HTTPS content after healthy image update and rollback,
with bounded endpoint convergence separate from Pod readiness. This guard stays
in the core recovery follow-up, not a completed route-transition requirement.
Application chart 0.1.2 adds the private class; old chart identities are preserved
and cross-version upgrades remain unsupported.

## Executed verification

| Scope | Observed result |
| --- | --- |
| Focused local workflow | 2 tests, 0 failures, 0 skips at `bfe62aaf89fe381a89ed0150e9e5ae84bb3da7a8`; actual private HTTPS deployment, URL reload, image update/rollback, source/route/cookie and listener checks passed. |
| Full clean-source CI | [Run 45378412653](https://github.com/piotrek1459/k8s-internal-platform/actions/runs/45378412653) passed **21 tests, 0 failures, 0 skips** at `ddec3aa7fdd1fb5615308027db265c36278c9490`. All five fast GitHub checks passed on that source. |
| Native configuration/transport guard regressions | **80 Python cases passed**, including certificate/key/date/SAN/hostname checks, assigned-address/configuration validation, process-argument privacy and refusal of a bundle without LAN support. Shell syntax passed. |
| Other fast verification | Go format/vet/regressions, UI type-check/lint/build, full OpenAPI and chart render/negative/synchronization checks passed. |

The full CI artifact `14914456323` was downloaded. Its archive SHA-256
matched GitHub metadata (`5417198975718b7e46fe271e5fd99fc972f0d2774d54626aab284b87b5e2627e`); JUnit and the strict summary
were verified independently. Conventional PEM private-key markers were absent.
Case durations total 765.857 seconds; these validation timings are not
an evaluation dataset. The [machine-readable summary](2026-11-24-private-lan.json)
records individual case names/times and evidence hashes.

The full LAN case verified CA/hostname TLS, server-side unknown-SNI refusal even
with client certificate checking disabled for that negative probe, browser
login/deployment/URL reload, actual HTTP v1/v2 and rollback convergence, Secure/
HttpOnly/SameSite=Strict host-only cookies, application-origin CORS/CSRF refusal,
management/outside/nested/duplicate hostname rejection, immutable access transition
guards, Internal-route absence, independent-controller forged Host/SNI refusal,
IPv6 refusal and excluded-peer requests with forged forwarding headers.
The earlier focused local run precedes the final transport-privacy/CORS/strict-SNI
assertions; the full source CI run covers those additions.

After that full run, a read-only installer capability preflight was added to
refuse a legacy bundle that lacks the listener. Its guard regression passed in
the 80-case suite. Application runtime/chart and E2E code are unchanged after the
full CI run. Native installer execution remains the next release gate.

## Development failures and cleanup

The first local probe found that an explicit PROXY-protocol flag instantiated a
listener parser without trusted IPs. It was removed so the protocol remains
disabled. Recovery of that failed platform change also exposed retained omitted
environment entries; the chart now explicitly clears network fields when LAN is
disabled, and LAN configuration uses atomic rollback/cleanup on failure.

A subsequent run was interrupted by editing the shell launcher while it was
executing. That attempt was not accepted; later runs use fixed committed source.
Another run correctly returned 403 because k3d's published-port proxy used a
peer other than the presumed Docker gateway. The portable test now measures
an established TLS connection on only its owned node/listener, then applies that
exact /32 before positive and excluded-source/forged-header checks. No CIDR guard
was weakened, and no failed attempt is counted as passing acceptance. A later
probe observed a transient HTTPS failure immediately after successful rollback;
the final test requires bounded actual HTTP convergence separately from Pod
readiness, without treating a transport failure as a successful response.

Owned local clusters/proxies and ephemeral CA/key/session fixtures are removed
after acceptance; credentials, raw keys and cookie headers are not published.

## Native release gate

Published v0.1.7 remains running on the reference Pi. No native LAN setup or
workstation trust-store mutation was performed for this source PR. Source browser
acceptance pins only the ephemeral leaf SPKI; independent curl/Python clients
verify the CA and hostname. This is not ordinary OS/browser trust acceptance.

After this PR is merged and a release is manually published, native installation,
second-device DNS/trust, real app HTTP, Secure cookies, source/Host/SNI/IPv6
isolation, same-version configuration retention and reboot must be verified.
Only then are the corresponding M0/M3 checkboxes eligible for completion and the
next recovery/security implementation PR started. Resource calibration
and repeated manual-Helm/UI/API evaluation remain the third cycle. Issue #16
stays open until the remaining core criteria have evidence.

## Release verification compatibility correction

The [manual v0.1.8 release run](https://github.com/piotrek1459/k8s-internal-platform/actions/runs/45383021528)
resolved `main` to `40f467a0d39701caf474dc1e02c79318edb9afc6` and failed
before E2E, bundle creation or publication. Its wrong-hostname regression
observed successful installer preflight for an invalid certificate. No v0.1.8
release or tag was created by that run.

The previously successful PR Python job ran Ubuntu 26.04.1; release verification
ran Ubuntu 24.04. OpenSSL 3.0.13 reproduced the defect: `x509 -checkhost`
returned success despite a hostname mismatch. The implementation now uses
`verify -verify_hostname` with the supplied leaf as the explicit local trust
anchor and partial-chain verification. This establishes local certificate
identity without requiring the server to trust its private issuer; browser/client
CA trust remains a separate requirement. The behavior follows the
[OpenSSL verification contract](https://docs.openssl.org/3.0/man1/openssl-verification-options/).

All **81 Python regressions and shell syntax checks passed** with OpenSSL 3.0.13
in Ubuntu 24.04 and OpenSSL 3.6.2 on the development host. The original wrong-host
case remains required; an additional valid privately issued leaf checks against
overly restrictive rejection. The Ubuntu container base was
`ubuntu@sha256:562111b88caa79c71391c0c4d3c2114a7d5c35915c20561fd893cb1e07929e6b`.
PR Python/OpenAPI checks now use Ubuntu 24.04 like release verification, and both
workflows print the OpenSSL version. Application runtime/chart and E2E code were
not changed by this compatibility correction.

The next release requires a new manual workflow dispatch from the fixed branch
HEAD. Re-executing the failed run retains its previously prepared commit. Native
LAN acceptance still follows successful publication.
