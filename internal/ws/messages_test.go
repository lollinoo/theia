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

func TestCloneSnapshot_PreservesNormalizedRuntimeFields(t *testing.T) {
	deviceID := uuid.New().String()
	lastCollectedAt := "2026-04-13T13:00:00Z"

	snapshot := &SnapshotPayload{
		Devices: map[string]DeviceRuntimeDTO{
			deviceID: {
				DeviceID:          deviceID,
				OperationalStatus: "up",
				Reachability:      "up",
				Health:            "warning",
				Freshness:         "fresh",
				PrimaryReason:     "ok",
				MetricsStatus:     "available",
				MetricsReason:     "ok",
				AlertStatus:       "normal",
				FiringAlertCount:  0,
				LastCollectedAt:   &lastCollectedAt,
			},
		},
		Links: map[string]LinkRuntimeDTO{},
	}

	cloned := CloneSnapshot(snapshot)
	got, ok := cloned.Devices[deviceID]
	if !ok {
		t.Fatalf("cloned snapshot missing device runtime for %s", deviceID)
	}

	if got.OperationalStatus != "up" {
		t.Fatalf("OperationalStatus = %q, want %q", got.OperationalStatus, "up")
	}

	if got.Reachability != "up" {
		t.Fatalf("Reachability = %q, want %q", got.Reachability, "up")
	}

	if got.LastCollectedAt == nil || *got.LastCollectedAt != lastCollectedAt {
		t.Fatalf("LastCollectedAt = %#v, want %q", got.LastCollectedAt, lastCollectedAt)
	}
}

func TestNewSnapshotMessage_UsesNormalizedRuntimeContract(t *testing.T) {
	deviceID := uuid.New().String()
	lastCollectedAt := "2026-04-13T13:00:00Z"

	message := NewSnapshotMessage(&SnapshotPayload{
		Devices: map[string]DeviceRuntimeDTO{
			deviceID: {
				DeviceID:         deviceID,
				OperationalStatus: "up",
				Reachability:      "up",
				Health:            "warning",
				Freshness:         "fresh",
				PrimaryReason:     "ok",
				MetricsStatus:     "available",
				MetricsReason:     "ok",
				AlertStatus:       "normal",
				FiringAlertCount:  0,
				CPUPercent:        float64Ptr(17),
				MemPercent:        float64Ptr(34),
				LastCollectedAt:   &lastCollectedAt,
			},
		},
		Links: map[string]LinkRuntimeDTO{},
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

	deviceMetrics, ok := snapshot["devices"].(map[string]any)
	if !ok {
		t.Fatalf("devices = %#v, want object", snapshot["devices"])
	}
	metric, ok := deviceMetrics[deviceID].(map[string]any)
	if !ok {
		t.Fatalf("devices[%s] = %#v, want object", deviceID, deviceMetrics[deviceID])
	}
	if _, ok := snapshot["device_metrics"]; ok {
		t.Fatal("expected normalized snapshot to omit legacy device_metrics")
	}
	if _, ok := snapshot["link_metrics"]; ok {
		t.Fatal("expected normalized snapshot to omit legacy link_metrics")
	}
	if _, ok := snapshot["device_statuses"]; ok {
		t.Fatal("expected normalized snapshot to omit legacy device_statuses")
	}
	if got := metric["last_collected_at"]; got != lastCollectedAt {
		t.Fatalf("last_collected_at = %#v, want %q", got, lastCollectedAt)
	}
}

func float64Ptr(value float64) *float64 {
	return &value
}
