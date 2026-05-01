package ws

import (
	"log"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
)

const bootstrapHelloWait = 500 * time.Millisecond

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

// ServeHTTP upgrades the request, sends the bootstrap state, then registers the
// client for live broadcasts. This keeps runtime deltas from racing ahead of
// the initial ready/snapshot message.
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
		hello:        make(chan clientControlMessage, clientHelloBuffer),
		disconnected: make(chan struct{}),
	}

	h.hub.register <- client
	go client.readPump()

	hello, hasHello, connected := waitForClientHello(client)
	if !connected {
		return
	}

	snapshot := EmptySnapshot()
	version := uint64(0)
	if h.snapshotFunc != nil {
		snapshot, version = h.snapshotFunc()
		snapshot = CloneSnapshot(snapshot)
	}
	runtimeIdentity := RuntimeIdentityForSnapshot(snapshot)

	alerts := AlertMessagePayload{Alerts: []AlertDTO{}}
	if h.alertsFunc != nil {
		alerts = h.alertsFunc()
	}

	var bootstrapMessage Message
	if hasHello && clientRuntimeCurrent(hello, version, runtimeIdentity) {
		bootstrapMessage = NewReadyMessage(version, alerts.Version, runtimeIdentity)
	} else if hasHello {
		bootstrapMessage = Message{
			Type: MessageTypeResyncRequired,
			Payload: ResyncRequiredPayload{
				Scope:  ResyncScopeOverview,
				Reason: ResyncReasonClientMissingRuntimeSnapshot,
			},
		}
	} else {
		bootstrapMessage = NewSnapshotMessage(snapshot, version)
	}

	if !h.hub.WriteTo(client, bootstrapMessage) {
		return
	}
	if !h.hub.WriteTo(client, NewAlertMessage(alerts.Alerts, alerts.Version)) {
		return
	}

	// Send current Prometheus status so the client doesn't have to wait for
	// the next health-check transition to learn Prometheus is unreachable.
	// Disabled Prometheus is treated as "no status" to preserve SNMP-only mode.
	if h.promStatusFunc != nil {
		status := h.promStatusFunc()
		if status.Enabled {
			if !h.hub.WriteTo(client, Message{
				Type:    MessageTypePrometheusStatus,
				Payload: status,
			}) {
				return
			}
		}
	}

	select {
	case <-client.disconnected:
		return
	default:
	}
	go client.writePump()
}

func clientRuntimeCurrent(hello clientControlMessage, runtimeVersion uint64, runtimeIdentity string) bool {
	if hello.RuntimeVersion != nil && *hello.RuntimeVersion == runtimeVersion {
		return true
	}
	return hello.RuntimeIdentity != "" && hello.RuntimeIdentity == runtimeIdentity
}

func waitForClientHello(client *Client) (clientControlMessage, bool, bool) {
	if client.hello == nil {
		return clientControlMessage{}, false, true
	}

	timer := time.NewTimer(bootstrapHelloWait)
	defer timer.Stop()

	select {
	case hello := <-client.hello:
		return hello, true, true
	case <-client.disconnected:
		return clientControlMessage{}, false, false
	case <-timer.C:
		return clientControlMessage{}, false, true
	}
}
