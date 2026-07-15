package ws

// This file defines hub WebSocket protocol behavior, subscriptions, and runtime update delivery.

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/lollinoo/theia/internal/logging"
	"github.com/lollinoo/theia/internal/observability"
)

const (
	writeWait         = 60 * time.Second
	pongWait          = 60 * time.Second
	pingPeriod        = 54 * time.Second
	maxMessageSize    = 4096
	clientHelloBuffer = 1

	// These bounded queues are part of the WebSocket anti-OOM contract:
	// producers drop/resync slow clients instead of blocking or growing memory.
	sendBufferSize     = 16
	overviewBufferSize = 32

	wsBackpressureScopeBroadcast    = "broadcast"
	wsBackpressureScopeClientSend   = "client_send"
	wsBackpressureScopeOverviewSend = "overview_send"
	wsMessageScopeOverviewFallback  = "overview_fallback"

	wsBackpressureReasonHubBufferFull     = "hub_buffer_full"
	wsBackpressureReasonClientBufferFull  = "client_buffer_full"
	wsBackpressureReasonClientMailboxFull = "client_mailbox_full"

	wsConnectionEventConnected    = "connected"
	wsConnectionEventDisconnected = "disconnected"

	wsResyncBootstrapHTTP   = "http"
	wsResyncBootstrapLegacy = "legacy"

	wsOverviewMailboxClearReasonSnapshot          = "snapshot"
	wsOverviewMailboxClearReasonClientMailboxFull = wsBackpressureReasonClientMailboxFull
	wsOverviewMailboxClearReasonExplicitResync    = "resync"
	wsOverviewMailboxClearReasonHTTPResyncPending = "http_resync_pending"
)

// Hub manages all active WebSocket clients and server-side broadcasts.
type Hub struct {
	clients    map[*Client]bool
	broadcast  chan []byte
	register   chan *Client
	unregister chan *Client
	mu         sync.RWMutex
}

// HubOption represents hub option data used by the WebSocket protocol.
type HubOption func(*Hub)

func WithBroadcastRecorder() HubOption {
	return func(h *Hub) {
		h.broadcast = make(chan []byte, 32)
	}
}

// Client is a single WebSocket connection.
type Client struct {
	hub                      *Hub
	conn                     *websocket.Conn
	mu                       sync.Mutex
	closed                   bool
	disconnected             chan struct{}
	disconnectOnce           sync.Once
	send                     chan []byte
	overviewSend             chan []byte
	hello                    chan clientControlMessage
	needsResync              bool
	overviewEpoch            uint64
	overviewRecoveryGen      uint64
	bootstrapRecoveryGen     uint64
	usesHTTPRuntimeBootstrap bool
	bootstrapping            bool
	runtimeProtocol          int
	ackedRuntimeCursor       RuntimeCursor
	runtimeAckCeiling        RuntimeCursor
	runtimeSync              RuntimeSyncFunc
	runtimeAck               RuntimeAckFunc
	pendingOverviewRecovery  *overviewRecoveryState
	pendingRuntimeHelloEcho  *runtimeHelloEcho
	detailDeviceID           uuid.UUID
}

// RuntimeSyncRequest asks the pipeline to synchronize one client from a runtime cursor.
type RuntimeSyncRequest struct {
	Cursor RuntimeCursor
	Reason string
}

// OverviewSyncMode identifies the state carried by one atomic overview replacement.
type OverviewSyncMode string

const (
	OverviewSyncModeCurrent  OverviewSyncMode = "current"
	OverviewSyncModeReplay   OverviewSyncMode = "replay"
	OverviewSyncModeSnapshot OverviewSyncMode = "snapshot"
)

// OverviewSyncBatch describes one complete runtime recovery replacement.
type OverviewSyncBatch struct {
	Reason          string
	Mode            OverviewSyncMode
	RuntimeStreamID string
	TargetVersion   uint64
	RuntimeIdentity string
	ReplayCursor    RuntimeCursor
	Replay          *RuntimeDeltaPayload
	Snapshot        *SnapshotPayload
	AlertVersion    uint64
}

// OverviewSyncReplacementResult reports clients whose replacement batch was
// installed atomically and clients for which installation failed.
type OverviewSyncReplacementResult struct {
	Installed []*Client
	Failed    []*Client
}

type overviewRecoveryState struct {
	streamID      string
	targetVersion uint64
	mode          OverviewSyncMode
	startedAt     time.Time
}

