package worker

// This file defines pipeline worker behavior, background lifecycle, and runtime state updates.

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"

	"github.com/lollinoo/theia/internal/cache"
	"github.com/lollinoo/theia/internal/collector"
	"github.com/lollinoo/theia/internal/domain"
	"github.com/lollinoo/theia/internal/logging"
	"github.com/lollinoo/theia/internal/observability"
	"github.com/lollinoo/theia/internal/polling"
	"github.com/lollinoo/theia/internal/pollingbudget"
	"github.com/lollinoo/theia/internal/scheduler"
	"github.com/lollinoo/theia/internal/service"
	"github.com/lollinoo/theia/internal/state"
	"github.com/lollinoo/theia/internal/ws"
)

const (
	pipelineBroadcastCoalesceWindow = 250 * time.Millisecond
	// Disabled by default: overview clients now resync on connect or explicit
	// degradation signals instead of periodic forced full snapshots.
	pipelineFullResyncInterval    = 0 * time.Second
	prometheusEnrichmentRetention = 30 * time.Second

	refreshSnapshotModeDirty = "dirty"
	refreshSnapshotModeFull  = "full"

	refreshReloadReasonStartup               = "startup"
	refreshReloadReasonTopologyDirty         = "topology_dirty"
	refreshReloadReasonFullResync            = "full_resync"
	refreshReloadReasonDirtyDeltaFallback    = "dirty_delta_fallback"
	refreshReloadReasonTopologyDrainFallback = "topology_drain_fallback"
	refreshReloadReasonStateChangesDropped   = ws.ResyncReasonStateChangesDrop
	refreshReloadReasonHubBufferFull         = ws.ResyncReasonHubBufferFull

	runtimeRecoveryAttemptLimit      = 4096
	defaultRuntimeRecoveryAttemptTTL = 2 * time.Minute
	minimumRuntimeRecoveryTimerDelay = time.Millisecond
)

type pipelineScheduler interface {
	Start(context.Context) error
	Stop()
	Tasks() <-chan scheduler.PollTask
	Complete(scheduler.Completion)
	Status() string
	PollingHealth() polling.HealthSnapshot
}

// ErrAlreadyStarted stores shared err already started state for the background worker lifecycle.
var ErrAlreadyStarted = errors.New("pipeline orchestrator: already started")

type pipelineTaskRunning interface {
	runWorker(context.Context)
	runTask(context.Context, scheduler.PollTask)
	topologyDiscoveryMode(domain.Device) domain.TopologyDiscoveryMode
	publishSubscribedDetailDelta(domain.Device)
}

// PipelineOrchestrator represents pipeline orchestrator data used by the background worker lifecycle.
type PipelineOrchestrator struct {
	scheduler         pipelineScheduler
	taskRunner        pipelineTaskRunning
	broadcaster       *pipelineSnapshotBroadcaster
	prometheusMonitor *pipelinePrometheusMonitor
	stateStore        *state.Store
	cache             *cache.DeviceLinkCache
	hub               *ws.Hub
	essential         *collector.EssentialCollector
	performance       *collector.PerformanceCollector
	operational       *collector.OperationalCollector
	staticCollector   *collector.StaticCollector
	prometheus        *collector.PrometheusCollector
	topologyService   interface {
		ApplyStaticDiscovery(uuid.UUID, service.StaticDiscoveryInput) (service.StaticPersistenceResult, error)
	}
	settingsRepo            domain.SettingsRepository
	topologyNotify          chan struct{}
	deviceChangeNotify      <-chan domain.DeviceChangeEvent
	linkChangeNotify        <-chan domain.LinkChangeEvent
	alertNotify             chan struct{}
	broadcastCoalesceWindow time.Duration
	fullResyncInterval      time.Duration
	lifecycleMu             sync.Mutex
	stopping                atomic.Bool
	stopDone                chan struct{}
	runtimeCallbacksBlocked atomic.Bool
	running                 atomic.Bool
	cancel                  context.CancelFunc
	done                    chan struct{}
	healthDone              chan struct{}
	runtime                 *pipelineRuntimeState
	overviewBuildMu         sync.Mutex
	runtimeRecoveryAttempts map[*ws.Client]runtimeRecoveryAttempt
	runtimeRecoveryTTL      time.Duration
	runtimeRecoveryTimer    *time.Timer
	runtimeRecoveryTimerGen uint64
	staticPersistenceMu     sync.Mutex
	staticPersistenceCache  map[uuid.UUID]staticPersistenceCacheEntry
	staticPersistenceNow    func() time.Time
}

type runtimeRecoveryAttempt struct {
	mode          string
	reason        string
	streamID      string
	targetVersion uint64
	startedAt     time.Time
}

type staticPersistenceCacheEntry struct {
	fingerprint                 string
	topologyFingerprint         string
	persistedAt                 time.Time
	topologyMaterializedAt      time.Time
	topologyUnresolvedNeighbors int
}

