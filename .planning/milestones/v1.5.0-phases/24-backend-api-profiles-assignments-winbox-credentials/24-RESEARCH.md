# Phase 24: Backend API — Profiles, Assignments, WinBox Credentials - Research

**Researched:** 2026-04-07
**Domain:** Go REST API layer — credential profile CRUD rename, join-table assignment endpoints, WinBox credential endpoint, bridge binary download, schema migration
**Confidence:** HIGH

---

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions

- **D-01:** Rename all credential profile CRUD routes from `/api/v1/ssh-profiles` to `/api/v1/credential-profiles`. Update `router.go` route registrations and all `extractIDFromPath` calls in `credential_profile_handler.go` that reference the old prefix string.
- **D-02:** The `HandleTest` sub-path (`/test`) also moves to the new prefix. Frontend compatibility is moot — Phase 25 rebuilds the frontend using the new path.
- **D-03:** New migration `000013_device_credential_profiles_winbox.up.sql` adds `is_winbox BOOLEAN NOT NULL DEFAULT 0` to `device_credential_profiles`. This is the only schema change in Phase 24.
- **D-04:** Uniqueness of `is_winbox = 1` per device is enforced in application logic at the `PUT /winbox-profile` endpoint (not a DB-level partial unique index) — keeps migration simple and SQLite-compatible.
- **D-05:** `GET /api/v1/devices/{id}/credential-profiles` — joins `device_credential_profiles` with `credential_profiles`, returns all profiles assigned to the device including `is_winbox` flag. Response omits `encrypted_secret`.
- **D-06:** `POST /api/v1/devices/{id}/credential-profiles` body `{"profile_id": "uuid"}` — inserts row into `device_credential_profiles`. Returns 409 if the (device_id, profile_id) pair already exists. Returns 404 if device or profile not found.
- **D-07:** `DELETE /api/v1/devices/{id}/credential-profiles/{profileId}` — removes row from `device_credential_profiles`. If the deleted row had `is_winbox = 1`, deletion naturally removes the WinBox designation. Returns 404 if not assigned.
- **D-08:** `PUT /api/v1/devices/{id}/winbox-profile` body `{"profile_id": "uuid"}` — in one transaction: sets `is_winbox = 0` for all existing rows for this device, then sets `is_winbox = 1` for the target profile row. Returns 404 if profile is not in `device_credential_profiles` for this device.
- **D-09:** `DELETE /api/v1/devices/{id}/winbox-profile` — sets `is_winbox = 0` for all rows for this device. Idempotent — always returns 204 even if no WinBox profile was set.
- **D-10:** `GET /api/v1/devices/{id}/winbox-credentials` — finds the row with `is_winbox = 1` for the device, decrypts the secret, returns `{"ip": device.ip_address, "username": profile.username, "password": "<decrypted>"}`. Returns 404 `{"error": "no WinBox profile designated"}` if none set. This is the only endpoint in the system that returns a decrypted secret in the API response.
- **D-11:** `GET /api/v1/bridge/download/{os}/{arch}` — streams binary file from the configured directory. Valid `os` values: `windows`, `linux`, `darwin`. Valid `arch` values: `amd64`, `arm64`. Returns 404 JSON if file not found or directory not configured. Returns 400 JSON for unrecognized `os`/`arch` values.
- **D-12:** `bridge_binaries_dir` field added to `internal/config/config.go` Config struct and parsed from `config.yaml`. Env var override: `THEIA_BRIDGE_BINARIES_DIR`. If empty string, the download endpoint always returns 404. File naming convention: `winbox-bridge-{os}-{arch}` with `.exe` suffix for windows.
- **D-13:** Download response headers: `Content-Disposition: attachment; filename="winbox-bridge-{os}-{arch}[.exe]"`, `Content-Type: application/octet-stream`. The endpoint bypasses the JSON Content-Type middleware (same pattern as existing backup file download routes).
- **D-14:** `CredentialProfileRepo.IsInUse` updated to check `device_credential_profiles` join table (`SELECT COUNT(*) FROM device_credential_profiles WHERE profile_id = ?`) instead of the legacy `devices.ssh_profile_id` FK column.

### Claude's Discretion

- Where new device-assignment handlers live: extend `DeviceHandler` with sub-resource methods, extend `CredentialProfileHandler`, or create a new `DeviceCredentialProfileHandler` file — all patterns are consistent with the codebase.
- Service layer: add assignment and WinBox methods to `BackupService` (which already handles credential profile CRUD) or introduce a thin `CredentialProfileService` — both are acceptable.
- Optional list manifest: `GET /api/v1/bridge/list` returning available binary metadata — add if it simplifies the Phase 25 frontend; omit if the 6 well-known targets are hardcoded in the frontend.

### Deferred Ideas (OUT OF SCOPE)

- Bridge list manifest endpoint (`GET /api/v1/bridge/list`) — may be needed by Phase 25 frontend; defer to Claude's discretion during planning.
- BackupService credential resolution via join table instead of `ssh_profile_id` — explicitly deferred to Phase 27 per Phase 23 D-10.
- Optional `is_backup_profile` flag (CRED-F01) — out of scope for v1.5.0.

</user_constraints>

