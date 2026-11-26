# Reference-host evaluation

These opt-in experiments complement the [verification suites](../README.md).
They run against an explicitly selected native installation and create temporary
owned applications; they are not fast PR tests. The fixed
[protocol](../../docs/evaluation.md) and [plan](plan.json) define interpretation.
The evidence-handling regressions run on every PR.

Python 3.10+, cgroup v2 with `memory.peak`, the installer-owned tools and root
access to the reference Pi are required. The separate HTTPS load client uses only
Python's standard library. No monitoring stack, Docker dependency, synthetic
operator-time estimate or global container-store prune is introduced.

## Preparation

Retain each attempt in a new private output directory. Prepare a private JSON
file containing the existing administrator `password` field (mode 0600), the
public issuing CA certificate and the published `release.json`. Do not pass a
password in an argument or place any credential in Git. Generated cookies remain
in memory and are revoked when the client closes. The SSH password and private
CA key are not experiment inputs.

Copy the exact `broker/internal/helm/chart` directory from the release manifest's
Git commit. This is the chart embedded in the broker. The documented
`deploy/helm/application` directory also includes examples/README files; those
change Helm's stored chart identity despite matching templates. The runner
compares the complete stored chart and manual configuration with the API's
actual execution. Copy the tooling and fixed plan to the Pi; record their source
commit/file hashes before executing. Authenticate and inspect background workloads
before timing.

In a privileged Pi shell, from the copied evaluation directory:

```bash
umask 077
evaluation_args=(
  --plan plan.json --release-manifest release.json
  --state access.json --ca ca.crt
  --management-url https://platform.home.arpa:8443 --address 192.168.68.227
  --chart embedded-chart
)
python3 tools/run.py healthy "${evaluation_args[@]}" --output smoke-01
python3 tools/run.py lifecycle "${evaluation_args[@]}" --output lifecycle-01
```

The first command checks healthy scenarios; the second includes the six real
300-second unhealthy-update waits and therefore takes more than 30 minutes.
Three repetitions alternate method order. Preparation/authentication precede
measurement; actual HTTP through the Service proxy completes both methods.
Manual failure handling explicitly invokes scripted Helm rollback, so it measures
system execution rather than a person's response time. Missing/failed scenarios
or failed timing targets produce a failing summary/exit code.

Successful runs delete their own applications/releases and retain platform
history. An interrupted run retains `owned.json`. After inspecting its current
operations, use the guarded cleanup command; it refuses changed identities,
active operations, foreign names and recreated Helm Secrets:

```bash
python3 tools/cleanup.py --run lifecycle-01 \
  --state access.json --ca ca.crt \
  --management-url https://platform.home.arpa:8443 --address 192.168.68.227
```

## Resource and request observations

Create one fixture at a time, using a new directory for every repetition:

```bash
python3 tools/run.py load-fixture "${evaluation_args[@]}" --output go-01
# Add --public-image for the plan's pinned unprivileged Nginx workload.
# load-fixture.json contains the owned app ID and HTTPS URL; no credentials.
python3 tools/host.py --app-id APP_ID --output go-01/host --seconds 110
```

Once `host/ready.json` exists, preserve the initial 30 seconds as idle. At that
boundary, run the following from a separate workstation for 10 seconds of warmup
and 60 seconds of loaded observations. Retain clock offset and actual start/end
timestamps. Use `/version` and `--version v1` for the demo, or `/` and
`--contains 'Welcome to nginx!'` for Nginx:

```bash
python3 tests/evaluation/load.py \
  --url https://OWNED_HOST.apps.home.arpa:8443/version \
  --ca /private/path/ca.crt --address 192.168.68.227 \
  --version v1 --rate 20 --warmup 10 --seconds 60 --output .local/load/go-01
```

The explicit IP override preserves SNI and normal CA/hostname validation. The
client uses HTTP/1.1, up to 32 workers and a bounded outstanding-request count; the initial four-slot client was retained
as an invalid attempt after unsent requests during a latency burst. The amended
plan repeats the entire resource matrix with the original rate/targets;
client saturation, late scheduling, missing responses and unexpected content
remain failures. This controlled client does not measure browser usability or
all browser protocols. After sampling, delete the fixture with `cleanup.py`.
Repeat three times for each workload, alternating workload order. Avoid unrelated
load during this matrix. Retain raw JSONL alongside summaries.

Use `analyze.py --host go-01/host --client .local/load/go-01 --plan plan.json
--fixture go-01/load-fixture.json --output result.json` on a machine with both datasets. The analyzer requires a
complete load interval, stable container identity, sufficient samples and matching
counts; it records throttling, peaks and target failures rather than treating
missing counters as zero.

## Public image cache conditions

Only after the sampled Nginx fixture is deleted, the optional command below may
remove the exact unused predeclared CRI image reference. Live Pod identities and
every retained revision of undeleted applications are checked first. It never
removes shared content directly or prunes the store:

```bash
python3 tools/pulls.py "${evaluation_args[@]}" --output pulls-01 \
  --allow-remove-unused-reference
```

Three pull-required/cached pairs retain native manifest/config/layer inventories,
missing compressed layer bytes, unpacked native snapshot-chain identities, Kubernetes pull events and actual image identity.
A missing image reference with all layers present is **not** a fully cold pull;
fully cold, partial-cache and all-content-cached results remain separate. A reset
refusal is evidence and must not be bypassed by broad pruning.

## Human trial

The [short walkthrough](HUMAN.md) compares prepared equivalent Helm and UI tasks.
Record actual observations in a private copy of [human.csv](human.csv). No rows
means pending, never zero seconds. Validate a supplied result with:

```bash
python3 tests/evaluation/human.py --input .local/human.csv --output .local/human-result.json
```

A paired result requires both methods, distinct order positions and valid active/
elapsed time and step counts. The supplied observations remain a single
administrator case study. Automated/backend timings cannot fill the human record.
