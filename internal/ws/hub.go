package ws

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/lollinoo/theia/internal/observability"
)

const (
	writeWait          = 60 * time.Second
	pongWait           = 60 * time.Second
	pingPeriod         = 54 * time.Second
	maxMessageSize     = 4096
	sendBufferSize     = 16
	overviewBufferSize = 2
	clientHelloBuffer  = 1

	wsBackpressureScopeBroadcast    = "broadcast"
	wsBackpressureScopeClientSend   = "client_send"
	wsBackpressureScopeOverviewSend = "overview_send"

	wsBackpressureReasonHubBufferFull     = "hub_buffer_full"
	wsBackpressureReasonClientBufferFull  = "client_buffer_full"
	wsBackpressureReasonClientMailboxFull = "client_mailbox_full"
)

// Hub manages all active WebSocket clients and server-side broadcasts.
type Hub struct {
	clients    map[*Client]bool
	broadcast  chan []byte
	register   chan *Client
	unregister chan *Client
	mu         sync.RWMutex
}

// Client is a single WebSocket connection.
type Client struct {
	hub            *Hub
	conn           *websocket.Conn
	mu             sync.Mutex
	closed         bool
	disconnected   chan struct{}
	disconnectOnce sync.Once
	send           chan []byte
	overviewSend   chan []byte
	hello          chan clientControlMessage
	needsResync    bool
	detailDeviceID uuid.UUID
}

// NewHub creates an empty WebSocket hub.
func NewHub() *Hub {
	return &Hub{
		clients:    make(map[*Client]bool),
		broadcast:  make(chan []byte, 32),
		register:   make(chan *Client, 32),
		unregister: make(chan *Client, 32),
	}
}

// Run processes hub registration, unregistration, and broadcast events.
func (h *Hub) Run() {
	for {
		select {
		case client := <-h.register:
			h.mu.Lock()
			h.clients[client] = true
			h.mu.Unlock()
		case client := <-h.unregister:
			h.removeClient(client)
		}
	}
}

// Broadcast serializes a message and sends it to all connected clients.
func (h *Hub) Broadcast(msg Message) {
	payload, err := json.Marshal(msg)
	if err != nil {
		log.Printf("WebSocket hub: failed to marshal broadcast message: %v", err)
		return
	}
	observability.Default().ObserveWSMessage("broadcast", msg.Type, len(payload))
	h.recordBroadcast(payload)
	for _, client := range h.copyClients() {
		h.enqueue(client, payload)
	}
}

// SendTo serializes a message and queues it for a single client.
func (h *Hub) SendTo(client *Client, msg Message) {
	payload, err := json.Marshal(msg)
	if err != nil {
		log.Printf("WebSocket hub: failed to marshal client message: %v", err)
		return
	}
	observability.Default().ObserveWSMessage("unicast", msg.Type, len(payload))
	h.enqueue(client, payload)
}

// WriteTo serializes and writes a message directly to a client. It must only be
// used before the client's write pump starts.
func (h *Hub) WriteTo(client *Client, msg Message) bool {
	payload, err := json.Marshal(msg)
	if err != nil {
		log.Printf("WebSocket hub: failed to marshal direct client message: %v", err)
		return false
	}
	observability.Default().ObserveWSMessage("unicast", msg.Type, len(payload))
	return client.writePayload(payload, true)
}

// BroadcastOverviewSnapshot broadcasts a versioned full overview snapshot.
func (h *Hub) BroadcastOverviewSnapshot(snapshot *SnapshotPayload, version uint64) {
	msg := NewSnapshotMessage(snapshot, version)
	payload, err := json.Marshal(msg)
	if err != nil {
		log.Printf("WebSocket hub: failed to marshal overview snapshot: %v", err)
		return
	}
	observability.Default().ObserveWSMessage("broadcast", msg.Type, len(payload))
	h.recordBroadcast(payload)
	for _, client := range h.copyClients() {
		h.enqueueOverviewSnapshot(client, payload)
	}
}