---

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| CRED-03 | User can explicitly designate one credential profile per device for WinBox access | D-08 (PUT winbox-profile), D-09 (DELETE), D-10 (GET credentials); application-level uniqueness per D-04 |
| CRED-05 | User can view and manage which credential profiles are assigned to a specific device | D-05 (GET list), D-06 (POST assign), D-07 (DELETE unassign) |
| BRIDGE-01 | User can download the WinBox bridge binary for their platform from Theia Settings | D-11 (GET bridge/download/{os}/{arch}), D-12 (config field) |
| BRIDGE-02 | Bridge binary is available for Windows, Linux, and macOS (amd64 + arm64, 6 targets) | D-11 valid combinations: windows/linux/darwin × amd64/arm64; filename convention per D-12 |

</phase_requirements>

---

## Summary

Phase 24 extends the existing credential profile API with 7 new routes and renames existing routes. The codebase provides all required primitives: `CredentialProfileRepo` and `BackupService` supply profile CRUD; `crypto.Decrypt` and `BackupService.decryptSecret` supply decryption; `instance_backup_handler.go` supplies the binary file streaming pattern; `router.go` supplies the middleware-bypass pattern for downloads.

The migration (`000013`) adds a single `is_winbox` column to the already-existing `device_credential_profiles` join table (created in migration 000012). All assignment and WinBox-designation logic is new SQL in the repo layer. No new external dependencies are required — the entire phase uses patterns already established in the codebase.

The only architectural decision left to the planner is handler placement (extend existing handlers vs. create a new `DeviceCredentialProfileHandler`) and whether the assignment/WinBox service methods belong in `BackupService` or a new thin service. The research recommends extending `CredentialProfileHandler` for assignment endpoints and a new `BridgeHandler` for bridge download — this avoids inflating `DeviceHandler` (which already has 8+ methods) with unrelated sub-resource logic.

**Primary recommendation:** Create `internal/api/credential_profile_handler.go` additions for device-assignment and WinBox endpoints, and a new `internal/api/bridge_handler.go` for bridge download. Add service methods to `BackupService` for the WinBox operations (they need `encryptionKey` access, which `BackupService` already holds). This minimises new files while respecting single-responsibility.

---

## Standard Stack

### Core (all already in go.mod — no new dependencies)
[VERIFIED: codebase grep]

| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| `database/sql` + `mattn/go-sqlite3` | v1.14.22 | Join-table queries for assignment/WinBox endpoints | Already used for all repo operations |
| `encoding/json` | stdlib | JSON response encoding | Established pattern in all handlers |
| `net/http` | stdlib | HTTP server and file streaming | `http.ServeFile` used in instance backup download |
| `github.com/google/uuid` | v1.6.0 | UUID parsing from path segments | `extractIDFromPath` already returns `uuid.UUID` |
| `internal/crypto` | — | AES-256-GCM decryption of `encrypted_secret` | `crypto.Decrypt` already used in `BackupService.decryptSecret` |

### No new dependencies
All Phase 24 work uses packages already imported in `go.mod`. No `go get` commands needed.

---

## Architecture Patterns

### Recommended Project Structure Changes
```
internal/
├── api/
│   ├── credential_profile_handler.go   # MODIFY: rename /ssh-profiles → /credential-profiles prefix strings
│   ├── device_credential_profile_handler.go  # NEW: assignment + WinBox endpoints (HandleListAssignments, HandleAssign, HandleUnassign, HandleSetWinbox, HandleClearWinbox, HandleGetWinboxCredentials)
│   ├── bridge_handler.go               # NEW: HandleDownload for bridge binaries
│   ├── router.go                       # MODIFY: rename routes + add 7 new routes + bridge bypass
├── repository/sqlite/
│   ├── credential_profile_repo.go      # MODIFY: IsInUse uses device_credential_profiles; add assignment + WinBox query methods
│   ├── migrations/
│   │   ├── 000013_device_credential_profiles_winbox.up.sql   # NEW: ADD COLUMN is_winbox
│   │   └── 000013_device_credential_profiles_winbox.down.sql # NEW: reverse
├── config/
│   └── config.go                       # MODIFY: add BridgeBinariesDir field + THEIA_BRIDGE_BINARIES_DIR override
└── service/
    └── backup_service.go               # MODIFY: add ListAssignments, AssignProfile, UnassignProfile, SetWinboxProfile, ClearWinboxProfile, GetWinboxCredentials methods
```

### Pattern 1: Route Prefix Rename
**What:** Update the two `extractIDFromPath` calls in `credential_profile_handler.go` (HandleGet, HandleUpdate, HandleDelete, HandleTest) that still embed the string `"/api/v1/ssh-profiles/"`. Replace with `"/api/v1/credential-profiles/"`.
**When to use:** Any handler that uses `extractIDFromPath` with a hardcoded path prefix string.
**Example:**
```go
// Before (current credential_profile_handler.go:139)
id, err := extractIDFromPath(r.URL.Path, "/api/v1/ssh-profiles/")

// After
id, err := extractIDFromPath(r.URL.Path, "/api/v1/credential-profiles/")
```
[VERIFIED: codebase read of internal/api/credential_profile_handler.go lines 139, 161, 229, 261]