type runtimeHelloEcho struct {
	protocol int
	cursor   RuntimeCursor
}

type overviewBootstrapSelection struct {
	mailboxEpoch           uint64
	recoveryGeneration     uint64
	completeRecoveryQueued bool
}

type overviewClientRuntimeState struct {
	protocol      int
	cursor        RuntimeCursor
	httpBootstrap bool
}

// NewHub creates an empty WebSocket hub.
func NewHub(options ...HubOption) *Hub {
	hub := &Hub{
		clients:    make(map[*Client]bool),
		register:   make(chan *Client, 32),
		unregister: make(chan *Client, 32),
	}
	for _, option := range options {
		option(hub)
	}
	return hub
}

// Run processes hub registration, unregistration, and broadcast events.
func (h *Hub) Run() {
	for {
		select {
		case client := <-h.register:
			h.addClient(client)
		case client := <-h.unregister:
			h.removeClient(client)
		}
	}
}

func (h *Hub) addClient(client *Client) {
	h.mu.Lock()
	_, existed := h.clients[client]
	h.clients[client] = true
	clientCount := len(h.clients)
	h.mu.Unlock()
	if !existed {
		observability.Default().IncWSConnectionEvent(wsConnectionEventConnected)
	}
	observability.Default().SetWSConnectedClients(clientCount)
	logging.Debugf("websocket client registered clients=%d", clientCount)
}

