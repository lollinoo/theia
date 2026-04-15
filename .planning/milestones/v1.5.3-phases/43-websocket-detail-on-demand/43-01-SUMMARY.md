---
phase: 43-websocket-detail-on-demand
plan: 01
subsystem: backend
tags: [go, websocket, snapshot-delta, subscriptions]
requires:
  - phase: 42-pipeline-orchestrator-cutover
    provides: "Existing snapshot and snapshot_delta contract plus pipeline targeted-send seam"
provides:
  - "Typed subscribe_detail and unsubscribe_detail control parsing on the shared WebSocket connection"
  - "Additive detail-ready device_metrics fields for health, reachability, staleness, last poll time, and expected poll interval"
  - "One-device-per-client detail subscription tracking and safe disconnect cleanup in the WebSocket hub"
affects: [pipeline-orchestrator, websocket-hub, frontend-snapshot-types]
tech-stack:
  added: []
  patterns: [shared snapshot detail fields, one-device-per-client subscriptions, log-and-ignore control parsing]
key-files:
  created: [internal/ws/messages_test.go, internal/ws/hub_test.go]
  modified: [internal/ws/messages.go, internal/ws/hub.go]
key-decisions:
  - "Detail-on-demand control traffic reuses the existing socket and snapshot_delta contract instead of adding a second message family."
  - "Each Client stores exactly one detailDeviceID under hub mutex protection, so subscribe_detail always replaces prior selection."
  - "Malformed or unsupported inbound control frames are logged and ignored so the socket lifecycle stays intact."
patterns-established:
  - "Device-level real-time enrichments should extend DeviceMetricsDTO additively so overview and detail updates share the same merge path."
  - "Targeted WebSocket delivery should resolve recipients through Hub.DetailSubscribers instead of ad-hoc client filtering."
requirements-completed: [WS-02]
duration: 1m
completed: 2026-04-13
---

# Phase 43 Plan 01: WebSocket Contract And Hub Subscription Summary

**WebSocket detail subscriptions now reuse the shared snapshot contract, with typed subscribe/unsubscribe control parsing and one active device selection per client**

## Performance

- **Duration:** 1m
- **Started:** 2026-04-13T13:44:46Z
- **Completed:** 2026-04-13T13:45:04Z
- **Tasks:** 2
- **Files modified:** 4

## Accomplishments

- Extended `internal/ws/messages.go` so `DeviceMetricsDTO` can carry additive detail metadata without introducing a parallel `device_detail` payload.
- Added pure parsing for `subscribe_detail` and `unsubscribe_detail`, including safe empty-device unsubscribe behavior and invalid UUID rejection.
- Taught the WebSocket hub to track one detail device per client, clear subscriptions on unsubscribe/disconnect, and expose `DetailSubscribers` for later targeted pipeline sends.

## Task Commits

Each task was committed atomically:

1. **Task 1: Extend the shared WebSocket contract with additive detail fields and typed control parsing** - `e2d94cf` (feat)
2. **Task 2: Track one active detail device per client and wire control-message handling into readPump** - `461f049` (feat)

## Files Created/Modified

- `internal/ws/messages.go` - adds detail-ready `DeviceMetricsDTO` fields, control message constants, and the typed parser helper.
- `internal/ws/messages_test.go` - covers subscribe/unsubscribe parsing and snapshot clone preservation of optional detail fields.
- `internal/ws/hub.go` - stores per-client detail subscriptions, handles inbound control messages, and clears state on remove.
- `internal/ws/hub_test.go` - verifies subscription replacement, filtering, unsubscribe cleanup, and disconnect cleanup behavior.

## Decisions Made

- `unsubscribe_detail` accepts a missing or empty `payload.device_id` and maps it to `uuid.Nil`, letting the hub clear the current selection without trusting the caller to repeat the previous device ID.
- `subscribe_detail` rejects nil or malformed UUIDs so hub state is never updated with an invalid target.
- `removeClient()` now nil-checks `client.conn` before closing, which keeps disconnect cleanup safe in tests and non-networked call sites.

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered

None.

## User Setup Required

None - no external setup or migration required.

## Next Phase Readiness

- `internal/worker/pipeline.go` can now ask `Hub.DetailSubscribers(deviceID)` for the exact clients that should receive post-poll detail deltas.
- Frontend typing and socket work in plan `43-03` should mirror the exact field names added here: `health`, `reachability`, `stale`, `last_polled_at`, and `expected_poll_interval_seconds`.

## Self-Check: PASSED

- Verified `go test ./internal/ws -run 'TestParseClientControlMessage|TestCloneSnapshot' -count=1 -v` passes.
- Verified `go test ./internal/ws -run 'TestHub(Set|Detail|Clear|Remove)' -count=1 -v` passes.
- Verified `go test ./internal/ws -count=1` passes.
- Verified task commits `e2d94cf` and `461f049` exist in `git log --oneline --all`.

---
*Phase: 43-websocket-detail-on-demand*
*Completed: 2026-04-13*