// NewPipelineOrchestrator constructs pipeline orchestrator state for the background worker lifecycle.
func NewPipelineOrchestrator(
	sched pipelineScheduler,
	stateStore *state.Store,
	cache *cache.DeviceLinkCache,
	hub *ws.Hub,
	essential *collector.EssentialCollector,
	performance *collector.PerformanceCollector,
	operational *collector.OperationalCollector,
	staticCollector *collector.StaticCollector,
	prometheus *collector.PrometheusCollector,
	topologyService interface {
		ApplyStaticDiscovery(uuid.UUID, service.StaticDiscoveryInput) (service.StaticPersistenceResult, error)
	},
	settingsRepo domain.SettingsRepository,
	topologyNotify chan struct{},
	deviceChangeNotify <-chan domain.DeviceChangeEvent,
	linkChangeNotify <-chan domain.LinkChangeEvent,
) *PipelineOrchestrator {
	p := &PipelineOrchestrator{
		scheduler:               sched,
		stateStore:              stateStore,
		cache:                   cache,
		hub:                     hub,
		essential:               essential,
		performance:             performance,
		operational:             operational,
		staticCollector:         staticCollector,
		prometheus:              prometheus,
		topologyService:         topologyService,
		settingsRepo:            settingsRepo,
		topologyNotify:          topologyNotify,
		deviceChangeNotify:      deviceChangeNotify,
		linkChangeNotify:        linkChangeNotify,
		alertNotify:             make(chan struct{}, 1),
		broadcastCoalesceWindow: pollingPolicyFromSettings(settingsRepo).WebSocketCoalesce,
		fullResyncInterval:      pipelineFullResyncInterval,
		done:                    make(chan struct{}),
		healthDone:              make(chan struct{}),
		runtime:                 newPipelineRuntimeState(initialPrometheusStatus(settingsRepo)),
		runtimeRecoveryAttempts: make(map[*ws.Client]runtimeRecoveryAttempt),
		runtimeRecoveryTTL:      defaultRuntimeRecoveryAttemptTTL,
		staticPersistenceNow:    time.Now,
	}
	p.taskRunner = &pipelineTaskRunner{pipeline: p}
	p.broadcaster = &pipelineSnapshotBroadcaster{pipeline: p}
	p.prometheusMonitor = &pipelinePrometheusMonitor{pipeline: p}
	return p
}

func (p *PipelineOrchestrator) Start(ctx context.Context) error {
	p.lifecycleMu.Lock()
	defer p.lifecycleMu.Unlock()
	if p.cancel != nil || p.stopping.Load() {
		return ErrAlreadyStarted
	}

	runCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	healthDone := make(chan struct{})

	p.cancel = cancel
	p.done = done
	p.healthDone = healthDone

	if p.stateStore != nil {
		if err := p.stateStore.Start(runCtx); err != nil {
			cancel()
			p.cancel = nil
			p.done = make(chan struct{})
			p.healthDone = make(chan struct{})
			return err
		}
	}
	if p.scheduler != nil {
		if err := p.scheduler.Start(runCtx); err != nil {
			if p.stateStore != nil {
				p.stateStore.Stop()
			}
			cancel()
			p.cancel = nil
			p.done = make(chan struct{})
			p.healthDone = make(chan struct{})
			return err
		}
	}
	p.runtimeCallbacksBlocked.Store(false)
	p.running.Store(true)

	go func() {
		defer close(done)
		var wg sync.WaitGroup

		for i := 0; i < p.workerCount(); i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				p.taskRunner.runWorker(runCtx)
			}()
		}

		wg.Add(1)
		go func() {
			defer wg.Done()
			p.prometheusMonitor.run(runCtx)
		}()

		wg.Add(1)
		go func() {
			defer wg.Done()
			p.broadcaster.broadcastLoop(runCtx)
		}()

		<-runCtx.Done()
		if p.scheduler != nil {
			p.scheduler.Stop()
		}
		if p.stateStore != nil {
			p.stateStore.Stop()
		}
		wg.Wait()

		p.runtimeCallbacksBlocked.Store(true)
		p.clearRuntimeRecoveryTracking()
		p.running.Store(false)
		p.lifecycleMu.Lock()
		if p.done == done && !p.stopping.Load() {
			p.cancel = nil
		}
		p.lifecycleMu.Unlock()
	}()

	return nil
}

func (p *PipelineOrchestrator) Stop() {
	if p == nil {
		return
	}
	p.lifecycleMu.Lock()
	if p.stopping.Load() {
		stopDone := p.stopDone
		p.lifecycleMu.Unlock()
		if stopDone != nil {
			<-stopDone
		}
		return
	}
	p.stopping.Store(true)
	p.runtimeCallbacksBlocked.Store(true)
	stopDone := make(chan struct{})
	p.stopDone = stopDone
	if p.cancel == nil {
		p.lifecycleMu.Unlock()
		p.clearRuntimeRecoveryTracking()
		p.finishStop(stopDone, nil)
		return
	}
	cancel := p.cancel
	done := p.done
	healthDone := p.healthDone
	p.lifecycleMu.Unlock()

	cancel()
	p.clearRuntimeRecoveryTracking()
	<-done
	<-healthDone
	p.clearRuntimeRecoveryTracking()
	p.finishStop(stopDone, done)
}

