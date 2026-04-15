---
phase: 41-jittered-scheduler
verified: 2026-04-12T19:30:03Z
status: passed
score: 18/18 must-haves verified
overrides_applied: 0
re_verification:
  previous_status: passed
  previous_score: 18/18
  gaps_closed: []
  gaps_remaining: []
  regressions: []
---

# Phase 41: Jittered Scheduler Verification Report

**Phase Goal:** Polls are distributed across the interval window with per-device timing offsets and concurrency limits, eliminating thundering herd bursts and separating discovery walks from metrics polling
**Verified:** 2026-04-12T19:30:03Z
**Status:** passed
**Re-verification:** Yes — current codebase rechecked; no regressions found

## Goal Achievement

### Observable Truths

| # | Truth | Status | Evidence |
| --- | --- | --- | --- |
| 1 | Discovery walks run on a slower shared static cadence (`300s` system-wide in v1.5.3) independent of performance polling | ✓ VERIFIED | [types.go](/home/azmin/projects/theia/internal/scheduler/types.go) resolves static work to `domain.StaticClassInterval`, [poll_class.go](/home/azmin/projects/theia/internal/domain/poll_class.go) defines that interval as `300s`, and [scheduler_test.go](/home/azmin/projects/theia/internal/scheduler/scheduler_test.go:16) verifies static seeding stays separate from performance. |
| 2 | Performance polling runs per device on poll-class cadence plus performance-only override | ✓ VERIFIED | [types.go](/home/azmin/projects/theia/internal/scheduler/types.go) uses `device.PollClass.Interval()` only for performance work and honors `PollIntervalOverride` only there; [types_test.go](/home/azmin/projects/theia/internal/scheduler/types_test.go:10) covers override and fallback behavior. |
| 3 | Each device's initial poll offset is deterministically computed from an FNV hash of its UUID and spread across the interval window | ✓ VERIFIED | [jitter.go](/home/azmin/projects/theia/internal/scheduler/jitter.go) uses `fnv.New64a()` over the device UUID and maps the result into the interval; [jitter_test.go](/home/azmin/projects/theia/internal/scheduler/jitter_test.go:36) proves 256 UUIDs distribute across 8 buckets without burst clustering. |
| 4 | A concurrency limiter caps simultaneous SNMP operations | ✓ VERIFIED | [scheduler.go](/home/azmin/projects/theia/internal/scheduler/scheduler.go) dispatches only while `s.inFlight < s.maxInFlight()`, and `maxInFlight()` reads `domain.SettingSNMPWorkerPoolSize` with fallback `5`; [scheduler_test.go](/home/azmin/projects/theia/internal/scheduler/scheduler_test.go:316) covers missing, invalid, non-positive, and configured values. |
| 5 | `internal/scheduler` defines `PollTask`, `TaskKey`, `Completion`, and interval helpers using existing `domain.PollClass` / `domain.VolatilityClass` types | ✓ VERIFIED | [types.go](/home/azmin/projects/theia/internal/scheduler/types.go) defines the scheduler contracts directly against the shared domain enums and durations. |
| 6 | Performance interval resolution honors `PollIntervalOverride` only for performance tasks; operational and static always use shared system intervals | ✓ VERIFIED | [types.go](/home/azmin/projects/theia/internal/scheduler/types.go) returns `OperationalClassInterval` and `StaticClassInterval` for non-performance volatility classes; [types_test.go](/home/azmin/projects/theia/internal/scheduler/types_test.go:30) and [types_test.go](/home/azmin/projects/theia/internal/scheduler/types_test.go:41) verify overrides are ignored there. |
| 7 | Deterministic initial offsets always fall inside `[0, interval)` | ✓ VERIFIED | [jitter.go](/home/azmin/projects/theia/internal/scheduler/jitter.go) returns `0` for non-positive intervals and `% interval` otherwise; [jitter_test.go](/home/azmin/projects/theia/internal/scheduler/jitter_test.go:11) verifies determinism and bounds. |
| 8 | Subsequent fire-time jitter stays within +/-10% of the base interval and never creates zero or negative windows | ✓ VERIFIED | [jitter.go](/home/azmin/projects/theia/internal/scheduler/jitter.go) computes a ±10% delta around the base interval; [jitter_test.go](/home/azmin/projects/theia/internal/scheduler/jitter_test.go:82) checks repeated samples stay bounded and that zero interval falls back safely. |
| 9 | Scheduler uses a single min-heap of due times instead of per-device polling goroutines | ✓ VERIFIED | [heap.go](/home/azmin/projects/theia/internal/scheduler/heap.go) defines one `taskHeap`, and [scheduler.go](/home/azmin/projects/theia/internal/scheduler/scheduler.go) owns one scheduler goroutine with one timer/ticker loop instead of per-device timers. |
| 10 | Inventory refresh is pull-based from a `GetDevices() ([]domain.Device, error)` source and seeds managed devices only | ✓ VERIFIED | [scheduler.go](/home/azmin/projects/theia/internal/scheduler/scheduler.go) pulls through `s.source.GetDevices()`, and [scheduler_test.go](/home/azmin/projects/theia/internal/scheduler/scheduler_test.go:68) verifies unmanaged devices are skipped. |
| 11 | Each managed device gets three independent task entries: static, operational, and performance, each with its own interval and first due time | ✓ VERIFIED | [scheduler.go](/home/azmin/projects/theia/internal/scheduler/scheduler.go) seeds all three volatility classes via `scheduledVolatilityClasses()`, and [scheduler_test.go](/home/azmin/projects/theia/internal/scheduler/scheduler_test.go:16) verifies exactly three keys with distinct intervals and deterministic due times. |
| 12 | Refresh updates device snapshots and interval metadata without duplicating keys, and removes missing or unmanaged keys from future scheduling | ✓ VERIFIED | [scheduler.go](/home/azmin/projects/theia/internal/scheduler/scheduler.go) updates existing items in place and removes or disables unseen keys; [scheduler_test.go](/home/azmin/projects/theia/internal/scheduler/scheduler_test.go:94) and [scheduler_test.go](/home/azmin/projects/theia/internal/scheduler/scheduler_test.go:165) cover stale-key removal and in-place metadata refresh. |
| 13 | Static cadence remains independent from performance cadence and uses `domain.StaticClassInterval` | ✓ VERIFIED | [types.go](/home/azmin/projects/theia/internal/scheduler/types.go) hardcodes static work to the shared static interval rather than poll-class cadence, and [scheduler_test.go](/home/azmin/projects/theia/internal/scheduler/scheduler_test.go:16) proves the independent seeded interval. |
| 14 | Scheduler exposes `Start(ctx)`, `Stop()`, `Tasks()`, and `Complete()` with the project's long-running worker lifecycle shape | ✓ VERIFIED | [scheduler.go](/home/azmin/projects/theia/internal/scheduler/scheduler.go) implements the lifecycle API, and manual comparison against [store.go](/home/azmin/projects/theia/internal/state/store.go:269) shows the same guarded `Start`, cancel + `<-done` shutdown, and `done` channel recreation pattern; [scheduler_test.go](/home/azmin/projects/theia/internal/scheduler/scheduler_test.go:358) covers restart safety. |
| 15 | Ready dispatch prefers `performance`, then `operational`, then `static` | ✓ VERIFIED | [heap.go](/home/azmin/projects/theia/internal/scheduler/heap.go) sorts by volatility priority when due times tie, [scheduler.go](/home/azmin/projects/theia/internal/scheduler/scheduler.go) drains `ready[0]`, then `ready[1]`, then `ready[2]`, and [scheduler_test.go](/home/azmin/projects/theia/internal/scheduler/scheduler_test.go:246) verifies emitted order. |
| 16 | Backlog is coalesced to at most one queued or pending rerun per `device + volatility` | ✓ VERIFIED | [scheduler.go](/home/azmin/projects/theia/internal/scheduler/scheduler.go) refuses duplicate ready enqueues and marks overlapped work `pending` instead of growing an unbounded queue; [scheduler_test.go](/home/azmin/projects/theia/internal/scheduler/scheduler_test.go:417) verifies in-flight due events collapse into one pending rerun. |
| 17 | Completion-based reinsertion uses the completion timestamp, and a coalesced overlap produces exactly one immediate rerun before normal cadence resumes | ✓ VERIFIED | [scheduler.go](/home/azmin/projects/theia/internal/scheduler/scheduler.go) reinserts from `FinishedAt`, queues one immediate rerun for pending or elapsed overlaps, and otherwise jitters the next due time; [scheduler_test.go](/home/azmin/projects/theia/internal/scheduler/scheduler_test.go:452), [scheduler_test.go](/home/azmin/projects/theia/internal/scheduler/scheduler_test.go:499), and [scheduler_test.go](/home/azmin/projects/theia/internal/scheduler/scheduler_test.go:573) cover those paths. |
| 18 | The runtime loop uses `time.NewTimer`, not `time.After`, so there is one active wake timer in runtime code | ✓ VERIFIED | [scheduler.go](/home/azmin/projects/theia/internal/scheduler/scheduler.go:116) creates a single `time.NewTimer`, and `rg -n "time.After\\(" internal/scheduler --glob "!**/*_test.go"` returned no runtime matches. |

