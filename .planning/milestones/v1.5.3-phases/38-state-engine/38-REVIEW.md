---
phase: 38-state-engine
reviewed: 2026-04-12T00:00:00Z
depth: standard
files_reviewed: 4
files_reviewed_list:
  - internal/state/store.go
  - internal/state/health.go
  - internal/state/health_test.go
  - internal/state/store_test.go
findings:
  critical: 0
  warning: 5
  info: 7
  total: 12
status: issues_found
---

# Phase 38: Code Review Report

**Reviewed:** 2026-04-12
**Depth:** standard
**Files Reviewed:** 4
**Status:** issues_found

## Summary

Phase 38 delivers `internal/state/` — a thread-safe in-memory runtime state store
with hysteresis-based health evaluation, a three-strike soft/hard reachability
state machine, staleness ticking, and diff-based change emission on a
non-blocking channel. The implementation is well-documented, test coverage
is good, and the concurrency design (single RWMutex, emit-outside-lock,
non-blocking fan-out) matches the patterns agreed in `38-RESEARCH.md`.

No critical security or data-loss issues were found. However, there are
five correctness warnings that should be addressed before this store is
wired into the pipeline orchestrator in a later phase:

1. **Goroutine lifecycle bug**: `Stop()` followed by `Start()` will panic
   because `s.done` is never re-created after it is closed by the previous
   staleness goroutine.
2. **TOCTOU race in `Start`/`Stop`**: `s.cancel` is checked and written
   without a mutex, so concurrent lifecycle calls may double-launch or skip
   goroutine startup.
3. **`HealthStatusHealthy` reported for devices with no metrics**: a
   successful poll with all-nil metric pointers produces `Health=healthy`
   instead of `unknown`, because `aggregateHealth` treats the empty-string
   zero value as equivalent to OK.
4. **Health not re-evaluated on soft/hard-down → up transition when metrics
   are nil**: a `PollSuccess=true` update with `Metrics=nil` reaches
   Reachability=Up but leaves Health frozen at the pre-outage value with no
   indication it was not re-evaluated.
5. **Missing `Metrics.CollectedAt` / `DeviceID` in diff equality**: if the
   metric values are byte-identical but `CollectedAt` advances, the store
   silently drops the update, so `Snapshot()` will return a stale
   `CollectedAt` — which may surprise downstream consumers that rely on the
   freshness of the embedded `DeviceMetrics`.

The remaining items are informational: dead code, an incorrect comment, and
minor test robustness improvements.

## Warnings

### WR-01: `Stop()` followed by `Start()` will panic on `close(s.done)`

**File:** `internal/state/store.go:247-265`
**Issue:**
`NewStore` creates `s.done = make(chan struct{})` once. The staleness
goroutine in `runStaleness` calls `defer close(s.done)`. `Stop()` cancels
the context, waits for the goroutine to finish (`<-s.done`), and clears
`s.cancel = nil` — but it does **not** re-create `s.done`.

