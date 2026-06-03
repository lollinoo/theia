package state

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/lollinoo/theia/internal/domain"
	"github.com/lollinoo/theia/internal/observability"
	"github.com/lollinoo/theia/internal/polling"
)

// --- Concurrent access tests (STATE-01, T-38-01) ---

func TestStoreUpdateEssentialAppliesCoherentPartialResult(t *testing.T) {
	store := NewStore()
	deviceID := uuid.New()
	collectedAt := time.Date(2026, 4, 24, 10, 0, 0, 0, time.UTC)
	cpu := 42.0
	uptime := 120.0

	store.Update(StateUpdate{
		DeviceID:         deviceID,
		ExpectedInterval: 10 * time.Second,
		Timestamp:        collectedAt,
		PollSuccess:      true,
		Metrics: &domain.DeviceMetrics{
			DeviceID:    deviceID,
			CPUPercent:  &cpu,
			UptimeSecs:  &uptime,
			CollectedAt: collectedAt,
		},
		Essential: &EssentialUpdate{
			PollStatus:       polling.PollStatusPartial,
			NetworkReachable: polling.TriStateTrue,
			SNMPReachable:    polling.TriStateTrue,
			Uptime:           polling.FieldStateOK,
			CPU:              polling.FieldStateOK,
			Memory:           polling.FieldStateMissing,
			DeadlineMissed:   true,
		},
	})

	got, ok := store.GetDevice(deviceID)
	if !ok {
		t.Fatal("expected device state")
	}
	if got.PrimaryHealth != polling.PrimaryHealthUpFresh {
		t.Fatalf("PrimaryHealth = %q, want up_fresh", got.PrimaryHealth)
	}
	if !got.RuntimeFlags[polling.FlagPartialTelemetry] {
		t.Fatalf("RuntimeFlags missing partial_telemetry: %#v", got.RuntimeFlags)
	}
	if !got.RuntimeFlags[polling.FlagDeadlineMissed] {
		t.Fatalf("RuntimeFlags missing deadline_missed: %#v", got.RuntimeFlags)
	}
	if got.FieldStates["memory"] != polling.FieldStateMissing {
		t.Fatalf("memory field state = %q, want missing", got.FieldStates["memory"])
	}
	if got.Metrics.TempCelsius != nil {
		t.Fatalf("TempCelsius = %#v, want nil for essential update", got.Metrics.TempCelsius)
	}
}

func TestStoreUpdateEssentialPreservesPerformanceMetricsWhenFieldsAreMissing(t *testing.T) {
	store := NewStore()
	deviceID := uuid.New()
	performanceAt := time.Date(2026, 4, 24, 10, 0, 0, 0, time.UTC)
	cpu := 37.0
	mem := 44.0
	uptime := 1200.0

	store.Update(StateUpdate{
		DeviceID:         deviceID,
		VolatilityClass:  domain.VolatilityClassPerformance,
		ExpectedInterval: 30 * time.Second,
		Timestamp:        performanceAt,
		PollSuccess:      true,
		Metrics: &domain.DeviceMetrics{
			DeviceID:    deviceID,
			CPUPercent:  &cpu,
			MemPercent:  &mem,
			UptimeSecs:  &uptime,
			CollectedAt: performanceAt,
		},
	})

	essentialAt := performanceAt.Add(5 * time.Second)
	freshUptime := 1205.0
	store.Update(StateUpdate{
		DeviceID:         deviceID,
		ExpectedInterval: 10 * time.Second,
		Timestamp:        essentialAt,
		PollSuccess:      true,
		Metrics: &domain.DeviceMetrics{
			DeviceID:    deviceID,
			UptimeSecs:  &freshUptime,
			CollectedAt: essentialAt,
		},
		Essential: &EssentialUpdate{
			PollStatus:       polling.PollStatusPartial,
			NetworkReachable: polling.TriStateTrue,
			SNMPReachable:    polling.TriStateTrue,
			Uptime:           polling.FieldStateOK,
			CPU:              polling.FieldStateMissing,
			Memory:           polling.FieldStateMissing,
		},
	})

	got, ok := store.GetDevice(deviceID)
	if !ok {
		t.Fatal("expected device state")
	}
	if got.Metrics.CPUPercent == nil || *got.Metrics.CPUPercent != cpu {
		t.Fatalf("CPUPercent = %#v, want preserved %v", got.Metrics.CPUPercent, cpu)
	}
	if got.Metrics.MemPercent == nil || *got.Metrics.MemPercent != mem {
		t.Fatalf("MemPercent = %#v, want preserved %v", got.Metrics.MemPercent, mem)
	}
	if got.Metrics.UptimeSecs == nil || *got.Metrics.UptimeSecs != freshUptime {
		t.Fatalf("UptimeSecs = %#v, want refreshed %v", got.Metrics.UptimeSecs, freshUptime)
	}
	if got.FieldStates["cpu"] != polling.FieldStateOK {
		t.Fatalf("cpu field state = %q, want ok while CPUPercent is preserved", got.FieldStates["cpu"])
	}
	if got.FieldStates["memory"] != polling.FieldStateOK {
		t.Fatalf("memory field state = %q, want ok while MemPercent is preserved", got.FieldStates["memory"])
	}
	if got.RuntimeFlags[polling.FlagPartialTelemetry] {
		t.Fatalf("partial_telemetry flag set despite merged metric fields being ok: %#v", got.RuntimeFlags)
	}
}

func TestStorePerformanceUpdateMarksMetricFieldStatesOK(t *testing.T) {
	store := NewStore()
	deviceID := uuid.New()
	collectedAt := time.Date(2026, 4, 24, 10, 1, 0, 0, time.UTC)
	cpu := 37.0
	mem := 44.0
	uptime := 1200.0

	store.Update(StateUpdate{
		DeviceID:         deviceID,
		VolatilityClass:  domain.VolatilityClassPerformance,
		ExpectedInterval: 30 * time.Second,
		Timestamp:        collectedAt,
		PollSuccess:      true,
		Metrics: &domain.DeviceMetrics{
			DeviceID:    deviceID,
			CPUPercent:  &cpu,
			MemPercent:  &mem,
			UptimeSecs:  &uptime,
			CollectedAt: collectedAt,
		},
	})

	got, ok := store.GetDevice(deviceID)
	if !ok {
		t.Fatal("expected device state")
	}
	if got.FieldStates["cpu"] != polling.FieldStateOK {
		t.Fatalf("cpu field state = %q, want ok", got.FieldStates["cpu"])
	}
	if got.FieldStates["memory"] != polling.FieldStateOK {
		t.Fatalf("memory field state = %q, want ok", got.FieldStates["memory"])
	}
	if got.FieldStates["uptime"] != polling.FieldStateOK {
		t.Fatalf("uptime field state = %q, want ok", got.FieldStates["uptime"])
	}
}

