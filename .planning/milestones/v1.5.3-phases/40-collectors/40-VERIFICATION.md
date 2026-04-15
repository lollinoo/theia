---
phase: 40-collectors
verified: 2026-04-12T14:46:57Z
status: human_needed
score: 11/11 must-haves verified
overrides_applied: 0
re_verification:
  previous_status: human_needed
  previous_score: 11/11
  gaps_closed: []
  gaps_remaining: []
  regressions: []
deferred:
  - truth: "Collector constructors and store adapters are not yet instantiated by the production pipeline outside tests"
    addressed_in: "Phase 42"
    evidence: "Phase 42 goal: 'A unified PipelineOrchestrator replaces both Poller and MetricsCollector, wiring the scheduler to worker goroutines to the state engine to WebSocket broadcast'"
human_verification:
  - test: "Run PerformanceCollector, OperationalCollector, and StaticCollector against a reachable managed SNMP device with at least one linked interface."
    expected: "Performance returns SNMP metrics/counters with missing OIDs left nil, Operational returns reachability plus partial uptime/status data, and Static returns inventory/topology without DB/service side effects."
    why_human: "Requires live SNMP endpoints and real device/vendor behavior; repository coverage uses mocked SNMP clients only."
  - test: "Run PrometheusCollector CollectDeviceEnrichment and CollectAlerts against the project's Prometheus using one device with explicit label metadata and one using IP fallback."
    expected: "Hostname, probe reachability, and alerts map back to device IDs while core metrics and throughput remain SNMP-owned."
    why_human: "Requires live Prometheus series, label conventions, and active alert payloads that are not present in the local test harness."
---

# Phase 40: Collectors Verification Report

**Phase Goal:** Stateless collector functions exist for each volatility class, producing typed results that the state engine can consume -- making data collection independent of scheduling and state management
**Verified:** 2026-04-12T14:46:57Z
**Status:** human_needed
**Re-verification:** Yes — refreshed against current HEAD; previous report existed and no regressions were found

## Goal Achievement

### Observable Truths

