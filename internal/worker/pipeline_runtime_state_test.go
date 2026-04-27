package worker

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/lollinoo/theia/internal/collector"
	"github.com/lollinoo/theia/internal/domain"
	"github.com/lollinoo/theia/internal/state"
	"github.com/lollinoo/theia/internal/ws"
)

func TestPipelineRuntimeStateSetAlertsIncrementsVersionOnlyOnChange(t *testing.T) {
	deviceID := uuid.New()
	runtime := newPipelineRuntimeState(ws.PrometheusStatusPayload{})

	runtime.setAlerts(map[uuid.UUID][]domain.AlertState{
		deviceID: {{
			DeviceID:  deviceID,
			Severity:  "critical",
			AlertName: "DeviceDown",
			State:     "firing",
			Summary:   "device down",
		}},
	})
	first := runtime.getAlerts()
	if first.Version != 1 {
		t.Fatalf("first alert version = %d, want 1", first.Version)
	}
	if len(first.Alerts) != 1 {
		t.Fatalf("expected 1 alert after initial set, got %d", len(first.Alerts))
	}

	runtime.setAlerts(map[uuid.UUID][]domain.AlertState{
		deviceID: {{
			DeviceID:  deviceID,
			Severity:  "critical",
			AlertName: "DeviceDown",
			State:     "firing",
			Summary:   "device down",
		}},
	})
	second := runtime.getAlerts()
	if second.Version != first.Version {
		t.Fatalf("unchanged alert version = %d, want %d", second.Version, first.Version)
	}

	runtime.setAlerts(map[uuid.UUID][]domain.AlertState{
		deviceID: {{
			DeviceID:  deviceID,
			Severity:  "warning",
			AlertName: "DeviceDown",
			State:     "firing",
			Summary:   "device down",
		}},
	})
	third := runtime.getAlerts()
	if third.Version != first.Version+1 {
		t.Fatalf("changed alert version = %d, want %d", third.Version, first.Version+1)
	}
	if len(third.Alerts) != 1 || third.Alerts[0].Severity != "warning" {
		t.Fatalf("expected updated alert payload, got %#v", third.Alerts)
	}
}

func TestPipelineRuntimeStatePrunePrometheusHostnamesDropsExpiredEntries(t *testing.T) {
	now := time.Date(2026, 4, 18, 12, 0, 0, 0, time.UTC)
	freshDeviceID := uuid.New()
	expiredDeviceID := uuid.New()
	runtime := newPipelineRuntimeState(ws.PrometheusStatusPayload{})
	runtime.now = func() time.Time { return now }

	runtime.recordPrometheusHostname(freshDeviceID, "fresh-host")
	runtime.recordPrometheusHostname(expiredDeviceID, "expired-host")

	runtime.mu.Lock()
	runtime.hostnameObservedAt[expiredDeviceID] = now.Add(-prometheusEnrichmentRetention - time.Second)
	runtime.mu.Unlock()

	runtime.prunePrometheusHostnames()

	runtime.mu.RLock()
	defer runtime.mu.RUnlock()
	if got := runtime.hostnames[freshDeviceID]; got != "fresh-host" {
		t.Fatalf("fresh hostname = %q, want %q", got, "fresh-host")
	}
	if got := runtime.hostnames[expiredDeviceID]; got != "" {
		t.Fatalf("expired hostname = %q, want empty", got)
	}
	if _, ok := runtime.hostnameObservedAt[expiredDeviceID]; ok {
		t.Fatal("expected expired observation timestamp to be pruned")
	}
	if _, ok := runtime.hostnameObservedAt[freshDeviceID]; !ok {
		t.Fatal("expected fresh observation timestamp to remain")
	}
}

func TestPipelineOrchestratorResetDeviceRuntimeClearsVolatileDeviceState(t *testing.T) {
	deviceID := uuid.New()
	store := state.NewStore()
	cpu := 42.0
	now := time.Date(2026, 4, 27, 8, 30, 0, 0, time.UTC)
	store.Update(state.StateUpdate{
		DeviceID:        deviceID,
		VolatilityClass: domain.VolatilityClassPerformance,
		Metrics: &domain.DeviceMetrics{
			DeviceID:    deviceID,
			CPUPercent:  &cpu,
			CollectedAt: now,
		},
		PollSuccess:      true,
		ExpectedInterval: 30 * time.Second,
		Timestamp:        now,
	})
	<-store.Changes()

	pipeline := &PipelineOrchestrator{
		stateStore: store,
		runtime:    newPipelineRuntimeState(ws.PrometheusStatusPayload{}),
	}
	pipeline.runtime.prevCounters[deviceID] = map[string]collector.CounterBaseline{
		"ether1": {InOctets: 100, OutOctets: 200, SampledAt: now},
	}
	pipeline.runtime.hostnames[deviceID] = "old-prom-host"
	pipeline.runtime.hostnameObservedAt[deviceID] = now

	pipeline.ResetDeviceRuntime(deviceID)

	if _, ok := store.GetDevice(deviceID); ok {
		t.Fatal("expected reset to remove device state from store")
	}

	pipeline.runtime.mu.RLock()
	_, countersPresent := pipeline.runtime.prevCounters[deviceID]
	hostname := pipeline.runtime.hostnames[deviceID]
	_, hostnameObserved := pipeline.runtime.hostnameObservedAt[deviceID]
	pipeline.runtime.mu.RUnlock()
	if countersPresent {
		t.Fatal("expected reset to remove interface counter baselines")
	}
	if hostname != "" {
		t.Fatalf("expected reset to remove prometheus hostname, got %q", hostname)
	}
	if hostnameObserved {
		t.Fatal("expected reset to remove prometheus hostname observation timestamp")
	}

	select {
	case ids := <-store.Changes():
		if len(ids) != 1 || ids[0] != deviceID {
			t.Fatalf("reset emitted changed device IDs = %v, want [%s]", ids, deviceID)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("expected reset to emit a state change")
	}
}