func TestStoreSnapshotClonesEssentialRuntimeMaps(t *testing.T) {
	store := NewStore()
	deviceID := uuid.New()
	collectedAt := time.Date(2026, 4, 24, 10, 5, 0, 0, time.UTC)

	store.Update(StateUpdate{
		DeviceID:         deviceID,
		ExpectedInterval: 10 * time.Second,
		Timestamp:        collectedAt,
		PollSuccess:      true,
		Essential: &EssentialUpdate{
			PollStatus:       polling.PollStatusPartial,
			NetworkReachable: polling.TriStateTrue,
			SNMPReachable:    polling.TriStateTrue,
			Uptime:           polling.FieldStateOK,
			CPU:              polling.FieldStateError,
			Memory:           polling.FieldStateMissing,
			DeadlineMissed:   true,
		},
	})

	snap := store.Snapshot()
	got := snap[deviceID]
	got.FieldStates["cpu"] = polling.FieldStateOK
	delete(got.RuntimeFlags, polling.FlagDeadlineMissed)

	again := store.Snapshot()[deviceID]
	if again.FieldStates["cpu"] != polling.FieldStateError {
		t.Fatalf("snapshot mutation corrupted FieldStates: cpu = %q, want error", again.FieldStates["cpu"])
	}
	if !again.RuntimeFlags[polling.FlagDeadlineMissed] {
		t.Fatalf("snapshot mutation corrupted RuntimeFlags: %#v", again.RuntimeFlags)
	}
}

func TestStoreGetDeviceClonesEssentialRuntimeMaps(t *testing.T) {
	store := NewStore()
	deviceID := uuid.New()
	collectedAt := time.Date(2026, 4, 24, 10, 10, 0, 0, time.UTC)

	store.Update(StateUpdate{
		DeviceID:         deviceID,
		ExpectedInterval: 10 * time.Second,
		Timestamp:        collectedAt,
		PollSuccess:      true,
		Essential: &EssentialUpdate{
			PollStatus:       polling.PollStatusPartial,
			NetworkReachable: polling.TriStateTrue,
			SNMPReachable:    polling.TriStateTrue,
			Uptime:           polling.FieldStateOK,
			CPU:              polling.FieldStateOK,
			Memory:           polling.FieldStateMissing,
			DeadlineMissed:   true,
		},
	})

	got, ok := store.GetDevice(deviceID)
	if !ok {
		t.Fatal("expected device state")
	}
	got.FieldStates["memory"] = polling.FieldStateOK
	delete(got.RuntimeFlags, polling.FlagPartialTelemetry)

	again, ok := store.GetDevice(deviceID)
	if !ok {
		t.Fatal("expected device state on second read")
	}
	if again.FieldStates["memory"] != polling.FieldStateMissing {
		t.Fatalf("GetDevice mutation corrupted FieldStates: memory = %q, want missing", again.FieldStates["memory"])
	}
	if !again.RuntimeFlags[polling.FlagPartialTelemetry] {
		t.Fatalf("GetDevice mutation corrupted RuntimeFlags: %#v", again.RuntimeFlags)
	}
}

func TestStalenessTransitionsPrimaryHealthUpFreshToUpStale(t *testing.T) {
	store := NewStore()
	deviceID := uuid.New()
	collectedAt := time.Date(2026, 4, 24, 10, 15, 0, 0, time.UTC)

	store.Update(StateUpdate{
		DeviceID:         deviceID,
		ExpectedInterval: 10 * time.Second,
		Timestamp:        collectedAt,
		PollSuccess:      true,
		Essential: &EssentialUpdate{
			PollStatus:       polling.PollStatusComplete,
			NetworkReachable: polling.TriStateTrue,
			SNMPReachable:    polling.TriStateTrue,
			Uptime:           polling.FieldStateOK,
			CPU:              polling.FieldStateOK,
			Memory:           polling.FieldStateOK,
		},
	})
	<-store.Changes()

	store.markStale(collectedAt.Add(21 * time.Second))

	got, ok := store.GetDevice(deviceID)
	if !ok {
		t.Fatal("expected device state")
	}
	if got.PrimaryHealth != polling.PrimaryHealthUpStale {
		t.Fatalf("PrimaryHealth = %q, want up_stale", got.PrimaryHealth)
	}
	if !got.Stale {
		t.Fatal("Stale = false, want true")
	}
	if got.RuntimeFlags == nil {
		t.Fatal("RuntimeFlags = nil, want initialized map")
	}
}

func TestStore_ConcurrentUpdateAndSnapshot(t *testing.T) {
	s := NewStore()
	if err := s.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer s.Stop()

	// Drain changes channel in a goroutine so Update's emitChanges does not
	// accumulate dropped messages.
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

	ids := make([]uuid.UUID, 5)
	for i := range ids {
		ids[i] = uuid.New()
	}

	var wg sync.WaitGroup
	for w := 0; w < 4; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 200; i++ {
				cpu := float64(i % 100)
				s.Update(StateUpdate{
					DeviceID:         ids[i%len(ids)],
					Metrics:          &domain.DeviceMetrics{CPUPercent: &cpu},
					PollSuccess:      true,
					ExpectedInterval: time.Second,
					Timestamp:        time.Now(),
				})
			}
		}()
	}
	for r := 0; r < 4; r++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 200; i++ {
				_ = s.Snapshot()
			}
		}()
	}
	wg.Wait()
}

func TestStore_SnapshotIsDeepCopy(t *testing.T) {
	s := NewStore()
	id := uuid.New()
	orig := 42.0
	s.Update(StateUpdate{
		DeviceID:         id,
		Metrics:          &domain.DeviceMetrics{CPUPercent: &orig},
		PollSuccess:      true,
		ExpectedInterval: time.Second,
		Timestamp:        time.Now(),
	})

	snap1 := s.Snapshot()
	ds1, ok := snap1[id]
	if !ok {
		t.Fatalf("device not in snapshot")
	}
	if ds1.Metrics.CPUPercent == nil || *ds1.Metrics.CPUPercent != 42.0 {
		t.Fatalf("snapshot CPU: got %v, want 42", ds1.Metrics.CPUPercent)
	}
	// Mutate through the returned pointer.
	*ds1.Metrics.CPUPercent = 999.0

	snap2 := s.Snapshot()
	ds2 := snap2[id]
	if ds2.Metrics.CPUPercent == nil || *ds2.Metrics.CPUPercent != 42.0 {
		t.Errorf("mutating snapshot corrupted store: got %v, want 42", ds2.Metrics.CPUPercent)
	}
}

func TestStoreConsumeOverflowed_IsStickyUntilConsumed(t *testing.T) {
	s := NewStore()

	for i := 0; i < cap(s.changes); i++ {
		s.changes <- []uuid.UUID{uuid.New()}
	}

	s.emitChanges([]uuid.UUID{uuid.New()})
	s.emitChanges([]uuid.UUID{uuid.New()})

	if !s.ConsumeOverflowed() {
		t.Fatal("expected overflow marker after dropped state changes")
	}
	if s.ConsumeOverflowed() {
		t.Fatal("expected overflow marker to clear after consumption")
	}
}