| # | Truth | Status | Evidence |
| --- | --- | --- | --- |
| 1 | `internal/collector` defines a shared collector contract and typed result surface keyed to `domain.VolatilityClass` | ✓ VERIFIED | [internal/collector/results.go](/home/azmin/projects/theia/internal/collector/results.go:15) defines `StateUpdate`, `SNMPClient`, `NewSNMPClientFunc`, and typed result structs; [internal/collector/results_test.go](/home/azmin/projects/theia/internal/collector/results_test.go:32) and [internal/collector/results_test.go](/home/azmin/projects/theia/internal/collector/results_test.go:183) verify interface satisfaction and exact volatility constants. |
| 2 | Performance and operational results can be adapted into the existing state engine without redesigning `internal/state`; failed performance polls stay failed in store updates | ✓ VERIFIED | [internal/collector/results.go](/home/azmin/projects/theia/internal/collector/results.go:83) now sets `PollSuccess` from `Err` and leaves `Metrics=nil` on failure; [internal/collector/results_test.go](/home/azmin/projects/theia/internal/collector/results_test.go:101) covers the post-review failure path from commit `928a621`; [internal/state/store.go](/home/azmin/projects/theia/internal/state/store.go:86) remains unchanged. |
| 3 | `PerformanceCollector` is stateless and SNMP-primary for CPU, memory, temperature, uptime, and raw counters | ✓ VERIFIED | [internal/collector/performance.go](/home/azmin/projects/theia/internal/collector/performance.go:18) contains only `registry`, `newClient`, and `now`; [internal/collector/performance.go](/home/azmin/projects/theia/internal/collector/performance.go:99) calls `ResolvePerformanceOIDs`, `snmp.PollDeviceMetrics`, and `snmp.PollInterfaceCounters`. |
| 4 | Performance collection keeps Prometheus non-authoritative and preserves partial-result semantics | ✓ VERIFIED | [internal/collector/results.go](/home/azmin/projects/theia/internal/collector/results.go:44) reserves enrichment fields only; [internal/collector/performance.go](/home/azmin/projects/theia/internal/collector/performance.go:99) collects via SNMP only; [internal/snmp/discovery.go](/home/azmin/projects/theia/internal/snmp/discovery.go:761) returns nil metric pointers when data is missing; [internal/collector/performance_test.go](/home/azmin/projects/theia/internal/collector/performance_test.go:140) exercises missing-field behavior and zero-value enrichment. |
| 5 | Raw counter snapshots carry discovered link speed metadata needed for later sanity checks | ✓ VERIFIED | [internal/collector/results.go](/home/azmin/projects/theia/internal/collector/results.go:37) defines `SpeedBps`; [internal/collector/performance.go](/home/azmin/projects/theia/internal/collector/performance.go:106) populates it from `device.Interfaces`; [internal/worker/metrics_collector.go](/home/azmin/projects/theia/internal/worker/metrics_collector.go:990) reuses the same snapshot shape in the current runtime seam. |
| 6 | Counter-rate computation is isolated in a pure helper with explicit baseline state, not inline worker math | ✓ VERIFIED | [internal/collector/rates.go](/home/azmin/projects/theia/internal/collector/rates.go:13) defines `CounterBaseline` and [internal/collector/rates.go](/home/azmin/projects/theia/internal/collector/rates.go:22) defines `ComputeCounterRates`; [internal/worker/metrics_collector.go](/home/azmin/projects/theia/internal/worker/metrics_collector.go:690) delegates to it; repo search shows the old inline `counter.InOctets - old.InOctets` path is gone from the worker. |
| 7 | Reset, gap, overspeed, and warm-up intervals discard throughput instead of emitting zero or stale values | ✓ VERIFIED | [internal/collector/rates.go](/home/azmin/projects/theia/internal/collector/rates.go:44) discards warm-up, gap, reset, invalid-time, and overspeed samples by returning no `LinkMetrics`; [internal/collector/rates_test.go](/home/azmin/projects/theia/internal/collector/rates_test.go:41) through [internal/collector/rates_test.go](/home/azmin/projects/theia/internal/collector/rates_test.go:201) cover those edge cases. |
| 8 | The existing worker SNMP link path reuses the helper and integration tests prove discard and recovery behavior | ✓ VERIFIED | [internal/worker/metrics_collector.go](/home/azmin/projects/theia/internal/worker/metrics_collector.go:642) routes SNMP link counters through `collector.ComputeCounterRates`; [internal/worker/metrics_collector_test.go](/home/azmin/projects/theia/internal/worker/metrics_collector_test.go:1479) through [internal/worker/metrics_collector_test.go](/home/azmin/projects/theia/internal/worker/metrics_collector_test.go:1583) verify reset, gap, and overspeed discards plus post-warm-up recovery. |
| 9 | `OperationalCollector` is a stateless wrapper over focused SNMP uptime and interface-status polling with partial-result behavior | ✓ VERIFIED | [internal/collector/operational.go](/home/azmin/projects/theia/internal/collector/operational.go:17) is stateless and [internal/collector/operational.go](/home/azmin/projects/theia/internal/collector/operational.go:94) wraps `snmp.PollOperationalStatus`; [internal/snmp/discovery.go](/home/azmin/projects/theia/internal/snmp/discovery.go:76) implements fallback OIDs and partial results; [internal/collector/operational_test.go](/home/azmin/projects/theia/internal/collector/operational_test.go:62) covers success, partial data, and error paths. |
| 10 | `StaticCollector` is a stateless wrapper over SNMP discovery returning typed inventory and topology data without DB or service coupling | ✓ VERIFIED | [internal/collector/static.go](/home/azmin/projects/theia/internal/collector/static.go:16) is stateless and [internal/collector/static.go](/home/azmin/projects/theia/internal/collector/static.go:88) wraps `snmp.DiscoverDevice`; [internal/collector/static_test.go](/home/azmin/projects/theia/internal/collector/static_test.go:212) checks the struct has no service/repository collaborators, while code inspection shows only `snmp` and `vendor` dependencies. |
| 11 | Prometheus support is enrichment-only, with label/IP fallback and alert-to-device mapping helpers | ✓ VERIFIED | [internal/collector/prometheus.go](/home/azmin/projects/theia/internal/collector/prometheus.go:19) narrows the collector-facing interface to `QueryHostnames`, `QueryProbeStatus`, and `QueryAlerts`; [internal/collector/prometheus.go](/home/azmin/projects/theia/internal/collector/prometheus.go:42) and [internal/collector/prometheus.go](/home/azmin/projects/theia/internal/collector/prometheus.go:127) implement label fallback and alert mapping; [internal/collector/prometheus_test.go](/home/azmin/projects/theia/internal/collector/prometheus_test.go:135) and [internal/collector/prometheus_test.go](/home/azmin/projects/theia/internal/collector/prometheus_test.go:293) verify the narrowed behavior. |

