package ws

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
)

func TestParseClientControlMessage_SubscribeDetail(t *testing.T) {
	deviceID := uuid.New()

	cmd, err := parseClientControlMessage([]byte(`{"type":"subscribe_detail","payload":{"device_id":"` + deviceID.String() + `"}}`))
	if err != nil {
		t.Fatalf("parseClientControlMessage returned error: %v", err)
	}

	if cmd.Type != MessageTypeSubscribeDetail {
		t.Fatalf("Type = %q, want %q", cmd.Type, MessageTypeSubscribeDetail)
	}

	if cmd.DeviceID != deviceID {
		t.Fatalf("DeviceID = %s, want %s", cmd.DeviceID, deviceID)
	}
}

func TestParseClientControlMessage_UnsubscribeDetailAllowsEmptyDevice(t *testing.T) {
	tests := []struct {
		name string
		raw  string
	}{
		{
			name: "missing device id",
			raw:  `{"type":"unsubscribe_detail","payload":{}}`,
		},
		{
			name: "empty device id",
			raw:  `{"type":"unsubscribe_detail","payload":{"device_id":""}}`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cmd, err := parseClientControlMessage([]byte(tc.raw))
			if err != nil {
				t.Fatalf("parseClientControlMessage returned error: %v", err)
			}

			if cmd.Type != MessageTypeUnsubscribeDetail {
				t.Fatalf("Type = %q, want %q", cmd.Type, MessageTypeUnsubscribeDetail)
			}

			if cmd.DeviceID != uuid.Nil {
				t.Fatalf("DeviceID = %s, want nil UUID", cmd.DeviceID)
			}
		})
	}
}

func TestParseClientControlMessage_RejectsBadSubscribeUUID(t *testing.T) {
	_, err := parseClientControlMessage([]byte(`{"type":"subscribe_detail","payload":{"device_id":"not-a-uuid"}}`))
	if err == nil {
		t.Fatal("parseClientControlMessage error = nil, want error")
	}
}

func TestCloneSnapshot_PreservesSlimOverviewFields(t *testing.T) {
	stale := true
	deviceID := uuid.New().String()

	snapshot := &SnapshotPayload{
		DeviceMetrics: map[string]DeviceMetricsDTO{
			deviceID: {
				DeviceID:     deviceID,
				CPUPercent:   float64Ptr(17),
				Health:       "warning",
				Reachability: "reachable",
				Stale:        &stale,
				CollectedAt:  "2026-04-13T13:00:00Z",
			},
		},
		LinkMetrics:    map[string][]LinkMetricsDTO{},
		DeviceStatuses: map[string]string{},
	}

	cloned := CloneSnapshot(snapshot)
	got, ok := cloned.DeviceMetrics[deviceID]
	if !ok {
		t.Fatalf("cloned snapshot missing device metrics for %s", deviceID)
	}

	if got.Health != "warning" {
		t.Fatalf("Health = %q, want %q", got.Health, "warning")
	}

	if got.Reachability != "reachable" {
		t.Fatalf("Reachability = %q, want %q", got.Reachability, "reachable")
	}

	if got.Stale == nil || *got.Stale != stale {
		t.Fatalf("Stale = %v, want %v", got.Stale, stale)
	}
}

func TestNewSnapshotMessage_UsesSlimOverviewContract(t *testing.T) {
	deviceID := uuid.New().String()

	message := NewSnapshotMessage(&SnapshotPayload{
		DeviceMetrics: map[string]DeviceMetricsDTO{
			deviceID: {
				DeviceID:    deviceID,
				CPUPercent:  float64Ptr(17),
				MemPercent:  float64Ptr(34),
				CollectedAt: "2026-04-13T13:00:00Z",
				Health:      "warning",
			},
		},
		LinkMetrics:    map[string][]LinkMetricsDTO{},
		DeviceStatuses: map[string]string{deviceID: "down"},
	}, 42)

	raw, err := json.Marshal(message)
	if err != nil {
		t.Fatalf("json.Marshal returned error: %v", err)
	}

	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("json.Unmarshal returned error: %v", err)
	}

	payload, ok := decoded["payload"].(map[string]any)
	if !ok {
		t.Fatalf("payload = %#v, want object", decoded["payload"])
	}
	snapshot, ok := payload["snapshot"].(map[string]any)
	if !ok {
		t.Fatalf("snapshot = %#v, want object", payload["snapshot"])
	}
	if _, ok := snapshot["alerts"]; ok {
		t.Fatal("expected slim snapshot to omit alerts")
	}
	if _, ok := snapshot["device_hostnames"]; ok {
		t.Fatal("expected slim snapshot to omit device_hostnames")
	}
	if _, ok := snapshot["device_models"]; ok {
		t.Fatal("expected slim snapshot to omit device_models")
	}

	deviceMetrics, ok := snapshot["device_metrics"].(map[string]any)
	if !ok {
		t.Fatalf("device_metrics = %#v, want object", snapshot["device_metrics"])
	}
	metric, ok := deviceMetrics[deviceID].(map[string]any)
	if !ok {
		t.Fatalf("device_metrics[%s] = %#v, want object", deviceID, deviceMetrics[deviceID])
	}
	if _, ok := metric["temp_celsius"]; ok {
		t.Fatal("expected slim device_metrics to omit temp_celsius")
	}
	if _, ok := metric["uptime_secs"]; ok {
		t.Fatal("expected slim device_metrics to omit uptime_secs")
	}
	if _, ok := metric["last_polled_at"]; ok {
		t.Fatal("expected slim device_metrics to omit last_polled_at")
	}
	if _, ok := metric["expected_poll_interval_seconds"]; ok {
		t.Fatal("expected slim device_metrics to omit expected_poll_interval_seconds")
	}
}

func float64Ptr(value float64) *float64 {
	return &value
}
