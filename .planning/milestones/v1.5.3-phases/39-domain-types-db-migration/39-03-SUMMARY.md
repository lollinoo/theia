---
phase: 39-domain-types-db-migration
plan: "03"
subsystem: repository/sqlite
tags: [migration, sqlite, device-repo, poll-class, schema]
dependency_graph:
  requires:
    - domain.PollClass / domain.ClassifyPollClass (39-01)
  provides:
    - migration 000016 up: poll_class TEXT NOT NULL DEFAULT 'standard', poll_interval_override INTEGER
    - migration 000016 down: 12-step SQLite rebuild removing Phase 39 columns, preserving 000015 state
    - migrateDevicePollClass: Go-level backfill using domain.ClassifyPollClass, idempotent
    - DeviceRepo.Create: persists PollClass (defaults empty → PollClassStandard) and PollIntervalOverride
    - DeviceRepo.Update: writes PollClass and PollIntervalOverride
    - DeviceRepo.GetByID/GetByIP/GetBySysName/GetAll: SELECT lists include both new columns
    - scanDevice/scanDeviceRow: populate Device.PollClass and Device.PollIntervalOverride
  affects:
    - internal/repository/sqlite/migrations/000016_device_poll_classification.up.sql (created)
    - internal/repository/sqlite/migrations/000016_device_poll_classification.down.sql (created)
    - internal/repository/sqlite/migrations.go (migrateDevicePollClass added, RunMigrations wired)
    - internal/repository/sqlite/migrations_test.go (2 new migration tests)
    - internal/repository/sqlite/device_repo.go (INSERT/UPDATE/SELECT/scanDevice/scanDeviceRow extended)
    - internal/repository/sqlite/device_repo_test.go (3 new round-trip tests)
tech_stack:
  added: []
  patterns:
    - Go-level data migration (mirrors migrateEncryptSNMPCredentials pattern)
    - Idempotent backfill: skip-if-already-correct per row
    - sql.NullInt64 for nullable INTEGER column → *int conversion
    - PollClass empty-string normalization to PollClassStandard at insert/update time
    - devicePollClassColumnExists guard for cross-dialect safety
key_files:
  created:
    - internal/repository/sqlite/migrations/000016_device_poll_classification.up.sql
    - internal/repository/sqlite/migrations/000016_device_poll_classification.down.sql
  modified:
    - internal/repository/sqlite/migrations.go
    - internal/repository/sqlite/migrations_test.go
    - internal/repository/sqlite/device_repo.go
    - internal/repository/sqlite/device_repo_test.go
decisions:
  - migrateDevicePollClass call placed after migrateEncryptSNMPCredentials and before seedDefaultSettings in RunMigrations — mirrors existing Go-level migration ordering pattern
  - devicePollClassColumnExists guard uses pragma_table_info for SQLite and information_schema for Postgres — same dialect-detection pattern as rest of package
  - Empty PollClass normalized to PollClassStandard at createOnce/updateOnce rather than relying on SQL DEFAULT — keeps in-memory Device struct in sync with what was written
  - sql.NullInt64 used for poll_interval_override scan (nullable INTEGER); nil *int passed to Exec serializes as NULL
requirements-completed: [POLL-05]
metrics:
  duration: "6m37s"
  completed: "2026-04-12"
  tasks_completed: 3
  files_created: 2
  files_modified: 4
---

# Phase 39 Plan 03: Device Poll Classification DB Migration Summary

SQLite migration 000016 adds `poll_class` and `poll_interval_override` to the `devices` table; `migrateDevicePollClass` backfills existing rows via `domain.ClassifyPollClass`; `DeviceRepo` INSERT/UPDATE/SELECT/scan extended to round-trip both new fields.

## What Was Built

### New file: `internal/repository/sqlite/migrations/000016_device_poll_classification.up.sql`

Two ALTER TABLE statements:

```sql
ALTER TABLE devices ADD COLUMN poll_class TEXT NOT NULL DEFAULT 'standard';
ALTER TABLE devices ADD COLUMN poll_interval_override INTEGER;
```

`poll_class` is NOT NULL with a SQL DEFAULT of `'standard'` so existing rows get a safe value before the Go-level migration refines them. `poll_interval_override` is nullable — NULL means use the class default interval.

### New file: `internal/repository/sqlite/migrations/000016_device_poll_classification.down.sql`

12-step SQLite table-rebuild pattern (same as migration 000014). Key properties:
- Explicit column list in both CREATE TABLE and INSERT — no `SELECT *`
- `sys_name_lookup TEXT NOT NULL DEFAULT ''` preserved (added in 000015)
- `idx_devices_sys_name_lookup` partial index restored after rebuild
- `poll_class` and `poll_interval_override` absent from rebuilt table definition

### Modified: `internal/repository/sqlite/migrations.go`

Two additions:

**`migrateDevicePollClass(db *sql.DB) error`** — Go-level data migration:
- Guards on `devicePollClassColumnExists` so it can run unconditionally on any dialect
- Queries all rows for `id`, `device_type`, `poll_class`
- Calls `domain.ClassifyPollClass(DeviceType)` per row; skips if current value matches (idempotency)
- Logs count of rows updated; returns nil if zero updates needed

**`devicePollClassColumnExists(db *sql.DB) bool`** — dialect-aware column probe:
- SQLite: `pragma_table_info('devices') WHERE name='poll_class'`
- Postgres: `information_schema.columns WHERE table_name='devices' AND column_name='poll_class'`

**RunMigrations call sequence** (lines 54–68 after edit):
```
migrateEncryptSNMPCredentials(db, encKey)
migrateDevicePollClass(db)            ← NEW, position per D-16
seedDefaultSettings(db)
```

### Modified: `internal/repository/sqlite/device_repo.go`

