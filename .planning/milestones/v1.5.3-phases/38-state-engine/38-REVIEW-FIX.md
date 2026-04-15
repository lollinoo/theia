---
phase: 38-state-engine
fixed_at: 2026-04-12T00:00:00Z
review_path: .planning/phases/38-state-engine/38-REVIEW.md
iteration: 1
findings_in_scope: 5
fixed: 5
skipped: 0
status: all_fixed
---

# Phase 38: Code Review Fix Report

**Fixed at:** 2026-04-12
**Source review:** `.planning/phases/38-state-engine/38-REVIEW.md`
**Iteration:** 1

**Summary:**
- Findings in scope: 5 (WR-01 through WR-05; critical+warning)
- Fixed: 5
- Skipped: 0

All five warning-level findings from the review were fixed and committed
atomically. Each fix was verified by `go build ./internal/state/...` and
`go test ./internal/state/...` (38 tests pass, including 2 new regression
tests added for WR-04 and WR-05). The full suite also passes with the
race detector enabled (`-race -count=2` → 76 pass).

IN-06 flagged that `TestHealth_NilMetricsDoNotPanic` encodes the WR-03
bug; per the cross-reference in the prompt, that test was updated as
part of the WR-03 commit even though IN-06 itself is out of scope.

## Fixed Issues

### WR-01: `Stop()` followed by `Start()` will panic on `close(s.done)`

**Files modified:** `internal/state/store.go`
**Commit:** `9fd0382`
**Applied fix:** `Stop()` now re-creates `s.done = make(chan struct{})`
after awaiting the staleness goroutine. This makes the Store reusable:
a subsequent `Start()` spawns a fresh goroutine whose
`defer close(s.done)` operates on a newly-allocated channel instead of
panicking on the previously closed one. Docstring updated to document
the contract.

### WR-02: TOCTOU race on `Start`/`Stop` lifecycle

**Files modified:** `internal/state/store.go`
**Commit:** `ee6b900`
**Applied fix:** Added a dedicated `lifecycleMu sync.Mutex` field to
`Store` and held it for the full duration of both `Start()` and `Stop()`.
This serializes concurrent lifecycle transitions, eliminating the
double-spawn and silent-no-op races on `s.cancel`. Holding the lock
across the entire `Stop` sequence also closes a window in which a
racing caller could observe `s.cancel == nil` alongside a closed
`s.done`. Docstrings updated to note that the methods are safe to
call concurrently.

### WR-03: All-nil metrics on a successful poll produce `Health=healthy`, not `unknown`

**Files modified:** `internal/state/health.go`, `internal/state/health_test.go`
**Commit:** `a56e120`
**Applied fix:** `aggregateHealth` now tracks an `allEmpty` flag across
the three severities. If every severity is the empty-string zero value
(i.e. no metric has been evaluated yet), the function returns
`HealthStatusUnknown` instead of falling through to `HealthStatusHealthy`.
Existing "partial nil" cases — e.g. only Memory reported — still classify
correctly because at least one severity is non-empty.

Also updated `TestHealth_NilMetricsDoNotPanic` in `health_test.go` to
assert `HealthStatusUnknown` instead of `HealthStatusHealthy`. This
pulls in IN-06, which explicitly documents that the original assertion
was locking in the WR-03 bug and must be updated alongside the fix.

**Manual-verification note:** This change alters an observable contract
(`aggregateHealth` output for all-empty inputs). The existing worst-of
and hysteresis tests still pass, but downstream consumers of Health
surfaces should be re-verified when the store is wired into the
pipeline orchestrator in a later phase.

### WR-04: Soft/hard-down → Up transition with `Metrics=nil` leaves Health frozen

**Files modified:** `internal/state/store.go`, `internal/state/store_test.go`
**Commit:** `2d53dda`
**Applied fix:** In `Store.Update`, the `Reachability == Up` branch now
handles two sub-cases:
  - `u.Metrics != nil`: deep-copy metrics and re-evaluate health (unchanged).
  - `u.Metrics == nil`: clear `next.Metrics` and all per-metric
    severities, and set `next.Health = HealthStatusUnknown`.

This prevents a hard_down → up transition from leaving operators with a
stale "Critical" health from before the outage. Added
`TestReachability_HardDownToUpWithNilMetricsResetsHealthToUnknown` in
`store_test.go` which drives the Critical → hard_down → Up-with-nil
sequence and asserts Health resets to Unknown and all severities clear.

### WR-05: `deviceStateEqual` ignores `Metrics.CollectedAt` / `Metrics.DeviceID`

**Files modified:** `internal/state/store.go`, `internal/state/store_test.go`
**Commit:** `d47e5f1`
**Applied fix:** `Store.Update` now always writes
`s.devices[u.DeviceID] = next` regardless of the `changed` diff result.
The `changed` flag continues to gate the `Changes()` channel emission,
so subscribers still do not see spurious no-op notifications, but
`Snapshot()` now always returns a fresh `Metrics.CollectedAt` even when
two consecutive polls yield byte-identical metric values.

Rationale for choosing option 1 (always write) over option 2 (document
stale CollectedAt): downstream consumers that rely on
`Metrics.CollectedAt` for freshness are best served by correctness-first
semantics, and the map write is cheap (one struct copy per poll cycle).

Added `TestSnapshot_CollectedAtAdvancesOnIdenticalMetricValues` which
polls the store twice with the same CPU value but advancing CollectedAt
and asserts the Snapshot reflects the latest timestamp. The existing
`TestChanges_UnchangedDeviceDoesNotEmit` also still passes (it uses a
fixed timestamp, so no false emission occurs).

---

_Fixed: 2026-04-12_
_Fixer: Claude (gsd-code-fixer)_
_Iteration: 1_
