---
phase: 24-backend-api-profiles-assignments-winbox-credentials
verified: 2026-04-07T20:03:30Z
status: passed
score: 5/5 must-haves verified
---

# Phase 24: Backend API — Profiles, Assignments, WinBox Credentials Verification Report

**Phase Goal:** The backend exposes full CRUD for credential profiles and per-device assignments, plus a WinBox credential endpoint and bridge binary download
**Verified:** 2026-04-07T20:03:30Z
**Status:** passed
**Re-verification:** No — initial verification

## Goal Achievement

### Observable Truths

| # | Truth | Status | Evidence |
|---|-------|--------|----------|
| 1 | User can create, read, update, and delete credential profiles via REST API | VERIFIED | `CredentialProfileHandler` at `/api/v1/credential-profiles` with HandleList, HandleCreate, HandleGet, HandleUpdate, HandleDelete — wired in router.go lines 276-302 |
| 2 | User can list all credential profiles assigned to a specific device | VERIFIED | `DeviceCredentialProfileHandler.HandleListAssignments` at `GET /api/v1/devices/{id}/credential-profiles` — returns `{"data": [...]}` with `is_winbox` field; 12 passing tests |
| 3 | User can designate exactly one profile per device as the WinBox profile via API | VERIFIED | `HandleSetWinbox` at `PUT /api/v1/devices/{id}/winbox-profile` uses `SetWinboxProfile` transaction that atomically clears all flags then sets target; `HandleClearWinbox` at `DELETE /api/v1/devices/{id}/winbox-profile` is idempotent |
| 4 | A dedicated endpoint returns the WinBox credential (IP + decrypted username/password) for a device — only when a WinBox profile is designated | VERIFIED | `HandleGetWinboxCredentials` at `GET /api/v1/devices/{id}/winbox-credentials`; calls `GetWinboxAssignment` and returns 404 if none designated; decryption in `BackupService.GetWinboxCredentials` (T-24-05) |
| 5 | Bridge binaries for all 6 targets (Windows/Linux/macOS x amd64/arm64) are downloadable from Theia Settings via the API | VERIFIED | `BridgeHandler.HandleDownload` at `GET /api/v1/bridge/download/{os}/{arch}`; validates against allowlist maps; table-driven test `TestBridgeDownload_AllSixTargets` passes for all 6 combinations |

**Score:** 5/5 truths verified

### Required Artifacts

| Artifact | Expected | Status | Details |
|----------|----------|--------|---------|
| `internal/repository/sqlite/migrations/000013_device_credential_profiles_winbox.up.sql` | is_winbox column migration | VERIFIED | Contains exactly `ALTER TABLE device_credential_profiles ADD COLUMN is_winbox BOOLEAN NOT NULL DEFAULT 0;` |
| `internal/repository/sqlite/migrations/000013_device_credential_profiles_winbox.down.sql` | Reverse migration | VERIFIED | Contains `device_credential_profiles_backup` backup-and-rename pattern |
| `internal/repository/sqlite/credential_profile_repo.go` | Assignment and WinBox repo methods | VERIFIED | All 6 methods present: `ListAssignedProfiles`, `AssignProfile`, `UnassignProfile`, `SetWinboxProfile`, `ClearWinboxProfile`, `GetWinboxAssignment`; `IsInUse` checks `device_credential_profiles` table (D-14); old `ssh_profile_id` check absent; 320 lines |
| `internal/config/config.go` | BridgeBinariesDir config field | VERIFIED | `BridgeBinariesDir string \`yaml:"bridge_binaries_dir"\`` present; `THEIA_BRIDGE_BINARIES_DIR` env override present at line 62-64 |
| `internal/api/credential_profile_handler.go` | Renamed path prefix strings | VERIFIED | All 4 `extractIDFromPath` calls use `/api/v1/credential-profiles/`; no `/api/v1/ssh-profiles/` present; error message uses "credential profile" not "SSH profile" |
| `internal/api/router.go` | Renamed route registrations | VERIFIED | `/api/v1/credential-profiles` and `/api/v1/credential-profiles/` registered; no `/api/v1/ssh-profiles` present; bridge download bypass at line 465; all 6 `deviceCredHandler` dispatch calls present |
| `internal/api/device_credential_profile_handler.go` | Handler for device assignment and WinBox endpoints | VERIFIED | 267 lines; `DeviceCredentialProfileHandler`, `NewDeviceCredentialProfileHandler`, and all 6 Handle* methods present |
| `internal/api/device_credential_profile_handler_test.go` | Tests for all 6 device assignment and WinBox endpoints | VERIFIED | 382 lines; 12 tests covering all happy paths and error cases including 409 conflict, 404 not-assigned, 400 validation, 204 idempotent clear, 422 empty password |
| `internal/service/backup_service.go` | GetWinboxCredentials method | VERIFIED | `func (s *BackupService) GetWinboxCredentials(deviceID uuid.UUID, encryptedSecret, username string) (ip, password string, err error)` at line 516; fetches device IP via repo, decrypts via internal `decryptSecret` |
| `internal/api/bridge_handler.go` | Bridge binary download handler | VERIFIED | 65 lines; `BridgeHandler`, `NewBridgeHandler`, `HandleDownload`; `application/octet-stream` Content-Type; `Content-Disposition` with filename; `validOS`/`validArch` allowlist maps; `.exe` suffix for Windows; `http.ServeFile` |
| `internal/api/bridge_handler_test.go` | Tests for bridge download endpoint | VERIFIED | 232 lines; 8 tests covering happy path, Windows .exe suffix, all 6 targets (table-driven), invalid OS, invalid arch, no binaries dir, file not found, method not allowed |

