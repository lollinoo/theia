---
phase: 39-domain-types-db-migration
plan: "02"
subsystem: vendor
tags: [vendor, snmp, oids, volatility-tiers, schema, registry, discovery]
dependency_graph:
  requires:
    - domain.PollClass / domain.VolatilityClass (39-01)
  provides:
    - vendor.SNMPConfig (nested Static/Operational/Performance)
    - vendor.StaticOIDs (placeholder struct)
    - vendor.OperationalOIDs (SysUpTimeOID, IfOperStatusOID)
    - vendor.PerformanceOIDs (CPUOID, MemoryUsedOID, MemoryTotalOID, TemperatureOID, TemperatureScale)
    - Registry.ResolveStaticOIDs(vendorName) StaticOIDs
    - Registry.ResolveOperationalOIDs(vendorName) OperationalOIDs
    - Registry.ResolvePerformanceOIDs(vendorName) PerformanceOIDs
    - Registry.ResolveSNMPConfig rebuilt as thin composition over per-tier resolvers
    - snmp.PollDeviceMetrics(client, vendor.PerformanceOIDs) updated signature
  affects:
    - internal/vendor/schema.go (SNMPConfig rewritten)
    - internal/vendor/registry.go (new resolver methods + ResolveSNMPConfig rebuilt)
    - internal/vendor/data/default.yaml (nested snmp section, new operational OIDs)
    - internal/vendor/data/mikrotik.yaml (nested snmp section)
    - internal/vendor/data/ubiquiti.yaml (nested snmp section)
    - internal/snmp/discovery.go (PollDeviceMetrics signature changed)
    - cmd/theia/main.go (newSNMPMetricsPollFunc call site updated)
tech_stack:
  added: []
  patterns:
    - Tiered OID struct grouping (StaticOIDs / OperationalOIDs / PerformanceOIDs)
    - Per-tier resolver methods with empty-string-preserving merge semantics
    - ResolveSNMPConfig as thin composition over tier resolvers (backward compat)
    - Standard-MIB OIDs in default.yaml; vendor YAMLs override only on genuine divergence
key_files:
  created: []
  modified:
    - internal/vendor/schema.go
    - internal/vendor/registry.go
    - internal/vendor/registry_test.go
    - internal/vendor/schema_test.go
    - internal/vendor/data/default.yaml
    - internal/vendor/data/mikrotik.yaml
    - internal/vendor/data/ubiquiti.yaml
    - internal/snmp/discovery.go
    - cmd/theia/main.go
decisions:
  - SNMPConfig flat fields replaced by Static/Operational/Performance nested structs per D-10/D-12/D-14
  - StaticOIDs is an empty placeholder in Phase 39 per D-12; fields migrate in Phase 40 when StaticCollector is wired
  - OperationalOIDs carries standard-MIB sysUpTime and ifOperStatus in default.yaml per D-11; vendor YAMLs leave operational empty unless genuinely diverging
  - ResolveSNMPConfig kept for backward compat, rebuilt as thin composition calling the three tier resolvers
  - PollDeviceMetrics signature changed from vendor.SNMPConfig to vendor.PerformanceOIDs; only the parameter type changes, no logic delta
  - Empty-string semantics preserved in all tier resolvers — vendor empty OID does NOT clobber default
requirements-completed: [POLL-03]
metrics:
  duration: "5m27s"
  completed: "2026-04-12"
  tasks_completed: 2
  files_created: 0
  files_modified: 9
---

# Phase 39 Plan 02: Vendor SNMP Tier Restructure Summary

Flat `SNMPConfig` rewritten into three volatility-tier structs (Static/Operational/Performance) with per-tier registry resolver methods; `PollDeviceMetrics` updated to consume `vendor.PerformanceOIDs` directly; standard-MIB operational OIDs added to `default.yaml`; all three vendor YAML files migrated atomically per D-14.

## What Was Built

### Modified: `internal/vendor/schema.go`

Old flat `SNMPConfig` (5 fields: TemperatureOID, TemperatureScale, CPUOID, MemoryUsedOID, MemoryTotalOID) replaced with:

```go
type SNMPConfig struct {
    Static      StaticOIDs      `yaml:"static" json:"static"`
    Operational OperationalOIDs `yaml:"operational" json:"operational"`
    Performance PerformanceOIDs `yaml:"performance" json:"performance"`
}
```

