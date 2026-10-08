package ws

import "testing"

type countedDetailPayload struct{ calls *int }

func (p countedDetailPayload) MarshalJSON() ([]byte, error) {
	*p.calls++
	return []byte(`{"device":"selected"}`), nil
}

func TestSendToManyEncodesOnceAndKeepsClientBackpressure(t *testing.T) {
	hub := NewHub()
	first, second, slow := newObservedTestClient(hub), newObservedTestClient(hub), newObservedTestClient(hub)
	for i := 0; i < cap(slow.send); i++ {
		slow.send <- []byte("full")
	}
	calls := 0
	hub.SendToMany([]*Client{slow, first, second}, Message{Type: MessageTypeSnapshotDelta, Payload: countedDetailPayload{calls: &calls}})
	if calls != 1 {
		t.Fatalf("JSON encodes=%d, want 1", calls)
	}
	a, b := <-first.send, <-second.send
	if &a[0] != &b[0] {
		t.Fatal("subscribers received separate payload allocations")
	}
	if len(slow.send) != cap(slow.send) {
		t.Fatal("slow-client queue bound changed")
	}
}
