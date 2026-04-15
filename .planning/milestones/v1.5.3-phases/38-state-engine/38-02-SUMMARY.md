---
phase: 38-state-engine
plan: 02
subsystem: backend-state
tags: [go, concurrency, sync-rwmutex, race-detector, state-machine, diff-emission, staleness-tick, reachability-frozen-health]

# Dependency graph
requires:
  - phase: 38-01
    provides: internal/state type foundation — HealthStatus/ReachabilityStatus/MetricSeverity enums, DeviceState / StateUpdate / Store skeleton with mu/devices/changes/cancel/done fields, NewStore constructor, and lock-agnostic evaluateHealth / evaluateMetricSeverity / aggregateHealth pure functions with hardcoded 70/60/90/80 hysteresis thresholds
provides:
  - Store.Update(StateUpdate) — reachability state machine (1 fail=soft, 3=hard, 1 success=up), frozen health on unreachable (D-02), field-by-field diff with change emission outside the lock
  - Store.Snapshot() map[uuid.UUID]DeviceState — RLock atomic point-in-time read with deep-copied *float64 metric pointers (D-06, Pitfall 6)
  - Store.GetDevice(uuid.UUID) (DeviceState, bool) — single-device deep-copied read under RLock
  - Store.Remove(uuid.UUID) — deletes device and emits removal on Changes channel
  - Store.Changes() <-chan []uuid.UUID — receive-only notify channel carrying batches of changed device UUIDs
  - Store.Start(context.Context) / Store.Stop() — context-cancellable staleness goroutine lifecycle with hardcoded 5s tick
  - markStale(time.Time) — lock-safe staleness pass marking any device where LastPolledAt + 2*ExpectedInterval < now
  - emitChanges — non-blocking select/default send pattern (Pitfall 1, matches main.go topologyNotify)
  - cloneMetrics / deviceStateEqual / floatPtrEqual — zero-dependency deep-copy and field-by-field diff helpers (no reflect)
affects: [39-volatility-classes, 40-jittered-scheduler, 42-pipeline-cutover, 43-detail-on-demand, 44-freshness-ui]

# Tech tracking
tech-stack:
  added: []  # D-14 satisfied — zero new third-party dependencies
  patterns:
    - "Lock-and-emit-outside-lock boundary rule (Pitfall 1): mutate state under s.mu, release, then send on changes channel via non-blocking select/default"
    - "Field-by-field DeviceState diff (A2): no reflect, compares pointer dereferences nil-safely via floatPtrEqual"
    - "Deep-copied Snapshot via cloneMetrics: each returned DeviceState owns independent *float64 pointers (Pitfall 6)"
    - "Context-cancellable background tick with defer close(done) + Stop() wait pattern (T-38-03)"
    - "Reachability state machine inside Update: PollSuccess→Up, count++ at fail, ≥3=HardDown"
    - "Frozen-health-on-unreachable invariant enforced by `if next.Reachability == ReachabilityUp` guard before evaluateHealth"

key-files:
  created:
    - internal/state/store_test.go
  modified:
    - internal/state/store.go

key-decisions:
  - "Staleness tick interval hardcoded at 5 * time.Second — responsive enough for freshness UI indicators without dominating CPU (Claude's discretion per CONTEXT.md)"
  - "Changes channel buffer size 32 (already set in Plan 01) — matches ws.Hub broadcast channel (A3 in research); verified under load by TestStore_ConcurrentUpdateAndSnapshot which legitimately exercises the drop path"
  - "Update() deep-copies u.Metrics via cloneMetrics before calling evaluateHealth so that external mutations of the caller's DeviceMetrics cannot corrupt store state — extends Pitfall 6 protection from Snapshot inbound to Update as well"
  - "Start() panics on second call rather than no-op — surfaces misuse early per plan instruction ('executor's choice'); Stop() is idempotent and safe even if Start was never invoked"
  - "Health normalized to HealthStatusUnknown when a first-observation failure leaves Health as the empty-string zero value — prevents '' from leaking into the Changes channel as a valid but meaningless enum"
  - "Field-by-field diff in deviceStateEqual compares LastPolledAt via time.Time.Equal (not ==) so wall-clock equality survives monotonic-clock stripping (matters for TestChanges_UnchangedDeviceDoesNotEmit which uses time.Unix fixed timestamps)"

