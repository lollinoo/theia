package ws

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/observability"
)

func TestHubSetDetailSubscription_ReplacesPreviousDevice(t *testing.T) {
	hub := NewHub()
	client := registerTestClient(hub)
	firstDeviceID := uuid.New()
	secondDeviceID := uuid.New()

	hub.SetDetailSubscription(client, firstDeviceID)
	hub.SetDetailSubscription(client, secondDeviceID)

	if client.detailDeviceID != secondDeviceID {
		t.Fatalf("detailDeviceID = %s, want %s", client.detailDeviceID, secondDeviceID)
	}
}

func TestHubDetailSubscribers_ReturnsOnlyMatchingClients(t *testing.T) {
	hub := NewHub()
	selectedDeviceID := uuid.New()
	otherDeviceID := uuid.New()

	matchingA := registerTestClient(hub)
	matchingB := registerTestClient(hub)
	other := registerTestClient(hub)
	unsubscribed := registerTestClient(hub)

	hub.SetDetailSubscription(matchingA, selectedDeviceID)
	hub.SetDetailSubscription(matchingB, selectedDeviceID)
	hub.SetDetailSubscription(other, otherDeviceID)

	subscribers := hub.DetailSubscribers(selectedDeviceID)
	if len(subscribers) != 2 {
		t.Fatalf("subscriber count = %d, want 2", len(subscribers))
	}

	if !containsClient(subscribers, matchingA) {
		t.Fatal("matchingA missing from subscribers")
	}

	if !containsClient(subscribers, matchingB) {
		t.Fatal("matchingB missing from subscribers")
	}

	if containsClient(subscribers, other) {
		t.Fatal("other client unexpectedly present in subscribers")
	}

	if containsClient(subscribers, unsubscribed) {
		t.Fatal("unsubscribed client unexpectedly present in subscribers")
	}
}

func TestHubClearDetailSubscription_RemovesClientFromSelection(t *testing.T) {
	hub := NewHub()
	deviceID := uuid.New()
	client := registerTestClient(hub)

	hub.SetDetailSubscription(client, deviceID)
	hub.ClearDetailSubscription(client)

	if client.detailDeviceID != uuid.Nil {
		t.Fatalf("detailDeviceID = %s, want nil UUID", client.detailDeviceID)
	}

	if subscribers := hub.DetailSubscribers(deviceID); len(subscribers) != 0 {
		t.Fatalf("subscriber count = %d, want 0", len(subscribers))
	}
}

func TestHubRemoveClient_DropsSubscriptionState(t *testing.T) {
	hub := NewHub()
	deviceID := uuid.New()
	client := registerTestClient(hub)

	hub.SetDetailSubscription(client, deviceID)
	hub.removeClient(client)

	if client.detailDeviceID != uuid.Nil {
		t.Fatalf("detailDeviceID = %s, want nil UUID", client.detailDeviceID)
	}

	if subscribers := hub.DetailSubscribers(deviceID); len(subscribers) != 0 {
		t.Fatalf("subscriber count = %d, want 0", len(subscribers))
	}
}

