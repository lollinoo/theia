---
phase: 27
slug: schema-cleanup-drop-legacy-fk
status: compliant
nyquist_compliant: true
wave_0_complete: false
created: 2026-04-09
---

# Phase 27 — Validation Strategy

> Per-phase validation contract for schema-cleanup-drop-legacy-fk.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework (Go)** | standard `testing` package |
| **Framework (Frontend)** | Vitest 4.1 + @testing-library/react |
| **Go config** | none (standard Go modules) |
| **Frontend config** | `frontend/vitest.config.ts` |
| **Quick run (Go)** | `PATH=$PATH:/usr/local/go/bin go test ./internal/repository/sqlite/... ./internal/service/...` |
| **Quick run (Frontend)** | `cd frontend && npm run test -- --run SSHCredentialForm` |
| **Full suite (Go)** | `PATH=$PATH:/usr/local/go/bin go test ./internal/...` |
| **Full suite (Frontend)** | `cd frontend && npm run test -- --run` |

---

## Sampling Rate

- **After every task commit:** Run quick run commands above
- **After every plan wave:** Run full suite
- **Before `/gsd-verify-work`:** Full suite must be green
- **Max feedback latency:** ~30 seconds (Go) / ~60 seconds (frontend)

---

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Threat Ref | Secure Behavior | Test Type | Automated Command | File | Status |
|---------|------|------|-------------|------------|-----------------|-----------|-------------------|------|--------|
| 27-01-01 | 01 | 1 | WINBOX-04 | T-27-01 | Migration wraps table recreation in PRAGMA foreign_keys=off/on | integration | `go test ./internal/repository/sqlite/... -run TestMigration000014_DevicesColumnDropped` | `migrations_test.go` | ✅ green |
| 27-01-02 | 01 | 1 | WINBOX-04 | T-27-02 | All device rows survive migration with every field intact | integration | `go test ./internal/repository/sqlite/... -run TestMigration000014_DeviceDataIntegrity` | `migrations_test.go` | ✅ green |
| 27-01-03 | 01 | 1 | WINBOX-04 | — | verifyLegacyTablesMigrated skips safely on post-000014 DBs | integration | `go test ./internal/repository/sqlite/... -run TestVerifyLegacyTablesMigrated_PostMigration000014` | `migrations_test.go` | ✅ green |
| 27-01-04 | 01 | 1 | WINBOX-04 | T-27-03 | GetBackupProfileForDevice returns error when no profile assigned | integration | `go test ./internal/repository/sqlite/... -run TestGetBackupProfileForDevice_NoProfileAssigned` | `credential_profile_assignment_test.go` | ✅ green |
| 27-01-05 | 01 | 1 | WINBOX-04 | T-27-03 | GetBackupProfileForDevice returns assigned profile | integration | `go test ./internal/repository/sqlite/... -run TestGetBackupProfileForDevice_ReturnsProfile` | `credential_profile_assignment_test.go` | ✅ green |
| 27-01-06 | 01 | 1 | WINBOX-04 | T-27-04 | GetBackupProfileForDevice prefers non-WinBox profile (ORDER BY is_winbox ASC) | integration | `go test ./internal/repository/sqlite/... -run TestGetBackupProfileForDevice_PrefersNonWinBox` | `credential_profile_assignment_test.go` | ✅ green |
| 27-01-07 | 01 | 1 | WINBOX-04 | — | TriggerBackup returns error when no credential profile assigned to device | unit | `go test ./internal/service/... -run TestTriggerBackup_NoCredentialProfileAssigned` | `backup_service_test.go` | ✅ green |
| 27-02-01 | 02 | 2 | WINBOX-04 | T-27-07 | SSHCredentialForm calls assignCredentialProfile, not updateDevice | unit | `cd frontend && npm run test -- --run SSHCredentialForm` | `SSHCredentialForm.test.tsx` | ✅ green |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

*Existing infrastructure covers all phase requirements — no Wave 0 installs needed.*

---

## Manual-Only Verifications

*All phase behaviors have automated verification.*

---

## Validation Sign-Off

- [x] All tasks have automated verify commands
- [x] Sampling continuity: no 3 consecutive tasks without automated verify
- [x] Wave 0: not required — framework already present
- [x] No watch-mode flags in any command
- [x] Feedback latency < 60s
- [x] `nyquist_compliant: true` set in frontmatter

**Approval:** approved 2026-04-09

---

## Validation Audit 2026-04-09

| Metric | Count |
|--------|-------|
| Gaps found | 6 |
| Resolved | 6 |
| Escalated | 0 |

### Tests Generated

| Gap | Test Function | File |
|-----|--------------|------|
| 1 | `TestMigration000014_DevicesColumnDropped` | `internal/repository/sqlite/migrations_test.go` |
| 2 | `TestMigration000014_DeviceDataIntegrity` | `internal/repository/sqlite/migrations_test.go` |
| 3 | `TestVerifyLegacyTablesMigrated_PostMigration000014` | `internal/repository/sqlite/migrations_test.go` |
| 4 | `TestGetBackupProfileForDevice_NoProfileAssigned`, `TestGetBackupProfileForDevice_ReturnsProfile`, `TestGetBackupProfileForDevice_PrefersNonWinBox` | `internal/repository/sqlite/credential_profile_assignment_test.go` |
| 5 | `TestTriggerBackup_NoCredentialProfileAssigned` | `internal/service/backup_service_test.go` |
| 6 | 6 tests in `SSHCredentialForm.test.tsx` | `frontend/src/components/dashboard/SSHCredentialForm.test.tsx` |
