---
phase: 39-domain-types-db-migration
plan: "04"
subsystem: service
tags: [service, poll-class, auto-reclassify, probeDevice, roadmap]
dependency_graph:
  requires:
    - domain.ClassifyPollClass (39-01)
    - domain.Device.PollClass / PollIntervalOverride fields (39-01)
  provides:
    - probeDevice() auto-reclassify hook (D-19)
    - Override guard: PollIntervalOverride != nil skips recompute
    - TestProbeDevice_ReclassifyOnTypeChange
    - TestProbeDevice_RespectsPollIntervalOverride
    - TestProbeDevice_NoTypeChangeStillSyncsPollClassWhenEmpty
    - ROADMAP §Phase 39 success criterion 2: 3-bucket PollClass model (D-09)
  affects:
    - internal/service/device_service.go (probeDevice auto-reclassify hook)
    - internal/service/device_service_test.go (three new regression tests + intPtr helper)
    - .planning/ROADMAP.md (Phase 39 success criterion 2 rewritten)
tech_stack:
  added: []
  patterns:
    - Inline D-19 guard: PollIntervalOverride nil-check before ClassifyPollClass call
    - Unconditional recompute heals legacy empty PollClass rows on first probe
    - TDD RED-GREEN cycle: tests written before implementation
key_files:
  created: []
  modified:
    - internal/service/device_service.go
    - internal/service/device_service_test.go
    - .planning/ROADMAP.md
decisions:
  - D-19: ClassifyPollClass called unconditionally (not only on device_type change) so empty PollClass on legacy rows is healed on first probe after deployment
  - D-09: ROADMAP success criterion 2 now describes 3-bucket model; D-09 deviation resolved -- ROADMAP and code agree
  - Override guard is a simple nil-check on PollIntervalOverride; no new public method or field needed
requirements-completed: [POLL-05]
metrics:
  duration: "3m33s"
  completed: "2026-04-12"
  tasks_completed: 2
  files_created: 0
  files_modified: 3
---

# Phase 39 Plan 04: probeDevice Auto-Reclassify + ROADMAP Update Summary

`probeDevice()` now recomputes `PollClass` via `domain.ClassifyPollClass(fresh.DeviceType)` after every successful SNMP probe, skipping the recompute only when `PollIntervalOverride` is non-nil; three regression tests cover all branches; `ROADMAP.md` §Phase 39 success criterion 2 updated to reflect the 3-bucket PollClass model (D-09 resolved).

## What Was Built

### Modified: `internal/service/device_service.go`

Inserted 6 lines into `probeDevice()` immediately after `fresh.DeviceType = result.DeviceType` and before `fresh.Status = domain.DeviceStatusUp`:

```go
fresh.DeviceType = result.DeviceType
// D-19: Auto-reclassify poll_class to follow device_type unless the operator
// has set a manual PollIntervalOverride (per-device override = manual control,
// do not stomp). Empty PollClass on a legacy row is also healed here on first
// probe after this code lands -- the unconditional recompute is intentional.
if fresh.PollIntervalOverride == nil {
    fresh.PollClass = domain.ClassifyPollClass(fresh.DeviceType)
}
fresh.Status = domain.DeviceStatusUp
```

No other changes to `probeDevice()`. `DeviceUpdate` struct is unchanged — `DeviceType` is still not user-mutable through the API.

**Ordering confirmed:** Line 212 (DeviceType) → Line 217 (guard) → Line 218 (ClassifyPollClass) → Line 220 (Status) — recompute happens after device_type is set, before Update is called.

### Modified: `internal/service/device_service_test.go`

Added `intPtr(v int) *int` helper (mirrors existing `strPtr` pattern) and three new test functions:

| Test | Setup | Expected |
|------|-------|----------|
| `TestProbeDevice_ReclassifyOnTypeChange` | DeviceType=unknown, PollClass=standard, override=nil; discover returns router | DeviceType=router, PollClass=core, override=nil |
| `TestProbeDevice_RespectsPollIntervalOverride` | DeviceType=unknown, PollClass=standard, override=15; discover returns router | DeviceType=router (propagates), PollClass=standard (unchanged), override=15 (intact) |
| `TestProbeDevice_NoTypeChangeStillSyncsPollClassWhenEmpty` | DeviceType=router, PollClass="" (legacy row), override=nil; discover returns router | PollClass=core (healed unconditionally) |

All three tests call `probeDevice()` directly via `probeWg.Add(1)` / goroutine / `WaitForProbes()` pattern, mirroring the existing test fixture style.

### Modified: `.planning/ROADMAP.md`

Phase 39 success criterion 2 replaced. Old text (4-interval model with the removed AP=120s):

```
Devices are auto-classified into polling profiles by device type on first probe
(router=30s, switch=60s, AP=120s, virtual=300s) with a configurable default
fallback for unknown types
```

