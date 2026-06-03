package ws

import (
	"math"
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

func TestHubClientConnectionChurnMetrics(t *testing.T) {
	registry := observability.ResetDefaultForTest()
	t.Cleanup(func() {
		observability.ResetDefaultForTest()
	})

	hub := NewHub()
	first := newObservedTestClient(hub)
	second := newObservedTestClient(hub)

	hub.addClient(first)
	hub.addClient(first)
	hub.addClient(second)
	hub.removeClient(first)
	hub.removeClient(second)

	metrics := string(registry.MarshalPrometheus())
	if !strings.Contains(metrics, `theia_ws_connections_total{event="connected"} 2`) {
		t.Fatalf("expected connected churn metric, got:\n%s", metrics)
	}
	if !strings.Contains(metrics, `theia_ws_connections_total{event="disconnected"} 2`) {
		t.Fatalf("expected disconnected churn metric, got:\n%s", metrics)
	}
	if !strings.Contains(metrics, `theia_ws_connected_clients 0`) {
		t.Fatalf("expected connected client gauge to return to zero, got:\n%s", metrics)
	}
}

func TestHubBroadcast_DefaultRecorderDisabledDoesNotEmitHubBufferBackpressure(t *testing.T) {
	registry := observability.ResetDefaultForTest()
	t.Cleanup(func() {
		observability.ResetDefaultForTest()
	})

	hub := NewHub()
	for i := 0; i < sendBufferSize*3; i++ {
		hub.Broadcast(Message{Type: MessageTypeSnapshot, Payload: EmptySnapshot()})
	}

	metrics := string(registry.MarshalPrometheus())
	if strings.Contains(metrics, `reason="hub_buffer_full",scope="broadcast"`) {
		t.Fatalf("unexpected hub recorder backpressure metric, got:\n%s", metrics)
	}
}

func TestHubBroadcastCh_EnablesTestRecorder(t *testing.T) {
	hub := NewHub()
	recorded := hub.BroadcastCh()

	hub.Broadcast(Message{Type: MessageTypeSnapshot, Payload: EmptySnapshot()})

	select {
	case payload := <-recorded:
		if !strings.Contains(string(payload), MessageTypeSnapshot) {
			t.Fatalf("expected snapshot payload, got %s", string(payload))
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for recorded broadcast")
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

func TestHubOverviewDelta_RecordsLegacyFallbackSnapshotPayloadMetrics(t *testing.T) {
	registry := observability.ResetDefaultForTest()
	t.Cleanup(func() {
		observability.ResetDefaultForTest()
	})

	hub := NewHub()
	firstClient := registerTestClient(hub)
	secondClient := registerTestClient(hub)
	for _, client := range []*Client{firstClient, secondClient} {
		for i := 0; i < cap(client.overviewSend); i++ {
			client.overviewSend <- []byte("occupied")
		}
	}

	fallback := EmptySnapshot()
	fallback.Devices["dev-1"] = DeviceRuntimeDTO{
		DeviceID:      "dev-1",
		PrimaryHealth: "up_fresh",
	}
	hub.BroadcastOverviewDelta(EmptyRuntimeDeltaPayload(), 1, 2, fallback)

	metrics := string(registry.MarshalPrometheus())
	if !strings.Contains(metrics, `theia_ws_messages_total{scope="overview_fallback",type="snapshot"} 2`) {
		t.Fatalf("expected legacy fallback snapshot message metric, got:\n%s", metrics)
	}
	if !strings.Contains(metrics, `theia_ws_message_payload_bytes_count{scope="overview_fallback",type="snapshot"} 2`) {
		t.Fatalf("expected legacy fallback snapshot payload histogram, got:\n%s", metrics)
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

func TestHubOverviewDelta_SkipsFallbackSerializationWhenAllClientsUseHTTPBootstrap(t *testing.T) {
	hub := NewHub()
	client := registerTestClient(hub)
	client.usesHTTPRuntimeBootstrap = true
	for i := 0; i < cap(client.overviewSend); i++ {
		client.overviewSend <- []byte("occupied")
	}

	unsupported := math.NaN()
	fallback := EmptySnapshot()
	fallback.Devices["dev-1"] = DeviceRuntimeDTO{
		DeviceID:   "dev-1",
		CPUPercent: &unsupported,
	}

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
}

func TestHubOverviewDelta_SkipsFallbackSerializationForReadyLegacyClient(t *testing.T) {
	hub := NewHub()
	client := registerTestClient(hub)

	unsupported := math.NaN()
	fallback := EmptySnapshot()
	fallback.Devices["dev-1"] = DeviceRuntimeDTO{
		DeviceID:   "dev-1",
		CPUPercent: &unsupported,
	}

	hub.BroadcastOverviewDelta(EmptyRuntimeDeltaPayload(), 1, 2, fallback)

	if got := len(client.overviewSend); got != 1 {
		t.Fatalf("overview mailbox length = %d, want 1", got)
	}
	payload := <-client.overviewSend
	if !strings.Contains(string(payload), MessageTypeRuntimeDelta) {
		t.Fatalf("expected overview message to be runtime_delta, got %s", string(payload))
	}
	if strings.Contains(string(payload), MessageTypeSnapshot) {
		t.Fatalf("expected ready legacy client not to receive snapshot, got %s", string(payload))
	}
}

func TestHubOverviewDelta_SkipsLegacyFallbackWhenClientBecomesHTTPBootstrap(t *testing.T) {
	hub := NewHub()
	client := registerTestClient(hub)
	for i := 0; i < cap(client.overviewSend); i++ {
		client.overviewSend <- []byte("occupied")
	}

	resyncPayload, err := marshalOverviewResyncPayload(ResyncReasonClientResync)
	if err != nil {
		t.Fatalf("marshal resync payload: %v", err)
	}
	_, needsFallback, observedOverviewEpoch := hub.enqueueOverviewDelta(client, []byte(`{"type":"runtime_delta"}`), resyncPayload)
	if !needsFallback {
		t.Fatal("expected legacy overflow to request fallback")
	}

	version := uint64(2)
	client.acceptHello(clientControlMessage{
		Type:           MessageTypeHello,
		RuntimeVersion: &version,
	})

	if ok := hub.enqueueOverviewLegacyFallback(client, []byte(`{"type":"runtime_delta"}`), resyncPayload, []byte(`{"type":"snapshot"}`), observedOverviewEpoch); ok {
		t.Fatal("expected fallback enqueue to be skipped after HTTP bootstrap")
	}
	if got := len(client.overviewSend); got != 0 {
		t.Fatalf("overview mailbox length = %d, want 0", got)
	}
	if client.needsResync {
		t.Fatal("expected client to remain out of legacy fallback resync state")
	}
}

func TestHubOverviewDelta_SkipsStaleLegacyFallbackAfterNewerSnapshot(t *testing.T) {
	hub := NewHub()
	client := registerTestClient(hub)
	for i := 0; i < cap(client.overviewSend); i++ {
		client.overviewSend <- []byte("occupied")
	}

	resyncPayload, err := marshalOverviewResyncPayload(ResyncReasonClientResync)
	if err != nil {
		t.Fatalf("marshal resync payload: %v", err)
	}
	_, needsFallback, observedOverviewEpoch := hub.enqueueOverviewDelta(client, []byte(`{"type":"old-runtime-delta"}`), resyncPayload)
	if !needsFallback {
		t.Fatal("expected legacy overflow to request fallback")
	}

	hub.enqueueOverviewSnapshot(client, []byte(`{"type":"new-snapshot"}`))

	if ok := hub.enqueueOverviewLegacyFallback(client, []byte(`{"type":"old-runtime-delta"}`), resyncPayload, []byte(`{"type":"old-snapshot"}`), observedOverviewEpoch); ok {
		t.Fatal("expected stale fallback enqueue to be skipped after newer snapshot")
	}
	if got := len(client.overviewSend); got != 1 {
		t.Fatalf("overview mailbox length = %d, want 1", got)
	}
	payload := <-client.overviewSend
	if !strings.Contains(string(payload), "new-snapshot") {
		t.Fatalf("expected newer snapshot to remain queued, got %s", string(payload))
	}
	if strings.Contains(string(payload), "old-runtime-delta") || strings.Contains(string(payload), "old-snapshot") {
		t.Fatalf("expected stale fallback payloads not to be queued, got %s", string(payload))
	}
	if client.needsResync {
		t.Fatal("expected newer snapshot state not to be marked for legacy resync")
	}
}

func TestHubOverviewDelta_RecordsHTTPResyncMetricsOnceWhilePending(t *testing.T) {
	registry := observability.ResetDefaultForTest()
	t.Cleanup(func() {
		observability.ResetDefaultForTest()
	})

	hub := NewHub()
	client := registerTestClient(hub)
	client.usesHTTPRuntimeBootstrap = true
	for i := 0; i < cap(client.overviewSend); i++ {
		client.overviewSend <- []byte("occupied")
	}

	hub.BroadcastOverviewDelta(EmptyRuntimeDeltaPayload(), 1, 2, EmptySnapshot())
	hub.BroadcastOverviewDelta(EmptyRuntimeDeltaPayload(), 2, 3, EmptySnapshot())

	if got := len(client.overviewSend); got != 1 {
		t.Fatalf("overview mailbox length = %d, want 1", got)
	}

	metrics := string(registry.MarshalPrometheus())
	if !strings.Contains(metrics, `theia_ws_client_resync_required_total{bootstrap="http",reason="client_resync_scheduled",scope="overview"} 1`) {
		t.Fatalf("expected one HTTP client resync metric, got:\n%s", metrics)
	}
	if !strings.Contains(metrics, `theia_ws_overview_mailbox_clear_total{reason="client_mailbox_full"} 32`) {
		t.Fatalf("expected cleared mailbox metric for the first overflow, got:\n%s", metrics)
	}
	if !strings.Contains(metrics, `theia_ws_overview_resync_suppressed_total{reason="client_resync_scheduled"} 1`) {
		t.Fatalf("expected duplicate resync suppression metric, got:\n%s", metrics)
	}
}

func TestHubAddRemoveClientUpdatesConnectedClientMetric(t *testing.T) {
	registry := observability.ResetDefaultForTest()
	t.Cleanup(func() {
		observability.ResetDefaultForTest()
	})

	hub := NewHub()
	client := &Client{
		hub:          hub,
		send:         make(chan []byte, sendBufferSize),
		overviewSend: make(chan []byte, overviewBufferSize),
	}

	hub.addClient(client)
	if metrics := string(registry.MarshalPrometheus()); !strings.Contains(metrics, `theia_ws_connected_clients 1`) {
		t.Fatalf("expected connected client gauge to be 1, got:\n%s", metrics)
	}

	hub.removeClient(client)
	if metrics := string(registry.MarshalPrometheus()); !strings.Contains(metrics, `theia_ws_connected_clients 0`) {
		t.Fatalf("expected connected client gauge to be 0, got:\n%s", metrics)
	}
}

func TestClientAcceptHelloClearsQueuedOverviewMessages(t *testing.T) {
	hub := NewHub()
	client := registerTestClient(hub)
	client.usesHTTPRuntimeBootstrap = true
	client.needsResync = true
	client.overviewSend <- []byte("stale-runtime-delta")
	client.overviewSend <- []byte("stale-resync-marker")

	version := uint64(12)
	client.acceptHello(clientControlMessage{
		Type:           MessageTypeHello,
		RuntimeVersion: &version,
	})

	if client.needsResync {
		t.Fatal("expected client hello to clear HTTP resync marker")
	}
	if got := len(client.overviewSend); got != 0 {
		t.Fatalf("overview mailbox length = %d, want 0", got)
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

func TestHubOverviewResyncSendsMarkerOnlyForHTTPBootstrapClient(t *testing.T) {
	hub := NewHub()
	client := registerTestClient(hub)
	client.usesHTTPRuntimeBootstrap = true

	snapshot := EmptySnapshot()
	snapshot.DeviceStatuses["dev-1"] = "up"
	hub.BroadcastOverviewResync(ResyncReasonStateChangesDrop, snapshot, 7)

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

func newObservedTestClient(hub *Hub) *Client {
	return &Client{
		hub:          hub,
		send:         make(chan []byte, 1),
		overviewSend: make(chan []byte, overviewBufferSize),
	}
}

func containsClient(clients []*Client, target *Client) bool {
	for _, client := range clients {
		if client == target {
			return true
		}
	}

	return false
}
