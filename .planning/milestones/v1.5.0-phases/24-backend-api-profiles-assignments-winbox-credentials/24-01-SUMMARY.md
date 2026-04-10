---
phase: 24-backend-api-profiles-assignments-winbox-credentials
plan: "01"
subsystem: backend
tags: [migration, repository, api, config, winbox, credential-profiles]
dependency_graph:
  requires: [23-02]
  provides: [device_credential_profiles.is_winbox, assignment-repo-methods, credential-profiles-api-paths, BridgeBinariesDir-config]
  affects: [internal/repository/sqlite/credential_profile_repo.go, internal/api/credential_profile_handler.go, internal/api/router.go, internal/config/config.go]
tech_stack:
  added: []
  patterns: [TDD red-green, transactional SQLite UPDATE, parameterized queries]
key_files:
  created:
    - internal/repository/sqlite/migrations/000013_device_credential_profiles_winbox.up.sql
    - internal/repository/sqlite/migrations/000013_device_credential_profiles_winbox.down.sql
    - internal/repository/sqlite/credential_profile_assignment_test.go
  modified:
    - internal/repository/sqlite/credential_profile_repo.go
    - internal/api/credential_profile_handler.go
    - internal/api/credential_profile_handler_test.go
    - internal/api/oversized_body_test.go
    - internal/api/router.go
    - internal/config/config.go
decisions:
  - IsInUse now checks device_credential_profiles join table (D-14) — legacy devices.ssh_profile_id check removed
  - SetWinboxProfile uses a transaction to atomically clear-then-set is_winbox (D-04, D-08)
  - ClearWinboxProfile is idempotent — no error when no winbox flag is currently set (D-09)
  - BridgeBinariesDir added as server-side only config; not exposed via API (T-24-04 accepted)
  - Bridge download bypass added in router outer handler using strings.HasPrefix pattern matching existing bypasses
metrics:
  duration_minutes: 4
  completed_date: "2026-04-07"
  tasks_completed: 2
  files_changed: 8
requirements:
  - CRED-03
  - CRED-05
---

# Phase 24 Plan 01: Migration 000013, Assignment/WinBox Repo Methods, API Path Rename Summary

**One-liner:** Migration 000013 adds is_winbox column; 6 new CredentialProfileRepo methods for device-profile assignment and WinBox designation; all API paths renamed from ssh-profiles to credential-profiles; BridgeBinariesDir config field added.

## Tasks Completed

| # | Name | Commit | Key Files |
|---|------|--------|-----------|
| RED | Add failing TDD tests | 89dce7e | credential_profile_assignment_test.go (new) |
| 1 | Migration 000013 + repo methods + IsInUse fix | c1247c6 | migrations/000013_*.sql (new), credential_profile_repo.go |
| 2 | Rename API paths + config field + router updates + test fixes | 6271021 | credential_profile_handler.go, router.go, config.go, test files |

## What Was Built

### Migration 000013
- `internal/repository/sqlite/migrations/000013_device_credential_profiles_winbox.up.sql`: Adds `is_winbox BOOLEAN NOT NULL DEFAULT 0` to `device_credential_profiles` table
- `internal/repository/sqlite/migrations/000013_device_credential_profiles_winbox.down.sql`: Reverses via backup table pattern (SQLite has no DROP COLUMN)

### New Repo Types and Methods
Added to `internal/repository/sqlite/credential_profile_repo.go`:
- `DeviceCredentialProfileRow` struct — join-table row for `ListAssignedProfiles`
- `WinboxAssignmentRow` struct — minimal data for WinBox launch
- `ListAssignedProfiles(deviceID)` — returns all profiles assigned to a device, ordered by name
- `AssignProfile(deviceID, profileID)` — inserts join-table row with `is_winbox=0`; UNIQUE constraint error on duplicate
- `UnassignProfile(deviceID, profileID)` — deletes join-table row; error if not assigned
- `SetWinboxProfile(deviceID, profileID)` — transactional: clears all is_winbox flags then sets target to 1; error if profile not assigned
- `ClearWinboxProfile(deviceID)` — sets all is_winbox=0 for device; idempotent (no error on zero rows)
- `GetWinboxAssignment(deviceID)` — returns profile data+encrypted_secret for the winbox-designated profile; error "no WinBox profile designated" if none

### IsInUse Fix (D-14)
Updated `IsInUse` from checking `devices WHERE ssh_profile_id = ?` to `device_credential_profiles WHERE profile_id = ?`.

### API Path Rename
All 4 `extractIDFromPath` calls in `credential_profile_handler.go` updated from `/api/v1/ssh-profiles/` to `/api/v1/credential-profiles/`. GoDoc comments and error messages updated accordingly.

### Router Updates
- Route registrations changed from `/api/v1/ssh-profiles` and `/api/v1/ssh-profiles/` to `/api/v1/credential-profiles` and `/api/v1/credential-profiles/`
- Comment updated to "Credential profile routes"
- Bridge binary download bypass added: `strings.HasPrefix(r.URL.Path, "/api/v1/bridge/download/")` — bypasses JSON content-type and body size middleware

### Config Field
`BridgeBinariesDir string` added to `Config` struct with `yaml:"bridge_binaries_dir"` tag and `THEIA_BRIDGE_BINARIES_DIR` env override.

### Test Updates
- `credential_profile_handler_test.go`: All URLs updated from `/api/v1/ssh-profiles` to `/api/v1/credential-profiles`; `TestCredentialProfileHandlerDelete_InUse` now inserts into `device_credential_profiles` join table instead of using `ssh_profile_id` FK
- `oversized_body_test.go`: Updated 3 test case paths and comment from "SSHProfileHandler" to "CredentialProfileHandler"

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] Updated oversized_body_test.go to use new credential-profiles paths**
- **Found during:** Task 2 acceptance verification (`grep -r "ssh-profiles" internal/api/`)
- **Issue:** `oversized_body_test.go` still referenced `/api/v1/ssh-profiles` in 3 test cases and had an outdated comment "SSHProfileHandler mutations"
- **Fix:** Updated all 3 paths and the comment to use `/api/v1/credential-profiles`
- **Files modified:** `internal/api/oversized_body_test.go`
- **Commit:** 6271021

## Threat Model Coverage

| Threat ID | Disposition | Status |
|-----------|-------------|--------|
| T-24-01 | mitigate | All 6 new repo methods use parameterized queries (`?` placeholders) — no string interpolation |
| T-24-02 | mitigate | IsInUse updated to check canonical `device_credential_profiles` table |
| T-24-03 | mitigate | `credentialProfileResponse` struct omits `EncryptedSecret` entirely; `GetWinboxAssignment` returns it only for server-side use |
| T-24-04 | accept | `BridgeBinariesDir` is server-side only; not exposed via API |

## Known Stubs

None — all new repo methods are fully implemented and tested.

## Threat Flags

None — no new network endpoints, auth paths, or trust boundary changes introduced beyond what is in the plan's threat model.

## Self-Check: PASSED

Files created/exist:
- FOUND: internal/repository/sqlite/migrations/000013_device_credential_profiles_winbox.up.sql
- FOUND: internal/repository/sqlite/migrations/000013_device_credential_profiles_winbox.down.sql
- FOUND: internal/repository/sqlite/credential_profile_assignment_test.go

Commits verified:
- FOUND: 89dce7e (TDD RED tests)
- FOUND: c1247c6 (Task 1 implementation)
- FOUND: 6271021 (Task 2 implementation)

Build: exit 0
Tests: all pass (14 repo tests, 15 API handler tests)
