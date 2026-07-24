# Theia Grafana Dashboard Guide

This guide explains how to configure and operate the Theia Grafana dashboard in
[`dashboard.json`](dashboard.json). The goal is not only to import a working
dashboard, but to use the existing Prometheus metrics to understand how the
backend behaves under normal load, polling load, SNMP latency, topology refresh
work, WebSocket delivery, bulk backup/download operations, and CPU spikes.

The dashboard is built around Theia's `/metrics` endpoint and the custom metric
families emitted by `internal/observability/registry.go`. It also uses the Go
runtime and process metrics exported by the backend process.

## Source Files

- Dashboard JSON: [`docs/grafana/dashboard.json`](dashboard.json)
- Metric registry and help text: [`internal/observability/registry.go`](../../internal/observability/registry.go)
- Development Prometheus config: [`docker/prometheus/prometheus.yml`](../../docker/prometheus/prometheus.yml)
- Production Prometheus config: [`docker/prometheus/prometheus.prod.yml`](../../docker/prometheus/prometheus.prod.yml)
- Prometheus alert rules: [`docker/prometheus/alert_rules.yml`](../../docker/prometheus/alert_rules.yml)
- Production setup notes: [`SETUP.md`](../../SETUP.md)

## Backend Mental Model

Read the dashboard from left to right and top to bottom as a backend execution
flow:

1. The backend exposes `/metrics`.
2. Prometheus scrapes the backend as `job="theia-backend"`.
3. The scheduler moves due polling tasks into ready queues.
4. Worker goroutines dispatch scheduler tasks by `task_kind` and
   `volatility_class`.
5. Polling tasks call SNMP collectors and optional Prometheus enrichment.
6. Poll results update runtime state, caches, topology observations, links, and
   refresh snapshots.
7. Snapshot and delta payloads are emitted to WebSocket clients.
8. Bulk backup/download operations run through their own concurrency and quota
   controls.
9. The dashboard correlates all of that with CPU, heap, goroutines, and GC.

This order is intentional. When the backend has a spike or degradation, start at
the overview row, then follow the pressure through scheduler, polling, SNMP,
snapshot refresh, WebSocket delivery, and runtime resource panels.

## Prerequisites

### 1. Prometheus Must Scrape Theia

The dashboard expects Theia metrics to be present under the Prometheus job
`theia-backend`.

The development scrape config defines:

```yaml
scrape_configs:
  - job_name: theia-backend
    metrics_path: /metrics
    static_configs:
      - targets:
          - backend:8080
```

The production scrape config uses the same target and endpoint, but adds bearer
token authentication:

```yaml
scrape_configs:
  - job_name: theia-backend
    metrics_path: /metrics
    authorization:
      type: Bearer
      credentials_file: /run/secrets/theia_metrics_token
    static_configs:
      - targets:
          - backend:8080
```

Start the metrics stack with:

```bash
make prod-metrics
```

Then check Prometheus targets:

```text
http://localhost:9090/targets
```

The `theia-backend` target must be `UP`. If it is `DOWN`, fix Prometheus
scraping before working on Grafana.

### 2. Prometheus Must Have Theia Series

Open the Prometheus expression browser and verify these queries return data:

```promql
up{job="theia-backend"}
```

```promql
theia_scheduler_in_flight_tasks{job="theia-backend"}
```

```promql
process_cpu_seconds_total{job="theia-backend"}
```

The dashboard variables are anchored on Theia metrics. If
`theia_scheduler_in_flight_tasks` is missing, the `job` and `instance` variables
will be empty.

### 3. Grafana Must Have A Prometheus Data Source

Create a Grafana Prometheus data source that points at the same Prometheus
server that scrapes Theia. In a local Compose stack, this is usually:

```text
http://prometheus:9090
```

If Grafana runs outside the Compose network, use a host-reachable URL instead,
for example:

```text
http://localhost:9090
```

The Grafana data source is separate from Theia's own `prometheus_url` setting.
Theia's setting controls backend runtime enrichment and alert reads. Grafana's
data source controls dashboard queries.

## Import The Existing Dashboard

1. Open Grafana.
2. Go to **Dashboards**.
3. Select **New**.
4. Select **Import**.
5. Upload [`docs/grafana/dashboard.json`](dashboard.json).
6. When Grafana asks for `DS_PROMETHEUS`, select the Prometheus data source that
   can query `theia-backend`.
7. Import the dashboard.
8. Set the time range to `Last 1 hour`.
9. Use a refresh interval such as `10s`, `15s`, or `30s`.

The dashboard title is `Theia Metrics`, and the exported dashboard UID is
`pr5vjtg`.

For file provisioning, use the same JSON but make sure the Prometheus data source
UID used by Grafana matches the dashboard's `${DS_PROMETHEUS}` input mapping, or
replace `${DS_PROMETHEUS}` with the provisioned data source UID.

## Dashboard Variables

The dashboard uses variables so the same panels can inspect one backend instance
or a fleet of instances. Most variables are multi-select with an `All` value of
`.*`, so panel queries must use Prometheus regex matchers (`=~`) instead of exact
matchers (`=`).

| Variable | Label | Query | Purpose | Current behavior |
| --- | --- | --- | --- | --- |
| `job` | Job | `label_values(theia_scheduler_in_flight_tasks,job)` | Select the Prometheus job that exposes Theia backend metrics. | Single-select. Usually `theia-backend`. |
| `instance` | Instance | `label_values(theia_scheduler_in_flight_tasks{job="$job"},instance)` | Select the backend scrape target. | Single-select. Usually `backend:8080` in Compose. |
| `volatility_class` | Volatility class | `label_values(theia_poll_results_total{job=~"$job", instance=~"$instance"},volatility_class)` | Filter scheduler and polling panels by task volatility. | Multi-select, `All` = `.*`. |
| `task_kind` | Task kind | `label_values(theia_scheduler_task_dispatch_total{job=~"$job", instance=~"$instance"},task_kind)` | Filter scheduler dispatch panels by task type. | Multi-select, `All` = `.*`. |
| `collector` | SNMP collector | `label_values(theia_snmp_collector_operations_total{job=~"$job", instance=~"$instance"},collector)` | Filter internal Theia SNMP collector metrics. | Multi-select, `All` = `.*`. |
| `operation` | SNMP operation | `label_values(theia_snmp_collector_operations_total{job=~"$job", instance=~"$instance", collector=~"$collector"},operation)` | Filter SNMP collector operation labels. | Multi-select, `All` = `.*`. |
| `result` | Result | `label_values(theia_snmp_collector_operations_total{job=~"$job", instance=~"$instance"},result)` | Filter success/error/timeout style results. | Multi-select, `All` = `.*`. |
| `ws_scope` | WS scope | `label_values(theia_ws_messages_total{job=~"$job", instance=~"$instance"},scope)` | Filter WebSocket broadcast/unicast/detail/overview style scopes. | Multi-select, `All` = `.*`. |
| `bulk_operation` | Bulk operation | `label_values(theia_bulk_operation_completions_total{job=~"$job", instance=~"$instance"},operation)` | Filter bulk backup/download operation panels. | Multi-select, `All` = `.*`. |

### Variable Rules

Use this pattern for variables that can be multi-select:

```promql
metric_name{
  job=~"$job",
  instance=~"$instance",
  label_name=~"$variable"
}
```

