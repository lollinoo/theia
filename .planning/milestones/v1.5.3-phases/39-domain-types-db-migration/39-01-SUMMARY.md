---
phase: 39-domain-types-db-migration
plan: "01"
subsystem: domain
tags: [domain, poll-class, volatility-class, types, enums]
dependency_graph:
  requires: []
  provides:
    - domain.PollClass (core/standard/low)
    - domain.VolatilityClass (static/operational/performance)
    - domain.PollClassCoreInterval (30s)
    - domain.PollClassStandardInterval (60s)
    - domain.PollClassLowInterval (300s)
    - domain.OperationalClassInterval (60s)
    - domain.StaticClassInterval (300s)
    - domain.ClassifyPollClass(DeviceType) PollClass
    - domain.PollClass.Interval() time.Duration
    - domain.Device.PollClass field
    - domain.Device.PollIntervalOverride field
  affects:
    - internal/domain/device.go (Device struct extended)
tech_stack:
  added: []
  patterns:
    - Typed-string enum with Type-prefix constants (PollClass, VolatilityClass)
    - Hardcoded duration constants co-located with type (mirrors Phase 38 threshold pattern)
    - Table-driven tests with subtests for complete coverage
key_files:
  created:
    - internal/domain/poll_class.go
    - internal/domain/poll_class_test.go
  modified:
    - internal/domain/device.go
decisions:
  - PollClass values locked at core/standard/low per D-03/D-04 (3-bucket model, not 4)
  - ClassifyPollClass is the single source of truth for DeviceType→PollClass mapping
  - PollIntervalOverride stored as *int (nil=use class default) per D-17/D-18
  - Interval constants hardcoded (not configurable) per D-06 — same pattern as Phase 38 thresholds
requirements-completed: [POLL-05]
metrics:
  duration: "2m29s"
  completed: "2026-04-12"
  tasks_completed: 2
  files_created: 2
  files_modified: 1
---

# Phase 39 Plan 01: Domain PollClass Types Summary

Typed foundation for tiered SNMP polling — `PollClass`, `VolatilityClass`, interval constants, `ClassifyPollClass` helper, and `Interval()` accessor — added to domain layer; `Device` struct extended with `PollClass` and `PollIntervalOverride` fields for downstream Phase 39–41 consumption.

## What Was Built

### New file: `internal/domain/poll_class.go`

All exports match D-04/D-07/D-08 exactly. Verbatim constant values:

| Export | Value / Type |
|--------|-------------|
| `PollClassCore` | `PollClass = "core"` |
| `PollClassStandard` | `PollClass = "standard"` |
| `PollClassLow` | `PollClass = "low"` |
| `VolatilityClassStatic` | `VolatilityClass = "static"` |
| `VolatilityClassOperational` | `VolatilityClass = "operational"` |
| `VolatilityClassPerformance` | `VolatilityClass = "performance"` |
| `PollClassCoreInterval` | `30 * time.Second` |
| `PollClassStandardInterval` | `60 * time.Second` |
| `PollClassLowInterval` | `300 * time.Second` |
| `OperationalClassInterval` | `60 * time.Second` |
| `StaticClassInterval` | `300 * time.Second` |

**`ClassifyPollClass(deviceType DeviceType) PollClass`** — switch-based helper:
- `DeviceTypeRouter`, `DeviceTypeSwitch` → `PollClassCore`
- `DeviceTypeAP`, `DeviceTypeUnknown`, `""`, any unknown literal → `PollClassStandard`
- `DeviceTypeVirtual` → `PollClassLow`

**`(c PollClass) Interval() time.Duration`** — returns the performance-tier interval for this class; unknown/empty PollClass falls back to `PollClassStandardInterval` (no panic path).

### Modified file: `internal/domain/device.go`

Two fields added to `Device` struct, inserted **after `DeviceType` and before `Status`** (line ~91–92 after the edit):

```go
PollClass            PollClass         `json:"poll_class"`
PollIntervalOverride *int              `json:"poll_interval_override"`
```

`*int` semantics: `nil` = use class default interval; non-nil = override performance polling interval in seconds (validation deferred to Phase 40/42 per T-39-01).

### New file: `internal/domain/poll_class_test.go`

Three test functions, 16 test cases total:

| Function | Cases | What It Covers |
|----------|-------|----------------|
| `TestClassifyPollClass` | 7 | All 5 DeviceType constants + empty-string + bogus literal fallbacks |
| `TestPollClass_Interval` | 4 | All 3 PollClass constants + garbage-value fallback |
| `TestDevice_PollClassFields_JSONRoundTrip` | 2 | Non-nil override round-trip; nil override round-trip |

All 16 pass under `go test -race ./internal/domain/...`.

## Commit

| Commit | Hash | Files |
|--------|------|-------|
| feat(39-01): add PollClass/VolatilityClass domain types and extend Device struct | `0e4c737` | `internal/domain/poll_class.go` (created), `internal/domain/poll_class_test.go` (created), `internal/domain/device.go` (modified) |

## Verification Results

```
go test -race ./internal/domain/...  → 16 passed
go build ./...                       → Success (no consumer broke)
grep -c 'PollClass' poll_class.go    → 32 (≥8 required)
grep -c 'VolatilityClass' poll_class.go → 8 (≥4 required)
```

## Deviations from Plan

None — plan executed exactly as written. All constant values, field names, JSON tags, and function signatures match D-04/D-07/D-08/D-20 verbatim. TDD RED→GREEN cycle followed; test file written before implementation.

## Known Stubs

None. This plan delivers typed scaffolding only — no runtime consumers, no data flow. The new fields on `Device` will be wired to the DB in Plan 39-02.

## Threat Flags

No new network endpoints, auth paths, file access patterns, or schema changes introduced in this plan. The domain layer receives only typed-string additions with no external input crossing the boundary. T-39-01/T-39-02/T-39-03 all carry `accept` or `mitigate` dispositions already documented in the plan's threat model.

## Self-Check: PASSED

- [x] `internal/domain/poll_class.go` exists — FOUND
- [x] `internal/domain/poll_class_test.go` exists — FOUND
- [x] `internal/domain/device.go` modified with new fields — FOUND
- [x] Commit `0e4c737` exists in git log — FOUND
