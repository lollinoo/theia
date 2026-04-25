// Package state provides a thread-safe in-memory store for live device
// runtime state: metrics, health, reachability, and staleness. It is the
// architectural centerpiece of the SNMP pipeline and is consumed by the
// metrics collector (and later, by the pipeline orchestrator in Phase 42).
//
// D-07: this package coexists with internal/cache/DeviceLinkCache — they have
// separate concerns and must not be merged in this phase. The state engine
// holds VOLATILE RUNTIME state (metrics, health, reachability, staleness,
// consecutive failure counts), while DeviceLinkCache holds DB-BACKED CONFIG
// data (hostnames, IPs, interfaces, credentials). Whether the cache is later
// absorbed into the state engine is a Phase 42 decision; for Phase 38 the
// two remain architecturally independent.
package state

import (
	"context"
	"errors"
	"log"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/lollinoo/theia/internal/domain"
	"github.com/lollinoo/theia/internal/observability"
	"github.com/lollinoo/theia/internal/polling"
)

// stalenessTickInterval is how often the background goroutine checks all
// devices for expired poll intervals. 5 seconds is responsive enough for
// user-facing freshness indicators without dominating CPU (Claude's
// discretion per CONTEXT.md).
const stalenessTickInterval = 5 * time.Second

var ErrAlreadyStarted = errors.New("state store: already started")

// HealthStatus is the overall metric health of a device, computed by the
// state engine from per-metric severities using worst-of semantics (D-03).
type HealthStatus string

const (
	HealthStatusHealthy  HealthStatus = "healthy"
	HealthStatusWarning  HealthStatus = "warning"
	HealthStatusCritical HealthStatus = "critical"
	HealthStatusUnknown  HealthStatus = "unknown"
)

// ReachabilityStatus represents whether the device responds to polls.
// Independent of HealthStatus per D-01. Frozen while unreachable per D-02.
type ReachabilityStatus string

const (
	ReachabilityUnknown  ReachabilityStatus = "unknown"
	ReachabilityUp       ReachabilityStatus = "up"
	ReachabilitySoftDown ReachabilityStatus = "soft_down"
	ReachabilityHardDown ReachabilityStatus = "hard_down"
)

// MetricSeverity is the threshold evaluation result for a single metric.
// Tracked per-metric (CPU, memory, temperature) per D-03.
type MetricSeverity string

const (
	MetricSeverityOK       MetricSeverity = "ok"
	MetricSeverityWarning  MetricSeverity = "warning"
	MetricSeverityCritical MetricSeverity = "critical"
)

// DeviceState holds all live runtime state for a single device. Three
// independent dimensions per D-01, D-10: Health (metric quality),
// Reachability (poll success/failure), and Stale (poll freshness).
type DeviceState struct {
	// Metrics snapshot (pointer fields may be nil; see domain.DeviceMetrics).
	Metrics domain.DeviceMetrics
	// LinkMetrics stores the last-known per-link throughput snapshot for the
	// overview WebSocket payload.
	LinkMetrics []domain.LinkMetrics

	// Health dimension (computed from metrics, frozen when unreachable per D-02).
	Health       HealthStatus
	CPUSeverity  MetricSeverity
	MemSeverity  MetricSeverity
	TempSeverity MetricSeverity

	// Reachability dimension (computed from poll success/failure).
	Reachability        ReachabilityStatus
	ConsecutiveFailures int
	PrimaryHealth       polling.PrimaryHealth
	NetworkReachable    polling.TriState
	SNMPReachable       polling.TriState
	FieldStates         map[string]polling.FieldState
	RuntimeFlags        map[polling.RuntimeFlag]bool

	// Staleness dimension (computed by background tick per D-09).
	Stale            bool
	LastPolledAt     time.Time
	ExpectedInterval time.Duration
}

