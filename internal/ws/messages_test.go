package ws

import (
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

func TestCloneSnapshot_PreservesOptionalDetailFields(t *testing.T) {
	stale := true
	expectedPollIntervalSeconds := int64(45)
	deviceID := uuid.New().String()

	snapshot := &SnapshotPayload{
		DeviceMetrics: map[string]DeviceMetricsDTO{
			deviceID: {
				DeviceID:                    deviceID,
				CPUPercent:                  float64Ptr(17),
				Health:                      "warning",
				Reachability:                "reachable",
				Stale:                       &stale,
				LastPolledAt:                "2026-04-13T13:00:00Z",
				ExpectedPollIntervalSeconds: &expectedPollIntervalSeconds,
				CollectedAt:                 "2026-04-13T13:00:00Z",
			},
		},
		LinkMetrics:     map[string][]LinkMetricsDTO{},
		Alerts:          []AlertDTO{},
		DeviceStatuses:  map[string]string{},
		DeviceHostnames: map[string]string{},
		DeviceModels:    map[string]string{},
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

	if got.LastPolledAt != "2026-04-13T13:00:00Z" {
		t.Fatalf("LastPolledAt = %q, want %q", got.LastPolledAt, "2026-04-13T13:00:00Z")
	}

	if got.ExpectedPollIntervalSeconds == nil || *got.ExpectedPollIntervalSeconds != expectedPollIntervalSeconds {
		t.Fatalf("ExpectedPollIntervalSeconds = %v, want %d", got.ExpectedPollIntervalSeconds, expectedPollIntervalSeconds)
	}
}

func float64Ptr(value float64) *float64 {
	return &value
}
