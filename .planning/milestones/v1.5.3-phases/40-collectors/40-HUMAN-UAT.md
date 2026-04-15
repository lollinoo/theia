---
status: partial
phase: 40-collectors
source:
  - 40-VERIFICATION.md
started: 2026-04-12T14:50:43Z
updated: 2026-04-14T20:53:23Z
---

## Current Test

Human verification is approved for Phase 40 closure: live SNMP collector proof is complete, and Prometheus `probe_success` reachability remains an accepted environment limitation in this lab.

## Tests

### 1. Live SNMP Collector Probe
expected: PerformanceCollector returns SNMP metrics and raw counters with missing OIDs left nil, OperationalCollector returns reachability plus partial uptime/status data when some OIDs are absent, and StaticCollector returns inventory/topology data without DB or service side effects.
result: passed — verified against two simulated devices (`127.0.10.10` and `127.0.10.11`). Performance returned SNMP metrics/counters with partial nil fields preserved, Operational returned reachability and interface status, and Static returned inventory plus LLDP neighbors without DB/service coupling.

### 2. Live Prometheus Enrichment Probe
expected: Hostname, probe reachability, and alert mappings populate correctly for one explicit-label device and one IP-fallback device, while no core metric or throughput authority shifts away from SNMP.
result: skipped — accepted environment limitation; hostname fallback, explicit-label hostname mapping, and alert mapping passed, SNMP remained authoritative for core metrics and throughput, and Prometheus exposed no `probe_success` series in this lab. Fallback hostname and alert mapping worked first with `instance=127.0.10.11`, then explicit-label hostname and alert mapping worked after adding an `identity` scrape label and querying with `identity=127.0.10.11`. The current Prometheus environment exposed no `probe_success` series, so `ProbeReachable` could not be exercised in this lab.

## Summary

total: 2
passed: 1
issues: 0
pending: 0
skipped: 1
blocked: 0

## Gaps

None. Prometheus `probe_success` absence is an accepted environment limitation for this lab and no longer blocks milestone archival.
