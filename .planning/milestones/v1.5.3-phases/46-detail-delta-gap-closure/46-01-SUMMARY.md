---
phase: 46-detail-delta-gap-closure
plan: 01
subsystem: backend
tags: [go, typescript, websocket, snapshot-delta, targeted-detail]
requires:
  - phase: 43-websocket-detail-on-demand
    provides: "Per-client device detail subscriptions and the existing publishSubscribedDetailDelta runtime seam"
  - phase: 45-polling-cadence-gap-closure
    provides: "Performance-owned freshness metadata that targeted detail deltas must preserve"
provides:
  - "Selected-device link_metrics on the existing targeted snapshot_delta payload"
  - "Worker regressions proving subscribed-only delivery and no unsubscribed leakage for targeted link metrics"
  - "Frontend regressions proving sparse targeted link_metrics deltas merge into the shared snapshot atom without clearing other devices"
affects: [websocket-detail-mode, interface-stats-panel, shared-snapshot-merge]
tech-stack:
  added: []
  patterns:
    - Selected-device targeted detail payloads widen existing snapshot sections by one device key instead of adding a new websocket family
    - Frontend detail updates continue to merge through the existing shared snapshot atom and mergeSnapshotDelta seam
key-files:
  created:
    - .planning/phases/46-detail-delta-gap-closure/46-01-SUMMARY.md
  modified:
    - internal/worker/snapshot_builder.go
    - internal/worker/snapshot_builder_test.go
    - internal/worker/pipeline_test.go
    - frontend/src/types/metrics.test.ts
    - frontend/src/hooks/useWebSocket.test.ts
key-decisions:
  - "Targeted detail stays on snapshot_delta and now populates only link_metrics[device.id] alongside the existing device_metrics and device_statuses sections."
  - "publishSubscribedDetailDelta() remained the only runtime delivery seam; the closure work was payload composition plus tighter regressions, not a new send path."
  - "Frontend correctness stayed on the existing mergeSnapshotDelta shared atom path; no second websocket, cache, or panel-local state was introduced."
patterns-established:
  - "Worker targeted-detail regressions should seed a counter baseline before asserting preserved throughput on later operational detail sends."
  - "Sparse link_metrics deltas are safe so long as they overwrite only one device key and the frontend merges by top-level map spread."
requirements-completed: [WS-02]
duration: 6m50s
completed: 2026-04-14
---

# Phase 46 Plan 01: Detail Delta Gap Closure Summary

**Targeted snapshot_delta payloads now include selected-device link_metrics, and worker/frontend regressions lock the existing shared detail path without widening broadcasts or adding a second cache**

## Performance

- **Duration:** 6m50s
- **Started:** 2026-04-14T08:53:40Z
- **Completed:** 2026-04-14T09:00:30Z
- **Tasks:** 3
- **Files modified:** 5

## Accomplishments

- Updated `buildDeviceDetailDelta()` to emit only the selected device's `link_metrics[device.id]` on the existing targeted `snapshot_delta` contract while leaving alerts, hostnames, and models empty.
- Replaced the old worker omission regression with subscriber-only delivery coverage that proves selected-device link metrics reach subscribed clients, stay out of unsubscribed clients, and remain present on later same-device operational detail sends.
- Added frontend regression coverage that proves sparse targeted `link_metrics` deltas merge into the existing shared snapshot atom without clearing other devices' interface data.

## Verification Results

- `rtk go test ./internal/worker -run 'TestBuildDeviceDetailDelta_(EmbedsOptionalDetailFieldsInDeviceMetrics|IncludesSelectedDeviceLinkMetricsOnly)' -count=1 -v` passed.
- `rtk go test ./internal/worker -run 'TestPipelineOrchestratorRunTask_(PerformancePollSendsOnlySelectedDeviceLinkMetricsToSubscribedClient|DetailDeltaKeepsPerformanceFreshnessMetadataAfterOperationalPoll|DetailDeltaDoesNotReachUnsubscribedClient)' -count=1 -v` passed.
- `rtk go test ./internal/worker -count=1` passed.
- `rtk npm test -- src/types/metrics.test.ts src/hooks/useWebSocket.test.ts` passed.
- `rtk rg -n 'LinkMetricsToDTOs|PerformancePollSendsOnlySelectedDeviceLinkMetricsToSubscribedClient|targeted link_metrics' internal/worker/snapshot_builder.go internal/worker/pipeline_test.go frontend/src/types/metrics.test.ts frontend/src/hooks/useWebSocket.test.ts` returned the expected matches.

## Task Commits

Each task was committed atomically:

1. **Task 1: Add selected-device `link_metrics` to the targeted detail builder without widening the payload** - `20ee120` (test), `4f7c7c5` (feat)
2. **Task 2: Replace the worker regressions that currently lock link-metric omission and prove subscriber-only delivery** - `50c6338` (test)
3. **Task 3: Lock the existing frontend shared-snapshot merge behavior for targeted `link_metrics` deltas** - `fed58cc` (test)

## Files Created/Modified

- `internal/worker/snapshot_builder.go` - builds selected-device targeted detail payloads with a copied `link_metrics[device.id]` slice on the existing snapshot contract.
- `internal/worker/snapshot_builder_test.go` - replaces the old omission regression with selected-device inclusion coverage for targeted detail payloads.
- `internal/worker/pipeline_test.go` - proves subscribed-only targeted link-metric delivery and preservation through later operational detail sends.
- `frontend/src/types/metrics.test.ts` - locks map-merge behavior for one-device `link_metrics` snapshot deltas.
- `frontend/src/hooks/useWebSocket.test.ts` - proves sparse websocket `snapshot_delta` messages merge targeted `link_metrics` into the shared snapshot state.

## Decisions Made

- Targeted detail widened only inside the existing `SnapshotPayload.LinkMetrics` map, keeping the Phase 46 contract on `snapshot_delta` and avoiding any new websocket family or cache path.
- Runtime delivery stayed on `publishSubscribedDetailDelta()` and `Hub.DetailSubscribers(...)`; no overview broadcast behavior changed.
- Frontend production code stayed unchanged because the existing `mergeSnapshotDelta()` path already handled sparse device-key merges correctly once the backend payload included `link_metrics`.

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered

- The mixed-tier worker regression initially failed because its fixture did not seed a counter baseline, so the first performance poll had no throughput slice to preserve. Updating the test setup fixed the coverage without requiring runtime changes.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

- `WS-02` is now closed on the existing targeted detail seam: selected device interface panels can receive per-poll throughput updates immediately without waiting for the next overview broadcast.
- The runtime and frontend contracts remain narrow and backward-compatible, so no follow-up transport or state-management migration is required for this gap closure.

## Self-Check: PASSED

- Found `.planning/phases/46-detail-delta-gap-closure/46-01-SUMMARY.md` on disk.
- Verified task commits `20ee120`, `4f7c7c5`, `50c6338`, and `fed58cc` exist in `git log --oneline --all`.
