package ws

// BroadcastCh returns the broadcast channel for test inspection.
// The channel is buffered (capacity 32); in tests without hub.Run(), messages accumulate.
func (h *Hub) BroadcastCh() chan []byte {
	return h.broadcast
}