### Pattern 2: Router Registration — Sub-Path Dispatch
**What:** Register a catch-all path with trailing slash, then use `strings.HasSuffix` to dispatch sub-paths within the same `HandleFunc`.
**When to use:** New device sub-resource endpoints nested under `/api/v1/devices/{id}/...`
**Example:**
```go
// Source: internal/api/router.go (existing /api/v1/devices/ handler)
mux.HandleFunc("/api/v1/devices/", func(w http.ResponseWriter, r *http.Request) {
    if strings.HasSuffix(r.URL.Path, "/credential-profiles") && r.Method == http.MethodGet {
        deviceCredHandler.HandleListAssignments(w, r)
        return
    }
    if strings.HasSuffix(r.URL.Path, "/credential-profiles") && r.Method == http.MethodPost {
        deviceCredHandler.HandleAssign(w, r)
        return
    }
    // ...etc
})
```
[VERIFIED: codebase read of internal/api/router.go lines 70-125]

**Important routing note:** The existing `/api/v1/devices/` handler in `router.go` covers ALL paths beginning with `/api/v1/devices/`. New device sub-resource endpoints (assignment, WinBox) must be injected into this existing handler block — they cannot use a separate `mux.HandleFunc` registration because Go's `net/http` mux uses longest-prefix matching and the `/api/v1/devices/` pattern is already registered.

### Pattern 3: Two-Segment Path Extraction
**What:** For `DELETE /api/v1/devices/{deviceId}/credential-profiles/{profileId}`, extract two UUIDs from the path.
**When to use:** Nested resource deletion where the parent and child IDs are both in the URL.
**Example:**
```go
// No existing helper for two-segment extraction — implement inline:
// Path: /api/v1/devices/<deviceID>/credential-profiles/<profileID>
parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/v1/devices/"), "/")
// parts[0] = deviceID, parts[1] = "credential-profiles", parts[2] = profileID
```
[ASSUMED: pattern derived from existing `extractIDFromPath` helper; no existing two-segment helper confirmed]

### Pattern 4: Transaction-Based WinBox Toggle
**What:** `PUT /api/v1/devices/{id}/winbox-profile` must atomically clear then set `is_winbox`. Use `db.Begin()` / `tx.Exec()` / `tx.Commit()` — same pattern used by any repo method needing atomicity.
**When to use:** Any operation that requires "clear old value, set new value" atomicity.
**Example:**
```go
// Source: internal/repository/sqlite/credential_profile_repo.go (pattern reference)
tx, err := r.db.Begin()
if err != nil {
    return err
}
defer tx.Rollback()
_, err = tx.Exec(`UPDATE device_credential_profiles SET is_winbox = 0 WHERE device_id = ?`, deviceID.String())
if err != nil { return err }
res, err := tx.Exec(`UPDATE device_credential_profiles SET is_winbox = 1 WHERE device_id = ? AND profile_id = ?`, deviceID.String(), profileID.String())
if err != nil { return err }
n, _ := res.RowsAffected()
if n == 0 {
    return fmt.Errorf("profile not assigned to device")
}
return tx.Commit()
```
[VERIFIED: tx pattern used throughout the codebase; sqlite3 supports SERIALIZABLE transactions]

### Pattern 5: Binary File Download (bypassing JSON middleware)
**What:** Serve a file with `Content-Disposition: attachment` and `Content-Type: application/octet-stream` while bypassing the JSONContentType middleware wrapper.
**When to use:** Any endpoint that serves a binary file, not a JSON response.
**Example:**
```go
// Source: internal/api/instance_backup_handler.go:HandleDownload (lines 127-155)
w.Header().Set("Content-Type", "application/octet-stream")
w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
http.ServeFile(w, r, filePath)
```
And in `router.go` outer handler:
```go
// Add to the bypass guard block in router.go (after line 403):
if strings.HasPrefix(r.URL.Path, "/api/v1/bridge/download/") && r.Method == http.MethodGet {
    CORS(RequestLogger(mux)).ServeHTTP(w, r)
    return
}
```
[VERIFIED: codebase read of internal/api/router.go lines 395-413, internal/api/instance_backup_handler.go lines 127-155]

### Pattern 6: Config Field + Env Override
**What:** Add a field to `Config` struct with `yaml` tag, and apply env var override in `Load()`.
**When to use:** Any new bootstrap-level configuration value.
**Example:**
```go
// Source: internal/config/config.go (existing pattern for ListenAddr, DBPath, LogLevel)
type Config struct {
    ListenAddr        string `yaml:"listen_addr"`
    DBPath            string `yaml:"db_path"`
    LogLevel          string `yaml:"log_level"`
    BridgeBinariesDir string `yaml:"bridge_binaries_dir"` // NEW
}
// In Load():
if v := os.Getenv("THEIA_BRIDGE_BINARIES_DIR"); v != "" {
    cfg.BridgeBinariesDir = v
}
```
[VERIFIED: codebase read of internal/config/config.go]