Do not use this pattern for multi-select variables:

```promql
metric_name{label_name="$variable"}
```

The exact matcher works only when the variable expands to one literal value. It
breaks when the value is `.*` or a regex built from multiple selected values.

If you make `job` multi-select later, update the `instance` variable query to:

```promql
label_values(theia_scheduler_in_flight_tasks{job=~"$job"},instance)
```

## PromQL Conventions Used By The Dashboard

### Gauges

Gauges represent current values. Query them directly:

```promql
theia_scheduler_in_flight_tasks{job=~"$job", instance=~"$instance"}
```

Use gauges for current queue depth, in-flight work, connected WebSocket clients,
heap, goroutines, and state flags.

### Counters

Counters only increase until the process restarts. Use `rate()` for per-second
velocity:

```promql
rate(theia_scheduler_task_dispatch_total{job=~"$job"}[$__rate_interval])
```

Use `increase()` when you want "how many events happened in this range":

```promql
increase(theia_polling_deadline_miss_total{job=~"$job"}[$__rate_interval])
```

### Histograms

Histograms emit `_bucket`, `_sum`, and `_count` series. Use
`histogram_quantile()` with `rate()` and keep `le` in the grouping:

```promql
histogram_quantile(
  0.95,
  sum by (le, volatility_class) (
    rate(theia_scheduler_task_duration_seconds_bucket{
      job=~"$job",
      instance=~"$instance",
      volatility_class=~"$volatility_class"
    }[$__rate_interval])
  )
)
```

Use `_sum / _count` for average duration:

```promql
sum(rate(metric_seconds_sum[$__rate_interval]))
/
clamp_min(sum(rate(metric_seconds_count[$__rate_interval])), 0.001)
```

`clamp_min()` prevents divide-by-zero spikes when there are no samples in the
selected time range.

### `$__rate_interval`

Use Grafana's `$__rate_interval` for most `rate()` and `increase()` windows. It
adapts to the dashboard time range and Prometheus scrape interval, which is safer
than hard-coding a short range everywhere.

Use a fixed short range such as `[30s]` only for burst detection or CPU
correlation panels where the goal is to show short-lived spikes.

## Metric Families And Backend Subsystems

| Subsystem | Metric families | What they explain |
| --- | --- | --- |
| Runtime process | `process_*`, `go_*` | CPU, heap, goroutines, GC behavior. |
| Scheduler | `theia_scheduler_*`, `theia_polling_*`, `theia_poll_results_total` | Task dispatch, queue pressure, deadline misses, polling success/failure. |
| SNMP collection | `theia_snmp_collector_*` | Internal Theia SNMP operation volume, latency, errors, timeouts, and early exits. |
| Topology and refresh | `theia_refresh_*`, `theia_topology_materialization_*`, `theia_link_upserts_total`, `theia_unknown_neighbors_total`, `theia_static_collection_skips_total`, `theia_static_persistence_skips_total` | Snapshot cost, topology reload churn, link discovery behavior, and static discovery timeout skip behavior. |
| WebSocket delivery | `theia_ws_*` | Connected clients, message rates, payload sizes, backpressure, resync pressure. |
| Bulk operations | `theia_bulk_operation_*` | Backup/download throughput, duration, rejection reasons, concurrency pressure. |
| Prometheus runtime integration | `theia_prometheus_runtime_*` | Backend calls from Theia to Prometheus for runtime data, alerts, or enrichment. |
| Cache and state store | `theia_cache_*`, `theia_state_changes_dropped_total` | Cache invalidation/reload behavior and dropped state-store change batches. |

Important distinction: the Prometheus `snmp` job is the external SNMP exporter
scraping device metrics such as `ifHCInOctets`. The
`theia_snmp_collector_*` metrics are emitted by the Theia backend itself and
describe Theia's internal SNMP collection path. Device-aware SNMP panels require
`theia_snmp_collector_device_operation_last_duration_seconds` and
`theia_snmp_collector_device_slow_operations_total`, which are emitted by the
backend collector instrumentation.

## Row 1: Executive Overview

Use this row first. It answers: "Is the backend busy, and is the scheduler under
pressure right now?"

### CPU Usage

Panel type: Time series  
Unit: Percent

```promql
rate(process_cpu_seconds_total{job=~"$job", instance=~"$instance"}[$__rate_interval]) * 100
```

This shows process CPU time consumed per second as a percentage. Short spikes can
be normal during polling bursts, topology materialization, snapshot builds, or
bulk operations. Sustained high CPU should be correlated with the scheduler,
SNMP, WebSocket, and snapshot rows.

Normal behavior:

- Low baseline when the system is idle.
- Short peaks during active polling or refresh cycles.

Concerning behavior:

- CPU spikes align with growing queue lag.
- CPU spikes align with SNMP p95/p99 latency.
- CPU spikes align with full topology reloads or snapshot build latency.
- CPU stays high while dispatch rate is low, which can indicate GC, blocking,
  oversized payload work, or unrelated runtime pressure.

### Scheduler Dispatch Rate

Panel type: Time series  
Unit: Operations per second

```promql
sum by (task_kind, volatility_class) (
  rate(theia_scheduler_task_dispatch_total{
    job=~"$job",
    instance=~"$instance",
    task_kind=~"$task_kind",
    volatility_class=~"$volatility_class"
  }[$__rate_interval])
)
```

This shows how much work the scheduler is actually dispatching. The labels let
you separate high-frequency performance polling from slower static/topology
work.

Interpretation:

- High dispatch with stable queue lag means the backend is keeping up.
- High dispatch with rising queue lag means work is arriving faster than workers
  can complete it.
- Low dispatch with high ready queue depth can indicate backpressure,
  concurrency limits, blocked workers, or no available worker capacity.

### Scheduler In-Flight Tasks

Panel type: Time series

```promql
theia_scheduler_in_flight_tasks{
  job=~"$job",
  instance=~"$instance"
}
```

This is the current number of scheduler tasks running. It is a gauge.

Interpretation:

- Stable in-flight values near worker capacity usually mean the backend is busy.
- In-flight stuck high while dispatch falls can mean tasks are slow, blocked, or
  timing out.
- In-flight near zero while queue depth rises can indicate scheduling or worker
  starvation.

### Essential Overloaded

Panel type: Stat

```promql
theia_polling_essential_overloaded{
  job=~"$job",
  instance=~"$instance"
}
```

This is a boolean-like gauge. `0` means the essential polling lane is not
overloaded. `1` means the essential lane is overloaded.

Use it as an immediate signal that essential polling freshness is at risk.

### Deadline Misses

Panel type: Time series

```promql
increase(theia_polling_deadline_miss_total{
  job=~"$job",
  instance=~"$instance"
}[$__rate_interval])
```

This shows essential polling deadline misses in the selected rate window.

Interpretation:

- Zero is expected in a healthy system.
- Any recurring value means essential polling is failing freshness targets.
- Pair this with queue lag, in-flight tasks, SNMP latency, and backpressure to
  identify whether the cause is scheduling pressure or slow collection.

### Backend Scrape Health

Panel type: Stat

```promql
up{
  job=~"$job",
  instance=~"$instance"
}
```

This shows Prometheus target health for the selected backend. `1` means
Prometheus can scrape Theia's `/metrics` endpoint. `0` means the target is down
or the scrape is failing.

