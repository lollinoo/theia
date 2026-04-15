---
phase: 43-websocket-detail-on-demand
plan: 03
subsystem: frontend
tags: [react, typescript, websocket, canvas, snapshot-delta]
requires:
  - phase: 43-websocket-detail-on-demand
    provides: "Backend subscribe/unsubscribe parsing and targeted detail delta delivery from plans 01 and 02"
provides:
  - "Shared frontend snapshot types for additive device detail fields"
  - "Single-socket subscribe/unsubscribe and reconnect resubscribe behavior in useWebSocket()"
  - "Canvas-owned device-panel lifecycle that derives the active detail device ID without new UI chrome"
affects: [frontend-snapshot-parser, websocket-hook, canvas-panels]
tech-stack:
  added: []
  patterns: [single socket owner, derived subscription lifecycle from panelContent, pure panel-type resolver helper]
key-files:
  created: [frontend/src/components/canvas/detailSubscription.ts, frontend/src/components/canvas/detailSubscription.test.ts]
  modified: [frontend/src/types/metrics.ts, frontend/src/types/metrics.test.ts, frontend/src/hooks/useWebSocket.ts, frontend/src/hooks/useWebSocket.test.ts, frontend/src/App.tsx, frontend/src/components/Canvas.tsx]
key-decisions:
  - "useWebSocket() remains the only websocket and snapshot owner; detail mode is represented only by an optional detailDeviceId input."
  - "Canvas.tsx owns subscription lifecycle because it already owns panelContent transitions, close paths, and device-panel switching."
  - "Only deviceConfig and device-scoped interfaceStats panels resolve to a detail device ID; link panels and all other side panels resolve to null."
patterns-established:
  - "Future frontend websocket enrichments should extend the shared snapshot parser and reuse the existing mergeSnapshotDelta path instead of creating panel-local caches."
  - "UI lifecycle decisions that translate into socket commands should be derived from a pure helper so panel ownership remains testable without rendering the full canvas."
requirements-completed: [WS-02]
duration: 1m
completed: 2026-04-13
---

# Phase 43 Plan 03: Frontend Detail Subscription Summary

**Frontend canvas panels now drive subscribe/unsubscribe over the existing websocket, while shared snapshot parsing and reconnect resubscribe stay on the single useWebSocket owner**

## Performance

- **Duration:** 1m
- **Started:** 2026-04-13T13:57:35Z
- **Completed:** 2026-04-13T13:58:37Z
- **Tasks:** 2
- **Files modified:** 8

## Accomplishments

- Extended the shared frontend snapshot types and parser so `DeviceMetricsDTO` preserves the additive backend detail fields without altering top-level snapshot merge behavior.
- Updated `useWebSocket(url, detailDeviceId)` to send `subscribe_detail` and `unsubscribe_detail` on the existing socket and to re-send the current device subscription on reconnect.
- Lifted `detailDeviceId` into `App.tsx` and made `Canvas.tsx` derive that value from `panelContent`, keeping Canvas as the sole owner of detail subscription lifecycle.
- Added a pure helper and tests proving that only `deviceConfig` and device-scoped `interfaceStats` panels own detail subscriptions in this phase.

## Task Commits

Each task was committed atomically:

1. **Task 1: Add additive detail fields to the shared snapshot types and teach the single websocket hook to send control messages** - `e51d217` (feat)
2. **Task 2: Make Canvas.tsx the sole owner of detail subscription lifecycle and lift the selected detail device into App.tsx** - `4379f1e` (feat)

## Files Created/Modified

- `frontend/src/types/metrics.ts` - adds optional detail fields plus parser helpers for optional strings, booleans, and nullable numbers.
- `frontend/src/types/metrics.test.ts` - verifies the parser preserves additive detail fields.
- `frontend/src/hooks/useWebSocket.ts` - sends subscribe/unsubscribe control frames, tracks the last subscribed device, and re-sends the active device after reconnect.
- `frontend/src/hooks/useWebSocket.test.ts` - covers subscribe on open, unsubscribe on clear, unsubscribe-then-subscribe on switch, and reconnect resubscribe behavior.
- `frontend/src/App.tsx` - lifts `detailDeviceId` into the single websocket owner and passes it down to `Canvas`.
- `frontend/src/components/Canvas.tsx` - derives `detailDeviceId` from `panelContent` and emits lifecycle changes through `onDetailDeviceChange`.
- `frontend/src/components/canvas/detailSubscription.ts` - pure resolver for device-owning panel types.
- `frontend/src/components/canvas/detailSubscription.test.ts` - proves device-only panels subscribe and link/other panels do not.

## Decisions Made

- The derived-effect pattern in `Canvas.tsx` intentionally lets cleanup emit `null` before the next panel value, which naturally gives the websocket hook an unsubscribe-before-subscribe sequence on device switches.
- `CanvasPanels.tsx` stayed render-only; lifecycle ownership lives one level up in `Canvas.tsx` where close, pane-click, and replace-panel behavior already exists.
- Reconnect logic stores the current intended device in refs so reconnect sends exactly one subscribe for the current device rather than replaying stale transitions.

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered

None.

## User Setup Required

None - no external setup or migration required.

## Next Phase Readiness

- `WS-02` is now wired end to end in code: backend targeted detail delivery, frontend control messages, and canvas panel ownership all align on the same socket and snapshot atom.
- Phase 44 can focus on presentation of health, freshness, and polling interval labels because the transport and lifecycle plumbing are already in place.

## Self-Check: PASSED

- Verified `npm test -- src/types/metrics.test.ts src/hooks/useWebSocket.test.ts` passes.
- Verified `npm test -- src/components/canvas/detailSubscription.test.ts src/hooks/useWebSocket.test.ts` passes.
- Verified task commits `e51d217` and `4379f1e` exist in `git log --oneline --all`.

---
*Phase: 43-websocket-detail-on-demand*
*Completed: 2026-04-13*
