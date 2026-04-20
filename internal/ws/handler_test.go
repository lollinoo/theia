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