// finishStop publishes lifecycle completion only after recovery teardown.
func (p *PipelineOrchestrator) finishStop(stopDone chan struct{}, runDone chan struct{}) {
	p.lifecycleMu.Lock()
	defer p.lifecycleMu.Unlock()
	if runDone == nil || p.done == runDone {
		p.cancel = nil
	}
	p.running.Store(false)
	p.stopping.Store(false)
	p.stopDone = nil
	close(stopDone)
}

// clearRuntimeRecoveryTracking invalidates pending callbacks without treating
// process shutdown as a failed client recovery.
func (p *PipelineOrchestrator) clearRuntimeRecoveryTracking() {
	p.overviewBuildMu.Lock()
	defer p.overviewBuildMu.Unlock()
	p.runtimeRecoveryTimerGen++
	if p.runtimeRecoveryTimer != nil {
		p.runtimeRecoveryTimer.Stop()
		p.runtimeRecoveryTimer = nil
	}
	clear(p.runtimeRecoveryAttempts)
}

// GetOverviewSnapshot retrieves overview snapshot data from the background worker lifecycle.
func (p *PipelineOrchestrator) GetOverviewSnapshot() (*ws.SnapshotPayload, uint64) {
	return p.runtime.getOverviewSnapshot()
}

// GetOrBuildOverviewSnapshot retrieves or builds overview snapshot data from the background worker lifecycle.
func (p *PipelineOrchestrator) GetOrBuildOverviewSnapshot() (*ws.SnapshotPayload, uint64) {
	state := p.GetOrBuildOverviewState()
	return state.Snapshot, state.Version
}

// GetOrBuildOverviewState retrieves or builds one atomic overview snapshot lineage state.
func (p *PipelineOrchestrator) GetOrBuildOverviewState() ws.RuntimeOverviewState {
	p.overviewBuildMu.Lock()
	defer p.overviewBuildMu.Unlock()
	return p.getOrBuildOverviewStateLocked()
}

// SyncOverviewClient selects and installs one atomic runtime synchronization batch.
func (p *PipelineOrchestrator) SyncOverviewClient(client *ws.Client, request ws.RuntimeSyncRequest) {
	if p == nil || p.hub == nil || client == nil || p.runtimeCallbacksBlocked.Load() {
		return
	}
	p.overviewBuildMu.Lock()
	defer p.overviewBuildMu.Unlock()
	if p.runtimeCallbacksBlocked.Load() {
		return
	}
	p.syncOverviewClientLocked(client, request)
}

// ObserveRuntimeAck receives a runtime ACK after the WebSocket client validates
// a monotonic cursor or completion of its installed recovery batch.
func (p *PipelineOrchestrator) ObserveRuntimeAck(client *ws.Client, cursor ws.RuntimeCursor) {
	if p == nil || p.runtime == nil || client == nil || !cursor.Known || p.runtimeCallbacksBlocked.Load() {
		return
	}

	p.overviewBuildMu.Lock()
	defer p.overviewBuildMu.Unlock()
	if p.runtimeCallbacksBlocked.Load() {
		return
	}

	p.runtime.mu.RLock()
	currentStreamID := p.runtime.overviewStreamID
	currentVersion := p.runtime.overviewVersion
	p.runtime.mu.RUnlock()
	if cursor.StreamID != currentStreamID {
		logging.Debugf("runtime ACK ignored reason=stream_mismatch")
		return
	}
	if cursor.Version > currentVersion {
		logging.Debugf("runtime ACK ignored reason=beyond_current")
		return
	}

	now := p.clockNow()
	p.pruneRuntimeRecoveryAttemptsLocked(now)
	defer p.armRuntimeRecoveryTimerLocked(now)
	observability.Default().ObserveWSRuntimeAckLag(currentVersion - cursor.Version)
	attempt, ok := p.runtimeRecoveryAttempts[client]
	if !ok || cursor.StreamID != attempt.streamID || cursor.Version < attempt.targetVersion {
		return
	}

	delete(p.runtimeRecoveryAttempts, client)
	registry := observability.Default()
	registry.IncWSRuntimeRecovery(attempt.mode, attempt.reason, "completed")
	registry.ObserveWSRuntimeRecoveryDuration(
		attempt.mode,
		"completed",
		runtimeRecoveryDuration(attempt.startedAt, now),
	)
}