// StateUpdate is the input to Store.Update(). One update represents the
// result of a single poll cycle for one device. Shape is Claude's discretion
// per CONTEXT.md (A4).
type StateUpdate struct {
	DeviceID uuid.UUID
	// VolatilityClass domain.VolatilityClass identifies the tier that produced
	// this update.
	VolatilityClass  domain.VolatilityClass
	Metrics          *domain.DeviceMetrics // nil allowed if PollSuccess=false
	LinkMetrics      []domain.LinkMetrics
	PollSuccess      bool          // false => SNMP timeout/error
	ExpectedInterval time.Duration // 2x this value is the stale threshold
	Timestamp        time.Time     // when the poll completed
	Essential        *EssentialUpdate
}

type EssentialUpdate struct {
	PollStatus       polling.PollStatus
	NetworkReachable polling.TriState
	SNMPReachable    polling.TriState
	Uptime           polling.FieldState
	CPU              polling.FieldState
	Memory           polling.FieldState
	DeadlineMissed   bool
	Overloaded       bool
}

// Store is the centralized in-memory state for all devices. Concurrency via
// sync.RWMutex per D-11. Consumers read changed device IDs from Changes()
// and rebuild WS delta payloads via Snapshot() per D-04, D-05, D-06.
type Store struct {
	mu         sync.RWMutex
	devices    map[uuid.UUID]DeviceState
	changes    chan []uuid.UUID
	overflowed bool

	// Staleness goroutine lifecycle managed by Start/Stop.
	// lifecycleMu guards concurrent Start/Stop calls. Do not hold
	// s.mu simultaneously — the two mutexes are independent.
	lifecycleMu sync.Mutex
	cancel      context.CancelFunc
	done        chan struct{}
}

// NewStore constructs an empty Store with an initialized changes channel.
// Buffer size of 32 matches the ws.Hub broadcast channel (A3). The
// background staleness goroutine is not started here; call Start(ctx) to
// begin staleness ticking.
func NewStore() *Store {
	return &Store{
		devices: make(map[uuid.UUID]DeviceState),
		changes: make(chan []uuid.UUID, 32),
		done:    make(chan struct{}),
	}
}

// Update applies a StateUpdate to the store, re-evaluating health and
// reachability, computing a diff against the previous state, and emitting
// the device ID on the Changes() channel if (and only if) the state
// differs from the previous state.
//
// Health is re-evaluated only when the new reachability is Up. If the
// device is transitioning to soft_down or hard_down, the last-known
// HealthStatus and per-metric severities are preserved per D-02.
func (s *Store) Update(u StateUpdate) {
	if u.DeviceID == uuid.Nil {
		return
	}

	s.mu.Lock()
	prev, existed := s.devices[u.DeviceID]
	next := prev

	if u.Essential != nil {
		applyEssentialUpdate(&next, prev, u)
	} else {
		switch u.VolatilityClass {
		case domain.VolatilityClassPerformance:
			applyFreshnessMetadata(&next, u)
			applyPerformanceUpdate(&next, u)
		case domain.VolatilityClassOperational:
			applyOperationalUpdate(&next, prev, u)
		case domain.VolatilityClassStatic:
			applyStaticUpdate(&next, u)
		default:
			// Preserve the Phase 38 contract for pre-cutover callers that have not
			// started stamping volatility yet. Phase 42 collectors set an explicit
			// volatility class and bypass this path.
			applyFreshnessMetadata(&next, u)
			applyLegacyUpdate(&next, prev, existed, u)
		}
	}

	// If this is the first observation and the update did not establish
	// health yet, normalize to Unknown instead of leaking the zero value.
	if next.Health == "" {
		next.Health = HealthStatusUnknown
	}

	// Always write next into the map so that fields which advance on every
	// poll (LastPolledAt and, when metrics are present, Metrics.CollectedAt)
	// remain fresh in Snapshot() even when the diff equality otherwise
	// reports no semantic change. The `changed` flag still gates the
	// Changes channel emission so subscribers do not see spurious no-op
	// notifications (WR-05).
	changed := !existed || !deviceStateEqual(prev, next)
	s.devices[u.DeviceID] = next
	s.mu.Unlock()

	if changed {
		s.emitChanges([]uuid.UUID{u.DeviceID})
	}
}

