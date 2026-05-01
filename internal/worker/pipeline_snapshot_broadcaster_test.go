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
	first := <-pipeline.hub.BroadcastCh()
	if !strings.Contains(string(first), ws.MessageTypePollingHealthChanged) {
		t.Fatalf("expected polling_health_changed broadcast, got %s", string(first))
	}

	broadcaster.broadcastPollingHealthIfChanged()
	select {
	case payload := <-pipeline.hub.BroadcastCh():
		t.Fatalf("unexpected duplicate polling health broadcast: %s", string(payload))
	default:
	}

	sched.health = polling.HealthSnapshot{
		EssentialOverloaded: true,
		ConfiguredWorkers:   64,
		ActiveWorkers:       63,
	}
	broadcaster.broadcastPollingHealthIfChanged()
	third := <-pipeline.hub.BroadcastCh()
	if !strings.Contains(string(third), `"active_workers":63`) {
		t.Fatalf("expected updated polling health payload, got %s", string(third))
	}
}

func TestPipelineSnapshotBroadcasterIgnoresSubBucketPollingHealthLagDrift(t *testing.T) {
	sched := newPipelineTestScheduler()
	pipeline := NewPipelineOrchestrator(sched, nil, nil, ws.NewHub(), nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	broadcaster := &pipelineSnapshotBroadcaster{pipeline: pipeline}

	sched.health = polling.HealthSnapshot{
		EssentialQueueLagSeconds: 120.1,
		ConfiguredWorkers:        64,
	}
	broadcaster.broadcastPollingHealthIfChanged()
	<-pipeline.hub.BroadcastCh()

	sched.health = polling.HealthSnapshot{
		EssentialQueueLagSeconds: 124.9,
		ConfiguredWorkers:        64,
	}
	broadcaster.broadcastPollingHealthIfChanged()
	select {
	case payload := <-pipeline.hub.BroadcastCh():
		t.Fatalf("unexpected polling health broadcast for sub-bucket lag drift: %s", string(payload))
	default:
	}

	sched.health = polling.HealthSnapshot{
		EssentialQueueLagSeconds: 125.1,
		ConfiguredWorkers:        64,
	}
	broadcaster.broadcastPollingHealthIfChanged()
	payload := <-pipeline.hub.BroadcastCh()
	if !strings.Contains(string(payload), `"essential_queue_lag_seconds":125.1`) {
		t.Fatalf("expected bucket-crossing polling health payload, got %s", string(payload))
	}
}
