---
phase: 25-frontend-credential-profile-manager-winbox-actions
verified: 2026-04-08T10:54:00Z
status: passed
score: 5/5 must-haves verified
re_verification: false
---

# Phase 25: Frontend Credential Profile Manager + WinBox Actions Verification Report

**Phase Goal:** Users can manage credential profiles and launch WinBox from the topology map and device table
**Verified:** 2026-04-08T10:54:00Z
**Status:** passed
**Re-verification:** No — initial verification

## Goal Achievement

### Observable Truths (Roadmap Success Criteria)

| # | Truth | Status | Evidence |
|---|-------|--------|----------|
| 1 | User can open WinBox pre-authenticated from the canvas device context menu | VERIFIED | `Canvas.tsx` line 351: `{ id: 'winbox', label: 'Open in WinBox', icon: 'open_in_new', ... onClick: () => { void handleLaunchWinBox(d.id); } }` — `handleLaunchWinBox` fetches credentials and POSTs to `localhost:1337/launch` |
| 2 | User can open WinBox pre-authenticated from the Devices table row action | VERIFIED | `DeviceRow.tsx` line 101: `<IconAction icon="open_in_new" title={winboxTitle} onClick={onWinBox} disabled={winboxDisabled} />` — wired through `DeviceTable` → `Dashboard.handleWinBox` which calls `fetchWinBoxCredentials` and POSTs to `localhost:1337/launch` |
| 3 | WinBox action is visually disabled with an explanatory tooltip when no WinBox profile is designated | VERIFIED | Canvas: `winboxTitle = !hasWinboxProfile ? 'No WinBox profile designated' : ...`; DeviceRow: `title={winboxTitle}` + `disabled={winboxDisabled}`; IconAction renders `disabled` HTML attribute and `cursor-not-allowed opacity-40` styling |
| 4 | Frontend detects whether the bridge is running via health check and reflects bridge status | VERIFIED | `useBridgeHealth.ts` polls `http://localhost:1337/health` on mount and every 30 seconds, returns `{ bridgeRunning: boolean }`; used in both `Canvas.tsx` and `Dashboard.tsx`; tooltip reads "WinBox bridge not running — download from Settings" when `bridgeRunning=false` |
| 5 | User can view, create, edit, delete, and assign credential profiles to a device from within Theia UI | VERIFIED | `CredentialProfileManager.tsx` (exported from `SettingsPanel`) handles global CRUD; `DeviceConfigPanel.tsx` Credentials section handles per-device assignment with add/remove/WinBox-designation |

**Score:** 5/5 truths verified

### Required Artifacts

| Artifact | Expected | Status | Details |
|----------|----------|--------|---------|
| `frontend/src/types/api.ts` | `CredentialProfile`, `DeviceCredentialProfile`, `WinBoxCredentials` types | VERIFIED | All 3 interfaces defined at lines 300, 313, 321; parsers exist at lines 435, 460, 478, 499 |
| `frontend/src/api/client.ts` | 10 API functions for credential profiles + assignments + WinBox | VERIFIED | `fetchCredentialProfiles`, `createCredentialProfile`, `updateCredentialProfile`, `deleteCredentialProfile`, `fetchDeviceCredentialProfiles`, `assignCredentialProfile`, `unassignCredentialProfile`, `setWinBoxProfile`, `clearWinBoxProfile`, `fetchWinBoxCredentials` all present |
| `frontend/src/components/CredentialProfileManager.tsx` | Renamed manager with role field | VERIFIED | `export function CredentialProfileManager` at line 289; `emptyForm()` returns `role: 'Admin'`; Role input with `placeholder="e.g. Admin"` |
| `frontend/src/components/CredentialProfileManager.test.tsx` | Tests for CredentialProfileManager | VERIFIED | File exists; tests pass (41 test files, 440 tests all passing) |
| `frontend/src/components/SettingsPanel.tsx` | Imports and renders CredentialProfileManager | VERIFIED | `import { CredentialProfileManager } from './CredentialProfileManager'` at line 6; `<CredentialProfileManager />` at line 382 |
| `frontend/src/components/DeviceConfigPanel.tsx` | Credentials section replacing ssh_profile_id | VERIFIED | Credentials section at line 535; no `ssh_profile_id` or `sshProfileId` remaining |
| `frontend/src/hooks/useBridgeHealth.ts` | Bridge health polling hook | VERIFIED | 30-line implementation; polls on mount + 30s interval; silent failure; cleanup on unmount |
| `frontend/src/hooks/useBridgeHealth.test.ts` | Tests for bridge health hook | VERIFIED | 5 test cases covering ok, failure, not-ok, polling interval, cleanup |
| `frontend/src/components/ContextMenu.tsx` | `title?: string` in ContextMenuItem interface | VERIFIED | Line 11: `title?: string`; line 83: `title={item.title}` on button element |
| `frontend/src/components/Canvas.tsx` | WinBox context menu item with 3-state logic | VERIFIED | `useBridgeHealth`, `fetchWinBoxCredentials`, `Open in WinBox`, `localhost:1337/launch` all present; `winbox` not in `virtualItemIds` |
| `frontend/src/components/Dashboard.tsx` | Bridge health + WinBox handler | VERIFIED | `useBridgeHealth`, `handleWinBox`, `fetchWinBoxCredentials`, `localhost:1337/launch` all present |
| `frontend/src/components/dashboard/DeviceTable.tsx` | onWinBox/winboxDisabled/winboxTitle props | VERIFIED | All 3 props in interface + destructured + passed to DeviceRow |
| `frontend/src/components/dashboard/DeviceRow.tsx` | WinBox IconAction with disabled support | VERIFIED | `open_in_new` icon; `onWinBox`, `winboxDisabled`, `winboxTitle` props; `disabled` in IconAction; virtual device gate at line 99 |
| `frontend/src/components/SSHProfileManager.tsx` | Must NOT exist | VERIFIED | File deleted — `ls` confirms absence |
| `frontend/src/components/SSHProfileManager.test.tsx` | Must NOT exist | VERIFIED | File deleted — `ls` confirms absence |