**Score:** 11/11 truths verified

### Deferred Items

Items not yet met but explicitly addressed in later milestone phases.

| # | Item | Addressed In | Evidence |
| --- | --- | --- | --- |
| 1 | Production runtime does not yet instantiate the new collector constructors or store adapters outside tests | Phase 42 | [.planning/ROADMAP.md](/home/azmin/projects/theia/.planning/ROADMAP.md:166) defines Phase 42 as the cutover that wires scheduler, collectors, state engine, and WebSocket broadcast together. |

### Required Artifacts

| Artifact | Expected | Status | Details |
| --- | --- | --- | --- |
| `internal/collector/results.go` | Shared collector contract, typed results, adapters | ✓ VERIFIED | Substantive contract and adapters at [internal/collector/results.go](/home/azmin/projects/theia/internal/collector/results.go:15); current HEAD includes the `928a621` fix in [internal/collector/results.go](/home/azmin/projects/theia/internal/collector/results.go:83). |
| `internal/collector/results_test.go` | Contract and adapter tests | ✓ VERIFIED | Covers interface satisfaction, constructor signature, adapters, and volatility classes at [internal/collector/results_test.go](/home/azmin/projects/theia/internal/collector/results_test.go:32). |
| `internal/collector/performance.go` | Stateless SNMP-primary performance collector | ✓ VERIFIED | Wires vendor OID resolution and SNMP polling at [internal/collector/performance.go](/home/azmin/projects/theia/internal/collector/performance.go:99); no collector-side mutable state. |
| `internal/collector/performance_test.go` | Performance collector behavior tests | ✓ VERIFIED | Exercises happy path, partial results, connect failure, and zero-value enrichment starting at [internal/collector/performance_test.go](/home/azmin/projects/theia/internal/collector/performance_test.go:47). |
| `internal/collector/rates.go` | Pure rate helper with explicit baseline state | ✓ VERIFIED | No hidden state; worker delegates to it at [internal/worker/metrics_collector.go](/home/azmin/projects/theia/internal/worker/metrics_collector.go:690). |
| `internal/collector/rates_test.go` | Reset, gap, overspeed, warm-up tests | ✓ VERIFIED | Covers all phase-required edge cases at [internal/collector/rates_test.go](/home/azmin/projects/theia/internal/collector/rates_test.go:10). |
| `internal/worker/metrics_collector.go` | Runtime seam reusing the rate helper | ✓ VERIFIED | SNMP link path converts counters into `collector.InterfaceCounterSnapshot` and delegates rate math at [internal/worker/metrics_collector.go](/home/azmin/projects/theia/internal/worker/metrics_collector.go:642). |
| `internal/worker/metrics_collector_test.go` | Runtime discard and recovery coverage | ✓ VERIFIED | Integration tests at [internal/worker/metrics_collector_test.go](/home/azmin/projects/theia/internal/worker/metrics_collector_test.go:1479) confirm discarded samples stay absent until warm-up completes. |
| `internal/snmp/discovery.go` | Reusable operational helper and SNMP metric/discovery primitives | ✓ VERIFIED | `PollOperationalStatus`, `PollInterfaceCounters`, `DiscoverDevice`, and `PollDeviceMetrics` at [internal/snmp/discovery.go](/home/azmin/projects/theia/internal/snmp/discovery.go:76), [internal/snmp/discovery.go](/home/azmin/projects/theia/internal/snmp/discovery.go:141), [internal/snmp/discovery.go](/home/azmin/projects/theia/internal/snmp/discovery.go:213), and [internal/snmp/discovery.go](/home/azmin/projects/theia/internal/snmp/discovery.go:761) all contain real SNMP `Get` and `BulkWalk` logic. |
| `internal/snmp/discovery_test.go` | Operational helper tests | ✓ VERIFIED | `TestPollOperationalStatus_*` covers success, fallback OIDs, partial results, and query errors at [internal/snmp/discovery_test.go](/home/azmin/projects/theia/internal/snmp/discovery_test.go:136). |
| `internal/collector/operational.go` | Stateless operational collector | ✓ VERIFIED | One-client-per-poll wrapper over `snmp.PollOperationalStatus` at [internal/collector/operational.go](/home/azmin/projects/theia/internal/collector/operational.go:35). |
| `internal/collector/operational_test.go` | Operational collector tests | ✓ VERIFIED | Verifies happy path, partial results, error path, and interface satisfaction starting at [internal/collector/operational_test.go](/home/azmin/projects/theia/internal/collector/operational_test.go:62). |
| `internal/collector/static.go` | Stateless static collector | ✓ VERIFIED | One-client-per-poll wrapper over `snmp.DiscoverDevice` at [internal/collector/static.go](/home/azmin/projects/theia/internal/collector/static.go:34). |
| `internal/collector/static_test.go` | Static collector tests | ✓ VERIFIED | Verifies typed discovery copy, failure path, interface satisfaction, and collaborator shape starting at [internal/collector/static_test.go](/home/azmin/projects/theia/internal/collector/static_test.go:19). |
| `internal/collector/prometheus.go` | Enrichment-only Prometheus helper | ✓ VERIFIED | Narrowed interface and mapping helpers at [internal/collector/prometheus.go](/home/azmin/projects/theia/internal/collector/prometheus.go:16); runtime cutover is intentionally deferred to Phase 42. |
| `internal/collector/prometheus_test.go` | Enrichment-only tests | ✓ VERIFIED | Verifies label fallback, enrichment-only collection, alert grouping, and interface narrowing at [internal/collector/prometheus_test.go](/home/azmin/projects/theia/internal/collector/prometheus_test.go:79). |

