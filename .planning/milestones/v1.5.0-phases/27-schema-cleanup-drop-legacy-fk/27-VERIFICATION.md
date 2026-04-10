---
phase: 27-schema-cleanup-drop-legacy-fk
verified: 2026-04-08T13:15:00Z
status: passed
score: 11/11 must-haves verified
re_verification: false
---

# Phase 27: Schema Cleanup — Drop Legacy FK — Verification Report

**Phase Goal:** Drop the legacy `ssh_profile_id` FK column from the devices table, remove all Go references to `SSHProfileID`, and update the backup service to resolve credentials through the `device_credential_profiles` join table.
**Verified:** 2026-04-08T13:15:00Z
**Status:** PASSED
**Re-verification:** No — initial verification

---

## Goal Achievement

### Observable Truths

| # | Truth | Status | Evidence |
|---|-------|--------|----------|
| 1 | `ssh_profile_id` column absent from devices table after migration 000014 | VERIFIED | Migration 000014 up.sql uses 12-step pattern — `CREATE TABLE devices_new` without `ssh_profile_id`, `DROP TABLE devices`, `ALTER TABLE devices_new RENAME TO devices` |
| 2 | Application compiles with zero SSHProfileID references in production Go code | VERIFIED | `go build ./...` exits 0; `grep -r SSHProfileID internal/ --include=*.go` returns 0 matches total (including test files) |
| 3 | Backup service resolves credentials via `device_credential_profiles` join table | VERIFIED | `backup_service.go` has 3 calls to `s.credentialProfileRepo.GetBackupProfileForDevice(...)` replacing all former `device.SSHProfileID` lookups |
| 4 | Go test suite passes with no failures | VERIFIED | `go test ./internal/... ./cmd/...` exits 0; all packages pass |
| 5 | Device type has no `ssh_profile_id` field in TypeScript | VERIFIED | `grep -r ssh_profile_id frontend/src/ --include=*.ts --include=*.tsx` returns 0 functional matches (3 comment-only occurrences) |
| 6 | Create and update API calls do not send `ssh_profile_id` in payload | VERIFIED | `CreateDevicePayload` and `updateDevice` payload in `client.ts` have zero `ssh_profile_id` references |
| 7 | BulkBackupPanel checks credential profile assignment via join-table API | VERIFIED | `handleStart` is async; imports and calls `fetchDeviceCredentialProfiles` via `Promise.allSettled`; skip reason is "no credential profile assigned" |
| 8 | TypeScript compilation succeeds | VERIFIED | `npx tsc --noEmit` exits 0 from `frontend/` |
| 9 | All frontend tests pass | VERIFIED | `npx vitest run` exits 0; 440 tests across 41 files all pass |
| 10 | `GetBackupProfileForDevice` defined in domain interface | VERIFIED | `internal/domain/credential_profile.go` contains the method signature in `CredentialProfileRepository` interface |
| 11 | `verifyLegacyTablesMigrated` handles post-000014 databases safely | VERIFIED | `migrations.go` checks `pragma_table_info('devices')` for `ssh_profile_id` column existence before querying; logs "ssh_profile_id column already dropped, skipping" |

**Score:** 11/11 truths verified

---

### Required Artifacts

| Artifact | Expected | Status | Details |
|----------|----------|--------|---------|
| `internal/repository/sqlite/migrations/000014_drop_ssh_profile_id.up.sql` | 12-step recreation dropping ssh_profile_id | VERIFIED | Contains `CREATE TABLE devices_new`, `DROP TABLE devices`, `ALTER TABLE devices_new RENAME TO devices`, wrapped in `PRAGMA foreign_keys=off/on` |
| `internal/repository/sqlite/migrations/000014_drop_ssh_profile_id.down.sql` | Reverse migration re-adding ssh_profile_id | VERIFIED | `ALTER TABLE devices ADD COLUMN ssh_profile_id TEXT DEFAULT NULL` |
| `internal/domain/device.go` | Device struct without SSHProfileID field | VERIFIED | `grep SSHProfileID internal/domain/device.go` returns 0 matches |
| `internal/domain/credential_profile.go` | Interface with GetBackupProfileForDevice method | VERIFIED | Method signature present in `CredentialProfileRepository` interface |
| `internal/service/backup_service.go` | Backup credential resolution via join table | VERIFIED | 3 call sites use `GetBackupProfileForDevice` |
| `internal/repository/sqlite/device_repo.go` | SQL queries without ssh_profile_id column | VERIFIED | `grep -c ssh_profile_id device_repo.go` returns 0 |
| `frontend/src/components/dashboard/BulkBackupPanel.tsx` | Backup eligibility using fetchDeviceCredentialProfiles | VERIFIED | Imports and calls `fetchDeviceCredentialProfiles` in async `handleStart` |

---

### Key Link Verification

