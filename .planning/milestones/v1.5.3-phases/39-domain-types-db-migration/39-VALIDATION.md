---
phase: 39
slug: domain-types-db-migration
status: verified
nyquist_compliant: true
wave_0_complete: true
created: 2026-04-15
---

# Phase 39 -- Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | Go standard `testing` package with `go build` smoke checks |
| **Config file** | None -- repo Go test conventions |
| **Quick run command** | `go test -race ./internal/domain/... ./internal/vendor/... ./internal/repository/sqlite/... ./internal/service/...` |
| **Full suite command** | `go test -race ./internal/domain/... ./internal/vendor/... ./internal/repository/sqlite/... ./internal/service/... && go build ./...` |
| **Estimated runtime** | ~30 seconds |

---

## Sampling Rate

- **After every task commit:** Run the exact task command captured in the phase plan.
- **After every plan wave:** Re-run the accepted package-scoped race suites plus `go build ./...`.
- **Before `/gsd-verify-work`:** The accepted `39-VERIFICATION.md` race-enabled package evidence and full build proof must still be green.
- **Max feedback latency:** 60 seconds

---

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Threat Ref | Secure Behavior | Test Type | Automated Command | Key Artifact | Status |
|---------|------|------|-------------|------------|-----------------|-----------|-------------------|--------------|--------|
| 39-01-T1 | 01 | 1 | POLL-05 | -- | Poll classification and volatility tiers are typed, exported, and deterministic | unit + race | `cd /home/azmin/projects/theia && go test -race ./internal/domain/...` | `internal/domain/poll_class.go` | ✅ green |
| 39-01-T2 | 01 | 1 | POLL-05 | -- | `domain.Device` carries `poll_class` and `poll_interval_override` across the JSON seam | unit + build | `cd /home/azmin/projects/theia && go test -race ./internal/domain/... && go build ./...` | `internal/domain/device.go` | ✅ green |
| 39-02-T1 | 02 | 1 | POLL-03 | -- | Vendor SNMP config is split into static, operational, and performance groups | unit + race | `cd /home/azmin/projects/theia && go test -race ./internal/vendor/...` | `internal/vendor/schema.go`, `internal/vendor/data/*.yaml` | ✅ green |
| 39-02-T2 | 02 | 1 | POLL-03 | -- | Registry resolves per-tier OIDs and metrics polling consumes performance-only config | integration + build | `cd /home/azmin/projects/theia && go test -race ./internal/vendor/... ./internal/snmp/... && go build ./...` | `internal/vendor/registry.go`, `internal/snmp/discovery.go`, `cmd/theia/main.go` | ✅ green |
| 39-03-T1 | 03 | 2 | POLL-05 | -- | SQLite schema adds `poll_class` and `poll_interval_override` without breaking migrations | migration | `cd /home/azmin/projects/theia && go test -race ./internal/repository/sqlite/ -run TestRunMigrations` | `internal/repository/sqlite/migrations/000016_device_poll_classification.{up,down}.sql` | ✅ green |
| 39-03-T2 | 03 | 2 | POLL-05 | -- | Existing rows backfill through `domain.ClassifyPollClass` in the Go migration layer | migration | `cd /home/azmin/projects/theia && go test -race ./internal/repository/sqlite/ -run 'TestMigrateDevicePollClass|TestRunMigrations'` | `internal/repository/sqlite/migrations.go` | ✅ green |
| 39-03-T3 | 03 | 2 | POLL-05 | -- | Repository CRUD paths persist and return poll metadata end to end | repository + build | `cd /home/azmin/projects/theia && go test -race ./internal/repository/sqlite/... && go build ./...` | `internal/repository/sqlite/device_repo.go` | ✅ green |
| 39-04-T1 | 04 | 2 | POLL-05 | -- | `probeDevice()` auto-reclassifies by device type unless an explicit override is pinned | unit | `cd /home/azmin/projects/theia && go test ./internal/service/ -run 'TestProbeDevice_ReclassifyOnTypeChange|TestProbeDevice_RespectsPollIntervalOverride|TestProbeDevice_NoTypeChangeStillSyncsPollClassWhenEmpty' -count=1 -v` | `internal/service/device_service.go` | ✅ green |
| 39-04-T2 | 04 | 2 | POLL-05 | -- | Roadmap success criteria reflect the shipped 3-bucket poll-class model | docs verify | `cd /home/azmin/projects/theia && awk '/^### Phase 39/,/^### Phase 40/' .planning/ROADMAP.md | grep -E '(core.*30s|standard.*60s|low.*300s)' && awk '/^### Phase 39/,/^### Phase 40/' .planning/ROADMAP.md | grep -v '120s' > /dev/null && echo OK` | `.planning/ROADMAP.md` | ✅ green |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

- [x] `internal/domain/poll_class_test.go` locks enum values, interval helpers, and JSON round-trip behavior for the new poll metadata types.
- [x] `internal/vendor` package tests cover nested SNMP config loading and per-tier resolver behavior for fallback and vendor-specific OIDs.
- [x] `internal/repository/sqlite` migration and repository tests cover schema application, Go-level backfill, and CRUD round-trips for the new columns.
- [x] `internal/service/device_service_test.go` proves probe-time reclassification respects overrides while keeping empty/changed device types in sync.
- [x] No new framework install was required -- existing Go test infrastructure and race detection already covered the phase.

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| -- | -- | All phase behaviors have automated verification. | -- |

*All phase behaviors have automated verification.*

---

## Validation Sign-Off

- [x] All tasks have `<automated>` verify or Wave 0 dependencies.
- [x] Sampling continuity: no 3 consecutive tasks without automated verify.
- [x] Wave 0 covers all missing validation references for domain, vendor, repository, and service coverage.
- [x] No watch-mode flags were used in accepted evidence.
- [x] Feedback latency remains under 60 seconds for the accepted package suites.
- [x] `nyquist_compliant: true` is set in frontmatter.

**Approval:** verified 2026-04-15
