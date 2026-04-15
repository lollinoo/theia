---
phase: 40-collectors
reviewed: 2026-04-12T14:57:55Z
depth: standard
files_reviewed: 16
files_reviewed_list:
  - internal/collector/operational.go
  - internal/collector/operational_test.go
  - internal/collector/performance.go
  - internal/collector/performance_test.go
  - internal/collector/prometheus.go
  - internal/collector/prometheus_test.go
  - internal/collector/rates.go
  - internal/collector/rates_test.go
  - internal/collector/results.go
  - internal/collector/results_test.go
  - internal/collector/static.go
  - internal/collector/static_test.go
  - internal/snmp/discovery.go
  - internal/snmp/discovery_test.go
  - internal/worker/metrics_collector.go
  - internal/worker/metrics_collector_test.go
findings:
  critical: 0
  warning: 4
  info: 0
  total: 4
status: issues_found
---

# Phase 40: Code Review Report

**Reviewed:** 2026-04-12T14:57:55Z
**Depth:** standard
**Files Reviewed:** 16
**Status:** issues_found

## Summary

Reviewed the new collector, SNMP discovery, and metrics-worker changes at file scope plus a few adjacent consumers where the reviewed code depends on protocol semantics. The main problems are behavioral: alert mapping in the worker no longer handles `instance="ip:port"` devices, sparse deltas cannot express removals, empty Prometheus metrics fabricate timestamp-only changes, and several SNMP paths still downgrade transport/query failures to "successful but empty" results.

I could not execute the Go package tests in this environment because the `go` toolchain is not installed.

## Warnings

### WR-01: Worker alert mapping drops devices that use explicit `instance` label values

**File:** `internal/worker/metrics_collector.go:887-913`
**Issue:** `attachAlerts` only keys devices by `device.IP`, so alerts whose `Instance` is an explicit `instance` label like `192.0.2.40:9116` are discarded. The reviewed collector code already treats that form as valid in [internal/collector/prometheus.go](/home/azmin/projects/theia/internal/collector/prometheus.go:124) and tests it in [internal/collector/prometheus_test.go](/home/azmin/projects/theia/internal/collector/prometheus_test.go:293), so the worker and collector now disagree on the same alert payload.
**Fix:** Reuse the same mapping rule in the worker: index devices by raw IP and, when `ResolvePrometheusLabel(device)` returns `labelName == "instance"`, also index `labelValue`. A minimal fix is to replace the local IP-only lookup with the `MapAlertsToDevices` logic and flatten the grouped result back into the worker’s slice form. Add a regression test in `metrics_collector_test.go` for an alert with `Instance: "192.0.2.40:9116"`.

### WR-02: `snapshot_delta` generation never reports removed keys

**File:** `internal/worker/metrics_collector.go:1086-1142`
**Issue:** `buildDelta` only iterates current-section keys. If a hostname/model disappears, a device is removed, or a section shrinks, the delta contains no tombstone and downstream clients keep the stale entry forever. This breaks the advertised "changed entries since the last broadcast" contract because deletions are also changes.
**Fix:** Detect keys present in `prevHashes` but missing from `currentHashes`. With the current client merge semantics, the safest fix is to fall back to a full `snapshot` broadcast whenever any section loses keys (or when alerts clear to empty), instead of sending a sparse delta that cannot represent removals. Add regression tests for hostname/model removal and device deletion.

### WR-03: Missing Prometheus metrics are turned into timestamp-only changes every poll

**File:** `internal/worker/metrics_collector.go:811-837`
**Issue:** When a device has no Prometheus metrics, `attachDeviceMetrics` synthesizes `CollectedAt = time.Now().UTC()`. `computeSnapshotHashes` then hashes that timestamp ([internal/worker/metrics_collector.go](/home/azmin/projects/theia/internal/worker/metrics_collector.go:1029)), so a device with unchanged all-`nil` metrics will still look "changed" on every real poll interval. The current no-change test only avoids this because both collections usually land in the same RFC3339 second.
**Fix:** Leave `CollectedAt` zero when all metric fields are nil, or preserve the last real metric timestamp instead of stamping `time.Now()`. A targeted regression test should force two cycles at least one second apart and assert that no `snapshot_delta` is emitted when all metric fields remain absent.

### WR-04: SNMP query failures are still reported as successful empty static/performance polls

**File:** `internal/snmp/discovery.go:252-320`, `internal/snmp/discovery.go:761-905`, `internal/collector/static.go:88-105`, `internal/collector/performance.go:99-121`, `internal/collector/results.go:83-99`
**Issue:** Core SNMP walks are silently ignored in both discovery and performance collection. `discoverInterfaces`, `discoverNeighbors`, `PollDeviceMetrics`, and `PollInterfaceCounters` swallow `BulkWalk`/`Get` errors, so `StaticCollector.Poll` can return a "successful" result with empty interfaces/neighbors and `PerformanceCollector.Poll` can leave `Err == nil` after a total metrics/counter query failure. Because `PerformanceResult.ToStoreUpdate` derives `PollSuccess` from `Err == nil`, the state layer will treat those failed polls as healthy.
**Fix:** Surface query/transport failures from the SNMP helpers instead of collapsing them into empty slices/pointers. For example, have the helper return `(data, error)` or an aggregated partial-error type, and set `result.Err` in the collectors when a required walk fails. Add tests mirroring `TestPollOperationalStatus_QueryError` for the static and performance paths so failed walks cannot regress back to "success with zero data".

---

_Reviewed: 2026-04-12T14:57:55Z_
_Reviewer: Claude (gsd-code-reviewer)_
_Depth: standard_