### Prometheus Scrape Duration

Panel type: Time series  
Unit: Seconds

```promql
scrape_duration_seconds{
  job=~"$job",
  instance=~"$instance"
}
```

This shows how long Prometheus spends scraping the selected backend target.
Rising scrape duration can point to expensive metrics collection, target
pressure, or Prometheus-side scrape problems.

## Row 2: Scheduler And Queue Health

Use this row to understand whether polling work is queued, delayed, slow, or
being intentionally throttled.

### Ready Queue Depth By Class

Panel type: Time series

```promql
theia_scheduler_ready_queue_depth{
  job=~"$job",
  instance=~"$instance",
  volatility_class=~"$volatility_class"
}
```

This gauge shows how many tasks are ready to dispatch by volatility class.

Interpretation:

- Short bursts are normal when many devices become due at the same time.
- Persistent depth means worker capacity, task duration, or backpressure is
  limiting progress.
- If only one volatility class grows, focus on that class's collectors and task
  kinds.

### Queue Lag By Class

Panel type: Time series

```promql
theia_scheduler_queue_lag_seconds{
  job=~"$job",
  instance=~"$instance",
  volatility_class=~"$volatility_class"
}
```

Queue lag is the overdue time for queued work. It is one of the best scheduler
health indicators.

Interpretation:

- Near zero means scheduled work is close to its intended cadence.
- Rising lag means tasks are missing their intended execution time.
- Lag above the alert threshold in `alert_rules.yml` currently indicates
  `SchedulerQueueLagHigh`.

### Scheduler Task Duration p95 And p99

Panel type: Time series

p95:

```promql
histogram_quantile(
  0.95,
  sum by (le, volatility_class) (
    rate(theia_scheduler_task_duration_seconds_bucket{
      job=~"$job",
      instance=~"$instance",
      volatility_class=~"$volatility_class"
    }[$__rate_interval])
  )
)
```

p99 uses the same expression with `0.99`.

These panels show task completion latency by volatility class. Use p95 for
regular operational behavior and p99 for tail latency.

Interpretation:

- High duration plus high SNMP latency points to collector/device/network
  slowness.
- High duration plus normal SNMP latency can point to topology persistence,
  snapshot building, WebSocket serialization, Prometheus enrichment, or database
  work around the task.
- High duration with rising in-flight tasks usually explains queue lag.

### Scheduler Backpressure By Reason

Panel type: Time series

```promql
sum by (volatility_class, reason) (
  rate(theia_scheduler_backpressure_total{
    job=~"$job",
    instance=~"$instance",
    volatility_class=~"$volatility_class"
  }[$__rate_interval])
)
```

Backpressure events explain why dispatch is being limited. Reasons come from the
scheduler and include class, profile, essential-lane, and global limit paths.

Interpretation:

- Backpressure with stable queue lag can be healthy throttling.
- Backpressure with rising queue lag means the limits are protecting the backend
  but data freshness is degrading.
- Use the `reason` label to decide whether to tune global worker capacity,
  per-class limits, essential polling capacity, or task profile limits.

### Dispatch Heatmap

Panel type: Heatmap

```promql
sum by (task_kind, volatility_class) (
  increase(theia_scheduler_task_dispatch_total{
    job=~"$job",
    instance=~"$instance"
  }[30s])
)
```

This panel highlights short bursts by task kind and volatility class.

Use it to detect synchronized polling waves. If CPU spikes happen every fixed
interval and the heatmap lights up at the same time, investigate timer alignment,
poll interval settings, jitter, and device batches.

## Row 3: Polling Results

Use this row to answer: "Are scheduled polls succeeding, or is the backend
spending time on failing work?"

### Poll Failures

Panel type: Time series  
Unit: Operations per second

```promql
sum by (volatility_class, outcome) (
  rate(theia_poll_results_total{
    job=~"$job",
    instance=~"$instance",
    volatility_class=~"$volatility_class"
  }[$__rate_interval])
)
```

This includes both success and failure outcomes. Keep both visible because a
failure rate only makes sense relative to total poll volume.

### Poll Failure Ratio

Panel type: Time series  
Unit: Percent

```promql
100 *
sum by (volatility_class) (
  rate(theia_poll_results_total{
    job=~"$job",
    instance=~"$instance",
    outcome="failure",
    volatility_class=~"$volatility_class"
  }[$__rate_interval])
)
/
clamp_min(
  sum by (volatility_class) (
    rate(theia_poll_results_total{
      job=~"$job",
      instance=~"$instance",
      volatility_class=~"$volatility_class"
    }[$__rate_interval])
  ),
  0.001
)
```

Interpretation:

- A low, occasional failure ratio can be normal for unreachable devices or
  transient SNMP errors.
- A sustained rise means freshness and topology accuracy may degrade.
- If failures correlate with SNMP timeouts, inspect device reachability,
  collector operations, SNMP timeout/retry settings, and network latency.
- If failures correlate with deadline misses but not SNMP errors, inspect
  scheduler pressure and worker saturation.

## Row 4: SNMP Collector

Use this row to understand Theia's internal SNMP collection work. These metrics
are about the backend's collector code, not the external Prometheus `snmp` job.

### SNMP Operations Rate

Panel type: Time series  
Unit: Operations per second

```promql
sum by (collector, operation, result) (
  rate(theia_snmp_collector_operations_total{
    job=~"$job",
    instance=~"$instance",
    collector=~"$collector",
    operation=~"$operation",
    result=~"$result"
  }[$__rate_interval])
)
```

Interpretation:

- High operation rate with low latency is healthy polling throughput.
- High operation rate with rising timeout/error results indicates devices or
  network paths are causing expensive failed work.
- Operation labels let you identify broad walks versus smaller gets.

### SNMP Early Exits

Panel type: Time series

```promql
sum by (collector, reason) (
  rate(theia_snmp_collector_early_exit_total{
    job=~"$job",
    instance=~"$instance",
    collector=~"$collector"
  }[$__rate_interval])
)
```

Early exits are useful because they show work that stopped before completing the
full collection path. For example, a failed probe before a broader walk can
reduce load but also indicates missing data.

### SNMP Errors And Timeouts

Panel type: Time series

```promql
sum by (collector, operation, result) (
  rate(theia_snmp_collector_operations_total{
    job=~"$job",
    instance=~"$instance",
    collector=~"$collector",
    operation=~"$operation",
    result=~"$result",
    result!="success"
  }[$__rate_interval])
)
```

This is the true per-second error/timeout rate. It intentionally does not apply
visual multipliers, so the values can be compared directly with operation rate.

### SNMP Latency p95 And p99

Panel type: Time series  
Unit: Seconds

p95:

```promql
histogram_quantile(
  0.95,
  sum by (le, collector, operation, result) (
    rate(theia_snmp_collector_operation_seconds_bucket{
      job=~"$job",
      instance=~"$instance",
      collector=~"$collector",
      operation=~"$operation",
      result=~"$result"
    }[$__rate_interval])
  )
)
```

p99 uses the same expression with `0.99`.

Interpretation:

- High p95 means common operations are slow.
- High p99 with normal p95 means tail devices or rare operations are slow.
- Slow SNMP plus rising scheduler task duration usually means collectors are the
  scheduler bottleneck.

### SNMP Operation Duration Average

Panel type: Time series  
Unit: Seconds