func TestStoreEmitChangesCoalescesDuplicateQueuedIDsWhenChannelFull(t *testing.T) {
	registry := observability.ResetDefaultForTest()
	t.Cleanup(func() {
		observability.ResetDefaultForTest()
	})

	s := NewStore()
	id := uuid.New()
	for i := 0; i < cap(s.changes); i++ {
		s.changes <- []uuid.UUID{id}
	}

	s.emitChanges([]uuid.UUID{id})

	if s.ConsumeOverflowed() {
		t.Fatal("duplicate-only coalescing should not mark overflowed")
	}

	select {
	case batch := <-s.Changes():
		if len(batch) != 1 || batch[0] != id {
			t.Fatalf("coalesced batch = %v, want [%s]", batch, id)
		}
	default:
		t.Fatal("expected coalesced state-change batch")
	}
	if len(s.changes) != 0 {
		t.Fatalf("expected only one coalesced batch, found %d queued batches", len(s.changes)+1)
	}

	metrics := string(registry.MarshalPrometheus())
	if strings.Contains(metrics, `theia_state_changes_dropped_total`) &&
		!strings.Contains(metrics, `theia_state_changes_dropped_total 0`) {
		t.Fatalf("duplicate-only coalescing should not record dropped changes, got:\n%s", metrics)
	}
}

func TestStoreEmitChangesKeepsOverflowWhenUniqueCoalescedIDsExceedCapacity(t *testing.T) {
	registry := observability.ResetDefaultForTest()
	t.Cleanup(func() {
		observability.ResetDefaultForTest()
	})

	s := NewStore()
	for i := 0; i < cap(s.changes); i++ {
		s.changes <- []uuid.UUID{uuid.New()}
	}

	s.emitChanges([]uuid.UUID{uuid.New()})

	if !s.ConsumeOverflowed() {
		t.Fatal("expected overflow marker when unique coalesced IDs exceed capacity")
	}
	select {
	case batch := <-s.Changes():
		if len(batch) != cap(s.changes) {
			t.Fatalf("coalesced unique batch length = %d, want %d", len(batch), cap(s.changes))
		}
	default:
		t.Fatal("expected bounded coalesced state-change batch")
	}

	metrics := string(registry.MarshalPrometheus())
	if !strings.Contains(metrics, `theia_state_changes_dropped_total 1`) {
		t.Fatalf("expected one dropped state change, got:\n%s", metrics)
	}
}

func TestStoreUpdate_OperationalPollDoesNotOverwritePerformanceFreshnessMetadata(t *testing.T) {
	t.Parallel()

	s := NewStore()
	id := uuid.New()
	perfAt := time.Date(2026, 4, 13, 8, 0, 0, 0, time.UTC)
	operationalAt := perfAt.Add(time.Minute)

	cpu := 42.5
	mem := 61.25
	temp := 55.5
	initialUptime := 1200.0
	updatedUptime := 1800.0
	tx := 1000.0
	rx := 2000.0
	util := 33.0

	s.Update(StateUpdate{
		DeviceID:        id,
		VolatilityClass: domain.VolatilityClassPerformance,
		Metrics: &domain.DeviceMetrics{
			CPUPercent:  &cpu,
			MemPercent:  &mem,
			TempCelsius: &temp,
			UptimeSecs:  &initialUptime,
			CollectedAt: perfAt,
		},
		LinkMetrics: []domain.LinkMetrics{
			{
				LinkID:      "link-1",
				DeviceID:    id,
				IfName:      "ether1",
				TxBps:       &tx,
				RxBps:       &rx,
				Utilization: &util,
				CollectedAt: perfAt,
			},
		},
		PollSuccess:      true,
		ExpectedInterval: 30 * time.Second,
		Timestamp:        perfAt,
	})
	<-s.Changes()

	s.Update(StateUpdate{
		DeviceID:        id,
		VolatilityClass: domain.VolatilityClassOperational,
		Metrics: &domain.DeviceMetrics{
			UptimeSecs:  &updatedUptime,
			CollectedAt: operationalAt,
		},
		PollSuccess:      true,
		ExpectedInterval: time.Minute,
		Timestamp:        operationalAt,
	})

	ds, ok := s.GetDevice(id)
	if !ok {
		t.Fatal("device missing")
	}
	assertFloatPtrEqual(t, ds.Metrics.CPUPercent, cpu, "CPUPercent")
	assertFloatPtrEqual(t, ds.Metrics.MemPercent, mem, "MemPercent")
	assertFloatPtrEqual(t, ds.Metrics.TempCelsius, temp, "TempCelsius")
	assertFloatPtrEqual(t, ds.Metrics.UptimeSecs, updatedUptime, "UptimeSecs")
	if !ds.Metrics.CollectedAt.Equal(perfAt) {
		t.Fatalf("CollectedAt = %s, want %s", ds.Metrics.CollectedAt, perfAt)
	}
	if ds.Reachability != ReachabilityUp {
		t.Fatalf("Reachability = %q, want %q", ds.Reachability, ReachabilityUp)
	}
	if len(ds.LinkMetrics) != 1 {
		t.Fatalf("LinkMetrics len = %d, want 1", len(ds.LinkMetrics))
	}
	if ds.LinkMetrics[0].IfName != "ether1" {
		t.Fatalf("IfName = %q, want %q", ds.LinkMetrics[0].IfName, "ether1")
	}
	assertFloatPtrEqual(t, ds.LinkMetrics[0].TxBps, tx, "LinkMetrics[0].TxBps")
	assertFloatPtrEqual(t, ds.LinkMetrics[0].RxBps, rx, "LinkMetrics[0].RxBps")
	assertFloatPtrEqual(t, ds.LinkMetrics[0].Utilization, util, "LinkMetrics[0].Utilization")
	if !ds.LastPolledAt.Equal(perfAt) {
		t.Fatalf("LastPolledAt = %s, want performance poll timestamp %s", ds.LastPolledAt, perfAt)
	}
	if ds.ExpectedInterval != 30*time.Second {
		t.Fatalf("ExpectedInterval = %s, want 30s performance cadence", ds.ExpectedInterval)
	}
	if ds.Stale {
		t.Fatal("Stale = true, want false")
	}
}

