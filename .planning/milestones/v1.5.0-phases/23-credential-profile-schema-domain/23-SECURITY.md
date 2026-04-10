---
phase: 23-credential-profile-schema-domain
audited_by: gsd-security-auditor
asvs_level: 1
result: SECURED
threats_total: 6
threats_closed: 6
threats_open: 0
audited_at: 2026-04-07
---

# Security Audit — Phase 23: Credential Profile Schema + Domain

**ASVS Level:** 1
**Result:** SECURED — 6/6 threats closed, 0 open

---

## Threat Verification

| Threat ID | Category | Disposition | Status | Evidence |
|-----------|----------|-------------|--------|----------|
| T-23-01 | Information Disclosure | mitigate | CLOSED | `internal/domain/credential_profile.go:18` — `EncryptedSecret string \`json:"-"\`` present |
| T-23-02 | Tampering | mitigate | CLOSED | `internal/repository/sqlite/migrations/000012_credential_profiles.up.sql:5` — `DEFAULT 'Admin'` on role column; line 22-23 — INSERT...SELECT seeds join table from `devices WHERE ssh_profile_id IS NOT NULL` with no data fabrication |
| T-23-03 | Denial of Service | accept | CLOSED | Accepted: migration runs once on startup; ALTER TABLE RENAME and ADD COLUMN are lightweight in SQLite; INSERT...SELECT bounded by device count |
| T-23-04 | Information Disclosure | mitigate | CLOSED | `internal/api/credential_profile_handler.go:36-46` — `credentialProfileResponse` struct contains no `EncryptedSecret` field; only safe fields exposed (ID, Name, Description, Username, Port, AuthMethod, Role, CreatedAt, UpdatedAt) |
| T-23-05 | Tampering | mitigate | CLOSED | `internal/repository/sqlite/credential_profile_repo.go` — all SQL queries (INSERT line 32, SELECT by ID line 51, SELECT all line 61, UPDATE line 85, DELETE line 108, IsInUse line 123) use `?` parameterized placeholders; table name `credential_profiles` is a hardcoded string literal in all queries |
| T-23-06 | Information Disclosure | accept | CLOSED | Accepted: `backup_service.go` decrypts `EncryptedSecret` only within `runFullBackup` (line 267) for SSH connection; never returned by any service method; type rename did not alter this flow |

---

## Accepted Risks Log

| Threat ID | Category | Rationale |
|-----------|----------|-----------|
| T-23-03 | Denial of Service | Migration executes once at startup; SQLite ALTER TABLE RENAME and ADD COLUMN are O(1) metadata operations; INSERT...SELECT seeding is bounded by device table size (typically <1000 rows in target deployments). Risk level: negligible. |
| T-23-06 | Information Disclosure | BackupService decrypts EncryptedSecret only at the point of SSH connection establishment (runFullBackup, TestSSHConnection, TestCredentialProfile). The decrypted secret is a local variable that never escapes the call stack. No service method returns the plaintext or encrypted secret. |

---

## Unregistered Flags

None — neither 23-01-SUMMARY.md nor 23-02-SUMMARY.md contain a `## Threat Flags` section. All mitigation notes in key-decisions map to registered threat IDs T-23-01 and T-23-04.

---

## Verification Scope

**Plans audited:** 23-01, 23-02
**Files verified:**
- `internal/domain/credential_profile.go`
- `internal/repository/sqlite/migrations/000012_credential_profiles.up.sql`
- `internal/repository/sqlite/credential_profile_repo.go`
- `internal/api/credential_profile_handler.go`
- `internal/service/backup_service.go`
