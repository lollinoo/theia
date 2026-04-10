# Phase 24: Backend API — Profiles, Assignments, WinBox Credentials - Context

**Gathered:** 2026-04-07
**Status:** Ready for planning

<domain>
## Phase Boundary

The backend exposes 7 new REST endpoints: per-device credential profile assignment management (list, assign, unassign), WinBox profile designation (set, clear) and credential retrieval (decrypted IP + username + password), and bridge binary download. The existing credential profile CRUD is renamed from `/ssh-profiles` to `/credential-profiles`. No frontend changes — Phase 25 handles that. No bridge binary is compiled — Phase 26 handles that.

Requirements in scope: CRED-03, CRED-05, BRIDGE-01, BRIDGE-02.

</domain>

<decisions>
## Implementation Decisions

### API Path Rename
- **D-01:** Rename all credential profile CRUD routes from `/api/v1/ssh-profiles` to `/api/v1/credential-profiles`. Update `router.go` route registrations and all `extractIDFromPath` calls in `credential_profile_handler.go` that reference the old prefix string.
- **D-02:** The `HandleTest` sub-path (`/test`) also moves to the new prefix. Frontend compatibility concern is moot — Phase 25 will rebuild the frontend using the new path.

### Schema Migration (000013)
- **D-03:** New migration `000013_device_credential_profiles_winbox.up.sql` adds `is_winbox BOOLEAN NOT NULL DEFAULT 0` to `device_credential_profiles`. This is the only schema change in Phase 24.
- **D-04:** Uniqueness of `is_winbox = 1` per device is enforced in application logic at the `PUT /winbox-profile` endpoint (not a DB-level partial unique index) — keeps migration simple and SQLite-compatible.

### Device Assignment Endpoints (3 routes)
- **D-05:** `GET /api/v1/devices/{id}/credential-profiles` — joins `device_credential_profiles` with `credential_profiles`, returns all profiles assigned to the device including `is_winbox` flag. Response omits `encrypted_secret`.
- **D-06:** `POST /api/v1/devices/{id}/credential-profiles` body `{"profile_id": "uuid"}` — inserts row into `device_credential_profiles`. Returns 409 if the (device_id, profile_id) pair already exists. Returns 404 if device or profile not found.
- **D-07:** `DELETE /api/v1/devices/{id}/credential-profiles/{profileId}` — removes row from `device_credential_profiles`. If the deleted row had `is_winbox = 1`, deletion naturally removes the WinBox designation (no extra cleanup needed). Returns 404 if not assigned.

### WinBox Designation + Credential Endpoints (3 routes)
- **D-08:** `PUT /api/v1/devices/{id}/winbox-profile` body `{"profile_id": "uuid"}` — in one transaction: sets `is_winbox = 0` for all existing rows for this device, then sets `is_winbox = 1` for the target profile row. Returns 404 if profile is not in `device_credential_profiles` for this device (must be assigned first).
- **D-09:** `DELETE /api/v1/devices/{id}/winbox-profile` — sets `is_winbox = 0` for all rows for this device. Idempotent — always returns 204 even if no WinBox profile was set.
- **D-10:** `GET /api/v1/devices/{id}/winbox-credentials` — finds the row with `is_winbox = 1` for the device, decrypts the secret, returns `{"ip": device.ip_address, "username": profile.username, "password": "<decrypted>"}`. Returns 404 `{"error": "no WinBox profile designated"}` if none set. This is the only endpoint in the system that returns a decrypted secret in the API response.

### Bridge Binary Download (1 route)
- **D-11:** `GET /api/v1/bridge/download/{os}/{arch}` — streams binary file from the configured directory. Valid `os` values: `windows`, `linux`, `darwin`. Valid `arch` values: `amd64`, `arm64`. Returns 404 JSON `{"error": "bridge binary not available for this platform"}` if file not found or directory not configured. Returns 400 JSON for unrecognized `os`/`arch` values.
- **D-12:** `bridge_binaries_dir` field added to `internal/config/config.go` Config struct and parsed from `config.yaml`. Env var override: `THEIA_BRIDGE_BINARIES_DIR`. If empty string, the download endpoint always returns 404. File naming convention: `winbox-bridge-{os}-{arch}` with `.exe` suffix for windows (e.g., `winbox-bridge-windows-amd64.exe`, `winbox-bridge-linux-arm64`).
- **D-13:** Download response headers: `Content-Disposition: attachment; filename="winbox-bridge-{os}-{arch}[.exe]"`, `Content-Type: application/octet-stream`. The endpoint bypasses the JSON Content-Type middleware (same pattern as existing backup file download routes).

### `IsInUse` Update
- **D-14:** `CredentialProfileRepo.IsInUse` updated to check `device_credential_profiles` join table (`SELECT COUNT(*) FROM device_credential_profiles WHERE profile_id = ?`) instead of the legacy `devices.ssh_profile_id` FK column. The legacy column is still present (dropped in Phase 27) but this check must use the canonical join table going forward.

