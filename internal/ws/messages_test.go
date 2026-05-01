package ws

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/polling"
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

func TestParseClientControlMessage_HelloAllowsKnownVersions(t *testing.T) {
	cmd, err := parseClientControlMessage([]byte(`{
		"type":"hello",
		"payload":{
			"canvas_schema_version":1,
			"topology_version":"topo-123",
			"runtime_version":42,
			"runtime_identity":"rt-sha256:abc",
			"alert_version":7
		}
	}`))
	if err != nil {
		t.Fatalf("parseClientControlMessage returned error: %v", err)
	}

	if cmd.Type != MessageTypeHello {
		t.Fatalf("Type = %q, want %q", cmd.Type, MessageTypeHello)
	}
	if cmd.RuntimeVersion == nil || *cmd.RuntimeVersion != 42 {
		t.Fatalf("RuntimeVersion = %#v, want 42", cmd.RuntimeVersion)
	}
	if cmd.AlertVersion == nil || *cmd.AlertVersion != 7 {
		t.Fatalf("AlertVersion = %#v, want 7", cmd.AlertVersion)
	}
	if cmd.TopologyVersion != "topo-123" {
		t.Fatalf("TopologyVersion = %q, want topo-123", cmd.TopologyVersion)
	}
	if cmd.RuntimeIdentity != "rt-sha256:abc" {
		t.Fatalf("RuntimeIdentity = %q, want rt-sha256:abc", cmd.RuntimeIdentity)
	}
	if cmd.CanvasSchemaVersion != 1 {
		t.Fatalf("CanvasSchemaVersion = %d, want 1", cmd.CanvasSchemaVersion)
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
				PrimaryHealth:     "up_fresh",
				RuntimeFlags:      []string{"partial_telemetry"},
				FieldStates:       map[string]string{"uptime": "ok", "cpu": "missing", "memory": "ok"},
				NetworkReachable:  "true",
				SNMPReachable:     "true",
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
	if got.PrimaryHealth != "up_fresh" {
		t.Fatalf("PrimaryHealth = %q, want up_fresh", got.PrimaryHealth)
	}
	if len(got.RuntimeFlags) != 1 || got.RuntimeFlags[0] != "partial_telemetry" {
		t.Fatalf("RuntimeFlags = %#v, want partial_telemetry", got.RuntimeFlags)
	}
	if got.FieldStates["cpu"] != "missing" {
		t.Fatalf("FieldStates[cpu] = %q, want missing", got.FieldStates["cpu"])
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
				DeviceID:          deviceID,
				OperationalStatus: "up",
				Reachability:      "up",
				Health:            "warning",
				Freshness:         "fresh",
				PrimaryHealth:     "up_fresh",
				RuntimeFlags:      []string{},
				FieldStates:       map[string]string{"uptime": "ok", "cpu": "ok", "memory": "ok"},
				NetworkReachable:  "true",
				SNMPReachable:     "true",
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
	if got := metric["primary_health"]; got != "up_fresh" {
		t.Fatalf("primary_health = %#v, want up_fresh", got)
	}
	if _, ok := metric["runtime_flags"].([]any); !ok {
		t.Fatalf("runtime_flags = %#v, want array", metric["runtime_flags"])
	}
	if fields, ok := metric["field_states"].(map[string]any); !ok || fields["cpu"] != "ok" {
		t.Fatalf("field_states = %#v, want cpu=ok", metric["field_states"])
	}
	if _, ok := payload["runtime_identity"].(string); !ok {
		t.Fatalf("payload.runtime_identity = %#v, want string", payload["runtime_identity"])
	}
}

func TestNewRuntimeDeltaMessageUsesStableEnvelope(t *testing.T) {
	delta := EmptySnapshot()
	delta.Devices["dev-1"] = DeviceRuntimeDTO{DeviceID: "dev-1", PrimaryHealth: "up_fresh"}
	current := EmptySnapshot()
	current.Devices["dev-1"] = DeviceRuntimeDTO{DeviceID: "dev-1", PrimaryHealth: "up_fresh"}

	msg := NewRuntimeDeltaMessage(delta, 7, 8, current)
	raw, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	if !strings.Contains(string(raw), `"type":"runtime_delta"`) {
		t.Fatalf("message = %s, want runtime_delta", raw)
	}
	if !strings.Contains(string(raw), `"base_version":7`) || !strings.Contains(string(raw), `"version":8`) {
		t.Fatalf("message = %s, want versions", raw)
	}
	if !strings.Contains(string(raw), `"runtime_identity"`) {
		t.Fatalf("message = %s, want runtime_identity", raw)
	}
}

func TestNewTopologyChangedMessageUsesVersionedInvalidationEnvelope(t *testing.T) {
	msg := NewTopologyChangedMessage(12, "topology_dirty")
	raw, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	if !strings.Contains(string(raw), `"type":"topology_changed"`) {
		t.Fatalf("message = %s, want topology_changed", raw)
	}
	if !strings.Contains(string(raw), `"topology_version":12`) {
		t.Fatalf("message = %s, want topology_version", raw)
	}
	if !strings.Contains(string(raw), `"recommended_endpoint":"/api/v1/topology/canvas"`) {
		t.Fatalf("message = %s, want recommended endpoint", raw)
	}
}

func TestNewReadyMessageUsesVersionedHandshakeEnvelope(t *testing.T) {
	msg := NewReadyMessage(42, 7, "rt-sha256:abc")
	raw, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	if !strings.Contains(string(raw), `"type":"ready"`) {
		t.Fatalf("message = %s, want ready", raw)
	}
	if !strings.Contains(string(raw), `"runtime_version":42`) {
		t.Fatalf("message = %s, want runtime_version", raw)
	}
	if !strings.Contains(string(raw), `"alert_version":7`) {
		t.Fatalf("message = %s, want alert_version", raw)
	}
	if !strings.Contains(string(raw), `"runtime_identity":"rt-sha256:abc"`) {
		t.Fatalf("message = %s, want runtime_identity", raw)
	}
}

func TestNewPollingHealthChangedMessage(t *testing.T) {
	msg := NewPollingHealthChangedMessage(polling.HealthSnapshot{
		EssentialOverloaded: true,
		ConfiguredWorkers:   64,
		ActiveWorkers:       64,
	})
	if msg.Type != MessageTypePollingHealthChanged {
		t.Fatalf("Type = %q, want polling_health_changed", msg.Type)
	}
}

func TestNewAlertMessage_EmptyAlertsMarshalAsArray(t *testing.T) {
	message := NewAlertMessage([]AlertDTO{}, 3)

	raw, err := json.Marshal(message)
	if err != nil {
		t.Fatalf("json.Marshal returned error: %v", err)
	}

	var decoded struct {
		Payload struct {
			Version uint64 `json:"version"`
			Alerts  any    `json:"alerts"`
		} `json:"payload"`
	}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("json.Unmarshal returned error: %v", err)
	}

	alerts, ok := decoded.Payload.Alerts.([]any)
	if !ok {
		t.Fatalf("payload.alerts = %#v, want empty array", decoded.Payload.Alerts)
	}
	if len(alerts) != 0 {
		t.Fatalf("payload.alerts length = %d, want 0", len(alerts))
	}
	if decoded.Payload.Version != 3 {
		t.Fatalf("payload.version = %d, want 3", decoded.Payload.Version)
	}
}

func float64Ptr(value float64) *float64 {
	return &value
}
