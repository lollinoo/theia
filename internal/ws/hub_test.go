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

	done := make(chan struct{})
	go func() {
		defer close(done)
		hub.Broadcast(Message{Type: MessageTypeSnapshot, Payload: EmptySnapshot()})
	}()

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		metrics := string(registry.MarshalPrometheus())
		if strings.Contains(metrics, `theia_ws_backpressure_total{reason="hub_buffer_full",scope="broadcast"} 1`) {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	<-hub.broadcast

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Broadcast did not unblock after draining the hub buffer")
	}

	metrics := string(registry.MarshalPrometheus())
	if !strings.Contains(metrics, `theia_ws_backpressure_total{reason="hub_buffer_full",scope="broadcast"} 1`) {
		t.Fatalf("expected hub buffer backpressure metric, got:\n%s", metrics)
	}
}

func TestHubConsumeBroadcastOverflow_IsStickyUntilConsumed(t *testing.T) {
	hub := NewHub()
	for i := 0; i < cap(hub.broadcast); i++ {
		hub.broadcast <- []byte("prefill")
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		hub.Broadcast(Message{Type: MessageTypeSnapshot, Payload: EmptySnapshot()})
	}()

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		hub.mu.RLock()
		overflowed := hub.broadcastOverflow
		hub.mu.RUnlock()
		if overflowed {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	<-hub.broadcast
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Broadcast did not unblock after draining the hub buffer")
	}

	if !hub.ConsumeBroadcastOverflow() {
		t.Fatal("expected broadcast overflow marker after hub buffer backpressure")
	}
	if hub.ConsumeBroadcastOverflow() {
		t.Fatal("expected broadcast overflow marker to clear after consumption")
	}

	client := registerTestClient(hub)
	client.send <- []byte("occupied")
	if ok := hub.enqueue(client, []byte("blocked")); ok {
		t.Fatal("expected client-send backpressure to remain isolated")
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
		hub:  hub,
		send: make(chan []byte, 1),
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
