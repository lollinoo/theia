---
phase: 23-credential-profile-schema-domain
plan: 02
subsystem: api,service,repository
tags: [rename, credential-profile, ssh-profile, go-refactor, zero-behavior-change]

# Dependency graph
requires:
  - 23-01 (CredentialProfile domain type + migration 000012)
provides:
  - CredentialProfileRepo implementing CredentialProfileRepository (replaces SSHProfileRepo)
  - CredentialProfileHandler with Role field in request/response (replaces SSHProfileHandler)
  - BackupService with credentialProfileRepo field and renamed CRUD methods
  - All Go files compile with zero errors after SSHProfile -> CredentialProfile rename
  - All existing Go tests pass with no behavior changes
affects:
  - 24-winbox-profile-schema (can now reference CredentialProfile domain and repo cleanly)
  - 25-winbox-local-bridge
  - 26-winbox-ui
  - 27-legacy-cleanup

# Tech tracking
tech-stack:
  added: []
  patterns:
    - Pure rename pattern: new file creation + old file deletion (same as Plan 01)
    - Zero behavior change: all logic identical, only type names updated

key-files:
  created:
    - internal/repository/sqlite/credential_profile_repo.go
    - internal/api/credential_profile_handler.go
    - internal/api/credential_profile_handler_test.go
  modified:
    - internal/service/backup_service.go
    - internal/api/router.go
    - internal/api/device_handler.go
    - cmd/theia/main.go
    - internal/service/backup_service_test.go
    - internal/api/device_handler_test.go
    - internal/api/oversized_body_test.go
    - internal/api/backup_handler_test.go
    - internal/worker/device_backup_scheduler_test.go
  deleted:
    - internal/repository/sqlite/ssh_profile_repo.go
    - internal/api/ssh_profile_handler.go
    - internal/api/ssh_profile_handler_test.go

key-decisions:
  - "API URL paths preserved as /api/v1/ssh-profiles — frontend compatibility until Phase 27"
  - "devices.ssh_profile_id FK column preserved per D-06 — IsInUse still queries legacy FK"
  - "credentialProfileResponse struct excludes EncryptedSecret (T-23-04 mitigated)"
  - "Role field defaults to Admin in both handler and service if not provided"
  - "BackupService logic unchanged: still uses device.SSHProfileID for credential lookup (D-10)"

patterns-established:
  - "CredentialProfileRepository interface pattern for all downstream credential types"
  - "credentialProfileToResponse function excludes EncryptedSecret — must be preserved in all credential handlers"

requirements-completed: [CRED-01, CRED-02, CRED-04]

# Metrics
duration: 11min
completed: 2026-04-07
---

# Phase 23 Plan 02: Credential Profile Schema + Domain (Consumer Rename) Summary

**Pure rename of all Go SSHProfile consumers to CredentialProfile: repository, service, API handler, router, main.go, and all test files — zero behavior changes, full test suite passes**

## Performance

- **Duration:** 11 min
- **Started:** 2026-04-07T18:29:53Z
- **Completed:** 2026-04-07T18:41:00Z
- **Tasks:** 3
- **Files modified:** 12 (3 created, 7 modified, 3 deleted)

## Accomplishments

- Created `credential_profile_repo.go`: CredentialProfileRepo implementing CredentialProfileRepository, SQL queries targeting `credential_profiles` table, Role field in all CRUD operations, IsInUse queries `devices WHERE ssh_profile_id` (legacy FK per D-06)
- Created `credential_profile_handler.go`: CredentialProfileHandler with Role field in request/response, EncryptedSecret excluded from response (T-23-04), Admin default for Role, API paths preserved as `/api/v1/ssh-profiles`
- Updated `backup_service.go`: renamed `sshProfileRepo` field to `credentialProfileRepo`, all CRUD methods renamed (Create/Get/GetAll/Update/Delete/TestCredentialProfile), `runFullBackup` param type updated, logic unchanged per D-10
- Updated `router.go`: `NewRouter` param type updated, `credentialProfileHandler` wired, URL paths preserved
- Updated `device_handler.go`: `sshProfileRepo` -> `credentialProfileRepo` with all 3 validation call sites updated
- Updated `main.go`: `NewCredentialProfileRepo` constructor call
- Renamed `ssh_profile_handler_test.go` -> `credential_profile_handler_test.go` with all types updated
- Updated all other test files: mockCredentialProfileRepo, stubCredentialProfileRepo, domain.CredentialProfile with Role field
- `go build ./...` passes with zero errors
- `go test ./internal/... ./cmd/...` passes with all tests passing