func TestStoreUpdate_StaticPollDoesNotOverwritePerformanceFreshnessMetadata(t *testing.T) {
	t.Parallel()

	s := NewStore()
	id := uuid.New()
	perfAt := time.Date(2026, 4, 13, 8, 30, 0, 0, time.UTC)
	staticAt := perfAt.Add(5 * time.Minute)

	cpu := 38.0
	tx := 750.0
	rx := 1250.0
	util := 21.0

	s.Update(StateUpdate{
		DeviceID:        id,
		VolatilityClass: domain.VolatilityClassPerformance,
		Metrics: &domain.DeviceMetrics{
			CPUPercent:  &cpu,
			CollectedAt: perfAt,
		},
		LinkMetrics: []domain.LinkMetrics{
			{
				LinkID:      "link-1",
				DeviceID:    id,
				IfName:      "ether1",
				TxBps:       &tx,
				RxBps:       &rx,
				Utilization: &util,
				CollectedAt: perfAt,
			},
		},
		PollSuccess:      true,
		ExpectedInterval: 30 * time.Second,
		Timestamp:        perfAt,
	})
	<-s.Changes()

	s.Update(StateUpdate{
		DeviceID:         id,
		VolatilityClass:  domain.VolatilityClassStatic,
		PollSuccess:      true,
		ExpectedInterval: 5 * time.Minute,
		Timestamp:        staticAt,
	})

	ds, ok := s.GetDevice(id)
	if !ok {
		t.Fatal("device missing")
	}
	assertFloatPtrEqual(t, ds.Metrics.CPUPercent, cpu, "CPUPercent")
	if len(ds.LinkMetrics) != 1 {
		t.Fatalf("LinkMetrics len = %d, want 1", len(ds.LinkMetrics))
	}
	assertFloatPtrEqual(t, ds.LinkMetrics[0].TxBps, tx, "LinkMetrics[0].TxBps")
	assertFloatPtrEqual(t, ds.LinkMetrics[0].RxBps, rx, "LinkMetrics[0].RxBps")
	assertFloatPtrEqual(t, ds.LinkMetrics[0].Utilization, util, "LinkMetrics[0].Utilization")
	if !ds.LastPolledAt.Equal(perfAt) {
		t.Fatalf("LastPolledAt = %s, want performance poll timestamp %s", ds.LastPolledAt, perfAt)
	}
	if ds.ExpectedInterval != 30*time.Second {
		t.Fatalf("ExpectedInterval = %s, want 30s performance cadence", ds.ExpectedInterval)
	}
	if ds.Stale {
		t.Fatal("Stale = true, want false")
	}
}

func TestStoreUpdate_FailedPerformancePollKeepsLastKnownMetricsAndLinks(t *testing.T) {
	t.Parallel()

	s := NewStore()
	id := uuid.New()
	perfAt := time.Date(2026, 4, 13, 9, 0, 0, 0, time.UTC)
	operationalAt := perfAt.Add(time.Minute)
	failedPerfAt := operationalAt.Add(30 * time.Second)

	cpu := 48.0
	mem := 58.0
	temp := 51.0
	initialUptime := 900.0
	updatedUptime := 1500.0
	tx := 1200.0
	rx := 2200.0
	util := 27.5

	s.Update(StateUpdate{
		DeviceID:        id,
		VolatilityClass: domain.VolatilityClassPerformance,
		Metrics: &domain.DeviceMetrics{
			CPUPercent:  &cpu,
			MemPercent:  &mem,
			TempCelsius: &temp,
			UptimeSecs:  &initialUptime,
			CollectedAt: perfAt,
		},
		LinkMetrics: []domain.LinkMetrics{
			{
				LinkID:      "link-1",
				DeviceID:    id,
				IfName:      "ether1",
				TxBps:       &tx,
				RxBps:       &rx,
				Utilization: &util,
				CollectedAt: perfAt,
			},
		},
		PollSuccess:      true,
		ExpectedInterval: 30 * time.Second,
		Timestamp:        perfAt,
	})
	<-s.Changes()

	s.Update(StateUpdate{
		DeviceID:        id,
		VolatilityClass: domain.VolatilityClassOperational,
		Metrics: &domain.DeviceMetrics{
			UptimeSecs:  &updatedUptime,
			CollectedAt: operationalAt,
		},
		PollSuccess:      true,
		ExpectedInterval: time.Minute,
		Timestamp:        operationalAt,
	})
	<-s.Changes()

	s.Update(StateUpdate{
		DeviceID:         id,
		VolatilityClass:  domain.VolatilityClassPerformance,
		PollSuccess:      false,
		ExpectedInterval: 30 * time.Second,
		Timestamp:        failedPerfAt,
	})

	ds, ok := s.GetDevice(id)
	if !ok {
		t.Fatal("device missing")
	}
	assertFloatPtrEqual(t, ds.Metrics.CPUPercent, cpu, "CPUPercent")
	assertFloatPtrEqual(t, ds.Metrics.MemPercent, mem, "MemPercent")
	assertFloatPtrEqual(t, ds.Metrics.TempCelsius, temp, "TempCelsius")
	assertFloatPtrEqual(t, ds.Metrics.UptimeSecs, updatedUptime, "UptimeSecs")
	if !ds.Metrics.CollectedAt.Equal(perfAt) {
		t.Fatalf("CollectedAt = %s, want %s", ds.Metrics.CollectedAt, perfAt)
	}
	if len(ds.LinkMetrics) != 1 {
		t.Fatalf("LinkMetrics len = %d, want 1", len(ds.LinkMetrics))
	}
	assertFloatPtrEqual(t, ds.LinkMetrics[0].TxBps, tx, "LinkMetrics[0].TxBps")
	assertFloatPtrEqual(t, ds.LinkMetrics[0].RxBps, rx, "LinkMetrics[0].RxBps")
	assertFloatPtrEqual(t, ds.LinkMetrics[0].Utilization, util, "LinkMetrics[0].Utilization")
	if ds.Health != HealthStatusHealthy {
		t.Fatalf("Health = %q, want %q", ds.Health, HealthStatusHealthy)
	}
	if ds.Reachability != ReachabilityUp {
		t.Fatalf("Reachability = %q, want %q", ds.Reachability, ReachabilityUp)
	}
	if ds.ConsecutiveFailures != 0 {
		t.Fatalf("ConsecutiveFailures = %d, want 0", ds.ConsecutiveFailures)
	}
	if !ds.LastPolledAt.Equal(failedPerfAt) {
		t.Fatalf("LastPolledAt = %s, want %s", ds.LastPolledAt, failedPerfAt)
	}
	if ds.ExpectedInterval != 30*time.Second {
		t.Fatalf("ExpectedInterval = %s, want 30s performance cadence", ds.ExpectedInterval)
	}
}

func TestStoreSnapshot_ClonesLinkMetrics(t *testing.T) {
	t.Parallel()

	s := NewStore()
	id := uuid.New()
	collectedAt := time.Date(2026, 4, 13, 10, 0, 0, 0, time.UTC)
	cpu := 35.0
	tx := 500.0
	rx := 700.0
	util := 12.5

	s.Update(StateUpdate{
		DeviceID:        id,
		VolatilityClass: domain.VolatilityClassPerformance,
		Metrics: &domain.DeviceMetrics{
			CPUPercent:  &cpu,
			CollectedAt: collectedAt,
		},
		LinkMetrics: []domain.LinkMetrics{
			{
				LinkID:      "link-1",
				DeviceID:    id,
				IfName:      "ether1",
				TxBps:       &tx,
				RxBps:       &rx,
				Utilization: &util,
				CollectedAt: collectedAt,
			},
		},
		PollSuccess:      true,
		ExpectedInterval: 30 * time.Second,
		Timestamp:        collectedAt,
	})

	snap := s.Snapshot()
	ds, ok := snap[id]
	if !ok {
		t.Fatal("device missing from snapshot")
	}
	if len(ds.LinkMetrics) != 1 {
		t.Fatalf("LinkMetrics len = %d, want 1", len(ds.LinkMetrics))
	}
	ds.LinkMetrics[0].IfName = "mutated"
	*ds.LinkMetrics[0].TxBps = 9999

	again := s.Snapshot()[id]
	if again.LinkMetrics[0].IfName != "ether1" {
		t.Fatalf("snapshot mutation corrupted store IfName: got %q", again.LinkMetrics[0].IfName)
	}
	assertFloatPtrEqual(t, again.LinkMetrics[0].TxBps, tx, "Snapshot LinkMetrics[0].TxBps")

	device, ok := s.GetDevice(id)
	if !ok {
		t.Fatal("device missing from GetDevice")
	}
	*device.LinkMetrics[0].RxBps = 12345

	deviceAgain, ok := s.GetDevice(id)
	if !ok {
		t.Fatal("device missing from second GetDevice")
	}
	assertFloatPtrEqual(t, deviceAgain.LinkMetrics[0].RxBps, rx, "GetDevice LinkMetrics[0].RxBps")
}