// Snapshot returns a deep copy of the current device state map. Callers
// may freely mutate the returned map and its values without affecting the
// store. Pointer fields inside DeviceMetrics are re-allocated (D-06,
// Pitfall 6 in research).
func (s *Store) Snapshot() map[uuid.UUID]DeviceState {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make(map[uuid.UUID]DeviceState, len(s.devices))
	for id, ds := range s.devices {
		cp := ds
		cp.Metrics = cloneMetrics(ds.Metrics)
		cp.LinkMetrics = cloneLinkMetrics(ds.LinkMetrics)
		cp.FieldStates = cloneFieldStates(ds.FieldStates)
		cp.RuntimeFlags = cloneRuntimeFlags(ds.RuntimeFlags)
		out[id] = cp
	}
	return out
}

// GetDevice returns a deep copy of a single device's state.
func (s *Store) GetDevice(id uuid.UUID) (DeviceState, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	ds, ok := s.devices[id]
	if !ok {
		return DeviceState{}, false
	}
	ds.Metrics = cloneMetrics(ds.Metrics)
	ds.LinkMetrics = cloneLinkMetrics(ds.LinkMetrics)
	ds.FieldStates = cloneFieldStates(ds.FieldStates)
	ds.RuntimeFlags = cloneRuntimeFlags(ds.RuntimeFlags)
	return ds, true
}

// Remove deletes a device from the store and emits its ID on the Changes
// channel. Safe to call for non-existent devices.
func (s *Store) Remove(id uuid.UUID) {
	s.mu.Lock()
	_, existed := s.devices[id]
	if existed {
		delete(s.devices, id)
	}
	s.mu.Unlock()

	if existed {
		s.emitChanges([]uuid.UUID{id})
	}
}

// Changes returns a receive-only channel that emits batches of device
// UUIDs whose state has changed. Each send is a slice of all device IDs
// changed in a single Update cycle (D-04, D-05).
func (s *Store) Changes() <-chan []uuid.UUID {
	return s.changes
}

// ConsumeOverflowed reports whether change batches were dropped since the
// last call and clears the sticky overflow marker.
func (s *Store) ConsumeOverflowed() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	overflowed := s.overflowed
	s.overflowed = false
	return overflowed
}

// Start launches the background staleness tick goroutine. The goroutine
// runs until Stop() is called or the provided parent context is cancelled.
// Calling Start more than once on the same running Store is not supported —
// the second call will panic to surface the misuse early. Start/Stop are
// safe to call concurrently from multiple goroutines; transitions are
// serialized by s.lifecycleMu.
func (s *Store) Start(ctx context.Context) error {
	s.lifecycleMu.Lock()
	defer s.lifecycleMu.Unlock()
	if s.cancel != nil {
		return ErrAlreadyStarted
	}
	derived, cancel := context.WithCancel(ctx)
	s.cancel = cancel
	go s.runStaleness(derived)
	return nil
}

// Stop cancels the staleness goroutine and waits for it to exit. Safe to
// call if Start was never invoked (no-op in that case) and safe to call
// concurrently with Start (transitions are serialized by s.lifecycleMu).
// After Stop returns, the Store is reusable: a subsequent Start() will
// launch a new staleness goroutine. The done channel is re-created so
// the new goroutine's `defer close(s.done)` does not panic on an
// already-closed channel.
//
// Stop holds lifecycleMu for the entire shutdown sequence, so a racing
// Start() will block until Stop finishes re-creating s.done. This avoids
// any window in which a caller could observe s.cancel == nil but still
// see a closed s.done.
func (s *Store) Stop() {
	s.lifecycleMu.Lock()
	defer s.lifecycleMu.Unlock()
	if s.cancel == nil {
		return
	}
	s.cancel()
	<-s.done
	s.cancel = nil
	// Re-create done so the store can be restarted with Start().
	s.done = make(chan struct{})
}