```promql
sum by (collector, operation, result) (
  rate(theia_snmp_collector_operation_seconds_sum{
    job=~"$job",
    instance=~"$instance",
    collector=~"$collector",
    operation=~"$operation",
    result=~"$result"
  }[$__rate_interval])
)
/
clamp_min(
  sum by (collector, operation, result) (
    rate(theia_snmp_collector_operation_seconds_count{
      job=~"$job",
      instance=~"$instance",
      collector=~"$collector",
      operation=~"$operation",
      result=~"$result"
    }[$__rate_interval])
  ),
  0.001
)
```

Use this with p95/p99. Average duration can hide tail latency, but it is useful
for estimating total collector cost.

## Row 5: WebSocket And Realtime UX

Use this row to understand backend-to-browser realtime delivery.

### WebSocket Connected Clients

Panel type: Stat

```promql
theia_ws_connected_clients{
  job=~"$job",
  instance=~"$instance"
}
```

This gauge shows current connected WebSocket clients. Client count matters
because payload serialization and broadcast fanout can increase CPU pressure.

### WebSocket Messages Rate

Panel type: Time series

```promql
sum by (scope, type) (
  rate(theia_ws_messages_total{
    job=~"$job",
    instance=~"$instance",
    scope=~"$ws_scope"
  }[$__rate_interval])
)
```

Interpretation:

- Message bursts after polling batches are expected.
- Very high message rates with high CPU can indicate too many deltas, repeated
  resyncs, or too many connected clients.
- Correlate with snapshot build p95 and payload size p95.

### WebSocket Payload Size p95

Panel type: Time series  
Unit: Bytes

```promql
histogram_quantile(
  0.95,
  sum by (le, scope, type) (
    rate(theia_ws_message_payload_bytes_bucket{
      job=~"$job",
      instance=~"$instance",
      scope=~"$ws_scope"
    }[$__rate_interval])
  )
)
```

Interpretation:

- Large payloads can explain CPU spikes, heap growth, GC activity, and slow
  browser updates.
- If payload p95 rises with snapshot build p95, inspect full snapshot frequency
  and topology churn.

### WebSocket Backpressure

Panel type: Time series

```promql
sum by (scope, reason) (
  rate(theia_ws_backpressure_total{
    job=~"$job",
    instance=~"$instance"
  }[$__rate_interval])
)
or on() vector(0)
```

Backpressure means WebSocket client or hub buffers could not keep up with the
backend's realtime update stream. This panel intentionally does not use
`$ws_scope` because backpressure scopes are `broadcast`, `client_send`, and
`overview_send`, while `$ws_scope` is populated from message-emission scopes.
The final `or on() vector(0)` renders healthy zero-event periods as `0` instead
of `No data`.

### WebSocket Resync Required

Panel type: Time series

```promql
sum by (scope, reason, bootstrap) (
  rate(theia_ws_client_resync_required_total{
    job=~"$job",
    instance=~"$instance"
  }[$__rate_interval])
)
or on() vector(0)
```

Resync markers mean clients need to recover from missed incremental updates.
This panel also avoids `$ws_scope` because client-resync scope currently uses
`overview`, which is distinct from normal message scopes.

### WebSocket Overview Mailbox Clears

Panel type: Time series  
Unit: Messages/sec

```promql
sum by (reason) (
  rate(theia_ws_overview_mailbox_clear_total{
    job=~"$job",
    instance=~"$instance"
  }[$__rate_interval])
)
or on() vector(0)
```

Mailbox clears count messages removed from the overview mailbox while stale
backlog is replaced with fresher state. The unit is messages/sec because the
counter is incremented by the number of messages cleared, not by the number of
clear operations.

### WebSocket Overview Resync Suppressed

Panel type: Time series

```promql
sum by (reason) (
  rate(theia_ws_overview_resync_suppressed_total{
    job=~"$job",
    instance=~"$instance"
  }[$__rate_interval])
)
or on() vector(0)
```

Suppression means the backend avoided sending redundant overview resync markers
while one was already pending. Missing rare-event series are rendered as zero.

## Row 6: Snapshot And Topology Refresh

Use this row to understand how much work is caused by topology materialization,
snapshot construction, link updates, and unknown neighbor observations.

### Snapshot Build p95

Panel type: Time series  
Unit: Seconds

```promql
histogram_quantile(
  0.95,
  sum by (le, mode, result) (
    rate(theia_refresh_snapshot_build_seconds_bucket{
      job=~"$job",
      instance=~"$instance"
    }[$__rate_interval])
  )
)
```

Interpretation:

- Spikes here mean building realtime snapshots is expensive.
- If this correlates with CPU and heap growth, inspect snapshot size and full
  snapshot frequency.
- If `result="error"` appears, check backend logs and refresh code paths.

### Full Topology Reload Rate

Panel type: Time series

```promql
sum by (reason) (
  rate(theia_refresh_topology_reload_total{
    job=~"$job",
    instance=~"$instance"
  }[$__rate_interval])
)
```

This shows why the backend decides to reload full topology state.

Interpretation:

- Startup reloads are expected.
- Frequent reloads during normal operation can cause snapshot and CPU pressure.
- Use the `reason` label to identify dirty topology paths or explicit reloads.

### Link Upserts

Panel type: Time series

```promql
sum by (protocol, result) (
  rate(theia_link_upserts_total{
    job=~"$job",
    instance=~"$instance"
  }[$__rate_interval])
)
```

This shows link persistence churn by discovery protocol and result.

Interpretation:

- Link upserts after topology discovery are expected.
- High repeated upserts can indicate unstable discovery data or reconciliation
  churn.

### Unknown Neighbors

Panel type: Time series

```promql
sum by (protocol) (
  rate(theia_unknown_neighbors_total{
    job=~"$job",
    instance=~"$instance"
  }[$__rate_interval])
)
```

Unknown neighbors indicate discovery observations that Theia cannot map to known
devices.

Interpretation:

- Occasional unknown neighbors can be normal in incomplete inventories.
- Sustained unknown neighbor rates can explain repeated topology work and should
  be investigated by adding missing devices or fixing discovery identifiers.

### Topology Materialization p95

Panel type: Time series  
Unit: Seconds

```promql
histogram_quantile(
  0.95,
  sum by (le, result) (
    rate(theia_topology_materialization_seconds_bucket{
      job=~"$job",
      instance=~"$instance"
    }[$__rate_interval])
  )
)
```

Interpretation:

- High materialization duration means static discovery persistence is expensive.
- If materialization grows with link upserts and unknown neighbors, topology
  discovery is likely contributing to load.

### Topology Materialization Skips

Panel type: Time series  
Unit: Operations per second

```promql
sum by (reason) (
  rate(theia_topology_materialization_skips_total{
    job=~"$job",
    instance=~"$instance",
    reason="unchanged"
  }[$__rate_interval])
)
or on() vector(0)
```

This counter shows static discovery persistence runs where Theia updated
non-topology static data but skipped canonical topology materialization.
`reason="unchanged"` means the topology fingerprint did not change, so the
backend avoided reprocessing observations and links.

Interpretation:

- A steady `unchanged` rate is usually healthy: static persistence is still
  running, but topology discovery input is stable.
- Zero can be normal when static polling is idle or when every static poll
  changes topology-relevant input.
- If this drops while topology materialization p95, link upserts, or full
  topology reloads rise, topology inputs may be changing too often.

