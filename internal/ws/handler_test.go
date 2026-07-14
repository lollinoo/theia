package ws

// This file exercises handler behavior so refactors preserve the documented contract.

import (
	"bytes"
	"encoding/json"
	"log"
	"math"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/lollinoo/theia/internal/logging"
	"github.com/lollinoo/theia/internal/observability"
)

type synchronizedLogBuffer struct {
	mu     sync.Mutex
	buffer bytes.Buffer
}

func (b *synchronizedLogBuffer) Write(payload []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buffer.Write(payload)
}

func (b *synchronizedLogBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buffer.String()
}

func captureDebugLogs(t *testing.T) *synchronizedLogBuffer {
	t.Helper()

	var buf synchronizedLogBuffer
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

func TestClientHelloFromRequest_ParsesRuntimeProtocolCursor(t *testing.T) {
	params := url.Values{}
	params.Set("runtime_protocol", "2")
	params.Set("runtime_stream_id", "runtime-stream-1")
	params.Set("runtime_version", "42")
	req := httptest.NewRequest(http.MethodGet, "/ws?"+params.Encode(), nil)

	hello, ok := clientHelloFromRequest(req)
	if !ok {
		t.Fatal("clientHelloFromRequest returned no hello")
	}
	if hello.RuntimeProtocol != RuntimeStreamProtocolVersion {
		t.Fatalf("RuntimeProtocol = %d, want %d", hello.RuntimeProtocol, RuntimeStreamProtocolVersion)
	}
	wantCursor := RuntimeCursor{StreamID: "runtime-stream-1", Version: 42, Known: true}
	if hello.RuntimeCursor != wantCursor {
		t.Fatalf("RuntimeCursor = %#v, want %#v", hello.RuntimeCursor, wantCursor)
	}
}

func TestClientHelloFromRequest_RecognizesRuntimeProtocolOnlyHello(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/ws?runtime_protocol=2", nil)

	hello, ok := clientHelloFromRequest(req)
	if !ok {
		t.Fatal("clientHelloFromRequest returned no hello")
	}
	if hello.RuntimeProtocol != RuntimeStreamProtocolVersion {
		t.Fatalf("RuntimeProtocol = %d, want %d", hello.RuntimeProtocol, RuntimeStreamProtocolVersion)
	}
	if hello.RuntimeCursor.Known {
		t.Fatalf("RuntimeCursor = %#v, want unknown cursor", hello.RuntimeCursor)
	}
}

func TestClientHelloFromRequest_PreservesLegacyHello(t *testing.T) {
	params := url.Values{}
	params.Set("canvas_schema_version", "1")
	params.Set("runtime_identity", "rt-sha256:abc")
	params.Set("runtime_version", "42")
	req := httptest.NewRequest(http.MethodGet, "/ws?"+params.Encode(), nil)

	hello, ok := clientHelloFromRequest(req)
	if !ok {
		t.Fatal("clientHelloFromRequest returned no hello")
	}
	if hello.RuntimeProtocol != 0 {
		t.Fatalf("RuntimeProtocol = %d, want legacy default 0", hello.RuntimeProtocol)
	}
	if hello.RuntimeCursor.Known {
		t.Fatalf("RuntimeCursor = %#v, want unknown legacy cursor", hello.RuntimeCursor)
	}
	if hello.RuntimeVersion == nil || *hello.RuntimeVersion != 42 {
		t.Fatalf("RuntimeVersion = %#v, want 42", hello.RuntimeVersion)
	}
	if hello.RuntimeIdentity != "rt-sha256:abc" {
		t.Fatalf("RuntimeIdentity = %q, want rt-sha256:abc", hello.RuntimeIdentity)
	}
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

func TestHandlerServeHTTP_RejectsUnlistedOrigin(t *testing.T) {
	hub := NewHub()
	go hub.Run()

	server := httptest.NewServer(NewHandler(
		hub,
		func() (*SnapshotPayload, uint64) {
			return EmptySnapshot(), 1
		},
		nil,
		nil,
		WithAllowedOrigins([]string{"https://ops.example"}),
	))
	t.Cleanup(server.Close)

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	headers := make(http.Header)
	headers.Set("Origin", "https://evil.example")
	conn, resp, err := websocket.DefaultDialer.Dial(wsURL, headers)
	if err == nil {
		_ = conn.Close()
		t.Fatal("expected websocket dial to fail for unlisted origin")
	}
	if resp == nil || resp.StatusCode != http.StatusForbidden {
		if resp == nil {
			t.Fatal("expected 403 response for rejected origin, got nil response")
		}
		t.Fatalf("status = %d, want 403", resp.StatusCode)
	}
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

func TestHandlerServeHTTP_QueryHelloStaleRepairBroadcastSkipsBootstrappingClient(t *testing.T) {
	hub := NewHub()
	go hub.Run()
	snapshot := EmptySnapshot()
	snapshot.Devices["dev-1"] = DeviceRuntimeDTO{DeviceID: "dev-1", PrimaryHealth: "up_fresh"}
	runtimeIdentity := RuntimeIdentityForSnapshot(snapshot)
	var snapshotCalls int

	server := httptest.NewServer(NewHandler(
		hub,
		func() (*SnapshotPayload, uint64) {
			snapshotCalls++
			if snapshotCalls == 1 {
				hub.BroadcastOverviewSnapshot(snapshot, 42)
			}
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

	for i, want := range []string{MessageTypeReady, MessageTypeAlert} {
		conn.SetReadDeadline(time.Now().Add(time.Second))
		_, raw, err := conn.ReadMessage()
		if err != nil {
			t.Fatalf("failed to read websocket message %d: %v", i+1, err)
		}
		var message struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(raw, &message); err != nil {
			t.Fatalf("failed to decode websocket message %d: %v", i+1, err)
		}
		if message.Type != want {
			t.Fatalf("websocket message %d type = %q, want %q", i+1, message.Type, want)
		}
	}

	conn.SetReadDeadline(time.Now().Add(150 * time.Millisecond))
	_, raw, err := conn.ReadMessage()
	if err == nil {
		var message struct {
			Type string `json:"type"`
		}
		_ = json.Unmarshal(raw, &message)
		t.Fatalf("unexpected post-bootstrap overview message type=%q raw=%s", message.Type, string(raw))
	}
	conn.SetReadDeadline(time.Time{})
}

func TestHandlerServeHTTP_QueryHelloQueuesOverviewBroadcastAfterSnapshotCallback(t *testing.T) {
	hub := NewHub()
	go hub.Run()
	snapshot := EmptySnapshot()
	snapshot.Devices["dev-1"] = DeviceRuntimeDTO{DeviceID: "dev-1", PrimaryHealth: "up_fresh"}
	runtimeIdentity := RuntimeIdentityForSnapshot(snapshot)
	nextSnapshot := CloneSnapshot(snapshot)
	nextSnapshot.Devices["dev-1"] = DeviceRuntimeDTO{DeviceID: "dev-1", PrimaryHealth: "up_degraded"}
	broadcastQueued := make(chan struct{})
	releaseAlerts := make(chan struct{})
	var releaseOnce sync.Once

	server := httptest.NewServer(NewHandler(
		hub,
		func() (*SnapshotPayload, uint64) {
			return snapshot, 42
		},
		func() AlertMessagePayload {
			hub.BroadcastOverviewSnapshot(nextSnapshot, 43)
			close(broadcastQueued)
			<-releaseAlerts
			return AlertMessagePayload{Version: 7, Alerts: []AlertDTO{}}
		},
		func() PrometheusStatusPayload {
			return PrometheusStatusPayload{}
		},
	))
	t.Cleanup(func() {
		releaseOnce.Do(func() {
			close(releaseAlerts)
		})
		server.Close()
	})

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

	select {
	case <-broadcastQueued:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for post-snapshot overview broadcast")
	}
	if err := conn.WriteJSON(map[string]any{
		"type": "hello",
		"payload": map[string]any{
			"canvas_schema_version": 1,
			"runtime_version":       42,
			"runtime_identity":      runtimeIdentity,
		},
	}); err != nil {
		t.Fatalf("write duplicate hello: %v", err)
	}
	releaseOnce.Do(func() {
		close(releaseAlerts)
	})

	for i, want := range []string{MessageTypeReady, MessageTypeAlert, MessageTypeResyncRequired} {
		conn.SetReadDeadline(time.Now().Add(time.Second))
		_, raw, err := conn.ReadMessage()
		if err != nil {
			t.Fatalf("failed to read websocket message %d: %v", i+1, err)
		}
		var message struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(raw, &message); err != nil {
			t.Fatalf("failed to decode websocket message %d: %v", i+1, err)
		}
		if message.Type != want {
			t.Fatalf("websocket message %d type = %q, want %q", i+1, message.Type, want)
		}
	}
	conn.SetReadDeadline(time.Time{})
}

func TestHandlerServeHTTP_QueryHelloVersionOnlyDuplicatePreservesQueuedCatchUp(t *testing.T) {
	hub := NewHub()
	go hub.Run()
	snapshot := EmptySnapshot()
	snapshot.Devices["dev-1"] = DeviceRuntimeDTO{DeviceID: "dev-1", PrimaryHealth: "up_fresh"}
	nextSnapshot := CloneSnapshot(snapshot)
	nextSnapshot.Devices["dev-1"] = DeviceRuntimeDTO{DeviceID: "dev-1", PrimaryHealth: "up_degraded"}
	broadcastQueued := make(chan struct{})
	releaseAlerts := make(chan struct{})
	var releaseOnce sync.Once

	server := httptest.NewServer(NewHandler(
		hub,
		func() (*SnapshotPayload, uint64) {
			return snapshot, 42
		},
		func() AlertMessagePayload {
			hub.BroadcastOverviewSnapshot(nextSnapshot, 43)
			close(broadcastQueued)
			<-releaseAlerts
			return AlertMessagePayload{Version: 7, Alerts: []AlertDTO{}}
		},
		func() PrometheusStatusPayload {
			return PrometheusStatusPayload{}
		},
	))
	t.Cleanup(func() {
		releaseOnce.Do(func() {
			close(releaseAlerts)
		})
		server.Close()
	})

	params := url.Values{}
	params.Set("canvas_schema_version", "1")
	params.Set("runtime_version", "42")
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "?" + params.Encode()
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("failed to dial websocket test server: %v", err)
	}
	t.Cleanup(func() {
		_ = conn.Close()
	})

	select {
	case <-broadcastQueued:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for post-snapshot overview broadcast")
	}
	if err := conn.WriteJSON(map[string]any{
		"type": "hello",
		"payload": map[string]any{
			"canvas_schema_version": 1,
			"runtime_version":       42,
			"runtime_identity":      "rt-sha256:stale",
		},
	}); err != nil {
		t.Fatalf("write duplicate hello: %v", err)
	}
	releaseOnce.Do(func() {
		close(releaseAlerts)
	})

	for i, want := range []string{MessageTypeReady, MessageTypeAlert, MessageTypeResyncRequired} {
		conn.SetReadDeadline(time.Now().Add(time.Second))
		_, raw, err := conn.ReadMessage()
		if err != nil {
			t.Fatalf("failed to read websocket message %d: %v", i+1, err)
		}
		var message struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(raw, &message); err != nil {
			t.Fatalf("failed to decode websocket message %d: %v", i+1, err)
		}
		if message.Type != want {
			t.Fatalf("websocket message %d type = %q, want %q", i+1, message.Type, want)
		}
	}
	conn.SetReadDeadline(time.Time{})
}

func TestHandlerServeHTTP_QueryHelloRequestsResyncWhenRuntimeChangesAfterSnapshotSelection(t *testing.T) {
	hub := NewHub()
	go hub.Run()
	initialSnapshot := EmptySnapshot()
	initialSnapshot.Devices["dev-1"] = DeviceRuntimeDTO{DeviceID: "dev-1", PrimaryHealth: "up_fresh"}
	initialIdentity := RuntimeIdentityForSnapshot(initialSnapshot)
	nextSnapshot := EmptySnapshot()
	nextSnapshot.Devices["dev-1"] = DeviceRuntimeDTO{DeviceID: "dev-1", PrimaryHealth: "up_degraded"}

	var mu sync.Mutex
	currentSnapshot := initialSnapshot
	currentVersion := uint64(42)

	server := httptest.NewServer(NewHandler(
		hub,
		func() (*SnapshotPayload, uint64) {
			mu.Lock()
			defer mu.Unlock()
			return currentSnapshot, currentVersion
		},
		func() AlertMessagePayload {
			mu.Lock()
			currentSnapshot = nextSnapshot
			currentVersion = 43
			mu.Unlock()
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
	params.Set("runtime_identity", initialIdentity)
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "?" + params.Encode()
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("failed to dial websocket test server: %v", err)
	}
	t.Cleanup(func() {
		_ = conn.Close()
	})

	for i, want := range []string{MessageTypeReady, MessageTypeAlert, MessageTypeResyncRequired} {
		conn.SetReadDeadline(time.Now().Add(time.Second))
		_, raw, err := conn.ReadMessage()
		if err != nil {
			t.Fatalf("failed to read websocket message %d: %v", i+1, err)
		}
		var message struct {
			Type    string                `json:"type"`
			Payload ResyncRequiredPayload `json:"payload"`
		}
		if err := json.Unmarshal(raw, &message); err != nil {
			t.Fatalf("failed to decode websocket message %d: %v", i+1, err)
		}
		if message.Type != want {
			t.Fatalf("websocket message %d type = %q, want %q", i+1, message.Type, want)
		}
		if i == 2 && message.Payload.Reason != ResyncReasonClientResync {
			t.Fatalf("catch-up resync reason = %q, want %q", message.Payload.Reason, ResyncReasonClientResync)
		}
	}
	conn.SetReadDeadline(time.Time{})
}

func TestHandlerServeHTTP_LegacyBootstrapDoesNotDuplicateBroadcastCatchUp(t *testing.T) {
	hub := NewHub()
	go hub.Run()
	initialSnapshot := EmptySnapshot()
	initialSnapshot.Devices["dev-1"] = DeviceRuntimeDTO{DeviceID: "dev-1", PrimaryHealth: "up_fresh"}
	nextSnapshot := CloneSnapshot(initialSnapshot)
	nextSnapshot.Devices["dev-1"] = DeviceRuntimeDTO{DeviceID: "dev-1", PrimaryHealth: "up_degraded"}

	var mu sync.Mutex
	currentSnapshot := initialSnapshot
	currentVersion := uint64(42)

	server := httptest.NewServer(NewHandler(
		hub,
		func() (*SnapshotPayload, uint64) {
			mu.Lock()
			defer mu.Unlock()
			return currentSnapshot, currentVersion
		},
		func() AlertMessagePayload {
			mu.Lock()
			currentSnapshot = nextSnapshot
			currentVersion = 43
			mu.Unlock()
			hub.BroadcastOverviewSnapshot(nextSnapshot, 43)
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

	for i, want := range []string{MessageTypeSnapshot, MessageTypeAlert, MessageTypeSnapshot} {
		conn.SetReadDeadline(time.Now().Add(time.Second))
		_, raw, err := conn.ReadMessage()
		if err != nil {
			t.Fatalf("failed to read websocket message %d: %v", i+1, err)
		}
		var message struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(raw, &message); err != nil {
			t.Fatalf("failed to decode websocket message %d: %v", i+1, err)
		}
		if message.Type != want {
			t.Fatalf("websocket message %d type = %q, want %q", i+1, message.Type, want)
		}
	}

	conn.SetReadDeadline(time.Now().Add(150 * time.Millisecond))
	_, raw, err := conn.ReadMessage()
	if err == nil {
		var message struct {
			Type string `json:"type"`
		}
		_ = json.Unmarshal(raw, &message)
		t.Fatalf("unexpected duplicate catch-up message type=%q raw=%s", message.Type, string(raw))
	}
	conn.SetReadDeadline(time.Time{})
}

func TestHandlerServeHTTP_StaleQueryHelloRecordsHTTPBootstrapResyncMetric(t *testing.T) {
	registry := observability.ResetDefaultForTest()
	t.Cleanup(func() {
		observability.ResetDefaultForTest()
	})

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

	params := url.Values{}
	params.Set("canvas_schema_version", "1")
	params.Set("runtime_version", "1")
	params.Set("runtime_identity", "rt-sha256:stale")
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

	conn.SetReadDeadline(time.Now().Add(time.Second))
	_, raw, err = conn.ReadMessage()
	if err != nil {
		t.Fatalf("failed to read alert bootstrap websocket message: %v", err)
	}
	var alertMessage struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(raw, &alertMessage); err != nil {
		t.Fatalf("failed to decode alert bootstrap websocket message: %v", err)
	}
	if alertMessage.Type != MessageTypeAlert {
		t.Fatalf("second bootstrap message = %q, want alert", alertMessage.Type)
	}

	metrics := string(registry.MarshalPrometheus())
	if !strings.Contains(metrics, `theia_ws_client_resync_required_total{bootstrap="http",reason="client_missing_runtime_snapshot",scope="overview"} 1`) {
		t.Fatalf("expected HTTP bootstrap resync metric, got:\n%s", metrics)
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

func TestHandlerServeHTTP_StaleHelloMarksClientAwaitingHTTPResync(t *testing.T) {
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
		Type string `json:"type"`
	}
	if err := json.Unmarshal(raw, &message); err != nil {
		t.Fatalf("failed to decode bootstrap websocket message: %v", err)
	}
	if message.Type != MessageTypeResyncRequired {
		t.Fatalf("first bootstrap message = %q, want resync_required", message.Type)
	}

	clients := hub.copyClients()
	if len(clients) != 1 {
		t.Fatalf("connected clients = %d, want 1", len(clients))
	}
	clients[0].mu.Lock()
	needsResync := clients[0].needsResync
	clients[0].mu.Unlock()
	if !needsResync {
		t.Fatal("expected stale hello client to be marked as awaiting HTTP resync")
	}
	conn.SetReadDeadline(time.Time{})
}

func TestHandlerRuntimeV2QueryCursorInvokesSyncBeforeBroadcastReady(t *testing.T) {
	hub := NewHub()
	go hub.Run()

	type syncEvent struct {
		request        RuntimeSyncRequest
		broadcastReady bool
		protocol       int
		cursor         RuntimeCursor
	}
	events := make(chan syncEvent, 2)
	server := httptest.NewServer(NewHandler(
		hub,
		func() (*SnapshotPayload, uint64) { return EmptySnapshot(), 42 },
		func() AlertMessagePayload { return AlertMessagePayload{} },
		nil,
		WithRuntimeSync(func(client *Client, request RuntimeSyncRequest) {
			events <- syncEvent{
				request:        request,
				broadcastReady: client.canReceiveOverviewBroadcast(),
				protocol:       client.RuntimeProtocol(),
				cursor:         client.AckedRuntimeCursor(),
			}
		}),
	))
	t.Cleanup(server.Close)

	params := url.Values{}
	params.Set("runtime_protocol", "2")
	params.Set("runtime_stream_id", "runtime-stream-1")
	params.Set("runtime_version", "41")
	conn, _, err := websocket.DefaultDialer.Dial(
		"ws"+strings.TrimPrefix(server.URL, "http")+"?"+params.Encode(),
		nil,
	)
	if err != nil {
		t.Fatalf("dial protocol-v2 client: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	wantCursor := RuntimeCursor{StreamID: "runtime-stream-1", Version: 41, Known: true}
	select {
	case event := <-events:
		if event.request.Cursor != wantCursor {
			t.Fatalf("sync request cursor = %#v, want %#v", event.request.Cursor, wantCursor)
		}
		if event.broadcastReady {
			t.Fatal("runtime sync callback ran after overview broadcasts became eligible")
		}
		if event.protocol != RuntimeStreamProtocolVersion || event.cursor != wantCursor {
			t.Fatalf("client runtime state = protocol %d cursor %#v, want protocol %d cursor %#v", event.protocol, event.cursor, RuntimeStreamProtocolVersion, wantCursor)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for protocol-v2 bootstrap sync callback")
	}

	select {
	case event := <-events:
		t.Fatalf("protocol-v2 query invoked duplicate sync callback: %#v", event.request)
	case <-time.After(100 * time.Millisecond):
	}
}

func TestHandlerResumeRuntimeInvokesSyncAfterBootstrap(t *testing.T) {
	hub := NewHub()
	go hub.Run()

	events := make(chan RuntimeSyncRequest, 2)
	server := httptest.NewServer(NewHandler(
		hub,
		func() (*SnapshotPayload, uint64) { return EmptySnapshot(), 42 },
		func() AlertMessagePayload { return AlertMessagePayload{} },
		nil,
		WithRuntimeSync(func(client *Client, request RuntimeSyncRequest) {
			_ = client.AckedRuntimeCursor()
			events <- request
		}),
	))
	t.Cleanup(server.Close)

	params := url.Values{}
	params.Set("runtime_protocol", "2")
	params.Set("runtime_stream_id", "runtime-stream-1")
	params.Set("runtime_version", "41")
	conn, _, err := websocket.DefaultDialer.Dial(
		"ws"+strings.TrimPrefix(server.URL, "http")+"?"+params.Encode(),
		nil,
	)
	if err != nil {
		t.Fatalf("dial protocol-v2 client: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	select {
	case <-events:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for bootstrap sync callback")
	}
	deadline := time.Now().Add(time.Second)
	for !hub.HasOverviewClients() && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if !hub.HasOverviewClients() {
		t.Fatal("client did not become overview-broadcast ready after bootstrap sync")
	}

	if err := conn.WriteJSON(map[string]any{
		"type": MessageTypeResumeRuntime,
		"payload": map[string]any{
			"runtime_stream_id": "runtime-stream-1",
			"runtime_version":   42,
		},
	}); err != nil {
		t.Fatalf("write resume_runtime: %v", err)
	}

	want := RuntimeCursor{StreamID: "runtime-stream-1", Version: 42, Known: true}
	select {
	case request := <-events:
		if request.Cursor != want {
			t.Fatalf("resume sync cursor = %#v, want %#v", request.Cursor, want)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for resume_runtime sync callback")
	}
}

func TestHandlerAckRuntimeUpdatesOnlyMonotonicSameStreamCursor(t *testing.T) {
	hub := NewHub()
	go hub.Run()

	bootstrapSynced := make(chan struct{}, 1)
	type ackEvent struct {
		cursor       RuntimeCursor
		clientCursor RuntimeCursor
	}
	acks := make(chan ackEvent, 3)
	server := httptest.NewServer(NewHandler(
		hub,
		func() (*SnapshotPayload, uint64) { return EmptySnapshot(), 10 },
		func() AlertMessagePayload { return AlertMessagePayload{} },
		nil,
		WithRuntimeSync(func(_ *Client, _ RuntimeSyncRequest) {
			bootstrapSynced <- struct{}{}
		}),
		WithRuntimeAck(func(client *Client, cursor RuntimeCursor) {
			acks <- ackEvent{cursor: cursor, clientCursor: client.AckedRuntimeCursor()}
		}),
	))
	t.Cleanup(server.Close)

	params := url.Values{}
	params.Set("runtime_protocol", "2")
	params.Set("runtime_stream_id", "runtime-stream-1")
	params.Set("runtime_version", "5")
	conn, _, err := websocket.DefaultDialer.Dial(
		"ws"+strings.TrimPrefix(server.URL, "http")+"?"+params.Encode(),
		nil,
	)
	if err != nil {
		t.Fatalf("dial protocol-v2 client: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	select {
	case <-bootstrapSynced:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for bootstrap sync callback")
	}

	writeAck := func(streamID string, version uint64) {
		t.Helper()
		if err := conn.WriteJSON(map[string]any{
			"type": MessageTypeRuntimeAck,
			"payload": map[string]any{
				"runtime_stream_id": streamID,
				"runtime_version":   version,
			},
		}); err != nil {
			t.Fatalf("write runtime_ack %s/%d: %v", streamID, version, err)
		}
	}
	assertAck := func(want RuntimeCursor) {
		t.Helper()
		select {
		case event := <-acks:
			if event.cursor != want || event.clientCursor != want {
				t.Fatalf("ack event = cursor %#v client cursor %#v, want %#v", event.cursor, event.clientCursor, want)
			}
		case <-time.After(time.Second):
			t.Fatalf("timed out waiting for runtime ACK %#v", want)
		}
	}

	writeAck("runtime-stream-1", 6)
	assertAck(RuntimeCursor{StreamID: "runtime-stream-1", Version: 6, Known: true})
	writeAck("runtime-stream-1", 4)
	writeAck("runtime-stream-2", 7)
	select {
	case event := <-acks:
		t.Fatalf("invalid ACK invoked observer: %#v", event.cursor)
	case <-time.After(100 * time.Millisecond):
	}
	writeAck("runtime-stream-1", 7)
	assertAck(RuntimeCursor{StreamID: "runtime-stream-1", Version: 7, Known: true})
}

func TestHandlerAckRuntimeAcceptsInstalledRecoveryStreamTransition(t *testing.T) {
	hub := NewHub()
	go hub.Run()

	bootstrapSynced := make(chan bool, 1)
	acks := make(chan RuntimeCursor, 1)
	snapshot := EmptySnapshot()
	server := httptest.NewServer(NewHandler(
		hub,
		func() (*SnapshotPayload, uint64) { return snapshot, 10 },
		func() AlertMessagePayload { return AlertMessagePayload{} },
		nil,
		WithRuntimeSync(func(client *Client, _ RuntimeSyncRequest) {
			bootstrapSynced <- hub.ReplaceOverviewStream(client, OverviewSyncBatch{
				Reason:          ResyncReasonClientResync,
				Mode:            OverviewSyncModeSnapshot,
				RuntimeStreamID: "runtime-stream-2",
				TargetVersion:   10,
				RuntimeIdentity: RuntimeIdentityForSnapshot(snapshot),
				Snapshot:        snapshot,
			})
		}),
		WithRuntimeAck(func(client *Client, _ RuntimeCursor) {
			acks <- client.AckedRuntimeCursor()
		}),
	))
	t.Cleanup(server.Close)

	params := url.Values{}
	params.Set("runtime_protocol", "2")
	params.Set("runtime_stream_id", "runtime-stream-1")
	params.Set("runtime_version", "5")
	conn, _, err := websocket.DefaultDialer.Dial(
		"ws"+strings.TrimPrefix(server.URL, "http")+"?"+params.Encode(),
		nil,
	)
	if err != nil {
		t.Fatalf("dial protocol-v2 client: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	select {
	case ok := <-bootstrapSynced:
		if !ok {
			t.Fatal("failed to install rotated-stream snapshot recovery")
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for rotated-stream snapshot recovery")
	}
	if err := conn.WriteJSON(map[string]any{
		"type": MessageTypeRuntimeAck,
		"payload": map[string]any{
			"runtime_stream_id": "runtime-stream-2",
			"runtime_version":   10,
		},
	}); err != nil {
		t.Fatalf("write rotated-stream runtime ACK: %v", err)
	}

	want := RuntimeCursor{StreamID: "runtime-stream-2", Version: 10, Known: true}
	select {
	case cursor := <-acks:
		if cursor != want {
			t.Fatalf("acknowledged rotated-stream cursor = %#v, want %#v", cursor, want)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for rotated-stream runtime ACK")
	}
}

func TestHandlerAckRuntimeUnknownCursorRequiresInstalledRecoveryStream(t *testing.T) {
	hub := NewHub()
	go hub.Run()

	type syncResult struct {
		client    *Client
		installed bool
	}
	bootstrapSynced := make(chan syncResult, 1)
	acks := make(chan RuntimeCursor, 2)
	snapshot := EmptySnapshot()
	server := httptest.NewServer(NewHandler(
		hub,
		func() (*SnapshotPayload, uint64) { return snapshot, 10 },
		func() AlertMessagePayload { return AlertMessagePayload{} },
		nil,
		WithRuntimeSync(func(client *Client, _ RuntimeSyncRequest) {
			bootstrapSynced <- syncResult{
				client: client,
				installed: hub.ReplaceOverviewStream(client, OverviewSyncBatch{
					Reason:          ResyncReasonClientResync,
					Mode:            OverviewSyncModeSnapshot,
					RuntimeStreamID: "runtime-stream-A",
					TargetVersion:   10,
					RuntimeIdentity: RuntimeIdentityForSnapshot(snapshot),
					Snapshot:        snapshot,
				}),
			}
		}),
		WithRuntimeAck(func(_ *Client, cursor RuntimeCursor) {
			acks <- cursor
		}),
	))
	t.Cleanup(server.Close)

	params := url.Values{}
	params.Set("runtime_protocol", "2")
	conn, _, err := websocket.DefaultDialer.Dial(
		"ws"+strings.TrimPrefix(server.URL, "http")+"?"+params.Encode(),
		nil,
	)
	if err != nil {
		t.Fatalf("dial protocol-v2 client without cursor: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	var client *Client
	select {
	case result := <-bootstrapSynced:
		if !result.installed {
			t.Fatal("failed to install unknown-cursor snapshot recovery")
		}
		client = result.client
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for unknown-cursor snapshot recovery")
	}
	if got := client.AckedRuntimeCursor(); got.Known {
		t.Fatalf("initial acknowledged cursor = %#v, want unknown", got)
	}

	writeAck := func(streamID string, version uint64) {
		t.Helper()
		if err := conn.WriteJSON(map[string]any{
			"type": MessageTypeRuntimeAck,
			"payload": map[string]any{
				"runtime_stream_id": streamID,
				"runtime_version":   version,
			},
		}); err != nil {
			t.Fatalf("write runtime_ack %s/%d: %v", streamID, version, err)
		}
	}

	writeAck("runtime-stream-B", 1)
	select {
	case cursor := <-acks:
		t.Fatalf("arbitrary first ACK invoked observer with %#v", cursor)
	case <-time.After(100 * time.Millisecond):
	}
	client.mu.Lock()
	invalidCursor := client.ackedRuntimeCursor
	pending := client.pendingOverviewRecovery
	needsResync := client.needsResync
	client.mu.Unlock()
	if invalidCursor.Known {
		t.Fatalf("arbitrary first ACK changed cursor to %#v", invalidCursor)
	}
	if pending == nil || pending.streamID != "runtime-stream-A" || pending.targetVersion != 10 || !needsResync {
		t.Fatalf("arbitrary first ACK changed recovery state: pending=%#v needs_resync=%t", pending, needsResync)
	}

	writeAck("runtime-stream-A", 10)
	want := RuntimeCursor{StreamID: "runtime-stream-A", Version: 10, Known: true}
	select {
	case cursor := <-acks:
		if cursor != want {
			t.Fatalf("observed recovery ACK = %#v, want %#v", cursor, want)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for installed recovery-stream ACK")
	}
	client.mu.Lock()
	acceptedCursor := client.ackedRuntimeCursor
	pending = client.pendingOverviewRecovery
	needsResync = client.needsResync
	client.mu.Unlock()
	if acceptedCursor != want || pending != nil || needsResync {
		t.Fatalf("accepted recovery ACK state = cursor %#v pending %#v needs_resync %t", acceptedCursor, pending, needsResync)
	}
}

func TestHandlerAckRuntimeCompletesInstalledSameCursorRecovery(t *testing.T) {
	hub := NewHub()
	go hub.Run()

	type syncResult struct {
		client    *Client
		installed bool
	}
	bootstrapSynced := make(chan syncResult, 1)
	acks := make(chan RuntimeCursor, 2)
	snapshot := EmptySnapshot()
	server := httptest.NewServer(NewHandler(
		hub,
		func() (*SnapshotPayload, uint64) { return snapshot, 10 },
		func() AlertMessagePayload { return AlertMessagePayload{} },
		nil,
		WithRuntimeSync(func(client *Client, _ RuntimeSyncRequest) {
			bootstrapSynced <- syncResult{
				client: client,
				installed: hub.ReplaceOverviewStream(client, OverviewSyncBatch{
					Reason:          ResyncReasonClientResync,
					Mode:            OverviewSyncModeSnapshot,
					RuntimeStreamID: "runtime-stream-1",
					TargetVersion:   10,
					RuntimeIdentity: RuntimeIdentityForSnapshot(snapshot),
					Snapshot:        snapshot,
				}),
			}
		}),
		WithRuntimeAck(func(_ *Client, cursor RuntimeCursor) {
			acks <- cursor
		}),
	))
	t.Cleanup(server.Close)

	params := url.Values{}
	params.Set("runtime_protocol", "2")
	params.Set("runtime_stream_id", "runtime-stream-1")
	params.Set("runtime_version", "10")
	conn, _, err := websocket.DefaultDialer.Dial(
		"ws"+strings.TrimPrefix(server.URL, "http")+"?"+params.Encode(),
		nil,
	)
	if err != nil {
		t.Fatalf("dial same-cursor recovery client: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	var client *Client
	select {
	case result := <-bootstrapSynced:
		if !result.installed {
			t.Fatal("failed to install same-cursor snapshot recovery")
		}
		client = result.client
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for same-cursor snapshot recovery")
	}

	writeAck := func() {
		t.Helper()
		if err := conn.WriteJSON(map[string]any{
			"type": MessageTypeRuntimeAck,
			"payload": map[string]any{
				"runtime_stream_id": "runtime-stream-1",
				"runtime_version":   10,
			},
		}); err != nil {
			t.Fatalf("write same-cursor runtime ACK: %v", err)
		}
	}
	want := RuntimeCursor{StreamID: "runtime-stream-1", Version: 10, Known: true}
	writeAck()
	select {
	case cursor := <-acks:
		if cursor != want {
			t.Fatalf("observed same-cursor recovery ACK = %#v, want %#v", cursor, want)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for same-cursor recovery ACK")
	}
	client.mu.Lock()
	acceptedCursor := client.ackedRuntimeCursor
	pending := client.pendingOverviewRecovery
	needsResync := client.needsResync
	client.mu.Unlock()
	if acceptedCursor != want || pending != nil || needsResync {
		t.Fatalf("same-cursor recovery ACK state = cursor %#v pending %#v needs_resync %t", acceptedCursor, pending, needsResync)
	}

	writeAck()
	select {
	case cursor := <-acks:
		t.Fatalf("duplicate same-cursor ACK without pending recovery invoked observer with %#v", cursor)
	case <-time.After(100 * time.Millisecond):
	}
}

func TestHandlerRuntimeV2HelloRoutesRequestWithoutRegressingAckCursor(t *testing.T) {
	hub := NewHub()
	go hub.Run()

	type syncEvent struct {
		request RuntimeSyncRequest
		cursor  RuntimeCursor
	}
	events := make(chan syncEvent, 2)
	server := httptest.NewServer(NewHandler(
		hub,
		func() (*SnapshotPayload, uint64) { return EmptySnapshot(), 6 },
		func() AlertMessagePayload { return AlertMessagePayload{} },
		nil,
		WithRuntimeSync(func(client *Client, request RuntimeSyncRequest) {
			events <- syncEvent{request: request, cursor: client.AckedRuntimeCursor()}
		}),
	))
	t.Cleanup(server.Close)

	params := url.Values{}
	params.Set("runtime_protocol", "2")
	params.Set("runtime_stream_id", "runtime-stream-1")
	params.Set("runtime_version", "5")
	conn, _, err := websocket.DefaultDialer.Dial(
		"ws"+strings.TrimPrefix(server.URL, "http")+"?"+params.Encode(),
		nil,
	)
	if err != nil {
		t.Fatalf("dial protocol-v2 client: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	select {
	case <-events:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for bootstrap sync callback")
	}
	deadline := time.Now().Add(time.Second)
	for !hub.HasOverviewClients() && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if !hub.HasOverviewClients() {
		t.Fatal("client did not become overview-broadcast ready")
	}

	if err := conn.WriteJSON(map[string]any{
		"type": MessageTypeHello,
		"payload": map[string]any{
			"runtime_protocol":  RuntimeStreamProtocolVersion,
			"runtime_stream_id": "runtime-stream-1",
			"runtime_version":   3,
		},
	}); err != nil {
		t.Fatalf("write post-bootstrap hello: %v", err)
	}

	wantRequest := RuntimeCursor{StreamID: "runtime-stream-1", Version: 3, Known: true}
	wantAck := RuntimeCursor{StreamID: "runtime-stream-1", Version: 5, Known: true}
	select {
	case event := <-events:
		if event.request.Cursor != wantRequest {
			t.Fatalf("hello sync request cursor = %#v, want %#v", event.request.Cursor, wantRequest)
		}
		if event.cursor != wantAck {
			t.Fatalf("hello regressed acknowledged cursor to %#v, want %#v", event.cursor, wantAck)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for post-bootstrap hello sync callback")
	}
}

func TestHandlerLegacyHelloRoutesSyncWithoutClearingRecovery(t *testing.T) {
	hub := NewHub()
	go hub.Run()

	type syncEvent struct {
		client      *Client
		request     RuntimeSyncRequest
		needsResync bool
		pending     bool
	}
	events := make(chan syncEvent, 2)
	server := httptest.NewServer(NewHandler(
		hub,
		func() (*SnapshotPayload, uint64) { return EmptySnapshot(), 6 },
		func() AlertMessagePayload { return AlertMessagePayload{} },
		nil,
		WithRuntimeSync(func(client *Client, request RuntimeSyncRequest) {
			client.mu.Lock()
			event := syncEvent{
				client:      client,
				request:     request,
				needsResync: client.needsResync,
				pending:     client.pendingOverviewRecovery != nil,
			}
			client.mu.Unlock()
			events <- event
		}),
	))
	t.Cleanup(server.Close)

	params := url.Values{}
	params.Set("runtime_protocol", "2")
	params.Set("runtime_stream_id", "runtime-stream-1")
	params.Set("runtime_version", "5")
	conn, _, err := websocket.DefaultDialer.Dial(
		"ws"+strings.TrimPrefix(server.URL, "http")+"?"+params.Encode(),
		nil,
	)
	if err != nil {
		t.Fatalf("dial protocol-v2 client: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	var client *Client
	select {
	case event := <-events:
		client = event.client
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for bootstrap sync callback")
	}
	deadline := time.Now().Add(time.Second)
	for !hub.HasOverviewClients() && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if !hub.HasOverviewClients() {
		t.Fatal("client did not become overview-broadcast ready")
	}

	client.mu.Lock()
	client.needsResync = true
	client.pendingOverviewRecovery = &overviewRecoveryState{
		streamID:      "runtime-stream-1",
		targetVersion: 6,
		mode:          OverviewSyncModeReplay,
	}
	client.mu.Unlock()

	if err := conn.WriteJSON(map[string]any{
		"type": MessageTypeHello,
		"payload": map[string]any{
			"canvas_schema_version": 1,
			"runtime_version":       6,
		},
	}); err != nil {
		t.Fatalf("write post-bootstrap legacy hello: %v", err)
	}

	select {
	case event := <-events:
		if event.request.Cursor.Known {
			t.Fatalf("legacy hello sync cursor = %#v, want unknown", event.request.Cursor)
		}
		if !event.needsResync || !event.pending {
			t.Fatalf("legacy hello recovery state = needs_resync %t pending %t, want true true", event.needsResync, event.pending)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for post-bootstrap legacy hello sync callback")
	}
}

func TestHandlerRuntimeV2BootstrapInstallsOneAtomicSyncBatch(t *testing.T) {
	hub := NewHub()
	go hub.Run()

	var snapshotMu sync.Mutex
	snapshotCalls := 0
	recoverySnapshot := EmptySnapshot()
	recoverySnapshot.Devices["dev-1"] = DeviceRuntimeDTO{DeviceID: "dev-1", PrimaryHealth: "up_fresh"}
	syncResults := make(chan bool, 1)
	server := httptest.NewServer(NewHandler(
		hub,
		func() (*SnapshotPayload, uint64) {
			snapshotMu.Lock()
			defer snapshotMu.Unlock()
			snapshotCalls++
			return EmptySnapshot(), 42
		},
		func() AlertMessagePayload { return AlertMessagePayload{} },
		nil,
		WithRuntimeSync(func(client *Client, _ RuntimeSyncRequest) {
			syncResults <- hub.ReplaceOverviewStream(client, OverviewSyncBatch{
				Reason:          ResyncReasonClientResync,
				Mode:            OverviewSyncModeSnapshot,
				RuntimeStreamID: "runtime-stream-1",
				TargetVersion:   43,
				RuntimeIdentity: RuntimeIdentityForSnapshot(recoverySnapshot),
				Snapshot:        recoverySnapshot,
			})
		}),
	))
	t.Cleanup(server.Close)

	params := url.Values{}
	params.Set("runtime_protocol", "2")
	params.Set("runtime_stream_id", "runtime-stream-1")
	params.Set("runtime_version", "42")
	conn, _, err := websocket.DefaultDialer.Dial(
		"ws"+strings.TrimPrefix(server.URL, "http")+"?"+params.Encode(),
		nil,
	)
	if err != nil {
		t.Fatalf("dial protocol-v2 client: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	select {
	case ok := <-syncResults:
		if !ok {
			t.Fatal("runtime sync callback could not install atomic snapshot batch")
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for runtime sync batch installation")
	}

	runtimeTypes := make([]string, 0, 3)
	for len(runtimeTypes) < 3 {
		if err := conn.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
			t.Fatalf("set bootstrap read deadline: %v", err)
		}
		_, raw, err := conn.ReadMessage()
		if err != nil {
			t.Fatalf("read protocol-v2 bootstrap message: %v", err)
		}
		var message struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(raw, &message); err != nil {
			t.Fatalf("decode protocol-v2 bootstrap message: %v", err)
		}
		if message.Type == MessageTypeAlert || message.Type == MessageTypePrometheusStatus {
			continue
		}
		runtimeTypes = append(runtimeTypes, message.Type)
	}
	want := []string{MessageTypeResyncRequired, MessageTypeSnapshot, MessageTypeReady}
	for i := range want {
		if runtimeTypes[i] != want[i] {
			t.Fatalf("runtime bootstrap message types = %v, want %v", runtimeTypes, want)
		}
	}
	snapshotMu.Lock()
	gotSnapshotCalls := snapshotCalls
	snapshotMu.Unlock()
	if gotSnapshotCalls != 1 {
		t.Fatalf("snapshot callback calls = %d, want 1 without writeRuntimeCatchUp", gotSnapshotCalls)
	}
	conn.SetReadDeadline(time.Time{})
}

func TestHandlerRuntimeV2SyncBatchQueuesDeltaBeforeBroadcastReady(t *testing.T) {
	hub := NewHub()
	go hub.Run()

	snapshot := EmptySnapshot()
	batchInstalled := make(chan *Client, 1)
	releaseSync := make(chan struct{})
	var releaseOnce sync.Once
	release := func() {
		releaseOnce.Do(func() { close(releaseSync) })
	}
	server := httptest.NewServer(NewHandler(
		hub,
		func() (*SnapshotPayload, uint64) { return snapshot, 10 },
		func() AlertMessagePayload { return AlertMessagePayload{} },
		nil,
		WithRuntimeSync(func(client *Client, _ RuntimeSyncRequest) {
			if !hub.ReplaceOverviewStream(client, OverviewSyncBatch{
				Reason:          ResyncReasonClientResync,
				Mode:            OverviewSyncModeSnapshot,
				RuntimeStreamID: "runtime-stream-1",
				TargetVersion:   10,
				RuntimeIdentity: RuntimeIdentityForSnapshot(snapshot),
				Snapshot:        snapshot,
			}) {
				batchInstalled <- nil
				return
			}
			batchInstalled <- client
			<-releaseSync
		}),
	))
	t.Cleanup(func() {
		release()
		server.Close()
	})

	params := url.Values{}
	params.Set("runtime_protocol", "2")
	params.Set("runtime_stream_id", "runtime-stream-1")
	params.Set("runtime_version", "9")
	conn, _, err := websocket.DefaultDialer.Dial(
		"ws"+strings.TrimPrefix(server.URL, "http")+"?"+params.Encode(),
		nil,
	)
	if err != nil {
		t.Fatalf("dial protocol-v2 client: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	select {
	case client := <-batchInstalled:
		if client == nil {
			t.Fatal("runtime sync callback could not install bootstrap batch")
		}
		if err := conn.WriteJSON(map[string]any{
			"type": MessageTypeRuntimeAck,
			"payload": map[string]any{
				"runtime_stream_id": "runtime-stream-1",
				"runtime_version":   10,
			},
		}); err != nil {
			t.Fatalf("acknowledge queued bootstrap batch: %v", err)
		}
		deadline := time.Now().Add(time.Second)
		for time.Now().Before(deadline) {
			client.mu.Lock()
			pending := client.pendingOverviewRecovery
			cursor := client.ackedRuntimeCursor
			queued := len(client.overviewSend)
			client.mu.Unlock()
			if pending == nil && cursor.Version == 10 {
				if queued != 3 {
					t.Fatalf("ACK-cleared bootstrap batch queued messages = %d, want 3", queued)
				}
				break
			}
			time.Sleep(time.Millisecond)
		}
		client.mu.Lock()
		pending := client.pendingOverviewRecovery
		cursor := client.ackedRuntimeCursor
		client.mu.Unlock()
		if pending != nil || cursor.Version != 10 {
			t.Fatalf("bootstrap ACK state = pending %#v cursor %#v, want nil cursor version 10", pending, cursor)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for bootstrap batch installation")
	}
	hub.BroadcastOverviewStreamDelta(
		EmptyRuntimeDeltaPayload(),
		10,
		11,
		"runtime-stream-1",
	)
	release()

	runtimeTypes := make([]string, 0, 4)
	for len(runtimeTypes) < 4 {
		if err := conn.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
			t.Fatalf("set bootstrap read deadline: %v", err)
		}
		_, raw, err := conn.ReadMessage()
		if err != nil {
			t.Fatalf("read bootstrap/delta message %d: %v", len(runtimeTypes)+1, err)
		}
		var message struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(raw, &message); err != nil {
			t.Fatalf("decode bootstrap/delta message: %v", err)
		}
		if message.Type == MessageTypeAlert || message.Type == MessageTypePrometheusStatus {
			continue
		}
		runtimeTypes = append(runtimeTypes, message.Type)
	}
	want := []string{
		MessageTypeResyncRequired,
		MessageTypeSnapshot,
		MessageTypeReady,
		MessageTypeRuntimeDelta,
	}
	for i := range want {
		if runtimeTypes[i] != want[i] {
			t.Fatalf("runtime bootstrap/delta types = %v, want %v", runtimeTypes, want)
		}
	}
	conn.SetReadDeadline(time.Time{})
}

func TestHandlerRuntimeV2WithoutSyncOptionPreservesLegacyBootstrap(t *testing.T) {
	hub := NewHub()
	go hub.Run()

	server := httptest.NewServer(NewHandler(
		hub,
		func() (*SnapshotPayload, uint64) { return EmptySnapshot(), 42 },
		func() AlertMessagePayload { return AlertMessagePayload{} },
		nil,
	))
	t.Cleanup(server.Close)

	params := url.Values{}
	params.Set("runtime_protocol", "2")
	params.Set("runtime_stream_id", "runtime-stream-1")
	params.Set("runtime_version", "42")
	conn, _, err := websocket.DefaultDialer.Dial(
		"ws"+strings.TrimPrefix(server.URL, "http")+"?"+params.Encode(),
		nil,
	)
	if err != nil {
		t.Fatalf("dial protocol-v2 client without sync option: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	if err := conn.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatalf("set bootstrap read deadline: %v", err)
	}
	_, raw, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("read legacy-compatible bootstrap message: %v", err)
	}
	var message struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(raw, &message); err != nil {
		t.Fatalf("decode legacy-compatible bootstrap message: %v", err)
	}
	if message.Type != MessageTypeReady {
		t.Fatalf("bootstrap message type = %q, want legacy-compatible ready", message.Type)
	}
	conn.SetReadDeadline(time.Time{})
}

func TestHandlerRuntimeV2PrometheusBootstrapPrecedesQueuedNewerStatus(t *testing.T) {
	hub := NewHub()
	go hub.Run()

	server := httptest.NewServer(NewHandler(
		hub,
		func() (*SnapshotPayload, uint64) { return EmptySnapshot(), 42 },
		func() AlertMessagePayload { return AlertMessagePayload{} },
		func() PrometheusStatusPayload {
			hub.Broadcast(Message{
				Type: MessageTypePrometheusStatus,
				Payload: PrometheusStatusPayload{
					Enabled:   true,
					Available: true,
				},
			})
			return PrometheusStatusPayload{Enabled: true, Available: false}
		},
		WithRuntimeSync(func(_ *Client, _ RuntimeSyncRequest) {}),
	))
	t.Cleanup(server.Close)

	params := url.Values{}
	params.Set("runtime_protocol", "2")
	params.Set("runtime_stream_id", "runtime-stream-1")
	params.Set("runtime_version", "42")
	conn, _, err := websocket.DefaultDialer.Dial(
		"ws"+strings.TrimPrefix(server.URL, "http")+"?"+params.Encode(),
		nil,
	)
	if err != nil {
		t.Fatalf("dial protocol-v2 client: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	availability := make([]bool, 0, 2)
	for len(availability) < 2 {
		if err := conn.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
			t.Fatalf("set Prometheus status read deadline: %v", err)
		}
		_, raw, err := conn.ReadMessage()
		if err != nil {
			t.Fatalf("read Prometheus status message: %v", err)
		}
		var message struct {
			Type    string                  `json:"type"`
			Payload PrometheusStatusPayload `json:"payload"`
		}
		if err := json.Unmarshal(raw, &message); err != nil {
			t.Fatalf("decode Prometheus status message: %v", err)
		}
		if message.Type == MessageTypePrometheusStatus {
			availability = append(availability, message.Payload.Available)
		}
	}
	if availability[0] || !availability[1] {
		t.Fatalf("Prometheus availability order = %v, want bootstrap false then queued newer true", availability)
	}
	conn.SetReadDeadline(time.Time{})
}

func TestHandlerBootstrapFailureRemovesRegisteredClient(t *testing.T) {
	hub := NewHub()
	go hub.Run()

	unsupported := math.NaN()
	snapshot := EmptySnapshot()
	snapshot.Devices["dev-1"] = DeviceRuntimeDTO{DeviceID: "dev-1", CPUPercent: &unsupported}
	snapshotCalled := make(chan struct{})
	var snapshotOnce sync.Once
	server := httptest.NewServer(NewHandler(
		hub,
		func() (*SnapshotPayload, uint64) {
			snapshotOnce.Do(func() { close(snapshotCalled) })
			return snapshot, 42
		},
		func() AlertMessagePayload { return AlertMessagePayload{} },
		nil,
	))
	t.Cleanup(server.Close)

	conn, _, err := websocket.DefaultDialer.Dial(
		"ws"+strings.TrimPrefix(server.URL, "http"),
		nil,
	)
	if err != nil {
		t.Fatalf("dial bootstrap failure client: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	select {
	case <-snapshotCalled:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for failing snapshot callback")
	}
	registeredDeadline := time.Now().Add(time.Second)
	for len(hub.copyClients()) == 0 && time.Now().Before(registeredDeadline) {
		time.Sleep(time.Millisecond)
	}
	if len(hub.copyClients()) != 1 {
		t.Fatalf("registered clients = %d, want 1 before bootstrap failure", len(hub.copyClients()))
	}
	removedDeadline := time.Now().Add(2 * time.Second)
	for len(hub.copyClients()) != 0 && time.Now().Before(removedDeadline) {
		time.Sleep(time.Millisecond)
	}
	if got := len(hub.copyClients()); got != 0 {
		t.Fatalf("registered clients after bootstrap failure = %d, want 0", got)
	}
}
