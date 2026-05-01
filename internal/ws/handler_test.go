package ws

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestHandlerServeHTTP_BootstrapIncludesAlertMessage(t *testing.T) {
	hub := NewHub()
	go hub.Run()

	server := httptest.NewServer(NewHandler(
		hub,
		func() (*SnapshotPayload, uint64) {
			return EmptySnapshot(), 1
		},
		func() AlertMessagePayload {
			return AlertMessagePayload{Version: 1, Alerts: []AlertDTO{{
				DeviceID:  "device-1",
				Severity:  "critical",
				AlertName: "DeviceDown",
				State:     "firing",
				Summary:   "device down",
			}}}
		},
		func() PrometheusStatusPayload {
			return PrometheusStatusPayload{Enabled: true, Available: true}
		},
	))
	t.Cleanup(server.Close)

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("failed to dial websocket test server: %v", err)
	}
	t.Cleanup(func() {
		_ = conn.Close()
	})

	types := make([]string, 0, 3)
	for i := 0; i < 3; i++ {
		conn.SetReadDeadline(time.Now().Add(time.Second))
		_, raw, err := conn.ReadMessage()
		if err != nil {
			t.Fatalf("failed to read bootstrap websocket message %d: %v", i+1, err)
		}

		var message struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(raw, &message); err != nil {
			t.Fatalf("failed to decode bootstrap websocket message %d: %v", i+1, err)
		}
		types = append(types, message.Type)
	}

	if got, want := types, []string{MessageTypeSnapshot, MessageTypeAlert, MessageTypePrometheusStatus}; len(got) != len(want) || got[0] != want[0] || got[1] != want[1] || got[2] != want[2] {
		t.Fatalf("bootstrap message order = %v, want %v", got, want)
	}
	conn.SetReadDeadline(time.Time{})
}

func TestHandlerServeHTTP_BootstrapAlertIncludesVersionedPayload(t *testing.T) {
	hub := NewHub()
	go hub.Run()

	server := httptest.NewServer(NewHandler(
		hub,
		func() (*SnapshotPayload, uint64) {
			return EmptySnapshot(), 7
		},
		func() AlertMessagePayload {
			return AlertMessagePayload{
				Version: 9,
				Alerts: []AlertDTO{{
					DeviceID:  "device-1",
					Severity:  "critical",
					AlertName: "DeviceDown",
					State:     "firing",
					Summary:   "device down",
				}},
			}
		},
		func() PrometheusStatusPayload {
			return PrometheusStatusPayload{}
		},
	))
	t.Cleanup(server.Close)

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("failed to dial websocket test server: %v", err)
	}
	t.Cleanup(func() {
		_ = conn.Close()
	})

	for i := 0; i < 2; i++ {
		conn.SetReadDeadline(time.Now().Add(time.Second))
		_, raw, err := conn.ReadMessage()
		if err != nil {
			t.Fatalf("failed to read bootstrap websocket message %d: %v", i+1, err)
		}

		var message struct {
			Type    string          `json:"type"`
			Payload json.RawMessage `json:"payload"`
		}
		if err := json.Unmarshal(raw, &message); err != nil {
			t.Fatalf("failed to decode bootstrap websocket message %d: %v", i+1, err)
		}

		if message.Type != MessageTypeAlert {
			continue
		}

		var payload struct {
			Version uint64     `json:"version"`
			Alerts  []AlertDTO `json:"alerts"`
		}
		if err := json.Unmarshal(message.Payload, &payload); err != nil {
			t.Fatalf("failed to decode alert bootstrap payload: %v", err)
		}

		if payload.Version != 9 {
			t.Fatalf("alert bootstrap version = %d, want 9", payload.Version)
		}
		if len(payload.Alerts) != 1 || payload.Alerts[0].DeviceID != "device-1" {
			t.Fatalf("alert bootstrap alerts = %#v, want single device-1 alert", payload.Alerts)
		}
		return
	}

	t.Fatal("did not receive bootstrap alert message")
}

