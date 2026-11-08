# Unified local workflow verification — 2026-11-08

## Environment and scope

The local workflow was verified against a separate, disposable Docker/K3s
installation. The source base was `69c1882`, with the launcher and documentation
changes present as uncommitted working-tree changes.

| Setting | Value |
| --- | --- |
| Cluster | `platform-script-check` |
| Registry host port | `5003` |
| Kubernetes API port | `6447` |
| Management forward | `18080` |
| Application forward | `18081` |

The existing `platform-local` and `platform-dev` environments were not modified.
Images remained in the local Docker engine and registry. This verification does
not establish Raspberry Pi performance, native AMD64 execution, public hosting,
or browser interaction coverage.

## Results

| Check | Observed result |
| --- | --- |
| Fresh startup | `platform.sh up --no-ui` created the registry and cluster, built the UI, broker and demonstration images, and installed a ready platform. |
| First-run authentication | The generated bootstrap token created one administrator; cookie login and identity retrieval succeeded. The token command refused to print a consumed token. |
| Application deployment | An API-created Internal application completed its operation and returned v1 through `platform.sh forward`. |
| Stop and resume | `platform.sh stop` followed by `platform.sh up --no-ui` reused the installation. The administrator identity, existing session and successful application revision were retained. |
| Update | An image update completed and returned v2 from a different Pod. |
| Scaling | Changes to two, zero and one replica each completed successfully. |
| Rollback | Rollback to the recorded successful v1 revision completed and returned v1 over HTTP. |
| Deletion | The application disappeared from the active list; its recorded revisions remained available. |
| Logout | A subsequent identity request returned HTTP 401. |
| Helper regressions | All 15 Python unittest methods passed without requiring a cluster. |
| Charts | Application/platform lint, rendering, input rejection and embedded chart synchronization checks passed. |

The lifecycle requests were driven by an isolated Python HTTP client with
in-memory credentials. Management and application access used the actual
foreground CLI forwards. The UI production build succeeded; browser forms were
not automated. Deliberately unhealthy v3 deployment, application restart and
private registry authentication were not acceptance claims of this run.

## Local evidence

During the run, ignored evidence was written under `.local/script-check/`:
`build.log`, `resume.log`, `verification.log`, `results.json` and forwarding logs.
The temporary driver, raw files, clusters and registry were subsequently removed
during cleanup. This tracked report preserves the observations; the historical
raw directory is no longer available locally. The driver was not committed as an
automated acceptance suite.

The client kept the administrator password, bootstrap token and session cookies
out of result files. New runs must retain their own sanitized evidence before it
is cited as reproducible thesis material. Current commands and gaps are in the
[local guide](../../docs/local-development.md) and [status report](../../docs/status.md).