### Discovery Neighbors

Panel type: Time series

```promql
sum by (protocol) (
  theia_discovery_neighbors{
    job=~"$job",
    instance=~"$instance"
  }
)
```

This shows the current discovered neighbor count by discovery protocol. Use it
with unknown neighbors, link upserts, and topology materialization duration to
understand whether topology discovery is stable or constantly changing.

### Static Collection Timeout Skips

Panel type: Time series  
Unit: Operations per second

```promql
sum by (operation, reason) (
  rate(theia_static_collection_skips_total{
    job=~"$job",
    instance=~"$instance"
  }[$__rate_interval])
)
or on() vector(0)
```

This counter shows static discovery collection operations that were skipped
instead of retried immediately. The current timeout-skip implementation emits
`reason="cooldown"` after a previous timeout puts an optional static collection
operation into a cooldown window.

Important labels:

- `operation`: the skipped static table walk, such as `if_descr_walk`,
  `if_name_walk`, or `if_high_speed_walk`.
- `reason`: why the operation was skipped. `cooldown` means the backend is
  intentionally avoiding repeated timeout-heavy work.

Interpretation:

- Low or zero values mean the static collector is not suppressing optional table
  work.
- A sustained `cooldown` rate means one or more static table walks recently
  timed out and the backend is protecting the polling lane from repeated slow
  retries.
- Correlate this panel with SNMP timeout percentage, slow SNMP device tables,
  scheduler task duration, and poll failure ratio.

### Static Persistence Skips

Panel type: Time series  
Unit: Operations per second

```promql
sum by (reason) (
  rate(theia_static_persistence_skips_total{
    job=~"$job",
    instance=~"$instance"
  }[$__rate_interval])
)
or on() vector(0)
```

This counter shows static discovery persistence work that the backend skipped
before writing topology or device state. `reason="unchanged"` means the static
discovery fingerprint matched the last persisted result, so Theia avoided a
redundant persistence pass. `reason="self_heal_deferred"` means the result was
unchanged but the periodic self-heal persistence window has not reached its
jittered deadline yet.

Interpretation:

- A steady `unchanged` rate is usually healthy dedupe: static polls are running,
  but they are not producing new topology or device facts.
- A `self_heal_deferred` rate means Theia is intentionally spreading unchanged
  self-heal persistence work over time instead of running it immediately for
  every unchanged static poll.
- A drop to zero can be normal if no static polls are running in the selected
  range, but compare with scheduler dispatch and poll result panels.
- If persistence skips disappear while topology materialization, link upserts,
  or full topology reloads rise, static discovery input may be changing more
  often than expected.

## Row 7: CPU Correlations

Use these panels to test hypotheses about CPU spikes. They intentionally overlay
CPU with one candidate cause at a time.

### CPU vs Scheduler Dispatch

CPU:

```promql
rate(process_cpu_seconds_total{
  job=~"$job",
  instance=~"$instance"
}[30s]) * 100
```

Dispatch:

```promql
sum(rate(theia_scheduler_task_dispatch_total{
  job=~"$job",
  instance=~"$instance",
  task_kind=~"$task_kind",
  volatility_class=~"$volatility_class"
}[30s]))
```

If CPU spikes align with dispatch bursts, the scheduler is triggering the work.
Then check which task kinds and volatility classes are active.

### CPU vs In-Flight Tasks

```promql
theia_scheduler_in_flight_tasks{
  job=~"$job",
  instance=~"$instance"
}
```

If CPU rises when in-flight tasks reach a plateau, worker concurrency is likely
driving CPU consumption.

### CPU vs Queue Lag

```promql
max(theia_scheduler_queue_lag_seconds{
  job=~"$job",
  instance=~"$instance",
  volatility_class=~"$volatility_class"
})
```

If CPU spikes do not reduce queue lag, the backend may be spending CPU on work
that is not improving freshness fast enough.

### CPU vs Snapshot Build

```promql
histogram_quantile(
  0.95,
  sum by (le, mode) (
    rate(theia_refresh_snapshot_build_seconds_bucket{
      job=~"$job",
      instance=~"$instance",
      result="success"
    }[5m])
  )
)
```

If CPU aligns with snapshot p95, inspect full snapshot frequency, topology
churn, payload size, and WebSocket fanout.

### CPU vs SNMP Latency p95

```promql
histogram_quantile(
  0.95,
  sum by (le) (
    rate(theia_snmp_collector_operation_seconds_bucket{
      job=~"$job",
      instance=~"$instance",
      collector=~"$collector",
      operation=~"$operation",
      result=~"$result"
    }[5m])
  )
)
```

If CPU and SNMP latency rise together, slow collection work may be blocking
workers and increasing queue pressure.

### CPU vs SNMP Operations

```promql
sum(rate(theia_snmp_collector_operations_total{
  job=~"$job",
  instance=~"$instance",
  collector=~"$collector",
  operation=~"$operation",
  result=~"$result"
}[30s]))
```

If CPU aligns with operation volume but latency is stable, the system may simply
be doing more polling work at once.

### CPU vs WebSocket Messages

```promql
sum(rate(theia_ws_messages_total{
  job=~"$job",
  instance=~"$instance",
  scope=~"$ws_scope"
}[30s]))
```

If CPU aligns with WebSocket message volume, inspect payload size and connected
clients.

### CPU vs GC

```promql
go_memstats_gc_cpu_fraction{
  job=~"$job",
  instance=~"$instance"
} * 100
```

If GC CPU rises with backend CPU, inspect heap allocation, snapshot payloads, and
large temporary allocations.

### Heap And Goroutines

Heap:

```promql
go_memstats_heap_alloc_bytes{
  job=~"$job",
  instance=~"$instance"
}
```

Goroutines:

```promql
go_goroutines{
  job=~"$job",
  instance=~"$instance"
}
```

Interpretation:

- Heap spikes with snapshot or WebSocket payload size point to serialization or
  clone pressure.
- Goroutine growth that never returns to baseline can indicate leaks or blocked
  work.

## Row 8: SNMP Tables

Use these table panels for an operator-friendly breakdown of high-volume or slow
SNMP operations.

### SNMP Operations Per Volume

Panel type: Table

```promql
sum by (operation, result) (
  rate(theia_snmp_collector_operations_total{
    job=~"$job",
    instance=~"$instance",
    collector=~"$collector",
    operation=~"$operation",
    result=~"$result"
  }[5m])
)
```

Use this to identify which operations dominate collector volume.

### Slow SNMP Walks

Panel type: Table  
Unit: Seconds

```promql
histogram_quantile(
  0.95,
  sum by (le, operation) (
    rate(theia_snmp_collector_operation_seconds_bucket{
      job=~"$job",
      instance=~"$instance",
      collector=~"$collector",
      operation=~"$operation",
      result=~"$result"
    }[5m])
  )
)
```

Use this to identify operations with high p95 latency.

### SNMP Walks Timeout

Panel type: Table

```promql
100 *
(
  sum by (operation)(
    rate(theia_snmp_collector_operations_total{
      job=~"$job",
      instance=~"$instance",
      collector=~"$collector",
      operation=~"$operation",
      result="timeout"
    }[5m])
  )
)
/
clamp_min(
  sum by (operation)(
    rate(theia_snmp_collector_operations_total{
      job=~"$job",
      instance=~"$instance",
      collector=~"$collector",
      operation=~"$operation"
    }[5m])
  ),
  0.001
)
```

