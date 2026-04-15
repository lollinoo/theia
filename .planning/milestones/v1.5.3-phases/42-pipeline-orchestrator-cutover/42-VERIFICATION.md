---
phase: 42-pipeline-orchestrator-cutover
verified: 2026-04-13T08:51:01Z
status: human_needed
score: 9/10 must-haves verified
overrides_applied: 0
human_verification:
  - test: "Start the app with a representative device inventory and confirm the topology map loads live data through the new pipeline runtime."
    expected: "Overview clients receive an initial snapshot, then periodic snapshot_delta updates, without Poller or MetricsCollector startup."
    why_human: "Requires a running backend, real or representative devices, and frontend rendering behavior that unit tests do not cover."
  - test: "Trigger a topology-changing static poll or reprobe while a WebSocket client is connected."
    expected: "Clients observe refreshed snapshot data before topology_changed, with no stale-map or split-brain behavior."
    why_human: "Ordering spans runtime timing, websocket delivery, and frontend state handling."
  - test: "Let mixed poll-class devices run long enough to observe scheduled performance, operational, and static polls."
    expected: "Devices update on their effective cadences and continue showing last-known performance data until staleness, rather than flickering empty on transient misses."
    why_human: "Classified scheduling and operator-visible freshness need runtime observation against a live system."
---

# Phase 42: Pipeline Orchestrator & Cutover Verification Report

**Phase Goal:** A unified PipelineOrchestrator replaces both Poller and MetricsCollector, wiring the scheduler to worker goroutines to the state engine to WebSocket broadcast -- completing the architectural transition
**Verified:** 2026-04-13T08:51:01Z
**Status:** human_needed
**Re-verification:** No -- initial verification

## Goal Achievement

### Observable Truths

