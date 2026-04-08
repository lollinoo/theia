---
phase: 27-schema-cleanup-drop-legacy-fk
plan: "01"
subsystem: backend
tags: [migration, schema, credential-profiles, ssh, cleanup]
dependency_graph:
  requires: [23-01, 23-02]
  provides: [devices-table-without-ssh-profile-id, join-table-credential-resolution]
  affects: [backup-service, device-repo, device-handler, device-service]
tech_stack:
  added: []
  patterns: [SQLite-12-step-table-recreation, join-table-credential-resolution]
key_files:
  created:
    - internal/repository/sqlite/migrations/000014_drop_ssh_profile_id.up.sql
    - internal/repository/sqlite/migrations/000014_drop_ssh_profile_id.down.sql
  modified:
    - internal/repository/sqlite/migrations.go
    - internal/domain/device.go
    - internal/domain/credential_profile.go
    - internal/repository/sqlite/device_repo.go
    - internal/repository/sqlite/credential_profile_repo.go
    - internal/api/device_handler.go
    - internal/api/device_handler_test.go
    - internal/service/device_service.go
    - internal/service/device_service_test.go
    - internal/service/backup_service.go
    - internal/service/backup_service_test.go
    - internal/worker/device_backup_scheduler_test.go
decisions:
  - "Migration 000014 uses SQLite 12-step recreation wrapped in PRAGMA foreign_keys=off/on per T-27-01"
  - "GetBackupProfileForDevice orders by is_winbox ASC so non-WinBox profiles sort before WinBox profiles (T-27-04 accepted)"
  - "verifyLegacyTablesMigrated checks pragma_table_info before querying ssh_profile_id to handle post-000014 databases safely"
  - "device_service_test.go AddDevice calls fixed as Rule 3 auto-fix (blocking compilation)"
metrics:
  duration_minutes: 6
  completed_date: "2026-04-08"
  tasks_completed: 2
  files_modified: 12
---

# Phase 27 Plan 01: Drop Legacy SSH Profile FK — Summary

**One-liner:** SQLite 12-step migration drops `ssh_profile_id` from devices table; backup service resolves credentials via `device_credential_profiles` join table using new `GetBackupProfileForDevice` method.

## Tasks Completed

| Task | Name | Commit | Key Files |
|------|------|--------|-----------|
| 1 | Create migration 000014 and update verifyLegacyTablesMigrated | f85204f | 000014 up/down sql, migrations.go |
| 2 | Remove SSHProfileID from domain, repo, service, handler | d078064 | device.go, device_repo.go, backup_service.go, +9 files |

## What Was Built

**Migration 000014** (`000014_drop_ssh_profile_id.up.sql`): Drops the legacy `ssh_profile_id` column from `devices` using the SQLite 12-step table-recreation pattern. The migration:
- Creates `devices_new` without `ssh_profile_id`
- Copies all rows by explicit column name (not `SELECT *`)
- Drops `devices`, renames `devices_new` to `devices`
- Wrapped in `PRAGMA foreign_keys=off/on` per T-27-01

**Down migration** (`000014_drop_ssh_profile_id.down.sql`): Simple `ALTER TABLE devices ADD COLUMN ssh_profile_id TEXT DEFAULT NULL` — no data loss, existing `device_credential_profiles` records are unaffected.

**verifyLegacyTablesMigrated guard**: Added `pragma_table_info('devices')` check before querying `ssh_profile_id`. On post-000014 databases, the column is absent and the function exits early with a log message. This prevents runtime errors on freshly migrated instances.

**GetBackupProfileForDevice** (`credential_profile_repo.go`): New method on `CredentialProfileRepo` that queries the `device_credential_profiles` join table ordered by `is_winbox ASC, name ASC`, returning the first non-WinBox profile (or first profile if all are WinBox). Returns error if no profile is assigned.

**SSHProfileID removal** (coordinated across all layers):
- `domain.Device` struct: field removed
- `CredentialProfileRepository` interface: `GetBackupProfileForDevice` added
- `device_repo.go`: all SQL (INSERT 18 cols, UPDATE 16 SET fields, SELECT 18 cols, scan functions) cleaned
- `DeviceService.AddDevice`: parameter removed (11 params now)
- `DeviceUpdate`: `SSHProfileID **uuid.UUID` field removed
- `device_handler.go`: request structs, HandleCreate, HandleUpdate, HandleBatchAdd, deviceToResource all cleaned
- `backup_service.go`: 3 call sites replaced with `GetBackupProfileForDevice`

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] Fixed device_service_test.go AddDevice signature mismatch**
- **Found during:** Task 2 test run
- **Issue:** `device_service_test.go` had 8 `AddDevice` calls with 12 arguments (old signature with `sshProfileID`). The plan only listed `device_handler_test.go` and `device_backup_scheduler_test.go` as needing updates — `device_service_test.go` was also affected.
- **Fix:** Used `sed` to remove the extra `nil` argument from all affected calls (pattern `, nil, nil)` → `, nil)`)
- **Files modified:** `internal/service/device_service_test.go`
- **Commit:** d078064 (included in Task 2 commit)

## Known Stubs

None — all credential resolution is wired through the live `device_credential_profiles` join table.

## Threat Surface Scan

No new network endpoints, auth paths, or file access patterns introduced. The `GetBackupProfileForDevice` method accesses `device_credential_profiles` and `credential_profiles` tables — both already within the existing trust boundary. T-27-03 mitigation confirmed: `EncryptedSecret` has `json:"-"` and is decrypted only inside `BackupService.decryptSecret()`.

## Self-Check

Files created/exist:
- `internal/repository/sqlite/migrations/000014_drop_ssh_profile_id.up.sql` — FOUND
- `internal/repository/sqlite/migrations/000014_drop_ssh_profile_id.down.sql` — FOUND

Commits:
- f85204f (task 1) — FOUND
- d078064 (task 2) — FOUND

Build: PASSED (`go build ./...`)
Tests: PASSED (`go test ./internal/... ./cmd/...` — all packages)

## Self-Check: PASSED