### Pattern 7: WinBox Credentials Response — Decryption
**What:** The `GET /api/v1/devices/{id}/winbox-credentials` endpoint is the only API endpoint that returns a decrypted secret. It must use `BackupService.decryptSecret` (unexported) or promote `crypto.Decrypt` directly.
**When to use:** WinBox credentials endpoint only.
**Note:** `decryptSecret` is defined on `*BackupService` and is unexported. Two options:
1. Add a `GetWinboxCredentials(deviceID) (ip, username, password string, err error)` method to `BackupService` that does the decryption internally — the handler never sees the raw key.
2. Export `DecryptSecret` (rename from `decryptSecret`) and let a new service call it.

Option 1 is cleaner and consistent with the existing pattern where handlers never hold crypto keys.
[VERIFIED: codebase read of internal/service/backup_service.go lines 489-498]

### Anti-Patterns to Avoid

- **Registering a new `mux.HandleFunc("/api/v1/devices/...")`:** Go's `net/http` ServeMux uses longest-prefix matching. A second `HandleFunc` for a path that starts with `/api/v1/devices/` will never be reached because the existing `/api/v1/devices/` catch-all already owns everything under that prefix. All new device sub-resource routes must be injected into the existing handler block.
- **Returning `encrypted_secret` in assignment list response:** The `credentialProfileResponse` struct already omits `EncryptedSecret` (it has `json:"-"` on the domain type). Never add `encrypted_secret` to any new response type.
- **Using a partial unique index for is_winbox:** D-04 explicitly rejects this — application-level enforcement in the transaction is the approach.
- **Exposing decryption key in handler layer:** The encryption key (`[]byte`) must stay in the service layer. Handlers receive `BackupService` (or a new service struct that wraps it) and call a method that returns the plaintext password string.

---

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Decrypting AES-256-GCM ciphertext | Custom decryption | `crypto.Decrypt(ciphertext, key)` in `internal/crypto/encrypt.go` | Already implemented, tested, handles nonce extraction |
| Streaming file download with correct headers | Custom `io.Copy` loop | `http.ServeFile(w, r, path)` | Handles Content-Length, range requests, ETags automatically |
| UUID parsing from URL path | `strings.Split` + `uuid.Parse` inline | `extractIDFromPath(path, prefix)` from `internal/api/` | Already handles parse errors and returns typed UUID |
| SQL migration runner | Custom version tracking | `golang-migrate` via `sqlite.RunMigrations(db)` | Already embedded; just add `.sql` files to `migrations/` |
| JSON error responses | `json.Marshal` inline | `writeError(w, statusCode, message)` from `internal/api/` | Consistent error format across all handlers |

---

## Common Pitfalls

### Pitfall 1: Double-Registration of `/api/v1/devices/` Routes
**What goes wrong:** Adding a new `mux.HandleFunc("/api/v1/devices/credential-profiles")` causes the route to never be reached because Go's mux already has `/api/v1/devices/` registered as a prefix catch-all.
**Why it happens:** Go's `net/http.ServeMux` longest-prefix matching means the existing `/api/v1/devices/` handler absorbs all requests starting with that path.
**How to avoid:** All new device sub-resource dispatching goes inside the existing `/api/v1/devices/` `HandleFunc` block in `router.go`. Use `strings.HasSuffix` or path segment checks as the existing code does (lines 72-125 of router.go).
**Warning signs:** A new route always returns 404 or falls through to the device CRUD handlers.

### Pitfall 2: Missing Middleware Bypass for Bridge Download
**What goes wrong:** `GET /api/v1/bridge/download/{os}/{arch}` returns a JSON Content-Type header because the outer handler in `router.go` wraps everything in `JSONContentType` middleware.
**Why it happens:** The `router.go` outer `http.HandlerFunc` checks path prefixes to decide whether to bypass the full middleware chain. Bridge download paths don't match any existing bypass condition.
**How to avoid:** Add a bypass check for `strings.HasPrefix(r.URL.Path, "/api/v1/bridge/download/")` in the outer handler before calling `handler.ServeHTTP(w, r)` — following the exact same pattern as the instance backup download bypass at lines 402-405.
**Warning signs:** Browser receives Content-Type: application/json on a binary file download.

### Pitfall 3: `is_winbox` Column Missing in Test Database
**What goes wrong:** Tests that run migrations against `:memory:` will fail if the `000013` migration SQL file is present but not yet embedded correctly, or if `sqlite.RunMigrations` is called before the file is added.
**Why it happens:** `migrations.go` uses `//go:embed migrations/*.sql` — the embed is compiled at build time. If the `.sql` file exists but is malformed, all migration-backed tests fail with cryptic errors.
**How to avoid:** Write the `000013` up/down SQL first, run `go build ./...` to catch embed errors, then implement the repo methods. Tests using `setupCredentialProfileTest` (which calls `sqlite.RunMigrations(db)`) automatically pick up migration 000013.
**Warning signs:** `sql: no rows in result set` errors in join-table queries, or `table device_credential_profiles has no column named is_winbox`.