| # | Truth | Status | Evidence |
| --- | --- | --- | --- |
| 1 | `cmd/theia/main.go` starts only `PipelineOrchestrator` for live polling and metrics delivery | ✓ VERIFIED | `cmd/theia/main.go:392-405,411,431` wires `worker.NewPipelineOrchestrator(...)`, `pipeline.Start(ctx)`, `ws.NewHandler(hub, pipeline.GetSnapshot, pipeline.IsPromAvailable)`, and `pipeline.Stop()`. `rg 'NewPoller\(|NewMetricsCollector\(|poller\.Start\(|collector\.Start\(|collector\.Stop\(' cmd/theia/main.go` returned no matches. |
| 2 | The state engine accepts tiered updates without performance and link data being clobbered by operational or failed performance polls | ✓ VERIFIED | `internal/state/store.go:67-93,141-165,366-476` adds `VolatilityClass`, `LinkMetrics`, tier-specific merge helpers, and clone/equality helpers. Regression tests at `internal/state/store_test.go:99,183,280,344` cover preservation, failed-performance retention, cloning, and link diff emission. |
| 3 | Collector adapters stamp state updates with explicit volatility classes | ✓ VERIFIED | `internal/collector/results.go:72-95,121-145,176` sets `VolatilityClassPerformance`, `VolatilityClassOperational`, and exposes `GetVolatilityClass()` for static results. Tests at `internal/collector/results_test.go:130,196,210` assert the markers. |
| 4 | Static discovery persistence is reusable outside the legacy probe path and topology notification remains caller-owned | ✓ VERIFIED | `internal/service/static_persistence.go:13-29` defines `StaticDiscoveryInput`, `StaticPersistenceResult`, and `ApplyStaticDiscovery`. `internal/service/device_service.go:212-238` reuses the helper in `probeDevice()` and only then emits `TopologyNotify`. Tests at `internal/service/static_persistence_test.go:23,108,200,249` cover metadata/interface persistence, link creation, override guard, and notify ownership. |
| 5 | `PipelineOrchestrator` is the single runtime owner of scheduler task consumption, collector dispatch, state-store updates, Prometheus refresh, and broadcast lifecycle | ✓ VERIFIED | `internal/worker/pipeline.go:33-98,212-317,335-423` defines the orchestrator, starts scheduler and store, runs workers, refreshes Prometheus, and performs fixed-tick broadcast. Tests at `internal/worker/pipeline_test.go:273,363,436,495` exercise performance/static task handling, Prometheus refresh, and lifecycle status. |
| 6 | Worker goroutines consume `PollTask`s by volatility class and always complete scheduler work items | ✓ VERIFIED | `internal/worker/pipeline.go:212-225` reads from `p.scheduler.Tasks()` and defers `p.scheduler.Complete(...)` with `RunID` and `TaskKey`; `internal/worker/pipeline.go:265,274,283-290` updates the state store from performance, operational, and static paths. Tests at `internal/worker/pipeline_test.go:273,363` assert scheduler completions and state updates. |
| 7 | Broadcasts are built from `state.Store.Snapshot()` on a fixed tick and preserve the existing `snapshot` / `snapshot_delta` overview contract | ✓ VERIFIED | `internal/worker/pipeline.go:358-423` builds broadcasts on `pipelineBroadcastInterval`; `internal/worker/snapshot_builder.go:30-306` preserves `device_metrics`, `link_metrics`, `alerts`, `device_statuses`, `device_hostnames`, and `device_models`. The frontend parser still expects exactly those sections in `frontend/src/types/metrics.ts:23-41,105-169`. Tests at `internal/worker/snapshot_builder_test.go:13,133,170` and `internal/worker/pipeline_test.go:613` cover section preservation and delta behavior. |
| 8 | `topology_changed` remains post-broadcast and forces a full snapshot when a topology event arrives without an overview delta | ✓ VERIFIED | `internal/worker/pipeline.go:395-423,525-537` drains `topologyNotify`, broadcasts snapshot/delta first, then emits `topology_changed`, with a forced full snapshot when delta is nil. Tests at `internal/worker/pipeline_test.go:644,659` verify ordering and forced snapshot behavior. |
| 9 | WebSocket bootstrap and health status plumbing use the pipeline runtime directly instead of concrete `*worker.Poller` coupling | ✓ VERIFIED | `cmd/theia/main.go:411` passes `pipeline.GetSnapshot` and `pipeline.IsPromAvailable` to `ws.NewHandler`. `internal/api/health_handler.go:11-22` defines `statusProvider`; `internal/api/router.go:29,46` accepts that interface and passes it to `NewHealthHandler`. `internal/api/health_handler_test.go:15-123` verifies running, stopped, and nil-provider behavior. |
| 10 | The application starts, polls devices on their classified schedules, and presents a working live topology map through the cutover | ? UNCERTAIN | Code and tests establish the runtime path, but the repository does not contain an app-level verification that boots the full backend and confirms live frontend behavior against real polling cadence. Human verification is required. |

**Score:** 9/10 truths verified

### Required Artifacts

| Artifact | Expected | Status | Details |
| --- | --- | --- | --- |
| `internal/state/store.go` | Tier-aware state-store merge contract | ✓ VERIFIED | Exists, substantive, and wired through `PipelineOrchestrator` updates and snapshot reads. |
| `internal/state/store_test.go` | Cross-tier regression coverage | ✓ VERIFIED | Covers performance preservation, failed-performance retention, cloning, and link diff change emission. |
| `internal/collector/results.go` | Collector-to-store adapters with volatility class | ✓ VERIFIED | Wired into pipeline task handling through `ToStoreUpdate()` and `GetVolatilityClass()`. |
| `internal/service/static_persistence.go` | Shared static discovery persistence seam | ✓ VERIFIED | Used by both `probeDevice()` and `PipelineOrchestrator` static task handling. |
| `internal/service/device_service.go` | Legacy probe path reusing shared persistence helper | ✓ VERIFIED | `probeDevice()` delegates persistence to `ApplyStaticDiscovery()` and owns notify send. |
| `internal/service/static_persistence_test.go` | Persistence and notify-order regression tests | ✓ VERIFIED | Tests helper behavior and shared probe path semantics. |
| `internal/worker/pipeline.go` | Unified runtime orchestrator | ✓ VERIFIED | Scheduler, collectors, state store, Prometheus status, and broadcast loop are all present and wired. |
| `internal/worker/snapshot_builder.go` | Snapshot/delta builder preserving WS contract | ✓ VERIFIED | Called from `broadcastOnce()` and aligned with frontend snapshot parser contract. |
| `internal/worker/pipeline_test.go` | Runtime behavior tests | ✓ VERIFIED | Covers performance/static paths, completions, broadcast ordering, and Prometheus transition signaling. |
| `cmd/theia/main.go` | Atomic runtime cutover | ✓ VERIFIED | Sole live runtime wiring now targets pipeline; old workers are not started from `main.go`. |
| `internal/api/health_handler.go` | Generic status-provider health seam | ✓ VERIFIED | Uses `Status() string` abstraction rather than `*worker.Poller`. |
| `internal/api/health_handler_test.go` | Health output regression coverage | ✓ VERIFIED | Verifies `snmp_poller` remains `running` / `stopped` with interface-based provider. |

