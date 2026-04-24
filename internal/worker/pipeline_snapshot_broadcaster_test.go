package worker

import (
	"strings"
	"testing"

	"github.com/lollinoo/theia/internal/polling"
	"github.com/lollinoo/theia/internal/ws"
)

func TestPipelineSnapshotBroadcasterBroadcastsPollingHealthOnlyWhenChanged(t *testing.T) {
	sched := newPipelineTestScheduler()
	pipeline := NewPipelineOrchestrator(sched, nil, nil, ws.NewHub(), nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	broadcaster := &pipelineSnapshotBroadcaster{pipeline: pipeline}

	sched.health = polling.HealthSnapshot{
		EssentialOverloaded: true,
		ConfiguredWorkers:   64,
		ActiveWorkers:       64,
	}
	broadcaster.broadcastPollingHealthIfChanged()
	first := <-pipeline.hub.broadcast
	if !strings.Contains(string(first), ws.MessageTypePollingHealthChanged) {
		t.Fatalf("expected polling_health_changed broadcast, got %s", string(first))
	}

	broadcaster.broadcastPollingHealthIfChanged()
	select {
	case payload := <-pipeline.hub.broadcast:
		t.Fatalf("unexpected duplicate polling health broadcast: %s", string(payload))
	default:
	}

	sched.health = polling.HealthSnapshot{
		EssentialOverloaded: true,
		ConfiguredWorkers:   64,
		ActiveWorkers:       63,
	}
	broadcaster.broadcastPollingHealthIfChanged()
	third := <-pipeline.hub.broadcast
	if !strings.Contains(string(third), `"active_workers":63`) {
		t.Fatalf("expected updated polling health payload, got %s", string(third))
	}
}