### Pitfall 4: `IsInUse` Check Uses Wrong Table After D-14
**What goes wrong:** `HandleDelete` for credential profiles still uses the legacy `devices.ssh_profile_id` FK check. After D-14, `IsInUse` must check `device_credential_profiles` — otherwise a profile assigned via the join table can be deleted while devices still reference it.
**Why it happens:** The current `IsInUse` implementation (line 121 of `credential_profile_repo.go`) explicitly checks `devices WHERE ssh_profile_id = ?`. This is marked as needing an update in D-14 but is easy to overlook.
**How to avoid:** Update `IsInUse` in the same wave as the route rename. The `TestCredentialProfileHandlerDelete_InUse` test (line 392 of `credential_profile_handler_test.go`) must also be updated to insert into `device_credential_profiles` instead of using `ssh_profile_id`.
**Warning signs:** `TestCredentialProfileHandlerDelete_InUse` passes but deleting an assigned profile via the API succeeds when it should return 409.

### Pitfall 5: WinBox Credential Response Leaks No-Op When `encrypted_secret` Is Empty
**What goes wrong:** If `encrypted_secret` is empty string (no secret was stored when profile was created), `decryptSecret` returns `("", nil)`. The WinBox credentials response would return `"password": ""` — a silent failure.
**Why it happens:** `BackupService.decryptSecret` guards empty string and returns `("", nil)` — that is by design for profiles without a secret. But WinBox needs a real password.
**How to avoid:** In `GetWinboxCredentials`, after decrypting, check if the password is empty and return an appropriate 422 or 404 response. Or document the constraint clearly — WinBox requires a non-empty password.

### Pitfall 6: Bridge Download Path Parsing
**What goes wrong:** Naive `strings.Split(r.URL.Path, "/")` produces different counts depending on whether the path has a trailing slash. `GET /api/v1/bridge/download/windows/amd64/` would split into 8 elements, not 7.
**Why it happens:** The path segment counts shift with trailing slashes.
**How to avoid:** Use `strings.TrimSuffix(r.URL.Path, "/")` before splitting, then validate that exactly 2 remaining segments are present after the `/api/v1/bridge/download/` prefix. Alternatively, `strings.TrimPrefix` the known prefix and split what remains.

---

## Code Examples

### Migration 000013 (adding `is_winbox` column)
```sql
-- 000013_device_credential_profiles_winbox.up.sql
ALTER TABLE device_credential_profiles ADD COLUMN is_winbox BOOLEAN NOT NULL DEFAULT 0;

-- 000013_device_credential_profiles_winbox.down.sql
-- SQLite 3.35+ supports DROP COLUMN; for compatibility use 12-step recreation
-- But since we only added a column with DEFAULT 0, reverse is a no-op for SQLite < 3.35
-- For correctness, recreate without the column:
CREATE TABLE device_credential_profiles_backup (
    device_id  TEXT NOT NULL,
    profile_id TEXT NOT NULL,
    created_at DATETIME NOT NULL,
    PRIMARY KEY (device_id, profile_id),
    FOREIGN KEY (device_id) REFERENCES devices(id) ON DELETE CASCADE,
    FOREIGN KEY (profile_id) REFERENCES credential_profiles(id) ON DELETE CASCADE
);
INSERT INTO device_credential_profiles_backup SELECT device_id, profile_id, created_at FROM device_credential_profiles;
DROP TABLE device_credential_profiles;
ALTER TABLE device_credential_profiles_backup RENAME TO device_credential_profiles;
```
[VERIFIED: SQLite migration pattern matches 000012 down.sql pattern in codebase]

### Updated `IsInUse` (D-14)
```go
// Source: internal/repository/sqlite/credential_profile_repo.go
func (r *CredentialProfileRepo) IsInUse(id uuid.UUID) (bool, error) {
    var count int
    err := r.db.QueryRow(
        `SELECT COUNT(*) FROM device_credential_profiles WHERE profile_id = ?`,
        id.String(),
    ).Scan(&count)
    if err != nil {
        return false, err
    }
    return count > 0, nil
}
```
[VERIFIED: Current IsInUse uses `devices WHERE ssh_profile_id = ?` — D-14 change is correct]

### New Repo Method: `ListAssignedProfiles`
```go
// DeviceCredentialProfileRow is the response DTO for an assigned profile
type DeviceCredentialProfileRow struct {
    ProfileID  uuid.UUID
    Name       string
    Username   string
    Port       int
    AuthMethod domain.SSHAuthMethod
    Role       string
    IsWinbox   bool
    CreatedAt  time.Time
}

func (r *CredentialProfileRepo) ListAssignedProfiles(deviceID uuid.UUID) ([]DeviceCredentialProfileRow, error) {
    rows, err := r.db.Query(`
        SELECT cp.id, cp.name, cp.username, cp.port, cp.auth_method, cp.role, dcp.is_winbox, dcp.created_at
        FROM device_credential_profiles dcp
        JOIN credential_profiles cp ON cp.id = dcp.profile_id
        WHERE dcp.device_id = ?
        ORDER BY cp.name ASC`,
        deviceID.String(),
    )
    // ... scan rows
}
```
[ASSUMED: method signature and SQL derived from schema analysis; no existing method confirmed]