### Key Link Verification

| From | To | Via | Status | Details |
| --- | --- | --- | --- | --- |
| `internal/collector/performance.go` | `internal/snmp/discovery.go::PollDeviceMetrics` | SNMP CPU, memory, temperature, uptime polling | ✓ WIRED | [internal/collector/performance.go](/home/azmin/projects/theia/internal/collector/performance.go:100) calls `snmp.PollDeviceMetrics`; [internal/snmp/discovery.go](/home/azmin/projects/theia/internal/snmp/discovery.go:761) performs real SNMP polling. |
| `internal/collector/performance.go` | `internal/snmp/discovery.go::PollInterfaceCounters` | Raw counter snapshot collection | ✓ WIRED | [internal/collector/performance.go](/home/azmin/projects/theia/internal/collector/performance.go:107) calls `snmp.PollInterfaceCounters`; [internal/snmp/discovery.go](/home/azmin/projects/theia/internal/snmp/discovery.go:141) returns raw counters by interface. |
| `internal/collector/performance.go` | `internal/vendor/registry.go::ResolvePerformanceOIDs` | Vendor-tier OID resolution | ✓ WIRED | [internal/collector/performance.go](/home/azmin/projects/theia/internal/collector/performance.go:99) resolves performance OIDs; [internal/vendor/registry.go](/home/azmin/projects/theia/internal/vendor/registry.go:331) merges vendor and fallback settings. |
| `internal/worker/metrics_collector.go` | `internal/collector/rates.go::ComputeCounterRates` | Existing SNMP link path delegates all rate math to the helper | ✓ WIRED | [internal/worker/metrics_collector.go](/home/azmin/projects/theia/internal/worker/metrics_collector.go:690) calls `collector.ComputeCounterRates`; [internal/collector/rates.go](/home/azmin/projects/theia/internal/collector/rates.go:22) defines the helper contract. |
| `internal/collector/rates.go` | `internal/collector/results.go::InterfaceCounterSnapshot` | Helper consumes the raw counter snapshots defined in Plan 01 | ✓ WIRED | [internal/collector/rates.go](/home/azmin/projects/theia/internal/collector/rates.go:23) accepts `[]InterfaceCounterSnapshot`; the type is defined at [internal/collector/results.go](/home/azmin/projects/theia/internal/collector/results.go:37). |
| `internal/collector/operational.go` | `internal/snmp/discovery.go::PollOperationalStatus` | Operational collector reuses the shared SNMP helper | ✓ WIRED | [internal/collector/operational.go](/home/azmin/projects/theia/internal/collector/operational.go:94) calls `snmp.PollOperationalStatus`; [internal/snmp/discovery.go](/home/azmin/projects/theia/internal/snmp/discovery.go:76) implements it. |
| `internal/collector/static.go` | `internal/snmp/discovery.go::DiscoverDevice` | Static collector wraps the existing discovery path | ✓ WIRED | [internal/collector/static.go](/home/azmin/projects/theia/internal/collector/static.go:88) calls `snmp.DiscoverDevice`; [internal/snmp/discovery.go](/home/azmin/projects/theia/internal/snmp/discovery.go:213) implements the discovery walk. |
| `internal/collector/prometheus.go` | `internal/metrics/prometheus.go` | Uses only hostname, probe, and alert queries through a narrowed interface | ✓ WIRED | [internal/collector/prometheus.go](/home/azmin/projects/theia/internal/collector/prometheus.go:19) allows only `QueryHostnames`, `QueryProbeStatus`, and `QueryAlerts`; corresponding concrete methods exist at [internal/metrics/prometheus.go](/home/azmin/projects/theia/internal/metrics/prometheus.go:349), [internal/metrics/prometheus.go](/home/azmin/projects/theia/internal/metrics/prometheus.go:378), and [internal/metrics/prometheus.go](/home/azmin/projects/theia/internal/metrics/prometheus.go:430). |
| `internal/collector/prometheus.go` | `internal/domain/device.go` | Resolves label name/value from device metadata with IP fallback | ✓ WIRED | [internal/collector/prometheus.go](/home/azmin/projects/theia/internal/collector/prometheus.go:42) and [internal/collector/prometheus.go](/home/azmin/projects/theia/internal/collector/prometheus.go:144) use `PrometheusLabelName`, `PrometheusLabelValue`, and IP fallback; the fields exist at [internal/domain/device.go](/home/azmin/projects/theia/internal/domain/device.go:104). |

