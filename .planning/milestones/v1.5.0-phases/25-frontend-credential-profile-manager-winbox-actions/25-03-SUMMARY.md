---
phase: "25"
plan: "03"
subsystem: frontend
tags: [winbox, bridge-health, context-menu, device-table, react-hooks]
dependency_graph:
  requires: ["25-01"]
  provides: ["WINBOX-01", "WINBOX-02", "WINBOX-03", "BRIDGE-05"]
  affects: [Canvas, Dashboard, DeviceTable, DeviceRow, ContextMenu]
tech_stack:
  added: [useBridgeHealth hook]
  patterns: [polling hook with cleanup, per-device lazy fetch, 3-state disabled UI]
key_files:
  created:
    - frontend/src/hooks/useBridgeHealth.ts
    - frontend/src/hooks/useBridgeHealth.test.ts
  modified:
    - frontend/src/components/ContextMenu.tsx
    - frontend/src/components/Canvas.tsx
    - frontend/src/components/Dashboard.tsx
    - frontend/src/components/dashboard/DeviceTable.tsx
    - frontend/src/components/dashboard/DeviceTable.test.tsx
    - frontend/src/components/dashboard/DeviceRow.tsx
    - frontend/src/components/dashboard/DeviceRow.test.tsx
decisions:
  - "waitFor + fake timers causes timeout in Vitest — replaced with act + advanceTimersByTimeAsync"
  - "DeviceWinboxMap uses lazy fetch per-device on first render/menu-open rather than upfront batch"
metrics:
  duration: "~30 minutes"
  completed_date: "2026-04-08"
  tasks_completed: 1
  files_changed: 9
---

# Phase 25 Plan 03: WinBox Launch Actions Summary

WinBox launch UX delivered — useBridgeHealth hook polls localhost:1337/health every 30s; Canvas context menu and DeviceTable row show Open in WinBox with 3-state disabled logic (no profile, bridge not running, or enabled).

## What Was Implemented

### Part A: useBridgeHealth hook
- Created `frontend/src/hooks/useBridgeHealth.ts` — polls `http://localhost:1337/health` on mount and every 30 seconds
- Silent failure: catch block sets `bridgeRunning=false` only, no console logging (T-25-11 mitigated)
- Cleanup on unmount via `cancelled` flag + `clearInterval` (T-25-10 mitigated)
- Returns `{ bridgeRunning: boolean }`

### Part B: ContextMenu title support
- Added `title?: string` to `ContextMenuItem` interface
- Added `title={item.title}` on the button element — enables native browser tooltips on disabled items

### Part C: Canvas context menu WinBox item
- Added `useBridgeHealth` + `fetchDeviceCredentialProfiles` + `fetchWinBoxCredentials` imports
- Added `deviceWinboxState: Record<string, boolean>` state for lazy per-device profile fetch
- `useEffect` on `deviceMenu` change fetches WinBox profile status once per device
- `handleLaunchWinBox` fetches credentials and POSTs to `http://localhost:1337/launch`
- WinBox item inserted between grafana and interface-stats in `allItems`
- `virtualItemIds` excludes `winbox` automatically — virtual devices never see the item

### Part D: DeviceRow WinBox icon action
- `IconAction` updated to accept `disabled?: boolean` — visually grayed out + `cursor-not-allowed`
- Added `onWinBox`, `winboxDisabled`, `winboxTitle` props to `DeviceRowProps`
- WinBox `open_in_new` icon button rendered first in the actions cell for non-virtual devices

### Part E: DeviceTable props threading
- Added `onWinBox`, `winboxDisabled`, `winboxTitle` to `DeviceTableProps` interface
- Threaded down to each `DeviceRow` render

### Part F: Dashboard integration
- Added `useBridgeHealth`, `fetchDeviceCredentialProfiles`, `fetchWinBoxCredentials` imports
- Added `deviceWinboxMap` state + `useEffect` that lazily fetches per-device WinBox status
- `handleWinBox` fetches credentials and POSTs to `http://localhost:1337/launch`
- `isWinboxDisabled` and `getWinboxTitle` compute per-device disabled state with correct tooltip copy
- Passed `onWinBox`, `winboxDisabled`, `winboxTitle` to `DeviceTable`

### Part G: Tests
- `useBridgeHealth.test.ts` — 5 tests covering success, failure, not-ok, interval polling, cleanup
- `DeviceRow.test.tsx` — updated button count (4→5), added WinBox icon, disabled state, tooltip tests
- `DeviceTable.test.tsx` — refactored to `renderTable()` helper with WinBox props, added WinBox thread test

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] waitFor + fake timers timeout in useBridgeHealth tests**
- **Found during:** Task 1 (Part A test execution)
- **Issue:** Plan specified `waitFor(() => expect(...))` pattern for bridge health tests, but `vi.useFakeTimers()` intercepts `setTimeout` used internally by `waitFor`, causing 5000ms timeout on all 3 state-assertion tests
- **Fix:** Replaced `waitFor` with `act(async () => { await vi.advanceTimersByTimeAsync(0); })` to flush the async promise microtask queue before asserting state
- **Files modified:** `frontend/src/hooks/useBridgeHealth.test.ts`
- **Commit:** e0de800

## Test Results

```
Test Files  41 passed (41)
Tests       440 passed (440)
Duration    ~12s
```

All pre-existing tests continue to pass. 5 new useBridgeHealth tests pass, 3 new DeviceRow tests pass, 1 new DeviceTable test passes.

## Known Stubs

None — all WinBox launch logic is fully wired end-to-end.

## Threat Flags

No new security surface beyond what is declared in the plan's threat model (T-25-07 through T-25-11). Credentials are held in local function scope only, never stored in React state, never logged.

## Self-Check: PASSED

All key files exist. All acceptance criteria patterns verified present. Commit e0de800 confirmed. TypeScript zero errors. 440 tests pass.
