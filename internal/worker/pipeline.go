package worker

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"

	"github.com/lollinoo/theia/internal/cache"
	"github.com/lollinoo/theia/internal/collector"
	"github.com/lollinoo/theia/internal/domain"
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
}

var ErrAlreadyStarted = errors.New("pipeline orchestrator: already started")

type pipelineTaskRunning interface {
	runWorker(context.Context)
	runTask(context.Context, scheduler.PollTask)
	topologyDiscoveryMode(domain.Device) domain.TopologyDiscoveryMode
	publishSubscribedDetailDelta(domain.Device)
}

type PipelineOrchestrator struct {
	scheduler       pipelineScheduler
	taskRunner      pipelineTaskRunning
	broadcaster     *pipelineSnapshotBroadcaster
	stateStore      *state.Store
	cache           *cache.DeviceLinkCache
	hub             *ws.Hub
	performance     *collector.PerformanceCollector
	operational     *collector.OperationalCollector
	staticCollector *collector.StaticCollector
	prometheus      *collector.PrometheusCollector
	topologyService interface {
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

func NewPipelineOrchestrator(
	sched pipelineScheduler,
	stateStore *state.Store,
	cache *cache.DeviceLinkCache,
	hub *ws.Hub,
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
		broadcastCoalesceWindow: pipelineBroadcastCoalesceWindow,
		fullResyncInterval:      pipelineFullResyncInterval,
		done:                    make(chan struct{}),
		healthDone:              make(chan struct{}),
		runtime:                 newPipelineRuntimeState(initialPrometheusStatus(settingsRepo)),
	}
	p.taskRunner = &pipelineTaskRunner{pipeline: p}
	p.broadcaster = &pipelineSnapshotBroadcaster{pipeline: p}
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
			p.refreshPrometheus(runCtx)
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

func (p *PipelineOrchestrator) GetOverviewSnapshot() (*ws.SnapshotPayload, uint64) {
	return p.runtime.getOverviewSnapshot()
}

func (p *PipelineOrchestrator) IsPromAvailable() bool {
	return p.runtime.isPromAvailable()
}

func (p *PipelineOrchestrator) GetPrometheusStatus() ws.PrometheusStatusPayload {
	return p.runtime.getPrometheusStatus()
}

func (p *PipelineOrchestrator) GetAlerts() ws.AlertMessagePayload {
	return p.runtime.getAlerts()
}

func (p *PipelineOrchestrator) Status() string {
	if p.running.Load() {
		return "running"
	}
	return "stopped"
}

func (p *PipelineOrchestrator) workerCount() int {
	count := pollingbudget.Total(p.settingsRepo)
	if count <= 0 {
		return 5
	}
	return count
}

func (p *PipelineOrchestrator) refreshPrometheus(ctx context.Context) {
	defer close(p.healthDone)

	p.refreshPrometheusOnce(ctx)

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			p.refreshPrometheusOnce(ctx)
		}
	}
}

func (p *PipelineOrchestrator) refreshPrometheusOnce(ctx context.Context) {
	if p.prometheus == nil || p.cache == nil {
		return
	}

	promURL := p.prometheusURL()
	if promURL == "" {
		p.prometheus.SetPrometheusURL("")
		p.setAlerts(make(map[uuid.UUID][]domain.AlertState))
		p.clearPrometheusHostnames()
		p.publishPrometheusStatus(ws.PrometheusStatusPayload{
			Enabled:   false,
			Available: false,
		})
		return
	}

	p.prometheus.SetPrometheusURL(promURL)

	devices, err := p.cache.GetDevices()
	if err != nil {
		p.setAlerts(make(map[uuid.UUID][]domain.AlertState))
		p.prunePrometheusHostnames()
		p.publishPrometheusStatus(ws.PrometheusStatusPayload{
			Enabled:   true,
			Available: false,
			Error:     err.Error(),
		})
		return
	}

	alerts, err := p.prometheus.CollectAlerts(ctx, devices)
	if err != nil {
		p.setAlerts(make(map[uuid.UUID][]domain.AlertState))
		p.prunePrometheusHostnames()
		p.publishPrometheusStatus(ws.PrometheusStatusPayload{
			Enabled:   true,
			Available: false,
			Error:     err.Error(),
		})
		return
	}

	p.setAlerts(alerts)
	p.publishPrometheusStatus(ws.PrometheusStatusPayload{
		Enabled:   true,
		Available: true,
	})
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

func (p *PipelineOrchestrator) setAlerts(next map[uuid.UUID][]domain.AlertState) {
	changed := p.runtime.setAlerts(next)

	if changed {
		select {
		case p.alertNotify <- struct{}{}:
		default:
		}
	}
}

func (p *PipelineOrchestrator) publishPrometheusStatus(status ws.PrometheusStatusPayload) {
	changed := p.runtime.setPrometheusStatus(status)

	if !changed || p.hub == nil {
		return
	}

	p.hub.Broadcast(ws.Message{
		Type:    ws.MessageTypePrometheusStatus,
		Payload: status,
	})
}

func (p *PipelineOrchestrator) recordPrometheusHostname(deviceID uuid.UUID, hostname string) {
	p.runtime.recordPrometheusHostname(deviceID, hostname)
}

func (p *PipelineOrchestrator) clearPrometheusHostnames() {
	p.runtime.clearPrometheusHostnames()
}

func (p *PipelineOrchestrator) prunePrometheusHostnames() {
	p.runtime.prunePrometheusHostnames()
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

func (p *PipelineOrchestrator) prometheusURL() string {
	if p.settingsRepo == nil {
		return ""
	}

	value, err := p.settingsRepo.Get(domain.SettingPrometheusURL)
	if err != nil {
		return ""
	}

	return strings.TrimSpace(value)
}

func initialPrometheusStatus(settingsRepo domain.SettingsRepository) ws.PrometheusStatusPayload {
	if settingsRepo == nil {
		return ws.PrometheusStatusPayload{}
	}

	value, err := settingsRepo.Get(domain.SettingPrometheusURL)
	if err != nil {
		return ws.PrometheusStatusPayload{}
	}

	enabled := strings.TrimSpace(value) != ""
	return ws.PrometheusStatusPayload{
		Enabled:   enabled,
		Available: enabled,
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

	return len(payload.DeviceMetrics) == 0 &&
		len(payload.LinkMetrics) == 0 &&
		len(payload.DeviceStatuses) == 0
}

func mergeSnapshotPayload(base *ws.SnapshotPayload, delta *ws.SnapshotPayload) *ws.SnapshotPayload {
	merged := ws.CloneSnapshot(base)
	if merged == nil {
		merged = ws.EmptySnapshot()
	}
	if delta == nil {
		return merged
	}

	for key, value := range delta.DeviceMetrics {
		merged.DeviceMetrics[key] = value
	}
	for key, value := range delta.LinkMetrics {
		merged.LinkMetrics[key] = append([]ws.LinkMetricsDTO(nil), value...)
	}
	for key, value := range delta.DeviceStatuses {
		merged.DeviceStatuses[key] = value
	}

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
