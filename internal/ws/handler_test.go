package ws

import (
	"bytes"
	"encoding/json"
	"log"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/lollinoo/theia/internal/logging"
)

func captureDebugLogs(t *testing.T) *bytes.Buffer {
	t.Helper()

	var buf bytes.Buffer
	originalWriter := log.Writer()
	originalFlags := log.Flags()
	log.SetOutput(&buf)
	log.SetFlags(0)
	logging.Configure("debug")
	t.Cleanup(func() {
		logging.Configure("info")
		log.SetOutput(originalWriter)
		log.SetFlags(originalFlags)
	})
	return &buf
}

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

func TestHandlerServeHTTP_QueryHelloSkipsBootstrapSnapshotWhenClientHelloMessageIsDelayed(t *testing.T) {
	hub := NewHub()
	go hub.Run()
	snapshot := EmptySnapshot()
	snapshot.Devices["dev-1"] = DeviceRuntimeDTO{DeviceID: "dev-1", PrimaryHealth: "up_fresh"}
	runtimeIdentity := RuntimeIdentityForSnapshot(snapshot)

	server := httptest.NewServer(NewHandler(
		hub,
		func() (*SnapshotPayload, uint64) {
			return snapshot, 42
		},
		func() AlertMessagePayload {
			return AlertMessagePayload{Version: 7, Alerts: []AlertDTO{}}
		},
		func() PrometheusStatusPayload {
			return PrometheusStatusPayload{}
		},
	))
	t.Cleanup(server.Close)

	params := url.Values{}
	params.Set("canvas_schema_version", "1")
	params.Set("runtime_version", "42")
	params.Set("runtime_identity", runtimeIdentity)
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "?" + params.Encode()
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("failed to dial websocket test server: %v", err)
	}
	t.Cleanup(func() {
		_ = conn.Close()
	})

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
		t.Fatalf("bootstrap message type = %q, want %q", message.Type, MessageTypeReady)
	}
	conn.SetReadDeadline(time.Time{})
}

func TestHandlerServeHTTP_DebugLogsBootstrapDecision(t *testing.T) {
	logs := captureDebugLogs(t)
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
			"alert_version":         7,
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
		t.Fatalf("bootstrap message type = %q, want %q", message.Type, MessageTypeReady)
	}

	output := logs.String()
	if !strings.Contains(output, "DEBUG websocket bootstrap decision=ready") {
		t.Fatalf("debug output missing bootstrap decision: %q", output)
	}
	if !strings.Contains(output, "hello_runtime_version=42") {
		t.Fatalf("debug output missing hello runtime version: %q", output)
	}
}

func TestHandlerServeHTTP_DebugLogsHelloTimeout(t *testing.T) {
	logs := captureDebugLogs(t)
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
	if message.Type != MessageTypeSnapshot {
		t.Fatalf("bootstrap message type = %q, want %q", message.Type, MessageTypeSnapshot)
	}

	output := logs.String()
	if !strings.Contains(output, "hello_timeout=true") {
		t.Fatalf("debug output missing hello timeout: %q", output)
	}
	if !strings.Contains(output, "hello_wait_ms=500") {
		t.Fatalf("debug output missing hello wait: %q", output)
	}
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
	hub.BroadcastOverviewDelta(EmptyRuntimeDeltaPayload(), 1, 2, snapshot)
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

func TestHandlerServeHTTP_HelloCurrentAtConnectSkipsSnapshotWhenDeltaQueuedDuringHelloWait(t *testing.T) {
	hub := NewHub()
	go hub.Run()

	initialSnapshot := EmptySnapshot()
	initialSnapshot.Devices["dev-1"] = DeviceRuntimeDTO{DeviceID: "dev-1", PrimaryHealth: "up_fresh"}
	updatedSnapshot := CloneSnapshot(initialSnapshot)
	updatedSnapshot.Devices["dev-1"] = DeviceRuntimeDTO{DeviceID: "dev-1", PrimaryHealth: "down"}
	initialIdentity := RuntimeIdentityForSnapshot(initialSnapshot)

	var snapshotMu sync.Mutex
	currentSnapshot := initialSnapshot
	currentVersion := uint64(1)

	server := httptest.NewServer(NewHandler(
		hub,
		func() (*SnapshotPayload, uint64) {
			snapshotMu.Lock()
			defer snapshotMu.Unlock()
			return currentSnapshot, currentVersion
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
	snapshotMu.Lock()
	currentSnapshot = updatedSnapshot
	currentVersion = 2
	snapshotMu.Unlock()
	hub.BroadcastOverviewDelta(&RuntimeDeltaPayload{
		Devices: map[string]map[string]any{
			"dev-1": {
				"primary_health": "down",
			},
		},
		Links: map[string]map[string]any{},
	}, 1, 2, updatedSnapshot)
	time.Sleep(50 * time.Millisecond)

	if err := conn.WriteJSON(map[string]any{
		"type": "hello",
		"payload": map[string]any{
			"canvas_schema_version": 1,
			"runtime_version":       1,
			"runtime_identity":      initialIdentity,
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