The expression is scoped by dashboard variables so it does not mix unrelated
jobs, instances, collectors, or operations.

### Slow SNMP Devices

Panel type: Table  
Unit: Seconds

```promql
topk(
  20,
  max by (device_id, device, target, collector, operation, result) (
    theia_snmp_collector_device_operation_last_duration_seconds{
      job=~"$job",
      instance=~"$instance",
      collector=~"$collector",
      operation=~"$operation",
      result=~"$result"
    }
  )
)
```

Use this to identify which devices most recently produced the slowest SNMP
operations. The `device` label is Theia's display name fallback, `target` is the
SNMP target address, and `device_id` disambiguates duplicated names.

### SNMP Device Slow Events

Panel type: Table

```promql
topk(
  20,
  sum by (device_id, device, target, collector, operation, result) (
    rate(theia_snmp_collector_device_slow_operations_total{
      job=~"$job",
      instance=~"$instance",
      collector=~"$collector",
      operation=~"$operation",
      result=~"$result"
    }[$__rate_interval])
  )
)
```

Use this to find devices repeatedly crossing the backend slow-operation
threshold. This table appears only after at least one slow operation has been
recorded for a device and operation label set.

## Row 9: Bulk Operations And Backup

Use this row to understand backup/download run pressure, completion rate,
duration, active work, and rejected requests.

### Bulk Operation Duration p95

Panel type: Time series  
Unit: Seconds

```promql
histogram_quantile(
  0.95,
  sum by (le, operation, source, result) (
    rate(theia_bulk_operation_duration_seconds_bucket{
      job=~"$job",
      instance=~"$instance",
      operation=~"$bulk_operation"
    }[$__rate_interval])
  )
)
```

### Bulk Operation Completions

Panel type: Time series

```promql
sum by (operation, source, result) (
  rate(theia_bulk_operation_completions_total{
    job=~"$job",
    instance=~"$instance",
    operation=~"$bulk_operation"
  }[$__rate_interval])
)
```

### Bulk Operations In Flight

Panel type: Time series

```promql
theia_bulk_operation_in_flight{
  job=~"$job",
  instance=~"$instance",
  operation=~"$bulk_operation"
}
```

### Bulk Operation Rejections

Panel type: Time series

```promql
sum by (operation, reason, source) (
  rate(theia_bulk_operation_rejections_total{
    job=~"$job",
    instance=~"$instance",
    operation=~"$bulk_operation"
  }[$__rate_interval])
)
```

### Bulk Operation Saturation

Panel type: Time series  
Unit: Percent

```promql
theia_bulk_operation_in_flight{
  job=~"$job",
  instance=~"$instance",
  operation=~"$bulk_operation"
}
/
on(job, instance, operation, source)
group_left(scope)
theia_bulk_operation_concurrency_limit{
  job=~"$job",
  instance=~"$instance",
  operation=~"$bulk_operation",
  scope="global"
}
```

Interpretation:

- Values near `1` mean the selected bulk operation is at the configured global
  concurrency limit.
- Saturation with rejections means the concurrency limit is actively protecting
  the backend.

## Row 10: Prometheus Runtime Integration

These panels describe Theia's own calls to Prometheus. They do not describe
Grafana's queries. They are useful when the backend uses Prometheus for runtime
enrichment, alert reads, or device metrics.

### Prometheus Runtime Requests

Panel type: Time series

```promql
sum by (operation, result) (
  rate(theia_prometheus_runtime_requests_total{
    job=~"$job",
    instance=~"$instance"
  }[$__rate_interval])
)
```

Interpretation:

- `operation` identifies backend Prometheus use cases such as query or alert
  reads.
- `result` separates success from error categories.

### Prometheus Runtime Errors

Panel type: Time series

```promql
sum by (operation, result) (
  rate(theia_prometheus_runtime_requests_total{
    job=~"$job",
    instance=~"$instance",
    result!="success"
  }[$__rate_interval])
)
```

If this rises, check Theia's Prometheus URL setting, Prometheus availability,
query latency, and network connectivity from the backend container.

### Prometheus Runtime Latency p95

Panel type: Time series  
Unit: Seconds

```promql
histogram_quantile(
  0.95,
  sum by (le, operation, result) (
    rate(theia_prometheus_runtime_request_seconds_bucket{
      job=~"$job",
      instance=~"$instance"
    }[$__rate_interval])
  )
)
```

If runtime Prometheus latency aligns with scheduler task duration, Prometheus
enrichment may be contributing to polling cost.

## Row 11: Cache And State Store

Use this row to understand whether persistence or state propagation is causing
refresh churn.

### Cache Invalidations

Panel type: Time series

```promql
sum by (source) (
  rate(theia_cache_invalidation_total{
    job=~"$job",
    instance=~"$instance"
  }[$__rate_interval])
)
```

Cache invalidation source labels identify repository paths that triggered cache
updates.

### Cache Reloads

Panel type: Time series

```promql
rate(theia_cache_reload_total{
  job=~"$job",
  instance=~"$instance"
}[$__rate_interval])
```

Frequent full reloads can drive CPU and database work. Correlate with topology
reloads and snapshot builds.

### State Changes Dropped

Panel type: Stat

```promql
increase(theia_state_changes_dropped_total{
  job=~"$job",
  instance=~"$instance"
}[$__rate_interval])
```

This should be zero. Any increase means the non-blocking state-store change
channel was full and a change batch was dropped.

Follow-up checks:

- WebSocket message rate and payload size.
- Snapshot build p95.
- Connected client count.
- CPU and heap during the same interval.

## Row 12: Metrics

The final row repeats a few high-signal debugging views.

### Queue Lag

Panel type: Time series  
Unit: Seconds

```promql
theia_scheduler_queue_lag_seconds{
  job=~"$job",
  instance=~"$instance",
  volatility_class=~"$volatility_class"
}
```

Keep this visible when tuning worker capacity or polling intervals.

### State Changes Dropped

Panel type: Stat

```promql
increase(theia_state_changes_dropped_total{
  job=~"$job",
  instance=~"$instance"
}[5m])
```

This fixed `[5m]` view is useful even when the dashboard time range is broad.

### Burst Detector

Panel type: Bar chart

```promql
sum(increase(theia_scheduler_task_dispatch_total{
  job=~"$job",
  instance=~"$instance"
}[30s]))
```

This detects synchronized task dispatch bursts. Use it with CPU and the dispatch
heatmap when investigating cyclic CPU spikes.

## Recommended Configuration Workflow

Use this workflow when building or validating the dashboard from scratch.

### Step 1: Verify The Prometheus Target

Run in Prometheus:

```promql
up{job="theia-backend"}
```

Expected result:

- `1` for each healthy backend target.

If the result is empty:

- Prometheus is not scraping the backend.
- The job label is different from `theia-backend`.
- Grafana is querying the wrong Prometheus server.

If the result is `0`:

- The backend is unreachable from Prometheus.
- The `/metrics` endpoint is failing.
- In production, the bearer token secret may be missing or wrong.

### Step 2: Verify The Anchor Metric

Run:

```promql
theia_scheduler_in_flight_tasks
```

Expected result:

- At least one series with `job` and `instance` labels.