### Data-Flow Trace (Level 4)

| Artifact | Data Variable | Source | Produces Real Data | Status |
| --- | --- | --- | --- | --- |
| `internal/collector/performance.go` | `result.Metrics`, `result.Counters` | `snmp.PollDeviceMetrics` + `snmp.PollInterfaceCounters` | [internal/snmp/discovery.go](/home/azmin/projects/theia/internal/snmp/discovery.go:141) and [internal/snmp/discovery.go](/home/azmin/projects/theia/internal/snmp/discovery.go:761) issue SNMP queries and parse live PDU values; no static payloads | ✓ FLOWING |
| `internal/collector/rates.go` | `metrics []domain.LinkMetrics` | Worker snapshots + explicit `previous` baseline | [internal/worker/metrics_collector.go](/home/azmin/projects/theia/internal/worker/metrics_collector.go:687) builds snapshots from polled counters and persists returned baselines | ✓ FLOWING |
| `internal/collector/operational.go` | `result.UptimeSecs`, `result.InterfaceStatuses` | `snmp.PollOperationalStatus` | [internal/snmp/discovery.go](/home/azmin/projects/theia/internal/snmp/discovery.go:76) performs `Get` and `BulkWalk` calls with partial-result handling | ✓ FLOWING |
| `internal/collector/static.go` | `SysName`, `Interfaces`, `Neighbors` | `snmp.DiscoverDevice` | [internal/snmp/discovery.go](/home/azmin/projects/theia/internal/snmp/discovery.go:213) issues real discovery queries and walks | ✓ FLOWING |
| `internal/collector/prometheus.go` | `Hostname`, `ProbeReachable`, alert groups | `PromClient.QueryHostnames`, `QueryProbeStatus`, `QueryAlerts` | [internal/metrics/prometheus.go](/home/azmin/projects/theia/internal/metrics/prometheus.go:349) through [internal/metrics/prometheus.go](/home/azmin/projects/theia/internal/metrics/prometheus.go:468) execute Prometheus API calls and map live payloads | ✓ FLOWING |

