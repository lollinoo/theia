package ws

// This file exercises hub stress behavior so refactors preserve the documented contract.

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/observability"
)

func TestHubProductionScaleFanoutSlowClientsStayBoundedAndObservable(t *testing.T) {
	productionHub := NewHub()
	productionSlowClient := registerStressClient(productionHub)
	fillStressChannel(productionSlowClient.send)

	productionHub.Broadcast(Message{
		Type: MessageTypePrometheusStatus,
		Payload: PrometheusStatusPayload{
			Enabled:   true,
			Available: true,
		},
	})

	if productionHub.broadcast != nil {
		t.Fatal("default production hub initialized a broadcast recorder under slow-client pressure")
	}

	registry := observability.ResetDefaultForTest()
	t.Cleanup(func() {
		observability.ResetDefaultForTest()
	})

	hub := NewHub(WithBroadcastRecorder())
	recorder := hub.BroadcastCh()
	rounds := cap(recorder) + sendBufferSize

	const slowLegacyClients = 48
	const slowHTTPClients = 48
	const fastClients = 12

	legacySlow := make([]*Client, 0, slowLegacyClients)
	for i := 0; i < slowLegacyClients; i++ {
		client := registerStressClient(hub)
		fillStressChannel(client.send)
		fillStressChannel(client.overviewSend)
		legacySlow = append(legacySlow, client)
	}

	httpSlow := make([]*Client, 0, slowHTTPClients)
	for i := 0; i < slowHTTPClients; i++ {
		client := registerStressClient(hub)
		client.usesHTTPRuntimeBootstrap = true
		fillStressChannel(client.send)
		fillStressChannel(client.overviewSend)
		httpSlow = append(httpSlow, client)
	}

	fast := make([]*Client, 0, fastClients)
	for i := 0; i < fastClients; i++ {
		fast = append(fast, registerStressClient(hub))
	}

	for round := 0; round < rounds; round++ {
		version := uint64(round + 1)
		hub.Broadcast(Message{
			Type: MessageTypePrometheusStatus,
			Payload: PrometheusStatusPayload{
				Enabled:   true,
				Available: true,
			},
		})
		hub.BroadcastOverviewDelta(EmptyRuntimeDeltaPayload(), version, version+1, stressFallbackSnapshot())

		for _, client := range fast {
			assertStressMessageType(t, client.send, MessageTypePrometheusStatus)
			assertStressMessageType(t, client.overviewSend, MessageTypeRuntimeDelta)
		}
	}

	if got, want := len(recorder), cap(recorder); got != want {
		t.Fatalf("recorded broadcast payloads = %d, want bounded recorder capacity %d", got, want)
	}
	assertRecorderContainsOnlyFanoutPayloads(t, recorder)

	for _, client := range legacySlow {
		if got, want := len(client.send), cap(client.send); got != want {
			t.Fatalf("legacy slow send mailbox length = %d, want %d", got, want)
		}
		if got, want := len(client.overviewSend), cap(client.overviewSend); got == 0 || got > want {
			t.Fatalf("legacy slow overview mailbox length = %d, want bounded non-empty mailbox at or below %d", got, want)
		}
		assertStressMessageType(t, client.overviewSend, MessageTypeResyncRequired)
		assertStressMessageType(t, client.overviewSend, MessageTypeSnapshot)
		if client.needsResync {
			t.Fatal("legacy slow client remained marked for resync after fallback snapshot")
		}
	}

	for _, client := range httpSlow {
		if got, want := len(client.send), cap(client.send); got != want {
			t.Fatalf("HTTP slow send mailbox length = %d, want %d", got, want)
		}
		if got := len(client.overviewSend); got != 1 {
			t.Fatalf("HTTP slow overview mailbox length = %d, want one pending resync marker", got)
		}
		assertStressMessageType(t, client.overviewSend, MessageTypeResyncRequired)
		if !client.needsResync {
			t.Fatal("HTTP slow client did not remain marked for HTTP resync")
		}
	}

	metrics := string(registry.MarshalPrometheus())
	legacyResyncsPerClient := 1 + (rounds-1)/(overviewBufferSize-1)
	assertStressMetric(t, metrics, fmt.Sprintf(
		`theia_ws_backpressure_total{reason="client_buffer_full",scope="client_send"} %d`,
		(slowLegacyClients+slowHTTPClients)*rounds,
	))
	assertStressMetric(t, metrics, fmt.Sprintf(
		`theia_ws_backpressure_total{reason="client_mailbox_full",scope="overview_send"} %d`,
		slowLegacyClients*legacyResyncsPerClient+slowHTTPClients*rounds,
	))
	assertStressMetric(t, metrics, fmt.Sprintf(
		`theia_ws_backpressure_total{reason="hub_buffer_full",scope="broadcast"} %d`,
		rounds*2-cap(recorder),
	))
	assertStressMetric(t, metrics, fmt.Sprintf(
		`theia_ws_client_resync_required_total{bootstrap="legacy",reason="client_resync_scheduled",scope="overview"} %d`,
		slowLegacyClients*legacyResyncsPerClient,
	))
	assertStressMetric(t, metrics, fmt.Sprintf(
		`theia_ws_client_resync_required_total{bootstrap="http",reason="client_resync_scheduled",scope="overview"} %d`,
		slowHTTPClients,
	))
	assertStressMetric(t, metrics, fmt.Sprintf(
		`theia_ws_overview_resync_suppressed_total{reason="client_resync_scheduled"} %d`,
		slowHTTPClients*(rounds-1),
	))
}

