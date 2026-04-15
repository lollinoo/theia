# Phase 38: State Engine - Research

**Researched:** 2026-04-11
**Domain:** Go concurrent in-memory state management, health computation, diff-based change emission
**Confidence:** HIGH

## Summary

Phase 38 creates a new `internal/state/` package that serves as the centralized runtime state store for all live device data -- metrics, health, reachability, staleness, and failure tracking. This is a pure Go standard library implementation with no external dependencies, using `sync.RWMutex` for thread-safe concurrent access (locked decision D-11).

The state engine is the architectural centerpiece of v1.5.3. It must be designed as a standalone, fully testable package with a clean API surface that downstream phases (39, 40, 42, 44) consume. Phase 38 does NOT wire the state engine into the existing MetricsCollector/Poller pipeline -- that is Phase 42's responsibility. Phase 38 delivers the package, its tests, and its contract.

**Primary recommendation:** Build `internal/state/` as two files -- `store.go` (Store struct, DeviceState, types, Update/Snapshot/Changes API, staleness goroutine) and `health.go` (threshold evaluation with hysteresis, metric severity computation, health aggregation) -- plus corresponding `store_test.go` and `health_test.go`. All tests must pass with `-race`.

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions
- **D-01:** Two-dimensional model -- separate `HealthStatus` (healthy/warning/critical/unknown) and `ReachabilityStatus` (up/soft_down/hard_down) as independent enums
- **D-02:** When device goes soft-down or hard-down, HealthStatus frozen at last known value from when device was reachable
- **D-03:** Per-metric severity stored alongside overall health -- `MetricSeverity` (ok/warning/critical) tracked individually for CPU, memory, temperature. Overall HealthStatus = worst-of all individual metric severities
- **D-04:** Notify channel pattern -- state engine exposes `Changes() <-chan []uuid.UUID`. Consumers read changed device IDs, build WS delta payloads from state engine data, and broadcast
- **D-05:** Changes batched per update cycle -- each send on channel is `[]uuid.UUID` slice of all devices changed in that update
- **D-06:** State engine exposes `Snapshot() map[uuid.UUID]DeviceState` for atomic point-in-time reads under RLock
- **D-07:** State engine coexists with DeviceLinkCache -- separate concerns
- **D-08:** State engine lives in `internal/state/` package. Files: `store.go`, `health.go`, plus test files
- **D-09:** Active staleness tick -- background goroutine checks all devices against expected poll interval
- **D-10:** Staleness is independent third dimension. Device can be up + healthy + stale
- **D-11:** sync.RWMutex for store concurrency (not channel-based actor)
- **D-12:** Hysteresis thresholds: CPU warn 70%/clear 60%, critical 90%/clear 80%; same for memory, temperature
- **D-13:** Hardcoded sensible defaults -- configurable thresholds deferred
- **D-14:** No new third-party dependencies

### Claude's Discretion
- Staleness tick interval (hardcoded constant)
- Exact `StateUpdate` struct shape for `Update()` method input
- Internal diff computation approach
- Changes channel buffer size
- Whether `health.go` exports threshold constants or keeps them private

### Deferred Ideas (OUT OF SCOPE)
None -- discussion stayed within phase scope.
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| STATE-01 | Centralized in-memory device state store with per-device status, metrics, health, staleness | Store struct with `map[uuid.UUID]DeviceState` under RWMutex; DeviceState holds all dimensions (Architecture Patterns section) |
| STATE-02 | Backend computes health state (healthy/warning/critical/unknown) from metrics and thresholds | `health.go` with `evaluateHealth()` using worst-of per-metric severity (Code Examples section) |
| STATE-03 | Hardcoded thresholds with hysteresis to prevent flapping | ThresholdConfig with warn/clear/critical/clearCritical values; evaluation checks current severity to decide which threshold to compare against (Code Examples section) |
| STATE-04 | Soft/hard state transitions -- 1 fail = soft-down, 3 consecutive = hard-down, 1 success = recover | ReachabilityStatus enum with ConsecutiveFailures counter on DeviceState; transition logic in Update() (Architecture Patterns section) |
| STATE-05 | Diff-based change emission to existing FNV-64a delta WS broadcast layer | Changes() channel emitting `[]uuid.UUID` of changed devices per update cycle; consumers build WS payloads from Snapshot() (Architecture Patterns section) |
</phase_requirements>