### Behavioral Spot-Checks

| Behavior | Command | Result | Status |
| --- | --- | --- | --- |
| Collector, SNMP helper, and worker packages test together | `PATH=/usr/local/go/bin:$PATH rtk go test ./internal/collector ./internal/snmp ./internal/worker -count=1` | `151 passed in 3 packages` | ✓ PASS |
| Collector contract, adapters, collectors, enrichment, and rate helper hold on current HEAD | `PATH=/usr/local/go/bin:$PATH rtk go test ./internal/collector -run 'Test.*Result|Test.*StateUpdate|TestPerformanceCollector|TestOperationalCollector|TestStaticCollector|TestPrometheusCollector|TestResolvePrometheusLabel|TestMapAlertsToDevices|TestComputeCounterRates' -count=1` | `37 passed in 1 package` | ✓ PASS |
| Operational SNMP helper covers required paths | `PATH=/usr/local/go/bin:$PATH rtk go test ./internal/snmp -run 'TestPollOperationalStatus' -count=1` | `4 passed in 1 package` | ✓ PASS |
| Worker seam enforces reset, gap, overspeed discard behavior | `PATH=/usr/local/go/bin:$PATH rtk go test ./internal/worker -run 'TestBuildSnapshot_SNMPLinkRates_|TestBuildSnapshot_SNMPLinkPollSkipsDeviceWithNoValidLinks' -count=1` | `4 passed in 1 package` | ✓ PASS |
| Phase code compiles in full application context | `PATH=/usr/local/go/bin:$PATH rtk go build ./...` | `Go build: Success` | ✓ PASS |

### Requirements Coverage