func TestHubBroadcastMarksLegacyClientForResyncWhenMailboxIsFull(t *testing.T) {
	hub := NewHub()
	client := registerTestClient(hub)
	for i := 0; i < cap(client.overviewSend); i++ {
		client.overviewSend <- []byte("occupied")
	}

	fallback := EmptySnapshot()
	fallback.DeviceStatuses["dev-1"] = "up"

	hub.BroadcastOverviewDelta(EmptyRuntimeDeltaPayload(), 5, 6, fallback)

	if got := len(client.overviewSend); got != 2 {
		t.Fatalf("overview mailbox length = %d, want 2", got)
	}
	if client.needsResync {
		t.Fatal("expected client resync flag to clear after fallback snapshot enqueue")
	}

	first := decodeHubStressMessage(t, <-client.overviewSend)
	second := decodeHubStressMessage(t, <-client.overviewSend)

	if first.Type != MessageTypeResyncRequired {
		t.Fatalf("first message type = %q, want %q", first.Type, MessageTypeResyncRequired)
	}
	if second.Type != MessageTypeSnapshot {
		t.Fatalf("second message type = %q, want %q", second.Type, MessageTypeSnapshot)
	}

	var resyncPayload ResyncRequiredPayload
	if err := json.Unmarshal(first.Payload, &resyncPayload); err != nil {
		t.Fatalf("decode resync payload: %v", err)
	}
	if resyncPayload.Scope != ResyncScopeOverview {
		t.Fatalf("resync scope = %q, want %q", resyncPayload.Scope, ResyncScopeOverview)
	}
	if resyncPayload.Reason != ResyncReasonClientResync {
		t.Fatalf("resync reason = %q, want %q", resyncPayload.Reason, ResyncReasonClientResync)
	}
}

