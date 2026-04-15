---
phase: 45-polling-cadence-gap-closure
reviewed: 2026-04-13T20:56:27Z
depth: standard
files_reviewed: 9
files_reviewed_list:
  - cmd/theia/main.go
  - cmd/theia/main_test.go
  - internal/scheduler/scheduler.go
  - internal/scheduler/scheduler_test.go
  - internal/service/device_service.go
  - internal/service/device_service_test.go
  - internal/state/store.go
  - internal/state/store_test.go
  - internal/worker/pipeline_test.go
findings:
  critical: 2
  warning: 2
  info: 0
  total: 4
status: issues_found
---

# Phase 45: Code Review Report

**Reviewed:** 2026-04-13T20:56:27Z
**Depth:** standard
**Files Reviewed:** 9
**Status:** issues_found

## Summary

The review covered the new scheduler/state-store wiring, the poll-cadence reschedule path, and the updated startup/runtime wiring in `main.go`. The core scheduling and state logic is well-covered by tests, and both `go test` and `go test -race` passed for the scoped backend packages, but there are still two high-impact startup/restore defects and two correctness issues in the new runtime path.

## Critical Issues

### CR-01: Nil fallback from `loadRegistryFromDB` can crash startup

**File:** `cmd/theia/main.go:293-298`
**Issue:** `loadRegistryFromDB()` deliberately returns `nil, nil` when the DB registry is empty or every row is invalid JSON, but the caller only falls back to YAML on `err != nil`. In that state, `vendorRegistry` stays nil and `vendorRegistry.VendorCount()` panics during startup. The new test in `cmd/theia/main_test.go` already codifies the `nil` fallback contract, so the crash path is now part of the intended behavior surface.
**Fix:**
```go
vendorRegistry, err := loadRegistryFromDB(vendorConfigRepo)
if err != nil || vendorRegistry == nil {
	if err != nil {
		log.Printf("Warning: failed to load registry from DB, falling back to YAML: %v", err)
	} else {
		log.Printf("Warning: DB vendor registry empty/invalid, falling back to YAML registry")
	}
	vendorRegistry = yamlRegistry
}
```

### CR-02: Restore flow can destroy live backups/known_hosts before confirming replacement succeeded

**File:** `cmd/theia/main.go:109-145`
**Issue:** `applyPendingRestore()` deletes the live backup directory and `known_hosts` before the fallback copy path has succeeded, then removes the marker and staging directory even if those copies fail. A cross-device rename error or partial copy failure leaves the instance in a partially restored state with the old data already deleted and no way to retry on the next startup.
**Fix:**
```go
// Stage into a temp path first; only swap and clear the marker after every step succeeds.
tmpBackups := marker.DeviceBackupDir + ".restore-tmp"
if err := copyOrRenameDir(marker.StagedBackups, tmpBackups); err != nil {
	return false
}
if err := os.RemoveAll(marker.DeviceBackupDir); err != nil {
	return false
}
if err := os.Rename(tmpBackups, marker.DeviceBackupDir); err != nil {
	return false
}

// Same pattern for known_hosts, and only remove marker/staging on full success.
```

## Warnings

### WR-01: Collector client factory ignores the timeout/retry values the pipeline passes in

**File:** `cmd/theia/main.go:541-559`
**Issue:** `newCollectorSNMPClientFunc()` discards its `timeout` and `retries` parameters and re-reads settings instead. That silently breaks the contract of the collector API and makes the effective defaults diverge from `PipelineOrchestrator.snmpTimeout()/snmpRetries()` when settings are missing or invalid (`5s/1` here vs `10s/2` in the pipeline). Poll behavior will therefore be shorter and less retry-tolerant than the caller asked for.
**Fix:**
```go
func newCollectorSNMPClientFunc(settingsRepo domain.SettingsRepository) collector.NewSNMPClientFunc {
	return func(target string, creds domain.SNMPCredentials, timeout time.Duration, retries int) (collector.SNMPClient, error) {
		if timeout <= 0 {
			timeout = 10 * time.Second
		}
		if retries < 0 {
			retries = 2
		}
		if settingsRepo != nil {
			// Override only when a valid persisted setting exists.
		}
		return newCollectorSNMPClient(target, creds, timeout, retries)
	}
}
```

### WR-02: Successful discovery can leave a device stuck in `probing`

**File:** `internal/service/device_service.go:222-239`
**Issue:** when SNMP discovery succeeds but `ApplyStaticDiscovery()` fails, `probeDevice()` logs the error and returns before setting any terminal status. For newly added devices this leaves the row permanently at `probing`, even though the device was actually reachable and the only failure was topology persistence.
**Fix:**
```go
persisted, err := s.ApplyStaticDiscovery(deviceID, input)
if err != nil {
	if statusErr := s.updateDeviceStatus(deviceID, domain.DeviceStatusUp); statusErr != nil {
		log.Printf("Failed to update device %s status to up: %v", deviceIP, statusErr)
	}
	log.Printf("Failed to persist static discovery for %s: %v", deviceIP, err)
	return
}
```

---

_Reviewed: 2026-04-13T20:56:27Z_
_Reviewer: Codex (gsd-code-reviewer)_
_Depth: standard_
