# Phase 23: Credential Profile Schema + Domain - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions captured in CONTEXT.md — this log preserves the discussion.

**Date:** 2026-04-07
**Phase:** 23-credential-profile-schema-domain
**Mode:** discuss
**Areas discussed:** Naming, BackupService, Join table design

## Gray Areas Presented

### Naming
| Question | Options presented |
|----------|------------------|
| Rename `ssh_profiles` → `credential_profiles` in Phase 23? | Rename now / Keep ssh_profiles name |

### BackupService
| Question | Options presented |
|----------|------------------|
| Switch BackupService to join table in Phase 23 or keep SSHProfileID FK? | Keep using SSHProfileID FK / Switch to join table now |

### Join Table
| Question | Options presented |
|----------|------------------|
| Include `is_winbox` column in join table now or defer to Phase 24? | Defer to Phase 24 / Include is_winbox now |

## Decisions Made

### Naming
- **User chose:** Rename now
- **Rationale:** 5 downstream phases should use clean `credential_profiles`/`CredentialProfile` naming; SQLite `ALTER TABLE RENAME` makes this low-risk

### BackupService
- **User chose:** Keep using `SSHProfileID` FK (unchanged)
- **Rationale:** `devices.ssh_profile_id` stays until Phase 27; switching service logic now adds risk without benefit

### Join Table
- **User chose:** Defer `is_winbox` to Phase 24
- **Rationale:** Clean phase separation; ALTER TABLE ADD COLUMN in Phase 24 is trivial

## Scope Guardrails Applied

None — all questions stayed within Phase 23 scope.

## Prior Context Applied

- STATE.md confirmed `BackupService` must be updated in Phase 23 (interpreted as: updated for type renames, not logic changes)
- REQUIREMENTS.md CRED-F01 (explicit backup profile flag) marked as FUTURE — heuristic acceptable for now
- STATE.md confirmed `device_credential_profiles` join table approach and "Admin" migration
