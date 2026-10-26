# PR #6 review verification — 2026-10-26

## Scope

This run verifies the review corrections for PR #6 against the owned local
Docker/K3s cluster. The source base is `1f59b5c`, with the corrections captured in
the run's implementation archive. Raspberry Pi, public hosting, and native AMD64
execution are outside this verification.

## Results

| Check | Observed result |
| --- | --- |
| Go tests and vet | `go test -race ./...` and `go vet ./...` passed. |
| Chart validation | Strict lint, positive rendering cases, and 18 invalid configuration rejection cases passed. TCP probes accepted removed paths for readiness, startup, and liveness; HTTP probes rejected removed, relative, and empty paths. |
| Helper regressions | Nine Python unittest methods passed, including incomplete rollback states and four combinations of command/logger exit statuses with a deliberately delayed writer. |
| Docker images | Native v1, v2, and v3 images rebuilt successfully with test sources excluded from the build context. Images remained local. |
| Initial lifecycle | v1 installation and v2 update served the expected versions through the private route. |
| Deliberate failure | v3 remained unready, produced HTTP 503 readiness events, and failed the Helm upgrade; v2 continued serving. |
| Rollback | Revision 4 had status `deployed` immediately after rollback from failed revision 3. The Deployment targeted v2, its observed generation matched generation 4, and one updated, ready, available replica remained. The v3 ReplicaSet had zero desired replicas and no v3 Pods remained. |
| Non-default port | Revision 5 deployed v2 with `containerPort: 9090` and `PORT=9090`. Startup/readiness probes passed and the private route returned v2. |
| Route isolation | The private hostname returned HTTP 404 through the separate listener. |
| Transcript completion | The process exited with status 0 after the final PASS message was written to the transcript. |

The local lifecycle command was `./scripts/test-local.sh`. The final release is
revision 5, running v2 on container port 9090; the external URL remains
`http://demo.localhost:8082/version`. The retained rollback snapshot records
revision 4 as deployed before the subsequent port-change upgrade superseded it.

Recorded scenario durations were 4 s for installation, 5 s for update, 30 s for
the expected failed upgrade, 3 s for rollback, and 9 s for the port-change check.
These are single-run functional observations, not performance measurements.
Pod metrics were unavailable during the final snapshot; no resource-sizing
conclusion is drawn from this run.

## Evidence

The raw evidence directory is
`tests/experiments/runs/20261026T200015Z-36180/`. It contains the transcript,
source archive, component versions, image identities, failed-release evidence,
rollback snapshots, final port-9090 Deployment, Helm history, timings, and
exit status. Raw runs remain excluded from Git.

The SHA-256 of `implementation.tar.gz` is:

```text
b45f69ce8534403189c86fd90a5b7a2710ab3a5b8a81c5d2c00abef21c8fa414
```