// syncOverviewClientLocked requires overviewBuildMu to remain held.
func (p *PipelineOrchestrator) syncOverviewClientLocked(client *ws.Client, request ws.RuntimeSyncRequest) {
	if p == nil || p.hub == nil || client == nil {
		return
	}
	state := p.getOrBuildOverviewStateLocked()
	reason := request.Reason
	if reason == "" {
		reason = ws.ResyncReasonClientResync
	}
	metricReason := runtimeRecoveryReason(request, state)

	p.runtime.mu.RLock()
	alertVersion := p.runtime.alertVersion
	p.runtime.mu.RUnlock()
	batch := ws.OverviewSyncBatch{
		Reason:          reason,
		RuntimeStreamID: state.StreamID,
		TargetVersion:   state.Version,
		RuntimeIdentity: ws.RuntimeIdentityForSnapshot(state.Snapshot),
		AlertVersion:    alertVersion,
	}

	switch {
	case request.Cursor.Known &&
		request.Cursor.StreamID == state.StreamID &&
		request.Cursor.Version == state.Version:
		batch.Mode = ws.OverviewSyncModeCurrent
	case client.RuntimeProtocol() >= ws.RuntimeStreamProtocolVersion &&
		request.Cursor.Known &&
		request.Cursor.StreamID == state.StreamID &&
		request.Cursor.Version < state.Version:
		if replay, ok := p.runtime.overviewJournal.Replay(request.Cursor.Version, state.Version); ok {
			batch.Mode = ws.OverviewSyncModeReplay
			batch.ReplayCursor = request.Cursor
			batch.Replay = replay
			break
		}
		fallthrough
	default:
		batch.Mode = ws.OverviewSyncModeSnapshot
		batch.Snapshot = ws.CloneSnapshot(state.Snapshot)
	}

	installStartedAt := p.clockNow()
	if p.hub.ReplaceOverviewStream(client, batch) {
		p.recordRuntimeRecoveryScheduledLocked(client, batch, metricReason, installStartedAt)
		p.armRuntimeRecoveryTimerLocked(p.clockNow())
		return
	}
	if batch.Mode != ws.OverviewSyncModeReplay && batch.Mode != ws.OverviewSyncModeCurrent {
		p.recordRuntimeRecoveryInstallFailure(batch, metricReason, installStartedAt)
		return
	}

	// The client's acknowledged cursor or capability may have changed while the
	// synchronization mode was selected. Fall back before releasing overviewBuildMu.
	batch.Mode = ws.OverviewSyncModeSnapshot
	batch.ReplayCursor = ws.RuntimeCursor{}
	batch.Replay = nil
	batch.Snapshot = ws.CloneSnapshot(state.Snapshot)
	installStartedAt = p.clockNow()
	if p.hub.ReplaceOverviewStream(client, batch) {
		p.recordRuntimeRecoveryScheduledLocked(client, batch, metricReason, installStartedAt)
		p.armRuntimeRecoveryTimerLocked(p.clockNow())
		return
	}
	p.recordRuntimeRecoveryInstallFailure(batch, metricReason, installStartedAt)
}

// recordRuntimeRecoveryScheduledLocked requires overviewBuildMu to remain held.
func (p *PipelineOrchestrator) recordRuntimeRecoveryScheduledLocked(
	client *ws.Client,
	batch ws.OverviewSyncBatch,
	reason string,
	startedAt time.Time,
) {
	now := p.clockNow()
	p.pruneRuntimeRecoveryAttemptsLocked(now)
	if p.runtimeRecoveryAttempts == nil {
		p.runtimeRecoveryAttempts = make(map[*ws.Client]runtimeRecoveryAttempt)
	}
	previous, replacing := p.runtimeRecoveryAttempts[client]
	if replacing {
		p.recordRuntimeRecoveryTerminal(previous, "failed", now)
	}
	if !replacing && len(p.runtimeRecoveryAttempts) >= runtimeRecoveryAttemptLimit {
		p.evictOldestRuntimeRecoveryAttemptLocked(now)
	}

	attempt := runtimeRecoveryAttempt{
		mode:          string(batch.Mode),
		reason:        reason,
		streamID:      batch.RuntimeStreamID,
		targetVersion: batch.TargetVersion,
		startedAt:     startedAt,
	}
	if attempt.startedAt.IsZero() {
		attempt.startedAt = now
	}
	p.runtimeRecoveryAttempts[client] = attempt

	registry := observability.Default()
	registry.IncWSRuntimeRecovery(attempt.mode, attempt.reason, "scheduled")
	if batch.Mode == ws.OverviewSyncModeReplay &&
		batch.ReplayCursor.Known &&
		batch.TargetVersion >= batch.ReplayCursor.Version {
		registry.ObserveWSRuntimeReplayVersions(batch.TargetVersion - batch.ReplayCursor.Version)
	}
}

// recordRuntimeRecoveryInstallFailure records one final batch that could not be queued.
func (p *PipelineOrchestrator) recordRuntimeRecoveryInstallFailure(
	batch ws.OverviewSyncBatch,
	reason string,
	startedAt time.Time,
) {
	registry := observability.Default()
	mode := string(batch.Mode)
	registry.IncWSRuntimeRecovery(mode, reason, "scheduled")
	registry.IncWSRuntimeRecovery(mode, reason, "failed")
	registry.ObserveWSRuntimeRecoveryDuration(
		mode,
		"failed",
		runtimeRecoveryDuration(startedAt, p.clockNow()),
	)
}

// pruneRuntimeRecoveryAttemptsLocked requires overviewBuildMu to remain held.
func (p *PipelineOrchestrator) pruneRuntimeRecoveryAttemptsLocked(now time.Time) {
	cutoff := now.Add(-p.runtimeRecoveryAttemptTTL())
	for client, attempt := range p.runtimeRecoveryAttempts {
		if attempt.startedAt.After(cutoff) {
			continue
		}
		delete(p.runtimeRecoveryAttempts, client)
		p.recordRuntimeRecoveryTerminal(attempt, "failed", now)
	}
}