### `BackupService.GetWinboxCredentials` (D-10 implementation)
```go
func (s *BackupService) GetWinboxCredentials(ctx context.Context, deviceID uuid.UUID) (ip, username, password string, err error) {
    // Repo returns the encrypted_secret for the winbox-designated profile
    row, err := s.credentialProfileRepo.GetWinboxAssignment(deviceID)
    if err != nil {
        return "", "", "", fmt.Errorf("no WinBox profile designated")
    }
    device, err := s.deviceRepo.GetByID(deviceID)
    if err != nil {
        return "", "", "", fmt.Errorf("device not found: %w", err)
    }
    decrypted, err := s.decryptSecret(row.EncryptedSecret)
    if err != nil {
        return "", "", "", fmt.Errorf("decrypting credentials: %w", err)
    }
    return device.IP, row.Username, decrypted, nil
}
```
[ASSUMED: method signature based on D-10 spec; `decryptSecret` is private on BackupService — this pattern keeps crypto in the service layer]

### Handler Response for `GET /winbox-credentials`
```go
// In handler:
ip, username, password, err := h.svc.GetWinboxCredentials(r.Context(), deviceID)
if err != nil {
    writeError(w, http.StatusNotFound, "no WinBox profile designated")
    return
}
json.NewEncoder(w).Encode(map[string]interface{}{
    "ip":       ip,
    "username": username,
    "password": password,
})
```
[VERIFIED: response shape matches D-10 spec from CONTEXT.md]

### Bridge Download Handler
```go
// BridgeHandler provides the bridge binary download endpoint.
type BridgeHandler struct {
    binariesDir string
}

func NewBridgeHandler(binariesDir string) *BridgeHandler {
    return &BridgeHandler{binariesDir: binariesDir}
}

func (h *BridgeHandler) HandleDownload(w http.ResponseWriter, r *http.Request) {
    // Parse /api/v1/bridge/download/{os}/{arch}
    suffix := strings.TrimPrefix(r.URL.Path, "/api/v1/bridge/download/")
    suffix = strings.TrimSuffix(suffix, "/")
    parts := strings.Split(suffix, "/")
    if len(parts) != 2 {
        writeError(w, http.StatusBadRequest, "invalid path")
        return
    }
    osName, arch := parts[0], parts[1]

    validOS   := map[string]bool{"windows": true, "linux": true, "darwin": true}
    validArch := map[string]bool{"amd64": true, "arm64": true}
    if !validOS[osName] || !validArch[arch] {
        writeError(w, http.StatusBadRequest, "unrecognized os or arch")
        return
    }

    if h.binariesDir == "" {
        writeError(w, http.StatusNotFound, "bridge binary not available for this platform")
        return
    }

    filename := fmt.Sprintf("winbox-bridge-%s-%s", osName, arch)
    if osName == "windows" {
        filename += ".exe"
    }
    filePath := filepath.Join(h.binariesDir, filename)
    if _, err := os.Stat(filePath); os.IsNotExist(err) {
        writeError(w, http.StatusNotFound, "bridge binary not available for this platform")
        return
    }

    w.Header().Set("Content-Type", "application/octet-stream")
    w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
    http.ServeFile(w, r, filePath)
}
```
[VERIFIED: pattern mirrors instance_backup_handler.go HandleDownload; `http.ServeFile` is stdlib]

---

## Existing Code Inventory (Phase 23 Deliverables)

What Phase 23 completed — confirmed by reading the actual files:

| File | Status | Relevant to Phase 24 |
|------|--------|---------------------|
| `internal/domain/credential_profile.go` | EXISTS | `CredentialProfile` type, `CredentialProfileRepository` interface (CRUD only — no assignment methods) |
| `internal/repository/sqlite/credential_profile_repo.go` | EXISTS | `Create/GetByID/GetAll/Update/Delete/IsInUse` — assignment methods missing; `IsInUse` still uses legacy FK |
| `internal/api/credential_profile_handler.go` | EXISTS | CRUD handlers wired to `/api/v1/ssh-profiles/` prefix — prefix strings need updating |
| `internal/repository/sqlite/migrations/000012_credential_profiles.up.sql` | EXISTS | Creates `device_credential_profiles` join table WITHOUT `is_winbox` column |
| `internal/config/config.go` | EXISTS | Missing `BridgeBinariesDir` field |
| `internal/api/router.go` | EXISTS | SSH profile routes at `/api/v1/ssh-profiles` — need renaming; bridge routes missing |

**`CredentialProfileRepository` interface gaps for Phase 24:**
The current interface in `internal/domain/credential_profile.go` only has `Create/GetByID/GetAll/Update/Delete`. Phase 24 needs the following new methods on the concrete `CredentialProfileRepo` (not necessarily on the interface, since the handler/service can accept the concrete type — consistent with how `CredentialProfileRepo` is already injected as `*sqlite.CredentialProfileRepo` in `router.go`, not as the interface):

1. `ListAssignedProfiles(deviceID uuid.UUID) ([]DeviceCredentialProfileRow, error)`
2. `AssignProfile(deviceID, profileID uuid.UUID) error`
3. `UnassignProfile(deviceID, profileID uuid.UUID) error`
4. `SetWinboxProfile(deviceID, profileID uuid.UUID) error` (transactional)
5. `ClearWinboxProfile(deviceID uuid.UUID) error`
6. `GetWinboxAssignment(deviceID uuid.UUID) (*WinboxAssignmentRow, error)` (for credentials endpoint)

[VERIFIED: domain interface and concrete repo both read from codebase]