func TestHandlerServeHTTP_HelloWithCurrentRuntimeVersionSkipsBootstrapSnapshot(t *testing.T) {
	hub := NewHub()
	go hub.Run()

	server := httptest.NewServer(NewHandler(
		hub,
		func() (*SnapshotPayload, uint64) {
			return EmptySnapshot(), 42
		},
		func() AlertMessagePayload {
			return AlertMessagePayload{Version: 7, Alerts: []AlertDTO{}}
		},
		func() PrometheusStatusPayload {
			return PrometheusStatusPayload{Enabled: true, Available: true}
		},
	))
	t.Cleanup(server.Close)

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("failed to dial websocket test server: %v", err)
	}
	t.Cleanup(func() {
		_ = conn.Close()
	})

	if err := conn.WriteJSON(map[string]any{
		"type": "hello",
		"payload": map[string]any{
			"canvas_schema_version": 1,
			"topology_version":      "topo-123",
			"runtime_version":       42,
			"alert_version":         7,
		},
	}); err != nil {
		t.Fatalf("write hello: %v", err)
	}

	types := make([]string, 0, 3)
	for i := 0; i < 3; i++ {
		conn.SetReadDeadline(time.Now().Add(time.Second))
		_, raw, err := conn.ReadMessage()
		if err != nil {
			t.Fatalf("failed to read bootstrap websocket message %d: %v", i+1, err)
		}

		var message struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(raw, &message); err != nil {
			t.Fatalf("failed to decode bootstrap websocket message %d: %v", i+1, err)
		}
		types = append(types, message.Type)
	}

	if got, want := types, []string{MessageTypeReady, MessageTypeAlert, MessageTypePrometheusStatus}; len(got) != len(want) || got[0] != want[0] || got[1] != want[1] || got[2] != want[2] {
		t.Fatalf("bootstrap message order = %v, want %v", got, want)
	}
	conn.SetReadDeadline(time.Time{})
}

func TestHandlerServeHTTP_HelloWithCurrentRuntimeIdentitySkipsBootstrapSnapshot(t *testing.T) {
	hub := NewHub()
	go hub.Run()
	snapshot := EmptySnapshot()
	snapshot.Devices["dev-1"] = DeviceRuntimeDTO{DeviceID: "dev-1", PrimaryHealth: "up_fresh"}

	server := httptest.NewServer(NewHandler(
		hub,
		func() (*SnapshotPayload, uint64) {
			return snapshot, 1
		},
		func() AlertMessagePayload {
			return AlertMessagePayload{Version: 7, Alerts: []AlertDTO{}}
		},
		func() PrometheusStatusPayload {
			return PrometheusStatusPayload{}
		},
	))
	t.Cleanup(server.Close)

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("failed to dial websocket test server: %v", err)
	}
	t.Cleanup(func() {
		_ = conn.Close()
	})

	if err := conn.WriteJSON(map[string]any{
		"type": "hello",
		"payload": map[string]any{
			"canvas_schema_version": 1,
			"runtime_version":       42,
			"runtime_identity":      RuntimeIdentityForSnapshot(snapshot),
		},
	}); err != nil {
		t.Fatalf("write hello: %v", err)
	}

	conn.SetReadDeadline(time.Now().Add(time.Second))
	_, raw, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("failed to read bootstrap websocket message: %v", err)
	}
	var message struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(raw, &message); err != nil {
		t.Fatalf("failed to decode bootstrap websocket message: %v", err)
	}
	if message.Type != MessageTypeReady {
		t.Fatalf("first bootstrap message = %q, want ready", message.Type)
	}
	conn.SetReadDeadline(time.Time{})
}

func TestHandlerServeHTTP_DelayedHelloWithCurrentRuntimeIdentitySkipsBootstrapSnapshot(t *testing.T) {
	hub := NewHub()
	go hub.Run()
	snapshot := EmptySnapshot()
	snapshot.Devices["dev-1"] = DeviceRuntimeDTO{DeviceID: "dev-1", PrimaryHealth: "up_fresh"}

	server := httptest.NewServer(NewHandler(
		hub,
		func() (*SnapshotPayload, uint64) {
			return snapshot, 1
		},
		func() AlertMessagePayload {
			return AlertMessagePayload{Version: 7, Alerts: []AlertDTO{}}
		},
		func() PrometheusStatusPayload {
			return PrometheusStatusPayload{}
		},
	))
	t.Cleanup(server.Close)

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("failed to dial websocket test server: %v", err)
	}
	t.Cleanup(func() {
		_ = conn.Close()
	})

	time.Sleep(150 * time.Millisecond)
	if err := conn.WriteJSON(map[string]any{
		"type": "hello",
		"payload": map[string]any{
			"canvas_schema_version": 1,
			"runtime_version":       1,
			"runtime_identity":      RuntimeIdentityForSnapshot(snapshot),
		},
	}); err != nil {
		t.Fatalf("write hello: %v", err)
	}

	conn.SetReadDeadline(time.Now().Add(time.Second))
	_, raw, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("failed to read bootstrap websocket message: %v", err)
	}
	var message struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(raw, &message); err != nil {
		t.Fatalf("failed to decode bootstrap websocket message: %v", err)
	}
	if message.Type != MessageTypeReady {
		t.Fatalf("first bootstrap message = %q, want ready", message.Type)
	}
	conn.SetReadDeadline(time.Time{})
}

