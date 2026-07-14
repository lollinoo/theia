package worker

// This file exercises pipeline snapshot broadcaster behavior so refactors preserve the documented contract.

import (
	"bytes"
	"context"
	"log"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/lollinoo/theia/internal/domain"
	"github.com/lollinoo/theia/internal/logging"
	"github.com/lollinoo/theia/internal/polling"
	"github.com/lollinoo/theia/internal/state"
	"github.com/lollinoo/theia/internal/ws"
)

func TestSnapshotBroadcasterJournalBroadcastOnceDeltaKeepsStream(t *testing.T) {
	pipeline, hub, store, _, deviceID := newBroadcastTestPipeline(t)
	pipeline.broadcaster.broadcastOnce(context.Background())
	drainBroadcastCh(hub)
	initial := pipeline.GetOrBuildOverviewState()

	store.Update(state.StateUpdate{
		DeviceID:        deviceID,
		VolatilityClass: domain.VolatilityClassPerformance,
		Metrics: &domain.DeviceMetrics{
			DeviceID:    deviceID,
			CPUPercent:  floatPtr(51),
			CollectedAt: time.Date(2026, 7, 14, 10, 0, 0, 0, time.UTC),
		},
		PollSuccess:      true,
		ExpectedInterval: 30 * time.Second,
		Timestamp:        time.Date(2026, 7, 14, 10, 0, 0, 0, time.UTC),
	})
	pipeline.broadcaster.broadcastOnce(context.Background())

	current := pipeline.GetOrBuildOverviewState()
	if current.StreamID != initial.StreamID {
		t.Fatalf("sparse delta stream ID = %q, want stable %q", current.StreamID, initial.StreamID)
	}
	if current.Version != initial.Version+1 {
		t.Fatalf("sparse delta version = %d, want %d", current.Version, initial.Version+1)
	}

	pipeline.overviewBuildMu.Lock()
	replay, ok := pipeline.runtime.overviewJournal.Replay(initial.Version, current.Version)
	pipeline.overviewBuildMu.Unlock()
	if !ok || replay == nil {
		t.Fatalf("journal replay = (%#v, %t), want sparse delta", replay, ok)
	}
	if _, ok := replay.Devices[deviceID.String()]; !ok {
		t.Fatalf("journal replay does not contain device %s", deviceID)
	}
}

func TestSnapshotBroadcasterJournalDirtyDeltaKeepsStream(t *testing.T) {
	pipeline, hub, store, _, deviceID := newBroadcastTestPipeline(t)
	pipeline.broadcaster.broadcastOnce(context.Background())
	drainBroadcastCh(hub)
	initial := pipeline.GetOrBuildOverviewState()

	store.Update(state.StateUpdate{
		DeviceID:        deviceID,
		VolatilityClass: domain.VolatilityClassPerformance,
		Metrics: &domain.DeviceMetrics{
			DeviceID:    deviceID,
			CPUPercent:  floatPtr(52),
			CollectedAt: time.Date(2026, 7, 14, 10, 1, 0, 0, time.UTC),
		},
		PollSuccess:      true,
		ExpectedInterval: 30 * time.Second,
		Timestamp:        time.Date(2026, 7, 14, 10, 1, 0, 0, time.UTC),
	})
	if err := pipeline.broadcaster.broadcastDirty(
		context.Background(),
		map[uuid.UUID]struct{}{deviceID: {}},
		false,
		false,
		false,
	); err != nil {
		t.Fatalf("broadcastDirty returned error: %v", err)
	}

	current := pipeline.GetOrBuildOverviewState()
	if current.StreamID != initial.StreamID {
		t.Fatalf("dirty delta stream ID = %q, want stable %q", current.StreamID, initial.StreamID)
	}
	if current.Version != initial.Version+1 {
		t.Fatalf("dirty delta version = %d, want %d", current.Version, initial.Version+1)
	}

	pipeline.overviewBuildMu.Lock()
	replay, ok := pipeline.runtime.overviewJournal.Replay(initial.Version, current.Version)
	pipeline.overviewBuildMu.Unlock()
	if !ok || replay == nil {
		t.Fatalf("journal replay = (%#v, %t), want dirty delta", replay, ok)
	}
	if _, ok := replay.Devices[deviceID.String()]; !ok {
		t.Fatalf("journal replay does not contain device %s", deviceID)
	}
}

