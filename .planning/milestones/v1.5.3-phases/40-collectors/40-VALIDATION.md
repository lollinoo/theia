---
phase: 40
slug: collectors
status: verified
nyquist_compliant: true
wave_0_complete: true
created: 2026-04-15
---

# Phase 40 -- Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | Go standard `testing` package with `go build` smoke checks |
| **Config file** | None -- repo Go test conventions |
| **Quick run command** | `PATH=/usr/local/go/bin:$PATH rtk go test ./internal/collector ./internal/snmp ./internal/worker -count=1` |
| **Full suite command** | `PATH=/usr/local/go/bin:$PATH rtk bash -lc 'go test ./internal/collector ./internal/snmp ./internal/worker -count=1 && go build ./...'` |
| **Estimated runtime** | ~60 seconds |

---

## Sampling Rate

- **After every task commit:** Run the exact task command captured in the phase plan.
- **After every plan wave:** Re-run the accepted collector, SNMP helper, and worker package checks.
- **Before `/gsd-verify-work`:** The accepted Go package evidence and finalized `40-HUMAN-UAT.md` closure must both remain valid.
- **Max feedback latency:** 60 seconds

---

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Threat Ref | Secure Behavior | Test Type | Automated Command | Key Artifact | Status |
|---------|------|------|-------------|------------|-----------------|-----------|-------------------|--------------|--------|
| 40-01-T1 | 01 | 1 | PIPE-02 | -- | Shared collector contracts and typed result adapters keep volatility classes explicit | unit | `cd /home/azmin/projects/theia && go test ./internal/collector -run 'Test.*Result|Test.*StateUpdate' -count=1` | `internal/collector/results.go` | ✅ green |
| 40-01-T2 | 01 | 1 | PIPE-01, PIPE-02 | -- | `PerformanceCollector` stays stateless and SNMP-primary for core metrics and counters | unit | `cd /home/azmin/projects/theia && go test ./internal/collector -run 'TestPerformanceCollector' -count=1 -v` | `internal/collector/performance.go` | ✅ green |
| 40-02-T1 | 02 | 2 | PIPE-04 | -- | Counter-rate math discards warm-up, reset, gap, overspeed, and invalid-time samples | unit | `cd /home/azmin/projects/theia && go test ./internal/collector -run 'TestComputeCounterRates' -count=1 -v` | `internal/collector/rates.go` | ✅ green |
| 40-02-T2 | 02 | 2 | PIPE-04 | -- | Runtime SNMP link polling reuses the pure helper and preserves discard/recovery behavior | integration | `cd /home/azmin/projects/theia && go test ./internal/worker -run 'TestBuildSnapshot_SNMPLinkRates_|TestBuildSnapshot_SNMPLinkPollSkipsDeviceWithNoValidLinks' -count=1 -v` | `internal/worker/metrics_collector.go` | ✅ green |
| 40-03-T1 | 03 | 2 | PIPE-02 | -- | Operational SNMP helper preserves partial-result behavior for uptime and interface status | unit | `cd /home/azmin/projects/theia && go test ./internal/snmp -run 'TestPollOperationalStatus' -count=1 -v` | `internal/snmp/discovery.go` | ✅ green |
| 40-03-T2 | 03 | 2 | PIPE-02 | -- | Operational and static collectors stay typed, stateless wrappers over the shared SNMP helpers | unit | `cd /home/azmin/projects/theia && go test ./internal/collector -run 'TestOperationalCollector|TestStaticCollector' -count=1 -v` | `internal/collector/operational.go`, `internal/collector/static.go` | ✅ green |
| 40-04-T1 | 04 | 2 | PIPE-01, PIPE-02 | -- | Prometheus remains enrichment-only, with label fallback and alert-to-device mapping helpers | unit | `cd /home/azmin/projects/theia && go test ./internal/collector/... -run 'TestPrometheusCollector|TestResolvePrometheusLabel|TestMapAlertsToDevices' -count=1` | `internal/collector/prometheus.go` | ✅ green |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

- [x] `internal/collector/results_test.go`, `performance_test.go`, `operational_test.go`, `static_test.go`, and `prometheus_test.go` cover the collector contract, typed result adapters, and stateless wrapper behavior.
- [x] `internal/collector/rates_test.go` and `internal/worker/metrics_collector_test.go` cover reset, gap, overspeed, warm-up discard, and recovery behavior.
- [x] `internal/snmp/discovery_test.go` covers the operational helper's success, fallback-OID, partial-result, and error paths.
- [x] The accepted package regression in `40-VERIFICATION.md` re-ran `./internal/collector`, `./internal/snmp`, and `./internal/worker` on current HEAD and confirmed `go build ./...`.
- [x] No new framework install was required -- the existing Go harness covered the phase.

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Finalized Proof |
|----------|-------------|------------|-----------------|
| Live SNMP Collector Probe | PIPE-01 | Requires live SNMP endpoints, interface inventory, and real vendor behavior that the repo only mocks. | `40-HUMAN-UAT.md` records `passed` against simulated devices `127.0.10.10` and `127.0.10.11`: Performance returned SNMP metrics/counters with partial nil fields preserved, Operational returned reachability and interface status, and Static returned inventory plus LLDP neighbors without DB/service coupling. |
| Live Prometheus Enrichment Probe | PIPE-01 | Requires live Prometheus label conventions, alert payloads, and probe series that are not present in the local harness. | `40-HUMAN-UAT.md` records `skipped` as an accepted environment limitation: hostname fallback, explicit-label hostname mapping, and alert mapping all passed, SNMP remained authoritative for core metrics and throughput, but this lab exposed no `probe_success` series. |

Finalized HUMAN-UAT summary: `passed: 1`, `skipped: 1`, `pending: 0`.

---

## Validation Sign-Off

- [x] All tasks have `<automated>` verify or Wave 0 dependencies.
- [x] Sampling continuity: no 3 consecutive tasks without automated verify.
- [x] Wave 0 covers collector, SNMP helper, runtime discard, and enrichment regressions.
- [x] No watch-mode flags were used in accepted evidence.
- [x] Feedback latency remains under 60 seconds for the accepted package suites.
- [x] `nyquist_compliant: true` is set in frontmatter.

**Approval:** verified 2026-04-15 -- HUMAN-UAT finalized with `passed: 1`, `skipped: 1`, `pending: 0`, and Prometheus `probe_success` remains an accepted environment limitation in this lab.