func (s *Store) runStaleness(ctx context.Context) {
	defer close(s.done)
	ticker := time.NewTicker(stalenessTickInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			s.markStale(now)
		}
	}
}

// markStale iterates all devices and marks any whose LastPolledAt +
// 2*ExpectedInterval is before `now`. The set of newly-stale device IDs
// is emitted on the Changes channel after the lock is released (Pitfall 4
// avoidance: check and mutate under the same lock; emit outside).
func (s *Store) markStale(now time.Time) {
	var newlyStale []uuid.UUID
	s.mu.Lock()
	for id, ds := range s.devices {
		if ds.Stale {
			continue
		}
		if ds.ExpectedInterval <= 0 {
			continue
		}
		threshold := ds.LastPolledAt.Add(2 * ds.ExpectedInterval)
		if now.After(threshold) {
			ds.Stale = true
			if ds.PrimaryHealth == polling.PrimaryHealthUpFresh {
				ds.PrimaryHealth = polling.PrimaryHealthUpStale
			}
			if ds.RuntimeFlags == nil {
				ds.RuntimeFlags = map[polling.RuntimeFlag]bool{}
			}
			s.devices[id] = ds
			newlyStale = append(newlyStale, id)
		}
	}
	s.mu.Unlock()
	s.emitChanges(newlyStale)
}

// emitChanges sends a batch of changed device UUIDs on the Changes channel.
// Non-blocking: if the consumer is behind, the batch is dropped and a
// warning is logged. Consumers must rebuild from Snapshot() on next read.
// Matches the non-blocking pattern from cmd/theia/main.go topologyNotify.
func (s *Store) emitChanges(ids []uuid.UUID) {
	if len(ids) == 0 {
		return
	}
	select {
	case s.changes <- ids:
	default:
		s.mu.Lock()
		s.overflowed = true
		s.mu.Unlock()
		observability.Default().AddDroppedStateChanges(len(ids))
		log.Printf("state: changes channel full, %d device change(s) dropped", len(ids))
	}
}

// cloneMetrics returns a deep copy of DeviceMetrics with independently
// allocated *float64 pointer fields. Prevents external mutation from
// corrupting store state (Pitfall 6 in research).
func cloneMetrics(m domain.DeviceMetrics) domain.DeviceMetrics {
	out := m // copies all value fields; pointers still shared until overwritten
	if m.CPUPercent != nil {
		v := *m.CPUPercent
		out.CPUPercent = &v
	}
	if m.MemPercent != nil {
		v := *m.MemPercent
		out.MemPercent = &v
	}
	if m.TempCelsius != nil {
		v := *m.TempCelsius
		out.TempCelsius = &v
	}
	if m.UptimeSecs != nil {
		v := *m.UptimeSecs
		out.UptimeSecs = &v
	}
	return out
}

func applyFreshnessMetadata(next *DeviceState, update StateUpdate) {
	next.LastPolledAt = update.Timestamp
	if update.ExpectedInterval > 0 {
		next.ExpectedInterval = update.ExpectedInterval
	}
	next.Stale = false
}