// HasOverviewClients reports whether any client can receive overview broadcasts.
func (h *Hub) HasOverviewClients() bool {
	return len(h.copyOverviewClients()) > 0
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
	clients := h.copyClients()
	logging.Debugf("websocket message queued scope=broadcast type=%s bytes=%d clients=%d", msg.Type, len(payload), len(clients))
	for _, client := range clients {
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
	logging.Debugf("websocket message queued scope=unicast type=%s bytes=%d", msg.Type, len(payload))
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
	logging.Debugf("websocket message write-direct type=%s bytes=%d", msg.Type, len(payload))
	return client.writePayload(payload, true)
}

// writeHTTPRuntimeResync writes the legacy bootstrap marker only when no
// complete overview replacement has already won the client mailbox.
func (h *Hub) writeHTTPRuntimeResync(client *Client, msg Message, selection overviewBootstrapSelection) (bool, bool) {
	payload, err := json.Marshal(msg)
	if err != nil {
		log.Printf("WebSocket hub: failed to marshal direct runtime resync: %v", err)
		return false, false
	}

	client.mu.Lock()
	if client.closed {
		client.mu.Unlock()
		return false, false
	}
	client.usesHTTPRuntimeBootstrap = true
	if selection.completeRecoveryQueued || client.overviewRecoveryGen != selection.recoveryGeneration {
		client.mu.Unlock()
		return false, true
	}
	if pending := client.pendingOverviewRecovery; pending != nil && pending.mode != "" {
		client.mu.Unlock()
		return false, true
	}
	client.needsResync = true
	cleared := clearQueuedMessages(client.overviewSend)
	client.overviewEpoch++
	ok := client.writePayload(payload, true)
	client.mu.Unlock()

	h.recordOverviewMailboxClear(wsOverviewMailboxClearReasonHTTPResyncPending, cleared)
	observability.Default().ObserveWSMessage("unicast", msg.Type, len(payload))
	logging.Debugf("websocket message write-direct type=%s bytes=%d", msg.Type, len(payload))
	return true, ok
}

// BroadcastOverviewSnapshot broadcasts a versioned full overview snapshot.
func (h *Hub) BroadcastOverviewSnapshot(snapshot *SnapshotPayload, version uint64) {
	clients := h.copyOverviewClients()
	if len(clients) == 0 {
		return
	}

	msg := NewSnapshotMessage(snapshot, version)
	payload, err := json.Marshal(msg)
	if err != nil {
		log.Printf("WebSocket hub: failed to marshal overview snapshot: %v", err)
		return
	}
	observability.Default().ObserveWSMessage("broadcast", msg.Type, len(payload))
	h.recordBroadcast(payload)
	logging.Debugf("websocket message queued scope=overview_broadcast type=%s version=%d bytes=%d clients=%d", msg.Type, version, len(payload), len(clients))
	batch := OverviewSyncBatch{
		Reason:          ResyncReasonClientResync,
		Mode:            OverviewSyncModeSnapshot,
		TargetVersion:   version,
		RuntimeIdentity: RuntimeIdentityForSnapshot(snapshot),
		Snapshot:        snapshot,
	}
	for _, client := range clients {
		if client.usesHTTPBootstrap() {
			h.ReplaceOverviewStream(client, batch)
			continue
		}
		h.enqueueOverviewSnapshot(client, payload)
	}
}

// BroadcastOverviewDelta preserves legacy callers by installing a complete
// snapshot recovery whenever the stream-aware broadcast reports overflow.
func (h *Hub) BroadcastOverviewDelta(delta *RuntimeDeltaPayload, baseVersion, version uint64, fallbackSnapshot *SnapshotPayload) []*Client {
	overflowed := h.BroadcastOverviewStreamDelta(delta, baseVersion, version, "")
	var fallbackBytes int
	if len(overflowed) > 0 {
		if payload, err := json.Marshal(NewSnapshotMessage(fallbackSnapshot, version)); err == nil {
			fallbackBytes = len(payload)
		}
	}
	for _, client := range overflowed {
		if h.ReplaceOverviewStream(client, OverviewSyncBatch{
			Reason:          ResyncReasonClientResync,
			Mode:            OverviewSyncModeSnapshot,
			TargetVersion:   version,
			RuntimeIdentity: RuntimeIdentityForSnapshot(fallbackSnapshot),
			Snapshot:        fallbackSnapshot,
		}) && fallbackBytes > 0 {
			observability.Default().ObserveWSMessage(wsMessageScopeOverviewFallback, MessageTypeSnapshot, fallbackBytes)
		}
	}
	return overflowed
}

// BroadcastOverviewStreamDelta queues one stream-aware delta and returns clients
// whose bounded mailbox must be replaced by the lock-owning pipeline.
func (h *Hub) BroadcastOverviewStreamDelta(delta *RuntimeDeltaPayload, baseVersion, version uint64, streamID string) []*Client {
	clients := h.copyOverviewClients()
	if len(clients) == 0 {
		return nil
	}

	deltaMessage := NewStreamRuntimeDeltaMessage(delta, baseVersion, version, streamID)
	deltaPayload, err := json.Marshal(deltaMessage)
	if err != nil {
		log.Printf("WebSocket hub: failed to marshal overview delta: %v", err)
		return nil
	}
	observability.Default().ObserveWSMessage("broadcast", deltaMessage.Type, len(deltaPayload))
	h.recordBroadcast(deltaPayload)
	devicePatchCount, linkPatchCount := runtimeDeltaPatchCounts(delta)
	logging.Debugf(
		"websocket message queued scope=overview_broadcast type=%s base_version=%d version=%d device_patches=%d link_patches=%d bytes=%d fallback_bytes=%d clients=%d",
		deltaMessage.Type,
		baseVersion,
		version,
		devicePatchCount,
		linkPatchCount,
		len(deltaPayload),
		0,
		len(clients),
	)
	overflowed := make([]*Client, 0)
	for _, client := range clients {
		if h.enqueueOverviewStreamDelta(client, deltaPayload, streamID, baseVersion, version) {
			overflowed = append(overflowed, client)
		}
	}
	return overflowed
}

func runtimeDeltaPatchCounts(delta *RuntimeDeltaPayload) (int, int) {
	if delta == nil {
		return 0, 0
	}
	return len(delta.Devices), len(delta.Links)
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
	logging.Debugf("websocket message queued scope=overview_unicast type=%s version=%d bytes=%d", msg.Type, version, len(payload))
	h.enqueueOverviewSnapshot(client, payload)
}

// BroadcastOverviewResync broadcasts an explicit overview resync marker followed
// by a full versioned snapshot to all connected clients.
func (h *Hub) BroadcastOverviewResync(reason string, snapshot *SnapshotPayload, version uint64) {
	clients := h.copyOverviewClients()
	if len(clients) == 0 {
		return
	}

	snapshotMessage := NewSnapshotMessage(snapshot, version)
	snapshotPayload, err := json.Marshal(snapshotMessage)
	if err != nil {
		log.Printf("WebSocket hub: failed to marshal overview resync snapshot: %v", err)
		return
	}
	observability.Default().ObserveWSMessage("broadcast", snapshotMessage.Type, len(snapshotPayload))
	h.recordBroadcast(snapshotPayload)
	logging.Debugf(
		"websocket message queued scope=overview_broadcast type=%s reason=%s snapshot_version=%d snapshot_bytes=%d clients=%d",
		MessageTypeResyncRequired,
		reason,
		version,
		len(snapshotPayload),
		len(clients),
	)
	batch := OverviewSyncBatch{
		Reason:          reason,
		Mode:            OverviewSyncModeSnapshot,
		TargetVersion:   version,
		RuntimeIdentity: RuntimeIdentityForSnapshot(snapshot),
		Snapshot:        snapshot,
	}
	for _, client := range clients {
		h.ReplaceOverviewStream(client, batch)
	}
}

// ReplaceOverviewStream atomically replaces one client's bounded overview mailbox.
func (h *Hub) ReplaceOverviewStream(client *Client, batch OverviewSyncBatch) bool {
	if client == nil {
		return false
	}

	for {
		clientState := client.runtimeState()
		if !overviewSyncClientStateMatchesBatch(batch, clientState) {
			return false
		}
		payloads, err := marshalOverviewSyncBatch(batch, clientState)
		if err != nil {
			log.Printf("WebSocket hub: failed to marshal overview sync batch: %v", err)
			return false
		}
		if len(payloads) > cap(client.overviewSend) {
			return false
		}

		client.mu.Lock()
		if client.closed {
			client.mu.Unlock()
			return false
		}
		if client.runtimeProtocol != clientState.protocol || client.ackedRuntimeCursor != clientState.cursor {
			client.mu.Unlock()
			continue
		}
		if pending := client.pendingOverviewRecovery; pending != nil && batch.TargetVersion < pending.targetVersion {
			client.mu.Unlock()
			return false
		}

		cleared := clearQueuedMessages(client.overviewSend)
		client.overviewEpoch++
		client.overviewRecoveryGen++
		client.pendingOverviewRecovery = &overviewRecoveryState{
			streamID:      batch.RuntimeStreamID,
			targetVersion: batch.TargetVersion,
			mode:          batch.Mode,
			startedAt:     time.Now(),
		}
		client.needsResync = batch.Mode != OverviewSyncModeCurrent
		for _, payload := range payloads {
			client.overviewSend <- payload
		}
		client.installRuntimeAckCeilingLocked(batch.RuntimeStreamID, batch.TargetVersion)
		client.mu.Unlock()

		h.recordOverviewMailboxClear(wsOverviewMailboxClearReasonExplicitResync, cleared)
		if batch.Mode != OverviewSyncModeCurrent {
			h.recordOverviewResyncRequired(batch.Reason, clientState.httpBootstrap)
		}
		return true
	}
}

func overviewSyncClientStateMatchesBatch(batch OverviewSyncBatch, clientState overviewClientRuntimeState) bool {
	switch batch.Mode {
	case OverviewSyncModeCurrent:
		return clientState.cursor == (RuntimeCursor{
			StreamID: batch.RuntimeStreamID,
			Version:  batch.TargetVersion,
			Known:    true,
		})
	case OverviewSyncModeReplay:
		return clientState.protocol >= RuntimeStreamProtocolVersion &&
			batch.ReplayCursor.Known &&
			clientState.cursor == batch.ReplayCursor
	default:
		return true
	}
}

// ReplaceOverviewStreams records one logical state broadcast and atomically
// installs the complete replacement for every eligible overview client.
func (h *Hub) ReplaceOverviewStreams(batch OverviewSyncBatch) OverviewSyncReplacementResult {
	clients := h.copyClients()
	result := OverviewSyncReplacementResult{
		Installed: make([]*Client, 0, len(clients)),
		Failed:    make([]*Client, 0),
	}
	if len(clients) == 0 {
		return result
	}

	if stateMessage, ok := overviewSyncStateMessage(batch); ok {
		if payload, err := json.Marshal(stateMessage); err == nil {
			observability.Default().ObserveWSMessage("broadcast", stateMessage.Type, len(payload))
			h.recordBroadcast(payload)
		}
	}

	for _, client := range clients {
		if h.ReplaceOverviewStream(client, batch) {
			result.Installed = append(result.Installed, client)
		} else {
			result.Failed = append(result.Failed, client)
		}
	}
	return result
}

func overviewSyncStateMessage(batch OverviewSyncBatch) (Message, bool) {
	switch batch.Mode {
	case OverviewSyncModeReplay:
		if batch.Replay == nil ||
			!batch.ReplayCursor.Known ||
			batch.ReplayCursor.StreamID != batch.RuntimeStreamID ||
			batch.ReplayCursor.Version >= batch.TargetVersion {
			return Message{}, false
		}
		return NewRuntimeReplayMessage(
			batch.Replay,
			batch.ReplayCursor.Version,
			batch.TargetVersion,
			batch.RuntimeStreamID,
		), true
	case OverviewSyncModeSnapshot:
		if batch.Snapshot == nil {
			return Message{}, false
		}
		return Message{
			Type: MessageTypeSnapshot,
			Payload: SnapshotMessagePayload{
				Version:         batch.TargetVersion,
				RuntimeStreamID: batch.RuntimeStreamID,
				RuntimeIdentity: batch.RuntimeIdentity,
				Snapshot:        CloneSnapshot(batch.Snapshot),
			},
		}, true
	default:
		return Message{}, false
	}
}

func marshalOverviewSyncBatch(batch OverviewSyncBatch, clientState overviewClientRuntimeState) ([][]byte, error) {
	ready := NewStreamReadyMessage(
		batch.TargetVersion,
		batch.AlertVersion,
		batch.RuntimeIdentity,
		batch.RuntimeStreamID,
		string(batch.Mode),
	)
	messages := make([]Message, 0, 3)

	switch batch.Mode {
	case OverviewSyncModeCurrent:
		if !overviewSyncClientStateMatchesBatch(batch, clientState) {
			return nil, fmt.Errorf("current runtime cursor no longer matches target")
		}
		messages = append(messages, ready)
	case OverviewSyncModeReplay:
		if clientState.protocol < RuntimeStreamProtocolVersion {
			return nil, fmt.Errorf("runtime replay requires protocol %d", RuntimeStreamProtocolVersion)
		}
		if !batch.ReplayCursor.Known ||
			batch.ReplayCursor.StreamID != batch.RuntimeStreamID ||
			batch.ReplayCursor.Version >= batch.TargetVersion {
			return nil, fmt.Errorf("runtime replay source cursor does not precede target")
		}
		if clientState.cursor != batch.ReplayCursor {
			return nil, fmt.Errorf("runtime replay source cursor no longer matches client")
		}
		if batch.Replay == nil {
			return nil, fmt.Errorf("runtime replay payload is nil")
		}
		stateMessage, ok := overviewSyncStateMessage(batch)
		if !ok {
			return nil, fmt.Errorf("runtime replay state is invalid")
		}
		messages = append(
			messages,
			newOverviewResyncMessage(batch),
			stateMessage,
			ready,
		)
	case OverviewSyncModeSnapshot:
		if batch.Snapshot == nil {
			return nil, fmt.Errorf("runtime snapshot payload is nil")
		}
		stateMessage, ok := overviewSyncStateMessage(batch)
		if !ok {
			return nil, fmt.Errorf("runtime snapshot state is invalid")
		}
		messages = append(
			messages,
			newOverviewResyncMessage(batch),
			stateMessage,
			ready,
		)
	default:
		return nil, fmt.Errorf("unsupported overview sync mode %q", batch.Mode)
	}

	payloads := make([][]byte, 0, len(messages))
	for _, message := range messages {
		payload, err := json.Marshal(message)
		if err != nil {
			return nil, err
		}
		payloads = append(payloads, payload)
	}
	return payloads, nil
}

func newOverviewResyncMessage(batch OverviewSyncBatch) Message {
	targetVersion := batch.TargetVersion
	return Message{
		Type: MessageTypeResyncRequired,
		Payload: ResyncRequiredPayload{
			Scope:           ResyncScopeOverview,
			Reason:          batch.Reason,
			Strategy:        RuntimeSyncStrategyStream,
			TargetVersion:   &targetVersion,
			RuntimeStreamID: batch.RuntimeStreamID,
		},
	}
}

func (h *Hub) SetDetailSubscription(client *Client, deviceID uuid.UUID) {
	h.mu.Lock()
	defer h.mu.Unlock()

	client.detailDeviceID = deviceID
	logging.Debugf("websocket detail subscription set active=%t", deviceID != uuid.Nil)
}

func (h *Hub) ClearDetailSubscription(client *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()

	client.detailDeviceID = uuid.Nil
	logging.Debugf("websocket detail subscription cleared")
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
	h.recordOverviewMailboxClear(wsOverviewMailboxClearReasonSnapshot, clearQueuedMessages(client.overviewSend))
	client.overviewEpoch++
	client.needsResync = false
	client.pendingOverviewRecovery = nil
	select {
	case client.overviewSend <- payload:
		return true
	default:
		observability.Default().IncWSBackpressure(wsBackpressureScopeOverviewSend, wsBackpressureReasonClientMailboxFull)
		return false
	}
}

func (h *Hub) enqueueOverviewStreamDelta(client *Client, payload []byte, streamID string, baseVersion, version uint64) bool {
	client.mu.Lock()
	defer client.mu.Unlock()
	if client.closed {
		return false
	}

	pending := client.pendingOverviewRecovery
	if pending != nil {
		if pending.mode == "" {
			return true
		}
		if pending.streamID != "" && streamID != "" && pending.streamID != streamID {
			return h.markOverviewRecoveryPendingLocked(client, streamID, version)
		}
		if baseVersion < pending.targetVersion {
			return false
		}
	}
	if client.needsResync && pending == nil {
		return h.markOverviewRecoveryPendingLocked(client, streamID, version)
	}

	select {
	case client.overviewSend <- payload:
		client.overviewEpoch++
		client.advanceRuntimeAckCeilingLocked(streamID, version)
		return false
	default:
		return h.markOverviewRecoveryPendingLocked(client, streamID, version)
	}
}

func (h *Hub) markOverviewRecoveryPendingLocked(client *Client, streamID string, targetVersion uint64) bool {
	observability.Default().IncWSBackpressure(wsBackpressureScopeOverviewSend, wsBackpressureReasonClientMailboxFull)
	cleared := clearQueuedMessages(client.overviewSend)
	h.recordOverviewMailboxClear(wsOverviewMailboxClearReasonClientMailboxFull, cleared)
	client.overviewEpoch++
	client.needsResync = true
	client.pendingOverviewRecovery = &overviewRecoveryState{
		streamID:      streamID,
		targetVersion: targetVersion,
		startedAt:     time.Now(),
	}
	return true
}

func (h *Hub) removeClient(client *Client) {
	clientCount := 0
	h.mu.Lock()
	_, ok := h.clients[client]
	if ok {
		client.mu.Lock()
		client.detailDeviceID = uuid.Nil
		client.closed = true
		delete(h.clients, client)
		clientCount = len(h.clients)
		close(client.send)
		close(client.overviewSend)
		client.mu.Unlock()
	}
	h.mu.Unlock()

	if ok && client.conn != nil {
		_ = client.conn.Close()
	}
	if ok {
		observability.Default().IncWSConnectionEvent(wsConnectionEventDisconnected)
		observability.Default().SetWSConnectedClients(clientCount)
		logging.Debugf("websocket client unregistered clients=%d", clientCount)
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
		logging.Debugf("websocket control received type=%s", cmd.Type)

		switch cmd.Type {
		case MessageTypeHello:
			c.acceptHello(cmd)
		case MessageTypeResumeRuntime:
			c.acceptRuntimeResume(cmd.RuntimeCursor)
		case MessageTypeRuntimeAck:
			c.acceptRuntimeAck(cmd.RuntimeCursor)
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
	c.mu.Lock()
	bootstrapping := c.bootstrapping
	runtimeSync := c.runtimeSync
	queryHelloEcho := c.consumeRuntimeQueryHelloEchoLocked(cmd)
	acceptedCursor := cmd.RuntimeCursor
	if bootstrapping || runtimeSync == nil {
		c.initializeRuntimeHelloLocked(cmd)
		if cmd.RuntimeCursor.Known {
			acceptedCursor = c.ackedRuntimeCursor
		}
	} else {
		c.usesHTTPRuntimeBootstrap = true
		c.runtimeProtocol = cmd.RuntimeProtocol
	}
	if runtimeSync == nil && acceptedCursor.Known {
		if pending := c.pendingOverviewRecovery; pending != nil &&
			pending.streamID == acceptedCursor.StreamID &&
			acceptedCursor.Version >= pending.targetVersion {
			c.pendingOverviewRecovery = nil
			c.needsResync = false
		}
	}
	c.mu.Unlock()

	if queryHelloEcho {
		return
	}
	if !bootstrapping {
		if runtimeSync != nil {
			runtimeSync(c, RuntimeSyncRequest{
				Cursor: cmd.RuntimeCursor,
				Reason: ResyncReasonClientResync,
			})
		}
		return
	}
	if c.hello == nil {
		return
	}

	select {
	case c.hello <- cmd:
	default:
	}
}

func (c *Client) initializeRuntimeHello(cmd clientControlMessage) {
	c.mu.Lock()
	c.initializeRuntimeHelloLocked(cmd)
	c.mu.Unlock()
}

func (c *Client) initializeRuntimeQueryHello(cmd clientControlMessage) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.initializeRuntimeHelloLocked(cmd)
	if cmd.RuntimeProtocol >= RuntimeStreamProtocolVersion {
		c.pendingRuntimeHelloEcho = &runtimeHelloEcho{
			protocol: cmd.RuntimeProtocol,
			cursor:   cmd.RuntimeCursor,
		}
	}
}

func (c *Client) consumeRuntimeQueryHelloEchoLocked(cmd clientControlMessage) bool {
	echo := c.pendingRuntimeHelloEcho
	c.pendingRuntimeHelloEcho = nil
	return echo != nil && echo.protocol == cmd.RuntimeProtocol && echo.cursor == cmd.RuntimeCursor
}

func (c *Client) initializeRuntimeHelloLocked(cmd clientControlMessage) {
	c.usesHTTPRuntimeBootstrap = true
	c.runtimeProtocol = cmd.RuntimeProtocol
	if cmd.RuntimeCursor.Known {
		c.ackedRuntimeCursor = c.sanitizeRuntimeCursorForCeilingLocked(cmd.RuntimeCursor)
	}
}

func (c *Client) acceptRuntimeResume(cursor RuntimeCursor) {
	c.mu.Lock()
	bootstrapping := c.bootstrapping
	runtimeSync := c.runtimeSync
	c.mu.Unlock()
	if bootstrapping || runtimeSync == nil {
		return
	}
	runtimeSync(c, RuntimeSyncRequest{
		Cursor: cursor,
		Reason: ResyncReasonClientResync,
	})
}

func (c *Client) acceptRuntimeAck(cursor RuntimeCursor) {
	if !cursor.Known {
		return
	}

	c.mu.Lock()
	if !c.runtimeAckWithinCeilingLocked(cursor) {
		c.mu.Unlock()
		return
	}
	current := c.ackedRuntimeCursor
	pending := c.pendingOverviewRecovery
	sameStreamAdvance := current.Known &&
		cursor.StreamID == current.StreamID &&
		cursor.Version > current.Version
	// A matching recovery may complete at the current same-stream cursor, but
	// an ACK without pending recovery must still advance monotonically.
	pendingRecoveryCompletion := pending != nil &&
		pending.streamID == cursor.StreamID &&
		cursor.Version >= pending.targetVersion &&
		(!current.Known || cursor.StreamID != current.StreamID || cursor.Version >= current.Version)
	if !sameStreamAdvance && !pendingRecoveryCompletion {
		c.mu.Unlock()
		return
	}
	c.ackedRuntimeCursor = cursor
	if pending != nil &&
		pending.streamID == cursor.StreamID &&
		cursor.Version >= pending.targetVersion {
		c.pendingOverviewRecovery = nil
		c.needsResync = false
	}
	runtimeAck := c.runtimeAck
	c.mu.Unlock()

	if runtimeAck != nil {
		runtimeAck(c, cursor)
	}
}

// installRuntimeAckCeilingLocked publishes the exact cursor offered by a
// complete replacement and discards an impossible future same-stream claim.
func (c *Client) installRuntimeAckCeilingLocked(streamID string, version uint64) {
	c.runtimeAckCeiling = RuntimeCursor{}
	if streamID == "" {
		return
	}
	c.runtimeAckCeiling = RuntimeCursor{StreamID: streamID, Version: version, Known: true}
	if current := c.ackedRuntimeCursor; current.Known && current.StreamID == streamID && current.Version > version {
		c.ackedRuntimeCursor = RuntimeCursor{}
	}
}

// advanceRuntimeAckCeilingLocked records a successfully queued live delta.
func (c *Client) advanceRuntimeAckCeilingLocked(streamID string, version uint64) {
	if streamID == "" {
		return
	}
	current := c.runtimeAckCeiling
	if current.Known && current.StreamID == streamID && current.Version >= version {
		return
	}
	c.runtimeAckCeiling = RuntimeCursor{StreamID: streamID, Version: version, Known: true}
}

func (c *Client) sanitizeRuntimeCursorForCeilingLocked(cursor RuntimeCursor) RuntimeCursor {
	ceiling := c.runtimeAckCeiling
	if ceiling.Known && cursor.StreamID == ceiling.StreamID && cursor.Version > ceiling.Version {
		return RuntimeCursor{}
	}
	return cursor
}

func (c *Client) runtimeAckWithinCeilingLocked(cursor RuntimeCursor) bool {
	ceiling := c.runtimeAckCeiling
	return ceiling.Known && cursor.StreamID == ceiling.StreamID && cursor.Version <= ceiling.Version
}

// RuntimeProtocol reports the negotiated runtime stream protocol capability.
func (c *Client) RuntimeProtocol() int {
	if c == nil {
		return 0
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.runtimeProtocol
}

// AckedRuntimeCursor returns the last runtime cursor acknowledged by the client.
func (c *Client) AckedRuntimeCursor() RuntimeCursor {
	if c == nil {
		return RuntimeCursor{}
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.ackedRuntimeCursor
}

func (c *Client) runtimeState() overviewClientRuntimeState {
	c.mu.Lock()
	defer c.mu.Unlock()
	return overviewClientRuntimeState{
		protocol:      c.runtimeProtocol,
		cursor:        c.ackedRuntimeCursor,
		httpBootstrap: c.usesHTTPRuntimeBootstrap,
	}
}

func (c *Client) beginBootstrapSnapshotSelection() overviewBootstrapSelection {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.bootstrapRecoveryGen = c.overviewRecoveryGen
	return overviewBootstrapSelection{
		mailboxEpoch:       c.overviewEpoch,
		recoveryGeneration: c.overviewRecoveryGen,
	}
}

func (c *Client) markBootstrapSnapshotSelected(selection overviewBootstrapSelection, _ uint64, _ string) overviewBootstrapSelection {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.bootstrapping = false
	if c.overviewRecoveryGen != selection.recoveryGeneration {
		selection.completeRecoveryQueued = true
	} else if pending := c.pendingOverviewRecovery; pending != nil && pending.mode != "" {
		selection.completeRecoveryQueued = true
	}
	return selection
}

func (c *Client) overviewChangedSince(selection overviewBootstrapSelection) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return selection.completeRecoveryQueued ||
		c.overviewRecoveryGen != selection.recoveryGeneration ||
		c.overviewEpoch != selection.mailboxEpoch
}

func (c *Client) usesHTTPBootstrap() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.usesHTTPRuntimeBootstrap
}

func (c *Client) canReceiveOverviewBroadcast() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	// A complete recovery batch makes later deltas safe because they queue
	// behind its ready barrier even if an early ACK cleared pending metadata.
	completeRecoveryInstalled := c.overviewRecoveryGen != c.bootstrapRecoveryGen
	return !c.closed && (!c.bootstrapping || completeRecoveryInstalled)
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
	h.mu.RLock()
	recorder := h.broadcast
	h.mu.RUnlock()
	if recorder == nil {
		return
	}

	select {
	case recorder <- payload:
	default:
		observability.Default().IncWSBackpressure(wsBackpressureScopeBroadcast, wsBackpressureReasonHubBufferFull)
	}
}

func (h *Hub) recordOverviewResyncRequired(reason string, httpBootstrap bool) {
	bootstrap := wsResyncBootstrapLegacy
	if httpBootstrap {
		bootstrap = wsResyncBootstrapHTTP
	}
	observability.Default().IncWSClientResyncRequired(ResyncScopeOverview, reason, bootstrap)
}

func (h *Hub) recordOverviewMailboxClear(reason string, cleared int) {
	observability.Default().AddWSOverviewMailboxCleared(reason, cleared)
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

func (h *Hub) copyOverviewClients() []*Client {
	h.mu.RLock()
	defer h.mu.RUnlock()
	clients := make([]*Client, 0, len(h.clients))
	for client := range h.clients {
		if client.canReceiveOverviewBroadcast() {
			clients = append(clients, client)
		}
	}
	return clients
}

func clearQueuedMessages(ch chan []byte) int {
	cleared := 0
	for {
		select {
		case <-ch:
			cleared++
		default:
			return cleared
		}
	}
}
