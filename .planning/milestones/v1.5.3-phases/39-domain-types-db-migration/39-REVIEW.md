---
phase: 39-domain-types-db-migration
reviewed: 2026-04-12T11:33:26Z
depth: standard
files_reviewed: 20
files_reviewed_list:
  - internal/domain/poll_class.go
  - internal/domain/poll_class_test.go
  - internal/domain/device.go
  - internal/vendor/schema.go
  - internal/vendor/registry.go
  - internal/vendor/registry_test.go
  - internal/vendor/schema_test.go
  - internal/vendor/data/default.yaml
  - internal/vendor/data/mikrotik.yaml
  - internal/vendor/data/ubiquiti.yaml
  - internal/snmp/discovery.go
  - cmd/theia/main.go
  - internal/repository/sqlite/migrations/000016_device_poll_classification.up.sql
  - internal/repository/sqlite/migrations/000016_device_poll_classification.down.sql
  - internal/repository/sqlite/migrations.go
  - internal/repository/sqlite/migrations_test.go
  - internal/repository/sqlite/device_repo.go
  - internal/repository/sqlite/device_repo_test.go
  - internal/service/device_service.go
  - internal/service/device_service_test.go
findings:
  critical: 0
  warning: 4
  info: 4
  total: 8
status: issues_found
---

# Phase 39: Code Review Report

**Reviewed:** 2026-04-12T11:33:26Z
**Depth:** standard
**Files Reviewed:** 20
**Status:** issues_found

## Summary

Phase 39 introduces `PollClass` / `VolatilityClass` domain types, a three-tiered SNMP config shape (`static` / `operational` / `performance`) in the vendor registry, a SQLite migration adding `poll_class` and `poll_interval_override` columns to `devices`, a Go-level backfill migration, and wiring through `DeviceRepo` and `DeviceService`.

The overall structure is clean and the test coverage is solid. Four warnings are raised, all of which represent logic correctness gaps rather than style observations. Four informational items cover dead code, missing test coverage, and a subtle data-integrity concern.

## Warnings

### WR-01: `AddDevice` never sets `PollClass` on the initial device record

**File:** `internal/service/device_service.go:113-128`
**Issue:** The `Device` struct assembled in `AddDevice` has no explicit `PollClass` assignment. The field is left as the zero value (`""`). `DeviceRepo.createOnce` normalises this to `PollClassStandard` (line 75-78 of `device_repo.go`), so every newly-added device — including routers and switches — starts as `standard` rather than the correct class derived from `deviceType`. The correct class is only applied later when `probeDevice` writes back `domain.ClassifyPollClass(fresh.DeviceType)`. For Prometheus-only devices, `probeDevice` returns immediately after calling `markDeviceStatus` (line 183-186), which updates only `Status`, meaning a router added with `MetricsSourcePrometheus` will permanently carry `poll_class = 'standard'` unless a separate re-probe is triggered.

**Fix:**
```go
// In AddDevice, after the deviceType / metricsSource normalisations, add:
device := &domain.Device{
    ...
    DeviceType: deviceType,
    PollClass:  domain.ClassifyPollClass(deviceType), // derive at creation time
    ...
}
```

### WR-02: Prometheus-only probe path skips `PollClass` reclassification

**File:** `internal/service/device_service.go:183-187`
**Issue:** The early-return branch for `MetricsSourcePrometheus` devices calls `markDeviceStatus` (which performs a full re-fetch and update) but does not set `PollClass` before writing. A router whose `MetricsSource` is `prometheus` will therefore retain `poll_class = 'standard'` in the DB indefinitely — contradicting the D-19 comment at line 213-218 that reads "Empty PollClass on a legacy row is also healed here on first probe". The healing only happens on the non-prometheus path.

**Fix:**
```go
if device.MetricsSource == domain.MetricsSourcePrometheus {
    fresh, err := s.deviceRepo.GetByID(deviceID)
    if err != nil {
        log.Printf("Failed to re-fetch device %s for prometheus probe: %v", deviceIP, err)
        return
    }
    fresh.Status = domain.DeviceStatusUp
    if fresh.PollIntervalOverride == nil {
        fresh.PollClass = domain.ClassifyPollClass(fresh.DeviceType)
    }
    if err := s.deviceRepo.Update(fresh); err != nil {
        log.Printf("Failed to update device %s status to up: %v", deviceIP, err)
    }
    log.Printf("Skipped SNMP probe for %s (metrics_source=prometheus); marked up", deviceIP)
    return
}
```

### WR-03: Down-migration missing `idx_devices_sys_name_lookup` partial-index guard

**File:** `internal/repository/sqlite/migrations/000016_device_poll_classification.down.sql:44-45`
**Issue:** The down migration recreates `idx_devices_sys_name_lookup` unconditionally after the table rebuild:
```sql
CREATE INDEX IF NOT EXISTS idx_devices_sys_name_lookup
    ON devices(sys_name_lookup) WHERE sys_name_lookup != '';
```
This index was introduced by migration 000015. If, during a rollback, 000016 is reversed before 000015, the index will exist after 000015 also tries to create it. The `IF NOT EXISTS` guard prevents an error, but the partial-index clause (`WHERE sys_name_lookup != ''`) must exactly match the one in 000015's up migration — a copy/paste divergence between the two files would silently leave the wrong index definition in place. The concern is low-risk given the `IF NOT EXISTS` guard, but the duplication is fragile.