patterns-established:
  - "Pattern: Three-stage mutation — (1) acquire Lock, (2) compute next DeviceState and changed-flag under lock, (3) release Lock, (4) emitChanges outside lock — prevents Pitfall 1 lock-during-send deadlock. Applied to Update, Remove, markStale uniformly."
  - "Pattern: Snapshot deep-copy via per-row cloneMetrics call inside the RLock-held range loop — every returned DeviceState gets independent *float64 pointers at O(n) cost, matching ws.CloneSnapshot idiom."
  - "Pattern: Staleness background goroutine with deferred close(done) + context.WithCancel + ticker.Stop — matches cmd/theia Poller/MetricsCollector lifecycle; Stop() waits on done channel to prove no leak."
  - "Pattern: Non-blocking select/default send for change notifications — identical to topologyNotify in main.go. Drop is acceptable because Snapshot() recovers the full state on the consumer's next read."

requirements-completed:
  - STATE-01
  - STATE-04
  - STATE-05

# Metrics
duration: 3m 25s
completed: 2026-04-12
---

# Phase 38 Plan 02: Concurrent Store Methods & Staleness Tick Summary

**Full Store API (Update/Snapshot/GetDevice/Changes/Remove/Start/Stop/markStale) built on Plan 01's type foundation, with 17 new race-safe tests verifying STATE-01/04/05, the soft/hard-down state machine with frozen-health-on-unreachable invariant, diff-suppressed change emission, and clean staleness goroutine shutdown — all 25 state-package tests + 11 hysteresis sub-tests pass under `go test -race`.**

## Performance

- **Duration:** 3m 25s
- **Started:** 2026-04-12T09:24:05Z
- **Completed:** 2026-04-12T09:27:30Z
- **Tasks:** 2
- **Files changed:** 2 (1 modified, 1 created)
- **Lines added:** 283 in store.go + 446 in store_test.go = 729 lines

## Accomplishments

- `Store.Update` implements the full reachability state machine per STATE-04: `PollSuccess=true` → Up + counter reset; `PollSuccess=false` → counter increment, SoftDown at count 1–2, HardDown at count ≥3. The D-02 frozen-health invariant is enforced by a single `if next.Reachability == ReachabilityUp` guard that prevents evaluateHealth from running when unreachable, so health freezes at the last known severity automatically.
- `Store.Snapshot` and `Store.GetDevice` return independent deep copies — the map is freshly allocated and every `*float64` metric pointer is re-allocated via `cloneMetrics`. Mutating the returned DeviceState (including through `*ds.Metrics.CPUPercent = 999`) cannot corrupt store internals, proven by TestStore_SnapshotIsDeepCopy.
- `Store.Remove` deletes a device under Lock and emits its UUID on Changes after Unlock (emit-outside-lock invariant preserved).
- `Store.Changes()` returns a receive-only channel. Consumers read changed IDs and rebuild WS payloads from Snapshot — the state engine stays decoupled from WS serialization per D-04.
- `Store.Start`/`Store.Stop` manage a hardcoded 5-second staleness goroutine via context.WithCancel. Stop cancels the context and blocks on `s.done`, guaranteeing no goroutine leak (verified by TestStore_StartStopIsCleanShutdown within a 2-second timeout).
- `markStale` iterates all devices under Lock, sets `Stale=true` where `LastPolledAt + 2*ExpectedInterval < now`, and emits newly-stale UUIDs after releasing the lock. Update() clears the Stale flag on every fresh poll, so a marked-stale device instantly un-stales the moment real data arrives (Pitfall 4 avoidance — demonstrated by TestStaleness_UpdateClearsStaleFlag).
- `emitChanges` uses the non-blocking `select { case s.changes <- ids: default: log.Printf(...) }` pattern exactly mirroring cmd/theia/main.go's `topologyNotify`. When the 32-slot buffer overflows (as in TestStore_ConcurrentUpdateAndSnapshot's 1600-message burst), batches drop and the test log shows "state: changes channel full, 1 device change(s) dropped" — a feature, not a bug, because consumers rebuild from Snapshot().
- `deviceStateEqual` compares DeviceState field-by-field without reflect. Pointer fields in DeviceMetrics use `floatPtrEqual` for nil-safe dereference comparison. Time fields use `time.Time.Equal` to survive monotonic-clock stripping — essential for TestChanges_UnchangedDeviceDoesNotEmit which uses a fixed `time.Unix(1_700_000_000, 0)` timestamp on both updates.
- 17 new test functions covering: 2 concurrency (TestStore_ConcurrentUpdateAndSnapshot, TestStore_SnapshotIsDeepCopy), 6 reachability (single/two/three failures, success reset, frozen health on soft-down, frozen health on hard-down), 3 change emission (first always emits, unchanged does NOT emit, changed emits), 3 staleness (marks after threshold, fresh not marked, Update clears flag), 2 lifecycle (Start/Stop clean shutdown, Stop without Start no-op), 1 removal (Remove deletes and emits).