// BroadcastOverviewDelta broadcasts a versioned sparse overview delta.
// If a client cannot keep up, it receives resync_required plus the supplied
// fallback full snapshot instead of blocking the producer.
func (h *Hub) BroadcastOverviewDelta(delta *RuntimeDeltaPayload, baseVersion, version uint64, fallbackSnapshot *SnapshotPayload) {
	deltaMessage := NewRuntimeDeltaMessage(delta, baseVersion, version)
	deltaPayload, err := json.Marshal(deltaMessage)
	if err != nil {
		log.Printf("WebSocket hub: failed to marshal overview delta: %v", err)
		return
	}
	fallbackMessage := NewSnapshotMessage(fallbackSnapshot, version)
	fallbackPayload, err := json.Marshal(fallbackMessage)
	if err != nil {
		log.Printf("WebSocket hub: failed to marshal overview fallback snapshot: %v", err)
		return
	}
	resyncPayload, err := json.Marshal(Message{
		Type: MessageTypeResyncRequired,
		Payload: ResyncRequiredPayload{
			Scope:  ResyncScopeOverview,
			Reason: ResyncReasonClientResync,
		},
	})
	if err != nil {
		log.Printf("WebSocket hub: failed to marshal overview resync marker: %v", err)
		return
	}
	observability.Default().ObserveWSMessage("broadcast", deltaMessage.Type, len(deltaPayload))
	h.recordBroadcast(deltaPayload)
	for _, client := range h.copyClients() {
		h.enqueueOverviewDelta(client, deltaPayload, resyncPayload, fallbackPayload)
	}
}

// SendOverviewSnapshot sends a versioned full snapshot to one client.
func (h *Hub) SendOverviewSnapshot(client *Client, snapshot *SnapshotPayload, version uint64) {
	msg := NewSnapshotMessage(snapshot, version)
	payload, err := json.Marshal(msg)
	if err != nil {
		log.Printf("WebSocket hub: failed to marshal overview snapshot: %v", err)
		return
	}
	observability.Default().ObserveWSMessage("unicast", msg.Type, len(payload))
	h.enqueueOverviewSnapshot(client, payload)
}

// BroadcastOverviewResync broadcasts an explicit overview resync marker followed
// by a full versioned snapshot to all connected clients.
func (h *Hub) BroadcastOverviewResync(reason string, snapshot *SnapshotPayload, version uint64) {
	resyncPayload, err := json.Marshal(Message{
		Type: MessageTypeResyncRequired,
		Payload: ResyncRequiredPayload{
			Scope:  ResyncScopeOverview,
			Reason: reason,
		},
	})
	if err != nil {
		log.Printf("WebSocket hub: failed to marshal overview resync marker: %v", err)
		return
	}
	snapshotMessage := NewSnapshotMessage(snapshot, version)
	snapshotPayload, err := json.Marshal(snapshotMessage)
	if err != nil {
		log.Printf("WebSocket hub: failed to marshal overview resync snapshot: %v", err)
		return
	}
	observability.Default().ObserveWSMessage("broadcast", MessageTypeResyncRequired, len(resyncPayload))
	observability.Default().ObserveWSMessage("broadcast", snapshotMessage.Type, len(snapshotPayload))
	h.recordBroadcast(resyncPayload)
	h.recordBroadcast(snapshotPayload)
	for _, client := range h.copyClients() {
		h.enqueueOverviewResync(client, resyncPayload, snapshotPayload)
	}
}

func (h *Hub) SetDetailSubscription(client *Client, deviceID uuid.UUID) {
	h.mu.Lock()
	defer h.mu.Unlock()

	client.detailDeviceID = deviceID
}

func (h *Hub) ClearDetailSubscription(client *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()

	client.detailDeviceID = uuid.Nil
}

func (h *Hub) DetailSubscribers(deviceID uuid.UUID) []*Client {
	h.mu.RLock()
	defer h.mu.RUnlock()

	subscribers := make([]*Client, 0)
	for client := range h.clients {
		if client.detailDeviceID == deviceID {
			subscribers = append(subscribers, client)
		}
	}

	return subscribers
}

func (h *Hub) enqueue(client *Client, payload []byte) bool {
	client.mu.Lock()
	defer client.mu.Unlock()
	if client.closed {
		return false
	}
	select {
	case client.send <- payload:
		return true
	default:
		observability.Default().IncWSBackpressure(wsBackpressureScopeClientSend, wsBackpressureReasonClientBufferFull)
		return false
	}
}

func (h *Hub) enqueueOverviewSnapshot(client *Client, payload []byte) bool {
	client.mu.Lock()
	defer client.mu.Unlock()
	if client.closed {
		return false
	}
	clearQueuedMessages(client.overviewSend)
	client.needsResync = false
	select {
	case client.overviewSend <- payload:
		return true
	default:
		observability.Default().IncWSBackpressure(wsBackpressureScopeOverviewSend, wsBackpressureReasonClientMailboxFull)
		return false
	}
}