**Fix:** Add a comment cross-referencing migration 000015 so the index definition is explicitly kept in sync:
```sql
-- IMPORTANT: This index definition must match 000015_scale_indexes.up.sql exactly.
-- If 000015's index clause changes, update this down migration to match.
CREATE INDEX IF NOT EXISTS idx_devices_sys_name_lookup
    ON devices(sys_name_lookup) WHERE sys_name_lookup != '';
```

### WR-04: `loadRegistryFromDB` silently returns `nil` registry on empty DB

**File:** `cmd/theia/main.go:470-494`
**Issue:** `loadRegistryFromDB` returns `(nil, nil)` when `records` is empty (line 476-478). The caller at line 283-286 treats a `nil` registry as a fallback signal and uses `yamlRegistry` instead, which is correct for a truly-empty DB. However, if every record in the DB has invalid JSON (all are skipped at lines 483-487), the function also builds an empty `dbRecords` slice and passes it to `vendor.LoadRegistryFromDB`, which fails with "no default vendor config found in DB" — a non-nil error. This code path is fine. The risk is the opposite: if `records` is non-empty but all entries fail JSON validation, the function returns a load error rather than gracefully falling back, so startup fails hard instead of recovering to `yamlRegistry`. This is unlikely in practice (seeding happens at every start) but is worth noting.

**Fix:**
```go
// After the JSON validation loop, fall back to yaml if no valid records remain.
if len(dbRecords) == 0 {
    log.Printf("Warning: all DB vendor records failed JSON validation, falling back to YAML registry")
    return nil, nil
}
```

---

## Info

### IN-01: `DeviceUpdate` struct does not expose `PollClass` / `PollIntervalOverride`

**File:** `internal/service/device_service.go:27-37`
**Issue:** `DeviceUpdate` is the partial-update struct used by `UpdateDevice`. It omits `PollClass` and `PollIntervalOverride`, so there is currently no service-layer path for an API handler to set these fields on an existing device via `UpdateDevice`. The Phase 39 scope intentionally defers the API handler (`Phase 40+` per the up-migration comment), but this gap means any Phase 40 work will need to extend this struct. Document the gap to avoid confusion.

**Fix:** Add a comment to `DeviceUpdate`:
```go
// Note: PollClass and PollIntervalOverride are intentionally absent here.
// Phase 40 will add a dedicated PATCH /api/v1/devices/:id/poll-config endpoint.
```

### IN-02: `TestProbeCompletes_DeviceStatusUp` does not assert `PollClass` reclassification

**File:** `internal/service/device_service_test.go:288-328`
**Issue:** The test verifies that after a successful SNMP probe, `Status == up`, `SysName`, and `Interfaces` are set, but does not assert that `PollClass` was reclassified to `PollClassCore` (the discovery result returns `DeviceType = router`). This leaves the D-19 reclassification path untested at the service layer. The gap was identified in WR-01 (the Prometheus-only path is also unexercised for `PollClass`).

**Fix:** Add assertions:
```go
if updated.PollClass != domain.PollClassCore {
    t.Errorf("PollClass: got %q, want %q (router should be core)", updated.PollClass, domain.PollClassCore)
}
```

### IN-03: `copyFileForRestore` calls `out.Close()` twice

**File:** `cmd/theia/main.go:141-162`
**Issue:** `copyFileForRestore` calls `out.Close()` explicitly at line 161 and also via `defer out.Close()` at line 157. On successful copy the file is closed twice. In Go, calling `Close()` on a `*os.File` a second time returns an error (the file descriptor is invalid), but the second-call error is discarded because the function returns the result of the explicit `out.Close()` call. The `defer` then silently executes a second close with the result dropped. This is a no-op in practice on Linux (the OS handles it), but it is an anti-pattern that can surface on some platforms or with certain wrappers.

**Fix:** Remove the `defer out.Close()` and rely only on the explicit close at the end:
```go
out, err := os.Create(dst)
if err != nil {
    return err
}
// No defer close — we return out.Close() explicitly below.

if _, err := io.Copy(out, in); err != nil {
    out.Close() // discard error; io.Copy error is the primary failure
    return err
}
return out.Close()
```

### IN-04: `TemperatureScale` zero-value guard prevents explicitly setting scale to zero

**File:** `internal/vendor/registry.go:348-350`
**Issue:** `ResolvePerformanceOIDs` guards all vendor overrides with non-zero checks:
```go
if cfg.SNMP.Performance.TemperatureScale != 0 {
    result.TemperatureScale = cfg.SNMP.Performance.TemperatureScale
}
```
A vendor YAML that intentionally sets `temperature_scale: 0` (to disable scaling or indicate "no sensor") cannot override the default's non-zero value. This is consistent with how empty strings are handled for OID fields, but `0` is a semantically valid float for disabling a temperature reading and differs from an omitted field. In YAML, an absent `temperature_scale` and `temperature_scale: 0` are indistinguishable after unmarshalling. The issue is the YAML schema rather than the code, but callers should be aware that a vendor cannot zero-out the default scale.

**Fix:** Document the limitation in the YAML field comment or use a `*float64` pointer to distinguish absent from zero:
```go
// TemperatureScale converts raw sensor value to Celsius (e.g., 0.1 for tenths).
// Note: zero is treated as "not set" — use the default value instead.
// A vendor cannot override the default scale with 0.
TemperatureScale float64 `yaml:"temperature_scale" json:"temperature_scale"`
```

---

_Reviewed: 2026-04-12T11:33:26Z_
_Reviewer: Claude (gsd-code-reviewer)_
_Depth: standard_