// armRuntimeRecoveryTimerLocked keeps one timer for the earliest tracked deadline.
// It requires overviewBuildMu to remain held.
func (p *PipelineOrchestrator) armRuntimeRecoveryTimerLocked(now time.Time) {
	p.runtimeRecoveryTimerGen++
	generation := p.runtimeRecoveryTimerGen
	if p.runtimeRecoveryTimer != nil {
		p.runtimeRecoveryTimer.Stop()
		p.runtimeRecoveryTimer = nil
	}
	if len(p.runtimeRecoveryAttempts) == 0 {
		return
	}

	var earliest time.Time
	for _, attempt := range p.runtimeRecoveryAttempts {
		deadline := attempt.startedAt.Add(p.runtimeRecoveryAttemptTTL())
		if earliest.IsZero() || deadline.Before(earliest) {
			earliest = deadline
		}
	}
	delay := earliest.Sub(now)
	if delay < minimumRuntimeRecoveryTimerDelay {
		delay = minimumRuntimeRecoveryTimerDelay
	}
	p.runtimeRecoveryTimer = time.AfterFunc(delay, func() {
		p.expireRuntimeRecoveryAttempts(generation)
	})
}

func (p *PipelineOrchestrator) expireRuntimeRecoveryAttempts(generation uint64) {
	if p == nil {
		return
	}
	p.overviewBuildMu.Lock()
	defer p.overviewBuildMu.Unlock()
	if generation != p.runtimeRecoveryTimerGen {
		return
	}
	p.runtimeRecoveryTimer = nil
	now := p.clockNow()
	p.pruneRuntimeRecoveryAttemptsLocked(now)
	p.armRuntimeRecoveryTimerLocked(now)
}

func (p *PipelineOrchestrator) runtimeRecoveryAttemptTTL() time.Duration {
	if p.runtimeRecoveryTTL > 0 {
		return p.runtimeRecoveryTTL
	}
	return defaultRuntimeRecoveryAttemptTTL
}

// evictOldestRuntimeRecoveryAttemptLocked requires overviewBuildMu to remain held.
func (p *PipelineOrchestrator) evictOldestRuntimeRecoveryAttemptLocked(now time.Time) {
	var oldestClient *ws.Client
	var oldestAttempt runtimeRecoveryAttempt
	for client, attempt := range p.runtimeRecoveryAttempts {
		if oldestClient == nil || attempt.startedAt.Before(oldestAttempt.startedAt) {
			oldestClient = client
			oldestAttempt = attempt
		}
	}
	if oldestClient == nil {
		return
	}
	delete(p.runtimeRecoveryAttempts, oldestClient)
	p.recordRuntimeRecoveryTerminal(oldestAttempt, "failed", now)
}

func (p *PipelineOrchestrator) recordRuntimeRecoveryTerminal(
	attempt runtimeRecoveryAttempt,
	outcome string,
	now time.Time,
) {
	registry := observability.Default()
	registry.IncWSRuntimeRecovery(attempt.mode, attempt.reason, outcome)
	registry.ObserveWSRuntimeRecoveryDuration(
		attempt.mode,
		outcome,
		runtimeRecoveryDuration(attempt.startedAt, now),
	)
}

func runtimeRecoveryReason(request ws.RuntimeSyncRequest, state ws.RuntimeOverviewState) string {
	switch request.Reason {
	case ws.ResyncReasonClientMissingRuntimeSnapshot,
		ws.ResyncReasonStateChangesDrop,
		ws.ResyncReasonHubBufferFull,
		"timeout":
		return request.Reason
	}
	if !request.Cursor.Known {
		return "connect"
	}
	if request.Cursor.StreamID != state.StreamID {
		return "stream_mismatch"
	}
	if request.Cursor.Version < state.Version {
		return "client_gap"
	}
	return ws.ResyncReasonClientResync
}

func runtimeRecoveryDuration(startedAt, completedAt time.Time) time.Duration {
	if startedAt.IsZero() || completedAt.Before(startedAt) {
		return 0
	}
	return completedAt.Sub(startedAt)
}

func (p *PipelineOrchestrator) syncOverflowedOverviewClientsLocked(clients []*ws.Client, reason string) {
	for _, client := range clients {
		p.syncOverviewClientLocked(client, ws.RuntimeSyncRequest{
			Cursor: client.AckedRuntimeCursor(),
			Reason: reason,
		})
	}
}