### Key Link Verification

| From | To | Via | Status | Details |
| --- | --- | --- | --- | --- |
| `internal/state/store.go` | `internal/collector/results.go` | Volatility-marked collector results reach the state store | ✓ WIRED | `results.go:83-100,132-150` produces `state.StateUpdate{VolatilityClass: ...}`; `store.go:141-165` switches on `VolatilityClass`. |
| `internal/state/store.go` | `internal/ws/messages.go` | Stored metrics still feed the unchanged overview DTO contract | ✓ WIRED | `pipeline.go:395` reads `stateStore.Snapshot()`, `snapshot_builder.go:66-70` maps to `ws.DeviceMetricsToDTOs` / `ws.LinkMetricsToDTOs`, and `ws/messages.go:33-41` defines the unchanged payload sections. |
| `internal/service/static_persistence.go` | `internal/collector/static.go` | Static collector output is converted into service-owned persistence input | ✓ WIRED | `static.go:31-95` returns `StaticResult`; `pipeline.go:294-302` maps that result into `service.StaticDiscoveryInput`. |
| `internal/service/device_service.go` | `internal/worker/pipeline.go` | Legacy probe and new pipeline share one persistence seam | ✓ WIRED | `device_service.go:212` and `pipeline.go:294` both call `ApplyStaticDiscovery(...)`. |
| `internal/worker/pipeline.go` | `internal/scheduler/scheduler.go` | Scheduler tasks are consumed and completed | ✓ WIRED | `pipeline.go:212` reads `p.scheduler.Tasks()` and `pipeline.go:221-225` always defers `p.scheduler.Complete(...)`. The plan verifier's regex failure here was a tooling false positive. |
| `internal/worker/pipeline.go` | `internal/state/store.go` | Collector output flows through the state store before broadcast | ✓ WIRED | `pipeline.go:265,274,283-290` call `stateStore.Update(...)`; `pipeline.go:395` uses `stateStore.Snapshot()` for broadcast. |
| `internal/worker/snapshot_builder.go` | `frontend/src/types/metrics.ts` | Snapshot and delta payload shape remains frontend-compatible | ✓ WIRED | Builder outputs the six overview sections at `snapshot_builder.go:30-70`; frontend parses the same shape in `metrics.ts:23-41,105-169`. |
| `cmd/theia/main.go` | `internal/worker/pipeline.go` | Pipeline becomes the sole polling runtime | ✓ WIRED | `main.go:392-405` constructs and starts the pipeline; `main.go:411` binds WS bootstrap to pipeline getters. |
| `internal/api/health_handler.go` | `internal/api/router.go` | Router and health endpoint use a generic `Status() string` provider | ✓ WIRED | `health_handler.go:11-22` defines/uses `statusProvider`; `router.go:29,46` accepts that abstraction and constructs `HealthHandler`. The plan verifier's string-pattern miss here was also a tooling false positive. |

### Data-Flow Trace (Level 4)

| Artifact | Data Variable | Source | Produces Real Data | Status |
| --- | --- | --- | --- | --- |
| `internal/worker/pipeline.go` | `update` / `update.LinkMetrics` | `PerformanceCollector.Poll()` plus `collector.ComputeCounterRates(...)` | Yes | ✓ FLOWING |
| `internal/worker/pipeline.go` | `result` for static tasks | `StaticCollector.Poll()` then `ApplyStaticDiscovery(...)` | Yes | ✓ FLOWING |
| `internal/worker/pipeline.go` | `snapshot` | `cache.GetDevices()` / `cache.GetLinks()` plus `stateStore.Snapshot()` | Yes | ✓ FLOWING |
| `internal/worker/pipeline.go` | `alerts` / `hostnames` | `PrometheusCollector.QueryAlerts()` and `CollectDeviceEnrichment()` | Yes | ✓ FLOWING |