## Standard Stack

### Core
| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| `sync` (stdlib) | Go 1.24 | RWMutex for concurrent store access | [VERIFIED: go.mod] Go 1.24; RWMutex is the standard concurrency primitive for read-heavy shared state |
| `time` (stdlib) | Go 1.24 | Staleness tick goroutine, timestamps, intervals | [VERIFIED: go.mod] Used throughout codebase for tickers and deadlines |
| `github.com/google/uuid` | v1.6.0 | Device UUID keys in state map | [VERIFIED: go.mod] Already a direct dependency; used for all entity IDs |

### Supporting
| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| `testing` (stdlib) | Go 1.24 | Unit and race-condition tests | All test files |
| `math` (stdlib) | Go 1.24 | NaN checks for metric values | Defensive nil/NaN handling in health evaluation |

### Alternatives Considered
| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| sync.RWMutex | Channel-based actor | Actor pattern more complex for atomic snapshot reads; RWMutex is natural fit (D-11 locked) |
| Manual diff | sync.Map | sync.Map has worse performance for known-key workloads and no atomic snapshot |
| Hardcoded thresholds | Config-driven thresholds | Deferred to THRESH-01/02 (D-13 locked) |

**Installation:**
```bash
# No new packages needed -- all stdlib + existing dependencies
```

## Architecture Patterns

### Recommended Project Structure
```
internal/state/
  store.go           # Store struct, DeviceState, StateUpdate, public API
  store_test.go      # Concurrent access tests, diff emission tests, staleness tests
  health.go          # Threshold definitions, hysteresis evaluation, severity computation
  health_test.go     # Threshold boundary tests, hysteresis state transition tests
```

[VERIFIED: codebase grep] This follows the project's established pattern where each `internal/` package has a single focused responsibility with co-located test files.

### Pattern 1: Three-Dimensional Device State Model
**What:** Each device has three independent state dimensions: HealthStatus (metric quality), ReachabilityStatus (poll success/failure), and Stale (poll freshness). [VERIFIED: CONTEXT.md D-01, D-10]
**When to use:** Every state evaluation and every consumer read.
**Type definitions:**

```go
// HealthStatus represents the overall metric health of a device.
type HealthStatus string

const (
    HealthStatusHealthy  HealthStatus = "healthy"
    HealthStatusWarning  HealthStatus = "warning"
    HealthStatusCritical HealthStatus = "critical"
    HealthStatusUnknown  HealthStatus = "unknown"
)

// ReachabilityStatus represents whether the device responds to polls.
type ReachabilityStatus string

const (
    ReachabilityUp       ReachabilityStatus = "up"
    ReachabilitySoftDown ReachabilityStatus = "soft_down"
    ReachabilityHardDown ReachabilityStatus = "hard_down"
)

// MetricSeverity represents the threshold evaluation for a single metric.
type MetricSeverity string

const (
    MetricSeverityOK       MetricSeverity = "ok"
    MetricSeverityWarning  MetricSeverity = "warning"
    MetricSeverityCritical MetricSeverity = "critical"
)
```

**Note on downstream dependency:** Phase 39 depends on `HealthStatus` being defined in the state package. The roadmap says Phase 39 "uses VolatilityClass and HealthStatus types defined in state engine." VolatilityClass is a Phase 39 concept (OID classification), but if Phase 39 expects it from Phase 38, the planner should define it here as a simple typed string that Phase 39 elaborates. However, re-reading the roadmap more carefully, VolatilityClass is about OID grouping (static/operational/performance) which is a domain concept -- it more naturally belongs in `internal/domain/` alongside other typed enums. The planner should define HealthStatus and related types in `internal/state/` and note that VolatilityClass (if needed by Phase 39) should be placed in `internal/domain/` during Phase 39 itself. [ASSUMED]

