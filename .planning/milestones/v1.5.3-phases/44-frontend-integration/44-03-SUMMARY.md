---
phase: 44-frontend-integration
plan: 03
subsystem: ui
tags: [react, websocket, canvas, vitest, device-card]
requires:
  - phase: 44-01
    provides: overview snapshot health, freshness, and cadence fields on device_metrics
provides:
  - Health-owned DeviceCard header labels and glow behavior on physical and virtual canvas cards
  - Shared freshness/cadence copy helpers for Fresh/Stale/Dead and Polling every language
  - Canvas snapshot/stale fallback plumbing that preserves overview metadata while blanking only numeric metrics
affects: [44-04, canvas, websocket-overview]
tech-stack:
  added: []
  patterns:
    - Pure formatting helpers for backend-owned card metadata
    - Preserve snapshot DTO metadata through local stale fallback
key-files:
  created:
    - frontend/src/utils/freshness.ts
    - frontend/src/utils/freshness.test.ts
    - frontend/src/components/canvas/nodeBuilder.test.ts
    - frontend/src/components/canvas/useCanvasData.test.ts
  modified:
    - frontend/src/components/DeviceCard.tsx
    - frontend/src/components/DeviceCard.test.tsx
    - frontend/src/components/canvas/nodeBuilder.ts
    - frontend/src/components/canvas/useCanvasData.ts
key-decisions:
  - DeviceCard primary status dot, glow, and explicit label now map directly from metrics.health on both card branches.
  - Freshness and cadence copy live in a pure helper reused by both physical and virtual card rendering paths.
  - Local stale fallback blanks only numeric metric fields and preserves backend-owned health and cadence metadata on node.data.metrics.
patterns-established:
  - Shared overview snapshot metadata must survive node building for both physical and virtual cards.
  - Down-state rendering blanks numeric metric values locally without discarding last_polled_at or expected_poll_interval_seconds.
requirements-completed: [WS-01, WS-03, WS-04]
duration: 13m
completed: 2026-04-13
---

# Phase 44 Plan 03: Canvas Card Metadata Summary

**Health-owned DeviceCard labels with Fresh/Stale/Dead freshness and Polling every cadence metadata now render on both canvas card branches and survive stale snapshot fallback**

## Performance

- **Duration:** 13 min
- **Started:** 2026-04-13T17:10:00Z
- **Completed:** 2026-04-13T17:22:54Z
- **Tasks:** 2
- **Files modified:** 8

## Accomplishments
- Added `frontend/src/utils/freshness.ts` so Phase 44 freshness, relative age, cadence copy, and health labels are generated from one pure helper.
- Updated `DeviceCard` so physical and virtual cards both render backend-owned health labels plus compact freshness/cadence metadata without changing the existing card hierarchy.
- Preserved overview snapshot metadata through `nodeBuilder` and `useCanvasData` so down-device handling and stale fallback no longer erase health, last poll time, or expected interval.

## Task Commits

Each task was committed atomically:

1. **Task 1: Create pure freshness and cadence copy helpers for Phase 44 card metadata** - `eaefbc6` (`test`), `8df2be5` (`feat`)
2. **Task 2: Render health-owned card metadata across physical and virtual branches and preserve it through canvas snapshot updates** - `ad1831e` (`test`), `ab059c4` (`feat`)

## Files Created/Modified
- `frontend/src/utils/freshness.ts` - Canonical freshness tiers, relative age copy, cadence text, and health label mapping.
- `frontend/src/utils/freshness.test.ts` - Locks the 2x/5x thresholds and fixed Phase 44 copy contract.
- `frontend/src/components/DeviceCard.tsx` - Renders health-owned header status plus compact freshness/cadence metadata on physical and virtual cards.
- `frontend/src/components/DeviceCard.test.tsx` - Covers the new physical/virtual health and metadata behavior.
- `frontend/src/components/canvas/nodeBuilder.ts` - Keeps overview metrics attached for down and virtual nodes.
- `frontend/src/components/canvas/nodeBuilder.test.ts` - Verifies canvas node construction no longer nulls overview metadata.
- `frontend/src/components/canvas/useCanvasData.ts` - Preserves overview metadata during snapshot application and stale fallback.
- `frontend/src/components/canvas/useCanvasData.test.ts` - Verifies stale fallback blanks only numeric metrics while keeping health/freshness/cadence fields.

## Decisions Made

- Mapped the primary dot/glow/label from `metrics.health` instead of `alertStatus` or reachability to satisfy the Phase 44 integrity requirement.
- Kept the existing physical detail/model row and virtual title/IP layout intact, adding only one compact metadata row in each branch.
- Preserved `health`, `stale`, `last_polled_at`, and `expected_poll_interval_seconds` through local stale blanking so overview cards stay informative during reconnect gaps.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] Corrected the new down-status freshness expectation at the 5x boundary**
- **Found during:** Task 2 (Render health-owned card metadata across physical and virtual branches and preserve it through canvas snapshot updates)
- **Issue:** The new test expected `Dead` at exactly `5x` the poll interval, but the UI contract defines `Dead` only when age is `> 5x`.
- **Fix:** Updated the assertion to expect `Stale · 2m ago`, matching the shared freshness helper and UI-SPEC threshold contract.
- **Files modified:** `frontend/src/components/DeviceCard.test.tsx`
- **Verification:** `cd /home/azmin/projects/theia/frontend && rtk npm test -- src/utils/freshness.test.ts src/components/DeviceCard.test.tsx src/components/canvas/nodeBuilder.test.ts src/components/canvas/useCanvasData.test.ts`
- **Committed in:** `ab059c4`

---

**Total deviations:** 1 auto-fixed (1 bug)
**Impact on plan:** The correction aligned the new test with the specified freshness thresholds. No scope creep.

## Issues Encountered

None.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

- Phase 44-04 can build the override-first polling UX on top of the existing shared snapshot/device contract without introducing another state path for card metadata.
- Canvas cards now read backend-owned health/freshness/cadence directly from overview snapshot data, so the remaining frontend polling work is isolated to `DeviceConfigPanel`.

## Deviations from Threat Model

None.

## Self-Check

PASSED

- Found `.planning/phases/44-frontend-integration/44-03-SUMMARY.md`
- Found task commits `eaefbc6`, `8df2be5`, `ad1831e`, and `ab059c4`

---
*Phase: 44-frontend-integration*
*Completed: 2026-04-13*
