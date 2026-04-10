---
phase: 24-backend-api-profiles-assignments-winbox-credentials
plan: "02"
subsystem: backend
tags: [api, handler, winbox, credential-profiles, tdd]
dependency_graph:
  requires: [24-01]
  provides: [device-credential-profile-assignment-api, winbox-credentials-endpoint, bridgeBinariesDir-router-param]
  affects:
    - internal/api/device_credential_profile_handler.go
    - internal/api/device_credential_profile_handler_test.go
    - internal/service/backup_service.go
    - internal/api/router.go
    - cmd/theia/main.go
tech_stack:
  added: []
  patterns: [TDD red-green, handler-delegates-to-repo, service-layer-decryption]
key_files:
  created:
    - internal/api/device_credential_profile_handler.go
    - internal/api/device_credential_profile_handler_test.go
  modified:
    - internal/service/backup_service.go
    - internal/api/router.go
    - cmd/theia/main.go
decisions:
  - GetWinboxCredentials added to BackupService to keep decryption in service layer (T-24-05)
  - HandleGetWinboxCredentials returns flat JSON (no data envelope) per D-10 spec
  - bridgeBinariesDir added to NewRouter signature now (used by Plan 03 bridge handler)
  - bridgeBinariesDir stored as _ in router until Plan 03 bridge handler is wired
metrics:
  duration_minutes: 2
  completed_date: "2026-04-07"
  tasks_completed: 2
  files_changed: 5
requirements:
  - CRED-03
  - CRED-05
---

# Phase 24 Plan 02: DeviceCredentialProfileHandler + Router Wiring Summary

**One-liner:** DeviceCredentialProfileHandler with 6 endpoint methods (list/assign/unassign/set-winbox/clear-winbox/get-winbox-credentials), BackupService.GetWinboxCredentials for service-layer decryption, and all routes wired into the existing /api/v1/devices/ block with bridgeBinariesDir added to NewRouter.

## Tasks Completed

| # | Name | Commit | Key Files |
|---|------|--------|-----------|
| RED | Add failing TDD tests | 4821506 | device_credential_profile_handler_test.go (new) |
| 1 | Handler + BackupService.GetWinboxCredentials | a42f841 | device_credential_profile_handler.go (new), backup_service.go |
| 2 | Router wiring + main.go | 5143b46 | router.go, main.go |

## What Was Built

### DeviceCredentialProfileHandler (`internal/api/device_credential_profile_handler.go`)

New handler with 6 endpoint methods:

- `HandleListAssignments` — GET `/api/v1/devices/{id}/credential-profiles`: returns assigned profiles with `is_winbox` field; response omits `encrypted_secret` (T-24-06)
- `HandleAssign` — POST `/api/v1/devices/{id}/credential-profiles`: assigns a profile; 409 on UNIQUE conflict, 404 on FOREIGN KEY violation, 400 on missing/invalid `profile_id`
- `HandleUnassign` — DELETE `/api/v1/devices/{id}/credential-profiles/{profileId}`: unassigns; 404 if not assigned; both UUIDs parsed via `uuid.Parse` (T-24-08)
- `HandleSetWinbox` — PUT `/api/v1/devices/{id}/winbox-profile`: designates WinBox profile; 404 if profile not assigned
- `HandleClearWinbox` — DELETE `/api/v1/devices/{id}/winbox-profile`: clears WinBox designation; idempotent 204
- `HandleGetWinboxCredentials` — GET `/api/v1/devices/{id}/winbox-credentials`: returns flat `{ip, username, password}` JSON; 404 when no WinBox profile designated; 422 when profile has no password

### BackupService.GetWinboxCredentials (`internal/service/backup_service.go`)

New method that:
1. Fetches device IP via `deviceRepo.GetByID` (device stays in service layer, handler never holds the IP lookup)
2. Decrypts the credential secret via the internal `decryptSecret` (encryption key never leaves the service layer — T-24-05)
3. Returns an error if the decrypted password is empty string (T-24-10)

### Router Wiring (`internal/api/router.go`)