| Requirement | Source Plan | Description | Status | Evidence |
| --- | --- | --- | --- | --- |
| `PIPE-01` | `40-01`, `40-04` | SNMP is primary data source; Prometheus is optional enrichment | ? NEEDS HUMAN | Code-level contract is correct: [internal/collector/performance.go](/home/azmin/projects/theia/internal/collector/performance.go:99) uses only SNMP for core metrics/counters, and [internal/collector/prometheus.go](/home/azmin/projects/theia/internal/collector/prometheus.go:19) restricts Prometheus to enrichment. The "when configured and reachable" part still needs live SNMP/Prometheus validation. |
| `PIPE-02` | `40-01`, `40-03`, `40-04` | Stateless collectors exist per volatility class, wrapping existing SNMP/Prometheus clients | ✓ SATISFIED | [internal/collector/results.go](/home/azmin/projects/theia/internal/collector/results.go:15), [internal/collector/performance.go](/home/azmin/projects/theia/internal/collector/performance.go:18), [internal/collector/operational.go](/home/azmin/projects/theia/internal/collector/operational.go:17), [internal/collector/static.go](/home/azmin/projects/theia/internal/collector/static.go:16), and [internal/collector/prometheus.go](/home/azmin/projects/theia/internal/collector/prometheus.go:26) define stateless wrappers/contracts; package tests pass. |
| `PIPE-04` | `40-02` | Counter rate computation discards resets, overspeed, and first sample after a gap | ✓ SATISFIED | [internal/collector/rates.go](/home/azmin/projects/theia/internal/collector/rates.go:49) through [internal/collector/rates.go](/home/azmin/projects/theia/internal/collector/rates.go:75) implement the discard policy; [internal/collector/rates_test.go](/home/azmin/projects/theia/internal/collector/rates_test.go:41) and [internal/worker/metrics_collector_test.go](/home/azmin/projects/theia/internal/worker/metrics_collector_test.go:1479) verify helper and runtime behavior. |

No orphaned Phase 40 requirements were found: `REQUIREMENTS.md` maps exactly `PIPE-01`, `PIPE-02`, and `PIPE-04` to Phase 40, and all three appear in PLAN frontmatter.

### Anti-Patterns Found

| File | Line | Pattern | Severity | Impact |
| --- | --- | --- | --- | --- |
| `internal/collector/static_test.go` | 212 | Structural collaborator test only | ℹ️ Info | `TestStaticCollectorHasNoServiceOrRepositoryCollaborators` proves struct shape, not the full runtime absence of indirect DB writes; manual code inspection of [internal/collector/static.go](/home/azmin/projects/theia/internal/collector/static.go:34) covers the call path. |
| `internal/collector/performance.go` | 55 | Defensive nil/context error branches lack direct unit tests | ℹ️ Info | Current tests cover connect failure and success paths, but nil collector/registry/factory/nil client and context-cancel branches in the collector package are validated by inspection rather than explicit tests. |

### Human Verification Required

### 1. Live SNMP Collector Probe

**Test:** Instantiate `PerformanceCollector`, `OperationalCollector`, and `StaticCollector` against a reachable managed SNMP device with at least one linked interface.
**Expected:** `PerformanceCollector` returns SNMP metrics and raw counters with missing OIDs left nil, `OperationalCollector` returns reachability plus partial uptime/status data when some OIDs are absent, and `StaticCollector` returns inventory/topology data without DB or service side effects.
**Why human:** The repository only provides mocked SNMP clients; validating actual device/vendor behavior requires a live SNMP endpoint.

### 2. Live Prometheus Enrichment Probe

**Test:** Run `PrometheusCollector.CollectDeviceEnrichment()` and `CollectAlerts()` against the project's configured Prometheus using one device with explicit label metadata and one relying on IP fallback.
**Expected:** Hostname, probe reachability, and alert mappings populate correctly, while no core metric or throughput authority shifts away from SNMP.
**Why human:** The repository tests only a stubbed client; confirming real Prometheus label conventions and alert payloads needs a live Prometheus instance.

### Gaps Summary

No code or wiring gaps were found against the merged roadmap and PLAN must-haves. Phase 40 achieves its code-level goal on current HEAD, including the `928a621` fix to failed performance-store updates; remaining verification is limited to live SNMP and Prometheus integration, while production pipeline cutover is explicitly deferred to Phase 42.

---

_Verified: 2026-04-12T14:46:57Z_
_Verifier: Claude (gsd-verifier)_