If a caller then invokes `Start(ctx)` again, the `s.cancel != nil` guard
passes (it's nil), a new goroutine is launched, and its `defer close(s.done)`
will panic with `close of closed channel` when it eventually exits. The
`panic("state.Store: Start called more than once")` that the docstring
promises only fires for two back-to-back `Start` calls with no `Stop`
in between — restart-after-stop is silently broken.

The docstring on `Start` says "Calling Start more than once on the same
Store is not supported", so the contract permits panicking here, but the
second panic is obscure (channel close, stack deep inside the goroutine)
and happens only on the *next* tick, not on the call site.

**Fix:** Either reject restart explicitly in `Stop()` by leaving `s.cancel`
set (e.g., use a separate `started bool` guard), or re-create `s.done` in
`Stop()` after the goroutine drains so the store is usable again:

```go
func (s *Store) Stop() {
    if s.cancel == nil {
        return
    }
    s.cancel()
    <-s.done
    s.cancel = nil
    s.done = make(chan struct{}) // allow future Start()
}
```

If restart is genuinely unsupported, make the failure mode loud at the
call site rather than deep in the tick loop:

```go
func (s *Store) Start(ctx context.Context) {
    if s.cancel != nil {
        panic("state.Store: Start called more than once")
    }
    select {
    case <-s.done:
        panic("state.Store: Start after Stop is not supported")
    default:
    }
    // ...
}
```

### WR-02: TOCTOU race on `Start`/`Stop` lifecycle

**File:** `internal/state/store.go:247-265`
**Issue:**
Both `Start` and `Stop` read and write `s.cancel` without holding
`s.mu` (or any other mutex). Concurrent calls — e.g., two
goroutines racing to `Start` during bootstrap, or a `Stop` racing with a
delayed `Start` — can either double-spawn the staleness goroutine (both
see `s.cancel == nil`) or silently no-op a `Stop`.

The store is otherwise thread-safe, so it is reasonable for callers to
assume lifecycle methods are also safe to call from multiple goroutines.

**Fix:** Protect lifecycle transitions with a dedicated mutex (or reuse
`s.mu`), or document clearly that `Start`/`Stop` must be called from a
single goroutine:

```go
type Store struct {
    // ...
    lifecycleMu sync.Mutex
    cancel      context.CancelFunc
    done        chan struct{}
}

func (s *Store) Start(ctx context.Context) {
    s.lifecycleMu.Lock()
    defer s.lifecycleMu.Unlock()
    if s.cancel != nil {
        panic("state.Store: Start called more than once")
    }
    derived, cancel := context.WithCancel(ctx)
    s.cancel = cancel
    go s.runStaleness(derived)
}

func (s *Store) Stop() {
    s.lifecycleMu.Lock()
    cancel := s.cancel
    s.cancel = nil
    s.lifecycleMu.Unlock()
    if cancel == nil {
        return
    }
    cancel()
    <-s.done
}
```

### WR-03: All-nil metrics on a successful poll produce `Health=healthy`, not `unknown`

**File:** `internal/state/health.go:101-116`, `internal/state/store.go:170-180`
**Issue:**
`aggregateHealth` uses `MetricSeverity` comparisons:

```go
for _, s := range severities {
    if s == MetricSeverityCritical { return HealthStatusCritical }
    if s == MetricSeverityWarning { hasWarning = true }
}
if hasWarning { return HealthStatusWarning }
return HealthStatusHealthy
```

The zero value of `MetricSeverity` (type alias of `string`) is `""`, not
`MetricSeverityOK`. For a device where the first successful poll returns
`CPUPercent=nil`, `MemPercent=nil`, `TempCelsius=nil` (all three nil),
`evaluateHealth` skips all three `evaluateMetricSeverity` calls and leaves
severities as `""`. `aggregateHealth` then falls through both conditionals
and returns `HealthStatusHealthy`.

This is confirmed by `TestHealth_NilMetricsDoNotPanic` in `health_test.go:82-94`,
which actively asserts the current (wrong) behavior:

```go
if s.Health != HealthStatusHealthy {
    t.Errorf("Health = %q, want %q (all nil => remain healthy)", ...)
}
```

Reporting "healthy" for a device whose health we know nothing about is
misleading — especially given the dashboard will surface this state to
operators. The store already defines `HealthStatusUnknown` and the
`Update()` fallback at `store.go:178-180` normalizes empty Health to
Unknown, but that branch only fires on the first-observation *failure*
path; it is unreachable when `evaluateHealth` was already called and set
Health=Healthy.

**Fix:** Treat "no observations yet" as distinct from "all observations
are OK". Either:

(a) Return `HealthStatusUnknown` when every severity is the empty string:

```go
func aggregateHealth(cpu, mem, temp MetricSeverity) HealthStatus {
    severities := []MetricSeverity{cpu, mem, temp}
    allEmpty := true
    hasWarning := false
    for _, s := range severities {
        if s == MetricSeverityCritical {
            return HealthStatusCritical
        }
        if s != "" {
            allEmpty = false
        }
        if s == MetricSeverityWarning {
            hasWarning = true
        }
    }
    if allEmpty {
        return HealthStatusUnknown
    }
    if hasWarning {
        return HealthStatusWarning
    }
    return HealthStatusHealthy
}
```

(b) Or pre-initialize severities to `MetricSeverityOK` in `DeviceState`
only when at least one metric has been observed; stay empty otherwise.

Update `TestHealth_NilMetricsDoNotPanic` accordingly — the assertion is
currently locking in the bug.

### WR-04: Soft/hard-down → Up transition with `Metrics=nil` leaves Health frozen at pre-outage value

**File:** `internal/state/store.go:149-180`
**Issue:**
The `Update` logic is:

```go
if u.PollSuccess {
    next.Reachability = ReachabilityUp
    next.ConsecutiveFailures = 0
}
// ...
if next.Reachability == ReachabilityUp && u.Metrics != nil {
    next.Metrics = cloneMetrics(*u.Metrics)
    evaluateHealth(&next, &next.Metrics)
}
```

Consider this sequence:
1. Poll succeeds with `CPUPercent=95` → Health=Critical (via hysteresis)
2. 3 polls fail → Reachability=hard_down, Health frozen at Critical (by design)
3. Poll succeeds but caller passes `u.Metrics=nil` (e.g., transport
   succeeded but metric extraction returned empty)

After step 3, Reachability correctly moves to Up and failures reset, but
`evaluateHealth` is **skipped** because `u.Metrics == nil`, so Health
remains Critical from step 1. An operator sees "device is UP" with
"Critical health" and no explanation that the health data is stale.

Whether this is intentional depends on the caller contract. The docstring
for `StateUpdate.Metrics` says "nil allowed if PollSuccess=false", which
implicitly claims `Metrics` is non-nil when `PollSuccess=true`. If that
contract is strict, add a defensive assertion or comment. If it is lax
(pipeline code may legitimately send success without metrics), the store
should either:
  - Treat `PollSuccess=true && Metrics=nil` as "no new metric info" and
    reset Health to Unknown, or
  - Reject the update and log a warning.

**Fix:** Pick one of the two behaviors and document it. At minimum, add
an explicit branch:

```go
if next.Reachability == ReachabilityUp {
    if u.Metrics != nil {
        next.Metrics = cloneMetrics(*u.Metrics)
        evaluateHealth(&next, &next.Metrics)
    } else {
        // Poll succeeded but no metric payload — reset to unknown rather
        // than preserving a stale frozen health from a pre-outage reading.
        next.CPUSeverity = ""
        next.MemSeverity = ""
        next.TempSeverity = ""
        next.Health = HealthStatusUnknown
    }
}
```

Add a test case for this path.

### WR-05: `deviceStateEqual` ignores `Metrics.CollectedAt` and `Metrics.DeviceID`

**File:** `internal/state/store.go:349-384`
**Issue:**
The diff equality compares `CPUPercent`, `MemPercent`, `TempCelsius`,
`UptimeSecs` by dereferenced value, but not `Metrics.CollectedAt` or
`Metrics.DeviceID`. If two consecutive polls yield byte-identical metric
values at different wall times, the store:
  1. Does not update `s.devices[u.DeviceID]` (the `changed` flag is false)
  2. Does not emit a change on the Changes channel

Consequence: `Snapshot()` will return the **previous** `CollectedAt`
timestamp until a metric value actually changes. Downstream consumers
that compare `Metrics.CollectedAt` against "now" to detect freshness
(the most obvious way to use the field) will see a stale timestamp.

`LastPolledAt` is tracked separately on `DeviceState` and is updated on
every poll, so consumers that use `DeviceState.LastPolledAt` are unaffected.
This is likely the intended design — but the store's public API exposes
`Metrics.CollectedAt` via Snapshot, so the discrepancy deserves either
a fix or an explicit note in the doc comment.

**Fix (option 1, preferred):** Always overwrite `next.Metrics` on
Reachability=Up, regardless of the `changed` decision, so `CollectedAt`
tracks the latest poll:

```go
if next.Reachability == ReachabilityUp && u.Metrics != nil {
    next.Metrics = cloneMetrics(*u.Metrics)
    evaluateHealth(&next, &next.Metrics)
}
// ... diff check as today, but write on every update, not just on changed
s.devices[u.DeviceID] = next
```

The diff would then only control whether `emitChanges` fires, not whether
the map is mutated. This costs one map write per update cycle (cheap)
but keeps `CollectedAt` accurate.

**Fix (option 2):** Document that `DeviceMetrics.CollectedAt` returned by
`Snapshot()` is "the timestamp of the last metric change, not the last
poll" and steer consumers to `DeviceState.LastPolledAt` for freshness.

## Info

### IN-01: Dead code — defensive `!existed` branch in failure counter

**File:** `internal/state/store.go:152-159`
**Issue:**
```go
next.ConsecutiveFailures = prev.ConsecutiveFailures + 1
if !existed {
    // defensive: no-op today (zero-value prev already yields 1) but
    // future-proof if DeviceState initialization ever changes so the
    // first-observation failure count is explicit.
    next.ConsecutiveFailures = 1
}
```

The comment correctly identifies this as a no-op. Dead defensive code
tends to rot — a future refactor may change the assignment silently.
**Fix:** Either remove it or strengthen it into a real invariant check:

```go
if !existed && next.ConsecutiveFailures != 1 {
    log.Printf("state: first-observation failure count expected 1, got %d",
        next.ConsecutiveFailures)
}
```

### IN-02: `evaluateHealth` has a dead nil-metrics branch

**File:** `internal/state/health.go:83-96`, `internal/state/store.go:174`
**Issue:**
`evaluateHealth` is called only from `Store.Update` with
`evaluateHealth(&next, &next.Metrics)`. The second argument is always the
address of a value-type field, so it can never be nil. The top-level
`if metrics != nil` branch is dead.

**Fix:** Remove the outer nil check — the per-pointer-field nil checks
inside the block already handle all realistic cases:

```go
func evaluateHealth(state *DeviceState, metrics *domain.DeviceMetrics) {
    if metrics.CPUPercent != nil {
        state.CPUSeverity = evaluateMetricSeverity(*metrics.CPUPercent, state.CPUSeverity, defaultThresholds["cpu"])
    }
    // ...
}
```

Or, if keeping the defensive check, call it from the test too — tests
only pass non-nil DeviceMetrics pointers.

### IN-03: `Update` does not validate `u.Timestamp`

**File:** `internal/state/store.go:142-146`
**Issue:**
`next.LastPolledAt = u.Timestamp` unconditionally — a caller that forgets
to set `Timestamp` writes the zero time, and the very next `markStale`
tick will mark the device stale (because `time.Time{}.Add(...)` is in
the year 0001, far in the past of `now`).

**Fix:** Fall back to `time.Now()` when `u.Timestamp.IsZero()`, or reject
the update with a log warning:

```go
ts := u.Timestamp
if ts.IsZero() {
    ts = time.Now()
}
next.LastPolledAt = ts
```

### IN-04: Docstring claim "Each send is a slice of all device IDs changed in a single Update cycle" is inaccurate

**File:** `internal/state/store.go:236-241`
**Issue:**
The `Changes()` docstring says "Each send is a slice of all device IDs
changed in a single Update cycle". In practice:
  - `Update` always emits exactly one element (the single device it
    operates on).
  - Only `markStale` can emit a multi-element slice.

A reader expecting the store to batch multiple Update() calls into one
slice will be surprised.

**Fix:** Reword to match reality:

```go
// Changes returns a receive-only channel that emits batches of device
// UUIDs whose state has changed. Update() emits a 1-element slice per
// call; the background staleness goroutine emits one slice per tick
// containing every device newly marked stale (may be multi-element).
```

### IN-05: Test drain goroutine in `TestStore_ConcurrentUpdateAndSnapshot` has a non-deterministic shutdown

**File:** `internal/state/store_test.go:22-33`
**Issue:**
```go
drainDone := make(chan struct{})
go func() {
    for {
        select {
        case <-s.Changes():
        case <-drainDone:
            return
        }
    }
}()
defer close(drainDone)
```

When both cases of the `select` are ready (common at the end of the
test when the channel still has backlog and `drainDone` is closed), Go
picks pseudo-randomly. The test may return while Update() is still
attempting non-blocking sends — not a race in the store, but the test
is noisier than necessary. Also, `close(drainDone)` runs at function
exit but there is no `sync.WaitGroup` ensuring the drain goroutine has
actually returned before the test reports PASS. Subsequent tests sharing
the binary (sequential by default) are unaffected, but `go test -race
-count=10` occasionally reports spurious reads if the runtime schedules
the drainer during binary teardown.

**Fix:** Track the drain goroutine with a WaitGroup and have the main
wg.Wait() include it:

```go
drainDone := make(chan struct{})
var drainWg sync.WaitGroup
drainWg.Add(1)
go func() {
    defer drainWg.Done()
    for {
        select {
        case <-s.Changes():
        case <-drainDone:
            return
        }
    }
}()
defer drainWg.Wait()
defer close(drainDone)
```

### IN-06: `TestHealth_NilMetricsDoNotPanic` assertion encodes the WR-03 bug

**File:** `internal/state/health_test.go:82-94`
**Issue:**
The test actively asserts `s.Health != HealthStatusHealthy`  →  error.
This test will need to be updated along with the fix for WR-03. Flagged
here because it is easy to miss during the fix pass.

**Fix:** Update the expected value to `HealthStatusUnknown` (or whatever
WR-03's resolution chooses) when that issue is addressed. No change
until WR-03 is decided.

### IN-07: `Remove` emits a change indistinguishable from an Update

**File:** `internal/state/store.go:223-234`
**Issue:**
When a device is removed, `emitChanges([]uuid.UUID{id})` sends the same
signal shape as an Update. Consumers must re-read via `Snapshot()` /
`GetDevice()` to discover that the device is gone. This is consistent
with the documented "rebuild from snapshot" model, but the store does
not emit any sentinel to hint at deletion — which means consumers that
maintain their own map must check `GetDevice(id) -> (_, false)` on
*every* change to detect a removal.

**Fix:** Either (a) accept the current design and note it clearly in the
Remove docstring so consumers remember to handle missing devices, or
(b) define a separate channel or message type for removals. Not a bug,
but worth documenting explicitly.

---

_Reviewed: 2026-04-12_
_Reviewer: Claude (gsd-code-reviewer)_
_Depth: standard_
