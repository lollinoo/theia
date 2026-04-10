---
phase: 25-frontend-credential-profile-manager-winbox-actions
plan: "01"
subsystem: frontend
tags: [typescript, react, api-client, credential-profiles, winbox]
dependency_graph:
  requires: []
  provides: [CredentialProfile type, DeviceCredentialProfile type, WinBoxCredentials type, fetchCredentialProfiles, fetchDeviceCredentialProfiles, assignCredentialProfile, unassignCredentialProfile, setWinBoxProfile, clearWinBoxProfile, fetchWinBoxCredentials, CredentialProfileManager component]
  affects: [frontend/src/types/api.ts, frontend/src/api/client.ts, frontend/src/components/CredentialProfileManager.tsx, frontend/src/components/SettingsPanel.tsx]
tech_stack:
  added: []
  patterns: [type-safe API response parsing, named exports, co-located test files]
key_files:
  created:
    - frontend/src/components/CredentialProfileManager.tsx
    - frontend/src/components/CredentialProfileManager.test.tsx
  modified:
    - frontend/src/types/api.ts
    - frontend/src/api/client.ts
    - frontend/src/components/SettingsPanel.tsx
    - frontend/src/components/SettingsPanel.test.tsx
    - frontend/src/components/DeviceConfigPanel.tsx
    - frontend/src/components/DeviceConfigPanel.test.tsx
    - frontend/src/components/AddDevicePanel.tsx
    - frontend/src/components/AddDevicePanel.test.tsx
    - frontend/src/components/BulkEditPanel.tsx
    - frontend/src/components/BulkEditPanel.test.tsx
    - frontend/src/components/dashboard/SSHCredentialForm.tsx
    - frontend/src/components/__tests__/form-input-audit.test.ts
  deleted:
    - frontend/src/components/SSHProfileManager.tsx
    - frontend/src/components/SSHProfileManager.test.tsx
decisions:
  - SSHProfile interface renamed to CredentialProfile with role field added — no adapter layer needed since backend already serves /credential-profiles
  - SSHCredentialForm.tsx (dashboard) updated to use createCredentialProfile with role hardcoded to 'Admin' — this component will be superseded by Plan 02 DeviceConfigPanel credentials section
  - Local variable names (sshProfiles, sshProfileId) retained in BulkEditPanel/DeviceConfigPanel/AddDevicePanel — these are internal state names that don't affect type safety, renaming deferred to Plan 02 when those files get deeper rework
  - testSSHProfile function in client.ts retained as-is — it hits /api/v1/ssh-profiles/{id}/test which is unrelated to credential-profiles endpoint
metrics:
  duration: 15 min
  completed_date: "2026-04-08"
  tasks_completed: 1
  files_changed: 14
---

# Phase 25 Plan 01: Rename SSHProfile to CredentialProfile — Frontend Types, API Client, Manager Component

Renamed SSHProfile to CredentialProfile across types, API client, and the global manager component. Added role field to type system and manager form. Added 6 new API client functions for device credential assignments and WinBox credentials.

## What Was Implemented

### Part A: Types (frontend/src/types/api.ts)
- Renamed `SSHProfile` interface to `CredentialProfile`, added `role: string` field
- Renamed `parseSSHProfilesResponse` to `parseCredentialProfilesResponse`, added `role` to parsed object
- Added `parseCredentialProfileResponse` for single-item `{ data: {...} }` responses
- Added `DeviceCredentialProfile` interface: `profile_id`, `name`, `role`, `is_winbox` — backend serializes ID as `"id"` so parser maps `item.id` to `profile_id`
- Added `parseDeviceCredentialProfilesResponse` parser
- Added `WinBoxCredentials` interface: `ip`, `username`, `password`
- Added `parseWinBoxCredentialsResponse` parser (flat JSON, no data envelope)

### Part B: API Client (frontend/src/api/client.ts)
- Updated imports: removed `SSHProfile`/`parseSSHProfilesResponse`, added `CredentialProfile`, `DeviceCredentialProfile`, `WinBoxCredentials`, and all new parsers
- Renamed `SSHProfilePayload` to `CredentialProfilePayload`, added `role: string` field
- Renamed 4 functions and updated endpoints to `/credential-profiles`:
  - `fetchCredentialProfiles` → GET /api/v1/credential-profiles
  - `createCredentialProfile` → POST /api/v1/credential-profiles
  - `updateCredentialProfile` → PUT /api/v1/credential-profiles/{id}
  - `deleteCredentialProfile` → DELETE /api/v1/credential-profiles/{id}