**Score:** 18/18 truths verified

### Required Artifacts

| Artifact | Expected | Status | Details |
| --- | --- | --- | --- |
| [internal/scheduler/types.go](/home/azmin/projects/theia/internal/scheduler/types.go) | Task identity, completion contract, interval helpers, volatility ordering | ✓ VERIFIED | Exists, substantive, and wired through scheduler seeding and task dispatch. |
| [internal/scheduler/jitter.go](/home/azmin/projects/theia/internal/scheduler/jitter.go) | Deterministic initial-offset helper and bounded next-fire jitter helper | ✓ VERIFIED | Exists, substantive, and wired into initial seeding and completion reinsertion paths. |
| [internal/scheduler/jitter_test.go](/home/azmin/projects/theia/internal/scheduler/jitter_test.go) | Determinism, bounds, and burst-distribution tests for timing primitives | ✓ VERIFIED | Covers determinism, bounds, RNG behavior, and bucket spread. |
| [internal/scheduler/heap.go](/home/azmin/projects/theia/internal/scheduler/heap.go) | Heap item state and min-heap ordering by due time, volatility priority, and device UUID | ✓ VERIFIED | Exists, substantive, and used by refresh, due-task promotion, and reinsertion logic. |
| [internal/scheduler/scheduler.go](/home/azmin/projects/theia/internal/scheduler/scheduler.go) | Refresh reconciliation, runtime loop, dispatch, completion handling, and concurrency cap | ✓ VERIFIED | Core scheduler implementation is present, non-stub, and wired to real device-source, settings, and completion flows. |
| [internal/scheduler/scheduler_test.go](/home/azmin/projects/theia/internal/scheduler/scheduler_test.go) | Refresh, lifecycle, priority, concurrency, coalescing, reinsertion, and shutdown tests | ✓ VERIFIED | Substantive test coverage exists for the package's critical runtime behavior. |