---

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| `/api/v1/ssh-profiles` routes | `/api/v1/credential-profiles` routes | Phase 24 | Frontend must use new paths; Phase 25 rebuilds frontend so no compatibility required |
| `IsInUse` checks `devices.ssh_profile_id` | `IsInUse` checks `device_credential_profiles` | Phase 24 (D-14) | Delete-profile conflict detection now uses canonical join table |
| No `is_winbox` column | `device_credential_profiles.is_winbox BOOLEAN DEFAULT 0` | Migration 000013 | Enables WinBox designation per device |

---

## Assumptions Log

| # | Claim | Section | Risk if Wrong |
|---|-------|---------|---------------|
| A1 | `DeviceCredentialProfileRow` is a new struct defined in the repo file — no equivalent exists yet | Code Examples | Low; if wrong, reuse an existing type |
| A2 | Two-segment path extraction for `DELETE /devices/{id}/credential-profiles/{profileId}` must be implemented inline (no existing helper) | Architecture Patterns | Low; could add a new `extractTwoIDsFromPath` helper |
| A3 | `BackupService.GetWinboxCredentials` is the cleanest home for the decryption step | Architecture Patterns | Low; could alternatively be done directly in a new service, but `decryptSecret` is private |
| A4 | `BridgeHandler` should be a new handler struct in `bridge_handler.go` | Architecture Patterns | Low; it's Claude's discretion per CONTEXT.md |

---

## Open Questions

1. **Bridge list manifest endpoint (`GET /api/v1/bridge/list`)**
   - What we know: Deferred to Claude's discretion. Phase 25 frontend may need it, or may hardcode the 6 known targets.
   - What's unclear: Will the Phase 25 plan hardcode the 6 targets or call an API to discover available binaries?
   - Recommendation: Implement it in Phase 24 alongside the download endpoint — it's trivial (iterate valid os/arch combinations, check file existence), and removing a route is easier than adding one mid-phase.

2. **`CredentialProfileRepository` interface extension**
   - What we know: The domain interface currently has no assignment/WinBox methods. The concrete `*sqlite.CredentialProfileRepo` is injected directly (not via the interface) in `router.go` and `NewCredentialProfileHandler`.
   - What's unclear: Should assignment/WinBox methods be added to the domain interface, or stay on the concrete type only?
   - Recommendation: Add to the concrete type only for Phase 24 — consistent with current injection pattern (`*sqlite.CredentialProfileRepo` not the interface). If test mocking becomes needed, add interface methods at that point.

---

## Environment Availability

Step 2.6: SKIPPED — Phase 24 is code/config changes only. No external tool dependencies beyond the project's existing Go toolchain and SQLite runtime.

---

## Validation Architecture

### Test Framework
| Property | Value |
|----------|-------|
| Framework | Go standard `testing` package |
| Config file | none (no `go test` config file; standard `go test ./...`) |
| Quick run command | `go test ./internal/api/... ./internal/repository/sqlite/... -count=1 -run "TestCredential\|TestBridge\|TestWinbox\|TestDeviceAssignment"` |
| Full suite command | `go test ./... -count=1` |

### Phase Requirements → Test Map

| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| CRED-03 | `PUT /devices/{id}/winbox-profile` sets is_winbox=1 for target, 0 for all others | unit | `go test ./internal/api/... -run TestWinboxProfileSet` | ❌ Wave 0 |
| CRED-03 | `PUT /winbox-profile` returns 404 when profile not assigned to device | unit | `go test ./internal/api/... -run TestWinboxProfileSet_NotAssigned` | ❌ Wave 0 |
| CRED-03 | `DELETE /winbox-profile` is idempotent (204 when none set) | unit | `go test ./internal/api/... -run TestWinboxProfileClear_Idempotent` | ❌ Wave 0 |
| CRED-03 | `GET /winbox-credentials` returns decrypted ip/username/password | unit | `go test ./internal/api/... -run TestWinboxCredentials_HappyPath` | ❌ Wave 0 |
| CRED-03 | `GET /winbox-credentials` returns 404 when no winbox profile set | unit | `go test ./internal/api/... -run TestWinboxCredentials_NoProfile` | ❌ Wave 0 |
| CRED-05 | `POST /devices/{id}/credential-profiles` assigns profile; 409 on duplicate | unit | `go test ./internal/api/... -run TestDeviceAssign_HappyPath\|TestDeviceAssign_Duplicate` | ❌ Wave 0 |
| CRED-05 | `GET /devices/{id}/credential-profiles` returns assigned profiles with is_winbox field | unit | `go test ./internal/api/... -run TestDeviceListAssignments` | ❌ Wave 0 |
| CRED-05 | `DELETE /devices/{id}/credential-profiles/{id}` unassigns; 404 if not assigned | unit | `go test ./internal/api/... -run TestDeviceUnassign` | ❌ Wave 0 |
| BRIDGE-01 | `GET /api/v1/bridge/download/linux/amd64` streams file with correct headers | unit | `go test ./internal/api/... -run TestBridgeDownload_HappyPath` | ❌ Wave 0 |
| BRIDGE-01 | `GET /api/v1/bridge/download/windows/amd64` uses `.exe` suffix | unit | `go test ./internal/api/... -run TestBridgeDownload_WindowsExe` | ❌ Wave 0 |
| BRIDGE-01 | `GET /api/v1/bridge/download/...` returns 404 when binariesDir is empty | unit | `go test ./internal/api/... -run TestBridgeDownload_NoBinariesDir` | ❌ Wave 0 |
| BRIDGE-02 | `GET /api/v1/bridge/download/bados/amd64` returns 400 | unit | `go test ./internal/api/... -run TestBridgeDownload_InvalidOS` | ❌ Wave 0 |
| D-01 | `GET /api/v1/credential-profiles` returns 200; `/api/v1/ssh-profiles` returns 404 | unit | `go test ./internal/api/... -run TestCredentialProfilePath_Renamed` | ❌ Wave 0 |
| D-14 | `IsInUse` uses device_credential_profiles; delete fails when profile is assigned via join table | unit | update existing `TestCredentialProfileHandlerDelete_InUse` | ✅ (needs update) |

