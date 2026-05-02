package ws

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
)

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
