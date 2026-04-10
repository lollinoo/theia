---
phase: 24-backend-api-profiles-assignments-winbox-credentials
plan: "03"
subsystem: backend
tags: [api, bridge, binary-download, winbox, tdd]
dependency_graph:
  requires: [24-01]
  provides: [bridge-binary-download-endpoint, BridgeHandler]
  affects: [internal/api/bridge_handler.go, internal/api/router.go]
tech_stack:
  added: []
  patterns: [TDD red-green, allowlist validation, file streaming via http.ServeFile]
key_files:
  created:
    - internal/api/bridge_handler.go
    - internal/api/bridge_handler_test.go
  modified:
    - internal/api/router.go
decisions:
  - BridgeHandler validates os/arch via allowlist maps before constructing any file path (T-24-11, T-24-12)
  - Filename pattern hardcoded as winbox-bridge-{os}-{arch}[.exe] — raw user input never touches the path
  - Method enforcement (GET-only) delegated to the router wrapper, not the handler itself
  - Bridge download bypass in router outer handler was pre-wired in Plan 01 — no additional change needed
metrics:
  duration_minutes: 1
  completed_date: "2026-04-07"
  tasks_completed: 1
  files_changed: 3
requirements:
  - BRIDGE-01
  - BRIDGE-02
---

# Phase 24 Plan 03: BridgeHandler Binary Download Summary

**One-liner:** BridgeHandler streams bridge binaries for 6 platform targets (windows/linux/darwin x amd64/arm64) with allowlist-validated path construction, correct Content-Type/Content-Disposition headers, and JSON error responses for invalid inputs.

## Tasks Completed

| # | Name | Commit | Key Files |
|---|------|--------|-----------|
| RED | Add failing TDD tests | 22593c6 | bridge_handler_test.go (new, 232 lines) |
| 1 (GREEN) | BridgeHandler implementation + router wiring | 856ab30 | bridge_handler.go (new), router.go (modified) |

## What Was Built

### BridgeHandler (`internal/api/bridge_handler.go`)

- `BridgeHandler` struct with `binariesDir string` field
- `NewBridgeHandler(binariesDir string) *BridgeHandler` constructor
- `HandleDownload(w http.ResponseWriter, r *http.Request)` method:
  - Extracts `{os}` and `{arch}` path segments from `/api/v1/bridge/download/{os}/{arch}`
  - Validates against `validOS` map (`windows`, `linux`, `darwin`) and `validArch` map (`amd64`, `arm64`)
  - Returns 400 JSON for unrecognized os or arch values
  - Returns 404 JSON when `binariesDir` is empty
  - Constructs filename: `winbox-bridge-{os}-{arch}` (+ `.exe` for windows)
  - Returns 404 JSON when file does not exist on disk
  - Sets `Content-Type: application/octet-stream` and `Content-Disposition: attachment; filename="..."`
  - Streams file via `http.ServeFile`

### Router Wiring (`internal/api/router.go`)

- Replaced `_ = bridgeBinariesDir` placeholder with `bridgeHandler := NewBridgeHandler(bridgeBinariesDir)`
- Added route: `mux.HandleFunc("/api/v1/bridge/download/", ...)` — enforces GET-only, delegates to `bridgeHandler.HandleDownload`
- Bridge download bypass in outer handler (`strings.HasPrefix(r.URL.Path, "/api/v1/bridge/download/")`) was pre-wired in Plan 01

### Tests (`internal/api/bridge_handler_test.go`, 232 lines)

8 tests covering:
1. `TestBridgeDownload_HappyPath` — linux/amd64 returns 200 + correct headers
2. `TestBridgeDownload_WindowsExe` — windows/amd64 has `.exe` suffix in Content-Disposition
3. `TestBridgeDownload_AllSixTargets` — table-driven for all 6 valid combinations
4. `TestBridgeDownload_InvalidOS` — `bados/amd64` returns 400 JSON
5. `TestBridgeDownload_InvalidArch` — `linux/x86` returns 400 JSON
6. `TestBridgeDownload_NoBinariesDir` — empty string dir returns 404 JSON
7. `TestBridgeDownload_FileNotFound` — valid os/arch but no file returns 404 JSON
8. `TestBridgeDownload_MethodNotAllowed` — POST returns 405 via router wrapper pattern

## Deviations from Plan

None — plan executed exactly as written. The bridge bypass in the outer handler was already present from Plan 01 (as expected per the plan's context). The `_ = bridgeBinariesDir` placeholder was replaced as planned.

## Threat Model Coverage

| Threat ID | Disposition | Status |
|-----------|-------------|--------|
| T-24-11 | mitigate | os/arch validated against allowlist maps before any file path construction; `filepath.Join` used |
| T-24-12 | mitigate | Filename is `winbox-bridge-{validated-os}-{validated-arch}[.exe]` — no raw user input in path |
| T-24-13 | accept | Error messages indicate platform availability — low-sensitivity operational info |
| T-24-14 | accept | `http.ServeFile` handles Content-Length and range requests; ~10MB binaries; no amplification |

## Known Stubs

None — `BridgeHandler` is fully implemented and tested.

## Threat Flags

None — `GET /api/v1/bridge/download/{os}/{arch}` is a read-only file-serving endpoint already in the plan's threat model. No new trust boundary surfaces introduced.

## Self-Check: PASSED

Files created/exist:
- FOUND: internal/api/bridge_handler.go
- FOUND: internal/api/bridge_handler_test.go

Commits verified:
- FOUND: 22593c6 (TDD RED tests)
- FOUND: 856ab30 (GREEN implementation + router wiring)

Build: exit 0
Tests: all pass (8 bridge tests + full API suite)