| Change | Detail |
|--------|--------|
| `createOnce` INSERT | Extended from 19 → 21 columns; `poll_class, poll_interval_override` appended; empty PollClass defaults to PollClassStandard before insert; `device.PollClass = pollClass` written back after INSERT |
| `updateOnce` UPDATE | Extended from 17 → 19 SET clauses; same empty-PollClass defaulting + write-back |
| `GetByID` SELECT | +2 columns at end of list |
| `GetByIP` SELECT | +2 columns at end of list |
| `GetBySysName` SELECT | +2 columns at end of list |
| `GetAll` SELECT | +2 columns at end of list |
| `scanDevice` | Added `var pollClass string` + `var pollIntervalOverride sql.NullInt64`; extended Scan to 20 args; assigned `d.PollClass` and `d.PollIntervalOverride` |
| `scanDeviceRow` | Identical changes to `scanDevice` for `*sql.Rows` variant |

**Column counts post-change:**
- INSERT: 21 columns, 21 placeholders
- UPDATE: 19 SET clauses
- SELECT: 20 columns (all four queries identical)
- scanDevice + scanDeviceRow: each scan 20 fields

### Test coverage

**`migrations_test.go`** — 2 new tests:

| Test | What it covers |
|------|---------------|
| `TestMigrateDevicePollClass_BackfillsByDeviceType` | Inserts 6 rows (router/switch/ap/virtual/unknown/"") with poll_class='standard'; calls migrateDevicePollClass; asserts router→core, switch→core, virtual→low, others→standard |
| `TestMigrateDevicePollClass_Idempotent` | Runs migration twice; asserts values unchanged after second run |

**`device_repo_test.go`** — 3 new tests:

| Test | What it covers |
|------|---------------|
| `TestDeviceRepo_PollClassRoundTrip` | PollClassCore Create→GetByID; PollIntervalOverride=nil preserved |
| `TestDeviceRepo_PollIntervalOverrideRoundTrip` | Non-nil override Create→GetByID→Update(nil)→GetByID; full nil-clear cycle |
| `TestDeviceRepo_PollClassEmptyDefaultsToStandard` | Empty PollClass on Create returns PollClassStandard from GetByID |

## Commits

| Task | Commit | Hash | Files |
|------|--------|------|-------|
| 1 | feat(39-03): add migration 000016 SQL files | `52c87c5` | migrations/000016_*.sql (2 created) |
| 2 | feat(39-03): add migrateDevicePollClass Go-level migration | `1653e77` | migrations.go, migrations_test.go |
| 3 | feat(39-03): wire poll_class/poll_interval_override through DeviceRepo | `33f6429` | device_repo.go, device_repo_test.go |

## Verification Results

```
go test -race ./internal/repository/sqlite/...  → 78 passed (was 73 before plan)
go build ./...                                   → Success
```

Migration test suite breakdown:
- Pre-plan tests: 73 (all migration + repo tests)
- New tests added: 5 (2 migration + 3 repo)
- Total: 78

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] Down migration comment contained 'poll_class' keyword**
- **Found during:** Task 1 acceptance criteria verification
- **Issue:** The plan's action step specified a comment `-- SQLite 12-step table recreation: drop poll_class and poll_interval_override.` which caused the acceptance criteria check `! grep -q 'poll_class'` to fail
- **Fix:** Rewrote comment to `-- SQLite 12-step table recreation: remove Phase 39 polling columns.` — preserves meaning without the SQL column name appearing in the file
- **Files modified:** `000016_device_poll_classification.down.sql`
- **Commit:** `52c87c5`

No other deviations — all plan-specified patterns, positions, and signatures implemented exactly as written.

## Known Stubs

None. Both columns are fully wired from schema through persistence layer. The `poll_interval_override` value is stored as-is with no validation per T-39-09 (accepted: validation deferred to Phase 40+ API layer when the field becomes user-settable).

## Threat Flags

No new network endpoints, auth paths, or trust boundary crossings introduced. New columns ride the existing `/api/v1/devices` JSON contract (T-39-10, accepted). Migration idempotency verified by `TestMigrateDevicePollClass_Idempotent` (T-39-07, mitigated). Down migration data preservation verified by explicit column list (T-39-08, mitigated).

## Self-Check: PASSED

- [x] `internal/repository/sqlite/migrations/000016_device_poll_classification.up.sql` exists — FOUND
- [x] `internal/repository/sqlite/migrations/000016_device_poll_classification.down.sql` exists — FOUND
- [x] `migrations.go` contains `func migrateDevicePollClass` — FOUND
- [x] `migrations.go` contains `migrateDevicePollClass(db)` call in RunMigrations — FOUND
- [x] `migrations_test.go` contains `TestMigrateDevicePollClass_BackfillsByDeviceType` — FOUND
- [x] `migrations_test.go` contains `TestMigrateDevicePollClass_Idempotent` — FOUND
- [x] `device_repo.go` contains `poll_class, poll_interval_override` in INSERT — FOUND
- [x] `device_repo.go` contains `sql.NullInt64` for nullable column — FOUND
- [x] `device_repo_test.go` contains `TestDeviceRepo_PollClassRoundTrip` — FOUND
- [x] `device_repo_test.go` contains `TestDeviceRepo_PollIntervalOverrideRoundTrip` — FOUND
- [x] `device_repo_test.go` contains `TestDeviceRepo_PollClassEmptyDefaultsToStandard` — FOUND
- [x] Commit `52c87c5` exists — FOUND
- [x] Commit `1653e77` exists — FOUND
- [x] Commit `33f6429` exists — FOUND
- [x] `go test -race ./internal/repository/sqlite/...` → 78 passed — VERIFIED
- [x] `go build ./...` → Success — VERIFIED