## Task Commits

Each task was committed atomically on branch `gsd/v1.5.3-snmp-pipeline-architecture`:

1. **Task 1: Implement Store methods on store.go** — `965e63b` (feat)
2. **Task 2: Write store_test.go — concurrent access, reachability, diff emission, staleness** — `5688a63` (test)

_Note: Task 1 is labeled `tdd="true"` in the plan but in practice produces both the production code and a buildable package; Task 2 delivers the test file. Tests pass on first run against the Task 1 implementation — no RED/GREEN churn was observable because the plan action text specified the exact methods and the test file tests them as specified. This is the same pattern Plan 01 followed per its SUMMARY note._

## Files Created/Modified

- `internal/state/store.go` (was 116 lines → now 397 lines; +283 lines added in Plan 02)
  - Added package-level constant `stalenessTickInterval = 5 * time.Second`
  - Added `log` import for non-blocking emit logging
  - Removed the "Plan 01 only defines the struct" comment; Store doc now reflects full API
  - Appended all methods after NewStore: Update, Snapshot, GetDevice, Remove, Changes, Start, Stop, runStaleness, markStale, emitChanges
  - Appended package helpers: cloneMetrics, deviceStateEqual, floatPtrEqual
  - Preserved all Plan 01 type declarations verbatim (enums, DeviceState, StateUpdate, Store fields, NewStore)

- `internal/state/store_test.go` (new, 446 lines)
  - Package `state`, imports: context, sync, testing, time, uuid, domain
  - Concurrent access section: TestStore_ConcurrentUpdateAndSnapshot (4 writers × 4 readers × 200 ops with background drain goroutine), TestStore_SnapshotIsDeepCopy
  - Reachability section: TestReachability_SinglePollFailureIsSoftDown, TestReachability_TwoFailuresStayedSoftDown, TestReachability_ThreeFailuresIsHardDown, TestReachability_SuccessResetsToUp, TestReachability_HealthFrozenOnSoftDown, TestReachability_HealthFrozenOnHardDown
  - Changes section: TestChanges_FirstUpdateAlwaysEmits, TestChanges_UnchangedDeviceDoesNotEmit (uses fixed time.Unix timestamp), TestChanges_ChangedDeviceEmits
  - Staleness section: TestStaleness_MarksStaleAfterThreshold (past timestamp + 10s interval), TestStaleness_FreshDeviceNotMarked (1h interval), TestStaleness_UpdateClearsStaleFlag (Pitfall 4)
  - Lifecycle section: TestStore_StartStopIsCleanShutdown (2s timeout), TestStore_StopWithoutStartIsNoOp
  - Remove section: TestStore_RemoveDevice

## Decisions Made