func applyEssentialUpdate(next *DeviceState, prev DeviceState, update StateUpdate) {
	applyFreshnessMetadata(next, update)
	essential := update.Essential
	next.NetworkReachable = essential.NetworkReachable
	next.SNMPReachable = essential.SNMPReachable
	next.FieldStates = mergeEssentialFieldStates(prev.FieldStates, next.Metrics, essential)
	next.RuntimeFlags = cloneRuntimeFlags(prev.RuntimeFlags)
	setFlag(next.RuntimeFlags, polling.FlagDeadlineMissed, essential.DeadlineMissed)
	setFlag(next.RuntimeFlags, polling.FlagOverloaded, essential.Overloaded)

	if update.Metrics != nil {
		merged := cloneMetrics(next.Metrics)
		merged.DeviceID = update.DeviceID
		if update.Metrics.CPUPercent != nil {
			merged.CPUPercent = cloneFloat64Ptr(update.Metrics.CPUPercent)
		}
		if update.Metrics.MemPercent != nil {
			merged.MemPercent = cloneFloat64Ptr(update.Metrics.MemPercent)
		}
		if update.Metrics.UptimeSecs != nil {
			merged.UptimeSecs = cloneFloat64Ptr(update.Metrics.UptimeSecs)
		}
		merged.CollectedAt = update.Metrics.CollectedAt
		next.Metrics = merged
		next.FieldStates = mergeEssentialFieldStates(prev.FieldStates, next.Metrics, essential)
	}
	setFlag(next.RuntimeFlags, polling.FlagPartialTelemetry, essential.PollStatus == polling.PollStatusPartial && hasPartialTelemetryFields(next.FieldStates))

	switch {
	case essential.SNMPReachable == polling.TriStateTrue:
		next.Reachability = ReachabilityUp
		next.ConsecutiveFailures = 0
		next.PrimaryHealth = polling.PrimaryHealthUpFresh
		evaluateHealth(next, &next.Metrics)
	case essential.NetworkReachable == polling.TriStateTrue:
		next.ConsecutiveFailures = 0
		next.Reachability = ReachabilityUp
		next.PrimaryHealth = polling.PrimaryHealthSNMPDegraded
	case essential.NetworkReachable == polling.TriStateFalse:
		next.ConsecutiveFailures = prev.ConsecutiveFailures + 1
		if next.ConsecutiveFailures >= 3 {
			next.Reachability = ReachabilityHardDown
		} else {
			next.Reachability = ReachabilitySoftDown
		}
		next.PrimaryHealth = polling.PrimaryHealthUnreachable
	default:
		next.ConsecutiveFailures = 0
		next.Reachability = ReachabilityUnknown
		if essential.SNMPReachable == polling.TriStateFalse {
			next.PrimaryHealth = polling.PrimaryHealthSNMPDegraded
		} else {
			next.PrimaryHealth = polling.PrimaryHealthProbing
		}
	}
}

func mergeEssentialFieldStates(prev map[string]polling.FieldState, metrics domain.DeviceMetrics, essential *EssentialUpdate) map[string]polling.FieldState {
	fields := map[string]polling.FieldState{
		"uptime": polling.FieldStateMissing,
		"cpu":    polling.FieldStateMissing,
		"memory": polling.FieldStateMissing,
	}
	for key, value := range prev {
		fields[key] = value
	}

	fields["uptime"] = mergeEssentialFieldState(essential.Uptime, metrics.UptimeSecs)
	fields["cpu"] = mergeEssentialFieldState(essential.CPU, metrics.CPUPercent)
	fields["memory"] = mergeEssentialFieldState(essential.Memory, metrics.MemPercent)
	return fields
}

func mergeEssentialFieldState(observed polling.FieldState, existingValue *float64) polling.FieldState {
	if observed == polling.FieldStateMissing && existingValue != nil {
		return polling.FieldStateOK
	}
	return observed
}

func hasPartialTelemetryFields(fields map[string]polling.FieldState) bool {
	for _, key := range []string{"uptime", "cpu", "memory"} {
		if fields[key] != polling.FieldStateOK {
			return true
		}
	}
	return false
}

func applyPerformanceUpdate(next *DeviceState, update StateUpdate) {
	if !update.PollSuccess {
		return
	}

	if len(update.LinkMetrics) > 0 {
		next.LinkMetrics = cloneLinkMetrics(update.LinkMetrics)
	}

	if update.Metrics == nil {
		return
	}

	merged := cloneMetrics(next.Metrics)
	merged.DeviceID = update.DeviceID
	if update.Metrics.CPUPercent != nil {
		merged.CPUPercent = cloneFloat64Ptr(update.Metrics.CPUPercent)
	}
	if update.Metrics.MemPercent != nil {
		merged.MemPercent = cloneFloat64Ptr(update.Metrics.MemPercent)
	}
	if update.Metrics.TempCelsius != nil {
		merged.TempCelsius = cloneFloat64Ptr(update.Metrics.TempCelsius)
	}
	if update.Metrics.UptimeSecs != nil {
		merged.UptimeSecs = cloneFloat64Ptr(update.Metrics.UptimeSecs)
	}
	merged.CollectedAt = update.Metrics.CollectedAt
	next.Metrics = merged
	markMetricFieldStates(next, update.Metrics)
	evaluateHealth(next, &next.Metrics)
}