### Pattern 2: Store with RWMutex and Changes Channel
**What:** The Store struct holds a `map[uuid.UUID]DeviceState` protected by `sync.RWMutex`, with a buffered `chan []uuid.UUID` for change notification. [VERIFIED: CONTEXT.md D-04, D-05, D-06, D-11]
**When to use:** All reads (Snapshot, GetDevice) use RLock; all writes (Update) use Lock.

```go
// DeviceState holds all live runtime state for a single device.
type DeviceState struct {
    // Metrics
    Metrics         domain.DeviceMetrics
    
    // Health (computed from metrics)
    Health          HealthStatus
    CPUSeverity     MetricSeverity
    MemSeverity     MetricSeverity
    TempSeverity    MetricSeverity
    
    // Reachability (computed from poll success/failure)
    Reachability        ReachabilityStatus
    ConsecutiveFailures int
    
    // Staleness
    Stale       bool
    LastPolledAt time.Time
    
    // Expected poll interval for staleness detection
    ExpectedInterval time.Duration
}

// Store is the centralized in-memory state for all devices.
type Store struct {
    mu       sync.RWMutex
    devices  map[uuid.UUID]DeviceState
    changes  chan []uuid.UUID
    
    // Staleness tick
    cancel   context.CancelFunc
    done     chan struct{}
}
```

### Pattern 3: Hysteresis-Based Threshold Evaluation
**What:** To prevent flapping, each metric has two threshold pairs: warn/clearWarn and critical/clearCritical. The evaluation direction depends on the current severity -- rising thresholds are higher than falling thresholds. [VERIFIED: CONTEXT.md D-12]
**When to use:** Every time metrics arrive for a device.

```go
type ThresholdConfig struct {
    WarnRise      float64 // cross above = warning (70%)
    WarnFall      float64 // cross below = clear warning (60%)
    CriticalRise  float64 // cross above = critical (90%)
    CriticalFall  float64 // cross below = clear critical (80%)
}
```

The evaluation logic:
- If current severity is OK and value >= WarnRise -> Warning
- If current severity is Warning and value >= CriticalRise -> Critical
- If current severity is Warning and value < WarnFall -> OK
- If current severity is Critical and value < CriticalFall -> Warning
- If current severity is Critical and value < WarnFall -> OK (direct drop)

### Pattern 4: Soft/Hard Down State Machine
**What:** Reachability transitions follow a deterministic state machine based on consecutive failure count. [VERIFIED: CONTEXT.md D-04, Success Criteria 3]

```
Poll succeeds: -> ReachabilityUp, ConsecutiveFailures = 0
1 failure:     -> ReachabilitySoftDown, ConsecutiveFailures = 1
2 failures:    -> ReachabilitySoftDown, ConsecutiveFailures = 2
3 failures:    -> ReachabilityHardDown, ConsecutiveFailures = 3
Poll succeeds: -> ReachabilityUp, ConsecutiveFailures = 0 (immediate recovery)
```

When transitioning to soft-down or hard-down, health is frozen at last known value (D-02). When recovering, health is re-evaluated from the new metrics.

### Pattern 5: Diff Computation on Update
**What:** The `Update()` method compares new state against stored state and records which device UUIDs changed. Changed IDs are sent as a batch on the Changes channel. [VERIFIED: CONTEXT.md D-04, D-05]
**When to use:** Every `Update()` call.

The diff should compare all fields that affect downstream consumers: metrics values, health, reachability, staleness, and per-metric severities. A simple approach: serialize the previous and new DeviceState to comparable form and check equality. A more efficient approach: compare field by field.

Recommendation: Use field-by-field comparison since DeviceState has a bounded number of fields and this avoids allocation overhead of serialization. [ASSUMED]

