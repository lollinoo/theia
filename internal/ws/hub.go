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
	writeWait      = 60 * time.Second
	pongWait       = 60 * time.Second
	pingPeriod     = 54 * time.Second
	maxMessageSize = 4096
	sendBufferSize = 16
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
	send           chan []byte
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
		case message := <-h.broadcast:
			h.mu.RLock()
			clients := make([]*Client, 0, len(h.clients))
			for client := range h.clients {
				clients = append(clients, client)
			}
			h.mu.RUnlock()

			for _, client := range clients {
				if !h.enqueue(client, message) {
					h.removeClient(client)
				}
			}
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
	h.broadcast <- payload
}

// SendTo serializes a message and queues it for a single client.
func (h *Hub) SendTo(client *Client, msg Message) {
	payload, err := json.Marshal(msg)
	if err != nil {
		log.Printf("WebSocket hub: failed to marshal client message: %v", err)
		return
	}
	observability.Default().ObserveWSMessage("unicast", msg.Type, len(payload))
	if !h.enqueue(client, payload) {
		h.removeClient(client)
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
	select {
	case client.send <- payload:
		return true
	default:
		return false
	}
}

func (h *Hub) removeClient(client *Client) {
	h.mu.Lock()
	_, ok := h.clients[client]
	if ok {
		client.detailDeviceID = uuid.Nil
		delete(h.clients, client)
		close(client.send)
	}
	h.mu.Unlock()

	if ok && client.conn != nil {
		_ = client.conn.Close()
	}
}

func (c *Client) readPump() {
	defer func() {
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
		case MessageTypeSubscribeDetail:
			c.hub.SetDetailSubscription(c, cmd.DeviceID)
		case MessageTypeUnsubscribeDetail:
			c.hub.ClearDetailSubscription(c)
		}
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
		case message, ok := <-c.send:
			_ = c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				_ = c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			writer, err := c.conn.NextWriter(websocket.TextMessage)
			if err != nil {
				return
			}
			if _, err := writer.Write(message); err != nil {
				_ = writer.Close()
				return
			}
			if err := writer.Close(); err != nil {
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
