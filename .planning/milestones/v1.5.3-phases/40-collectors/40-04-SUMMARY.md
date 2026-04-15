---
phase: 40-collectors
plan: 04
subsystem: infra
tags: [prometheus, collectors, pipeline, alerts]
requires:
  - phase: 40-collectors
    provides: shared collector result types and Phase 40 collector package boundaries from Plan 01
provides:
  - narrowed Prometheus enrichment collector interface for hostname, probe, and alert queries only
  - reusable label-resolution helper with explicit label pair and instance/IP fallback rules
  - reusable alert-to-device grouping helper for later pipeline wiring
affects:
  - 41-scheduler
  - 42-pipeline-orchestrator
tech-stack:
  added: []
  patterns:
    - enrichment-only Prometheus collector interface with SNMP authority enforced by type shape
    - batch alert collection separated from per-device hostname and probe enrichment
key-files:
  created:
    - internal/collector/prometheus.go
    - internal/collector/prometheus_test.go
  modified: []
key-decisions:
  - "PrometheusCollector exposes only QueryHostnames, QueryProbeStatus, and QueryAlerts so Prometheus cannot regain authority over core metrics or link throughput in Phase 40."
  - "ResolvePrometheusLabel requires a complete explicit label pair and otherwise falls back to instance/device IP, returning ok=false when no safe query target exists."
patterns-established:
  - "Per-device enrichment fetches hostname and probe reachability only; alerts remain a batch helper for later pipeline stages."
  - "Alert mapping recognizes raw device IPs plus explicit instance label values and sorts alerts deterministically within each device group."
requirements-completed: [PIPE-01, PIPE-02]
duration: 2 min
completed: 2026-04-12
---

# Phase 40 Plan 04: Collectors Summary

**Prometheus enrichment collector limited to hostname, probe, and alert mapping with explicit instance/IP device resolution**

## Performance

- **Duration:** 2 min
- **Started:** 2026-04-12T14:28:50Z
- **Completed:** 2026-04-12T14:30:52Z
- **Tasks:** 1
- **Files modified:** 2

## Accomplishments
- Added a dedicated `PrometheusCollector` with an interface that only exposes hostname, probe, and alert queries.
- Added `ResolvePrometheusLabel()` so device metadata resolves to an explicit Prometheus label pair or a safe `instance`/IP fallback.
- Added `MapAlertsToDevices()` so later phases can batch alerts separately from per-device enrichment while keeping deterministic per-device ordering.

## Task Commits

Each task was committed atomically via TDD:

1. **Task 1 RED: Prometheus enrichment collector tests** - `1723e9a` (test)
2. **Task 1 GREEN: Prometheus enrichment collector implementation** - `b220ee2` (feat)

## Files Created/Modified
- `internal/collector/prometheus.go` - Narrowed Prometheus enrichment client, per-device enrichment collector, label resolver, and alert mapping helper.
- `internal/collector/prometheus_test.go` - TDD coverage for label fallback, enrichment-only behavior, batch alert collection, and interface narrowing.

## Verification
- `PATH=/usr/local/go/bin:$PATH rtk go test ./internal/collector/... -run 'TestPrometheusCollector|TestResolvePrometheusLabel|TestMapAlertsToDevices' -count=1` - passed
- `PATH=/usr/local/go/bin:$PATH rtk go test ./internal/collector/... -count=1` - passed
- `rtk rg -n "type PrometheusEnrichmentClient interface|QueryHostnames|QueryProbeStatus|QueryAlerts|func ResolvePrometheusLabel|func \\(c \\*PrometheusCollector\\) CollectDeviceEnrichment|func \\(c \\*PrometheusCollector\\) CollectAlerts|func MapAlertsToDevices" internal/collector/prometheus.go` - passed
- `if rtk rg -n "QueryDeviceMetrics|QueryLinkMetrics|QueryInterfaces" internal/collector/prometheus.go; then exit 1; fi` - passed
- `test -f internal/collector/prometheus_test.go && rtk rg -n "func TestResolvePrometheusLabel|func TestPrometheusCollector|func TestMapAlertsToDevices" internal/collector/prometheus_test.go` - passed
- `if [ -n "$(rtk git diff --name-only -- internal/worker/metrics_collector.go cmd/theia/main.go)" ]; then exit 1; fi` - passed

## Decisions Made
- Prometheus stayed enrichment-only by construction. The collector interface cannot call `QueryDeviceMetrics`, `QueryLinkMetrics`, or `QueryInterfaces`, so SNMP remains authoritative for CPU, memory, uptime, temperature, and throughput.
- Alert collection stayed batch-oriented through `CollectAlerts()`, while `CollectDeviceEnrichment()` only handles hostname and probe reachability for a single device.
- Alert mapping now handles explicit `instance` label values in addition to raw device IPs, which makes the extracted helper ready for later pipeline wiring without depending on worker-only code.

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered
- `go` was not on the default shell `PATH` in this environment. Verification commands were run as `PATH=/usr/local/go/bin:$PATH rtk go ...` while keeping the required `rtk` wrapper.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness
- Phase 40 now has a Prometheus-side collector/helper alongside the SNMP collectors from Plans 01 and 03.
- Phase 42 can merge typed Prometheus hostname, probe, and alert enrichment without reintroducing Prometheus fallback for core metrics or link throughput.

## Self-Check: PASSED
- Found summary file: `.planning/phases/40-collectors/40-04-SUMMARY.md`
- Found key files: `internal/collector/prometheus.go`, `internal/collector/prometheus_test.go`
- Found task commits: `1723e9a`, `b220ee2`

---
*Phase: 40-collectors*
*Completed: 2026-04-12*
