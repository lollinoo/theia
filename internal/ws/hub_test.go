package ws

// This file exercises hub behavior so refactors preserve the documented contract.

import (
	"encoding/json"
	"math"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/lollinoo/theia/internal/observability"
)

func TestHubHasOverviewClientsReflectsRegisteredClients(t *testing.T) {
	hub := NewHub()
	if hub.HasOverviewClients() {
		t.Fatal("new hub HasOverviewClients() = true, want false")
	}

	client := newObservedTestClient(hub)
	hub.addClient(client)
	if !hub.HasOverviewClients() {
		t.Fatal("after addClient HasOverviewClients() = false, want true")
	}

	hub.removeClient(client)
	if hub.HasOverviewClients() {
		t.Fatal("after removeClient HasOverviewClients() = true, want false")
	}
}

func TestHubHasOverviewClientsIgnoresBootstrappingClients(t *testing.T) {
	hub := NewHub()
	client := newObservedTestClient(hub)
	client.bootstrapping = true
	selection := client.beginBootstrapSnapshotSelection()
	hub.addClient(client)
	if hub.HasOverviewClients() {
		t.Fatal("bootstrapping client made HasOverviewClients() = true, want false")
	}

	_ = client.markBootstrapSnapshotSelected(selection, 1, "rt-sha256:test")
	if !hub.HasOverviewClients() {
		t.Fatal("bootstrap-complete client made HasOverviewClients() = false, want true")
	}
}

func TestHubOverviewSnapshotNoClientSkipsSerialization(t *testing.T) {
	registry := observability.ResetDefaultForTest()
	t.Cleanup(func() {
		observability.ResetDefaultForTest()
	})
	logs := captureDebugLogs(t)
	hub := NewHub(WithBroadcastRecorder())

	unsupported := math.NaN()
	snapshot := EmptySnapshot()
	snapshot.Devices["dev-1"] = DeviceRuntimeDTO{
		DeviceID:   "dev-1",
		CPUPercent: &unsupported,
	}

	hub.BroadcastOverviewSnapshot(snapshot, 1)

	assertNoNoClientOverviewBroadcastEffects(t, hub, registry.MarshalPrometheus(), logs.String())
}

func TestHubOverviewDeltaNoClientSkipsSerialization(t *testing.T) {
	registry := observability.ResetDefaultForTest()
	t.Cleanup(func() {
		observability.ResetDefaultForTest()
	})
	logs := captureDebugLogs(t)
	hub := NewHub(WithBroadcastRecorder())

	delta := EmptyRuntimeDeltaPayload()
	delta.Devices["dev-1"] = map[string]any{
		"cpu_percent": math.NaN(),
	}

	hub.BroadcastOverviewDelta(delta, 1, 2, EmptySnapshot())

	assertNoNoClientOverviewBroadcastEffects(t, hub, registry.MarshalPrometheus(), logs.String())
}

