package ws

import (
	"log"
	"net/http"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

// Handler upgrades HTTP requests and binds them to the hub.
type Handler struct {
	hub            *Hub
	snapshotFunc   func() (*SnapshotPayload, uint64)
	alertsFunc     func() AlertMessagePayload
	promStatusFunc func() PrometheusStatusPayload
}

// NewHandler creates a WebSocket handler that serves initial snapshots on connect.
// promStatusFunc returns the current Prometheus integration status.
func NewHandler(hub *Hub, snapshotFunc func() (*SnapshotPayload, uint64), alertsFunc func() AlertMessagePayload, promStatusFunc func() PrometheusStatusPayload) *Handler {
	return &Handler{
		hub:            hub,
		snapshotFunc:   snapshotFunc,
		alertsFunc:     alertsFunc,
		promStatusFunc: promStatusFunc,
	}
}

// ServeHTTP upgrades the request, registers the client, sends the initial snapshot, and starts pumps.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade failed: %v", err)
		return
	}

	client := &Client{
		hub:          h.hub,
		conn:         conn,
		send:         make(chan []byte, sendBufferSize),
		overviewSend: make(chan []byte, overviewBufferSize),
	}

	h.hub.register <- client

	snapshot := EmptySnapshot()
	version := uint64(0)
	if h.snapshotFunc != nil {
		snapshot, version = h.snapshotFunc()
		snapshot = CloneSnapshot(snapshot)
	}
	h.hub.SendOverviewSnapshot(client, snapshot, version)

	alerts := AlertMessagePayload{Alerts: []AlertDTO{}}
	if h.alertsFunc != nil {
		alerts = h.alertsFunc()
	}
	h.hub.SendTo(client, NewAlertMessage(alerts.Alerts, alerts.Version))

	// Send current Prometheus status so the client doesn't have to wait for
	// the next health-check transition to learn Prometheus is unreachable.
	// Disabled Prometheus is treated as "no status" to preserve SNMP-only mode.
	if h.promStatusFunc != nil {
		status := h.promStatusFunc()
		if status.Enabled {
			h.hub.SendTo(client, Message{
				Type:    MessageTypePrometheusStatus,
				Payload: status,
			})
		}
	}

	go client.writePump()
	client.readPump()
}
