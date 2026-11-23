# Controlled E2E workload

This standard-library Go fixture is built only for an owned disposable E2E
registry/cluster. Its image is not included in platform release bundles.
The broker fast check runs its formatting, vet and unit tests on every PR.

- `/healthz` and `/version` respond while `/readyz` remains closed. A random
  control token is injected through a versioned configuration Secret; authenticated
  `POST /control/ready` opens readiness. A pending Helm release, persisted running
  operation and running/unready container establish the test barrier.
- Two images embed distinct version responses. Moving the owned registry tag
  distinguishes a real immutable-digest retry from a mutable tag deployment.
- With explicit target URLs and the cluster's public CA, `POST /probe` issues
  requests from the managed Pod. It has no cookie jar, does not load ambient
  credentials or follow redirects, and verifies Kubernetes TLS. Its response
  contains status/transport classification and whitelisted Kubernetes Status
  fields; response bodies and credential material are not exported.
- Token absence, network reachability and explicit authorization refusal are
  checked independently. A network failure cannot establish access denial.

The fixture runs under the ordinary application chart's security/resource policy.
Its temporary image tags and registry/cluster are removed by environment cleanup.
Only sanitized status results and chart/values hashes are retained as evidence.
