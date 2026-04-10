# Phase 27: Schema Cleanup — Drop Legacy FK - Context

**Gathered:** 2026-04-08
**Status:** Ready for planning

<domain>
## Phase Boundary

Drop the `ssh_profile_id` FK column from the `devices` table via a SQLite 12-step table-recreation migration, then remove all Go and frontend code references to this column. The `device_credential_profiles` join table (introduced in Phase 23) is the canonical device-profile relationship — `ssh_profile_id` is fully redundant.

Requirements in scope: WINBOX-04.

</domain>

<decisions>
## Implementation Decisions

### Migration
- **D-01:** New migration `000014_drop_ssh_profile_id.up.sql` uses the SQLite 12-step table-recreation pattern to drop `ssh_profile_id` from the `devices` table. No other columns are touched.
- **D-02:** Down migration `000014_drop_ssh_profile_id.down.sql` re-adds `ssh_profile_id TEXT DEFAULT NULL` to the `devices` table (column only — data restoration is not feasible since `device_credential_profiles` is now the source of truth).

### Go Backend Cleanup — Full Removal
- **D-03:** `SSHProfileID *uuid.UUID` field is removed from the `Device` struct in `internal/domain/device.go`.
- **D-04:** All SQL references to `ssh_profile_id` in `internal/repository/sqlite/device_repo.go` are removed — from INSERT, SELECT, and UPDATE queries.
- **D-05:** Handler request types in `internal/api/device_handler.go` drop `SSHProfileID` from `createDeviceRequest`, `updateDeviceRequest`, and the batch import struct. All parsing/validation code for this field is deleted.

### Frontend Cleanup — Full Removal
- **D-06:** `ssh_profile_id?: string` is removed from the `Device` type in `frontend/src/types/api.ts`, including its attribute-mapping line in the type guard/parser.
- **D-07:** `ssh_profile_id` is removed from create/update request types and calls in `frontend/src/api/client.ts`.
- **D-08:** `BulkBackupPanel.tsx` backup eligibility check is updated: replace `if (!d.ssh_profile_id)` with a check against `credential_profiles` presence (i.e., check that the device has at least one assigned credential profile). This fixes a regression where all devices would appear unassigned once `ssh_profile_id` is absent from API responses.
- **D-09:** `AddDevicePanel.tsx` no longer sends `ssh_profile_id` in the create device payload. `SSHCredentialForm.tsx` is updated accordingly.

### Claude's Discretion
- Exact 12-step migration SQL structure (column order, constraint names)
- Down migration implementation details (whether to use ALTER TABLE ADD COLUMN or a full 12-step recreation)
- Specific credential_profiles presence check logic in BulkBackupPanel (how to query/access assigned profiles)

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Schema + Migration
- `internal/repository/sqlite/migrations/000012_credential_profiles.up.sql` — Shows the 12-step recreation pattern used for the down migration in 000012; reference for the SQLite reconstruction approach
- `internal/repository/sqlite/migrations/000013_device_credential_profiles_winbox.up.sql` — Most recent migration; 000014 follows this
- `internal/repository/sqlite/migrations.go` — Migration runner; new numbered .up.sql/.down.sql pair follows existing convention

### Go Code to Clean Up
- `internal/domain/device.go` — `Device.SSHProfileID` field to remove (line 100)
- `internal/repository/sqlite/device_repo.go` — All `ssh_profile_id` SQL references to remove (10+ occurrences)
- `internal/api/device_handler.go` — `SSHProfileID` in request structs and handler logic to remove (12+ occurrences)

### Frontend Code to Clean Up
- `frontend/src/types/api.ts` — `Device` type `ssh_profile_id` field + attribute mapper line
- `frontend/src/api/client.ts` — create/update request types that include `ssh_profile_id`
- `frontend/src/components/dashboard/BulkBackupPanel.tsx` — Backup eligibility check using `d.ssh_profile_id` → must be updated to use credential profiles
- `frontend/src/components/AddDevicePanel.tsx` — Sends `ssh_profile_id` in create payload
- `frontend/src/components/dashboard/SSHCredentialForm.tsx` — Sets `ssh_profile_id` in form payload

### Requirements
- `.planning/REQUIREMENTS.md` §WINBOX-04 — Acceptance criterion: `ssh_profile_id` absent from devices table after upgrade

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- `internal/repository/sqlite/migrations.go` — `golang-migrate` runner; adding `000014_drop_ssh_profile_id.up.sql` and `.down.sql` follows the exact same pattern as migrations 000012–000013
- `internal/repository/sqlite/migrations/000012_credential_profiles.down.sql` — Contains an existing 12-step SQLite recreation example (drop role column rollback); directly reusable as a template for the up migration

### Established Patterns
- SQLite column drop = 12-step table recreation: create temp table without the column → copy data → drop original → rename temp. The existing migrations already demonstrate this.
- Device repo uses a `scanDevice` helper that reads all columns; after cleanup it must no longer scan `ssh_profile_id`
- Handler request structs use struct tags (`json:"ssh_profile_id,omitempty"`) — removing the field removes the JSON binding entirely

### Integration Points
- `device_credential_profiles` join table (migration 000013) is the replacement for the FK; no new tables needed
- `BulkBackupPanel` eligibility logic: currently checks `device.ssh_profile_id != null` → after cleanup must check if the device has any assigned credential profiles (likely via the `credential_profiles` field already returned on the device, or a separate fetch)
- No Prometheus or WebSocket code references `ssh_profile_id`

</code_context>

<specifics>
## Specific Ideas

No specific references — standard cleanup approach.

</specifics>

<deferred>
## Deferred Ideas

None — discussion stayed within phase scope.

</deferred>

---

*Phase: 27-schema-cleanup-drop-legacy-fk*
*Context gathered: 2026-04-08*
