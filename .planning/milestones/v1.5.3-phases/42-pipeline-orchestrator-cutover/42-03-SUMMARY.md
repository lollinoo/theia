---
phase: 42-pipeline-orchestrator-cutover
plan: 03
subsystem: backend
tags: [go, scheduler, snmp, websocket, pipeline, prometheus]
requires:
  - phase: 40-collectors
    provides: "Per-tier performance, operational, static, and Prometheus collectors"
  - phase: 41-jittered-scheduler
    provides: "PollTask queueing and completion accounting for the new runtime"
provides:
  - "PipelineOrchestrator runtime that owns scheduler task execution, state-store updates, Prometheus refresh, and fixed-tick broadcast"
  - "Snapshot/delta builder that preserves the existing overview WebSocket payload sections and status-string contract"
  - "Topology notify draining after snapshot/delta broadcast with explicit prometheus_status transition signaling"
affects: [pipeline-orchestrator, websocket-overview, scheduler, prometheus-enrichment]
tech-stack:
  added: []
  patterns: [fixed-tick broadcast loop, scheduler-complete defer, state-store-backed snapshot building]
key-files:
  created: [internal/worker/pipeline.go, internal/worker/pipeline_test.go, internal/worker/snapshot_builder.go, internal/worker/snapshot_builder_test.go]
  modified: []
key-decisions:
  - "PipelineOrchestrator is now the single owner of scheduler task execution, collector dispatch, state-store writes, Prometheus refresh, and snapshot broadcast timing."
  - "Snapshot building reads from state.Store plus cache-backed topology data and preserves the existing snapshot/snapshot_delta contract rather than inventing a new payload."
  - "topologyNotify stays caller-owned and is only drained after snapshot or delta broadcast, with a forced full snapshot when topology changed but the overview delta is empty."
patterns-established:
  - "Counter-rate baselines live in prevCounters keyed by device ID and interface name so performance polls can compute link throughput without mutating collector state."
  - "Prometheus availability broadcasts only on transitions, while alerts and hostnames stay as best-effort enrichment over the SNMP-primary runtime."
requirements-completed: [PIPE-03]
duration: 27m
completed: 2026-04-13
---

# Phase 42 Plan 03: Pipeline Orchestrator Runtime Summary

**Unified scheduler-driven pipeline runtime with contract-preserving snapshot/delta broadcasts and post-broadcast topology ordering**

## Performance

- **Duration:** 27m
- **Started:** 2026-04-13T07:56:58Z
- **Completed:** 2026-04-13T08:23:58Z
- **Tasks:** 3
- **Files modified:** 4

## Accomplishments

- Extracted a dedicated snapshot/delta builder that preserves the existing overview sections: `device_metrics`, `link_metrics`, `alerts`, `device_statuses`, `device_hostnames`, and `device_models`.
- Implemented `PipelineOrchestrator` as the runtime owner for scheduler task consumption, per-tier collector dispatch, state-store updates, topology persistence, Prometheus refresh, and snapshot sourcing.
- Preserved legacy WebSocket ordering guarantees by broadcasting on a fixed tick, sending `prometheus_status` only on availability transitions, and draining `topologyNotify` only after snapshot or delta emission.

## Task Commits

Each task was committed atomically:

1. **Task 1: Extract a snapshot/delta builder that preserves the current overview contract** - `3c76008` (test), `c373b7f` (feat)
2. **Task 2: Implement PipelineOrchestrator worker runtime, tier dispatch, and scheduler completion** - `d3e94ec` (test), `b8acee3` (feat)
3. **Task 3: Add fixed-tick broadcast, Prometheus status signaling, and post-broadcast topology_changed ordering** - `8734efb` (test), `32e2ae9` (feat)

## Worker And Broadcast Layout

- `Start(ctx)` starts the state store and scheduler, then launches a worker pool sized from `domain.SettingSNMPWorkerPoolSize` with a fallback of `5`.
- Worker goroutines read `scheduler.Tasks()` and route each task through the volatility-specific collector path.
- `runTask(...)` defers `scheduler.Complete(...)` so task failures cannot leak in-flight scheduler slots.
- One Prometheus refresh loop runs every 5 seconds and updates in-memory alerts plus availability state.
- One broadcast loop runs every 5 seconds, builds a fresh snapshot from cache + state store, computes section hashes, emits either `snapshot` or `snapshot_delta`, and only then drains `topologyNotify`.

## Counter Baselines

- Performance polls compute link throughput through `collector.ComputeCounterRates(...)`.
- Baselines are stored in `prevCounters map[uuid.UUID]map[string]collector.CounterBaseline`, keyed by device ID and interface name.
- Each successful performance poll advances the stored baseline after the computed link metrics are attached to the state update, so the next poll can compute rates without hidden collector-local state.

## Snapshot Compatibility

- `buildPipelineSnapshot(...)` maps state-store data back into the existing DTOs instead of exposing internal reachability or health enums directly.
- `ReachabilityUp` maps to `up`; `ReachabilitySoftDown` and `ReachabilityHardDown` map to `down`; otherwise the persisted device status is used.
- Hostnames prefer persisted `SysName`, then Prometheus enrichment overrides; hardware models continue to come from persisted `HardwareModel`.
- `buildDelta(...)` reuses FNV-64a section hashing so unchanged sections are suppressed exactly like the legacy collector behavior.

## Files Created

- `internal/worker/snapshot_builder.go` - overview snapshot/delta construction from cache, state-store, alerts, and hostname side data.
- `internal/worker/snapshot_builder_test.go` - coverage for section preservation, reachability-to-status mapping, and delta suppression.
- `internal/worker/pipeline.go` - scheduler-driven runtime, Prometheus refresh loop, fixed-tick broadcast loop, topology notify drain, and direct snapshot getters.
- `internal/worker/pipeline_test.go` - coverage for scheduler completion, static persistence + notify behavior, Prometheus refresh/status, and broadcast ordering semantics.

## Decisions Made

- Added `broadcastOnce(...)` and `refreshPrometheusOnce(...)` helpers even though the plan only required loops; these smaller units make the runtime behavior directly testable without waiting on real timers.
- Kept `GetSnapshot()` returning a cloned snapshot so `ws.NewHandler` consumers cannot mutate broadcast state through shared pointers.
- Forced a full `snapshot` when `topologyNotify` is drained but the overview delta is nil, preserving the "fresh snapshot first, topology_changed second" contract even when topology changes do not alter overview sections.

## Deviations from Plan

None - plan goals and acceptance criteria landed as requested.

## Issues Encountered

None after the runtime and broadcast helpers were completed.

## User Setup Required

None - no external setup or migration commands required.

## Next Phase Readiness

- `cmd/theia/main.go` can now cut over to `worker.NewPipelineOrchestrator(...)` without inventing more runtime behavior.
- Health/router wiring can depend on `Status() string`, `GetSnapshot()`, and `IsPromAvailable()` directly.
- The scheduler, collectors, state store, topology persistence seam, and WebSocket contract are now connected end to end.

## Self-Check: PASSED

- Summary file exists at `.planning/phases/42-pipeline-orchestrator-cutover/42-03-SUMMARY.md`.
- Verified task commits `3c76008`, `c373b7f`, `d3e94ec`, `b8acee3`, `8734efb`, and `32e2ae9` exist in `git log --oneline --all`.
- Verified `go test ./internal/worker -count=1` passes.
