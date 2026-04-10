---
phase: 28-api-call-optimization-especially-websocket-payload-optimizat
plan: 02
subsystem: ui
tags: [websocket, typescript, react, hooks, delta, metrics]

# Dependency graph
requires:
  - phase: 28-api-call-optimization-especially-websocket-payload-optimizat
    plan: 01
    provides: snapshot_delta WS message type and server-side delta emission in MetricsCollector

provides:
  - Frontend TypeScript types for snapshot_delta WS message (SnapshotDeltaWSMessage interface, WSMessageType union extension)
  - mergeSnapshotDelta pure function for deep-merging sparse delta payloads into existing snapshot state
  - useWebSocket hook handles snapshot_delta messages with functional setState (stale-closure-safe)
  - Full test coverage: 6 unit tests in metrics.test.ts, 5 new integration tests in useWebSocket.test.ts

affects:
  - Any future phase touching useWebSocket.ts or metrics.ts WS message types

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Delta merge with spread operator — device_metrics/link_metrics/device_statuses/device_hostnames merged individually; alerts use whole-set replacement"
    - "Functional setState for WS delta handling — setSnapshot((prev) => ...) avoids stale closures in async message handlers"
    - "TDD RED→GREEN flow for TypeScript — write failing tests first, then implement to satisfy them"

key-files:
  created:
    - frontend/src/types/metrics.test.ts
  modified:
    - frontend/src/types/metrics.ts
    - frontend/src/hooks/useWebSocket.ts
    - frontend/src/hooks/useWebSocket.test.ts

key-decisions:
  - "Alerts use whole-set replacement semantics — empty delta alerts = no change, non-empty delta alerts = replace entirely (per D-07)"
  - "Delta before base snapshot is a no-op — prev === null guard in functional setState returns null unchanged"
  - "SnapshotDeltaWSMessage reuses SnapshotPayload type — wire format is identical shape, just sparse maps"

patterns-established:
  - "mergeSnapshotDelta: pure function in metrics.ts — testable in isolation, no React dependency"
  - "Functional setState form for WS-driven state merges — prevents stale closure with multiple rapid messages"

requirements-completed: []

# Metrics
duration: 3min
completed: 2026-04-08
---

# Phase 28 Plan 02: Frontend snapshot_delta Message Handling Summary

**Frontend deep-merges sparse WebSocket delta payloads into React state using SnapshotDeltaWSMessage type and mergeSnapshotDelta pure function — completing the client side of the 28-01 server-side delta optimization**

## Performance

- **Duration:** ~3 min
- **Started:** 2026-04-08T15:50:37Z
- **Completed:** 2026-04-08T15:53:13Z
- **Tasks:** 2
- **Files modified:** 3 (+ 1 created)

## Accomplishments

- Extended `WSMessageType` union with `'snapshot_delta'` and added `SnapshotDeltaWSMessage` interface in `metrics.ts`
- Implemented `mergeSnapshotDelta` pure function with spread-based merge for record maps and whole-set replacement for alerts
- Updated `parseWSMessage` to handle `snapshot_delta` type and included `SnapshotDeltaWSMessage` in return type signature
- Added `snapshot_delta` branch in `useWebSocket.ts` using functional `setSnapshot((prev) => ...)` to avoid stale closures
- Delta received before any full snapshot is safely ignored (null guard in functional setState)
- Created `metrics.test.ts` with 6 tests; added 5 tests to `useWebSocket.test.ts` — all 451 tests pass

## Task Commits

Each task was committed atomically:

1. **Task 1: Add snapshot_delta type to metrics.ts and parse support** - `b1b52b2` (feat)
2. **Task 2: Handle snapshot_delta in useWebSocket hook with deep merge + tests** - `8a32189` (feat)

_Note: TDD tasks have test+impl in same commit (test written first, verified RED, then GREEN implemented)_

## Files Created/Modified

- `frontend/src/types/metrics.ts` - Added `snapshot_delta` to WSMessageType, `SnapshotDeltaWSMessage` interface, `mergeSnapshotDelta` function, updated `parseWSMessage` return type and body
- `frontend/src/types/metrics.test.ts` - New file: 6 unit tests for `parseWSMessage` with `snapshot_delta` and `mergeSnapshotDelta` behavior
- `frontend/src/hooks/useWebSocket.ts` - Added `mergeSnapshotDelta`/`SnapshotDeltaWSMessage` imports, added `snapshot_delta` handler branch with functional setState
- `frontend/src/hooks/useWebSocket.test.ts` - Added 5 tests: merge correctness, ignore-before-base-snapshot, full-snapshot-replaces-after-delta, alerts-replaced, alerts-preserved

## Decisions Made

- Alerts use whole-set replacement semantics: empty delta alerts array means no alert change (preserve existing), non-empty means the alert set changed (replace entirely). Consistent with D-07 from context.
- Delta before base snapshot is a no-op: the `prev === null` guard in the functional setState returns `null` unchanged, matching D-04 (first message is always a full snapshot).
- `SnapshotDeltaWSMessage` reuses `SnapshotPayload` as its payload type — the wire format is the same shape (same fields), just sparsely populated. No new type needed for the payload itself.

## Deviations from Plan

None — plan executed exactly as written.

## Issues Encountered

None. The `useTheme must be used within ThemeProvider` console warnings in the full test run are pre-existing (not introduced by this plan) and do not affect test results — all 451 tests pass.

## Known Stubs

None — all delta merge logic is fully wired. The `mergeSnapshotDelta` function is called with live WS message payloads, not placeholder data.

## Next Phase Readiness

- WebSocket delta optimization is complete end-to-end: server emits `snapshot_delta` (28-01), frontend receives and deep-merges (28-02)
- No blockers for downstream phases

---
*Phase: 28-api-call-optimization-especially-websocket-payload-optimizat*
*Completed: 2026-04-08*

## Self-Check: PASSED

| Item | Status |
|------|--------|
| frontend/src/types/metrics.ts | FOUND |
| frontend/src/types/metrics.test.ts | FOUND |
| frontend/src/hooks/useWebSocket.ts | FOUND |
| frontend/src/hooks/useWebSocket.test.ts | FOUND |
| 28-02-SUMMARY.md | FOUND |
| commit b1b52b2 (Task 1) | FOUND |
| commit 8a32189 (Task 2) | FOUND |