- **Staleness tick interval = 5 * time.Second (hardcoded constant).** Per CONTEXT.md this is Claude's discretion. Rationale: users want freshness indicators to react within seconds of a missed poll, but a sub-second tick is wasteful for a check that iterates the full device map. 5s strikes the balance and is comfortably faster than any realistic `ExpectedInterval` (default global poll interval is 15–60s in this codebase).
- **Changes channel buffer size = 32 (inherited from Plan 01).** Matches ws.Hub broadcast channel (A3 research). Verified legitimate under TestStore_ConcurrentUpdateAndSnapshot which runs 1600 Updates through a concurrent drain and shows the drop path works: drops are logged via log.Printf, tests still pass, because consumers recover via Snapshot().
- **Update() deep-copies u.Metrics via cloneMetrics before calling evaluateHealth.** Beyond Pitfall 6 (which only required Snapshot deep-copy), this makes the inbound Update path also tamper-proof: a caller that retains a reference to the passed `*domain.DeviceMetrics` and later mutates `*m.CPUPercent` cannot corrupt the stored state. Not explicitly required by the plan action text, but a trivial extension of Pitfall 6 discipline that future-proofs the API.
- **Start() panics on second call, Stop() is idempotent-safe.** Plan said "executor's choice, document in code comment" — I chose panic-on-double-Start to surface wiring mistakes early (the constructor injection pattern in main.go should only call Start once per Store instance) and no-op on Stop-without-Start so deferred `defer s.Stop()` in tests never hangs. Both are documented in doc comments.
- **Normalization of empty Health enum to HealthStatusUnknown.** When a brand-new device's first Update is a failure, `next.Health` is left at its zero-value (empty string) because evaluateHealth is skipped. Emitting `""` as a valid HealthStatus on the Changes channel would leak an invalid enum to consumers. The normalization `if next.Health == "" { next.Health = HealthStatusUnknown }` ensures only valid enum values ever reach Snapshot or Changes.
- **time.Time.Equal instead of == in deviceStateEqual.** Go's `time.Time` can carry a monotonic clock component that is stripped by certain operations (including serialization through `time.Unix`). Using `==` would make `time.Unix(X,0) == time.Unix(X,0)` true but could fail for wall-equal but monotonic-differing values stored/re-stored through the struct. `time.Time.Equal` compares wall-clock only — the correct semantic for "same logical poll timestamp."

## Deviations from Plan

None — plan executed exactly as written.

All acceptance criteria pass verbatim:

- store.go: all 30 enumerated grep/content checks pass (stalenessTickInterval const, all method signatures, frozen-health guard `if next.Reachability == ReachabilityUp`, hard-down threshold `next.ConsecutiveFailures >= 3`, all three reachability setter lines, `case s.changes <- ids:` with `default:`, `log.Printf("state: changes channel full`, `ticker := time.NewTicker(stalenessTickInterval)`, `ds.LastPolledAt.Add(2 * ds.ExpectedInterval)`, `now.After(threshold)`, `context.WithCancel`)
- store.go: does NOT contain `reflect.DeepEqual` (only a comment reference at line 348 explaining why we avoid it — this is a documentation reference, not a code use; `go vet` and `go build` confirm no reflect import)
- store.go: does NOT contain `s.changes <- ids` outside a `select` block
- store_test.go: file exists with `package state` header; all 17 plan-required test functions present (TestStore_ConcurrentUpdateAndSnapshot, TestStore_SnapshotIsDeepCopy, 6 × TestReachability_*, 3 × TestChanges_*, 3 × TestStaleness_*, 2 × TestStore_Start/StopIsCleanShutdown/StopWithoutStartIsNoOp, TestStore_RemoveDevice)
- `cd /home/azmin/projects/theia && go build ./internal/state/...` — exit 0
- `cd /home/azmin/projects/theia && go vet ./internal/state/...` — exit 0
- `cd /home/azmin/projects/theia && go test -race ./internal/state/... -v -count=1` — exit 0, wall-clock ~1.17s
- Test output contains all required `--- PASS:` lines (TestStore_ConcurrentUpdateAndSnapshot, TestReachability_HealthFrozenOnSoftDown, TestReachability_HealthFrozenOnHardDown, TestChanges_UnchangedDeviceDoesNotEmit, TestStaleness_MarksStaleAfterThreshold, TestStore_StartStopIsCleanShutdown — all PASS)
- Test output does NOT contain `FAIL` or `DATA RACE` or `goroutine leaked`
- `go.mod` / `go.sum` — zero changes (D-14 satisfied)

