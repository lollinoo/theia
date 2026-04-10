---
phase: 24
slug: backend-api-profiles-assignments-winbox-credentials
status: verified
nyquist_compliant: true
wave_0_complete: true
created: 2026-04-07
---

# Phase 24 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | go test (standard library) |
| **Config file** | none — existing Go test infrastructure |
| **Quick run command** | `go test ./internal/api/... ./internal/service/... ./internal/repository/sqlite/... -count=1 -timeout 30s` |
| **Full suite command** | `go test ./... -count=1 -timeout 120s` |
| **Estimated runtime** | ~30 seconds (quick), ~60 seconds (full) |

---

## Sampling Rate

- **After every task commit:** Run quick run command
- **After every plan wave:** Run full suite command
- **Before `/gsd-verify-work`:** Full suite must be green
- **Max feedback latency:** 60 seconds

---

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Threat Ref | Secure Behavior | Test Type | Automated Command | File | Status |
|---------|------|------|-------------|------------|-----------------|-----------|-------------------|------|--------|
| 24-01-01 | 01 | 1 | CRED-03 | T-24-01, T-24-02 | Parameterized queries; IsInUse checks canonical table | unit | `go test ./internal/repository/sqlite/... -run TestCredentialProfile -count=1` | `credential_profile_assignment_test.go` | ✅ green |
| 24-01-02 | 01 | 1 | CRED-03 | T-24-01 | Assignment methods (Assign/Unassign/SetWinbox/Clear/GetWinbox) | unit | `go test ./internal/repository/sqlite/... -run TestCredentialProfile -count=1` | `credential_profile_assignment_test.go` | ✅ green |
| 24-01-03 | 01 | 1 | CRED-03 | T-24-03 | CredentialProfile CRUD API (renamed from ssh-profiles); EncryptedSecret omitted from response | integration | `go test ./internal/api/... -run TestCredentialProfileHandler -count=1` | `credential_profile_handler_test.go` | ✅ green |
| 24-02-01 | 02 | 1 | CRED-03 | T-24-07, T-24-08, T-24-09 | Assignment/Unassign/SetWinbox endpoints; UUID validation; 400/404/409 responses | unit | `go test ./internal/api/... -run TestDeviceCredentialProfile -count=1` | `device_credential_profile_handler_test.go` | ✅ green |
| 24-02-02 | 02 | 1 | CRED-03 | T-24-05, T-24-06, T-24-10 | WinBox endpoint only returns decrypted creds when profile designated; 404 when no winbox set | unit | `go test ./internal/api/... -run "TestDeviceCredentialProfile_GetWinboxCredentials" -count=1` | `device_credential_profile_handler_test.go` | ✅ green |
| 24-03-01 | 03 | 2 | CRED-05 | T-24-11, T-24-12 | Bridge endpoint returns 400 for invalid os/arch; allowlist enforced | unit | `go test ./internal/api/... -run "TestBridgeDownload_Invalid" -count=1` | `bridge_handler_test.go` | ✅ green |
| 24-03-02 | 03 | 2 | BRIDGE-01 | T-24-11 | Bridge endpoint returns 404 JSON when dir not configured | unit | `go test ./internal/api/... -run "TestBridgeDownload_NoBinariesDir" -count=1` | `bridge_handler_test.go` | ✅ green |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

- [x] `internal/repository/sqlite/credential_profile_assignment_test.go` — assignment methods (IsWinbox, SetWinbox, ListByDevice) — 15 tests
- [x] `internal/api/device_credential_profile_handler_test.go` — assignment/winbox-profile/winbox-credentials endpoint tests — 12 tests
- [x] `internal/api/bridge_handler_test.go` — bridge download endpoint tests — 12 tests

*Existing infrastructure covers Go test runner, db helpers, and mock patterns.*

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| Bridge binary file actually downloads with correct Content-Disposition header | BRIDGE-01 | File streaming + header validation requires real file on disk | Place a test binary in the configured dir, curl the endpoint, verify filename and content |
| WinBox credential decryption returns correct plaintext password | CRED-05 | Requires live encrypted row in SQLite + correct encryption key | POST profile with known password, GET winbox-credentials, verify password matches |

---

## Validation Audit 2026-04-09

| Metric | Count |
|--------|-------|
| Gaps found | 5 (stale test commands in VALIDATION.md) |
| Resolved | 5 (commands updated to match actual test names) |
| Escalated | 0 |

All 39 tests green: 15 repo (credential_profile_assignment_test.go) + 12 handler (device_credential_profile_handler_test.go) + 12 bridge (bridge_handler_test.go).

---

## Validation Sign-Off

- [x] All tasks have `<automated>` verify or Wave 0 dependencies
- [x] Sampling continuity: no 3 consecutive tasks without automated verify
- [x] Wave 0 covers all MISSING references
- [x] No watch-mode flags
- [x] Feedback latency < 60s
- [x] `nyquist_compliant: true` set in frontmatter

**Approval:** verified 2026-04-09