func TestSnapshotBroadcasterJournalFullSnapshotRotatesStreamAndClearsReplay(t *testing.T) {
	pipeline, hub, store, _, deviceID := newBroadcastTestPipeline(t)
	pipeline.broadcaster.broadcastOnce(context.Background())
	drainBroadcastCh(hub)
	initial := pipeline.GetOrBuildOverviewState()

	store.Update(state.StateUpdate{
		DeviceID:        deviceID,
		VolatilityClass: domain.VolatilityClassPerformance,
		Metrics: &domain.DeviceMetrics{
			DeviceID:    deviceID,
			CPUPercent:  floatPtr(53),
			CollectedAt: time.Date(2026, 7, 14, 10, 2, 0, 0, time.UTC),
		},
		PollSuccess:      true,
		ExpectedInterval: 30 * time.Second,
		Timestamp:        time.Date(2026, 7, 14, 10, 2, 0, 0, time.UTC),
	})
	if err := pipeline.broadcaster.broadcastDirty(
		context.Background(),
		map[uuid.UUID]struct{}{deviceID: {}},
		false,
		false,
		false,
	); err != nil {
		t.Fatalf("broadcastDirty returned error: %v", err)
	}
	beforeFull := pipeline.GetOrBuildOverviewState()

	pipeline.overviewBuildMu.Lock()
	_, replayableBefore := pipeline.runtime.overviewJournal.Replay(initial.Version, beforeFull.Version)
	pipeline.overviewBuildMu.Unlock()
	if !replayableBefore {
		t.Fatal("expected sparse delta to be replayable before full snapshot")
	}

	if err := pipeline.broadcaster.broadcastFullSnapshot(context.Background(), refreshReloadReasonFullResync, false); err != nil {
		t.Fatalf("broadcastFullSnapshot returned error: %v", err)
	}
	afterFull := pipeline.GetOrBuildOverviewState()
	if afterFull.StreamID == beforeFull.StreamID {
		t.Fatalf("full snapshot kept stream ID %q", afterFull.StreamID)
	}
	if _, err := uuid.Parse(afterFull.StreamID); err != nil {
		t.Fatalf("rotated stream ID %q is not a UUID: %v", afterFull.StreamID, err)
	}
	if afterFull.Version != beforeFull.Version+1 {
		t.Fatalf("full snapshot version = %d, want %d", afterFull.Version, beforeFull.Version+1)
	}

	pipeline.overviewBuildMu.Lock()
	replay, ok := pipeline.runtime.overviewJournal.Replay(initial.Version, beforeFull.Version)
	entries := len(pipeline.runtime.overviewJournal.entries)
	pipeline.overviewBuildMu.Unlock()
	if ok || replay != nil {
		t.Fatalf("old stream replay survived full snapshot: (%#v, %t)", replay, ok)
	}
	if entries != 0 {
		t.Fatalf("journal contains %d entries after full snapshot, want 0", entries)
	}
}

func TestRuntimeRecoveryRaceAck92Recovery94KeepsDelta95BehindBarrier(t *testing.T) {
	hub := ws.NewHub()
	go hub.Run()
	pipeline := NewPipelineOrchestrator(
		newPipelineTestScheduler(),
		nil,
		nil,
		hub,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
	)
	configureRuntimeRecoveryState(t, pipeline, "runtime-stream-1", 92)

	conn, client, releaseWritePump := attachRuntimeRecoveryClient(
		t,
		hub,
		pipeline.GetOverviewSnapshot,
		ws.RuntimeStreamProtocolVersion,
		ws.RuntimeCursor{StreamID: "runtime-stream-1", Version: 92, Known: true},
		true,
	)
	defer releaseWritePump()

	advanceRuntimeRecoveryState(t, pipeline, 92, 93, &ws.RuntimeDeltaPayload{
		Devices: map[string]map[string]any{"dev-1": {"primary_health": "degraded"}},
		Links:   map[string]map[string]any{},
	})
	advanceRuntimeRecoveryState(t, pipeline, 93, 94, &ws.RuntimeDeltaPayload{
		Devices: map[string]map[string]any{"dev-1": {"primary_health": "up_fresh"}},
		Links:   map[string]map[string]any{},
	})

	pipeline.SyncOverviewClient(client, ws.RuntimeSyncRequest{
		Cursor: ws.RuntimeCursor{StreamID: "runtime-stream-1", Version: 92, Known: true},
		Reason: ws.ResyncReasonClientResync,
	})
	advanceRuntimeRecoveryState(t, pipeline, 94, 95, &ws.RuntimeDeltaPayload{
		Devices: map[string]map[string]any{"dev-1": {"primary_health": "up"}},
		Links:   map[string]map[string]any{},
	})

	releaseWritePump()
	messages := readRuntimeRecoveryMessages(t, conn, 4)
	assertRuntimeRecoveryMessageTypes(
		t,
		messages,
		ws.MessageTypeResyncRequired,
		ws.MessageTypeRuntimeReplay,
		ws.MessageTypeReady,
		ws.MessageTypeRuntimeDelta,
	)

	var replay ws.RuntimeReplayMessagePayload
	decodeRuntimeRecoveryPayload(t, messages[1], &replay)
	if replay.FromVersion != 92 || replay.Version != 94 || replay.RuntimeStreamID != "runtime-stream-1" {
		t.Fatalf("recovery replay = %#v, want runtime-stream-1 92->94", replay)
	}
	var ready ws.ReadyPayload
	decodeRuntimeRecoveryPayload(t, messages[2], &ready)
	if ready.RuntimeVersion != 94 || ready.RuntimeStreamID != "runtime-stream-1" || ready.SyncMode != string(ws.OverviewSyncModeReplay) {
		t.Fatalf("recovery ready = %#v, want replay barrier runtime-stream-1/94", ready)
	}
	var next ws.RuntimeDeltaMessagePayload
	decodeRuntimeRecoveryPayload(t, messages[3], &next)
	if next.BaseVersion != 94 || next.Version != 95 || next.RuntimeStreamID != "runtime-stream-1" {
		t.Fatalf("post-recovery delta = %#v, want runtime-stream-1 94->95", next)
	}

	resyncCount := 0
	for _, message := range messages {
		if message.Type == ws.MessageTypeResyncRequired {
			resyncCount++
		}
	}
	if resyncCount != 1 {
		t.Fatalf("resync_required count = %d, want one recovery cycle", resyncCount)
	}
}