func TestHubBroadcastAvoidsSnapshotForHTTPBootstrapClientWhenMailboxIsFull(t *testing.T) {
	hub := NewHub()
	client := registerTestClient(hub)
	client.usesHTTPRuntimeBootstrap = true
	for i := 0; i < cap(client.overviewSend); i++ {
		client.overviewSend <- []byte("occupied")
	}

	fallback := EmptySnapshot()
	fallback.DeviceStatuses["dev-1"] = "up"

	hub.BroadcastOverviewDelta(EmptyRuntimeDeltaPayload(), 5, 6, fallback)

	if got := len(client.overviewSend); got != 1 {
		t.Fatalf("overview mailbox length = %d, want 1", got)
	}
	if !client.needsResync {
		t.Fatal("expected client to remain marked for HTTP resync")
	}

	first := decodeHubStressMessage(t, <-client.overviewSend)
	if first.Type != MessageTypeResyncRequired {
		t.Fatalf("message type = %q, want %q", first.Type, MessageTypeResyncRequired)
	}
}

func TestHubRepeatedDetailSubscriptionsConvergeToSingleTarget(t *testing.T) {
	hub := NewHub()
	client := registerTestClient(hub)
	firstDeviceID := uuid.New()
	secondDeviceID := uuid.New()

	hub.SetDetailSubscription(client, firstDeviceID)
	hub.SetDetailSubscription(client, secondDeviceID)
	hub.SetDetailSubscription(client, firstDeviceID)
	hub.SetDetailSubscription(client, firstDeviceID)

	if subscribers := hub.DetailSubscribers(secondDeviceID); len(subscribers) != 0 {
		t.Fatalf("second target subscribers = %d, want 0", len(subscribers))
	}

	subscribers := hub.DetailSubscribers(firstDeviceID)
	if len(subscribers) != 1 {
		t.Fatalf("first target subscribers = %d, want 1", len(subscribers))
	}
	if subscribers[0] != client {
		t.Fatal("expected repeated subscriptions to keep the final client target only")
	}
}

type hubStressMessage struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

func decodeHubStressMessage(t *testing.T, raw []byte) hubStressMessage {
	t.Helper()

	var message hubStressMessage
	if err := json.Unmarshal(raw, &message); err != nil {
		t.Fatalf("decode hub stress message: %v", err)
	}
	return message
}

func registerStressClient(hub *Hub) *Client {
	client := &Client{
		hub:          hub,
		send:         make(chan []byte, sendBufferSize),
		overviewSend: make(chan []byte, overviewBufferSize),
	}

	hub.mu.Lock()
	hub.clients[client] = true
	hub.mu.Unlock()

	return client
}

func fillStressChannel(ch chan []byte) {
	for len(ch) < cap(ch) {
		ch <- []byte("occupied")
	}
}

func stressFallbackSnapshot() *SnapshotPayload {
	snapshot := EmptySnapshot()
	snapshot.DeviceStatuses["stress-device"] = "up"
	return snapshot
}

func assertStressMessageType(t *testing.T, ch chan []byte, want string) {
	t.Helper()

	select {
	case raw := <-ch:
		message := decodeHubStressMessage(t, raw)
		if message.Type != want {
			t.Fatalf("message type = %q, want %q", message.Type, want)
		}
	default:
		t.Fatalf("missing queued %s message", want)
	}
}

func assertRecorderContainsOnlyFanoutPayloads(t *testing.T, recorder chan []byte) {
	t.Helper()

	seen := map[string]int{}
	for len(recorder) > 0 {
		message := decodeHubStressMessage(t, <-recorder)
		seen[message.Type]++
		switch message.Type {
		case MessageTypePrometheusStatus, MessageTypeRuntimeDelta:
		default:
			t.Fatalf("recorder retained %q payload under fanout pressure", message.Type)
		}
	}

	if seen[MessageTypePrometheusStatus] == 0 {
		t.Fatal("recorder did not retain any regular broadcast payloads")
	}
	if seen[MessageTypeRuntimeDelta] == 0 {
		t.Fatal("recorder did not retain any overview delta payloads")
	}
}

func assertStressMetric(t *testing.T, metrics, want string) {
	t.Helper()

	if !strings.Contains(metrics, want) {
		t.Fatalf("expected metric %q, got:\n%s", want, metrics)
	}
}