New text (3-bucket PollClass model):

```
Devices are auto-classified into one of three poll classes based on
device_type, with the following performance-tier intervals:
- core = 30s (router, switch)
- standard = 60s (ap, unknown)
- low = 300s (virtual)
Operators may set a per-device poll_interval_override (seconds, performance
tier only) to bypass the class default. Operational-class polling
(sysUpTime, ifOperStatus) defaults to 60s system-wide; static-class
polling defaults to 300s system-wide. Both are system-defined in v1.5.3
and become configurable in a later milestone.
```

No other success criteria or phase blocks modified. `**Requirements**: POLL-03, POLL-05` line unchanged.

## Commits

| Task | Commit | Hash | Files |
|------|--------|------|-------|
| 1 | feat(39-04): auto-reclassify poll_class in probeDevice with override guard (D-19) | `735fe66` | device_service.go, device_service_test.go |
| 2 | docs(39-04): update ROADMAP §Phase 39 success criterion 2 to 3-bucket PollClass (D-09) | `257a398` | .planning/ROADMAP.md |

## Verification Results

```
go build ./...                          → Success
go test ./internal/service/ -count=1   → 55 passed (was 52 before plan)
go test ./internal/domain/ -count=1    → 16 passed (no regressions)

New tests added: 3
Total service tests: 55

grep -nE 'fresh\.PollClass.*ClassifyPollClass'  → line 218 (exactly 1)
grep -nE 'PollIntervalOverride.*==.*nil'        → line 217 (exactly 1 in probeDevice)
grep -c DeviceType (DeviceUpdate struct)        → 0 (struct unchanged)

ROADMAP Phase 39 block:
  120s occurrences: 0    (old AP=120s gone)
  core.*30 matches: 1    (core = 30s present)
  standard.*60 matches: 1 (standard = 60s present)
  low.*300 matches: 1    (low = 300s present)
  poll_interval_override: 2 (present in success criterion)
  router: 1, virtual: 1  (DeviceType mapping present)
  Requirements: POLL-03, POLL-05 (untouched)
```

## Deviations from Plan

None — plan executed exactly as written. The 6-line D-19 hook was inserted at the precise location specified. Tests mirror the mock injection style of existing `TestProbeDevice_*` tests. ROADMAP criterion 2 contains all required facts in the specified format.

## Known Stubs

None. All wiring is complete: the hook calls `ClassifyPollClass` (Plan 39-01), which is consumed by the DeviceRepo `Update` (Plan 39-03). Full round-trip from SNMP probe to persisted poll_class is now complete.

## Threat Flags

No new network endpoints, auth paths, or trust boundary crossings introduced.

- T-39-11 (Tampering via malicious SNMP responder influencing DeviceType/PollClass): pre-existing risk, not enlarged by this plan. Accept disposition unchanged.
- T-39-12 (DoS via device flapping device_type between core/low): mitigated — the guard is a single in-process map operation per probe, no cascading writes.
- T-39-13 (EoP via poll_interval_override API write): deferred to Phase 40/42 when API accepts override writes. No new API surface added here.
- T-39-14 (Information disclosure via ROADMAP edit): accept — planning artifact, no secrets.

## Self-Check: PASSED

- [x] `internal/service/device_service.go` contains `ClassifyPollClass(fresh.DeviceType)` at line 218 — FOUND
- [x] `internal/service/device_service.go` contains `PollIntervalOverride == nil` guard at line 217 — FOUND
- [x] `internal/service/device_service_test.go` contains `TestProbeDevice_ReclassifyOnTypeChange` — FOUND
- [x] `internal/service/device_service_test.go` contains `TestProbeDevice_RespectsPollIntervalOverride` — FOUND
- [x] `internal/service/device_service_test.go` contains `TestProbeDevice_NoTypeChangeStillSyncsPollClassWhenEmpty` — FOUND
- [x] `internal/service/device_service_test.go` contains `intPtr` helper — FOUND
- [x] `.planning/ROADMAP.md` Phase 39 criterion 2 contains "core = 30s", "standard = 60s", "low = 300s" — FOUND
- [x] `.planning/ROADMAP.md` Phase 39 criterion 2 contains "poll_interval_override", "router", "virtual" — FOUND
- [x] `.planning/ROADMAP.md` Phase 39 criterion 2 does NOT contain "120s" — VERIFIED (0 occurrences)
- [x] `DeviceUpdate` struct has no `DeviceType` field — VERIFIED (grep returns 0)
- [x] Commit `735fe66` exists — FOUND
- [x] Commit `257a398` exists — FOUND
- [x] `go build ./...` → Success — VERIFIED
- [x] `go test ./internal/service/ -count=1` → 55 passed — VERIFIED
- [x] `go test ./internal/domain/ -count=1` → 16 passed — VERIFIED