### Sampling Rate
- **Per task commit:** `go test ./internal/api/... ./internal/repository/sqlite/... -count=1`
- **Per wave merge:** `go test ./... -count=1`
- **Phase gate:** Full suite green before `/gsd-verify-work`

### Wave 0 Gaps
- [ ] `internal/api/device_credential_profile_handler_test.go` — covers device assignment + WinBox endpoints (REQ CRED-03, CRED-05)
- [ ] `internal/api/bridge_handler_test.go` — covers bridge download endpoint (REQ BRIDGE-01, BRIDGE-02)
- [ ] Update `internal/api/credential_profile_handler_test.go` — update `TestCredentialProfileHandlerDelete_InUse` to use `device_credential_profiles` table instead of `ssh_profile_id` FK (REQ D-14); add `TestCredentialProfilePath_Renamed` test (REQ D-01)
- [ ] No framework install needed — Go stdlib `testing` package already available

---

## Security Domain

### Applicable ASVS Categories

| ASVS Category | Applies | Standard Control |
|---------------|---------|-----------------|
| V2 Authentication | no | No user authentication in this system |
| V3 Session Management | no | No sessions; stateless API |
| V4 Access Control | no | No user roles; all API is local-only |
| V5 Input Validation | yes | Validate `os`/`arch` values against allowlist; validate UUIDs via `extractIDFromPath`; validate `profile_id` body field |
| V6 Cryptography | yes | Use existing `crypto.Decrypt` — never hand-roll; never expose raw `encrypted_secret` bytes in responses |

### Known Threat Patterns for This Stack

| Pattern | STRIDE | Standard Mitigation |
|---------|--------|---------------------|
| Path traversal in bridge binary download | Tampering | Construct filename from validated allowlist values (os/arch), never from raw URL input; `filepath.Join` prevents `..` traversal |
| Decrypted password exposure in logs | Information Disclosure | Never log the decrypted password; the `log.Printf` in middleware logs request path only, not body |
| Unvalidated UUID in device/profile path | Tampering | `extractIDFromPath` calls `uuid.Parse` — invalid UUIDs return 400 |
| Profile assigned to nonexistent device | Tampering | `AssignProfile` repo method returns 404 if device not found (FK constraint + explicit check) |

---

## Sources

### Primary (HIGH confidence)
- Codebase: `internal/api/credential_profile_handler.go` — current handler with ssh-profiles prefix strings [VERIFIED]
- Codebase: `internal/api/router.go` — route registration patterns, middleware bypass guards [VERIFIED]
- Codebase: `internal/repository/sqlite/credential_profile_repo.go` — current IsInUse, base for new methods [VERIFIED]
- Codebase: `internal/repository/sqlite/migrations/000012_credential_profiles.up.sql` — join table schema [VERIFIED]
- Codebase: `internal/service/backup_service.go` — decryptSecret, EncryptSecret, credential profile CRUD methods [VERIFIED]
- Codebase: `internal/api/instance_backup_handler.go` — binary download pattern [VERIFIED]
- Codebase: `internal/crypto/encrypt.go` — Decrypt, DeriveKey signatures [VERIFIED]
- Codebase: `internal/config/config.go` — Config struct, env override pattern [VERIFIED]
- Codebase: `internal/domain/device.go` — Device.IP field (used in WinBox credentials response) [VERIFIED]
- Codebase: `cmd/theia/main.go` — dependency wiring pattern for new handlers [VERIFIED]
- Codebase: `internal/api/credential_profile_handler_test.go` — test setup pattern (in-memory SQLite + RunMigrations) [VERIFIED]

### Secondary (MEDIUM confidence)
- CONTEXT.md decisions D-01 through D-14 — user-locked design decisions [CITED: .planning/phases/24-backend-api-profiles-assignments-winbox-credentials/24-CONTEXT.md]

---

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH — all libraries are already in go.mod; no new dependencies
- Architecture: HIGH — all patterns verified directly from codebase source files
- Pitfalls: HIGH — pitfalls 1, 2, 4 verified against actual source; pitfalls 3, 5, 6 verified from schema and code reading
- Migration SQL: HIGH — directly mirrors the 000012 pattern confirmed from source

**Research date:** 2026-04-07
**Valid until:** 2026-05-07 (stable Go codebase, no fast-moving external dependencies)