### Claude's Discretion
- Where new device-assignment handlers live: extend `DeviceHandler` with sub-resource methods, extend `CredentialProfileHandler`, or create a new `DeviceCredentialProfileHandler` file — all patterns are consistent with the codebase
- Service layer: add assignment and WinBox methods to `BackupService` (which already handles credential profile CRUD) or introduce a thin `CredentialProfileService` — both are acceptable
- Optional list manifest: `GET /api/v1/bridge/list` returning available binary metadata — add if it simplifies the Phase 25 frontend; omit if the 6 well-known targets are hardcoded in the frontend

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Schema + Domain
- `internal/domain/credential_profile.go` — `CredentialProfile` type and `CredentialProfileRepository` interface (Phase 23 rename)
- `internal/repository/sqlite/migrations/000012_credential_profiles.up.sql` — Creates `device_credential_profiles` join table (Phase 23); Phase 24 migration extends this table
- `internal/domain/device.go` — `Device` type; `ip_address` field is used in the WinBox credentials response

### Repository
- `internal/repository/sqlite/credential_profile_repo.go` — Current `CredentialProfileRepo` with `IsInUse` that needs updating (D-14)
- `internal/repository/sqlite/credential_profile_repo.go` — Base for new join-table query methods (D-05 through D-09)

### API + Router
- `internal/api/credential_profile_handler.go` — Existing CRUD handler; path strings must update from `/ssh-profiles/` → `/credential-profiles/`
- `internal/api/router.go` — Route registrations; rename existing SSH profile routes; add 7 new route entries
- `internal/api/instance_backup_handler.go` — Reference pattern for binary file download (bypasses JSON middleware)

### Config
- `internal/config/config.go` — Config struct; `bridge_binaries_dir` field added here (D-12)

### Requirements
- `.planning/REQUIREMENTS.md` §Credential Profiles — CRED-03, CRED-05
- `.planning/REQUIREMENTS.md` §Bridge — BRIDGE-01, BRIDGE-02

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- `internal/crypto/encrypt.go` — `Decrypt(key, ciphertext)` — used by `BackupService` for decrypting SSH secrets; same function needed for WinBox credentials endpoint (D-10)
- `internal/api/instance_backup_handler.go` — Pattern for streaming file downloads with `Content-Disposition` and bypassing JSON middleware — directly reusable for bridge binary download (D-11, D-13)
- `writeError(w, statusCode, message)` helper — already handles all JSON error responses; use throughout new handlers

### Established Patterns
- Router uses `strings.HasSuffix` to dispatch sub-paths within a `HandleFunc` block — e.g., `if strings.HasSuffix(r.URL.Path, "/test")` in SSH profile routes; same pattern for `/winbox-profile` and `/winbox-credentials` sub-paths
- `extractIDFromPath(path, prefix)` helper extracts UUID from URL path — reuse for device-ID and profile-ID extraction in nested routes
- All handlers validate presence of body fields, return typed errors via `writeError` — maintain this pattern in new handlers
- `json.NewEncoder(w).Encode(map[string]interface{}{"data": ...})` — response envelope for all non-error responses

### Integration Points
- `internal/api/router.go:~line 330+` — Add 7 new route `HandleFunc` entries after existing credential profile routes
- `internal/api/router.go:~line 390+` — Add bridge download path to the "bypass JSON content-type" guard (same as `/download` suffix guard for backup files)
- `internal/config/config.go` — Add `BridgeBinariesDir string \`yaml:"bridge_binaries_dir"\`` field and parse `THEIA_BRIDGE_BINARIES_DIR` env override
- `cmd/theia/main.go` — Wire any new service or handler that takes `config.BridgeBinariesDir`

</code_context>

<specifics>
## Specific Ideas

- The 7 new routes match the ROADMAP description exactly ("7 new routes, per-device assignment management, WinBox credential endpoint, bridge download delivery")
- WinBox credentials endpoint returns decrypted password in plaintext — this is intentional; the app has no user auth layer, and credentials go only to the localhost bridge (not a remote server)
- Bridge download: `GET /api/v1/bridge/download/{os}/{arch}` where `{os}` is `windows`/`linux`/`darwin` and `{arch}` is `amd64`/`arm64` — 6 valid combinations total

</specifics>

<deferred>
## Deferred Ideas

- Bridge list manifest endpoint (`GET /api/v1/bridge/list`) — may be needed by Phase 25 frontend; defer to Claude's discretion during planning
- BackupService credential resolution via join table instead of `ssh_profile_id` — explicitly deferred to Phase 27 per Phase 23 D-10
- Optional `is_backup_profile` flag (CRED-F01) — out of scope for v1.5.0 per REQUIREMENTS.md

</deferred>

---

*Phase: 24-backend-api-profiles-assignments-winbox-credentials*
*Context gathered: 2026-04-07*