func TestHandlerServeHTTP_DoesNotReceiveBroadcastBeforeBootstrapReady(t *testing.T) {
	hub := NewHub()
	go hub.Run()
	snapshot := EmptySnapshot()
	snapshot.Devices["dev-1"] = DeviceRuntimeDTO{DeviceID: "dev-1", PrimaryHealth: "up_fresh"}

	server := httptest.NewServer(NewHandler(
		hub,
		func() (*SnapshotPayload, uint64) {
			return snapshot, 1
		},
		func() AlertMessagePayload {
			return AlertMessagePayload{Version: 7, Alerts: []AlertDTO{}}
		},
		func() PrometheusStatusPayload {
			return PrometheusStatusPayload{}
		},
	))
	t.Cleanup(server.Close)

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("failed to dial websocket test server: %v", err)
	}
	t.Cleanup(func() {
		_ = conn.Close()
	})

	time.Sleep(50 * time.Millisecond)
	hub.BroadcastOverviewDelta(EmptySnapshot(), 1, 2, snapshot)
	time.Sleep(100 * time.Millisecond)
	if err := conn.WriteJSON(map[string]any{
		"type": "hello",
		"payload": map[string]any{
			"canvas_schema_version": 1,
			"runtime_version":       1,
			"runtime_identity":      RuntimeIdentityForSnapshot(snapshot),
		},
	}); err != nil {
		t.Fatalf("write hello: %v", err)
	}

	conn.SetReadDeadline(time.Now().Add(time.Second))
	_, raw, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("failed to read bootstrap websocket message: %v", err)
	}
	var message struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(raw, &message); err != nil {
		t.Fatalf("failed to decode bootstrap websocket message: %v", err)
	}
	if message.Type != MessageTypeReady {
		t.Fatalf("first bootstrap message = %q, want ready", message.Type)
	}
	conn.SetReadDeadline(time.Time{})
}

func TestHandlerServeHTTP_StaleHelloRequestsHTTPResyncInsteadOfSnapshot(t *testing.T) {
	hub := NewHub()
	go hub.Run()
	snapshot := EmptySnapshot()
	snapshot.Devices["dev-1"] = DeviceRuntimeDTO{DeviceID: "dev-1", PrimaryHealth: "up_fresh"}

	server := httptest.NewServer(NewHandler(
		hub,
		func() (*SnapshotPayload, uint64) {
			return snapshot, 2
		},
		func() AlertMessagePayload {
			return AlertMessagePayload{Version: 7, Alerts: []AlertDTO{}}
		},
		func() PrometheusStatusPayload {
			return PrometheusStatusPayload{}
		},
	))
	t.Cleanup(server.Close)

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("failed to dial websocket test server: %v", err)
	}
	t.Cleanup(func() {
		_ = conn.Close()
	})

	if err := conn.WriteJSON(map[string]any{
		"type": "hello",
		"payload": map[string]any{
			"canvas_schema_version": 1,
			"runtime_version":       1,
			"runtime_identity":      "rt-sha256:stale",
		},
	}); err != nil {
		t.Fatalf("write hello: %v", err)
	}

	conn.SetReadDeadline(time.Now().Add(time.Second))
	_, raw, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("failed to read bootstrap websocket message: %v", err)
	}
	var message struct {
		Type    string                `json:"type"`
		Payload ResyncRequiredPayload `json:"payload"`
	}
	if err := json.Unmarshal(raw, &message); err != nil {
		t.Fatalf("failed to decode bootstrap websocket message: %v", err)
	}
	if message.Type != MessageTypeResyncRequired {
		t.Fatalf("first bootstrap message = %q, want resync_required", message.Type)
	}
	if message.Payload.Scope != ResyncScopeOverview {
		t.Fatalf("resync scope = %q, want %q", message.Payload.Scope, ResyncScopeOverview)
	}
	if message.Payload.Reason != ResyncReasonClientMissingRuntimeSnapshot {
		t.Fatalf("resync reason = %q, want %q", message.Payload.Reason, ResyncReasonClientMissingRuntimeSnapshot)
	}
	conn.SetReadDeadline(time.Time{})
}