func (p *PipelineOrchestrator) replaceOverviewClientsWithSnapshotLocked(state ws.RuntimeOverviewState, reason string) {
	if p == nil || p.hub == nil || p.runtimeCallbacksBlocked.Load() {
		return
	}
	p.runtime.mu.RLock()
	alertVersion := p.runtime.alertVersion
	p.runtime.mu.RUnlock()
	batch := ws.OverviewSyncBatch{
		Reason:          reason,
		Mode:            ws.OverviewSyncModeSnapshot,
		RuntimeStreamID: state.StreamID,
		TargetVersion:   state.Version,
		RuntimeIdentity: ws.RuntimeIdentityForSnapshot(state.Snapshot),
		Snapshot:        ws.CloneSnapshot(state.Snapshot),
		AlertVersion:    alertVersion,
	}
	startedAt := p.clockNow()
	metricReason := runtimeRecoveryBatchReason(reason)
	result := p.hub.ReplaceOverviewStreams(batch)
	for _, client := range result.Installed {
		p.recordRuntimeRecoveryScheduledLocked(client, batch, metricReason, startedAt)
	}
	for _, client := range result.Failed {
		if previous, ok := p.runtimeRecoveryAttempts[client]; ok {
			delete(p.runtimeRecoveryAttempts, client)
			p.recordRuntimeRecoveryTerminal(previous, "failed", p.clockNow())
		}
		p.recordRuntimeRecoveryInstallFailure(batch, metricReason, startedAt)
	}
	p.armRuntimeRecoveryTimerLocked(p.clockNow())
}

func runtimeRecoveryBatchReason(reason string) string {
	switch reason {
	case ws.ResyncReasonClientResync,
		ws.ResyncReasonClientMissingRuntimeSnapshot,
		ws.ResyncReasonStateChangesDrop,
		ws.ResyncReasonHubBufferFull,
		"timeout":
		return reason
	default:
		return ws.ResyncReasonClientResync
	}
}

// getOrBuildOverviewStateLocked requires overviewBuildMu to remain held.
func (p *PipelineOrchestrator) getOrBuildOverviewStateLocked() ws.RuntimeOverviewState {
	p.runtime.mu.RLock()
	if p.runtime.prevHashes != nil {
		state := ws.RuntimeOverviewState{
			Snapshot: ws.CloneSnapshot(p.runtime.lastSnapshot),
			Version:  p.runtime.overviewVersion,
			StreamID: p.runtime.overviewStreamID,
		}
		p.runtime.mu.RUnlock()
		return state
	}
	p.runtime.mu.RUnlock()

	snapshot, err := p.buildFullOverviewSnapshot()
	if err != nil {
		p.runtime.mu.RLock()
		state := ws.RuntimeOverviewState{
			Snapshot: ws.CloneSnapshot(p.runtime.lastSnapshot),
			Version:  p.runtime.overviewVersion,
			StreamID: p.runtime.overviewStreamID,
		}
		p.runtime.mu.RUnlock()
		return state
	}
	hashes := computeSnapshotHashes(snapshot)

	p.runtime.mu.Lock()
	if p.runtime.prevHashes != nil {
		state := ws.RuntimeOverviewState{
			Snapshot: ws.CloneSnapshot(p.runtime.lastSnapshot),
			Version:  p.runtime.overviewVersion,
			StreamID: p.runtime.overviewStreamID,
		}
		p.runtime.mu.Unlock()
		return state
	}
	p.runtime.lastSnapshot = snapshot
	p.runtime.prevHashes = hashes
	p.runtime.overviewVersion++
	p.runtime.overviewStreamID = uuid.NewString()
	state := ws.RuntimeOverviewState{
		Snapshot: ws.CloneSnapshot(snapshot),
		Version:  p.runtime.overviewVersion,
		StreamID: p.runtime.overviewStreamID,
	}
	p.runtime.mu.Unlock()
	p.runtime.overviewJournal.Reset()

	if p.hub != nil && p.hub.HasOverviewClients() {
		p.replaceOverviewClientsWithSnapshotLocked(state, ws.ResyncReasonClientResync)
	}

	return state
}

func (p *PipelineOrchestrator) IsPromAvailable() bool {
	return p.runtime.isPromAvailable()
}

// GetPrometheusStatus retrieves prometheus status data from the background worker lifecycle.
func (p *PipelineOrchestrator) GetPrometheusStatus() ws.PrometheusStatusPayload {
	return p.runtime.getPrometheusStatus()
}

// GetAlerts retrieves alerts data from the background worker lifecycle.
func (p *PipelineOrchestrator) GetAlerts() ws.AlertMessagePayload {
	return p.runtime.getAlerts()
}

func (p *PipelineOrchestrator) Status() string {
	if p.running.Load() {
		return "running"
	}
	return "stopped"
}

func (p *PipelineOrchestrator) PollingHealth() polling.HealthSnapshot {
	if p.scheduler == nil {
		return polling.HealthSnapshot{}
	}
	return p.scheduler.PollingHealth()
}

func (p *PipelineOrchestrator) workerCount() int {
	count := pollingbudget.Total(p.settingsRepo) + pollingPolicyFromSettings(p.settingsRepo).EssentialWorkers
	if count <= 0 {
		return 5
	}
	return count
}

func pollingPolicyFromSettings(settingsRepo domain.SettingsRepository) polling.Policy {
	policy, _ := polling.PolicyFromSettings(settingsRepo, 0, 0, 0)
	if policy.WebSocketCoalesce <= 0 {
		policy.WebSocketCoalesce = pipelineBroadcastCoalesceWindow
	}
	return policy
}

func (p *PipelineOrchestrator) broadcastLoop(ctx context.Context) {
	p.broadcaster.broadcastLoop(ctx)
}

func (p *PipelineOrchestrator) broadcastOnce(ctx context.Context) {
	p.broadcaster.broadcastOnce(ctx)
}