## Issues Encountered

- **Go toolchain not on default PATH**: Same as Plan 01. Resolved by prefixing commands with `PATH=$PATH:/usr/local/go/bin` since Go 1.24.0 is installed at `/usr/local/go/bin/go`. No blocker.
- **TestStore_ConcurrentUpdateAndSnapshot emits "changes channel full" log lines**: Expected behavior, not a failure. The test runs 1600 Updates (4 writers × 200 × 2 cpu-value cycles through 5 device IDs) against a buffer of 32 with a concurrent drain goroutine, and legitimately exercises the drop path under race-detector timing. The test still passes because the drop path is safe by design — consumers are expected to recover via Snapshot() (this is Pitfall 5 in the research, explicitly addressed).

## User Setup Required

None — no external service configuration required. Pure-Go internal package.

## Must-Haves Verification

From plan frontmatter `must_haves.truths`:

1. ✓ "Store.Update() is safe to call from multiple goroutines concurrently with Snapshot() (no data races under -race)" — verified via TestStore_ConcurrentUpdateAndSnapshot (4 writers × 4 readers × 200 ops each, zero DATA RACE warnings under `go test -race`)
2. ✓ "A single failed poll transitions reachability to soft_down; three consecutive failures transition to hard_down; a single success resets to up" — verified via TestReachability_SinglePollFailureIsSoftDown (count=1→soft), TestReachability_TwoFailuresStayedSoftDown (count=2→soft), TestReachability_ThreeFailuresIsHardDown (count=3→hard), TestReachability_SuccessResetsToUp (3 fails then 1 success → up, count=0)
3. ✓ "While a device is soft_down or hard_down, its HealthStatus is frozen at the last known value (not recomputed)" — verified via TestReachability_HealthFrozenOnSoftDown (CPU=50 healthy → fail → Reachability=soft_down AND Health=healthy) and TestReachability_HealthFrozenOnHardDown (CPU=95 critical → 3 fails → Reachability=hard_down AND Health=critical)
4. ✓ "Store.Update() only emits a device ID on Changes() when the new state differs from the previous state" — verified via TestChanges_UnchangedDeviceDoesNotEmit (identical Update with fixed timestamp — second Update does NOT emit within 150ms window) and TestChanges_FirstUpdateAlwaysEmits + TestChanges_ChangedDeviceEmits (positive controls)
5. ✓ "Snapshot() returns an independent deep copy — mutating returned DeviceState does not affect Store internals" — verified via TestStore_SnapshotIsDeepCopy (`*ds1.Metrics.CPUPercent = 999` then second Snapshot still reports 42)
6. ✓ "Staleness background tick marks a device Stale when LastPolledAt + 2*ExpectedInterval < now" — verified via TestStaleness_MarksStaleAfterThreshold (LastPolledAt=1 minute ago, ExpectedInterval=10s → 2×10s=20s threshold is in the past → Stale=true + emitted on Changes)
7. ✓ "Store.Stop() cancels the staleness goroutine cleanly (no goroutine leak)" — verified via TestStore_StartStopIsCleanShutdown (2-second timeout guard on Stop return) and TestStore_StopWithoutStartIsNoOp (defensive — Stop before Start is a no-op, not a panic)

## Phase 38 Success Criteria (ROADMAP.md §Phase 38)

All 5 Phase 38 success criteria are now observable:

1. ✓ **Centralized thread-safe in-memory store** — Store.devices map under sync.RWMutex, verified by TestStore_ConcurrentUpdateAndSnapshot under -race
2. ✓ **Backend computes health from metrics and thresholds** — evaluateHealth (Plan 01) called from Update() (Plan 02) under lock
3. ✓ **Hardcoded hysteresis thresholds prevent flapping** — verified by Plan 01's TestHysteresis and TestHysteresis_FlapPrevention
4. ✓ **Soft/hard state transitions with 1/3/1 semantics** — verified by 6 TestReachability_* functions
5. ✓ **Diff-based change emission to Changes() channel** — verified by 3 TestChanges_* functions + verified in practice by TestStore_ConcurrentUpdateAndSnapshot's drain goroutine pattern

