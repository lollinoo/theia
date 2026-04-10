# Phase 23: Credential Profile Schema + Domain - Context

**Gathered:** 2026-04-07
**Status:** Ready for planning

<domain>
## Phase Boundary

Introduce the multi-profile credential data model: add a `role` column to the credential profiles table, create the `device_credential_profiles` join table, rename the existing `ssh_profiles` table and `SSHProfile` domain type to their canonical `credential_profiles`/`CredentialProfile` names, and migrate all existing records to `role = "Admin"`. `BackupService` is verified against the new schema but its credential-resolution logic is unchanged (still uses `device.SSHProfileID` FK). No new API endpoints; no UI changes.

Requirements in scope: CRED-01, CRED-02, CRED-04.

</domain>

<decisions>
## Implementation Decisions

### Table + Type Naming
- **D-01:** Rename `ssh_profiles` table → `credential_profiles` via `ALTER TABLE RENAME TO` in the migration (SQLite supports this directly — no 12-step needed).
- **D-02:** Rename domain type `SSHProfile` → `CredentialProfile`, `SSHProfileRepository` → `CredentialProfileRepository` in `internal/domain/ssh_profile.go` (or new file `credential_profile.go`).
- **D-03:** Rename implementation files: `ssh_profile_repo.go` → `credential_profile_repo.go`, `ssh_profile_handler.go` → `credential_profile_handler.go`. Update all import references and route registrations accordingly.
- **D-04:** `SSHAuthMethod` constants (`SSHAuthPassword`, `SSHAuthKey`) and the `SSHAuthMethod` type stay in `internal/domain/backup.go` — they describe the auth mechanism, not the profile concept, so no rename needed there.

### Schema Migration
- **D-05:** New migration file `000012_credential_profiles.up.sql` does the following in order:
  1. `ALTER TABLE ssh_profiles RENAME TO credential_profiles;`
  2. `ALTER TABLE credential_profiles ADD COLUMN role TEXT NOT NULL DEFAULT 'Admin';`
  3. Create `device_credential_profiles (device_id TEXT NOT NULL, profile_id TEXT NOT NULL, created_at DATETIME NOT NULL, PRIMARY KEY (device_id, profile_id), FOREIGN KEY (device_id) REFERENCES devices(id) ON DELETE CASCADE, FOREIGN KEY (profile_id) REFERENCES credential_profiles(id) ON DELETE CASCADE);`
  4. `INSERT INTO device_credential_profiles (device_id, profile_id, created_at) SELECT id, ssh_profile_id, CURRENT_TIMESTAMP FROM devices WHERE ssh_profile_id IS NOT NULL;`
- **D-06:** The `devices.ssh_profile_id` FK column is NOT dropped in this migration — it stays until Phase 27's dedicated cleanup migration.
- **D-07:** `idx_ssh_profiles_name` unique index is recreated as `idx_credential_profiles_name` (SQLite doesn't rename indexes; drop + create in migration).

### Join Table Design
- **D-08:** `device_credential_profiles` has exactly three columns: `device_id`, `profile_id`, `created_at`. No `is_winbox` or other flags — those are added by Phase 24's migration.
- **D-09:** Primary key is `(device_id, profile_id)` composite — enforces one entry per device+profile pair, no duplicates.

### BackupService
- **D-10:** `BackupService` continues to resolve credentials via `device.SSHProfileID` — no join table query added in this phase. The service is updated only to reflect the renamed type (`SSHProfile` → `CredentialProfile`) and method name changes, not its logic.
- **D-11:** All `BackupService` tests must pass after the rename + migration. No behavior changes expected.

### Claude's Discretion
- Whether to keep `ssh_profile.go` as the file (renaming only the type inside) or create `credential_profile.go` and delete the old file
- Exact handling of `idx_ssh_profiles_name` index drop/recreate order in the migration

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Domain + Schema
- `internal/domain/ssh_profile.go` — Current `SSHProfile` type and `SSHProfileRepository` interface being renamed
- `internal/domain/backup.go` — `SSHAuthMethod`, `SSHAuthPassword`, `SSHAuthKey` constants — NOT being renamed
- `internal/repository/sqlite/migrations/000005_ssh_profiles.up.sql` — Original table creation; new migration builds on this
- `internal/repository/sqlite/ssh_profile_repo.go` — Implementation being renamed to `credential_profile_repo.go`

### Service + Worker
- `internal/service/backup_service.go` — Uses `SSHProfile` type and `SSHProfileRepository`; must be updated for rename; logic unchanged
- `internal/worker/device_backup_scheduler.go` — Calls `BackupService`; verify no regressions

### API
- `internal/api/ssh_profile_handler.go` — Handler being renamed; routes may need updating
- `internal/api/router.go` — Route registrations; check for any `/ssh-profile` paths that become `/credential-profile`

### Requirements
- `.planning/REQUIREMENTS.md` §Credential Profiles — CRED-01, CRED-02, CRED-04 are the acceptance criteria for this phase

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- `internal/crypto/encrypt.go` — `Encrypt`/`Decrypt` helpers used by `BackupService.EncryptSecret` / `decryptSecret`; no changes needed
- `internal/repository/sqlite/migrations.go` — `golang-migrate` runner; adding a new numbered `.up.sql` / `.down.sql` pair is the established pattern
- `internal/domain/backup.go` — `SSHAuthMethod` type and constants remain unchanged; `BackupJob`, `BackupFile` also unaffected

### Established Patterns
- Domain interfaces live in `internal/domain/`; implementations in `internal/repository/sqlite/`; both get the rename
- Migration file naming: `000012_credential_profiles.up.sql` / `000012_credential_profiles.down.sql`
- Down migration must reverse: drop `device_credential_profiles`, drop `role` column (requires 12-step on SQLite since DROP COLUMN is limited), rename `credential_profiles` back to `ssh_profiles`
- `encrypted_secret` carries `json:"-"` tag — this annotation must be preserved on the renamed `CredentialProfile` struct

### Integration Points
- `cmd/theia/main.go` — wires `SSHProfileRepo` → `BackupService` → handlers; needs type name updates but no structural change
- `internal/api/router.go` — registers SSH profile routes; path names (`/api/v1/ssh-profiles` vs `/api/v1/credential-profiles`) should be decided before implementation — keep existing path for backwards-compat OR rename to canonical form
- `internal/service/backup_service.go:34` — `sshProfileRepo domain.SSHProfileRepository` field → becomes `credentialProfileRepo domain.CredentialProfileRepository`
- `internal/service/backup_service.go:118` — `d.SSHProfileID` check for bulk backup; unchanged in this phase

</code_context>

<specifics>
## Specific Ideas

- "Rename now" — the user wants the canonical `credential_profiles`/`CredentialProfile` naming from Phase 23 onward so all downstream phases (24–27) use clean names
- BackupService logic stays untouched; the phase is purely schema + rename + migration

</specifics>

<deferred>
## Deferred Ideas

- `is_winbox` column on `device_credential_profiles` — deferred to Phase 24's migration
- BackupService switching to join-table-based credential resolution — deferred to Phase 27 (or optionally Phase 24 when WinBox logic is added)
- API path rename (`/ssh-profiles` → `/credential-profiles`) — treat as in-scope cosmetic rename during this phase OR explicitly defer; recommend deciding before planning

</deferred>

---

*Phase: 23-credential-profile-schema-domain*
*Context gathered: 2026-04-07*