- `NewRouter` signature updated to accept `bridgeBinariesDir string` parameter (used by Plan 03 bridge handler)
- `deviceCredHandler` constructed inside `NewRouter` after `credentialProfileHandler`
- 6 route dispatches injected after the existing backup routes block, before the final device CRUD `switch` statement
- Route priority: `/backups/latest` > `/backups` > `/credential-profiles` > `/credential-profiles/{id}` > `/winbox-profile` > `/winbox-credentials` > device CRUD

### main.go (`cmd/theia/main.go`)

- `api.NewRouter(...)` call updated to pass `cfg.BridgeBinariesDir` as the new `bridgeBinariesDir` parameter

## Test Coverage

12 tests covering all endpoint behaviors:
- `TestDeviceCredentialProfile_ListAssignments_Empty` — 200 with empty data array
- `TestDeviceCredentialProfile_ListAssignments_AfterAssign` — 200 with is_winbox field
- `TestDeviceCredentialProfile_Assign_HappyPath` — 201 with device_id/profile_id
- `TestDeviceCredentialProfile_Assign_Duplicate_Returns409` — 409 conflict
- `TestDeviceCredentialProfile_Assign_MissingProfileID_Returns400` — 400 validation
- `TestDeviceCredentialProfile_Unassign_HappyPath` — 204 no content
- `TestDeviceCredentialProfile_Unassign_NotAssigned_Returns404` — 404 not found
- `TestDeviceCredentialProfile_SetWinbox_HappyPath` — 200 with is_winbox=true
- `TestDeviceCredentialProfile_SetWinbox_NotAssigned_Returns404` — 404 not found
- `TestDeviceCredentialProfile_ClearWinbox_Idempotent` — 204 always
- `TestDeviceCredentialProfile_GetWinboxCredentials_HappyPath` — 200 with ip/username/password
- `TestDeviceCredentialProfile_GetWinboxCredentials_NoWinboxProfile_Returns404` — 404

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] Fixed device seed SQL: removed non-existent area_ids_json column**
- **Found during:** Task 1 GREEN test run
- **Issue:** Test setup SQL used `area_ids_json` column which does not exist — areas use a `device_areas` junction table (added in migration 000008)
- **Fix:** Removed `area_ids_json` from the INSERT column list
- **Files modified:** `internal/api/device_credential_profile_handler_test.go`
- **Commit:** a42f841 (included in GREEN commit)

## Threat Model Coverage

| Threat ID | Disposition | Status |
|-----------|-------------|--------|
| T-24-05 | mitigate | Decryption in BackupService.GetWinboxCredentials; handler never holds encryption key |
| T-24-06 | mitigate | assignedProfileResponse struct omits encrypted_secret entirely |
| T-24-07 | mitigate | profile_id parsed via uuid.Parse — rejects non-UUID input; FK errors mapped to 404 |
| T-24-08 | mitigate | Both deviceID and profileID in HandleUnassign parsed via uuid.Parse |
| T-24-09 | mitigate | SetWinboxProfile transaction scoped to 2 UPDATE statements (implemented in repo Plan 01) |
| T-24-10 | mitigate | GetWinboxCredentials returns error when decrypted password is empty string |

## Known Stubs

None — all endpoints are fully implemented with real repo and service calls.

## Threat Flags

None — no new network endpoints beyond those defined in the plan's threat model.

## Self-Check: PASSED

Files created/exist:
- FOUND: internal/api/device_credential_profile_handler.go
- FOUND: internal/api/device_credential_profile_handler_test.go

Files modified:
- FOUND: internal/service/backup_service.go (GetWinboxCredentials present)
- FOUND: internal/api/router.go (deviceCredHandler + bridgeBinariesDir present)
- FOUND: cmd/theia/main.go (cfg.BridgeBinariesDir in NewRouter call)

Commits verified:
- FOUND: 4821506 (TDD RED tests)
- FOUND: a42f841 (Task 1 implementation)
- FOUND: 5143b46 (Task 2 router wiring)

Build: exit 0
Tests: all pass (12 new tests + full API suite)