### Key Link Verification

| From | To | Via | Status | Details |
| --- | --- | --- | --- | --- |
| [types.go](/home/azmin/projects/theia/internal/scheduler/types.go) | [poll_class.go](/home/azmin/projects/theia/internal/domain/poll_class.go) | Shared cadence rules | ✓ WIRED | `EffectiveInterval()` reuses `PollClass.Interval()`, `OperationalClassInterval`, and `StaticClassInterval` directly. |
| [jitter.go](/home/azmin/projects/theia/internal/scheduler/jitter.go) | `hash/fnv` | Deterministic per-device offset | ✓ WIRED | `initialOffset()` calls `fnv.New64a()` directly. |
| [scheduler.go](/home/azmin/projects/theia/internal/scheduler/scheduler.go) | [cache.go::GetDevices](/home/azmin/projects/theia/internal/cache/cache.go:35) | Pull-based inventory source | ✓ WIRED | `DeviceSource` matches the cache surface and `refreshDevices()` consumes `GetDevices()` results. |
| [scheduler.go](/home/azmin/projects/theia/internal/scheduler/scheduler.go) | [poll_class.go](/home/azmin/projects/theia/internal/domain/poll_class.go) | Shared static, operational, and performance intervals | ✓ WIRED | Seeding delegates interval resolution to `EffectiveInterval()`, which consumes the shared domain constants and poll-class helper. |
| [scheduler.go](/home/azmin/projects/theia/internal/scheduler/scheduler.go) | [settings.go::SettingSNMPWorkerPoolSize](/home/azmin/projects/theia/internal/domain/settings.go:8) | Shared SNMP concurrency-cap setting | ✓ WIRED | `maxInFlight()` reads the existing worker-pool key and falls back to the same default value. |
| [scheduler.go](/home/azmin/projects/theia/internal/scheduler/scheduler.go) | [store.go::Start/Stop](/home/azmin/projects/theia/internal/state/store.go:269) | Long-running worker lifecycle shape | ✓ WIRED | Manual verification passed: both components guard `Start`, cancel via `context.CancelFunc`, wait on `done`, and recreate `done` for restart safety. `gsd-tools` could not verify this link automatically because the plan regex is invalid. |

### Data-Flow Trace (Level 4)

| Artifact | Data Variable | Source | Produces Real Data | Status |
| --- | --- | --- | --- | --- |
| [scheduler.go](/home/azmin/projects/theia/internal/scheduler/scheduler.go) | `devices` -> `s.items` / `s.heap` | `s.source.GetDevices()` from the device cache/repository view | Yes | ✓ FLOWING |
| [scheduler.go](/home/azmin/projects/theia/internal/scheduler/scheduler.go) | `value` -> `maxInFlight()` | `settingsRepo.Get(domain.SettingSNMPWorkerPoolSize)` | Yes | ✓ FLOWING |
| [scheduler.go](/home/azmin/projects/theia/internal/scheduler/scheduler.go) | `Completion.FinishedAt` -> `dueAt` / `ready` / `heap` | `Complete()` channel input processed by `handleCompletion()` | Yes | ✓ FLOWING |