Three new types added:
- `StaticOIDs` — empty placeholder per D-12; Phase 40 will populate with ifTable/LLDP/CDP OIDs
- `OperationalOIDs` — `SysUpTimeOID` + `IfOperStatusOID` for reachability polling
- `PerformanceOIDs` — `CPUOID`, `MemoryUsedOID`, `MemoryTotalOID`, `TemperatureOID`, `TemperatureScale`

### Modified: `internal/vendor/data/default.yaml`

`snmp:` section restructured from flat to nested with three sub-maps. New standard-MIB operational OIDs added per D-11:
- `operational.sys_uptime_oid: ".1.3.6.1.2.1.1.3.0"` (sysUpTime)
- `operational.if_oper_status_oid: ".1.3.6.1.2.1.2.2.1.8"` (ifOperStatus column)

### Modified: `internal/vendor/data/mikrotik.yaml` and `ubiquiti.yaml`

Both migrated to nested form in the same commit per D-14 (no intermediate state with mixed shapes). MikroTik's `operational: {}` (empty) means it falls back to default standard-MIB OIDs tier-by-tier.

### Modified: `internal/vendor/registry.go`

Three new per-tier resolver methods added:

| Method | Returns | Merge semantics |
|--------|---------|-----------------|
| `ResolveStaticOIDs(vendorName)` | `StaticOIDs` | No-op in Phase 39 (empty struct) |
| `ResolveOperationalOIDs(vendorName)` | `OperationalOIDs` | Empty vendor strings → fall back to default |
| `ResolvePerformanceOIDs(vendorName)` | `PerformanceOIDs` | Empty vendor strings → fall back to default |

`ResolveSNMPConfig` rebuilt as thin composition:
```go
func (r *Registry) ResolveSNMPConfig(vendorName string) SNMPConfig {
    return SNMPConfig{
        Static:      r.ResolveStaticOIDs(vendorName),
        Operational: r.ResolveOperationalOIDs(vendorName),
        Performance: r.ResolvePerformanceOIDs(vendorName),
    }
}
```

### Modified: `internal/snmp/discovery.go`

`PollDeviceMetrics` signature changed:
```go
// Before:
func PollDeviceMetrics(client ClientInterface, snmpCfg vendor.SNMPConfig) (...)
// After:
func PollDeviceMetrics(client ClientInterface, perfOIDs vendor.PerformanceOIDs) (...)
```

All `snmpCfg.CPUOID` → `perfOIDs.CPUOID`, `snmpCfg.TemperatureOID` → `perfOIDs.TemperatureOID`, `snmpCfg.TemperatureScale` → `perfOIDs.TemperatureScale`. No logic changes.

### Modified: `cmd/theia/main.go`

`newSNMPMetricsPollFunc` updated to call `ResolvePerformanceOIDs` instead of `ResolveSNMPConfig`:
```go
perfOIDs := vendorRegistry.ResolvePerformanceOIDs(vendorName)
cpu, mem, uptime, temp := snmp.PollDeviceMetrics(client, perfOIDs)
```

### Test coverage

`schema_test.go` — `TestVendorConfigUnmarshal` updated with nested assertions; `TestVendorConfigUnmarshal_NestedSNMPGroups` added with three sub-tests:
- `all_three_groups_populated`: asserts every tier's fields accessible
- `missing_snmp_performance_section_no_panic`: T-39-05 mitigation — zero-value struct, no panic
- `no_snmp_section_at_all_no_panic`: complete missing section, no panic

`registry_test.go` — Two new tests added:
- `TestResolvePerformanceOIDs_VendorOverride`: vendor override + fallback + nonexistent vendor
- `TestResolveOperationalOIDs_DefaultStandardMIB`: standard-MIB fallback for empty operational and nonexistent vendor

`TestRegistryConcurrency` reader loop extended to call `ResolvePerformanceOIDs("mikrotik")` and `ResolveOperationalOIDs("mikrotik")` so race-detector covers the new methods.

## Commits