### Behavioral Spot-Checks

| Behavior | Command | Result | Status |
| --- | --- | --- | --- |
| Phase-owned backend packages still build and pass tests after cutover | `rtk go test ./internal/state ./internal/collector ./internal/service ./internal/worker ./internal/api ./cmd/theia -count=1` | `491 passed in 6 packages` | ✓ PASS |
| Orchestrator and snapshot-builder behaviors remain covered | `rtk go test ./internal/worker -run 'TestPipelineOrchestrator|TestBuildPipelineSnapshot|TestBuildDelta' -count=1` | `15 passed in 1 package` | ✓ PASS |
| Static persistence seam and shared probe path still behave correctly | `rtk go test ./internal/service -run 'TestApplyStaticDiscovery|TestProbeDeviceUsesApplyStaticDiscoveryAndSignalsTopologyNotify' -count=1` | `4 passed in 1 package` | ✓ PASS |

### Requirements Coverage

| Requirement | Source Plan | Description | Status | Evidence |
| --- | --- | --- | --- | --- |
| `PIPE-03` | `42-01`, `42-02`, `42-03`, `42-04` | Pipeline orchestrator replaces Poller + MetricsCollector, wiring scheduler to workers to state engine to WS broadcast | ✓ SATISFIED | `main.go:392-411` wires only pipeline; `pipeline.go:212-423` covers scheduler→collector→state→broadcast; `snapshot_builder.go:30-306` preserves WS contract; `health_handler.go:11-22` and `router.go:29,46` remove concrete poller coupling. |

### Anti-Patterns Found

No blocker anti-patterns found in the phase-owned implementation files. Grep hits were limited to ordinary slice literals in non-stub code paths and did not indicate placeholders, empty implementations, or user-visible hollow data.

### Disconfirmation Notes

- Partial requirement: roadmap success criterion 4 is only partially machine-verifiable here; it still needs a live-system check for actual polling cadence and frontend behavior.
- Misleading test: `cmd/theia/main_test.go:92` only proves the new SNMP client factory helper is constructible. It does not by itself prove that `main.go` boots only the pipeline runtime.
- Untested error path: there is no targeted failing-collector test proving `runTask()` still reports `scheduler.Complete(...)` on collector errors, even though static inspection of `pipeline.go:221-225` shows the defer is in the correct place.

### Human Verification Required

### 1. Live Cutover Smoke Test

**Test:** Start Theia with a representative device inventory and connect a normal overview client.
**Expected:** The client receives an initial `snapshot`, then ongoing `snapshot_delta` updates sourced from the new pipeline runtime, with live metrics continuing to refresh.
**Why human:** This requires the full backend, network inputs, and frontend rendering behavior; the repository only contains unit-level verification.

### 2. Topology Ordering Check

**Test:** Cause a topology-changing static poll or manual reprobe while a WebSocket client is connected.
**Expected:** The refreshed snapshot data reaches the client before `topology_changed`, and the UI does not show a stale or split-brain topology.
**Why human:** The ordering guarantee is implemented in code, but the observable contract spans runtime timing and frontend handling.

### 3. Classified Scheduling Smoke Test

**Test:** Let devices with different poll classes and overrides run long enough to observe performance, operational, and static cadence differences.
**Expected:** Effective schedules are respected, and transient performance misses preserve last-known overview metrics until staleness rather than clearing cards/links immediately.
**Why human:** This is an operator-visible runtime behavior that unit tests do not prove end to end.

### Gaps Summary

No code gaps were found against Phase 42's must-haves or requirement `PIPE-03`. The remaining work is human-only verification of live runtime behavior: startup against real devices, observable scheduling cadence, and frontend ordering after topology updates.

---

_Verified: 2026-04-13T08:51:01Z_  
_Verifier: Claude (gsd-verifier)_
