# Observability

The monitoring stack is not implemented. Current diagnostics comprise structured
broker logs, persisted operation events/audit rows and cached workload status.
There is no platform metrics endpoint, application log-reading API or deployed
Prometheus/Grafana/Loki stack.

M8 plans Prometheus, Grafana, Loki and Alloy with bounded retention, measured
resource overhead, dashboards and alerts. The lightweight core evaluation can
use timestamps, HTTP observations and host measurements before that stack is
introduced. See [operations](../docs/operations.md#observability-profiles) and
[current status](../docs/status.md). Distributed tracing remains optional.