### Pattern 6: Staleness Background Goroutine
**What:** A background goroutine runs on a fixed tick, iterates all devices, and marks any device whose `LastPolledAt + 2*ExpectedInterval < now` as stale. Changed devices are emitted on the Changes channel. [VERIFIED: CONTEXT.md D-09, D-10, Success Criteria 5]
**When to use:** Always running while the Store is active.

The goroutine needs a `context.Context` for cancellation and a `Stop()` method. This follows the same Start/Stop pattern used by Poller and MetricsCollector. [VERIFIED: internal/worker/poller.go, metrics_collector.go patterns]

### Anti-Patterns to Avoid
- **Locking during channel send:** Never hold the RWMutex while sending on the Changes channel -- compute the diff under lock, release, then send. A blocked channel consumer would deadlock the entire store otherwise.
- **Snapshot returning internal map reference:** Snapshot() must return a deep copy, not a reference to the internal map. Callers mutating the returned map would corrupt state. [VERIFIED: ws.CloneSnapshot pattern in messages.go]
- **Testing without -race:** All tests for this package must pass under `go test -race` since the store is accessed by multiple goroutines concurrently.
- **Evaluating health on unreachable device:** When a device is soft-down or hard-down, health must be frozen at the last known value, not re-evaluated with stale/nil metrics (D-02).

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Thread-safe map | Custom locking wrapper | `sync.RWMutex` + `map[uuid.UUID]DeviceState` | stdlib RWMutex is battle-tested; `sync.Map` is worse for this access pattern |
| UUID generation | String-based IDs | `github.com/google/uuid` | Already used project-wide; consistent with all entity IDs |
| Channel-based pub/sub | Custom event bus | Single `chan []uuid.UUID` | One consumer (MetricsCollector/PipelineOrchestrator); no fan-out needed |

**Key insight:** This phase requires zero external dependencies. The complexity is in the logic (hysteresis, state machine, diff computation), not in infrastructure. Standard library concurrency primitives are the correct choice.

## Common Pitfalls

### Pitfall 1: Deadlock from Lock-during-Send
**What goes wrong:** Holding the write lock while sending on the Changes channel blocks if the channel consumer is slow or if the channel buffer is full.
**Why it happens:** Natural impulse to send changes while still holding the lock to ensure consistency.
**How to avoid:** Compute the changed UUID slice under lock, release the lock, then send on the channel outside the lock. Use non-blocking send with `select/default` as a safety net (matches the `cacheInvalidate` channel pattern in main.go). [VERIFIED: main.go line 290 uses buffered(1) channel with non-blocking send pattern]
**Warning signs:** Tests hang when running with `-race` or under load.

### Pitfall 2: Hysteresis Direction Bug
**What goes wrong:** Threshold evaluation uses the same comparison direction regardless of current severity, causing oscillation at the boundary.
**Why it happens:** Forgetting that the "clear" threshold must be lower than the "set" threshold, and that the comparison direction depends on current state.
**How to avoid:** Test every state transition edge case explicitly: ok->warn, warn->critical, critical->warn, warn->ok, ok->critical (skip warn), critical->ok (skip warn). Test values at exact boundaries.
**Warning signs:** Metric at 69.9% repeatedly toggling between ok and warning.

### Pitfall 3: Nil Metric Pointer Dereference
**What goes wrong:** `domain.DeviceMetrics` uses `*float64` pointers for optional metrics. Dereferencing nil pointers in health evaluation causes panics.
**Why it happens:** Not all devices report all metrics (e.g., temperature may be nil for virtual devices).
**How to avoid:** Always check for nil before evaluating. If a metric pointer is nil, its severity should remain unchanged from the previous evaluation (or stay at MetricSeverityOK if first evaluation). [VERIFIED: domain.DeviceMetrics uses *float64 throughout]
**Warning signs:** Panic in health evaluation with "invalid memory address" for devices that don't report temperature.

