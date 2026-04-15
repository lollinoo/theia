---
phase: 45
fixed_at: 2026-04-13T21:25:03Z
review_path: .planning/phases/45-polling-cadence-gap-closure/45-REVIEW.md
iteration: 1
findings_in_scope: 4
fixed: 4
skipped: 0
status: all_fixed
---

# Phase 45: Code Review Fix Report

**Fixed at:** 2026-04-13T21:25:03Z
**Source review:** `.planning/phases/45-polling-cadence-gap-closure/45-REVIEW.md`
**Iteration:** 1

**Summary:**
- Findings in scope: 4
- Fixed: 4
- Skipped: 0

## Fixed Issues

### CR-01: Nil fallback from `loadRegistryFromDB` can crash startup

**Status:** `fixed: requires human verification`
**Files modified:** `cmd/theia/main.go`
**Commit:** `87c99cb`
**Applied fix:** Startup now falls back to the YAML registry when the DB loader returns `nil` without an error, avoiding a nil dereference before `VendorCount()`.
**Verification:** `rtk go test ./cmd/theia -run 'TestLoadRegistryFromDB_FallsBackWhenAllRecordsInvalid|TestMainSNMPRuntimeHelpersRemainConstructibleAfterPipelineCutover|TestWirePollRescheduler_AttachesSchedulerToDeviceService'` passed.

### CR-02: Restore flow can destroy live backups/known_hosts before confirming replacement succeeded

**Status:** `fixed: requires human verification`
**Files modified:** `cmd/theia/main.go`, `cmd/theia/main_test.go`
**Commit:** `cc5b133`
**Applied fix:** Restore now stages backup and `known_hosts` replacements into temporary paths, swaps them in only after staging succeeds, and keeps the marker/staging directory intact when artifact replacement fails so startup can retry safely.
**Verification:** `rtk go test ./cmd/theia -run 'TestApplyPendingRestore_KeepsLiveArtifactsWhenBackupReplacementFails|TestApplyPendingRestore_CleansUpAfterSuccessfulArtifactSwap|TestLoadRegistryFromDB_FallsBackWhenAllRecordsInvalid|TestMainSNMPRuntimeHelpersRemainConstructibleAfterPipelineCutover|TestWirePollRescheduler_AttachesSchedulerToDeviceService'` passed.

### WR-01: Collector client factory ignores the timeout/retry values the pipeline passes in

**Status:** `fixed: requires human verification`
**Files modified:** `cmd/theia/main.go`, `cmd/theia/main_test.go`
**Commit:** `63b1b94`
**Applied fix:** The collector SNMP client factory now honors the caller-provided timeout/retry values, applies sane defaults only for invalid caller inputs, and lets valid persisted settings override those inputs explicitly.
**Verification:** `rtk go test ./cmd/theia -run 'TestMainSNMPRuntimeHelpersRemainConstructibleAfterPipelineCutover|TestApplyPendingRestore_KeepsLiveArtifactsWhenBackupReplacementFails|TestApplyPendingRestore_CleansUpAfterSuccessfulArtifactSwap|TestLoadRegistryFromDB_FallsBackWhenAllRecordsInvalid|TestWirePollRescheduler_AttachesSchedulerToDeviceService'` passed.

### WR-02: Successful discovery can leave a device stuck in `probing`

**Status:** `fixed: requires human verification`
**Files modified:** `internal/service/device_service.go`, `internal/service/device_service_test.go`
**Commit:** `c27e779`
**Applied fix:** `probeDevice()` now marks the device `up` after successful discovery even when static-topology persistence fails, so the row no longer remains stuck in `probing`.
**Verification:** `rtk go test ./internal/service -run 'TestProbeDevice_StaticDiscoveryPersistenceFailureStillMarksUp|TestProbeDevice_ReclassifyOnTypeChange|TestProbeDevice_RespectsPollIntervalOverride|TestProbeDevice_NoTypeChangeStillSyncsPollClassWhenEmpty'` passed.

---

_Fixed: 2026-04-13T21:25:03Z_
_Fixer: Claude (gsd-code-fixer)_
_Iteration: 1_