func markMetricFieldStates(next *DeviceState, metrics *domain.DeviceMetrics) {
	if metrics == nil {
		return
	}
	fields := cloneFieldStates(next.FieldStates)
	if fields == nil {
		fields = make(map[string]polling.FieldState, 3)
	}
	if metrics.UptimeSecs != nil {
		fields["uptime"] = polling.FieldStateOK
	}
	if metrics.CPUPercent != nil {
		fields["cpu"] = polling.FieldStateOK
	}
	if metrics.MemPercent != nil {
		fields["memory"] = polling.FieldStateOK
	}
	next.FieldStates = fields
}

func applyOperationalUpdate(next *DeviceState, prev DeviceState, update StateUpdate) {
	applyReachabilityUpdate(next, prev, update.PollSuccess)

	if update.Metrics != nil && update.Metrics.UptimeSecs != nil {
		merged := cloneMetrics(next.Metrics)
		merged.DeviceID = update.DeviceID
		merged.UptimeSecs = cloneFloat64Ptr(update.Metrics.UptimeSecs)
		next.Metrics = merged
	}

	if next.Reachability == ReachabilityUp {
		evaluateHealth(next, &next.Metrics)
	}
}

func applyStaticUpdate(next *DeviceState, update StateUpdate) {
	_ = next
	_ = update
}

func applyLegacyUpdate(next *DeviceState, prev DeviceState, existed bool, update StateUpdate) {
	applyReachabilityUpdate(next, prev, update.PollSuccess)
	if !update.PollSuccess {
		return
	}

	if len(update.LinkMetrics) > 0 {
		next.LinkMetrics = cloneLinkMetrics(update.LinkMetrics)
	}

	if update.Metrics != nil {
		next.Metrics = cloneMetrics(*update.Metrics)
		evaluateHealth(next, &next.Metrics)
		return
	}

	// Preserve the existing Phase 38 behavior for success-without-metrics
	// callers until all runtime paths move to explicit volatility classes.
	next.Metrics = domain.DeviceMetrics{}
	next.CPUSeverity = ""
	next.MemSeverity = ""
	next.TempSeverity = ""
	next.Health = HealthStatusUnknown
	if !existed {
		next.ConsecutiveFailures = 0
	}
}

func applyReachabilityUpdate(next *DeviceState, prev DeviceState, pollSuccess bool) {
	if pollSuccess {
		next.Reachability = ReachabilityUp
		next.ConsecutiveFailures = 0
		return
	}

	next.ConsecutiveFailures = prev.ConsecutiveFailures + 1
	if next.ConsecutiveFailures >= 3 {
		next.Reachability = ReachabilityHardDown
		return
	}
	next.Reachability = ReachabilitySoftDown
}

func cloneLinkMetrics(in []domain.LinkMetrics) []domain.LinkMetrics {
	if len(in) == 0 {
		return nil
	}

	out := make([]domain.LinkMetrics, len(in))
	for i, metric := range in {
		out[i] = metric
		out[i].TxBps = cloneFloat64Ptr(metric.TxBps)
		out[i].RxBps = cloneFloat64Ptr(metric.RxBps)
		out[i].Utilization = cloneFloat64Ptr(metric.Utilization)
	}
	return out
}

func cloneFieldStates(in map[string]polling.FieldState) map[string]polling.FieldState {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]polling.FieldState, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

func cloneRuntimeFlags(in map[polling.RuntimeFlag]bool) map[polling.RuntimeFlag]bool {
	out := make(map[polling.RuntimeFlag]bool, len(in)+2)
	for key, value := range in {
		if value {
			out[key] = true
		}
	}
	return out
}

