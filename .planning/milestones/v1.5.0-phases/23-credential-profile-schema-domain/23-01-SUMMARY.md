---
phase: 23-credential-profile-schema-domain
plan: 01
subsystem: database
tags: [sqlite, migration, domain, credentials, ssh-profiles]

# Dependency graph
requires: []
provides:
  - Migration 000012 that renames ssh_profiles to credential_profiles and adds role column
  - device_credential_profiles join table seeded from existing ssh_profile_id FK values
  - CredentialProfile domain struct with Role field and EncryptedSecret json:"-" protection
  - CredentialProfileRepository interface replacing SSHProfileRepository
affects:
  - 23-02 (credential_profile_repo.go rename + BackupService update)
  - 24-winbox-profile-schema
  - 25-winbox-local-bridge
  - 26-winbox-ui
  - 27-legacy-cleanup

# Tech tracking
tech-stack:
  added: []
  patterns:
    - SQLite 12-step column recreation in down migration (no DROP COLUMN on older SQLite)
    - Domain type rename via new file creation + old file deletion pattern
    - join table seeded from existing FK in single migration for zero data loss

key-files:
  created:
    - internal/repository/sqlite/migrations/000012_credential_profiles.up.sql
    - internal/repository/sqlite/migrations/000012_credential_profiles.down.sql
    - internal/domain/credential_profile.go
  modified:
    - internal/domain/ssh_profile.go (deleted)

key-decisions:
  - "Deleted ssh_profile.go and created credential_profile.go (new file) instead of renaming in place — cleaner git history"
  - "devices.ssh_profile_id FK column NOT dropped — deferred to Phase 27 per D-06"
  - "Role field is free-text string not enum — enables custom labels per CRED-01"
  - "SSHAuthMethod type stays in backup.go unmodified per D-04"

patterns-established:
  - "EncryptedSecret string with json:\"-\" tag pattern — must be preserved in all credential types"

requirements-completed: [CRED-01, CRED-02, CRED-04]

# Metrics
duration: 2min
completed: 2026-04-07
---

# Phase 23 Plan 01: Credential Profile Schema + Domain Summary

**SQLite migration 000012 renames ssh_profiles to credential_profiles with role='Admin' default, creates device_credential_profiles join table seeded from existing FK, and introduces CredentialProfile domain type replacing SSHProfile**

## Performance

- **Duration:** 2 min
- **Started:** 2026-04-07T18:25:08Z
- **Completed:** 2026-04-07T18:26:26Z
- **Tasks:** 2
- **Files modified:** 4 (2 created, 1 deleted, 1 created replacing deleted)

## Accomplishments
- Created migration 000012 up/down: renames ssh_profiles table, adds role column (DEFAULT 'Admin'), recreates unique index, creates device_credential_profiles join table, seeds from existing ssh_profile_id FK
- Created CredentialProfile domain struct with Role string field and EncryptedSecret json:"-" protection (T-23-01 mitigated)
- Defined CredentialProfileRepository interface with Create/GetByID/GetAll/Update/Delete matching prior SSHProfileRepository signatures
- Deleted internal/domain/ssh_profile.go; replaced by credential_profile.go

## Task Commits

Each task was committed atomically:

1. **Task 1: Create migration 000012 (up + down)** - `e125541` (chore)
2. **Task 2: Create CredentialProfile domain type and delete SSHProfile** - `4822e16` (feat)

**Plan metadata:** (see final commit)

## Files Created/Modified
- `internal/repository/sqlite/migrations/000012_credential_profiles.up.sql` - Up migration: rename table, add role column, recreate index, create join table, seed data
- `internal/repository/sqlite/migrations/000012_credential_profiles.down.sql` - Down migration: drop join table, 12-step column drop, rename back to ssh_profiles
- `internal/domain/credential_profile.go` - CredentialProfile struct and CredentialProfileRepository interface
- `internal/domain/ssh_profile.go` - Deleted (replaced by credential_profile.go)

## Decisions Made
- Used new file `credential_profile.go` + deletion of `ssh_profile.go` rather than renaming in place — cleaner separation and clearer git history
- Role field is `string` (free-text) not a typed enum — enables custom labels per CRED-01 requirements
- `SSHAuthMethod` type and constants remain in `backup.go` unmodified per D-04 — they describe the auth mechanism, not the profile concept
- `devices.ssh_profile_id` FK column intentionally NOT dropped — deferred to Phase 27 per D-06 to allow BackupService to continue functioning in intermediate phases

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered
None — migration SQL and domain type were straightforward. The codebase will be in a transitional broken state (other files still reference `domain.SSHProfile`) until plan 23-02 completes the repository and service layer renames.

## User Setup Required
None - no external service configuration required.

## Next Phase Readiness
- Migration 000012 is ready to apply; golang-migrate runner will pick it up on next startup
- CredentialProfile domain type is the canonical type for all downstream phases (23-02 through 27)
- Plan 23-02 must update: credential_profile_repo.go, backup_service.go, ssh_profile_handler.go, router.go, and cmd/theia/main.go to restore compilation

---
*Phase: 23-credential-profile-schema-domain*
*Completed: 2026-04-07*

## Self-Check: PASSED

- FOUND: internal/repository/sqlite/migrations/000012_credential_profiles.up.sql
- FOUND: internal/repository/sqlite/migrations/000012_credential_profiles.down.sql
- FOUND: internal/domain/credential_profile.go
- FOUND: internal/domain/ssh_profile.go deleted (as expected)
- FOUND: .planning/phases/23-credential-profile-schema-domain/23-01-SUMMARY.md
- FOUND: commit e125541 (migration)
- FOUND: commit 4822e16 (domain type)
