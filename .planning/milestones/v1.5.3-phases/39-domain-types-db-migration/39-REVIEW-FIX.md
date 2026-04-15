---
phase: 39-domain-types-db-migration
fixed_at: 2026-04-12T12:40:07Z
review_path: .planning/phases/39-domain-types-db-migration/39-REVIEW.md
iteration: 1
findings_in_scope: 4
fixed: 4
skipped: 0
status: all_fixed
---

# Phase 39: Code Review Fix Report

**Fixed at:** 2026-04-12T12:40:07Z
**Source review:** `.planning/phases/39-domain-types-db-migration/39-REVIEW.md`
**Iteration:** 1

**Summary:**
- Findings in scope: 4
- Fixed: 4
- Skipped: 0

**Verification notes:** Re-read every edited section and ran `git diff --check` on the modified files. Go-based tests could not run in this environment because the `go` toolchain is not installed.

## Fixed Issues

### WR-01: `AddDevice` never sets `PollClass` on the initial device record

**Status:** fixed: requires human verification
**Files modified:** `internal/service/device_service.go`, `internal/service/device_service_test.go`
**Commit:** `6a88da5`
**Applied fix:** `AddDevice` now derives `PollClass` from the normalized `DeviceType` before persistence, and a regression test verifies creation-time classification is correct for a Prometheus router.

### WR-02: Prometheus-only probe path skips `PollClass` reclassification

**Status:** fixed: requires human verification
**Files modified:** `internal/service/device_service.go`, `internal/service/device_service_test.go`
**Commit:** `1f0a653`
**Applied fix:** The Prometheus-only probe branch now re-fetches the device, marks it up, heals `PollClass` when no manual override is set, and the new regression test covers the legacy-row reclassification path.

### WR-03: Down-migration missing `idx_devices_sys_name_lookup` partial-index guard

**Status:** fixed
**Files modified:** `internal/repository/sqlite/migrations/000016_device_poll_classification.down.sql`
**Commit:** `ee6b0a4`
**Applied fix:** Added an explicit cross-reference comment tying the rollback index definition to `000015_scale_indexes.up.sql` so future edits keep the partial-index clause in sync.

### WR-04: `loadRegistryFromDB` silently returns `nil` registry on empty DB

**Status:** fixed: requires human verification
**Files modified:** `cmd/theia/main.go`, `cmd/theia/main_test.go`
**Commit:** `c1f84bf`
**Applied fix:** `loadRegistryFromDB` now falls back to YAML when every DB vendor record fails JSON validation, and a new main-package test reproduces the all-invalid-records case against an in-memory SQLite database.

---

_Fixed: 2026-04-12T12:40:07Z_
_Fixer: Claude (gsd-code-fixer)_
_Iteration: 1_
