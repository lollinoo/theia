package ws

import (
	"testing"

	"github.com/google/uuid"
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