The dashboard variables depend on this metric. If it is missing, fix backend
metrics before changing Grafana.

### Step 3: Configure Variables

Create variables in this order:

1. `job`
2. `instance`
3. `volatility_class`
4. `task_kind`
5. `collector`
6. `operation`
7. `result`
8. `ws_scope`
9. `bulk_operation`

Set `Include All` and `Multi-value` for every filter variable except `job` and
`instance`, unless you explicitly need fleet-wide views. Use `.*` as the custom
all value.

### Step 4: Build Overview Panels

Create the overview row first. Do not start with detailed SNMP or WebSocket
panels. The overview tells you whether the backend is busy, overloaded, missing
deadlines, or running normally.

Minimum overview panels:

- CPU Usage
- Scheduler dispatch rate
- Scheduler in-flight tasks
- Essential overloaded
- Deadline misses

### Step 5: Add Scheduler Panels

Add scheduler panels before collector panels. Scheduler pressure tells you
whether slow collectors are actually hurting backend freshness.

Minimum scheduler panels:

- Ready queue depth by class
- Queue lag by class
- Scheduler task duration p95
- Scheduler task duration p99
- Scheduler backpressure by reason
- Dispatch heatmap

### Step 6: Add Polling And SNMP Panels

Add polling and SNMP panels after the scheduler row.

Minimum panels:

- Poll failures
- Poll failure ratio
- SNMP operations rate
- SNMP errors/timeouts
- SNMP latency p95
- SNMP latency p99
- Slow SNMP walks table
- SNMP timeout percentage table
- Slow SNMP devices table
- SNMP device slow events table

### Step 7: Add Realtime And Topology Panels

These explain frontend freshness and topology churn.

Minimum panels:

- WebSocket connected clients
- WebSocket messages rate
- WebSocket payload size p95
- Snapshot build p95
- Full topology reload rate
- Link upserts
- Unknown neighbors
- Topology materialization p95
- Topology materialization skips
- Static collection timeout skips
- Static persistence skips

### Step 8: Add Correlation Panels

Add CPU correlation panels last. They are not primary health signals; they are
hypothesis tests.

Minimum correlations:

- CPU vs scheduler dispatch
- CPU vs queue lag
- CPU vs snapshot build
- CPU vs SNMP latency p95
- CPU vs WebSocket messages
- CPU vs GC
- Heap and goroutines

### Step 9: Add Bulk And Prometheus Runtime Panels

Add these when the deployment uses backup/download workflows or Prometheus
runtime enrichment.

Minimum bulk panels:

- Bulk operation duration p95
- Bulk operation completions
- Bulk operations in flight
- Bulk operation rejections
- Bulk saturation ratio

Minimum Prometheus runtime panels:

- Prometheus runtime requests
- Prometheus runtime errors
- Prometheus runtime latency p95

### Step 10: Add Cache And State Store Panels

Add these as final guardrails:

- Cache invalidations
- Cache reloads
- State changes dropped
- Queue lag
- Burst detector

## Step-By-Step Diagnostic Playbooks

### Diagnose A Cyclic CPU Spike

1. Open the dashboard to `Last 1 hour`.
2. Set `job=theia-backend` and the relevant `instance`.
3. Find the CPU spike in `CPU Usage`.
4. Check `Burst detector` and `Dispatch heatmap`.
5. If dispatch spikes align with CPU, inspect `task_kind` and
   `volatility_class`.
6. Check `Scheduler in-flight tasks`.
7. Check `Queue lag by class`.
8. If queue lag rises, inspect `Scheduler task duration p95/p99`.
9. If task duration rises, inspect `SNMP latency p95/p99`.
10. If SNMP is not slow, inspect `Snapshot build p95`, `WebSocket payload size
    p95`, `Prometheus runtime latency p95`, heap, and GC.
11. If CPU spikes align with `Full topology reload rate`, inspect topology dirty
    reasons and link upsert churn.
12. If CPU spikes align with bulk operations, inspect bulk duration, in-flight
    operations, and saturation ratio.

Common conclusions:

- CPU plus dispatch burst: synchronized polling or worker burst.
- CPU plus SNMP latency: slow devices or expensive SNMP walks.
- CPU plus snapshot build and payload size: large realtime snapshot work.
- CPU plus GC and heap: allocation-heavy path, often snapshots or payloads.
- CPU plus bulk in-flight: backup/download work competing with runtime work.

### Diagnose Polling Freshness Problems

1. Check `Essential overloaded`.
2. Check `Deadline Misses`.
3. Check `Queue lag by class`.
4. Check `Ready queue depth by class`.
5. Check `Scheduler backpressure by reason`.
6. Check `Scheduler task duration p95/p99`.
7. Check `Poll failure ratio`.
8. Check SNMP latency and errors.

Interpretation:

- Deadline misses with high queue lag: scheduler cannot keep up.
- Deadline misses with low queue lag but high SNMP latency: tasks are blocked in
  collection.
- Deadline misses with backpressure: configured limits are protecting the
  backend but freshness is degraded.
- Failure ratio without queue lag: devices or credentials may be failing, but
  scheduler capacity is probably not the main issue.

### Diagnose SNMP Failures

1. Filter `collector` to the suspected collector.
2. Filter `operation` to the slow or failing operation.
3. Compare `SNMP operations rate`, `SNMP errors/timeouts`, `SNMP latency p95`,
   and `SNMP latency p99`.
4. Use `SNMP operations per Volume` to find the largest operation families.
5. Use `SNMP Walks Timeout` to find timeout-heavy operations.
6. Use `Slow SNMP devices` and `SNMP device slow events` to identify the
   device, target, collector, operation, and result labels behind slow SNMP
   behavior.
7. Compare failures with `Poll failure ratio`.

Interpretation:

- High timeout percentage: network/device/SNMP configuration issue.
- High p99 only: a small set of devices or operations is slow; use the
  device-level SNMP tables to identify them.
- High p95 and high operation volume: collector path is broadly expensive.
- Early exits rising: collector is skipping deeper work because an earlier
  prerequisite failed.

### Diagnose WebSocket Or Browser Freshness Issues

1. Check `WebSocket connected clients`.
2. Check `WebSocket messages rate`.
3. Check `WebSocket payload size p95`.
4. Check `WebSocket backpressure` and `WebSocket resync required`.
5. Check `Snapshot build p95`.
6. Check `State changes dropped`.
7. Compare with CPU, heap, goroutines, and GC.

Interpretation:

- Many clients plus large payloads can cause CPU and allocation pressure.
- Backpressure means clients or hub buffers cannot keep up.
- Resyncs mean clients are missing incremental updates and need larger recovery
  payloads.
- State changes dropped means backend state propagation is overloaded.

### Diagnose Topology Churn

1. Check `Full topology reload rate`.
2. Check `Snapshot build p95`.
3. Check `Topology materialization p95`.
4. Check `Topology materialization skips`.
5. Check `Link upserts`.
6. Check `Unknown neighbors`.
7. Check `Static collection timeout skips`.
8. Check `Static persistence skips`.
9. Compare with CPU and WebSocket payload p95.

Interpretation:

- Frequent reloads can cause repeated snapshot work.
- Repeated link upserts can indicate unstable discovery input.
- Unknown neighbors can cause repeated discovery observations that do not settle
  into known topology.