### Behavioral Spot-Checks

| Behavior | Command | Result | Status |
| --- | --- | --- | --- |
| Timing spread, seeding, dispatch ordering, overlap coalescing, and concurrency-cap parsing | `rtk bash -lc 'go test ./internal/scheduler -run "TestInitialOffset_DistributionAcrossBuckets|TestRefreshDevices_SeedsManagedDeviceAcrossAllThreeVolatilityClasses|TestSchedulerDispatchesPriorityOrder|TestSchedulerComplete_CoalescesElapsedIntervalsToSingleImmediateRerun|TestSchedulerMaxInFlight_DefaultAndConfigured" -count=1 -v'` | All targeted tests passed | ✓ PASS |
| Full scheduler package regression | `rtk go test ./internal/scheduler -count=1` | `Go test: 37 passed in 1 packages` | ✓ PASS |

### Requirements Coverage

| Requirement | Source Plan | Description | Status | Evidence |
| --- | --- | --- | --- | --- |
| `POLL-01` | `41-02`, `41-03` | Discovery walks run on a slower shared static cadence separate from metrics polling | ✓ SATISFIED | Static interval resolution is fixed at `domain.StaticClassInterval` in [types.go](/home/azmin/projects/theia/internal/scheduler/types.go) and is seeded independently in [scheduler.go](/home/azmin/projects/theia/internal/scheduler/scheduler.go), with coverage in [scheduler_test.go](/home/azmin/projects/theia/internal/scheduler/scheduler_test.go:16). |
| `POLL-02` | `41-01`, `41-02`, `41-03` | Performance polling runs per device on poll-class cadence plus performance-only override, independent of discovery | ✓ SATISFIED | Performance cadence is resolved from poll class or override only in [types.go](/home/azmin/projects/theia/internal/scheduler/types.go), and that behavior is covered in [types_test.go](/home/azmin/projects/theia/internal/scheduler/types_test.go:10). |
| `POLL-04` | `41-01`, `41-02`, `41-03` | Scheduler distributes polls across the interval window with deterministic FNV offset and concurrency limits | ✓ SATISFIED | Deterministic offsets are implemented in [jitter.go](/home/azmin/projects/theia/internal/scheduler/jitter.go) and distribution-tested in [jitter_test.go](/home/azmin/projects/theia/internal/scheduler/jitter_test.go:36); concurrency limiting is implemented in [scheduler.go](/home/azmin/projects/theia/internal/scheduler/scheduler.go) and covered in [scheduler_test.go](/home/azmin/projects/theia/internal/scheduler/scheduler_test.go:316). |

All requirement IDs declared across `41-01-PLAN.md`, `41-02-PLAN.md`, and `41-03-PLAN.md` are present in [REQUIREMENTS.md](/home/azmin/projects/theia/.planning/REQUIREMENTS.md). No orphaned Phase 41 requirements were found.

### Anti-Patterns Found

No blocker anti-patterns were found. `rg -n "TODO|FIXME|XXX|HACK|PLACEHOLDER|placeholder|coming soon|not yet implemented|not available|console\\.log" internal/scheduler` returned no matches.

| File | Line | Pattern | Severity | Impact |
| --- | --- | --- | --- | --- |
| [internal/scheduler/scheduler_test.go](/home/azmin/projects/theia/internal/scheduler/scheduler_test.go:246) | 246 | Priority-order test seeds ready queues directly instead of exercising heap promotion plus dispatch in one assertion | ⚠️ Warning | Dispatch ordering is covered, but the combined due-promotion path is only indirectly protected. |
| [internal/scheduler/scheduler_test.go](/home/azmin/projects/theia/internal/scheduler/scheduler_test.go:358) | 358 | No regression test covers the documented double-start panic path | ⚠️ Warning | The misuse guard exists in runtime code but is not explicitly test-locked. |
| [internal/scheduler/scheduler.go](/home/azmin/projects/theia/internal/scheduler/scheduler.go:63) | 63 | Refresh failures are logged, but there is no dedicated recovery-path test for repeated `GetDevices()` errors | ℹ️ Info | Error handling is present by inspection, but retry behavior is only indirectly exercised. |

### Gaps Summary

No actionable gaps found. The scheduler package satisfies the Phase 41 roadmap contract and the plan-declared must-haves: static discovery work is separated from performance polling, per-device FNV-based offsets spread first fire times, and runtime dispatch is bounded by a shared concurrency cap.

---

_Verified: 2026-04-12T19:30:03Z_  
_Verifier: Claude (gsd-verifier)_