func TestHubOverviewResyncNoClientSkipsSerialization(t *testing.T) {
	registry := observability.ResetDefaultForTest()
	t.Cleanup(func() {
		observability.ResetDefaultForTest()
	})
	logs := captureDebugLogs(t)
	hub := NewHub(WithBroadcastRecorder())

	unsupported := math.NaN()
	snapshot := EmptySnapshot()
	snapshot.Devices["dev-1"] = DeviceRuntimeDTO{
		DeviceID:   "dev-1",
		CPUPercent: &unsupported,
	}

	hub.BroadcastOverviewResync(ResyncReasonClientResync, snapshot, 1)

	assertNoNoClientOverviewBroadcastEffects(t, hub, registry.MarshalPrometheus(), logs.String())
}

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
	third := <-client.overviewSend

	if !strings.Contains(string(first), MessageTypeResyncRequired) {
		t.Fatalf("expected first overview message to be resync_required, got %s", string(first))
	}
	if !strings.Contains(string(second), MessageTypeSnapshot) {
		t.Fatalf("expected second overview message to be snapshot, got %s", string(second))
	}
	if !strings.Contains(string(third), MessageTypeReady) {
		t.Fatalf("expected third overview message to be ready, got %s", string(third))
	}
	if !client.needsResync {
		t.Fatal("expected complete recovery to remain pending until acknowledged")
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

func TestHubOverviewDelta_FullMailboxSchedulesCompleteRecoveryForHTTPBootstrapClient(t *testing.T) {
	hub := NewHub()
	client := registerTestClient(hub)
	client.usesHTTPRuntimeBootstrap = true
	for i := 0; i < cap(client.overviewSend); i++ {
		client.overviewSend <- []byte("occupied")
	}

	fallback := EmptySnapshot()
	fallback.DeviceStatuses["dev-1"] = "up"
	hub.BroadcastOverviewDelta(EmptyRuntimeDeltaPayload(), 1, 2, fallback)

	if got := len(client.overviewSend); got != 3 {
		t.Fatalf("overview mailbox length = %d, want complete three-message recovery", got)
	}
	assertOverviewTestMessageTypes(
		t,
		drainOverviewTestMessages(t, client),
		MessageTypeResyncRequired,
		MessageTypeSnapshot,
		MessageTypeReady,
	)
	if !client.needsResync {
		t.Fatal("expected recovery to remain pending until acknowledged")
	}

	client.acceptHello(clientControlMessage{
		Type:            MessageTypeHello,
		RuntimeProtocol: RuntimeStreamProtocolVersion,
		RuntimeCursor: RuntimeCursor{
			Version: 2,
			Known:   false,
		},
	})
	if !client.needsResync {
		t.Fatal("unvalidated client hello cleared pending recovery")
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

	if got := len(client.overviewSend); got != 0 {
		t.Fatalf("overview mailbox length = %d, want no partial recovery after marshal failure", got)
	}
	if !client.needsResync {
		t.Fatal("marshal failure cleared recovery-pending state")
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

func TestHubOverviewDelta_RecordsHTTPRecoveryMetricsWhileKeepingMailboxBounded(t *testing.T) {
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

	if got, limit := len(client.overviewSend), cap(client.overviewSend); got > limit {
		t.Fatalf("overview mailbox length = %d, want at most %d", got, limit)
	}

	metrics := string(registry.MarshalPrometheus())
	if !strings.Contains(metrics, `theia_ws_client_resync_required_total{bootstrap="http",reason="client_resync_scheduled",scope="overview"} 1`) {
		t.Fatalf("expected one HTTP client resync metric, got:\n%s", metrics)
	}
	if !strings.Contains(metrics, `theia_ws_overview_mailbox_clear_total{reason="client_mailbox_full"} 32`) {
		t.Fatalf("expected cleared mailbox metric for the first overflow, got:\n%s", metrics)
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

func TestClientAcceptHelloKeepsRecoveryUntilTargetCursorIsAcknowledged(t *testing.T) {
	hub := NewHub()
	client := registerTestClient(hub)
	client.usesHTTPRuntimeBootstrap = true
	client.needsResync = true
	client.pendingOverviewRecovery = &overviewRecoveryState{
		streamID:      "runtime-stream-1",
		targetVersion: 12,
		mode:          OverviewSyncModeReplay,
	}
	client.overviewSend <- []byte("stale-runtime-delta")
	client.overviewSend <- []byte("stale-resync-marker")

	client.acceptHello(clientControlMessage{
		Type:            MessageTypeHello,
		RuntimeProtocol: RuntimeStreamProtocolVersion,
		RuntimeCursor: RuntimeCursor{
			StreamID: "runtime-stream-1",
			Version:  11,
			Known:    true,
		},
	})

	if !client.needsResync || client.pendingOverviewRecovery == nil {
		t.Fatal("intermediate cursor cleared pending recovery")
	}
	if got := len(client.overviewSend); got != 2 {
		t.Fatalf("overview mailbox length = %d, want recovery queue preserved", got)
	}

	client.acceptHello(clientControlMessage{
		Type:            MessageTypeHello,
		RuntimeProtocol: RuntimeStreamProtocolVersion,
		RuntimeCursor: RuntimeCursor{
			StreamID: "runtime-stream-1",
			Version:  12,
			Known:    true,
		},
	})
	if client.needsResync || client.pendingOverviewRecovery != nil {
		t.Fatal("target cursor did not acknowledge pending recovery")
	}
	if got := len(client.overviewSend); got != 2 {
		t.Fatalf("acknowledging recovery cleared %d queued messages, want queue preserved", 2-got)
	}
}

func TestHubOverviewSnapshotSendsCompleteBatchToHTTPBootstrapClient(t *testing.T) {
	hub := NewHub()
	client := registerTestClient(hub)
	client.usesHTTPRuntimeBootstrap = true

	snapshot := EmptySnapshot()
	snapshot.DeviceStatuses["dev-1"] = "up"
	hub.BroadcastOverviewSnapshot(snapshot, 4)

	if got := len(client.overviewSend); got != 3 {
		t.Fatalf("overview mailbox length = %d, want complete three-message recovery", got)
	}
	assertOverviewTestMessageTypes(
		t,
		drainOverviewTestMessages(t, client),
		MessageTypeResyncRequired,
		MessageTypeSnapshot,
		MessageTypeReady,
	)
}

func TestHubOverviewResyncSendsCompleteBatchToHTTPBootstrapClient(t *testing.T) {
	hub := NewHub()
	client := registerTestClient(hub)
	client.usesHTTPRuntimeBootstrap = true

	snapshot := EmptySnapshot()
	snapshot.DeviceStatuses["dev-1"] = "up"
	hub.BroadcastOverviewResync(ResyncReasonStateChangesDrop, snapshot, 7)

	if got := len(client.overviewSend); got != 3 {
		t.Fatalf("overview mailbox length = %d, want complete three-message recovery", got)
	}
	assertOverviewTestMessageTypes(
		t,
		drainOverviewTestMessages(t, client),
		MessageTypeResyncRequired,
		MessageTypeSnapshot,
		MessageTypeReady,
	)
	if !client.needsResync {
		t.Fatal("expected client recovery to remain pending until acknowledged")
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

func TestReplaceOverviewStreamReplacesFullMailboxWithCompleteSnapshotBatch(t *testing.T) {
	hub := NewHub()
	client := registerTestClient(hub)
	ackVersion := uint64(92)
	client.acceptHello(clientControlMessage{
		Type:            MessageTypeHello,
		RuntimeProtocol: RuntimeStreamProtocolVersion,
		RuntimeCursor: RuntimeCursor{
			StreamID: "runtime-stream-1",
			Version:  ackVersion,
			Known:    true,
		},
	})
	for len(client.overviewSend) < cap(client.overviewSend) {
		client.overviewSend <- []byte(`{"type":"stale_runtime_delta"}`)
	}

	snapshot := EmptySnapshot()
	snapshot.Devices["dev-1"] = DeviceRuntimeDTO{DeviceID: "dev-1", PrimaryHealth: "up_fresh"}
	batch := OverviewSyncBatch{
		Reason:          ResyncReasonClientResync,
		Mode:            OverviewSyncModeSnapshot,
		RuntimeStreamID: "runtime-stream-1",
		TargetVersion:   94,
		RuntimeIdentity: RuntimeIdentityForSnapshot(snapshot),
		Snapshot:        snapshot,
		AlertVersion:    7,
	}

	if ok := hub.ReplaceOverviewStream(client, batch); !ok {
		t.Fatal("ReplaceOverviewStream returned false")
	}

	messages := drainOverviewTestMessages(t, client)
	assertOverviewTestMessageTypes(t, messages, MessageTypeResyncRequired, MessageTypeSnapshot, MessageTypeReady)

	var marker ResyncRequiredPayload
	decodeOverviewTestPayload(t, messages[0], &marker)
	if marker.Strategy != RuntimeSyncStrategyStream || marker.TargetVersion == nil || *marker.TargetVersion != 94 || marker.RuntimeStreamID != "runtime-stream-1" {
		t.Fatalf("resync marker = %#v, want stream recovery target runtime-stream-1/94", marker)
	}

	var state SnapshotMessagePayload
	decodeOverviewTestPayload(t, messages[1], &state)
	if state.Version != 94 || state.RuntimeStreamID != "runtime-stream-1" || state.RuntimeIdentity != batch.RuntimeIdentity {
		t.Fatalf("snapshot state = %#v, want runtime-stream-1/94 identity %q", state, batch.RuntimeIdentity)
	}

	var ready ReadyPayload
	decodeOverviewTestPayload(t, messages[2], &ready)
	if ready.RuntimeVersion != 94 || ready.RuntimeStreamID != "runtime-stream-1" || ready.SyncMode != string(OverviewSyncModeSnapshot) {
		t.Fatalf("ready payload = %#v, want snapshot barrier runtime-stream-1/94", ready)
	}
}

func TestReplaceOverviewStreamLegacyClientReceivesSnapshotAndNeverReplay(t *testing.T) {
	hub := NewHub()
	client := registerTestClient(hub)
	ackVersion := uint64(92)
	client.acceptHello(clientControlMessage{
		Type:            MessageTypeHello,
		RuntimeProtocol: 0,
		RuntimeCursor: RuntimeCursor{
			StreamID: "runtime-stream-1",
			Version:  ackVersion,
			Known:    true,
		},
	})

	snapshot := EmptySnapshot()
	if ok := hub.ReplaceOverviewStream(client, OverviewSyncBatch{
		Reason:          ResyncReasonClientResync,
		Mode:            OverviewSyncModeSnapshot,
		RuntimeStreamID: "runtime-stream-1",
		TargetVersion:   94,
		RuntimeIdentity: RuntimeIdentityForSnapshot(snapshot),
		Snapshot:        snapshot,
	}); !ok {
		t.Fatal("ReplaceOverviewStream returned false")
	}

	messages := drainOverviewTestMessages(t, client)
	assertOverviewTestMessageTypes(t, messages, MessageTypeResyncRequired, MessageTypeSnapshot, MessageTypeReady)
	for _, message := range messages {
		if message.Type == MessageTypeRuntimeReplay {
			t.Fatal("legacy client received runtime_replay")
		}
	}
}

func TestReplaceOverviewStreamProtocolV2ClientReceivesReplayWhenAvailable(t *testing.T) {
	hub := NewHub()
	client := registerTestClient(hub)
	ackVersion := uint64(92)
	client.acceptHello(clientControlMessage{
		Type:            MessageTypeHello,
		RuntimeProtocol: RuntimeStreamProtocolVersion,
		RuntimeCursor: RuntimeCursor{
			StreamID: "runtime-stream-1",
			Version:  ackVersion,
			Known:    true,
		},
	})

	replay := EmptyRuntimeDeltaPayload()
	replay.Devices["dev-1"] = map[string]any{"primary_health": "up_fresh"}
	if ok := hub.ReplaceOverviewStream(client, OverviewSyncBatch{
		Reason:          ResyncReasonClientResync,
		Mode:            OverviewSyncModeReplay,
		RuntimeStreamID: "runtime-stream-1",
		TargetVersion:   94,
		RuntimeIdentity: "rt-sha256:replayed",
		ReplayCursor:    RuntimeCursor{StreamID: "runtime-stream-1", Version: 92, Known: true},
		Replay:          replay,
	}); !ok {
		t.Fatal("ReplaceOverviewStream returned false")
	}

	messages := drainOverviewTestMessages(t, client)
	assertOverviewTestMessageTypes(t, messages, MessageTypeResyncRequired, MessageTypeRuntimeReplay, MessageTypeReady)
	var state RuntimeReplayMessagePayload
	decodeOverviewTestPayload(t, messages[1], &state)
	if state.FromVersion != 92 || state.Version != 94 || state.RuntimeStreamID != "runtime-stream-1" {
		t.Fatalf("replay state = %#v, want runtime-stream-1 92->94", state)
	}
}

func TestReplaceOverviewStreamClosedAndMarshalFailuresDoNotPartiallyQueue(t *testing.T) {
	t.Run("closed client", func(t *testing.T) {
		hub := NewHub()
		client := registerTestClient(hub)
		client.overviewSend <- []byte("stale")
		client.closed = true

		if ok := hub.ReplaceOverviewStream(client, OverviewSyncBatch{
			Mode:            OverviewSyncModeSnapshot,
			RuntimeStreamID: "runtime-stream-1",
			TargetVersion:   94,
			Snapshot:        EmptySnapshot(),
		}); ok {
			t.Fatal("ReplaceOverviewStream returned true for closed client")
		}
		if got := <-client.overviewSend; string(got) != "stale" {
			t.Fatalf("closed client mailbox = %q, want unchanged stale payload", string(got))
		}
	})

	t.Run("marshal failure", func(t *testing.T) {
		hub := NewHub()
		client := registerTestClient(hub)
		client.overviewSend <- []byte("stale")
		unsupported := math.NaN()
		snapshot := EmptySnapshot()
		snapshot.Devices["dev-1"] = DeviceRuntimeDTO{DeviceID: "dev-1", CPUPercent: &unsupported}

		if ok := hub.ReplaceOverviewStream(client, OverviewSyncBatch{
			Mode:            OverviewSyncModeSnapshot,
			RuntimeStreamID: "runtime-stream-1",
			TargetVersion:   94,
			Snapshot:        snapshot,
		}); ok {
			t.Fatal("ReplaceOverviewStream returned true for unmarshalable snapshot")
		}
		if got := <-client.overviewSend; string(got) != "stale" {
			t.Fatalf("marshal-failed mailbox = %q, want unchanged stale payload", string(got))
		}
	})
}

func TestReplaceOverviewStreamDoesNotInvokeMarshalCallbackWhileHoldingClientLock(t *testing.T) {
	hub := NewHub()
	client := registerTestClient(hub)
	client.acceptHello(clientControlMessage{
		Type:            MessageTypeHello,
		RuntimeProtocol: RuntimeStreamProtocolVersion,
		RuntimeCursor: RuntimeCursor{
			StreamID: "runtime-stream-1",
			Version:  92,
			Known:    true,
		},
	})
	replay := EmptyRuntimeDeltaPayload()
	replay.Devices["dev-1"] = map[string]any{
		"probe": overviewClientLockMarshalProbe{client: client},
	}

	done := make(chan bool, 1)
	go func() {
		done <- hub.ReplaceOverviewStream(client, OverviewSyncBatch{
			Reason:          ResyncReasonClientResync,
			Mode:            OverviewSyncModeReplay,
			RuntimeStreamID: "runtime-stream-1",
			TargetVersion:   94,
			ReplayCursor:    RuntimeCursor{StreamID: "runtime-stream-1", Version: 92, Known: true},
			Replay:          replay,
		})
	}()

	select {
	case ok := <-done:
		if !ok {
			t.Fatal("ReplaceOverviewStream returned false")
		}
	case <-time.After(time.Second):
		t.Fatal("ReplaceOverviewStream deadlocked while invoking MarshalJSON")
	}
}

type overviewClientLockMarshalProbe struct {
	client *Client
}

func (p overviewClientLockMarshalProbe) MarshalJSON() ([]byte, error) {
	p.client.mu.Lock()
	p.client.mu.Unlock()
	return []byte(`"marshaled"`), nil
}

func TestReplaceOverviewStreamRejectsReplayWhenSameStreamCursorAdvancesDuringMarshal(t *testing.T) {
	hub := NewHub()
	client := registerTestClient(hub)
	client.acceptHello(clientControlMessage{
		Type:            MessageTypeHello,
		RuntimeProtocol: RuntimeStreamProtocolVersion,
		RuntimeCursor: RuntimeCursor{
			StreamID: "runtime-stream-1",
			Version:  92,
			Known:    true,
		},
	})
	client.overviewSend <- []byte("stale")
	replay := EmptyRuntimeDeltaPayload()
	replay.Devices["dev-1"] = map[string]any{
		"selected_from": overviewClientCursorAdvanceMarshalProbe{
			client: client,
			cursor: RuntimeCursor{StreamID: "runtime-stream-1", Version: 93, Known: true},
		},
	}

	if ok := hub.ReplaceOverviewStream(client, OverviewSyncBatch{
		Reason:          ResyncReasonClientResync,
		Mode:            OverviewSyncModeReplay,
		RuntimeStreamID: "runtime-stream-1",
		TargetVersion:   94,
		ReplayCursor:    RuntimeCursor{StreamID: "runtime-stream-1", Version: 92, Known: true},
		Replay:          replay,
	}); ok {
		messages := drainOverviewTestMessages(t, client)
		var emitted RuntimeReplayMessagePayload
		decodeOverviewTestPayload(t, messages[1], &emitted)
		t.Fatalf(
			"ReplaceOverviewStream relabeled replay selected from 92 as %d->%d",
			emitted.FromVersion,
			emitted.Version,
		)
	}
	if got := <-client.overviewSend; string(got) != "stale" {
		t.Fatalf("rejected replay mailbox = %q, want unchanged stale payload", string(got))
	}
}

type overviewClientCursorAdvanceMarshalProbe struct {
	client *Client
	cursor RuntimeCursor
}

func (p overviewClientCursorAdvanceMarshalProbe) MarshalJSON() ([]byte, error) {
	p.client.acceptHello(clientControlMessage{
		Type:            MessageTypeHello,
		RuntimeProtocol: RuntimeStreamProtocolVersion,
		RuntimeCursor:   p.cursor,
	})
	return []byte(`"selected-from-92"`), nil
}

func TestReplaceOverviewStreamPendingRecoveryKeepsNewestCompleteBatch(t *testing.T) {
	hub := NewHub()
	client := registerTestClient(hub)
	ackVersion := uint64(92)
	client.acceptHello(clientControlMessage{
		Type:            MessageTypeHello,
		RuntimeProtocol: RuntimeStreamProtocolVersion,
		RuntimeCursor: RuntimeCursor{
			StreamID: "runtime-stream-1",
			Version:  ackVersion,
			Known:    true,
		},
	})

	oldSnapshot := EmptySnapshot()
	oldSnapshot.Devices["old"] = DeviceRuntimeDTO{DeviceID: "old"}
	if ok := hub.ReplaceOverviewStream(client, OverviewSyncBatch{
		Reason:          "old-recovery",
		Mode:            OverviewSyncModeSnapshot,
		RuntimeStreamID: "runtime-stream-1",
		TargetVersion:   94,
		Snapshot:        oldSnapshot,
	}); !ok {
		t.Fatal("old ReplaceOverviewStream returned false")
	}

	newReplay := EmptyRuntimeDeltaPayload()
	newReplay.Devices["new"] = map[string]any{"primary_health": "up_fresh"}
	if ok := hub.ReplaceOverviewStream(client, OverviewSyncBatch{
		Reason:          "new-recovery",
		Mode:            OverviewSyncModeReplay,
		RuntimeStreamID: "runtime-stream-1",
		TargetVersion:   95,
		ReplayCursor:    RuntimeCursor{StreamID: "runtime-stream-1", Version: 92, Known: true},
		Replay:          newReplay,
	}); !ok {
		t.Fatal("new ReplaceOverviewStream returned false")
	}

	messages := drainOverviewTestMessages(t, client)
	assertOverviewTestMessageTypes(t, messages, MessageTypeResyncRequired, MessageTypeRuntimeReplay, MessageTypeReady)
	serialized, err := json.Marshal(messages)
	if err != nil {
		t.Fatalf("marshal queued messages: %v", err)
	}
	if strings.Contains(string(serialized), "old-recovery") || strings.Contains(string(serialized), "\"old\"") {
		t.Fatalf("new recovery retained stale batch: %s", serialized)
	}
	if !strings.Contains(string(serialized), "new-recovery") || !strings.Contains(string(serialized), "\"new\"") {
		t.Fatalf("new recovery batch missing newest state: %s", serialized)
	}
}

func TestReplaceOverviewStreamRejectsOlderBatchAfterNewerRecovery(t *testing.T) {
	hub := NewHub()
	client := registerTestClient(hub)
	client.acceptHello(clientControlMessage{
		Type:            MessageTypeHello,
		RuntimeProtocol: RuntimeStreamProtocolVersion,
		RuntimeCursor: RuntimeCursor{
			StreamID: "runtime-stream-1",
			Version:  92,
			Known:    true,
		},
	})

	newReplay := EmptyRuntimeDeltaPayload()
	newReplay.Devices["new"] = map[string]any{"primary_health": "up_fresh"}
	if ok := hub.ReplaceOverviewStream(client, OverviewSyncBatch{
		Reason:          "new-recovery",
		Mode:            OverviewSyncModeReplay,
		RuntimeStreamID: "runtime-stream-1",
		TargetVersion:   95,
		ReplayCursor:    RuntimeCursor{StreamID: "runtime-stream-1", Version: 92, Known: true},
		Replay:          newReplay,
	}); !ok {
		t.Fatal("new ReplaceOverviewStream returned false")
	}

	if ok := hub.ReplaceOverviewStream(client, OverviewSyncBatch{
		Reason:          "stale-recovery",
		Mode:            OverviewSyncModeSnapshot,
		RuntimeStreamID: "runtime-stream-1",
		TargetVersion:   94,
		Snapshot:        EmptySnapshot(),
	}); ok {
		t.Fatal("older ReplaceOverviewStream replaced newer pending recovery")
	}

	messages := drainOverviewTestMessages(t, client)
	assertOverviewTestMessageTypes(t, messages, MessageTypeResyncRequired, MessageTypeRuntimeReplay, MessageTypeReady)
	var replay RuntimeReplayMessagePayload
	decodeOverviewTestPayload(t, messages[1], &replay)
	if replay.Version != 95 {
		t.Fatalf("retained replay target = %d, want newest target 95", replay.Version)
	}
}

func TestReplaceOverviewStreamReplayFromAckRemainsOrderedAfterIntermediateDelta(t *testing.T) {
	hub := NewHub()
	client := registerTestClient(hub)
	ackVersion := uint64(92)
	client.acceptHello(clientControlMessage{
		Type:            MessageTypeHello,
		RuntimeProtocol: RuntimeStreamProtocolVersion,
		RuntimeCursor: RuntimeCursor{
			StreamID: "runtime-stream-1",
			Version:  ackVersion,
			Known:    true,
		},
	})

	if overflowed := hub.BroadcastOverviewStreamDelta(EmptyRuntimeDeltaPayload(), 92, 93, "runtime-stream-1"); len(overflowed) != 0 {
		t.Fatalf("92->93 overflowed %d clients", len(overflowed))
	}
	intermediate := decodeOverviewTestMessage(t, <-client.overviewSend)
	if intermediate.Type != MessageTypeRuntimeDelta {
		t.Fatalf("intermediate message type = %q, want runtime_delta", intermediate.Type)
	}
	var intermediateDelta RuntimeDeltaMessagePayload
	decodeOverviewTestPayload(t, intermediate, &intermediateDelta)
	browserVersion := ackVersion
	if intermediateDelta.BaseVersion != browserVersion {
		t.Fatalf("intermediate delta base = %d, want browser version %d", intermediateDelta.BaseVersion, browserVersion)
	}
	browserVersion = intermediateDelta.Version
	if browserVersion != 93 {
		t.Fatalf("browser version after intermediate delta = %d, want 93", browserVersion)
	}
	// The browser is now at version 93 while its last acknowledged cursor
	// intentionally remains at version 92.
	if got := client.AckedRuntimeCursor(); got.Version != ackVersion {
		t.Fatalf("acknowledged cursor = %d, want stale version %d", got.Version, ackVersion)
	}

	if ok := hub.ReplaceOverviewStream(client, OverviewSyncBatch{
		Reason:          ResyncReasonClientResync,
		Mode:            OverviewSyncModeReplay,
		RuntimeStreamID: "runtime-stream-1",
		TargetVersion:   94,
		ReplayCursor:    RuntimeCursor{StreamID: "runtime-stream-1", Version: ackVersion, Known: true},
		Replay:          EmptyRuntimeDeltaPayload(),
	}); !ok {
		t.Fatal("ReplaceOverviewStream returned false")
	}
	if overflowed := hub.BroadcastOverviewStreamDelta(EmptyRuntimeDeltaPayload(), 94, 95, "runtime-stream-1"); len(overflowed) != 0 {
		t.Fatalf("94->95 overflowed %d clients after recovery replacement", len(overflowed))
	}

	messages := drainOverviewTestMessages(t, client)
	assertOverviewTestMessageTypes(t, messages, MessageTypeResyncRequired, MessageTypeRuntimeReplay, MessageTypeReady, MessageTypeRuntimeDelta)
	var replay RuntimeReplayMessagePayload
	decodeOverviewTestPayload(t, messages[1], &replay)
	if replay.FromVersion != ackVersion || replay.FromVersion >= browserVersion || replay.Version != 94 {
		t.Fatalf(
			"replay range = %d->%d with browser at %d, want overlapping 92->94",
			replay.FromVersion,
			replay.Version,
			browserVersion,
		)
	}
	resyncCount := 0
	for _, message := range messages {
		if message.Type == MessageTypeResyncRequired {
			resyncCount++
		}
	}
	if resyncCount != 1 {
		t.Fatalf("resync_required count = %d, want one complete recovery cycle", resyncCount)
	}
}

func TestReplaceOverviewStreamCurrentQueuesReadyOnly(t *testing.T) {
	hub := NewHub()
	client := registerTestClient(hub)
	client.acceptHello(clientControlMessage{
		Type:            MessageTypeHello,
		RuntimeProtocol: RuntimeStreamProtocolVersion,
		RuntimeCursor: RuntimeCursor{
			StreamID: "runtime-stream-1",
			Version:  94,
			Known:    true,
		},
	})

	if ok := hub.ReplaceOverviewStream(client, OverviewSyncBatch{
		Mode:            OverviewSyncModeCurrent,
		RuntimeStreamID: "runtime-stream-1",
		TargetVersion:   94,
		RuntimeIdentity: "rt-sha256:current",
		AlertVersion:    8,
	}); !ok {
		t.Fatal("ReplaceOverviewStream returned false")
	}

	messages := drainOverviewTestMessages(t, client)
	assertOverviewTestMessageTypes(t, messages, MessageTypeReady)
	var ready ReadyPayload
	decodeOverviewTestPayload(t, messages[0], &ready)
	if ready.SyncMode != string(OverviewSyncModeCurrent) || ready.RuntimeVersion != 94 || ready.AlertVersion != 8 {
		t.Fatalf("ready payload = %#v, want current runtime 94 alert 8", ready)
	}
}

func TestOverviewMailboxRecoveryFullReplacementIncludesBootstrappingClient(t *testing.T) {
	hub := NewHub()
	client := newObservedTestClient(hub)
	client.bootstrapping = true
	selection := client.beginBootstrapSnapshotSelection()
	hub.addClient(client)

	snapshot := EmptySnapshot()
	hub.ReplaceOverviewStreams(OverviewSyncBatch{
		Reason:          ResyncReasonClientResync,
		Mode:            OverviewSyncModeSnapshot,
		RuntimeStreamID: "runtime-stream-2",
		TargetVersion:   94,
		RuntimeIdentity: RuntimeIdentityForSnapshot(snapshot),
		Snapshot:        snapshot,
	})
	client.acceptHello(clientControlMessage{
		Type:            MessageTypeHello,
		RuntimeProtocol: RuntimeStreamProtocolVersion,
		RuntimeCursor: RuntimeCursor{
			StreamID: "runtime-stream-2",
			Version:  94,
			Known:    true,
		},
	})
	client.mu.Lock()
	pending := client.pendingOverviewRecovery
	client.mu.Unlock()
	if pending != nil {
		t.Fatalf("target ACK retained pending recovery: %#v", pending)
	}

	selection = client.markBootstrapSnapshotSelected(selection, 93, "rt-sha256:stale")
	if !selection.completeRecoveryQueued {
		t.Fatal("bootstrap selection lost ACK-cleared complete recovery generation")
	}
	if !client.overviewChangedSince(selection) {
		t.Fatal("bootstrap selection did not preserve the already-queued full-snapshot catch-up")
	}
	messages := drainOverviewTestMessages(t, client)
	assertOverviewTestMessageTypes(t, messages, MessageTypeResyncRequired, MessageTypeSnapshot, MessageTypeReady)
}

func TestOverviewMailboxRecoveryHandlerPreservesFullReplacementBeforeStaleHTTPMarker(t *testing.T) {
	hub := NewHub()
	go hub.Run()

	oldSnapshot := EmptySnapshot()
	oldSnapshot.Devices["dev-1"] = DeviceRuntimeDTO{DeviceID: "dev-1", PrimaryHealth: "degraded"}
	newSnapshot := EmptySnapshot()
	newSnapshot.Devices["dev-1"] = DeviceRuntimeDTO{DeviceID: "dev-1", PrimaryHealth: "up_fresh"}
	var snapshotMu sync.Mutex
	currentSnapshot := oldSnapshot
	currentVersion := uint64(92)
	snapshotFunc := func() (*SnapshotPayload, uint64) {
		snapshotMu.Lock()
		defer snapshotMu.Unlock()
		return currentSnapshot, currentVersion
	}

	bootstrapSelected := make(chan struct{})
	releaseBootstrap := make(chan struct{})
	var selectedOnce sync.Once
	var releaseOnce sync.Once
	release := func() {
		releaseOnce.Do(func() { close(releaseBootstrap) })
	}
	t.Cleanup(release)

	server := httptest.NewServer(NewHandler(
		hub,
		snapshotFunc,
		func() AlertMessagePayload {
			selectedOnce.Do(func() { close(bootstrapSelected) })
			<-releaseBootstrap
			return AlertMessagePayload{Alerts: []AlertDTO{}}
		},
		nil,
	))
	t.Cleanup(server.Close)

	params := url.Values{}
	params.Set("runtime_protocol", "2")
	params.Set("runtime_stream_id", "runtime-stream-1")
	params.Set("runtime_version", "91")
	conn, _, err := websocket.DefaultDialer.Dial(
		"ws"+strings.TrimPrefix(server.URL, "http")+"?"+params.Encode(),
		nil,
	)
	if err != nil {
		t.Fatalf("dial gated bootstrap client: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	select {
	case <-bootstrapSelected:
	case <-time.After(time.Second):
		t.Fatal("handler did not reach post-selection bootstrap gate")
	}

	snapshotMu.Lock()
	currentSnapshot = newSnapshot
	currentVersion = 93
	snapshotMu.Unlock()
	hub.ReplaceOverviewStreams(OverviewSyncBatch{
		Reason:          ResyncReasonClientResync,
		Mode:            OverviewSyncModeSnapshot,
		RuntimeStreamID: "runtime-stream-2",
		TargetVersion:   93,
		RuntimeIdentity: RuntimeIdentityForSnapshot(newSnapshot),
		Snapshot:        newSnapshot,
	})
	clients := hub.copyOverviewClients()
	if len(clients) != 1 {
		t.Fatalf("overview client count = %d, want 1", len(clients))
	}
	if err := conn.WriteJSON(map[string]any{
		"type": MessageTypeHello,
		"payload": map[string]any{
			"runtime_protocol":  RuntimeStreamProtocolVersion,
			"runtime_stream_id": "runtime-stream-2",
			"runtime_version":   93,
		},
	}); err != nil {
		t.Fatalf("acknowledge queued bootstrap replacement: %v", err)
	}
	deadline := time.Now().Add(time.Second)
	for clients[0].AckedRuntimeCursor().Version != 93 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	clients[0].mu.Lock()
	pending := clients[0].pendingOverviewRecovery
	queued := len(clients[0].overviewSend)
	clients[0].mu.Unlock()
	if pending != nil || queued != 3 {
		t.Fatalf("acknowledged replacement pending=%#v queued=%d, want nil pending and complete queued batch", pending, queued)
	}
	release()

	if err := conn.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatalf("set bootstrap recovery read deadline: %v", err)
	}
	defer conn.SetReadDeadline(time.Time{})
	runtimeMessages := make([]overviewTestMessage, 0, 3)
	for len(runtimeMessages) < 4 {
		var message overviewTestMessage
		if err := conn.ReadJSON(&message); err != nil {
			t.Fatalf("read bootstrap recovery message: %v", err)
		}
		if message.Type == MessageTypeAlert {
			continue
		}
		runtimeMessages = append(runtimeMessages, message)
		if message.Type == MessageTypeReady {
			break
		}
	}
	assertOverviewTestMessageTypes(
		t,
		runtimeMessages,
		MessageTypeResyncRequired,
		MessageTypeSnapshot,
		MessageTypeReady,
	)
	var snapshot SnapshotMessagePayload
	decodeOverviewTestPayload(t, runtimeMessages[1], &snapshot)
	if snapshot.Version != 93 || snapshot.RuntimeStreamID != "runtime-stream-2" {
		t.Fatalf("bootstrap recovery snapshot = %#v, want runtime-stream-2/93", snapshot)
	}
}

func TestOverviewMailboxRecoveryHandlerMarkerClearsSparseDeltaWithoutCompleteReplacement(t *testing.T) {
	hub := NewHub()
	go hub.Run()

	snapshot := EmptySnapshot()
	snapshot.Devices["dev-1"] = DeviceRuntimeDTO{DeviceID: "dev-1", PrimaryHealth: "degraded"}
	var snapshotMu sync.Mutex
	currentVersion := uint64(92)
	snapshotFunc := func() (*SnapshotPayload, uint64) {
		snapshotMu.Lock()
		defer snapshotMu.Unlock()
		return snapshot, currentVersion
	}

	bootstrapSelected := make(chan struct{})
	releaseBootstrap := make(chan struct{})
	var selectedOnce sync.Once
	var releaseOnce sync.Once
	release := func() {
		releaseOnce.Do(func() { close(releaseBootstrap) })
	}
	t.Cleanup(release)
	server := httptest.NewServer(NewHandler(
		hub,
		snapshotFunc,
		func() AlertMessagePayload {
			selectedOnce.Do(func() { close(bootstrapSelected) })
			<-releaseBootstrap
			return AlertMessagePayload{Alerts: []AlertDTO{}}
		},
		nil,
	))
	t.Cleanup(server.Close)

	params := url.Values{}
	params.Set("runtime_protocol", "2")
	params.Set("runtime_stream_id", "runtime-stream-1")
	params.Set("runtime_version", "91")
	conn, _, err := websocket.DefaultDialer.Dial(
		"ws"+strings.TrimPrefix(server.URL, "http")+"?"+params.Encode(),
		nil,
	)
	if err != nil {
		t.Fatalf("dial sparse-delta bootstrap client: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	select {
	case <-bootstrapSelected:
	case <-time.After(time.Second):
		t.Fatal("handler did not reach sparse-delta bootstrap gate")
	}
	snapshotMu.Lock()
	currentVersion = 93
	snapshotMu.Unlock()
	if overflowed := hub.BroadcastOverviewStreamDelta(
		EmptyRuntimeDeltaPayload(),
		92,
		93,
		"runtime-stream-1",
	); len(overflowed) != 0 {
		t.Fatalf("sparse bootstrap delta overflowed %d clients", len(overflowed))
	}
	release()

	if err := conn.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatalf("set sparse bootstrap read deadline: %v", err)
	}
	defer conn.SetReadDeadline(time.Time{})
	for {
		var message overviewTestMessage
		if err := conn.ReadJSON(&message); err != nil {
			t.Fatalf("read sparse bootstrap message: %v", err)
		}
		if message.Type == MessageTypeAlert {
			continue
		}
		if message.Type != MessageTypeResyncRequired {
			t.Fatalf("sparse bootstrap message type = %q, want resync_required", message.Type)
		}
		break
	}
}

func TestOverviewMailboxRecoveryHTTPMarkerDecisionPreservesConcurrentReplacement(t *testing.T) {
	hub := NewHub()
	client := registerTestClient(hub)
	marshalEntered := make(chan struct{})
	releaseMarshal := make(chan struct{})
	var releaseOnce sync.Once
	release := func() {
		releaseOnce.Do(func() { close(releaseMarshal) })
	}
	t.Cleanup(release)
	probe := &overviewHTTPMarkerMarshalGate{
		entered: marshalEntered,
		release: releaseMarshal,
	}
	type markerResult struct {
		written bool
		ok      bool
	}
	result := make(chan markerResult, 1)
	selection := client.beginBootstrapSnapshotSelection()
	selection = client.markBootstrapSnapshotSelected(selection, 0, "")
	go func() {
		written, ok := hub.writeHTTPRuntimeResync(client, Message{
			Type:    MessageTypeResyncRequired,
			Payload: probe,
		}, selection)
		result <- markerResult{written: written, ok: ok}
	}()

	select {
	case <-marshalEntered:
	case <-time.After(time.Second):
		t.Fatal("HTTP marker did not reach marshal gate")
	}
	snapshot := EmptySnapshot()
	if ok := hub.ReplaceOverviewStream(client, OverviewSyncBatch{
		Reason:          ResyncReasonClientResync,
		Mode:            OverviewSyncModeSnapshot,
		RuntimeStreamID: "runtime-stream-2",
		TargetVersion:   93,
		RuntimeIdentity: RuntimeIdentityForSnapshot(snapshot),
		Snapshot:        snapshot,
	}); !ok {
		t.Fatal("concurrent complete replacement returned false")
	}
	release()

	select {
	case got := <-result:
		if got.written || !got.ok {
			t.Fatalf("HTTP marker result = %#v, want skipped successful write", got)
		}
	case <-time.After(time.Second):
		t.Fatal("HTTP marker decision did not finish")
	}
	assertOverviewTestMessageTypes(
		t,
		drainOverviewTestMessages(t, client),
		MessageTypeResyncRequired,
		MessageTypeSnapshot,
		MessageTypeReady,
	)
}

type overviewHTTPMarkerMarshalGate struct {
	once    sync.Once
	entered chan struct{}
	release chan struct{}
}

func (p *overviewHTTPMarkerMarshalGate) MarshalJSON() ([]byte, error) {
	p.once.Do(func() { close(p.entered) })
	<-p.release
	return []byte(`{"scope":"overview","reason":"gated"}`), nil
}

type overviewTestMessage struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

func drainOverviewTestMessages(t *testing.T, client *Client) []overviewTestMessage {
	t.Helper()
	messages := make([]overviewTestMessage, 0, len(client.overviewSend))
	for len(client.overviewSend) > 0 {
		messages = append(messages, decodeOverviewTestMessage(t, <-client.overviewSend))
	}
	return messages
}

func decodeOverviewTestMessage(t *testing.T, raw []byte) overviewTestMessage {
	t.Helper()
	var message overviewTestMessage
	if err := json.Unmarshal(raw, &message); err != nil {
		t.Fatalf("decode overview message: %v", err)
	}
	return message
}

func decodeOverviewTestPayload(t *testing.T, message overviewTestMessage, target any) {
	t.Helper()
	if err := json.Unmarshal(message.Payload, target); err != nil {
		t.Fatalf("decode %s payload: %v", message.Type, err)
	}
}

func assertOverviewTestMessageTypes(t *testing.T, messages []overviewTestMessage, want ...string) {
	t.Helper()
	if len(messages) != len(want) {
		t.Fatalf("overview message count = %d, want %d", len(messages), len(want))
	}
	for index := range want {
		if messages[index].Type != want[index] {
			t.Fatalf("overview message %d type = %q, want %q", index, messages[index].Type, want[index])
		}
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

func assertNoNoClientOverviewBroadcastEffects(t *testing.T, hub *Hub, metrics []byte, logs string) {
	t.Helper()

	if messages := drainHubRecordedBroadcasts(hub); len(messages) != 0 {
		t.Fatalf("expected no recorded overview broadcast messages, got %d", len(messages))
	}
	if strings.Contains(string(metrics), `theia_ws_messages_total{scope="broadcast"`) {
		t.Fatalf("expected no broadcast message metric, got:\n%s", string(metrics))
	}
	if strings.Contains(logs, "failed to marshal overview") {
		t.Fatalf("expected no overview marshal error log, got:\n%s", logs)
	}
}

func drainHubRecordedBroadcasts(hub *Hub) [][]byte {
	var messages [][]byte
	for {
		select {
		case message := <-hub.BroadcastCh():
			messages = append(messages, message)
		default:
			return messages
		}
	}
}