func (p *PipelineOrchestrator) broadcastDirty(ctx context.Context, dirtyDevices map[uuid.UUID]struct{}, alertsDirty bool, topologyDirty bool, forceFull bool) error {
	return p.broadcaster.broadcastDirty(ctx, dirtyDevices, alertsDirty, topologyDirty, forceFull)
}

func (p *PipelineOrchestrator) broadcastAlertsIfDirty(alertsDirty bool) {
	p.broadcaster.broadcastAlertsIfDirty(alertsDirty)
}

func (p *PipelineOrchestrator) broadcastFullSnapshot(ctx context.Context, reason string, topologyChanged bool) error {
	return p.broadcaster.broadcastFullSnapshot(ctx, reason, topologyChanged)
}

func (p *PipelineOrchestrator) broadcastFullSnapshotWithResync(ctx context.Context, reason string, resyncReason string, topologyChanged bool) error {
	return p.broadcaster.broadcastFullSnapshotWithResync(ctx, reason, resyncReason, topologyChanged)
}

func (p *PipelineOrchestrator) buildFullOverviewSnapshot() (_ *ws.SnapshotPayload, err error) {
	return p.broadcaster.buildFullOverviewSnapshot()
}

func (p *PipelineOrchestrator) buildDirtyOverviewDelta(dirtyDevices map[uuid.UUID]struct{}, alertsDirty bool) (*ws.SnapshotPayload, bool, error) {
	return p.broadcaster.buildDirtyOverviewDelta(dirtyDevices, alertsDirty)
}

func (p *PipelineOrchestrator) clockNow() time.Time {
	if p != nil && p.runtime != nil {
		return p.runtime.clockNow()
	}
	return time.Now().UTC()
}

func (p *PipelineOrchestrator) consumeResyncRequired() (string, bool) {
	if p.stateStore != nil && p.stateStore.ConsumeOverflowed() {
		return ws.ResyncReasonStateChangesDrop, true
	}
	return "", false
}

func reloadReasonForResync(reason string) string {
	switch reason {
	case ws.ResyncReasonStateChangesDrop:
		return refreshReloadReasonStateChangesDropped
	default:
		return refreshReloadReasonDirtyDeltaFallback
	}
}

func completionTime(collectedAt time.Time) time.Time {
	if collectedAt.IsZero() {
		return time.Now().UTC()
	}
	return collectedAt.UTC()
}

func cloneHostnameOverrides(in map[uuid.UUID]string) map[uuid.UUID]string {
	if len(in) == 0 {
		return nil
	}

	out := make(map[uuid.UUID]string, len(in))
	for id, hostname := range in {
		out[id] = hostname
	}
	return out
}

func cloneAlertGroups(in map[uuid.UUID][]domain.AlertState) map[uuid.UUID][]domain.AlertState {
	if len(in) == 0 {
		return nil
	}

	out := make(map[uuid.UUID][]domain.AlertState, len(in))
	for deviceID, alerts := range in {
		out[deviceID] = append([]domain.AlertState(nil), alerts...)
	}
	return out
}

func addDirtyDeviceIDs(target map[uuid.UUID]struct{}, ids []uuid.UUID) {
	for _, id := range ids {
		if id == uuid.Nil {
			continue
		}
		target[id] = struct{}{}
	}
}

func filterDevicesByID(devices []domain.Device, ids map[uuid.UUID]struct{}) []domain.Device {
	filtered := make([]domain.Device, 0, len(ids))
	for _, device := range devices {
		if _, ok := ids[device.ID]; ok {
			filtered = append(filtered, device)
		}
	}
	return filtered
}

func filterLinksByDeviceID(links []domain.Link, ids map[uuid.UUID]struct{}) []domain.Link {
	filtered := make([]domain.Link, 0, len(links))
	for _, link := range links {
		if _, ok := ids[link.SourceDeviceID]; ok {
			filtered = append(filtered, link)
			continue
		}
		if _, ok := ids[link.TargetDeviceID]; ok {
			filtered = append(filtered, link)
		}
	}
	return filtered
}

func snapshotPayloadEmpty(payload *ws.SnapshotPayload) bool {
	if payload == nil {
		return true
	}

	return len(payload.Devices) == 0 &&
		len(payload.Links) == 0
}

func mergeSnapshotPayload(base *ws.SnapshotPayload, delta *ws.SnapshotPayload) *ws.SnapshotPayload {
	merged := ws.CloneSnapshot(base)
	if merged == nil {
		merged = ws.EmptySnapshot()
	}
	if delta == nil {
		syncSnapshotCompatibility(merged)
		return merged
	}

	for key, value := range delta.Devices {
		merged.Devices[key] = value
	}
	for key, value := range delta.Links {
		merged.Links[key] = value
	}

	syncSnapshotCompatibility(merged)

	return merged
}