func TestStoreUpdate_LinkMetricDiffTriggersChange(t *testing.T) {
	t.Parallel()

	s := NewStore()
	id := uuid.New()
	collectedAt := time.Date(2026, 4, 13, 11, 0, 0, 0, time.UTC)
	cpu := 40.0
	initialTx := 100.0
	updatedTx := 250.0
	rx := 200.0
	util := 20.0

	baseUpdate := StateUpdate{
		DeviceID:        id,
		VolatilityClass: domain.VolatilityClassPerformance,
		Metrics: &domain.DeviceMetrics{
			CPUPercent:  &cpu,
			CollectedAt: collectedAt,
		},
		LinkMetrics: []domain.LinkMetrics{
			{
				LinkID:      "link-1",
				DeviceID:    id,
				IfName:      "ether1",
				TxBps:       &initialTx,
				RxBps:       &rx,
				Utilization: &util,
				CollectedAt: collectedAt,
			},
		},
		PollSuccess:      true,
		ExpectedInterval: 30 * time.Second,
		Timestamp:        collectedAt,
	}

	s.Update(baseUpdate)
	<-s.Changes()

	s.Update(StateUpdate{
		DeviceID:        id,
		VolatilityClass: domain.VolatilityClassPerformance,
		Metrics: &domain.DeviceMetrics{
			CPUPercent:  &cpu,
			CollectedAt: collectedAt,
		},
		LinkMetrics: []domain.LinkMetrics{
			{
				LinkID:      "link-1",
				DeviceID:    id,
				IfName:      "ether1",
				TxBps:       &updatedTx,
				RxBps:       &rx,
				Utilization: &util,
				CollectedAt: collectedAt,
			},
		},
		PollSuccess:      true,
		ExpectedInterval: 30 * time.Second,
		Timestamp:        collectedAt,
	})

	select {
	case batch := <-s.Changes():
		if len(batch) != 1 || batch[0] != id {
			t.Fatalf("expected single-ID change batch for %s, got %v", id, batch)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("link metric change did not emit")
	}
}

// --- Reachability state machine tests (STATE-04) ---

func TestReachability_SinglePollFailureIsSoftDown(t *testing.T) {
	s := NewStore()
	id := uuid.New()
	s.Update(StateUpdate{DeviceID: id, PollSuccess: false, Timestamp: time.Now(), ExpectedInterval: time.Second})

	ds, ok := s.GetDevice(id)
	if !ok {
		t.Fatal("device missing")
	}
	if ds.Reachability != ReachabilitySoftDown {
		t.Errorf("Reachability = %q, want %q", ds.Reachability, ReachabilitySoftDown)
	}
	if ds.ConsecutiveFailures != 1 {
		t.Errorf("ConsecutiveFailures = %d, want 1", ds.ConsecutiveFailures)
	}
}

func TestReachability_TwoFailuresStayedSoftDown(t *testing.T) {
	s := NewStore()
	id := uuid.New()
	for i := 0; i < 2; i++ {
		s.Update(StateUpdate{DeviceID: id, PollSuccess: false, Timestamp: time.Now(), ExpectedInterval: time.Second})
	}
	ds, _ := s.GetDevice(id)
	if ds.Reachability != ReachabilitySoftDown {
		t.Errorf("Reachability = %q, want %q", ds.Reachability, ReachabilitySoftDown)
	}
	if ds.ConsecutiveFailures != 2 {
		t.Errorf("ConsecutiveFailures = %d, want 2", ds.ConsecutiveFailures)
	}
}

func TestReachability_ThreeFailuresIsHardDown(t *testing.T) {
	s := NewStore()
	id := uuid.New()
	for i := 0; i < 3; i++ {
		s.Update(StateUpdate{DeviceID: id, PollSuccess: false, Timestamp: time.Now(), ExpectedInterval: time.Second})
	}
	ds, _ := s.GetDevice(id)
	if ds.Reachability != ReachabilityHardDown {
		t.Errorf("Reachability = %q, want %q", ds.Reachability, ReachabilityHardDown)
	}
	if ds.ConsecutiveFailures != 3 {
		t.Errorf("ConsecutiveFailures = %d, want 3", ds.ConsecutiveFailures)
	}
}

func TestEssentialSNMPFailureWithUnknownNetworkDoesNotMarkDeviceDown(t *testing.T) {
	s := NewStore()
	id := uuid.New()

	for i := 0; i < 3; i++ {
		s.Update(StateUpdate{
			DeviceID:         id,
			ExpectedInterval: time.Second,
			Timestamp:        time.Now(),
			PollSuccess:      false,
			Essential: &EssentialUpdate{
				PollStatus:       polling.PollStatusFailed,
				NetworkReachable: polling.TriStateUnknown,
				SNMPReachable:    polling.TriStateFalse,
				Uptime:           polling.FieldStateError,
				CPU:              polling.FieldStateMissing,
				Memory:           polling.FieldStateMissing,
			},
		})
	}

	ds, ok := s.GetDevice(id)
	if !ok {
		t.Fatal("device missing")
	}
	if ds.Reachability != ReachabilityUnknown {
		t.Fatalf("Reachability = %q, want %q", ds.Reachability, ReachabilityUnknown)
	}
	if ds.ConsecutiveFailures != 0 {
		t.Fatalf("ConsecutiveFailures = %d, want 0 without network-down evidence", ds.ConsecutiveFailures)
	}
	if ds.PrimaryHealth != polling.PrimaryHealthSNMPDegraded {
		t.Fatalf("PrimaryHealth = %q, want %q", ds.PrimaryHealth, polling.PrimaryHealthSNMPDegraded)
	}
}

func TestEssentialUnknownNetworkFailureDoesNotClearExistingHardDown(t *testing.T) {
	s := NewStore()
	id := uuid.New()
	now := time.Date(2026, 4, 25, 12, 0, 0, 0, time.UTC)

	for i := 0; i < 3; i++ {
		s.Update(StateUpdate{
			DeviceID:         id,
			ExpectedInterval: time.Second,
			Timestamp:        now.Add(time.Duration(i) * time.Second),
			PollSuccess:      false,
			Essential: &EssentialUpdate{
				PollStatus:       polling.PollStatusFailed,
				NetworkReachable: polling.TriStateFalse,
				SNMPReachable:    polling.TriStateFalse,
				Uptime:           polling.FieldStateError,
				CPU:              polling.FieldStateMissing,
				Memory:           polling.FieldStateMissing,
			},
		})
	}

	s.Update(StateUpdate{
		DeviceID:         id,
		ExpectedInterval: time.Second,
		Timestamp:        now.Add(4 * time.Second),
		PollSuccess:      false,
		Essential: &EssentialUpdate{
			PollStatus:       polling.PollStatusFailed,
			NetworkReachable: polling.TriStateUnknown,
			SNMPReachable:    polling.TriStateFalse,
			Uptime:           polling.FieldStateError,
			CPU:              polling.FieldStateMissing,
			Memory:           polling.FieldStateMissing,
		},
	})

	ds, ok := s.GetDevice(id)
	if !ok {
		t.Fatal("device missing")
	}
	if ds.Reachability != ReachabilityHardDown {
		t.Fatalf("Reachability = %q, want %q", ds.Reachability, ReachabilityHardDown)
	}
	if ds.NetworkReachable != polling.TriStateFalse {
		t.Fatalf("NetworkReachable = %q, want %q", ds.NetworkReachable, polling.TriStateFalse)
	}
	if ds.PrimaryHealth != polling.PrimaryHealthUnreachable {
		t.Fatalf("PrimaryHealth = %q, want %q", ds.PrimaryHealth, polling.PrimaryHealthUnreachable)
	}
}

func TestEssentialSNMPFailureWithReachableNetworkKeepsDeviceReachable(t *testing.T) {
	s := NewStore()
	id := uuid.New()

	for i := 0; i < 3; i++ {
		s.Update(StateUpdate{
			DeviceID:         id,
			ExpectedInterval: time.Second,
			Timestamp:        time.Now(),
			PollSuccess:      false,
			Essential: &EssentialUpdate{
				PollStatus:       polling.PollStatusFailed,
				NetworkReachable: polling.TriStateTrue,
				SNMPReachable:    polling.TriStateFalse,
				Uptime:           polling.FieldStateError,
				CPU:              polling.FieldStateMissing,
				Memory:           polling.FieldStateMissing,
			},
		})
	}

	ds, ok := s.GetDevice(id)
	if !ok {
		t.Fatal("device missing")
	}
	if ds.Reachability != ReachabilityUp {
		t.Fatalf("Reachability = %q, want %q", ds.Reachability, ReachabilityUp)
	}
	if ds.ConsecutiveFailures != 0 {
		t.Fatalf("ConsecutiveFailures = %d, want 0 when network is reachable", ds.ConsecutiveFailures)
	}
	if ds.PrimaryHealth != polling.PrimaryHealthSNMPDegraded {
		t.Fatalf("PrimaryHealth = %q, want %q", ds.PrimaryHealth, polling.PrimaryHealthSNMPDegraded)
	}
}

func TestOperationalFailureDoesNotOverrideReachableSNMPDegradedEvidence(t *testing.T) {
	s := NewStore()
	id := uuid.New()
	now := time.Date(2026, 4, 25, 12, 5, 0, 0, time.UTC)

	s.Update(StateUpdate{
		DeviceID:         id,
		ExpectedInterval: time.Second,
		Timestamp:        now,
		PollSuccess:      false,
		Essential: &EssentialUpdate{
			PollStatus:       polling.PollStatusFailed,
			NetworkReachable: polling.TriStateTrue,
			SNMPReachable:    polling.TriStateFalse,
			Uptime:           polling.FieldStateError,
			CPU:              polling.FieldStateMissing,
			Memory:           polling.FieldStateMissing,
		},
	})

	for i := 0; i < 3; i++ {
		s.Update(StateUpdate{
			DeviceID:         id,
			VolatilityClass:  domain.VolatilityClassOperational,
			ExpectedInterval: time.Second,
			Timestamp:        now.Add(time.Duration(i+1) * time.Second),
			PollSuccess:      false,
		})
	}

	ds, ok := s.GetDevice(id)
	if !ok {
		t.Fatal("device missing")
	}
	if ds.Reachability != ReachabilityUp {
		t.Fatalf("Reachability = %q, want %q", ds.Reachability, ReachabilityUp)
	}
	if ds.ConsecutiveFailures != 0 {
		t.Fatalf("ConsecutiveFailures = %d, want 0 while network is reachable", ds.ConsecutiveFailures)
	}
	if ds.PrimaryHealth != polling.PrimaryHealthSNMPDegraded {
		t.Fatalf("PrimaryHealth = %q, want %q", ds.PrimaryHealth, polling.PrimaryHealthSNMPDegraded)
	}
	if ds.NetworkReachable != polling.TriStateTrue || ds.SNMPReachable != polling.TriStateFalse {
		t.Fatalf("reachability evidence = network %q snmp %q, want true/false", ds.NetworkReachable, ds.SNMPReachable)
	}
}

func TestReachability_SuccessResetsToUp(t *testing.T) {
	s := NewStore()
	id := uuid.New()
	for i := 0; i < 3; i++ {
		s.Update(StateUpdate{DeviceID: id, PollSuccess: false, Timestamp: time.Now(), ExpectedInterval: time.Second})
	}
	cpu := 50.0
	s.Update(StateUpdate{
		DeviceID:         id,
		Metrics:          &domain.DeviceMetrics{CPUPercent: &cpu},
		PollSuccess:      true,
		ExpectedInterval: time.Second,
		Timestamp:        time.Now(),
	})
	ds, _ := s.GetDevice(id)
	if ds.Reachability != ReachabilityUp {
		t.Errorf("Reachability = %q, want %q", ds.Reachability, ReachabilityUp)
	}
	if ds.ConsecutiveFailures != 0 {
		t.Errorf("ConsecutiveFailures = %d, want 0", ds.ConsecutiveFailures)
	}
}

func TestReachability_HealthFrozenOnSoftDown(t *testing.T) {
	s := NewStore()
	id := uuid.New()
	// First: healthy
	cpu := 50.0
	s.Update(StateUpdate{
		DeviceID:         id,
		Metrics:          &domain.DeviceMetrics{CPUPercent: &cpu},
		PollSuccess:      true,
		ExpectedInterval: time.Second,
		Timestamp:        time.Now(),
	})
	// Then: single failure
	s.Update(StateUpdate{DeviceID: id, PollSuccess: false, Timestamp: time.Now(), ExpectedInterval: time.Second})

	ds, _ := s.GetDevice(id)
	if ds.Reachability != ReachabilitySoftDown {
		t.Fatalf("Reachability = %q, want %q", ds.Reachability, ReachabilitySoftDown)
	}
	if ds.Health != HealthStatusHealthy {
		t.Errorf("Health = %q, want %q (frozen)", ds.Health, HealthStatusHealthy)
	}
}

func TestReachability_HardDownToUpWithNilMetricsResetsHealthToUnknown(t *testing.T) {
	// WR-04: after a hard_down → up transition where the successful poll
	// carries no metric payload (Metrics=nil), Health must NOT remain frozen
	// at the pre-outage Critical value. The store should report Unknown
	// instead of misleading the operator with stale health data.
	s := NewStore()
	id := uuid.New()

	// Step 1: healthy-then-critical poll to set per-metric severities.
	cpuCritical := 95.0
	s.Update(StateUpdate{
		DeviceID:         id,
		Metrics:          &domain.DeviceMetrics{CPUPercent: &cpuCritical},
		PollSuccess:      true,
		ExpectedInterval: time.Second,
		Timestamp:        time.Now(),
	})
	ds, _ := s.GetDevice(id)
	if ds.Health != HealthStatusCritical {
		t.Fatalf("precondition: Health = %q, want %q", ds.Health, HealthStatusCritical)
	}

	// Step 2: three consecutive failures → hard_down with Health frozen.
	for i := 0; i < 3; i++ {
		s.Update(StateUpdate{DeviceID: id, PollSuccess: false, Timestamp: time.Now(), ExpectedInterval: time.Second})
	}
	ds, _ = s.GetDevice(id)
	if ds.Reachability != ReachabilityHardDown {
		t.Fatalf("precondition: Reachability = %q, want %q", ds.Reachability, ReachabilityHardDown)
	}
	if ds.Health != HealthStatusCritical {
		t.Fatalf("precondition: Health frozen = %q, want %q", ds.Health, HealthStatusCritical)
	}

	// Step 3: successful poll with nil metrics — e.g. transport succeeded
	// but metric extraction returned empty.
	s.Update(StateUpdate{
		DeviceID:         id,
		Metrics:          nil,
		PollSuccess:      true,
		ExpectedInterval: time.Second,
		Timestamp:        time.Now(),
	})
	ds, _ = s.GetDevice(id)
	if ds.Reachability != ReachabilityUp {
		t.Errorf("Reachability = %q, want %q", ds.Reachability, ReachabilityUp)
	}
	if ds.Health != HealthStatusUnknown {
		t.Errorf("Health = %q, want %q (stale frozen Critical must be reset)", ds.Health, HealthStatusUnknown)
	}
	if ds.CPUSeverity != "" {
		t.Errorf("CPUSeverity = %q, want empty", ds.CPUSeverity)
	}
	if ds.MemSeverity != "" {
		t.Errorf("MemSeverity = %q, want empty", ds.MemSeverity)
	}
	if ds.TempSeverity != "" {
		t.Errorf("TempSeverity = %q, want empty", ds.TempSeverity)
	}
}

func TestReachability_HealthFrozenOnHardDown(t *testing.T) {
	s := NewStore()
	id := uuid.New()
	cpu := 95.0 // critical
	s.Update(StateUpdate{
		DeviceID:         id,
		Metrics:          &domain.DeviceMetrics{CPUPercent: &cpu},
		PollSuccess:      true,
		ExpectedInterval: time.Second,
		Timestamp:        time.Now(),
	})
	for i := 0; i < 3; i++ {
		s.Update(StateUpdate{DeviceID: id, PollSuccess: false, Timestamp: time.Now(), ExpectedInterval: time.Second})
	}
	ds, _ := s.GetDevice(id)
	if ds.Reachability != ReachabilityHardDown {
		t.Fatalf("Reachability = %q, want %q", ds.Reachability, ReachabilityHardDown)
	}
	if ds.Health != HealthStatusCritical {
		t.Errorf("Health = %q, want %q (frozen at last known)", ds.Health, HealthStatusCritical)
	}
}

// --- Change emission tests (STATE-05) ---

func TestChanges_FirstUpdateAlwaysEmits(t *testing.T) {
	s := NewStore()
	id := uuid.New()
	cpu := 50.0
	s.Update(StateUpdate{
		DeviceID:         id,
		Metrics:          &domain.DeviceMetrics{CPUPercent: &cpu},
		PollSuccess:      true,
		ExpectedInterval: time.Second,
		Timestamp:        time.Now(),
	})

	select {
	case batch := <-s.Changes():
		if len(batch) != 1 || batch[0] != id {
			t.Errorf("expected single-ID batch for %s, got %v", id, batch)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("no change emitted for first Update")
	}
}

func TestSnapshot_CollectedAtAdvancesOnIdenticalMetricValues(t *testing.T) {
	// WR-05: two polls with byte-identical metric values but different
	// CollectedAt timestamps must keep Snapshot()'s Metrics.CollectedAt
	// fresh (the store always writes the map on Update, even when the
	// diff equality reports no semantic change).
	s := NewStore()
	id := uuid.New()
	cpu := 50.0

	t1 := time.Unix(1_700_000_000, 0)
	s.Update(StateUpdate{
		DeviceID: id,
		Metrics: &domain.DeviceMetrics{
			CPUPercent:  &cpu,
			CollectedAt: t1,
		},
		PollSuccess:      true,
		ExpectedInterval: time.Second,
		Timestamp:        t1,
	})
	ds1, _ := s.GetDevice(id)
	if !ds1.Metrics.CollectedAt.Equal(t1) {
		t.Fatalf("after first update, CollectedAt = %v, want %v", ds1.Metrics.CollectedAt, t1)
	}

	t2 := t1.Add(30 * time.Second)
	s.Update(StateUpdate{
		DeviceID: id,
		Metrics: &domain.DeviceMetrics{
			CPUPercent:  &cpu, // same value
			CollectedAt: t2,   // advanced timestamp
		},
		PollSuccess:      true,
		ExpectedInterval: time.Second,
		Timestamp:        t2,
	})
	ds2, _ := s.GetDevice(id)
	if !ds2.Metrics.CollectedAt.Equal(t2) {
		t.Errorf("after identical-value update, CollectedAt = %v, want %v (Snapshot must track latest poll)", ds2.Metrics.CollectedAt, t2)
	}
	if !ds2.LastPolledAt.Equal(t2) {
		t.Errorf("LastPolledAt = %v, want %v", ds2.LastPolledAt, t2)
	}
}

func TestChanges_UnchangedDeviceDoesNotEmit(t *testing.T) {
	s := NewStore()
	id := uuid.New()
	cpu := 50.0
	ts := time.Unix(1_700_000_000, 0)
	update := StateUpdate{
		DeviceID:         id,
		Metrics:          &domain.DeviceMetrics{CPUPercent: &cpu},
		PollSuccess:      true,
		ExpectedInterval: time.Second,
		Timestamp:        ts, // FIXED timestamp so LastPolledAt does not change
	}

	s.Update(update)
	// Drain the first change.
	<-s.Changes()

	// Apply the same update again — must NOT emit.
	s.Update(update)
	select {
	case batch := <-s.Changes():
		t.Errorf("unchanged Update unexpectedly emitted: %v", batch)
	case <-time.After(150 * time.Millisecond):
		// expected
	}
}

func TestChanges_ChangedDeviceEmits(t *testing.T) {
	s := NewStore()
	id := uuid.New()
	cpu := 50.0
	s.Update(StateUpdate{
		DeviceID:         id,
		Metrics:          &domain.DeviceMetrics{CPUPercent: &cpu},
		PollSuccess:      true,
		ExpectedInterval: time.Second,
		Timestamp:        time.Now(),
	})
	<-s.Changes() // drain first

	cpu2 := 75.0
	s.Update(StateUpdate{
		DeviceID:         id,
		Metrics:          &domain.DeviceMetrics{CPUPercent: &cpu2},
		PollSuccess:      true,
		ExpectedInterval: time.Second,
		Timestamp:        time.Now(),
	})
	select {
	case batch := <-s.Changes():
		if len(batch) != 1 || batch[0] != id {
			t.Errorf("expected single-ID batch for %s, got %v", id, batch)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("changed Update did not emit")
	}
}

// --- Staleness tests ---

func TestStaleness_MarksStaleAfterThreshold(t *testing.T) {
	s := NewStore()
	id := uuid.New()
	pastTimestamp := time.Now().Add(-1 * time.Minute)
	cpu := 50.0
	s.Update(StateUpdate{
		DeviceID:         id,
		Metrics:          &domain.DeviceMetrics{CPUPercent: &cpu},
		PollSuccess:      true,
		ExpectedInterval: 10 * time.Second, // 2x = 20s, which is in the past
		Timestamp:        pastTimestamp,
	})
	<-s.Changes() // drain the update

	s.markStale(time.Now())

	ds, _ := s.GetDevice(id)
	if !ds.Stale {
		t.Errorf("device should be marked Stale; got Stale=%v", ds.Stale)
	}
	select {
	case batch := <-s.Changes():
		if len(batch) != 1 || batch[0] != id {
			t.Errorf("expected stale emission for %s, got %v", id, batch)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("no stale change emitted")
	}
}

func TestStaleness_FreshDeviceNotMarked(t *testing.T) {
	s := NewStore()
	id := uuid.New()
	cpu := 50.0
	s.Update(StateUpdate{
		DeviceID:         id,
		Metrics:          &domain.DeviceMetrics{CPUPercent: &cpu},
		PollSuccess:      true,
		ExpectedInterval: time.Hour, // 2*1h in the future, never stale
		Timestamp:        time.Now(),
	})
	<-s.Changes()

	s.markStale(time.Now())

	ds, _ := s.GetDevice(id)
	if ds.Stale {
		t.Errorf("fresh device incorrectly marked stale")
	}
}

func TestStaleness_UpdateClearsStaleFlag(t *testing.T) {
	s := NewStore()
	id := uuid.New()
	cpu := 50.0
	pastTimestamp := time.Now().Add(-1 * time.Minute)
	s.Update(StateUpdate{
		DeviceID:         id,
		Metrics:          &domain.DeviceMetrics{CPUPercent: &cpu},
		PollSuccess:      true,
		ExpectedInterval: 10 * time.Second,
		Timestamp:        pastTimestamp,
	})
	<-s.Changes()
	s.markStale(time.Now())
	<-s.Changes()

	ds, _ := s.GetDevice(id)
	if !ds.Stale {
		t.Fatal("precondition: device should be stale before fresh update")
	}

	// Fresh update clears stale flag.
	s.Update(StateUpdate{
		DeviceID:         id,
		Metrics:          &domain.DeviceMetrics{CPUPercent: &cpu},
		PollSuccess:      true,
		ExpectedInterval: 10 * time.Second,
		Timestamp:        time.Now(),
	})
	ds, _ = s.GetDevice(id)
	if ds.Stale {
		t.Errorf("Update should have cleared Stale flag")
	}
}

// --- Lifecycle tests ---

func TestStore_StartStopIsCleanShutdown(t *testing.T) {
	s := NewStore()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := s.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	stopReturned := make(chan struct{})
	go func() {
		s.Stop()
		close(stopReturned)
	}()
	select {
	case <-stopReturned:
		// ok
	case <-time.After(2 * time.Second):
		t.Fatal("Stop() did not return within 2 seconds — goroutine leak suspected")
	}
}

func TestStore_StopWithoutStartIsNoOp(t *testing.T) {
	s := NewStore()
	s.Stop() // must not panic or hang
}

func TestStore_StartReturnsErrAlreadyStarted(t *testing.T) {
	s := NewStore()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := s.Start(ctx); err != nil {
		t.Fatalf("first Start() error = %v", err)
	}
	defer s.Stop()

	if err := s.Start(ctx); !errors.Is(err, ErrAlreadyStarted) {
		t.Fatalf("second Start() error = %v, want ErrAlreadyStarted", err)
	}
}

func TestStore_CanRestartAfterStop(t *testing.T) {
	s := NewStore()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := s.Start(ctx); err != nil {
		t.Fatalf("first Start() error = %v", err)
	}

	s.Stop()
	s.Stop()

	if err := s.Start(ctx); err != nil {
		t.Fatalf("restart Start() error = %v", err)
	}

	s.Stop()
	s.Stop()
}

func TestStore_UpdateDropsChangesWhenChannelFull(t *testing.T) {
	registry := observability.ResetDefaultForTest()
	t.Cleanup(func() {
		observability.ResetDefaultForTest()
	})

	s := NewStore()
	for i := 0; i < cap(s.changes); i++ {
		s.changes <- []uuid.UUID{uuid.New()}
	}

	id := uuid.New()
	cpu := 50.0
	s.Update(StateUpdate{
		DeviceID:         id,
		Metrics:          &domain.DeviceMetrics{CPUPercent: &cpu},
		PollSuccess:      true,
		ExpectedInterval: time.Second,
		Timestamp:        time.Now(),
	})

	if _, ok := s.GetDevice(id); !ok {
		t.Fatal("device state was not persisted when changes channel overflowed")
	}

	metrics := string(registry.MarshalPrometheus())
	if !strings.Contains(metrics, `theia_state_changes_dropped_total 1`) {
		t.Fatalf("expected dropped state change metric, got:\n%s", metrics)
	}
}

// --- Remove test ---

func TestStore_RemoveDevice(t *testing.T) {
	s := NewStore()
	id := uuid.New()
	cpu := 50.0
	s.Update(StateUpdate{
		DeviceID:         id,
		Metrics:          &domain.DeviceMetrics{CPUPercent: &cpu},
		PollSuccess:      true,
		ExpectedInterval: time.Second,
		Timestamp:        time.Now(),
	})
	<-s.Changes()

	s.Remove(id)

	if _, ok := s.GetDevice(id); ok {
		t.Errorf("device still present after Remove")
	}
	snap := s.Snapshot()
	if _, ok := snap[id]; ok {
		t.Errorf("device still in Snapshot after Remove")
	}
	select {
	case batch := <-s.Changes():
		if len(batch) != 1 || batch[0] != id {
			t.Errorf("expected remove emission for %s, got %v", id, batch)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("no change emitted for Remove")
	}
}

func assertFloatPtrEqual(t *testing.T, got *float64, want float64, field string) {
	t.Helper()

	if got == nil {
		t.Fatalf("%s = nil, want %v", field, want)
	}
	if *got != want {
		t.Fatalf("%s = %v, want %v", field, *got, want)
	}
}