### Key Link Verification

| From | To | Via | Status | Details |
|------|----|-----|--------|---------|
| `client.ts` | `/api/v1/credential-profiles` | `requestJSON` / `requestJSONWithBody` | WIRED | `fetchCredentialProfiles`, `createCredentialProfile`, `updateCredentialProfile`, `deleteCredentialProfile` all use this base path |
| `CredentialProfileManager.tsx` | `client.ts` | `import fetchCredentialProfiles` | WIRED | Imports `fetchCredentialProfiles`, `createCredentialProfile`, `updateCredentialProfile`, `deleteCredentialProfile` |
| `SettingsPanel.tsx` | `CredentialProfileManager.tsx` | `import { CredentialProfileManager }` | WIRED | Direct import and render at line 382 |
| `DeviceConfigPanel.tsx` | `client.ts` | `fetchDeviceCredentialProfiles` et al | WIRED | 5 assignment/winbox functions imported and called |
| `useBridgeHealth.ts` | `http://localhost:1337/health` | `fetch` with 30s interval | WIRED | Direct `fetch(BRIDGE_HEALTH_URL)` in `check()`, interval set at `POLL_INTERVAL_MS = 30_000` |
| `Canvas.tsx` | `client.ts` | `fetchWinBoxCredentials` + `fetchDeviceCredentialProfiles` | WIRED | Both imported and called in `handleLaunchWinBox` / deviceMenu useEffect |
| `Dashboard.tsx` | `useBridgeHealth.ts` | `import useBridgeHealth` | WIRED | Line 5 import; line 35 `const { bridgeRunning } = useBridgeHealth()` |
| `DeviceRow.tsx` | Disabled WinBox IconAction | `winboxDisabled` + `winboxTitle` props | WIRED | Props destructured and passed to `<IconAction disabled={winboxDisabled} title={winboxTitle} />` |

### Data-Flow Trace (Level 4)

| Artifact | Data Variable | Source | Produces Real Data | Status |
|----------|---------------|--------|--------------------|--------|
| `CredentialProfileManager.tsx` | `profiles` state | `fetchCredentialProfiles()` → `/api/v1/credential-profiles` → backend DB query | Yes — backend returns real DB data | FLOWING |
| `DeviceConfigPanel.tsx` | `assignments` state | `fetchDeviceCredentialProfiles(device.id)` → `/api/v1/devices/{id}/credential-profiles` | Yes — real device-profile join | FLOWING |
| `useBridgeHealth.ts` | `bridgeRunning` | `fetch('http://localhost:1337/health').resp.ok` | Yes — live network check | FLOWING |
| `Canvas.tsx` WinBox launch | credentials passed to `localhost:1337/launch` | `fetchWinBoxCredentials(deviceId)` → backend → resolved credentials | Yes — real credential fetch | FLOWING |
| `Dashboard.tsx` | `deviceWinboxMap` | `fetchDeviceCredentialProfiles` per non-virtual device | Yes — real assignment data | FLOWING |