func setFlag(flags map[polling.RuntimeFlag]bool, flag polling.RuntimeFlag, enabled bool) {
	if enabled {
		flags[flag] = true
		return
	}
	delete(flags, flag)
}

func linkMetricsEqual(a, b []domain.LinkMetrics) bool {
	if len(a) != len(b) {
		return false
	}
	if len(a) == 0 {
		return true
	}

	left := append([]domain.LinkMetrics(nil), a...)
	right := append([]domain.LinkMetrics(nil), b...)
	sort.Slice(left, func(i, j int) bool {
		if left[i].IfName != left[j].IfName {
			return left[i].IfName < left[j].IfName
		}
		return left[i].LinkID < left[j].LinkID
	})
	sort.Slice(right, func(i, j int) bool {
		if right[i].IfName != right[j].IfName {
			return right[i].IfName < right[j].IfName
		}
		return right[i].LinkID < right[j].LinkID
	})

	for i := range left {
		if left[i].LinkID != right[i].LinkID {
			return false
		}
		if left[i].DeviceID != right[i].DeviceID {
			return false
		}
		if left[i].IfName != right[i].IfName {
			return false
		}
		if !floatPtrEqual(left[i].TxBps, right[i].TxBps) {
			return false
		}
		if !floatPtrEqual(left[i].RxBps, right[i].RxBps) {
			return false
		}
		if !floatPtrEqual(left[i].Utilization, right[i].Utilization) {
			return false
		}
	}

	return true
}

func cloneFloat64Ptr(value *float64) *float64 {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

// deviceStateEqual compares two DeviceState values field by field for
// change detection. Pointer field values in DeviceMetrics are compared by
// dereferenced value (nil == nil). Field-by-field is preferred over
// reflect.DeepEqual to avoid allocation overhead (A2 in research).
func deviceStateEqual(a, b DeviceState) bool {
	if a.Health != b.Health {
		return false
	}
	if a.CPUSeverity != b.CPUSeverity || a.MemSeverity != b.MemSeverity || a.TempSeverity != b.TempSeverity {
		return false
	}
	if a.Reachability != b.Reachability {
		return false
	}
	if a.PrimaryHealth != b.PrimaryHealth || a.NetworkReachable != b.NetworkReachable || a.SNMPReachable != b.SNMPReachable {
		return false
	}
	if !fieldStatesEqual(a.FieldStates, b.FieldStates) || !runtimeFlagsEqual(a.RuntimeFlags, b.RuntimeFlags) {
		return false
	}
	if a.ConsecutiveFailures != b.ConsecutiveFailures {
		return false
	}
	if a.Stale != b.Stale {
		return false
	}
	if !a.LastPolledAt.Equal(b.LastPolledAt) {
		return false
	}
	if a.ExpectedInterval != b.ExpectedInterval {
		return false
	}
	if !floatPtrEqual(a.Metrics.CPUPercent, b.Metrics.CPUPercent) {
		return false
	}
	if !floatPtrEqual(a.Metrics.MemPercent, b.Metrics.MemPercent) {
		return false
	}
	if !floatPtrEqual(a.Metrics.TempCelsius, b.Metrics.TempCelsius) {
		return false
	}
	if !floatPtrEqual(a.Metrics.UptimeSecs, b.Metrics.UptimeSecs) {
		return false
	}
	if !linkMetricsEqual(a.LinkMetrics, b.LinkMetrics) {
		return false
	}
	return true
}

func fieldStatesEqual(a, b map[string]polling.FieldState) bool {
	if len(a) != len(b) {
		return false
	}
	for key, left := range a {
		if b[key] != left {
			return false
		}
	}
	return true
}

func runtimeFlagsEqual(a, b map[polling.RuntimeFlag]bool) bool {
	if len(a) != len(b) {
		return false
	}
	for key, left := range a {
		if b[key] != left {
			return false
		}
	}
	return true
}

func floatPtrEqual(a, b *float64) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return *a == *b
}
