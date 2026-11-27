# Human timed walkthrough

Human active time remains **pending** until a completed trial record is supplied.
The trial is scheduled last, after the rollout correction passes published-release
acceptance on the reference Pi.
The CSV template contains no invented measurements. The expected duration is
approximately 10–15 minutes with the environment prepared; passive image pulls
or troubleshooting may extend it.

Preparation is outside timing and must be recorded: a reachable reference
installation, normal certificate/DNS trust, an authenticated UI, a prepared SSH
session with pinned Helm, the released first-party application chart and an
available public/reference image. Use the same immutable image, port, readiness
path, resource settings and cache condition for both methods. Do not copy passwords,
bootstrap tokens, cookies or kubeconfig into a result.

Supply the workload brief to both methods. Chart/tool installation is excluded
for both. Editing the workload values is included for Helm, as entering the same
fields is included for the UI. Prepopulating only one method's workload would
change the comparison and must be disclosed. Use unique `eval-human-*` names;
existing applications remain untouched.

1. Choose the order before starting (coin flip or alternate a previous trial).
   Record the order, release, participant label and whether Helm/UI was previously
   familiar. One administrator is a descriptive case study, not a population sample.
2. Start two timers for the first method: elapsed time and active time. Pause only
   active time during passive rollout waits; resume it for reading decisions,
   entering/editing configuration, issuing actions and diagnosing problems.
3. Deploy the brief's v1 app, verify its HTTP response, update to v2, verify again,
   and delete that same owned app. Keep all errors and retries in the record.
4. Repeat the equivalent task through the other method with the same cache state.
   If the first task warms previously absent content, discard that pair for a fair
   cached comparison and repeat after recording the changed condition.
5. Fill `human.csv` with one row per method. An operator step is one deliberate
   command invocation, field edit/paste, navigation/confirmation, or corrective
   action. Count actions actually performed; do not infer them from the workflow.
   Active seconds must be nonnegative and no greater than elapsed seconds.

The UI uses the documented creation/update/delete pages. Manual Helm uses the
same released chart and a values file matching the brief; the chart's defaults
must not silently differ from the platform values. Record commands/values without
credentials. If the chart/source identity or image/cache differs, label the pair
incomparable (`comparable=false`) instead of declaring a faster method.
Set `comparable=true` only after checking the completed brief for both methods.

Suggested CSV values for categorical fields are `helm`/`ui` for method,
`deploy-update-delete` for task, `1`/`2` for order and `true`/`false` for success.
The `notes` field records familiarity, retries, trust/cache preparation and any
interruption. Do not enter names or other personal information; a neutral
participant label is sufficient.

Automated backend durations and scripted browser interactions are reported
separately. An incomplete/failed human trial remains evidence and cannot be
converted into a successful usability conclusion.

## Prepared task brief

Prepare the following outside the timers and retain the completed brief with the
CSV. The administrator supplies the release/source identity and the v1/v2 image
references from its ARM64 `release.json`; no tag resolution is performed during
the task. Both digests must already be imported by that release installer.

| Setting | Same task for both methods |
| --- | --- |
| Image | Published ARM64 demo v1, followed by demo v2 |
| Port/profile/replicas | 8080 / stateless HTTP / 1 |
| Startup/readiness | TCP startup (the current UI omits a startup path) / HTTP `/readyz` |
| Resources | The selected release's server defaults; explicitly match them in the manual values |
| Exposure | LAN HTTPS on the existing private listener |
| Hostname | A different unused `eval-human-TRIAL-METHOD.apps.home.arpa` per method |
| Verification | `/version` reports v1, then v2; after deletion the removed route returns 404 after bounded convergence |
| Cache | Exact v1/v2 native digests imported and verified before either task |

A template for the manual file is supplied as
[human-values.example.yaml](human-values.example.yaml). It reflects the published v0.1.11 measured profile; for an older release,
adjust the prepared defaults to that release before timing. Editing its empty task
fields is included in active time. The UI's equivalent name/image/hostname entry
is also included. Common profile defaults may be prepared for both methods; note
any difference in the record. Prepare paths to the chart, CA and installed tools
outside timing.

The manual task consists of editing the file and invoking the following actions
in the prepared privileged Pi terminal. `TASK_RELEASE`, `CHART`, `VALUES`, `HOST`
and `CA` are non-secret paths/names from the prepared brief. The chart is the
exact embedded chart from the chosen release source. A repeated existing release
name must be replaced with a new unused name, not overwritten.

```bash
helm_bin=/usr/local/lib/k8s-internal-platform/helm
kubeconfig=/etc/rancher/k3s/k3s.yaml
"$helm_bin" --kubeconfig "$kubeconfig" install "$TASK_RELEASE" "$CHART" \
  --namespace platform-apps --values "$VALUES" --wait --timeout 120s
curl --fail --cacert "$CA" --resolve "$HOST:8443:192.168.68.227" \
  "https://$HOST:8443/version"
# Edit image.digest to the brief's v2 digest; retain all other settings.
"$helm_bin" --kubeconfig "$kubeconfig" upgrade "$TASK_RELEASE" "$CHART" \
  --namespace platform-apps --values "$VALUES" --wait --timeout 120s
curl --fail --cacert "$CA" --resolve "$HOST:8443:192.168.68.227" \
  "https://$HOST:8443/version"
"$helm_bin" --kubeconfig "$kubeconfig" uninstall "$TASK_RELEASE" \
  --namespace platform-apps --wait --timeout 120s
curl --cacert "$CA" --resolve "$HOST:8443:192.168.68.227" \
  --output /dev/null --write-out '%{http_code}\n' "https://$HOST:8443/version"
```

For the UI task, create the equivalent app through **New application**, enter
its v1 image/port, readiness path `/readyz` and LAN hostname, wait for Ready and inspect the
actual `/version` response. Open that application, update the image to v2,
verify Ready and the response, then delete it and verify its removed URL.
The current UI applies server resource defaults and does not expose a resource
editor or startup-path field. The manual values therefore use those exact resource
defaults and the same TCP startup probe. The automated backend comparison uses
explicit HTTP startup/readiness probes for both of its methods; it is a separate
experiment. Use the labels visible in the selected release if wording differs. Authentication,
normal browser DNS/CA trust and connection preparation precede both timers.
Errors, retries and route-convergence waits remain in elapsed time.

Allow approximately 2 minutes for briefing and order selection, 4–5 minutes per
method, and 2 minutes to complete the record. A timeout or debugging interval
may exceed 15 minutes; retain it rather than stopping the timers early.
