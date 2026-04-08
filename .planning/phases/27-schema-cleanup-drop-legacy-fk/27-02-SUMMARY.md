---
phase: 27-schema-cleanup-drop-legacy-fk
plan: "02"
subsystem: frontend
tags: [schema-cleanup, credential-profiles, ssh, typescript, react]
dependency_graph:
  requires:
    - phase: 27-01
      provides: devices-table-without-ssh-profile-id
    - phase: 25
      provides: credential-profile-assignment-api
  provides:
    - frontend-device-type-without-ssh-profile-id
    - bulkbackuppanel-credential-profile-eligibility
    - sshcredentialform-uses-assignment-api
  affects: [dashboard, bulk-backup, bulk-edit, add-device]
tech_stack:
  added: []
  patterns: [credential-profile-assignment-api-for-ssh-operations, async-eligibility-prefetch-with-promise-allsettled]
key_files:
  created: []
  modified:
    - frontend/src/types/api.ts
    - frontend/src/api/client.ts
    - frontend/src/components/AddDevicePanel.tsx
    - frontend/src/components/dashboard/SSHCredentialForm.tsx
    - frontend/src/components/Dashboard.tsx
    - frontend/src/components/BulkEditPanel.tsx
    - frontend/src/components/dashboard/BulkBackupPanel.tsx
    - frontend/src/components/dashboard/BulkBackupPanel.test.tsx
key-decisions:
  - "SSHCredentialForm uses assignCredentialProfile/unassignCredentialProfile instead of updateDevice(ssh_profile_id) — T-27-07 mitigation: avoids exposing profile IDs in updateDevice request logs"
  - "Dashboard currentProfileId for SSHCredentialForm uses Option A: fetchDeviceCredentialProfiles on panel open, first non-WinBox profile used as current assignment"
  - "BulkBackupPanel pre-fetches credential profiles for all devices via Promise.allSettled before building initial entries — avoids N sequential API calls in hot path"
  - "AddDevicePanel SSH profile dropdown removed entirely — credential assignment handled post-creation via DeviceConfigPanel"
  - "BulkEditPanel SSH Profile section removed — bulk credential assignment not supported via updateDevice after ssh_profile_id removal"

requirements-completed: [WINBOX-04]

duration: 12min
completed: "2026-04-08"
---

# Phase 27 Plan 02: Frontend ssh_profile_id Removal — Summary

**All frontend ssh_profile_id references removed: Device type cleaned, BulkBackupPanel uses fetchDeviceCredentialProfiles for eligibility, SSHCredentialForm migrated to dedicated assignment API**

## Performance

- **Duration:** ~12 min
- **Started:** 2026-04-08T13:05:00Z
- **Completed:** 2026-04-08T13:10:00Z
- **Tasks:** 2
- **Files modified:** 8

## Accomplishments

- Removed `ssh_profile_id` from `Device` interface, `parseDevicesResponse`, `CreateDevicePayload`, and `updateDevice` payload type — TypeScript compilation succeeds with zero field references
- `SSHCredentialForm` migrated from `updateDevice({ssh_profile_id})` to `assignCredentialProfile`/`unassignCredentialProfile` — T-27-07 mitigated: no profile IDs exposed via device update request logs
- `BulkBackupPanel.handleStart` made async; pre-fetches `fetchDeviceCredentialProfiles` for all devices via `Promise.allSettled` before eligibility check; skip reason updated to "no credential profile assigned"
- `Dashboard` drops `sshOverrides`/`applyOverrides`; fetches live `currentProfileId` via `fetchDeviceCredentialProfiles` when ssh-credentials panel opens (Option A)
- `BulkEditPanel` and `AddDevicePanel` stripped of `sshProfileId` state, SSH profile dropdowns, and related `updateDevice` calls
- All 440 frontend tests pass; TypeScript exits 0

## Task Commits

1. **Task 1: Remove ssh_profile_id from types, API client, and simple components** - `0293014` (feat)
2. **Task 2: Update BulkBackupPanel eligibility check and fix tests** - `e1963bb` (feat)

## Files Created/Modified

- `frontend/src/types/api.ts` — Removed `ssh_profile_id` from `Device` interface and `parseDevicesResponse`
- `frontend/src/api/client.ts` — Removed `ssh_profile_id` from `CreateDevicePayload` and `updateDevice` payload
- `frontend/src/components/AddDevicePanel.tsx` — Removed `sshProfileId` state, `sshProfiles` state, `fetchCredentialProfiles` import, SSH profile dropdown section, and `ssh_profile_id` from `createDevice` call
- `frontend/src/components/dashboard/SSHCredentialForm.tsx` — Replaced `updateDevice({ssh_profile_id})` with `assignCredentialProfile`/`unassignCredentialProfile`
- `frontend/src/components/Dashboard.tsx` — Removed `sshOverrides`/`applyOverrides`/`useCallback`; added `sshPanelProfileId` state populated by `fetchDeviceCredentialProfiles` on panel open
- `frontend/src/components/BulkEditPanel.tsx` — Removed `sshProfileId` state, `sshProfiles` state, `commonSSHProfileId`, `displaySSHProfileId`, SSH Profile dropdown, and `ssh_profile_id` from `updateDevice` call
- `frontend/src/components/dashboard/BulkBackupPanel.tsx` — Made `handleStart` async; pre-fetches credential profiles via `Promise.allSettled`; updated skip reason
- `frontend/src/components/dashboard/BulkBackupPanel.test.tsx` — Added `fetchDeviceCredentialProfiles` to mock; removed `ssh_profile_id` from mockDevice; updated eligibility skip test

## Decisions Made

- **Option A for SSHCredentialForm `currentProfileId`:** Fetch via `fetchDeviceCredentialProfiles` on panel open, select first non-WinBox profile. This is the live source of truth rather than a stale field. Documented with a code comment in Dashboard.tsx.
- **BulkEditPanel SSH Profile removed entirely:** The bulk-edit SSH profile dropdown was wired to `updateDevice({ssh_profile_id})`. Since that field is gone and bulk credential assignment via the join-table API would require per-device calls (not a single bulk update), the section was removed. Credential assignment is per-device via DeviceConfigPanel.
- **AddDevicePanel SSH profile dropdown removed:** The new credential profile system assigns profiles post-creation via DeviceConfigPanel. The inline dropdown in AddDevicePanel was a legacy shortcut that no longer has a backend field to write to.

## Deviations from Plan

None — plan executed exactly as written. All changes matched plan specifications including T-27-07 mitigation for SSHCredentialForm and Option A for currentProfileId source of truth.

## Known Stubs

None — all credential resolution uses live `fetchDeviceCredentialProfiles` API calls.

## Threat Surface Scan

No new network endpoints, auth paths, or file access patterns introduced. Existing `fetchDeviceCredentialProfiles` endpoint already audited in Phase 24. T-27-07 mitigated: SSHCredentialForm now uses dedicated assignment API endpoints instead of `updateDevice`.

## Self-Check

Files modified/exist:
- `frontend/src/types/api.ts` — FOUND
- `frontend/src/api/client.ts` — FOUND
- `frontend/src/components/dashboard/BulkBackupPanel.tsx` — FOUND
- `frontend/src/components/dashboard/BulkBackupPanel.test.tsx` — FOUND

Commits:
- `0293014` (task 1) — FOUND
- `e1963bb` (task 2) — FOUND

TypeScript: PASSED (`npx tsc --noEmit` exits 0)
Tests: PASSED (440/440 tests across 41 files)

## Self-Check: PASSED

---
*Phase: 27-schema-cleanup-drop-legacy-fk*
*Completed: 2026-04-08*
