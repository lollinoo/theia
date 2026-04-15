---
phase: 43-websocket-detail-on-demand
plan: 02
subsystem: backend
tags: [go, websocket, pipeline, snapshot-delta, state-store]
requires:
  - phase: 43-websocket-detail-on-demand
    provides: "WebSocket detail subscription registry and additive detail-ready DeviceMetricsDTO fields from plan 01"
provides:
  - "Single-device detail delta builder on the shared snapshot contract"
  - "Targeted snapshot_delta sends to subscribed clients immediately after normal poll completion"
  - "Runtime tests proving no detail leakage to unsubscribed clients and no link-metric widening"
affects: [pipeline-orchestrator, websocket-hub, frontend-detail-mode]
tech-stack:
  added: []
  patterns: [targeted post-poll snapshot_delta, device-level detail-only payloads, websocket integration tests with real handler]
key-files:
  created: []
  modified: [internal/worker/snapshot_builder.go, internal/worker/snapshot_builder_test.go, internal/worker/pipeline.go, internal/worker/pipeline_test.go]
key-decisions:
  - "Detail delivery reuses snapshot_delta and DeviceMetricsDTO instead of adding a dedicated device-detail payload branch."
  - "runTask() publishes subscribed detail immediately after stateStore.Update(...) in performance, operational, and static branches, without touching broadcastOnce cadence."
  - "Targeted detail deltas intentionally omit link_metrics, alerts, hostnames, and models so Phase 43 stays device-level only."
patterns-established:
  - "Backend targeted detail sends should resolve recipients through Hub.DetailSubscribers and send via Hub.SendTo, never via overview Broadcast."
  - "Worker runtime tests can prove websocket subscription behavior end to end by pairing ws.NewHandler with httptest and real gorilla websocket clients."
requirements-completed: [WS-02]
duration: 3m
completed: 2026-04-13
---

# Phase 43 Plan 02: Targeted Detail Delivery Summary

**Pipeline runTask now emits subscribed device-level snapshot_delta payloads immediately after normal polls, while the overview broadcast loop remains unchanged**

## Performance

- **Duration:** 3m
- **Started:** 2026-04-13T13:52:14Z
- **Completed:** 2026-04-13T13:55:13Z
- **Tasks:** 2
- **Files modified:** 4

## Accomplishments

- Added `buildDeviceDetailDelta(device, deviceState)` so the backend can build one-device detail payloads on the existing snapshot contract, including health, reachability, staleness, last poll time, and expected interval.
- Extended snapshot hash computation to include the additive detail fields, keeping future delta detection aligned with the expanded `DeviceMetricsDTO`.
- Wired `PipelineOrchestrator.runTask()` to publish targeted `snapshot_delta` messages to subscribed clients right after performance, operational, and static poll updates.
- Added real websocket integration tests that prove subscribed delivery, no leakage to unsubscribed clients, and omission of `link_metrics` from targeted detail payloads.

## Task Commits

Each task was committed atomically:

1. **Task 1: Build a single-device detail delta helper on the shared snapshot contract** - `c80afbf` (feat)
2. **Task 2: Emit targeted post-poll detail deltas only to subscribed clients** - `3c1fa9f` (feat)

## Files Created/Modified

- `internal/worker/snapshot_builder.go` - adds `buildDeviceDetailDelta`, optional detail hashing helpers, and shared pointer-format helpers for the expanded device-metric hash.
- `internal/worker/snapshot_builder_test.go` - verifies detail field embedding, omission of unrelated sections, and hash changes when detail fields differ.
- `internal/worker/pipeline.go` - adds `publishSubscribedDetailDelta` and calls it from each `runTask()` volatility branch after state updates.
- `internal/worker/pipeline_test.go` - uses a real websocket handler plus gorilla clients to verify subscribed delivery behavior and no detail leakage.

## Decisions Made

- Targeted sends are driven by `stateStore.GetDevice(device.ID)` after each normal poll, so detail delivery uses the existing in-memory state model instead of a forced extra poll.
- Performance-path tests seed operational reachability first, matching the Phase 42 state model where operational updates own reachability and performance updates enrich the same device state.
- The targeted detail path leaves `broadcastOnce()` untouched and produces no hub broadcast messages during `runTask()`, proving cadence stays on the fixed Phase 42 overview loop.

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered

None.

## User Setup Required

None - no external setup or migration required.

## Next Phase Readiness

- The frontend can now rely on selected-device `snapshot_delta` delivery after normal polls without adding a second transport or state atom.
- Phase `43-03` only needs to send subscribe/unsubscribe control frames and derive device ownership from canvas panel lifecycle; backend targeted delivery is already in place.

## Self-Check: PASSED

- Verified `go test ./internal/worker -run 'TestBuildDeviceDetailDelta|TestComputeSnapshotHashes_DeviceMetricHashIncludesDetailFields' -count=1 -v` passes.
- Verified `go test ./internal/worker -run 'TestPipelineOrchestratorRunTask_(PerformancePollSendsDetailDeltaToSubscribedClient|OperationalPollSendsDetailDeltaToSubscribedClient|DetailDeltaDoesNotReachUnsubscribedClient|DetailDeltaOmitsLinkMetrics)' -count=1 -v` passes.
- Verified `go test ./internal/worker -count=1` passes.
- Verified task commits `c80afbf` and `3c1fa9f` exist in `git log --oneline --all`.

---
*Phase: 43-websocket-detail-on-demand*
*Completed: 2026-04-13*