## Task Commits

Each task was committed atomically:

1. **Task 1: Rename repository and API handler files** - `fed10ef` (feat)
2. **Task 2: Rename service methods, update router wiring, and update main.go** - `1beb158` (feat)
3. **Task 3: Update all test files and verify full compilation + test pass** - `08f69d8` (feat)

## Files Created/Modified

- `internal/repository/sqlite/credential_profile_repo.go` - CredentialProfileRepo with credential_profiles table, role column, IsInUse legacy FK query
- `internal/repository/sqlite/ssh_profile_repo.go` - Deleted (replaced by credential_profile_repo.go)
- `internal/api/credential_profile_handler.go` - CredentialProfileHandler with Role field, EncryptedSecret excluded from response
- `internal/api/ssh_profile_handler.go` - Deleted (replaced by credential_profile_handler.go)
- `internal/api/credential_profile_handler_test.go` - Full handler test suite (renamed from ssh_profile_handler_test.go)
- `internal/api/ssh_profile_handler_test.go` - Deleted (replaced by credential_profile_handler_test.go)
- `internal/service/backup_service.go` - credentialProfileRepo field, renamed CRUD methods, runFullBackup param type
- `internal/api/router.go` - credentialProfileRepo param, credentialProfileHandler wiring, paths preserved
- `internal/api/device_handler.go` - credentialProfileRepo field + 3 call sites, updated error messages
- `cmd/theia/main.go` - NewCredentialProfileRepo, updated BackupService and NewRouter calls
- `internal/service/backup_service_test.go` - mockCredentialProfileRepo, domain.CredentialProfile with Role
- `internal/api/device_handler_test.go` - mockCredentialProfileRepo, newTestDeviceHandler updated
- `internal/api/oversized_body_test.go` - CredentialProfileHandler test cases renamed
- `internal/api/backup_handler_test.go` - credentialProfileRepo in setupBackupHandler
- `internal/worker/device_backup_scheduler_test.go` - stubCredentialProfileRepo, newTestDeviceBackupService updated

## Decisions Made

- API URL paths `/api/v1/ssh-profiles` preserved — frontend still uses these paths; renaming deferred to Phase 27
- `devices.ssh_profile_id` FK column still queried in `IsInUse` — legacy column preserved per D-06 until Phase 27
- `credentialProfileResponse` struct intentionally excludes `EncryptedSecret` — T-23-04 mitigation enforced
- Role defaults to "Admin" in both handler and service if not provided in request — backward compatible
- `TestSSHConnection` method on BackupService kept as-is (tests SSH connectivity for a device, not a profile concept)

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered

One minor fix during Task 3: `backup_handler_test.go` had a second `sshProfileRepo` usage (line 524) beyond the `setupBackupHandler` function that was missed in the initial pass. Fixed inline per Rule 1 (auto-fix bug).

## User Setup Required

None.

## Next Phase Readiness

- Phase 24 (winbox-profile-schema) can now import `domain.CredentialProfile` and `sqlite.CredentialProfileRepo` without name conflicts
- All downstream phases (24-27) work against clean canonical names
- Migration 000012 + domain type (Plan 01) + consumer rename (Plan 02) = Phase 23 complete

---
*Phase: 23-credential-profile-schema-domain*
*Completed: 2026-04-07*

## Self-Check: PASSED

- FOUND: internal/repository/sqlite/credential_profile_repo.go
- FOUND: internal/api/credential_profile_handler.go
- FOUND: internal/api/credential_profile_handler_test.go
- FOUND: internal/repository/sqlite/ssh_profile_repo.go deleted
- FOUND: internal/api/ssh_profile_handler.go deleted
- FOUND: internal/api/ssh_profile_handler_test.go deleted
- FOUND: go build ./... passes
- FOUND: go test ./internal/... ./cmd/... passes (all packages ok)
- FOUND: commit fed10ef (Task 1)
- FOUND: commit 1beb158 (Task 2)
- FOUND: commit 08f69d8 (Task 3)
