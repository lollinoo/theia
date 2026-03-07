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
	hub          *Hub
	snapshotFunc func() *SnapshotPayload
}

// NewHandler creates a WebSocket handler that serves initial snapshots on connect.
func NewHandler(hub *Hub, snapshotFunc func() *SnapshotPayload) *Handler {
	return &Handler{
		hub:          hub,
		snapshotFunc: snapshotFunc,
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
		hub:  h.hub,
		conn: conn,
		send: make(chan []byte, sendBufferSize),
	}

	h.hub.register <- client

	snapshot := EmptySnapshot()
	if h.snapshotFunc != nil {
		snapshot = CloneSnapshot(h.snapshotFunc())
	}
	h.hub.SendTo(client, Message{
		Type:    MessageTypeSnapshot,
		Payload: snapshot,
	})

	go client.writePump()
	client.readPump()
}