func TestOverviewMailboxRecoveryFullSnapshotRotatesStreamAndInstallsCompleteBatch(t *testing.T) {
	pipeline, hub, _, _, _ := newNoClientBroadcastTestPipeline(t)
	go hub.Run()
	pipeline.broadcaster.broadcastOnce(context.Background())
	initial := pipeline.GetOrBuildOverviewState()

	conn, _, releaseWritePump := attachRuntimeRecoveryClient(
		t,
		hub,
		pipeline.GetOverviewSnapshot,
		ws.RuntimeStreamProtocolVersion,
		ws.RuntimeCursor{StreamID: initial.StreamID, Version: initial.Version, Known: true},
		true,
	)
	defer releaseWritePump()

	if err := pipeline.broadcaster.broadcastFullSnapshot(context.Background(), refreshReloadReasonFullResync, false); err != nil {
		t.Fatalf("broadcastFullSnapshot returned error: %v", err)
	}
	current := pipeline.GetOrBuildOverviewState()
	if current.StreamID == initial.StreamID {
		t.Fatalf("full snapshot kept runtime stream %q", current.StreamID)
	}
	if current.Version != initial.Version+1 {
		t.Fatalf("full snapshot version = %d, want %d", current.Version, initial.Version+1)
	}

	releaseWritePump()
	messages := readRuntimeRecoveryMessages(t, conn, 3)
	assertRuntimeRecoveryMessageTypes(t, messages, ws.MessageTypeResyncRequired, ws.MessageTypeSnapshot, ws.MessageTypeReady)
	var snapshot ws.SnapshotMessagePayload
	decodeRuntimeRecoveryPayload(t, messages[1], &snapshot)
	if snapshot.RuntimeStreamID != current.StreamID || snapshot.Version != current.Version {
		t.Fatalf("full snapshot payload = %#v, want stream/version %s/%d", snapshot, current.StreamID, current.Version)
	}
	var ready ws.ReadyPayload
	decodeRuntimeRecoveryPayload(t, messages[2], &ready)
	if ready.RuntimeStreamID != current.StreamID || ready.RuntimeVersion != current.Version || ready.SyncMode != string(ws.OverviewSyncModeSnapshot) {
		t.Fatalf("full snapshot ready = %#v, want snapshot barrier %s/%d", ready, current.StreamID, current.Version)
	}
}

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
	pipeline := NewPipelineOrchestrator(sched, nil, nil, ws.NewHub(ws.WithBroadcastRecorder()), nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
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
	pipeline := NewPipelineOrchestrator(sched, nil, nil, ws.NewHub(ws.WithBroadcastRecorder()), nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
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

func TestPipelinePrometheusMonitor_DebugLogsStatusTransition(t *testing.T) {
	logs := capturePipelineDebugLogs(t)
	pipeline := NewPipelineOrchestrator(newPipelineTestScheduler(), nil, nil, ws.NewHub(ws.WithBroadcastRecorder()), nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	monitor := &pipelinePrometheusMonitor{pipeline: pipeline}

	monitor.publishStatus(ws.PrometheusStatusPayload{
		Enabled:   true,
		Available: false,
		Error:     "dial tcp: connection refused",
	})

	output := logs.String()
	if !strings.Contains(output, "DEBUG prometheus status changed enabled=true available=false") {
		t.Fatalf("debug output missing Prometheus transition: %q", output)
	}
	if !strings.Contains(output, "error_set=true") {
		t.Fatalf("debug output missing sanitized error state: %q", output)
	}
	if strings.Contains(output, "connection refused") {
		t.Fatalf("debug output should not include raw Prometheus error: %q", output)
	}
}

func TestPipelineSnapshotBroadcasterThrottlesActiveWorkerOnlyPollingHealth(t *testing.T) {
	sched := newPipelineTestScheduler()
	pipeline := NewPipelineOrchestrator(sched, nil, nil, ws.NewHub(ws.WithBroadcastRecorder()), nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
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
	pipeline := NewPipelineOrchestrator(sched, nil, nil, ws.NewHub(ws.WithBroadcastRecorder()), nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
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
	pipeline := NewPipelineOrchestrator(sched, nil, nil, ws.NewHub(ws.WithBroadcastRecorder()), nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
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