func applySnapshotDeltaToRuntime(base *ws.SnapshotPayload, hashes *sectionHashes, delta *ws.SnapshotPayload) (*ws.SnapshotPayload, *sectionHashes) {
	if base == nil {
		base = ws.EmptySnapshot()
	}
	ensureSnapshotRuntimeMaps(base)
	if hashes == nil {
		hashes = computeSnapshotHashes(base)
	}
	ensureSectionHashMaps(hashes)
	if delta == nil {
		return base, hashes
	}

	for key, value := range delta.Devices {
		value = cloneDeviceRuntimeForSnapshot(value)
		base.Devices[key] = value
		base.DeviceMetrics[key] = value
		base.DeviceStatuses[key] = compatibilityOperationalStatus(value.OperationalStatus)

		deviceHash, statusHash := computeDeviceRuntimeHashes(value)
		hashes.devices[key] = deviceHash
		hashes.deviceMetrics[key] = deviceHash
		hashes.deviceStatuses[key] = statusHash
	}

	for key, value := range delta.Links {
		previous := base.Links[key]
		removeLinkRuntimeFromCompatibility(base.LinkMetrics, previous, key)
		base.Links[key] = value
		compatibilityValue := value
		if compatibilityValue.DeviceID == "" {
			compatibilityValue.DeviceID = compatibilityValue.SourceDeviceID
		}
		base.LinkMetrics[compatibilityValue.DeviceID] = append(base.LinkMetrics[compatibilityValue.DeviceID], compatibilityValue)

		linkHash := computeLinkRuntimeHash(value)
		hashes.links[key] = linkHash
		hashes.linkMetrics[key] = linkHash
	}

	return base, hashes
}

func ensureSnapshotRuntimeMaps(snapshot *ws.SnapshotPayload) {
	if snapshot.Devices == nil {
		snapshot.Devices = make(map[string]ws.DeviceRuntimeDTO)
	}
	if snapshot.Links == nil {
		snapshot.Links = make(map[string]ws.LinkRuntimeDTO)
	}
	if snapshot.DeviceMetrics == nil || snapshot.LinkMetrics == nil || snapshot.DeviceStatuses == nil {
		syncSnapshotCompatibility(snapshot)
		return
	}
}

func ensureSectionHashMaps(hashes *sectionHashes) {
	if hashes.devices == nil {
		hashes.devices = make(map[string]uint64)
	}
	if hashes.links == nil {
		hashes.links = make(map[string]uint64)
	}
	if hashes.deviceMetrics == nil {
		hashes.deviceMetrics = make(map[string]uint64)
	}
	if hashes.linkMetrics == nil {
		hashes.linkMetrics = make(map[string]uint64)
	}
	if hashes.deviceStatuses == nil {
		hashes.deviceStatuses = make(map[string]uint64)
	}
}

func cloneDeviceRuntimeForSnapshot(value ws.DeviceRuntimeDTO) ws.DeviceRuntimeDTO {
	value.RuntimeFlags = append([]string(nil), value.RuntimeFlags...)
	if value.FieldStates != nil {
		cloned := make(map[string]string, len(value.FieldStates))
		for key, state := range value.FieldStates {
			cloned[key] = state
		}
		value.FieldStates = cloned
	}
	return value
}

func removeLinkRuntimeFromCompatibility(metrics map[string][]ws.LinkRuntimeDTO, previous ws.LinkRuntimeDTO, linkID string) {
	if len(metrics) == 0 {
		return
	}
	if previous.LinkID != "" {
		deviceID := previous.DeviceID
		if deviceID == "" {
			deviceID = previous.SourceDeviceID
		}
		if removeLinkRuntimeFromCompatibilityBucket(metrics, deviceID, linkID) {
			return
		}
	}
	for deviceID := range metrics {
		if removeLinkRuntimeFromCompatibilityBucket(metrics, deviceID, linkID) {
			return
		}
	}
}

func removeLinkRuntimeFromCompatibilityBucket(metrics map[string][]ws.LinkRuntimeDTO, deviceID string, linkID string) bool {
	if deviceID == "" {
		return false
	}
	links := metrics[deviceID]
	for index, link := range links {
		if link.LinkID != linkID {
			continue
		}
		copy(links[index:], links[index+1:])
		links = links[:len(links)-1]
		if len(links) == 0 {
			delete(metrics, deviceID)
		} else {
			metrics[deviceID] = links
		}
		return true
	}
	return false
}

func drainBroadcastLoopInputs(
	stateChanges <-chan []uuid.UUID,
	deviceChanges <-chan domain.DeviceChangeEvent,
	linkChanges <-chan domain.LinkChangeEvent,
	topologyNotify <-chan struct{},
	alertNotify <-chan struct{},
) {
	for {
		drained := false

		select {
		case <-stateChanges:
			drained = true
		default:
		}

		select {
		case <-deviceChanges:
			drained = true
		default:
		}

		select {
		case <-linkChanges:
			drained = true
		default:
		}

		select {
		case <-topologyNotify:
			drained = true
		default:
		}

		select {
		case <-alertNotify:
			drained = true
		default:
		}

		if !drained {
			return
		}
	}
}

func drainTopologyNotify(topologyNotify chan struct{}) bool {
	if topologyNotify == nil {
		return false
	}

	drained := false
	for {
		select {
		case <-topologyNotify:
			drained = true
		default:
			return drained
		}
	}
}
