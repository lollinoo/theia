package worker

import (
	"bytes"
	"log"
	"strings"
	"testing"
	"time"

	"github.com/lollinoo/theia/internal/logging"
	"github.com/lollinoo/theia/internal/polling"
	"github.com/lollinoo/theia/internal/ws"
)

func capturePipelineDebugLogs(t *testing.T) *bytes.Buffer {
	t.Helper()

	var buf bytes.Buffer
	originalWriter := log.Writer()
	originalFlags := log.Flags()
	log.SetOutput(&buf)
	log.SetFlags(0)
	logging.Configure("debug")
	t.Cleanup(func() {
		logging.Configure("info")
		log.SetOutput(originalWriter)
		log.SetFlags(originalFlags)
	})
	return &buf
}

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

func TestPipelineSnapshotBroadcasterDebugLogsPollingHealthChange(t *testing.T) {
	logs := capturePipelineDebugLogs(t)
	sched := newPipelineTestScheduler()
	pipeline := NewPipelineOrchestrator(sched, nil, nil, ws.NewHub(), nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	broadcaster := &pipelineSnapshotBroadcaster{pipeline: pipeline}

	sched.health = polling.HealthSnapshot{
		EssentialOverloaded:      true,
		DegradedRisk:             true,
		EssentialQueueLagSeconds: 12.5,
		DeadlineMissTotal:        3,
		ConfiguredWorkers:        64,
		ActiveWorkers:            61,
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
	<-pipeline.hub.BroadcastCh()

	output := logs.String()
	if !strings.Contains(output, "DEBUG polling health changed") {
		t.Fatalf("debug output missing polling health change: %q", output)
	}
	if !strings.Contains(output, "deadline_miss_total=3") {
		t.Fatalf("debug output missing deadline miss total: %q", output)
	}
	if !strings.Contains(output, "performance ready=7 lag=125.1s active=32/32") {
		t.Fatalf("debug output missing queue summary: %q", output)
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