### Key Link Verification

| From | To | Via | Status | Details |
|------|-----|-----|--------|---------|
| `credential_profile_repo.go` | `000013...up.sql` | SQL queries reference `is_winbox` column | VERIFIED | `ListAssignedProfiles`, `SetWinboxProfile`, `ClearWinboxProfile`, `GetWinboxAssignment` all use `is_winbox` column |
| `credential_profile_handler.go` | `router.go` | Handler methods dispatched by `credential-profiles` path matching | VERIFIED | Router registers both `/api/v1/credential-profiles` and `/api/v1/credential-profiles/`; no ssh-profiles references in either file |
| `device_credential_profile_handler.go` | `credential_profile_repo.go` | Handler calls `credentialProfileRepo.ListAssignedProfiles` | VERIFIED | Line 53: `h.credentialProfileRepo.ListAssignedProfiles(deviceID)` |
| `device_credential_profile_handler.go` | `backup_service.go` | Handler calls `svc.GetWinboxCredentials` for decryption | VERIFIED | Line 247: `h.svc.GetWinboxCredentials(deviceID, assignment.EncryptedSecret, assignment.Username)` |
| `router.go` | `device_credential_profile_handler.go` | `deviceCredHandler` inside existing `/api/v1/devices/` block | VERIFIED | Line 45 constructs `deviceCredHandler`; lines 122-154 dispatch all 6 routes |
| `bridge_handler.go` | `config.go` | `BridgeBinariesDir` passed as constructor arg | VERIFIED | `bridgeHandler := NewBridgeHandler(bridgeBinariesDir)` at router.go line 50; `bridgeBinariesDir` parameter comes from `cfg.BridgeBinariesDir` via main.go |
| `router.go` | `bridge_handler.go` | Router dispatches `/api/v1/bridge/download/` to `BridgeHandler` | VERIFIED | router.go lines 405-411 register `mux.HandleFunc("/api/v1/bridge/download/", ...)` calling `bridgeHandler.HandleDownload`; outer handler bypass at line 465 |

### Data-Flow Trace (Level 4)

| Artifact | Data Variable | Source | Produces Real Data | Status |
|----------|---------------|--------|-------------------|--------|
| `device_credential_profile_handler.go` (HandleGetWinboxCredentials) | `assignment.EncryptedSecret` | `credentialProfileRepo.GetWinboxAssignment` → SQL JOIN with `is_winbox=1` | Yes — live SQLite query against `device_credential_profiles` and `credential_profiles` tables | FLOWING |
| `device_credential_profile_handler.go` (HandleGetWinboxCredentials) | `ip`, `password` | `BackupService.GetWinboxCredentials` → `deviceRepo.GetByID` + `decryptSecret(encryptedSecret)` | Yes — AES-GCM decryption of real stored secret; test verified round-trip via `TestDeviceCredentialProfile_GetWinboxCredentials_HappyPath` | FLOWING |
| `device_credential_profile_handler.go` (HandleListAssignments) | `rows []DeviceCredentialProfileRow` | `credentialProfileRepo.ListAssignedProfiles` → SQL JOIN ordered by `cp.name` | Yes — live SQLite query; returns empty slice (not nil) when no profiles assigned | FLOWING |
| `bridge_handler.go` (HandleDownload) | file bytes | `http.ServeFile(w, r, filePath)` | Yes — reads actual file from `binariesDir` filesystem path; returns 404 when `binariesDir` empty or file absent | FLOWING |

