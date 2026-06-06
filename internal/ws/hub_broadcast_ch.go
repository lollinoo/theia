package ws

// This file defines hub broadcast ch WebSocket protocol behavior, subscriptions, and runtime update delivery.

// BroadcastCh returns the broadcast channel for test inspection.
// Calling it enables an opt-in recorder; production hubs do not retain
// broadcast payloads unless this test seam is used.
func (h *Hub) BroadcastCh() chan []byte {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.broadcast == nil {
		h.broadcast = make(chan []byte, 32)
	}
	return h.broadcast
}