## Test Coverage

**Package test count (state/):**
- Plan 01: 8 top-level tests + 11 TestHysteresis sub-tests + 1 TestHysteresis_FlapPrevention (health_test.go)
- Plan 02: 17 top-level tests (store_test.go)
- **Total: 25 top-level test functions + 11 sub-tests = 36 distinct test cases**

**Commands run:**
- `PATH=$PATH:/usr/local/go/bin go test -race ./internal/state/... -v -count=1` → PASS (wall ~1.17s)
- `PATH=$PATH:/usr/local/go/bin go build ./internal/state/...` → OK
- `PATH=$PATH:/usr/local/go/bin go vet ./internal/state/...` → OK (zero warnings)

**Zero DATA RACE warnings. Zero FAIL lines. Zero goroutine leak reports.**

## Next Phase Readiness

**Phase 38 is complete.** Both plans in the phase have landed:
- Plan 01 (`3468d99`, `1170d46`) — type foundation + pure health logic
- Plan 02 (`965e63b`, `5688a63`) — concurrent Store API + race-safe tests

**Ready for downstream consumers:**
- Phase 39 (volatility classification) can import HealthStatus and DeviceState from `github.com/lollinoo/theia/internal/state`
- Phase 40 (jittered scheduler) can produce StateUpdate values and call store.Update()
- Phase 42 (pipeline cutover) can wire `state.NewStore()` into main.go and pass the Store to the new PipelineOrchestrator. The Orchestrator reads changed device IDs from `store.Changes()`, builds WS delta payloads from `store.Snapshot()`, and calls `wsHub.Broadcast()`. The existing MetricsCollector's FNV-64a delta mechanism is unchanged by Phase 38 — Phase 42 will replace it.
- Phase 43 (detail-on-demand) can call `store.GetDevice(id)` for single-device lookups.
- Phase 44 (freshness UI) consumes DeviceState.Stale + DeviceState.LastPolledAt through the WS delta payload built from Snapshot().

**API surface for Phase 42 cutover** (matches <output> requirement in plan):
```go
store := state.NewStore()
store.Start(ctx)
defer store.Stop()

// Producer (formerly MetricsCollector, future PipelineOrchestrator):
store.Update(state.StateUpdate{
    DeviceID:         deviceID,
    Metrics:          &metrics,
    PollSuccess:      true,
    ExpectedInterval: 15 * time.Second,
    Timestamp:        time.Now(),
})

// Consumer:
for changed := range store.Changes() {
    snap := store.Snapshot()
    for _, id := range changed {
        ds := snap[id]
        // build WS delta from ds, broadcast via hub
    }
}
```

**Blockers:** None.

## Self-Check: PASSED

Verified after write:
- `internal/state/store.go` — FOUND (397 lines, contains all 11 Plan-02 methods/helpers + all Plan-01 types)
- `internal/state/store_test.go` — FOUND (446 lines, 17 test functions)
- Commit `965e63b` — FOUND in `git log --oneline` (feat 38-02 Store methods)
- Commit `5688a63` — FOUND in `git log --oneline` (test 38-02 store_test.go)
- `PATH=$PATH:/usr/local/go/bin go build ./internal/state/...` — OK
- `PATH=$PATH:/usr/local/go/bin go vet ./internal/state/...` — OK
- `PATH=$PATH:/usr/local/go/bin go test -race ./internal/state/... -v -count=1` — OK (all 25 top-level + 11 sub-tests PASS, zero DATA RACE, zero FAIL, ~1.17s)
- `go.mod` / `go.sum` — zero changes (D-14)
- `git diff --diff-filter=D --name-only HEAD~2 HEAD` — empty (no unintended deletions from either Task 1 or Task 2 commit)
- `git worktree list` — confirmed running on main working tree, not in a prunable agent worktree

---
*Phase: 38-state-engine*
*Completed: 2026-04-12*