func TestHubBroadcast_RecordsHubBufferBackpressure(t *testing.T) {
	registry := observability.ResetDefaultForTest()
	t.Cleanup(func() {
		observability.ResetDefaultForTest()
	})

	hub := NewHub()
	for i := 0; i < cap(hub.broadcast); i++ {
		hub.broadcast <- []byte("prefill")
	}

	hub.Broadcast(Message{Type: MessageTypeSnapshot, Payload: EmptySnapshot()})

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		metrics := string(registry.MarshalPrometheus())
		if strings.Contains(metrics, `theia_ws_backpressure_total{reason="hub_buffer_full",scope="broadcast"} 1`) {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	metrics := string(registry.MarshalPrometheus())
	if !strings.Contains(metrics, `theia_ws_backpressure_total{reason="hub_buffer_full",scope="broadcast"} 1`) {
		t.Fatalf("expected hub buffer backpressure metric, got:\n%s", metrics)
	}
}

func TestHubOverviewBufferAbsorbsShortClientStalls(t *testing.T) {
	hub := NewHub()
	client := registerTestClient(hub)

	if cap(client.overviewSend) < 32 {
		t.Fatalf("overviewSend capacity = %d, want at least 32", cap(client.overviewSend))
	}
}

func TestHubOverviewDelta_FullMailboxSchedulesResyncAndSnapshotForLegacyClient(t *testing.T) {
	hub := NewHub()
	client := registerTestClient(hub)
	for i := 0; i < cap(client.overviewSend); i++ {
		client.overviewSend <- []byte("occupied")
	}

	fallback := EmptySnapshot()
	fallback.DeviceStatuses["dev-1"] = "up"
	hub.BroadcastOverviewDelta(EmptyRuntimeDeltaPayload(), 1, 2, fallback)

	first := <-client.overviewSend
	second := <-client.overviewSend

	if !strings.Contains(string(first), MessageTypeResyncRequired) {
		t.Fatalf("expected first overview message to be resync_required, got %s", string(first))
	}
	if !strings.Contains(string(second), MessageTypeSnapshot) {
		t.Fatalf("expected second overview message to be snapshot, got %s", string(second))
	}
	if client.needsResync {
		t.Fatal("expected client resync flag to clear after fallback snapshot")
	}
}

func TestHubOverviewDelta_FullMailboxSchedulesResyncOnlyForHTTPBootstrapClient(t *testing.T) {
	hub := NewHub()
	client := registerTestClient(hub)
	client.usesHTTPRuntimeBootstrap = true
	for i := 0; i < cap(client.overviewSend); i++ {
		client.overviewSend <- []byte("occupied")
	}

	fallback := EmptySnapshot()
	fallback.DeviceStatuses["dev-1"] = "up"
	hub.BroadcastOverviewDelta(EmptyRuntimeDeltaPayload(), 1, 2, fallback)

	if got := len(client.overviewSend); got != 1 {
		t.Fatalf("overview mailbox length = %d, want 1", got)
	}
	payload := <-client.overviewSend
	if !strings.Contains(string(payload), MessageTypeResyncRequired) {
		t.Fatalf("expected overview message to be resync_required, got %s", string(payload))
	}
	if strings.Contains(string(payload), MessageTypeSnapshot) {
		t.Fatalf("expected HTTP bootstrap client not to receive snapshot, got %s", string(payload))
	}
	if !client.needsResync {
		t.Fatal("expected client to remain marked for HTTP resync")
	}

	version := uint64(2)
	client.acceptHello(clientControlMessage{
		Type:           MessageTypeHello,
		RuntimeVersion: &version,
	})
	if client.needsResync {
		t.Fatal("expected client hello to clear HTTP resync marker")
	}
}

func TestHubOverviewSnapshotSendsResyncOnlyForHTTPBootstrapClient(t *testing.T) {
	hub := NewHub()
	client := registerTestClient(hub)
	client.usesHTTPRuntimeBootstrap = true

	snapshot := EmptySnapshot()
	snapshot.DeviceStatuses["dev-1"] = "up"
	hub.BroadcastOverviewSnapshot(snapshot, 4)

	if got := len(client.overviewSend); got != 1 {
		t.Fatalf("overview mailbox length = %d, want 1", got)
	}
	payload := <-client.overviewSend
	if !strings.Contains(string(payload), MessageTypeResyncRequired) {
		t.Fatalf("expected overview message to be resync_required, got %s", string(payload))
	}
	if strings.Contains(string(payload), MessageTypeSnapshot) {
		t.Fatalf("expected HTTP bootstrap client not to receive snapshot, got %s", string(payload))
	}
}

func TestHubOverviewDeltaUsesRuntimeDeltaEnvelope(t *testing.T) {
	hub := NewHub()
	client := registerTestClient(hub)

	hub.BroadcastOverviewDelta(EmptyRuntimeDeltaPayload(), 1, 2, EmptySnapshot())

	payload := <-client.overviewSend
	if !strings.Contains(string(payload), MessageTypeRuntimeDelta) {
		t.Fatalf("expected overview delta to use runtime_delta, got %s", string(payload))
	}
	if strings.Contains(string(payload), MessageTypeSnapshotDelta) {
		t.Fatalf("expected overview delta not to use snapshot_delta, got %s", string(payload))
	}
}

func TestHubOverviewDelta_DebugLogsPatchCounts(t *testing.T) {
	logs := captureDebugLogs(t)
	hub := NewHub()
	client := registerTestClient(hub)
	delta := &RuntimeDeltaPayload{
		Devices: map[string]map[string]any{
			"dev-1": {"primary_health": "up_fresh"},
			"dev-2": {"primary_health": "unreachable"},
		},
		Links: map[string]map[string]any{
			"link-1": {"utilization": 0.42},
		},
	}

	hub.BroadcastOverviewDelta(delta, 7, 8, EmptySnapshot())
	<-client.overviewSend

	output := logs.String()
	if !strings.Contains(output, "type=runtime_delta base_version=7 version=8") {
		t.Fatalf("debug output missing runtime delta version summary: %q", output)
	}
	if !strings.Contains(output, "device_patches=2 link_patches=1") {
		t.Fatalf("debug output missing runtime delta patch counts: %q", output)
	}
}

func TestHubEnqueue_RecordsClientBufferBackpressure(t *testing.T) {
	registry := observability.ResetDefaultForTest()
	t.Cleanup(func() {
		observability.ResetDefaultForTest()
	})

	hub := NewHub()
	client := registerTestClient(hub)
	client.send <- []byte("occupied")

	if ok := hub.enqueue(client, []byte("blocked")); ok {
		t.Fatal("enqueue succeeded despite full client buffer")
	}

	metrics := string(registry.MarshalPrometheus())
	if !strings.Contains(metrics, `theia_ws_backpressure_total{reason="client_buffer_full",scope="client_send"} 1`) {
		t.Fatalf("expected client buffer backpressure metric, got:\n%s", metrics)
	}
}

func registerTestClient(hub *Hub) *Client {
	client := &Client{
		hub:          hub,
		send:         make(chan []byte, 1),
		overviewSend: make(chan []byte, overviewBufferSize),
	}

	hub.mu.Lock()
	hub.clients[client] = true
	hub.mu.Unlock()

	return client
}

func containsClient(clients []*Client, target *Client) bool {
	for _, client := range clients {
		if client == target {
			return true
		}
	}

	return false
}