### Pitfall 4: Staleness Race with Update
**What goes wrong:** Staleness goroutine marks a device stale at the same moment an Update() arrives with fresh data. The stale flag gets set after the update, incorrectly marking a freshly-polled device as stale.
**Why it happens:** Two goroutines writing to the same device state.
**How to avoid:** Both Update() and the staleness tick acquire the write lock. The staleness tick should check `LastPolledAt` under the lock and only set stale if `LastPolledAt` is still older than the threshold. If Update() ran between the tick's read and write, the condition will be false.
**Warning signs:** Devices briefly showing as stale immediately after receiving fresh metrics.

### Pitfall 5: Changes Channel Buffer Overflow
**What goes wrong:** If the consumer of Changes() is slow (or temporarily stopped), the channel fills up and Update() either blocks (buffered channel) or drops changes (select/default).
**Why it happens:** Mismatch between production rate and consumption rate.
**How to avoid:** Use a reasonably sized buffer (16-32 is sufficient given polling intervals of 15-60s). Use non-blocking send with a fallback strategy: if the channel is full, the consumer is behind and will pick up the full state on next read anyway. Log when drops occur. [ASSUMED - buffer size is Claude's discretion per CONTEXT.md]
**Warning signs:** "changes channel full, dropping notification" log messages appearing frequently.

### Pitfall 6: Snapshot Deep Copy Correctness
**What goes wrong:** Shallow copy of DeviceState map returns structs that share pointer fields (`*float64` in DeviceMetrics). Consumer mutates metric pointer values, corrupting store state.
**Why it happens:** Go struct assignment copies value fields but shares pointer targets.
**How to avoid:** The Snapshot() method must either (a) copy `*float64` values into new pointers, or (b) document that the returned DeviceState is read-only. Option (a) is safer. However, since DeviceMetrics uses `*float64` (not mutable reference types), and float64 values are immutable, a struct copy is actually safe -- the pointer target (`float64`) cannot be mutated through a `*float64` in Go (you'd have to explicitly assign through the pointer). The real risk is if consumers nil-out pointers for their own purposes and a subsequent Snapshot() call returns the nil'd version. Since Snapshot returns copies of the DeviceState struct (value semantics for the map values), each caller gets independent `*float64` pointers that point to the same `float64` values. Mutating `*state.CPUPercent = 99.0` would affect the original. So: copy `*float64` fields in Snapshot(). [VERIFIED: ws.CloneSnapshot copies all fields individually -- same pattern needed here]
**Warning signs:** Intermittent test failures where metric values differ from expected.

## Code Examples

Verified patterns from the existing codebase:

### Store Constructor (follows project New* pattern)
```go
// Source: follows pattern from internal/cache/cache.go NewDeviceLinkCache
// and internal/ws/hub.go NewHub

func NewStore() *Store {
    return &Store{
        devices: make(map[uuid.UUID]DeviceState),
        changes: make(chan []uuid.UUID, 32),
        done:    make(chan struct{}),
    }
}
```
[VERIFIED: project uses `New*` constructor pattern throughout; buffer size of 32 matches ws.Hub broadcast channel]

### StateUpdate Input Struct
```go
// StateUpdate carries the result of a single poll cycle for one device.
// Passed to Store.Update() by the consumer (MetricsCollector now, PipelineOrchestrator later).
type StateUpdate struct {
    DeviceID         uuid.UUID
    Metrics          *domain.DeviceMetrics // nil if poll failed
    PollSuccess      bool                  // false = SNMP timeout/error
    ExpectedInterval time.Duration         // for staleness: 2x this = stale threshold
    Timestamp        time.Time             // when this poll completed
}
```
This struct shape is Claude's discretion per CONTEXT.md. The key insight is that `PollSuccess` being separate from `Metrics` allows distinguishing "poll succeeded but returned no metrics" from "poll failed." [ASSUMED]

### Hysteresis Evaluation
```go
// Source: logic derived from CONTEXT.md D-12 threshold values

var defaultThresholds = map[string]ThresholdConfig{
    "cpu":  {WarnRise: 70, WarnFall: 60, CriticalRise: 90, CriticalFall: 80},
    "mem":  {WarnRise: 70, WarnFall: 60, CriticalRise: 90, CriticalFall: 80},
    "temp": {WarnRise: 70, WarnFall: 60, CriticalRise: 90, CriticalFall: 80},
}

func evaluateMetricSeverity(value float64, current MetricSeverity, threshold ThresholdConfig) MetricSeverity {
    switch current {
    case MetricSeverityOK:
        if value >= threshold.CriticalRise {
            return MetricSeverityCritical
        }
        if value >= threshold.WarnRise {
            return MetricSeverityWarning
        }
        return MetricSeverityOK
    case MetricSeverityWarning:
        if value >= threshold.CriticalRise {
            return MetricSeverityCritical
        }
        if value < threshold.WarnFall {
            return MetricSeverityOK
        }
        return MetricSeverityWarning
    case MetricSeverityCritical:
        if value < threshold.WarnFall {
            return MetricSeverityOK
        }
        if value < threshold.CriticalFall {
            return MetricSeverityWarning
        }
        return MetricSeverityCritical
    default:
        // Unknown/first evaluation -- treat as fresh evaluation from OK
        if value >= threshold.CriticalRise {
            return MetricSeverityCritical
        }
        if value >= threshold.WarnRise {
            return MetricSeverityWarning
        }
        return MetricSeverityOK
    }
}
```
[VERIFIED: threshold values from CONTEXT.md D-12 and REQUIREMENTS.md STATE-03]

### Non-Blocking Change Emission (project pattern)
```go
// Source: follows non-blocking send pattern from cmd/theia/main.go topologyNotify

func (s *Store) emitChanges(ids []uuid.UUID) {
    if len(ids) == 0 {
        return
    }
    select {
    case s.changes <- ids:
    default:
        // Consumer is behind; changes will be picked up on next read.
        // This is safe because consumers rebuild from Snapshot() anyway.
        log.Printf("state: changes channel full, %d device changes dropped", len(ids))
    }
}
```
[VERIFIED: main.go line 310 uses identical select/default pattern for topologyNotify channel]

### Changes Channel Consumer Pattern
```go
// Source: follows pattern from internal/cache/cache.go drainInvalidations
// Consumers read from Changes() to know which devices changed, then call
// Snapshot() or GetDeviceState() to get the current values.

func (s *Store) Changes() <-chan []uuid.UUID {
    return s.changes
}
```
[VERIFIED: DeviceLinkCache exposes invalidateCh as read-only channel; same pattern]

### Test Pattern: Race-Safe Concurrent Access
```go
// Source: follows test patterns from internal/cache/cache_test.go
// and internal/worker/metrics_collector_test.go

func TestStore_ConcurrentUpdateAndSnapshot(t *testing.T) {
    store := NewStore()
    store.Start(context.Background())
    defer store.Stop()

    id := uuid.New()
    var wg sync.WaitGroup

    // Writer goroutine
    wg.Add(1)
    go func() {
        defer wg.Done()
        for i := 0; i < 100; i++ {
            cpu := float64(i)
            store.Update(StateUpdate{
                DeviceID:    id,
                Metrics:     &domain.DeviceMetrics{CPUPercent: &cpu},
                PollSuccess: true,
                Timestamp:   time.Now(),
            })
        }
    }()

    // Reader goroutine
    wg.Add(1)
    go func() {
        defer wg.Done()
        for i := 0; i < 100; i++ {
            _ = store.Snapshot()
        }
    }()

    wg.Wait()
}
```
[VERIFIED: project uses standard `testing` package with no external framework; race tests follow stdlib patterns]

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| Frontend computes health from raw metrics | Backend computes health, frontend displays enum | v1.5.3 (this phase) | Single source of truth; consistent health across all clients |
| MetricsCollector holds lastSnapshot | State engine holds authoritative device state | v1.5.3 (this phase) | Decoupled state from collection pipeline |
| Binary up/down device status | Three-dimensional model (health + reachability + staleness) | v1.5.3 (this phase) | Much richer device status information |
| Immediate status transitions | Soft/hard down with hysteresis | v1.5.3 (this phase) | Prevents flapping, reduces false alerts |

**Deprecated/outdated:**
- `domain.DeviceStatus` (up/down/probing/unknown): Will eventually be superseded by ReachabilityStatus. Phase 38 defines the new types but does not replace the old enum -- that happens during Phase 42 cutover. [VERIFIED: CONTEXT.md code_context section mentions "richer ReachabilityStatus that eventually supersedes this"]

## Assumptions Log

| # | Claim | Section | Risk if Wrong |
|---|-------|---------|---------------|
| A1 | VolatilityClass should be defined in `internal/domain/` during Phase 39, not in `internal/state/` during Phase 38 | Architecture Patterns, Pattern 1 note | LOW -- if Phase 39 expects it from state package, planner can add it trivially |
| A2 | Field-by-field comparison is better than serialization for diff computation | Architecture Patterns, Pattern 5 | LOW -- either approach works; field-by-field avoids allocation |
| A3 | Changes channel buffer size of 32 is appropriate | Code Examples, constructor | LOW -- tunable constant; 32 handles burst scenarios well |
| A4 | StateUpdate struct shape with separate PollSuccess bool and optional Metrics pointer | Code Examples, StateUpdate | LOW -- any equivalent shape works; the key contract is distinguishing success from failure |
| A5 | Temperature thresholds use the same 70/60/90/80 pattern as CPU and memory | Code Examples, hysteresis | MEDIUM -- temperature in Celsius operates on different ranges than percentages. 70C warn / 90C critical may be reasonable for network equipment but this is what the user confirmed in CONTEXT.md D-12 |

## Open Questions (RESOLVED)

1. **Temperature threshold units** — **RESOLVED**
   - What we know: CONTEXT.md D-12 says "same pattern for memory, temperature" with values 70/60/90/80
   - What's unclear: Whether 70C/90C is appropriate for all device types (routers typically warn at 50-60C)
   - **RESOLVED:** Implement as specified — D-12 is locked user decision. Plan 01 Task 1 hardcodes `TempWarnRise=70, TempWarnFall=60, TempCriticalRise=90, TempCriticalFall=80` verbatim. If operational experience shows 70C is too permissive for specific device classes, adjustment moves to the future THRESH-01/02 phase when thresholds become configurable (already on the backlog). Phase 38 does not block on this.

2. **ExpectedInterval source for staleness** — **RESOLVED**
   - What we know: Staleness is computed as `LastPolledAt + 2*ExpectedInterval < now` (Success Criteria 5)
   - What's unclear: Where ExpectedInterval comes from before Phase 39 (which adds per-device poll classification)
   - **RESOLVED:** `StateUpdate` carries `ExpectedInterval time.Duration` as an explicit field supplied by the caller. Before Phase 39, consumers pass the global `polling_interval_seconds` setting uniformly. After Phase 39, PipelineOrchestrator passes the per-device classified interval. This keeps the state engine decoupled from poll-class knowledge and makes Phase 39 a zero-change upgrade for the state package.

## Validation Architecture

### Test Framework
| Property | Value |
|----------|-------|
| Framework | Go standard `testing` package (Go 1.24) |
| Config file | None needed -- Go test conventions |
| Quick run command | `go test -race ./internal/state/...` |
| Full suite command | `go test -race ./internal/state/ -v -count=1` |

### Phase Requirements to Test Map
| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| STATE-01 | Thread-safe in-memory store with all per-device fields | unit + race | `go test -race ./internal/state/ -run TestStore` | Wave 0 |
| STATE-02 | Health computation from per-metric severity (worst-of) | unit | `go test ./internal/state/ -run TestHealth` | Wave 0 |
| STATE-03 | Hysteresis thresholds prevent flapping | unit | `go test ./internal/state/ -run TestHysteresis` | Wave 0 |
| STATE-04 | Soft/hard down transitions with failure counting | unit | `go test ./internal/state/ -run TestReachability` | Wave 0 |
| STATE-05 | Diff-based change emission (only changed devices emitted) | unit | `go test ./internal/state/ -run TestChanges` | Wave 0 |

### Sampling Rate
- **Per task commit:** `go test -race ./internal/state/...`
- **Per wave merge:** `go test -race ./internal/state/ -v -count=1`
- **Phase gate:** Full suite green with `-race` before `/gsd-verify-work`

### Wave 0 Gaps
- [ ] `internal/state/store_test.go` -- covers STATE-01, STATE-04, STATE-05
- [ ] `internal/state/health_test.go` -- covers STATE-02, STATE-03
- [ ] `internal/state/store.go` -- new file (Store, DeviceState, StateUpdate types)
- [ ] `internal/state/health.go` -- new file (threshold evaluation, hysteresis logic)

*(All files are new -- this is a greenfield package)*

## Security Domain

### Applicable ASVS Categories

| ASVS Category | Applies | Standard Control |
|---------------|---------|-----------------|
| V2 Authentication | No | N/A -- internal package, no user-facing auth |
| V3 Session Management | No | N/A -- no sessions |
| V4 Access Control | No | N/A -- internal package accessed by trusted goroutines |
| V5 Input Validation | Yes (minimal) | Validate DeviceID is not uuid.Nil; validate metric pointers before dereference |
| V6 Cryptography | No | N/A -- no encryption in state engine |

### Known Threat Patterns for Go In-Memory State

| Pattern | STRIDE | Standard Mitigation |
|---------|--------|---------------------|
| Data race on shared map | Tampering | sync.RWMutex with `-race` CI enforcement |
| Denial of service via unbounded map growth | Denial of Service | Devices only added via Update(); removed via explicit Remove() method; bounded by actual device count |
| Information disclosure via snapshot | Information Disclosure | Not applicable -- all state is meant to be broadcast to authenticated WS clients |

## Sources

### Primary (HIGH confidence)
- `internal/cache/cache.go` -- DeviceLinkCache implementation pattern (RWMutex usage, invalidation channel, constructor injection)
- `internal/ws/hub.go` -- Hub broadcast pattern, channel buffering, concurrent client management
- `internal/ws/messages.go` -- SnapshotPayload structure, CloneSnapshot deep copy pattern, DeviceMetricsDTO shape
- `internal/ws/handler.go` -- Snapshot delivery pattern on new WS client connect
- `internal/worker/metrics_collector.go` -- FNV-64a delta mechanism (sectionHashes, buildDelta, computeSnapshotHashes), collectAndBroadcast cycle
- `internal/domain/device.go` -- DeviceStatus enum, DeviceMetrics pointer fields pattern
- `internal/domain/metrics.go` -- DeviceMetrics struct with `*float64` optional fields
- `cmd/theia/main.go` -- Dependency wiring pattern, topologyNotify non-blocking channel, Start/Stop lifecycle
- `.planning/phases/38-state-engine/38-CONTEXT.md` -- All locked decisions D-01 through D-14
- `.planning/REQUIREMENTS.md` -- STATE-01 through STATE-05 acceptance criteria
- `.planning/ROADMAP.md` -- Success criteria with specific values, phase dependencies
- `.planning/codebase/TESTING.md` -- Go test patterns, mock conventions, race testing

### Secondary (MEDIUM confidence)
- `.planning/STATE.md` -- Project-level decisions on RWMutex, hysteresis values

### Tertiary (LOW confidence)
- None -- all research was conducted against the codebase and planning documents

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH -- pure stdlib, verified against go.mod and existing codebase patterns
- Architecture: HIGH -- all patterns verified against existing codebase implementations and locked decisions
- Pitfalls: HIGH -- derived from concrete code review of concurrent patterns in cache.go, hub.go, metrics_collector.go

**Research date:** 2026-04-11
**Valid until:** Indefinite (stdlib-only implementation with locked decisions; no external dependency drift risk)
