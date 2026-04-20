package worker

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/lollinoo/theia/internal/domain"
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
