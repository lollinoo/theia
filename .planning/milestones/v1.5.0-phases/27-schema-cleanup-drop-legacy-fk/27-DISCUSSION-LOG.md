# Phase 27: Schema Cleanup — Drop Legacy FK - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions captured in CONTEXT.md — this log preserves the discussion.

**Date:** 2026-04-08
**Phase:** 27-schema-cleanup-drop-legacy-fk
**Mode:** discuss

## Gray Areas Presented

| Area | Description |
|------|-------------|
| Go code cleanup depth | How far to take cleanup beyond the DB migration |
| Frontend scope | Whether to include frontend ssh_profile_id cleanup in this phase |

## Decisions Made

### Go Code Cleanup Depth
- **Question:** After the migration drops ssh_profile_id from the DB, how far should Go code cleanup go?
- **Answer:** Full removal — remove SSHProfileID from Device domain struct, all SQL queries in device_repo.go, and all handler request/response types in device_handler.go.
- **Rationale:** device_credential_profiles is the canonical approach; no dead code left behind.

### Frontend Scope
- **Question:** Frontend still uses ssh_profile_id in several places. BulkBackupPanel checks d.ssh_profile_id to determine backup eligibility — once the field is absent from API responses, all devices would appear unassigned and backup would stop working. Should Phase 27 include frontend cleanup?
- **Answer:** Yes, include frontend cleanup. BulkBackupPanel eligibility check updated to use credential_profiles presence.
- **Rationale:** Not including this would cause a backup regression on upgrade.

## No Corrections
All decisions confirmed as presented.