- Topology materialization `unchanged` skips mean static persistence is avoiding
  redundant canonical topology rebuild work because topology-relevant input is
  stable.
- Static collection cooldown skips mean timeout-heavy optional table walks are
  being suppressed to protect the static polling lane.
- Static persistence `unchanged` skips mean dedupe is avoiding redundant writes;
  losing that signal while topology churn rises can indicate unstable static
  discovery output.

### Diagnose Bulk Operation Pressure

1. Select `bulk_operation`.
2. Check `Bulk operations in flight`.
3. Check `Bulk operation duration p95`.
4. Check `Bulk operation completions`.
5. Check `Bulk operation rejections`.
6. Check `Bulk operation saturation`.
7. Compare with CPU, heap, and scheduler queue lag.

Interpretation:

- In-flight near concurrency limit: operation is saturated.
- Rejections by quota reason: client requested too much work.
- Rejections by concurrency reason: active operations are at configured limits.
- Bulk duration spikes plus CPU/heap spikes: backup/download work may be
  competing with polling and realtime delivery.

## Alert Mapping

The Prometheus alert rules in
[`docker/prometheus/alert_rules.yml`](../../docker/prometheus/alert_rules.yml)
map directly to dashboard signals.

| Alert | Dashboard panels to inspect |
| --- | --- |
| `BulkOperationRejections` | Bulk operation rejections, bulk completions, bulk duration p95. |
| `BulkOperationSaturated` | Bulk operations in flight, bulk saturation ratio, bulk concurrency limit. |
| `WebSocketBackpressure` | WebSocket backpressure, WebSocket messages rate, payload size p95, connected clients. |
| `WebSocketResyncRequired` | WebSocket resync required, snapshot build p95, payload size p95. |
| `PollingEssentialOverloaded` | Essential overloaded, deadline misses, queue lag, backpressure. |
| `PollingDeadlineMisses` | Deadline misses, queue lag, task duration, SNMP latency. |
| `PollingFailuresHigh` | Poll failures, poll failure ratio, SNMP errors/timeouts. |
| `SchedulerQueueLagHigh` | Queue lag by class, ready queue depth, in-flight tasks, task duration. |
| `SchedulerBackpressure` | Scheduler backpressure by reason, queue lag, dispatch rate. |
| `SchedulerTaskDurationHigh` | Scheduler task duration p95/p99, SNMP latency, Prometheus runtime latency, snapshot build p95. |
| `SNMPBulkWalkErrors` | SNMP errors/timeouts, SNMP timeout table, poll failure ratio. |
| `SNMPBulkWalkSlow` | SNMP latency p95/p99, slow SNMP walks table, slow SNMP devices, SNMP device slow events, task duration p95. |

## Troubleshooting

### Dashboard Variables Are Empty

Check:

```promql
theia_scheduler_in_flight_tasks
```

If empty:

- Prometheus is not scraping Theia.
- The backend metrics endpoint is not reachable.
- The production metrics bearer token is wrong or missing.
- Grafana is connected to the wrong Prometheus data source.

### `job` Exists But `instance` Is Empty

Check the `instance` variable query:

```promql
label_values(theia_scheduler_in_flight_tasks{job="$job"},instance)
```

If `job` is single-select, this is fine. If you make `job` multi-select, change
the query to:

```promql
label_values(theia_scheduler_in_flight_tasks{job=~"$job"},instance)
```

### Panels Are Empty When `All` Is Selected

Check whether the query uses exact matchers with multi-select variables.

Wrong:

```promql
metric{volatility_class="$volatility_class"}
```

Correct:

```promql
metric{volatility_class=~"$volatility_class"}
```

### SNMP Table Mixes Multiple Jobs Or Instances

Make sure every query includes:

```promql
job=~"$job", instance=~"$instance"
```

`SNMP Walks Timeout` is scoped this way so multi-instance dashboards do not mix
unrelated backend targets.

### Histograms Show No Data

Check whether the `_bucket` series exists:

```promql
theia_scheduler_task_duration_seconds_bucket
```

If `_count` and `_sum` exist but `_bucket` does not, the metric is not a
histogram in the expected format. In Theia's current registry, the duration
metrics used by this dashboard are exported as histograms.

### CPU Is High But Scheduler Panels Are Quiet

Check:

- `go_memstats_gc_cpu_fraction`
- `go_memstats_heap_alloc_bytes`
- `go_goroutines`
- `theia_refresh_snapshot_build_seconds_bucket`
- `theia_ws_message_payload_bytes_bucket`
- `theia_bulk_operation_duration_seconds_bucket`
- `theia_prometheus_runtime_request_seconds_bucket`

This pattern usually means CPU is not coming from dispatch volume alone.

### Poll Failure Ratio Is High But SNMP Errors Are Low

Check whether failures are coming from non-SNMP task paths, Prometheus runtime
enrichment, topology persistence, database calls, or task orchestration. Compare
poll failures with:

- Scheduler task duration.
- Prometheus runtime errors.
- Snapshot build errors.
- Backend logs for task failures.

### Grafana Works But Theia Prometheus Runtime Panels Show Errors

Grafana and Theia use different Prometheus clients.

Check Theia's configured Prometheus URL in the UI Settings panel. It must be
reachable from the backend container or host where Theia runs. A URL that works
from your laptop may not work from inside the backend container.

### Prometheus Runtime Works But Grafana Panels Are Empty

Check the Grafana data source URL. Grafana must query the Prometheus server that
scrapes Theia. A valid Theia runtime Prometheus setting does not automatically
configure Grafana.

## Maintenance Rules

When adding a new backend metric or dashboard panel:

1. Add or verify the metric in `internal/observability/registry.go`.
2. Make sure the metric has stable, low-cardinality labels.
3. Add a panel that uses `job=~"$job"` and `instance=~"$instance"`.
4. Use regex matchers for every multi-select dashboard variable.
5. Use `rate()` for counters when graphing per-second behavior.
6. Use `increase()` for counters when showing event counts in a time window.
7. Use `histogram_quantile()` over `_bucket` series for p95/p99 panels.
8. Keep `le` in the `sum by (...)` group for histogram quantiles.
9. Add the metric to this guide if it explains backend behavior.
10. Consider whether an alert in `docker/prometheus/alert_rules.yml` should also
    be added or updated.

## Quick Validation Checklist

Use this checklist after importing or changing the dashboard.

- `job` variable shows `theia-backend`.
- `instance` variable shows at least one backend target.
- `Backend scrape health` is `UP`.
- `Prometheus scrape duration` returns data.
- `CPU Usage` returns data.
- `Scheduler in-flight tasks` returns data.
- `Queue lag by class` returns data after polling starts.
- `Poll failures` shows success/failure series when polling runs.
- `SNMP operations rate` shows data when devices are polled through Theia SNMP
  collectors.
- `WebSocket connected clients` changes when browsers connect or disconnect.
- `WebSocket backpressure` and `WebSocket resync required` stay near zero under
  normal client load.
- `Snapshot build p95` shows data after snapshot refresh work runs.
- `Discovery neighbors` shows data after topology discovery has observations.
- `Bulk operation saturation` returns data when bulk backup/download operations
  are active.
- `Prometheus runtime requests` shows data only when Theia is configured to call
  Prometheus.
- `State changes dropped` remains zero.
- Table panels are scoped with `job` and `instance` filters in multi-instance
  deployments.