### Behavioral Spot-Checks

| Behavior | Command | Result | Status |
|----------|---------|--------|--------|
| TypeScript compiles clean | `cd frontend && npx tsc --noEmit; echo "EXIT:$?"` | EXIT:0 | PASS |
| All 440 tests pass | `cd frontend && npm test -- --run` | 41 test files passed, 440 tests passed | PASS |
| No `SSHProfile` or `SSHProfileManager` references in src | `grep -rn "SSHProfile\|SSHProfileManager" frontend/src/` | Only `testSSHProfile` orphan in `client.ts` (not imported anywhere) and a comment in audit test file | PASS (orphan is not wired) |
| `CredentialProfile` type has `role` field | `grep "role" frontend/src/types/api.ts` | `role: string` at line 307 | PASS |
| WinBox context menu item present in Canvas | `grep "Open in WinBox" frontend/src/components/Canvas.tsx` | Line 351 | PASS |
| Bridge health polls at 30s | `grep "30_000" frontend/src/hooks/useBridgeHealth.ts` | `const POLL_INTERVAL_MS = 30_000;` at line 4 | PASS |
| No console.error/log in useBridgeHealth | `grep "console" frontend/src/hooks/useBridgeHealth.ts` | No matches | PASS |

### Requirements Coverage

| Requirement | Plans | Description | Status | Evidence |
|-------------|-------|-------------|--------|---------|
| WINBOX-01 | 25-01, 25-03 | Canvas context menu WinBox launch | SATISFIED | Canvas.tsx has "Open in WinBox" item with fetchWinBoxCredentials → localhost:1337/launch |
| WINBOX-02 | 25-01, 25-03 | Device table WinBox launch | SATISFIED | DeviceRow "open_in_new" action → Dashboard.handleWinBox → localhost:1337/launch |
| WINBOX-03 | 25-02, 25-03 | WinBox 3-state disabled logic | SATISFIED | Tooltip: "No WinBox profile designated" / "WinBox bridge not running — download from Settings" / enabled |
| BRIDGE-05 | 25-03 | Frontend bridge health detection | SATISFIED | useBridgeHealth polls localhost:1337/health, bridgeRunning state reflected in both Canvas and DeviceTable |

### Anti-Patterns Found

| File | Line | Pattern | Severity | Impact |
|------|------|---------|----------|--------|
| `frontend/src/api/client.ts` | 417-428 | `testSSHProfile` function references `/api/v1/ssh-profiles/` endpoint | INFO | Orphaned export — not imported anywhere in the codebase. The backend no longer exposes this endpoint. The function is dead code but does not affect build or tests. |
| `frontend/src/components/__tests__/form-input-audit.test.ts` | 6 | Comment references `SSHProfileManager` | INFO | Comment text only — test code itself references `CredentialProfileManager.tsx` at line 20. No runtime impact. |

### Human Verification Required

None — all critical behaviors are code-verifiable. The 3-state disabled logic, virtual device exclusion, and WinBox credential resolution path are all confirmed by code inspection and passing tests.

### Gaps Summary

No gaps. All 5 roadmap success criteria are satisfied:

1. Canvas WinBox launch: fully wired from context menu through credential fetch to bridge POST
2. Device table WinBox launch: fully wired through DeviceRow → DeviceTable → Dashboard handler
3. Disabled states with tooltips: 3-state logic implemented correctly in both Canvas and DeviceRow
4. Bridge health detection: useBridgeHealth polls on mount + 30s interval, reflected in disabled states
5. Credential profile CRUD + assignment: CredentialProfileManager handles global CRUD; DeviceConfigPanel handles per-device assignment

TypeScript compiles clean (exit 0). All 440 tests pass across 41 test files.

One minor orphan exists: `testSSHProfile` function in `client.ts` still references the old `/api/v1/ssh-profiles/` endpoint but is not imported or used anywhere and has no runtime impact.

---

_Verified: 2026-04-08T10:54:00Z_
_Verifier: Claude (gsd-verifier)_