func (h *Hub) enqueueOverviewDelta(client *Client, deltaPayload, resyncPayload, fallbackPayload []byte) bool {
	client.mu.Lock()
	defer client.mu.Unlock()
	if client.closed {
		return false
	}
	if !client.needsResync {
		select {
		case client.overviewSend <- deltaPayload:
			return true
		default:
		}
	}
	observability.Default().IncWSBackpressure(wsBackpressureScopeOverviewSend, wsBackpressureReasonClientMailboxFull)
	client.needsResync = true
	clearQueuedMessages(client.overviewSend)
	select {
	case client.overviewSend <- resyncPayload:
	default:
	}
	select {
	case client.overviewSend <- fallbackPayload:
		client.needsResync = false
	default:
	}
	return true
}

func (h *Hub) enqueueOverviewResync(client *Client, resyncPayload, snapshotPayload []byte) bool {
	client.mu.Lock()
	defer client.mu.Unlock()
	if client.closed {
		return false
	}
	clearQueuedMessages(client.overviewSend)
	client.needsResync = false
	select {
	case client.overviewSend <- resyncPayload:
	default:
	}
	select {
	case client.overviewSend <- snapshotPayload:
	default:
		observability.Default().IncWSBackpressure(wsBackpressureScopeOverviewSend, wsBackpressureReasonClientMailboxFull)
	}
	return true
}

func (h *Hub) removeClient(client *Client) {
	h.mu.Lock()
	_, ok := h.clients[client]
	if ok {
		client.mu.Lock()
		client.detailDeviceID = uuid.Nil
		client.closed = true
		delete(h.clients, client)
		close(client.send)
		close(client.overviewSend)
		client.mu.Unlock()
	}
	h.mu.Unlock()

	if ok && client.conn != nil {
		_ = client.conn.Close()
	}
}

func (c *Client) readPump() {
	defer func() {
		c.markDisconnected()
		c.hub.unregister <- c
	}()

	c.conn.SetReadLimit(maxMessageSize)
	_ = c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error {
		return c.conn.SetReadDeadline(time.Now().Add(pongWait))
	})

	for {
		messageType, message, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("WebSocket read error: %v", err)
			}
			break
		}

		if messageType != websocket.TextMessage {
			continue
		}

		cmd, err := parseClientControlMessage(message)
		if err != nil {
			log.Printf("WebSocket control message ignored: %v", err)
			continue
		}

		switch cmd.Type {
		case MessageTypeHello:
			c.acceptHello(cmd)
		case MessageTypeSubscribeDetail:
			c.hub.SetDetailSubscription(c, cmd.DeviceID)
		case MessageTypeUnsubscribeDetail:
			c.hub.ClearDetailSubscription(c)
		}
	}
}

func (c *Client) markDisconnected() {
	if c.disconnected == nil {
		return
	}
	c.disconnectOnce.Do(func() {
		close(c.disconnected)
	})
}

func (c *Client) acceptHello(cmd clientControlMessage) {
	if c.hello == nil {
		return
	}

	select {
	case c.hello <- cmd:
	default:
	}
}

func (c *Client) writePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		_ = c.conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.overviewSend:
			if !c.writePayload(message, ok) {
				return
			}
			continue
		default:
		}

		select {
		case message, ok := <-c.overviewSend:
			if !c.writePayload(message, ok) {
				return
			}
		case message, ok := <-c.send:
			if !c.writePayload(message, ok) {
				return
			}
		case <-ticker.C:
			_ = c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				if err != http.ErrHandlerTimeout {
					log.Printf("WebSocket ping error: %v", err)
				}
				return
			}
		}
	}
}

func (c *Client) writePayload(message []byte, ok bool) bool {
	_ = c.conn.SetWriteDeadline(time.Now().Add(writeWait))
	if !ok {
		_ = c.conn.WriteMessage(websocket.CloseMessage, []byte{})
		return false
	}

	writer, err := c.conn.NextWriter(websocket.TextMessage)
	if err != nil {
		return false
	}
	if _, err := writer.Write(message); err != nil {
		_ = writer.Close()
		return false
	}
	if err := writer.Close(); err != nil {
		return false
	}
	return true
}

func (h *Hub) recordBroadcast(payload []byte) {
	select {
	case h.broadcast <- payload:
	default:
		observability.Default().IncWSBackpressure(wsBackpressureScopeBroadcast, wsBackpressureReasonHubBufferFull)
	}
}

func (h *Hub) copyClients() []*Client {
	h.mu.RLock()
	defer h.mu.RUnlock()
	clients := make([]*Client, 0, len(h.clients))
	for client := range h.clients {
		clients = append(clients, client)
	}
	return clients
}

func clearQueuedMessages(ch chan []byte) {
	for {
		select {
		case <-ch:
		default:
			return
		}
	}
}