- Added 6 new functions:
  - `fetchDeviceCredentialProfiles` → GET /api/v1/devices/{id}/credential-profiles
  - `assignCredentialProfile` → POST /api/v1/devices/{id}/credential-profiles
  - `unassignCredentialProfile` → DELETE /api/v1/devices/{id}/credential-profiles/{profileId}
  - `setWinBoxProfile` → PUT /api/v1/devices/{id}/winbox-profile
  - `clearWinBoxProfile` → DELETE /api/v1/devices/{id}/winbox-profile
  - `fetchWinBoxCredentials` → GET /api/v1/devices/{id}/winbox-credentials

### Part C: CredentialProfileManager Component
- Created `CredentialProfileManager.tsx` (renamed from `SSHProfileManager.tsx`)
- Added `role` field to `FormState` type
- `emptyForm()` returns `role: 'Admin'` (pre-filled default per D-03)
- `profileToForm(p)` maps `p.role` to form
- Added "Role" input field after Description field, with `placeholder="e.g. Admin"`
- Profile card list shows `Role: {p.role}` metadata text
- Deleted `SSHProfileManager.tsx`

### Part D: SettingsPanel
- Updated import from `SSHProfileManager` to `CredentialProfileManager`
- Updated JSX `<SSHProfileManager />` to `<CredentialProfileManager />`
- The `CredentialProfileManager` list header reads "Credential Profiles"
- Updated `SettingsPanel.test.tsx` mock from `./SSHProfileManager` to `./CredentialProfileManager`

### Part E: Other Consumers
- `DeviceConfigPanel.tsx`: updated type import and `fetchSSHProfiles` → `fetchCredentialProfiles`
- `AddDevicePanel.tsx`: same renames
- `BulkEditPanel.tsx`: same renames
- `SSHCredentialForm.tsx` (dashboard): updated imports + calls, added `role: 'Admin'` to `createCredentialProfile` payload
- `form-input-audit.test.ts`: updated file path from `SSHProfileManager.tsx` to `CredentialProfileManager.tsx`
- All test files: updated mock function names from `fetchSSHProfiles` to `fetchCredentialProfiles`

## Test Results

```
Test Files  40 passed (40)
Tests       428 passed (428)
Duration    11.86s
```

TypeScript: `npx tsc --noEmit` exits 0 (zero errors).

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 2 - Missing required field] Added `role: 'Admin'` to SSHCredentialForm.tsx createCredentialProfile call**
- **Found during:** Part E update of SSHCredentialForm.tsx
- **Issue:** `createCredentialProfile` now requires `role: string` in the payload, but `SSHCredentialForm.tsx` was not updated to include it — would have caused a TypeScript error
- **Fix:** Added `role: 'Admin'` as default to the `createCredentialProfile` payload in `SSHCredentialForm.tsx`
- **Files modified:** `frontend/src/components/dashboard/SSHCredentialForm.tsx`
- **Commit:** 564e9c6

**2. [Rule 3 - Blocking issue] Additional consumer files not listed in plan Part E**
- **Found during:** Initial grep scan for SSHProfile references
- **Issue:** Plan Part E listed DeviceConfigPanel and Dashboard.tsx but the full set of consumers also included AddDevicePanel.tsx, BulkEditPanel.tsx, SSHCredentialForm.tsx, and test files — all would have caused build failures
- **Fix:** Updated all consumers found via grep
- **Files modified:** `AddDevicePanel.tsx`, `AddDevicePanel.test.tsx`, `BulkEditPanel.tsx`, `BulkEditPanel.test.tsx`, `SSHCredentialForm.tsx`, `form-input-audit.test.ts`
- **Commit:** 564e9c6

## Known Stubs

None — all API functions are wired to real endpoints. No placeholder data.

## Threat Flags

None — no new network endpoints or auth paths introduced. All API calls use existing `requestJSON`/`requestJSONWithBody` helpers with `encodeURIComponent` on URL path segments (T-25-02 mitigation applied).

## Self-Check: PASSED

Files exist:
- frontend/src/components/CredentialProfileManager.tsx: FOUND
- frontend/src/components/CredentialProfileManager.test.tsx: FOUND
- frontend/src/types/api.ts: FOUND (contains CredentialProfile, DeviceCredentialProfile, WinBoxCredentials)
- frontend/src/api/client.ts: FOUND (contains fetchCredentialProfiles, fetchWinBoxCredentials)

Files deleted:
- frontend/src/components/SSHProfileManager.tsx: CONFIRMED DELETED
- frontend/src/components/SSHProfileManager.test.tsx: CONFIRMED DELETED

Commit: 564e9c6 FOUND in git log
