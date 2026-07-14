package ws

// This file defines handler WebSocket protocol behavior, subscriptions, and runtime update delivery.

import (
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/gorilla/websocket"
	"github.com/lollinoo/theia/internal/logging"
	"github.com/lollinoo/theia/internal/security"
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
	allowedOrigins []string
}

// HandlerOption customizes WebSocket handler behavior.
type HandlerOption func(*Handler)

// WithAllowedOrigins configures exact browser origins allowed to open WebSockets.
func WithAllowedOrigins(origins []string) HandlerOption {
	return func(h *Handler) {
		h.allowedOrigins = security.NormalizedAllowedOrigins(origins)
	}
}

// NewHandler creates a WebSocket handler that serves initial snapshots on connect.
// promStatusFunc returns the current Prometheus integration status.
func NewHandler(hub *Hub, snapshotFunc func() (*SnapshotPayload, uint64), alertsFunc func() AlertMessagePayload, promStatusFunc func() PrometheusStatusPayload, opts ...HandlerOption) *Handler {
	handler := &Handler{
		hub:            hub,
		snapshotFunc:   snapshotFunc,
		alertsFunc:     alertsFunc,
		promStatusFunc: promStatusFunc,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(handler)
		}
	}
	return handler
}

// ServeHTTP upgrades the request, sends the bootstrap state, then registers the
// client for live broadcasts. This keeps runtime deltas from racing ahead of
// the initial ready/snapshot message.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if !security.OriginAllowed(r, h.allowedOrigins) {
		http.Error(w, "websocket origin not allowed", http.StatusForbidden)
		return
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade failed: %v", err)
		return
	}

	client := &Client{
		hub:           h.hub,
		conn:          conn,
		send:          make(chan []byte, sendBufferSize),
		overviewSend:  make(chan []byte, overviewBufferSize),
		hello:         make(chan clientControlMessage, clientHelloBuffer),
		disconnected:  make(chan struct{}),
		bootstrapping: true,
	}

	hello, hasHello := clientHelloFromRequest(r)
	client.usesHTTPRuntimeBootstrap = hasHello
	bootstrapSelection := client.beginBootstrapSnapshotSelection()
	h.hub.addClient(client)
	go client.readPump()

	snapshot := EmptySnapshot()
	version := uint64(0)
	if h.snapshotFunc != nil {
		snapshot, version = h.snapshotFunc()
		snapshot = CloneSnapshot(snapshot)
	}

	connected := true
	helloTimedOut := false
	if !hasHello {
		hello, hasHello, connected, helloTimedOut = waitForClientHello(client)
		if !connected {
			return
		}
	}
	runtimeIdentity := RuntimeIdentityForSnapshot(snapshot)
	bootstrapSelection = client.markBootstrapSnapshotSelected(bootstrapSelection, version, runtimeIdentity)

	alerts := AlertMessagePayload{Alerts: []AlertDTO{}}
	if h.alertsFunc != nil {
		alerts = h.alertsFunc()
	}

	var bootstrapMessage Message
	bootstrapDecision := MessageTypeSnapshot
	bootstrapResyncReason := ""
	versionMatch := hasHello && hello.RuntimeVersion != nil && *hello.RuntimeVersion == version
	identityMatch := hasHello && hello.RuntimeIdentity != "" && hello.RuntimeIdentity == runtimeIdentity
	if hasHello && clientRuntimeCurrent(hello, version, runtimeIdentity) {
		bootstrapMessage = NewReadyMessage(version, alerts.Version, runtimeIdentity)
		bootstrapDecision = MessageTypeReady
	} else if hasHello {
		bootstrapResyncReason = ResyncReasonClientMissingRuntimeSnapshot
		bootstrapMessage = Message{
			Type: MessageTypeResyncRequired,
			Payload: ResyncRequiredPayload{
				Scope:  ResyncScopeOverview,
				Reason: bootstrapResyncReason,
			},
		}
		bootstrapDecision = MessageTypeResyncRequired
	} else {
		bootstrapMessage = NewSnapshotMessage(snapshot, version)
	}
	logging.Debugf(
		"websocket bootstrap decision=%s hello_timeout=%t hello_wait_ms=%d has_hello=%t version_match=%t identity_match=%t hello_runtime_version=%s server_runtime_version=%d snapshot_devices=%d snapshot_links=%d alert_version=%d",
		bootstrapDecision,
		helloTimedOut,
		bootstrapHelloWait.Milliseconds(),
		hasHello,
		versionMatch,
		identityMatch,
		debugRuntimeVersion(hello.RuntimeVersion),
		version,
		len(snapshot.Devices),
		len(snapshot.Links),
		alerts.Version,
	)

	bootstrapMessageWritten := true
	bootstrapWriteOK := false
	if bootstrapDecision == MessageTypeResyncRequired {
		bootstrapMessageWritten, bootstrapWriteOK = h.hub.writeHTTPRuntimeResync(
			client,
			bootstrapMessage,
			bootstrapSelection,
		)
	} else {
		bootstrapWriteOK = h.hub.WriteTo(client, bootstrapMessage)
	}
	if !bootstrapWriteOK {
		return
	}
	if bootstrapDecision == MessageTypeResyncRequired && bootstrapMessageWritten {
		h.hub.recordOverviewResyncRequired(bootstrapResyncReason, true)
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

	if !h.writeRuntimeCatchUp(client, version, runtimeIdentity, bootstrapSelection, hasHello) {
		return
	}

	select {
	case <-client.disconnected:
		return
	default:
	}
	go client.writePump()
}