### Behavioral Spot-Checks

| Behavior | Command | Result | Status |
|----------|---------|--------|--------|
| `go build ./...` compiles | `go build ./...` | exit 0 | PASS |
| `TestCredentialProfile*` repo tests pass | `go test ./internal/repository/sqlite/... -run TestCredentialProfile -count=1` | 14 tests PASS, exit 0 | PASS |
| Full API test suite passes | `go test ./internal/api/... -count=1 -timeout 60s` | All tests PASS, exit 0 | PASS |
| No `ssh-profiles` references remain in api package | `grep -r "ssh-profiles" internal/api/` | Zero matches | PASS |
| Migration 000013 adds `is_winbox` via ALTER TABLE | `grep "is_winbox" migrations/000013...up.sql` | `ALTER TABLE device_credential_profiles ADD COLUMN is_winbox BOOLEAN NOT NULL DEFAULT 0;` | PASS |
| `TestDeviceCredentialProfile*` handler tests | `go test ./internal/api/... -run TestDeviceCredentialProfile -count=1` | 12 tests PASS | PASS |
| `TestBridgeDownload*` tests | `go test ./internal/api/... -run TestBridgeDownload -count=1` | 8 tests PASS (including all 6 target subtests) | PASS |

### Requirements Coverage

| Requirement | Source Plan | Description | Status | Evidence |
|-------------|------------|-------------|--------|---------|
| CRED-03 | 24-01, 24-02 | Each device can designate one profile explicitly for WinBox access | SATISFIED | `SetWinboxProfile` (transactional, exclusive), `ClearWinboxProfile` (idempotent), `GetWinboxAssignment` all implemented and tested |
| CRED-05 | 24-01, 24-02 | A device can have multiple credential profiles (one per role/purpose) | SATISFIED | `device_credential_profiles` join table with `is_winbox` column; `AssignProfile`, `UnassignProfile`, `ListAssignedProfiles` implemented; assignment handler with 409/404 error mapping |
| BRIDGE-01 | 24-03 | Local Go Bridge binary available for download from Theia Settings | SATISFIED | `BridgeHandler.HandleDownload` at `GET /api/v1/bridge/download/{os}/{arch}`; wired in router; middleware bypass for binary streaming |
| BRIDGE-02 | 24-03 | Bridge runs on Windows, Linux, and macOS (6 platform targets) | SATISFIED | `validOS` map: `{windows, linux, darwin}`; `validArch` map: `{amd64, arm64}`; `.exe` suffix for windows; `TestBridgeDownload_AllSixTargets` covers all 6 |

### Anti-Patterns Found

| File | Line | Pattern | Severity | Impact |
|------|------|---------|----------|--------|
| None | — | — | — | — |

No TODOs, FIXMEs, placeholder returns, hardcoded empty data, or stub handlers found in any implementation file. All handler methods make real repo/service calls. No `console.log`-only implementations.

### Human Verification Required

None. All success criteria are verifiable programmatically and all checks passed.

### Gaps Summary

No gaps. All 5 roadmap success criteria are satisfied:

1. CRUD for credential profiles: existing `CredentialProfileHandler` at `/api/v1/credential-profiles` (renamed from ssh-profiles).
2. List assigned profiles per device: `GET /api/v1/devices/{id}/credential-profiles` with `is_winbox` field.
3. Designate one WinBox profile: `PUT /api/v1/devices/{id}/winbox-profile` with transactional exclusive-set, `DELETE` for clear.
4. WinBox credential endpoint: `GET /api/v1/devices/{id}/winbox-credentials` returns flat `{ip, username, password}` with 404 guard and service-layer decryption.
5. Bridge binary download for 6 targets: `GET /api/v1/bridge/download/{os}/{arch}` with allowlist validation, `.exe` suffix for Windows, correct Content-Type/Content-Disposition headers.

---

_Verified: 2026-04-07T20:03:30Z_
_Verifier: Claude (gsd-verifier)_