| From | To | Via | Status | Details |
|------|----|-----|--------|---------|
| `backup_service.go` | `domain.CredentialProfileRepository.GetBackupProfileForDevice` | method call replacing device.SSHProfileID lookup | WIRED | 3 call sites confirmed; `BulkBackup`, `TriggerBackup`, `TestSSHConnection` all updated |
| `device_repo.go` | devices table | SQL queries without ssh_profile_id | WIRED | INSERT, UPDATE, SELECT, scan functions all clean; 0 ssh_profile_id references |
| `BulkBackupPanel.tsx` | `client.ts fetchDeviceCredentialProfiles` | import and async call for eligibility check | WIRED | Import confirmed at line 1; used in `Promise.allSettled` batch fetch in `handleStart` |

---

### Data-Flow Trace (Level 4)

| Artifact | Data Variable | Source | Produces Real Data | Status |
|----------|---------------|--------|--------------------|--------|
| `BulkBackupPanel.tsx` | `deviceHasProfile` (Map) | `fetchDeviceCredentialProfiles` → `/api/v1/devices/:id/credential-profiles` | Yes — live DB query via join table | FLOWING |
| `backup_service.go` | `profile` (*CredentialProfile) | `GetBackupProfileForDevice` → `device_credential_profiles JOIN credential_profiles` SQL query | Yes — real DB query with `ORDER BY is_winbox ASC, name ASC LIMIT 1` | FLOWING |

---

### Behavioral Spot-Checks

| Behavior | Check | Result | Status |
|----------|-------|--------|--------|
| Go compilation | `go build ./...` exits 0 | Clean build, no errors | PASS |
| Go tests | `go test ./internal/... ./cmd/...` exits 0 | All packages pass including `internal/api`, `internal/service`, `internal/worker`, `internal/repository/sqlite` | PASS |
| TypeScript compilation | `npx tsc --noEmit` exits 0 | Zero type errors | PASS |
| Frontend tests | `npx vitest run` exits 0 | 440/440 tests, 41/41 files | PASS |

---

### Requirements Coverage

| Requirement | Source Plan | Description | Status | Evidence |
|-------------|-------------|-------------|--------|----------|
| WINBOX-04 | 27-01, 27-02 | Drop legacy ssh_profile_id FK, migrate backup credential resolution to join table | SATISFIED | Migration 000014 exists and is correct; `GetBackupProfileForDevice` implemented and wired throughout backend and frontend |

---

### Anti-Patterns Found

| File | Line | Pattern | Severity | Impact |
|------|------|---------|----------|--------|
| `BulkBackupPanel.tsx` | 98 | `// ...replaces legacy ssh_profile_id check` | Info | Comment-only; explains migration context; not a code smell |
| `Dashboard.tsx` | 42, 240 | `// after ssh_profile_id removal` | Info | Comment-only; documents Option A decision per plan requirement |

No blockers. No stubs. No `TODO`/`FIXME` markers in phase deliverables. The comment-only matches in the frontend are intentional documentation of the migration decision as required by the plan spec.

---

### Human Verification Required

None. All acceptance criteria are verifiable programmatically and have passed.

---

### Gaps Summary

No gaps. All 11 observable truths verified. All required artifacts exist, are substantive, and are correctly wired. Data flows through real DB queries in both backend and frontend paths.

---

## Acceptance Criteria Checklist

| Criterion | Result |
|-----------|--------|
| 1. `go build ./...` exits 0 | PASS |
| 2. `go test ./internal/... ./cmd/...` passes all tests | PASS |
| 3. `grep -r "SSHProfileID" internal/ --include="*.go"` returns 0 matches in production code | PASS (0 matches total including tests) |
| 4. `grep -c "ssh_profile_id" internal/repository/sqlite/device_repo.go` returns 0 | PASS |
| 5. Migration `000014_drop_ssh_profile_id.up.sql` exists with `CREATE TABLE devices_new` and `PRAGMA foreign_keys` | PASS |
| 6. `grep "GetBackupProfileForDevice" internal/domain/credential_profile.go` matches | PASS |
| 7. `grep "GetBackupProfileForDevice" internal/service/backup_service.go` matches | PASS (3 matches) |
| 8. `npx tsc --noEmit` from frontend/ exits 0 | PASS |
| 9. `npx vitest run` from frontend/ passes all tests | PASS (440/440) |
| 10. `grep -r "ssh_profile_id" frontend/src/ --include="*.ts" --include="*.tsx"` returns 0 functional matches | PASS (3 comment-only matches, 0 functional) |
| 11. `grep "fetchDeviceCredentialProfiles" BulkBackupPanel.tsx` has at least 1 match | PASS (2 matches: import + usage) |

---

_Verified: 2026-04-08T13:15:00Z_
_Verifier: Claude (gsd-verifier)_