func (h *Handler) writeRuntimeCatchUp(client *Client, selectedVersion uint64, selectedIdentity string, selection overviewBootstrapSelection, hasHello bool) bool {
	if h.snapshotFunc == nil {
		return true
	}
	snapshot, version := h.snapshotFunc()
	snapshot = CloneSnapshot(snapshot)
	identity := RuntimeIdentityForSnapshot(snapshot)
	if version == selectedVersion && identity == selectedIdentity {
		return true
	}
	if client.overviewChangedSince(selection) {
		return true
	}

	if hasHello {
		reason := ResyncReasonClientResync
		written, ok := h.hub.writeHTTPRuntimeResync(
			client,
			Message{
				Type: MessageTypeResyncRequired,
				Payload: ResyncRequiredPayload{
					Scope:  ResyncScopeOverview,
					Reason: reason,
				},
			},
			selection,
		)
		if written {
			h.hub.recordOverviewResyncRequired(reason, true)
		}
		return ok
	}
	return h.hub.WriteTo(client, NewSnapshotMessage(snapshot, version))
}

func clientRuntimeCurrent(hello clientControlMessage, runtimeVersion uint64, runtimeIdentity string) bool {
	if hello.RuntimeVersion != nil && *hello.RuntimeVersion == runtimeVersion {
		return true
	}
	return hello.RuntimeIdentity != "" && hello.RuntimeIdentity == runtimeIdentity
}

func debugRuntimeVersion(version *uint64) string {
	if version == nil {
		return "-"
	}
	return fmt.Sprintf("%d", *version)
}

func clientHelloFromRequest(r *http.Request) (clientControlMessage, bool) {
	query := r.URL.Query()
	if query.Get("canvas_schema_version") == "" &&
		query.Get("runtime_protocol") == "" &&
		query.Get("runtime_stream_id") == "" &&
		query.Get("topology_version") == "" &&
		query.Get("runtime_version") == "" &&
		query.Get("runtime_identity") == "" &&
		query.Get("alert_version") == "" {
		return clientControlMessage{}, false
	}

	hello := clientControlMessage{
		Type:            MessageTypeHello,
		TopologyVersion: query.Get("topology_version"),
		RuntimeIdentity: query.Get("runtime_identity"),
	}

	if schemaVersion := query.Get("canvas_schema_version"); schemaVersion != "" {
		if parsed, err := strconv.Atoi(schemaVersion); err == nil {
			hello.CanvasSchemaVersion = parsed
		}
	}
	if runtimeProtocol := query.Get("runtime_protocol"); runtimeProtocol != "" {
		if parsed, err := strconv.Atoi(runtimeProtocol); err == nil {
			hello.RuntimeProtocol = parsed
		}
	}

	if runtimeVersion := query.Get("runtime_version"); runtimeVersion != "" {
		if parsed, err := strconv.ParseUint(runtimeVersion, 10, 64); err == nil {
			hello.RuntimeVersion = &parsed
		}
	}

	if alertVersion := query.Get("alert_version"); alertVersion != "" {
		if parsed, err := strconv.ParseUint(alertVersion, 10, 64); err == nil {
			hello.AlertVersion = &parsed
		}
	}
	hello.RuntimeCursor = runtimeCursor(query.Get("runtime_stream_id"), hello.RuntimeVersion)

	return hello, true
}

func waitForClientHello(client *Client) (clientControlMessage, bool, bool, bool) {
	if client.hello == nil {
		return clientControlMessage{}, false, true, false
	}

	timer := time.NewTimer(bootstrapHelloWait)
	defer timer.Stop()

	select {
	case hello := <-client.hello:
		return hello, true, true, false
	case <-client.disconnected:
		return clientControlMessage{}, false, false, false
	case <-timer.C:
		return clientControlMessage{}, false, true, true
	}
}
