---
phase: 23
slug: credential-profile-schema-domain
status: validated
nyquist_compliant: true
wave_0_complete: false
created: 2026-04-09
audited: 2026-04-09
---

# Phase 23 — Validation Strategy

> Per-phase validation contract. Reconstructed from plan and summary artifacts (State B).

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | Go standard `testing` package |
| **Config file** | none |
| **Quick run command** | `/usr/local/go/bin/go test ./internal/api/... ./internal/repository/sqlite/... -run TestCredentialProfile -count=1` |
| **Full suite command** | `/usr/local/go/bin/go test ./internal/... ./cmd/... -count=1` |
| **Estimated runtime** | ~5 seconds |

---

## Sampling Rate

- **After every task commit:** Run quick run command
- **After every plan wave:** Run full suite command
- **Before `/gsd-verify-work`:** Full suite must be green
- **Max feedback latency:** ~5 seconds

---

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Threat Ref | Secure Behavior | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|------------|-----------------|-----------|-------------------|-------------|--------|
| 23-01-01 | 01 | 1 | CRED-04 | T-23-02 | role DEFAULT 'Admin' applied atomically; join table seeded from FK — no data fabrication | integration | `/usr/local/go/bin/go test ./internal/repository/sqlite/... -run TestMigration000012_DefaultRole -count=1` | ✅ | ✅ green |
| 23-01-02 | 01 | 1 | CRED-01 | T-23-01 | EncryptedSecret has json:"-" tag — never serialized | unit | `/usr/local/go/bin/go test ./internal/api/... -run 'TestCredentialProfile_CustomRole_RoundTrip\|TestCredentialProfile_EmptyRole_DefaultsToAdmin' -count=1` | ✅ | ✅ green |
| 23-02-01 | 02 | 2 | CRED-01 | T-23-04 | credentialProfileResponse excludes EncryptedSecret | integration | `/usr/local/go/bin/go test ./internal/api/... -run TestCredentialProfileHandler -count=1` | ✅ | ✅ green |
| 23-02-02 | 02 | 2 | CRED-02 | — | device_credential_profiles join table: multiple profiles per device accepted | integration | `/usr/local/go/bin/go test ./internal/repository/sqlite/... -run TestCredentialProfileAssignProfile_MultipleProfiles -count=1` | ✅ | ✅ green |
| 23-02-03 | 02 | 2 | CRED-01, CRED-02, CRED-04 | T-23-05 | SQL queries use parameterized placeholders; table name hardcoded | integration | `/usr/local/go/bin/go test ./internal/repository/sqlite/... -run TestCredentialProfile -count=1` | ✅ | ✅ green |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

Existing infrastructure covers all phase requirements.

---

## Manual-Only Verifications

All phase behaviors have automated verification.

---

## Validation Sign-Off

- [x] All tasks have `<automated>` verify commands
- [x] Sampling continuity: no 3 consecutive tasks without automated verify
- [x] Wave 0 covers all MISSING references (N/A — existing infra)
- [x] No watch-mode flags
- [x] Feedback latency < 5s
- [x] `nyquist_compliant: true` set in frontmatter

**Approval:** approved 2026-04-09

---

## Validation Audit 2026-04-09

| Metric | Count |
|--------|-------|
| Gaps found | 3 |
| Resolved | 3 |
| Escalated | 0 |

### Gaps Resolved

| Requirement | Gap Type | Test Added |
|-------------|----------|------------|
| CRED-01 | PARTIAL | `TestCredentialProfile_CustomRole_RoundTrip`, `TestCredentialProfile_EmptyRole_DefaultsToAdmin` in `internal/api/credential_profile_handler_test.go` |
| CRED-02 | PARTIAL | `TestCredentialProfileAssignProfile_MultipleProfiles` in `internal/repository/sqlite/credential_profile_assignment_test.go` |
| CRED-04 | MISSING | `TestMigration000012_DefaultRole` in `internal/repository/sqlite/migrations_test.go` |
