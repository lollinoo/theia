package worker

import (
	"strings"
	"testing"
	"time"

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
	select {
	case payload := <-pipeline.hub.BroadcastCh():
		t.Fatalf("unexpected polling health broadcast for active worker drift: %s", string(payload))
	default:
	}

	sched.health = polling.HealthSnapshot{
		EssentialOverloaded: true,
		ConfiguredWorkers:   65,
		ActiveWorkers:       63,
	}
	broadcaster.broadcastPollingHealthIfChanged()
	third := <-pipeline.hub.BroadcastCh()
	if !strings.Contains(string(third), `"configured_workers":65`) {
		t.Fatalf("expected updated polling health payload, got %s", string(third))
	}
}

func TestPipelineSnapshotBroadcasterThrottlesActiveWorkerOnlyPollingHealth(t *testing.T) {
	sched := newPipelineTestScheduler()
	pipeline := NewPipelineOrchestrator(sched, nil, nil, ws.NewHub(), nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	broadcaster := &pipelineSnapshotBroadcaster{pipeline: pipeline}

	now := time.Date(2026, 5, 1, 10, 0, 0, 0, time.UTC)
	pipeline.runtime.now = func() time.Time { return now }

	sched.health = polling.HealthSnapshot{
		ConfiguredWorkers: 64,
		ActiveWorkers:     10,
	}
	broadcaster.broadcastPollingHealthIfChanged()
	<-pipeline.hub.BroadcastCh()

	sched.health = polling.HealthSnapshot{
		ConfiguredWorkers: 64,
		ActiveWorkers:     11,
	}
	now = now.Add(time.Minute - time.Second)
	broadcaster.broadcastPollingHealthIfChanged()
	select {
	case payload := <-pipeline.hub.BroadcastCh():
		t.Fatalf("unexpected polling health heartbeat before interval: %s", string(payload))
	default:
	}

	now = now.Add(time.Second)
	broadcaster.broadcastPollingHealthIfChanged()
	select {
	case payload := <-pipeline.hub.BroadcastCh():
		if !strings.Contains(string(payload), `"active_workers":11`) {
			t.Fatalf("expected throttled active worker polling health payload, got %s", string(payload))
		}
	default:
		t.Fatalf("expected throttled active worker polling health payload")
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

func TestPipelineSnapshotBroadcasterBroadcastsPollingHealthWhenClassQueueLagBucketChanges(t *testing.T) {
	sched := newPipelineTestScheduler()
	pipeline := NewPipelineOrchestrator(sched, nil, nil, ws.NewHub(), nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	broadcaster := &pipelineSnapshotBroadcaster{pipeline: pipeline}

	sched.health = polling.HealthSnapshot{
		ConfiguredWorkers: 64,
		Queues: map[string]polling.QueueSnapshot{
			"performance": {
				ReadyDepth:        7,
				LagSeconds:        120.1,
				ActiveWorkers:     32,
				ConfiguredWorkers: 32,
			},
		},
	}
	broadcaster.broadcastPollingHealthIfChanged()
	<-pipeline.hub.BroadcastCh()

	sched.health = polling.HealthSnapshot{
		ConfiguredWorkers: 64,
		Queues: map[string]polling.QueueSnapshot{
			"performance": {
				ReadyDepth:        7,
				LagSeconds:        124.9,
				ActiveWorkers:     32,
				ConfiguredWorkers: 32,
			},
		},
	}
	broadcaster.broadcastPollingHealthIfChanged()
	select {
	case payload := <-pipeline.hub.BroadcastCh():
		t.Fatalf("unexpected polling health broadcast for sub-bucket performance queue lag drift: %s", string(payload))
	default:
	}

	sched.health = polling.HealthSnapshot{
		ConfiguredWorkers: 64,
		Queues: map[string]polling.QueueSnapshot{
			"performance": {
				ReadyDepth:        7,
				LagSeconds:        125.1,
				ActiveWorkers:     32,
				ConfiguredWorkers: 32,
			},
		},
	}
	broadcaster.broadcastPollingHealthIfChanged()
	payload := <-pipeline.hub.BroadcastCh()
	if !strings.Contains(string(payload), `"lag_seconds":125.1`) {
		t.Fatalf("expected class queue bucket-crossing polling health payload, got %s", string(payload))
	}
}