| Commit | Hash | Files |
|--------|------|-------|
| feat(39-02): restructure SNMPConfig into Static/Operational/Performance tiers | `01e6a6e` | schema.go, registry.go, schema_test.go, registry_test.go (partial), default.yaml, mikrotik.yaml, ubiquiti.yaml, discovery.go, main.go |
| test(39-02): add per-tier resolver tests for vendor override and operational fallback | `d522909` | registry_test.go |

## Verification Results

```
go test -race ./internal/vendor/...  → ok (all tests pass including new tests)
go test -race ./internal/snmp/...   → ok (PollDeviceMetrics new signature passes)
go build ./...                       → Success (full module compiles)
```

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] discovery.go and main.go updated in Task 1 commit**
- **Found during:** Task 1 implementation — changing SNMPConfig broke the build
- **Issue:** `internal/snmp/discovery.go` referenced `snmpCfg.CPUOID`, `snmpCfg.TemperatureOID`, `snmpCfg.TemperatureScale` (flat fields no longer on SNMPConfig); `cmd/theia/main.go` called `vendorRegistry.ResolveSNMPConfig(vendorName)` and passed result directly to `PollDeviceMetrics`
- **Fix:** Applied Task 2's Steps C and D (discovery.go signature change + main.go call site update) in the same commit as Task 1's schema/YAML/registry changes, to keep the build passing at every commit boundary
- **Files modified:** `internal/snmp/discovery.go`, `cmd/theia/main.go`
- **Commit:** `01e6a6e`

**2. [Rule 3 - Blocking] registry.go ResolveSNMPConfig updated in Task 1**
- **Found during:** Task 1 implementation — old ResolveSNMPConfig body accessed flat fields (`.SNMP.TemperatureOID`, `.SNMP.CPUOID`, etc.) that no longer exist on SNMPConfig
- **Fix:** Rewrote ResolveSNMPConfig as thin composition over the three new tier resolver methods. Also added all three tier resolver methods (ResolveStaticOIDs, ResolveOperationalOIDs, ResolvePerformanceOIDs) in Task 1 rather than Task 2 — necessary to make ResolveSNMPConfig compile and to unblock the build
- **Files modified:** `internal/vendor/registry.go`
- **Commit:** `01e6a6e`

The net result is that Task 2 was logically split: implementation (methods + call site) went into the Task 1 commit (build fix), and the behavioral tests went into the Task 2 commit. All plan-specified acceptance criteria for both tasks are met.

## Known Stubs

None. All three vendor YAML files have concrete values (no placeholder text). `StaticOIDs` is intentionally empty per D-12 — this is an architectural placeholder for Phase 40, not a data stub. The empty struct is documented in the type's GoDoc comment.

## Threat Flags

No new network endpoints, auth paths, or trust boundary crossings introduced. The DB-stored vendor JSON shape change (T-39-04, `accept`) is pre-existing. T-39-05 (missing `snmp.performance` section → zero-value struct) is mitigated by the new `missing_snmp_performance_section_no_panic` sub-test in `TestVendorConfigUnmarshal_NestedSNMPGroups`.

## Self-Check: PASSED

- [x] `internal/vendor/schema.go` contains `Static StaticOIDs`, `Operational OperationalOIDs`, `Performance PerformanceOIDs` — FOUND
- [x] `internal/vendor/registry.go` contains `ResolveStaticOIDs`, `ResolveOperationalOIDs`, `ResolvePerformanceOIDs` — FOUND
- [x] `internal/vendor/data/default.yaml` has nested `operational:` with `sys_uptime_oid` — FOUND
- [x] `internal/vendor/data/mikrotik.yaml` has nested `performance:` with MikroTik temp OID — FOUND
- [x] `internal/vendor/data/ubiquiti.yaml` has nested `performance:` section — FOUND
- [x] `internal/snmp/discovery.go` `PollDeviceMetrics` takes `vendor.PerformanceOIDs` — FOUND
- [x] `cmd/theia/main.go` calls `ResolvePerformanceOIDs(vendorName)` — FOUND
- [x] `internal/vendor/schema_test.go` has `TestVendorConfigUnmarshal_NestedSNMPGroups` — FOUND
- [x] `internal/vendor/registry_test.go` has `TestResolvePerformanceOIDs_VendorOverride` and `TestResolveOperationalOIDs_DefaultStandardMIB` — FOUND
- [x] Commit `01e6a6e` exists — FOUND
- [x] Commit `d522909` exists — FOUND
