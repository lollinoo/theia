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
	running                 atomic.Bool
	cancel                  context.CancelFunc
	done                    chan struct{}
	healthDone              chan struct{}
	runtime                 *pipelineRuntimeState
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
	}
	p.taskRunner = &pipelineTaskRunner{pipeline: p}
	p.broadcaster = &pipelineSnapshotBroadcaster{pipeline: p}
	p.prometheusMonitor = &pipelinePrometheusMonitor{pipeline: p}
	return p
}

func (p *PipelineOrchestrator) Start(ctx context.Context) error {
	p.lifecycleMu.Lock()
	defer p.lifecycleMu.Unlock()
	if p.cancel != nil {
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

		p.lifecycleMu.Lock()
		p.cancel = nil
		p.lifecycleMu.Unlock()
		p.running.Store(false)
	}()

	return nil
}

func (p *PipelineOrchestrator) Stop() {
	p.lifecycleMu.Lock()
	if p.cancel == nil {
		p.lifecycleMu.Unlock()
		return
	}
	cancel := p.cancel
	done := p.done
	healthDone := p.healthDone
	p.lifecycleMu.Unlock()

	cancel()
	<-done
	<-healthDone
}

// GetOverviewSnapshot retrieves overview snapshot data from the background worker lifecycle.
func (p *PipelineOrchestrator) GetOverviewSnapshot() (*ws.SnapshotPayload, uint64) {
	return p.runtime.getOverviewSnapshot()
}

// GetOrBuildOverviewSnapshot retrieves or build overview snapshot data from the background worker lifecycle.
func (p *PipelineOrchestrator) GetOrBuildOverviewSnapshot() (*ws.SnapshotPayload, uint64) {
	p.runtime.mu.RLock()
	hasRuntimeBase := p.runtime.prevHashes != nil
	p.runtime.mu.RUnlock()
	if hasRuntimeBase {
		return p.runtime.getOverviewSnapshot()
	}

	snapshot, err := p.buildFullOverviewSnapshot()
	if err != nil {
		return p.runtime.getOverviewSnapshot()
	}
	hashes := computeSnapshotHashes(snapshot)

	p.runtime.mu.Lock()
	p.runtime.lastSnapshot = snapshot
	p.runtime.prevHashes = hashes
	p.runtime.overviewVersion++
	version := p.runtime.overviewVersion
	p.runtime.mu.Unlock()

	return ws.CloneSnapshot(snapshot), version
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
