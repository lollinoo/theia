package worker

import (
	"context"
	"log"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"

	"github.com/lollinoo/theia/internal/cache"
	"github.com/lollinoo/theia/internal/collector"
	"github.com/lollinoo/theia/internal/domain"
	"github.com/lollinoo/theia/internal/observability"
	"github.com/lollinoo/theia/internal/pollingbudget"
	"github.com/lollinoo/theia/internal/scheduler"
	"github.com/lollinoo/theia/internal/service"
	"github.com/lollinoo/theia/internal/snmp"
	"github.com/lollinoo/theia/internal/state"
	"github.com/lollinoo/theia/internal/ws"
)

const (
	pipelineBroadcastCoalesceWindow = 250 * time.Millisecond
	pipelineFullResyncInterval      = 60 * time.Second
	prometheusEnrichmentRetention   = 30 * time.Second

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
	Start(context.Context)
	Stop()
	Tasks() <-chan scheduler.PollTask
	Complete(scheduler.Completion)
	Status() string
}

type PipelineOrchestrator struct {
	scheduler       pipelineScheduler
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
	running                 atomic.Bool
	cancel                  context.CancelFunc
	done                    chan struct{}
	healthDone              chan struct{}
	snapshotMu              sync.RWMutex
	lastSnapshot            *ws.SnapshotPayload
	promStatus              ws.PrometheusStatusPayload
	hostnames               map[uuid.UUID]string
	hostnameObservedAt      map[uuid.UUID]time.Time
	alerts                  map[uuid.UUID][]domain.AlertState
	prevCounters            map[uuid.UUID]map[string]collector.CounterBaseline
	prevHashes              *sectionHashes
	now                     func() time.Time
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
	return &PipelineOrchestrator{
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
		lastSnapshot:            ws.EmptySnapshot(),
		promStatus:              initialPrometheusStatus(settingsRepo),
		hostnames:               make(map[uuid.UUID]string),
		hostnameObservedAt:      make(map[uuid.UUID]time.Time),
		alerts:                  make(map[uuid.UUID][]domain.AlertState),
		prevCounters:            make(map[uuid.UUID]map[string]collector.CounterBaseline),
		now:                     time.Now,
	}
}

func (p *PipelineOrchestrator) Start(ctx context.Context) {
	if !p.running.CompareAndSwap(false, true) {
		panic("pipeline orchestrator: Start called more than once")
	}

	runCtx, cancel := context.WithCancel(ctx)
	p.cancel = cancel
	p.done = make(chan struct{})
	p.healthDone = make(chan struct{})

	if p.stateStore != nil {
		p.stateStore.Start(runCtx)
	}
	if p.scheduler != nil {
		p.scheduler.Start(runCtx)
	}

	go func() {
		defer close(p.done)
		var wg sync.WaitGroup

		for i := 0; i < p.workerCount(); i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				p.runWorker(runCtx)
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
			p.broadcastLoop(runCtx)
		}()

		<-runCtx.Done()
		if p.scheduler != nil {
			p.scheduler.Stop()
		}
		if p.stateStore != nil {
			p.stateStore.Stop()
		}
		wg.Wait()

		p.snapshotMu.Lock()
		p.cancel = nil
		p.snapshotMu.Unlock()
		p.running.Store(false)
	}()
}

func (p *PipelineOrchestrator) Stop() {
	if p.cancel == nil {
		return
	}

	p.cancel()
	<-p.done
	<-p.healthDone
}

func (p *PipelineOrchestrator) GetSnapshot() *ws.SnapshotPayload {
	p.snapshotMu.RLock()
	defer p.snapshotMu.RUnlock()
	return ws.CloneSnapshot(p.lastSnapshot)
}

func (p *PipelineOrchestrator) IsPromAvailable() bool {
	p.snapshotMu.RLock()
	defer p.snapshotMu.RUnlock()
	return p.promStatus.Enabled && p.promStatus.Available
}

func (p *PipelineOrchestrator) GetPrometheusStatus() ws.PrometheusStatusPayload {
	p.snapshotMu.RLock()
	defer p.snapshotMu.RUnlock()
	return p.promStatus
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

func (p *PipelineOrchestrator) runWorker(ctx context.Context) {
	if p.scheduler == nil {
		<-ctx.Done()
		return
	}

	for {
		select {
		case <-ctx.Done():
			return
		case task, ok := <-p.scheduler.Tasks():
			if !ok {
				return
			}
			p.runTask(ctx, task)
		}
	}
}

func (p *PipelineOrchestrator) runTask(ctx context.Context, task scheduler.PollTask) {
	finishedAt := time.Now().UTC()
	defer func() {
		if p.scheduler != nil {
			p.scheduler.Complete(scheduler.Completion{
				RunID:      task.RunID,
				Key:        task.Key,
				FinishedAt: finishedAt,
			})
		}
	}()

	if task.Device.DeviceType == domain.DeviceTypeVirtual {
		finishedAt = p.runVirtualTask(ctx, task)
		return
	}

	switch task.VolatilityClass {
	case domain.VolatilityClassPerformance:
		if p.performance == nil || p.stateStore == nil {
			return
		}

		result := p.performance.Poll(ctx, task.Device, p.snmpTimeout(), p.snmpRetries())
		finishedAt = completionTime(result.CollectedAt)
		observability.Default().IncPollResult(task.VolatilityClass, result.Err == nil)

		update := result.ToStoreUpdate(task.ExpectedInterval)
		if result.Err == nil {
			if p.prometheus != nil && p.GetPrometheusStatus().Enabled {
				enrichment, err := p.prometheus.CollectDeviceEnrichment(ctx, task.Device)
				if err == nil && enrichment.Hostname != "" {
					p.recordPrometheusHostname(task.Device.ID, enrichment.Hostname)
				}
			}

			p.snapshotMu.Lock()
			linkMetrics, next := collector.ComputeCounterRates(
				result.Counters,
				p.prevCounters[task.Device.ID],
				completionTime(result.CollectedAt),
				task.ExpectedInterval,
			)
			p.prevCounters[task.Device.ID] = next
			p.snapshotMu.Unlock()
			update.LinkMetrics = linkMetrics
		}

		p.stateStore.Update(update)
		p.publishSubscribedDetailDelta(task.Device)

	case domain.VolatilityClassOperational:
		if p.operational == nil || p.stateStore == nil {
			return
		}

		result := p.operational.Poll(ctx, task.Device, p.snmpTimeout(), p.snmpRetries())
		finishedAt = completionTime(result.CollectedAt)
		observability.Default().IncPollResult(task.VolatilityClass, result.Err == nil)
		p.stateStore.Update(result.ToStoreUpdate(task.ExpectedInterval))
		p.publishSubscribedDetailDelta(task.Device)

	case domain.VolatilityClassStatic:
		if p.staticCollector == nil || p.stateStore == nil {
			return
		}

		result := p.staticCollector.Poll(ctx, task.Device, p.snmpTimeout(), p.snmpRetries(), p.topologyDiscoveryMode(task.Device))
		finishedAt = completionTime(result.CollectedAt)
		observability.Default().IncPollResult(task.VolatilityClass, result.Err == nil)
		p.stateStore.Update(state.StateUpdate{
			DeviceID:         task.Device.ID,
			VolatilityClass:  domain.VolatilityClassStatic,
			PollSuccess:      result.Err == nil,
			ExpectedInterval: task.ExpectedInterval,
			Timestamp:        completionTime(result.CollectedAt),
		})
		p.publishSubscribedDetailDelta(task.Device)
		if result.Err != nil || p.topologyService == nil {
			return
		}

		persisted, err := p.topologyService.ApplyStaticDiscovery(task.Device.ID, service.StaticDiscoveryInput{
			SysName:       result.SysName,
			SysDescr:      result.SysDescr,
			SysObjectID:   result.SysObjectID,
			HardwareModel: result.HardwareModel,
			Vendor:        result.Vendor,
			DeviceType:    result.DeviceType,
			Interfaces:    append([]domain.Interface(nil), result.Interfaces...),
			Neighbors:     append([]snmp.NeighborInfo(nil), result.Neighbors...),
		})
		if err != nil {
			log.Printf("pipeline: static persistence failed for %s: %v", task.Device.ID, err)
			return
		}
		if persisted.TopologyChanged && p.topologyNotify != nil {
			select {
			case p.topologyNotify <- struct{}{}:
			default:
			}
		}
	}
}

func (p *PipelineOrchestrator) runVirtualTask(ctx context.Context, task scheduler.PollTask) time.Time {
	if domain.IsVirtualNoIPDevice(task.Device) {
		return time.Now().UTC()
	}

	switch task.VolatilityClass {
	case domain.VolatilityClassOperational:
		return p.runVirtualOperationalTask(ctx, task)
	case domain.VolatilityClassPerformance, domain.VolatilityClassStatic:
		return time.Now().UTC()
	default:
		return time.Now().UTC()
	}
}

func (p *PipelineOrchestrator) runVirtualOperationalTask(ctx context.Context, task scheduler.PollTask) time.Time {
	collectedAt := time.Now().UTC()
	if p.stateStore == nil {
		return collectedAt
	}

	result := collector.OperationalResult{
		DeviceID:    task.Device.ID,
		CollectedAt: collectedAt,
	}

	if p.prometheus != nil && p.GetPrometheusStatus().Enabled {
		enrichment, err := p.prometheus.CollectDeviceEnrichment(ctx, task.Device)
		if err != nil {
			log.Printf("pipeline: virtual enrichment failed for %s: %v", task.Device.ID, err)
		} else {
			if enrichment.Hostname != "" {
				p.recordPrometheusHostname(task.Device.ID, enrichment.Hostname)
			}
			if enrichment.ProbeReachable != nil {
				result.Reachable = *enrichment.ProbeReachable
				completedAt := completionTime(result.CollectedAt)
				p.stateStore.Update(result.ToStoreUpdate(task.ExpectedInterval))
				p.stateStore.Update(state.StateUpdate{
					DeviceID:         task.Device.ID,
					VolatilityClass:  domain.VolatilityClassPerformance,
					PollSuccess:      true,
					ExpectedInterval: task.ExpectedInterval,
					Timestamp:        completedAt,
				})
				p.publishSubscribedDetailDelta(task.Device)
				return completedAt
			}
		}
	}

	if err := service.ProbeVirtualReachability(ctx, task.Device.IP, p.snmpTimeout()); err != nil {
		result.Err = err
	} else {
		result.Reachable = true
	}
	observability.Default().IncPollResult(task.VolatilityClass, result.Err == nil)

	completedAt := completionTime(result.CollectedAt)
	p.stateStore.Update(result.ToStoreUpdate(task.ExpectedInterval))
	// Virtual nodes only run the operational tier, so stamp freshness metadata
	// explicitly to keep the UI footer out of the "waiting for first poll" state.
	p.stateStore.Update(state.StateUpdate{
		DeviceID:         task.Device.ID,
		VolatilityClass:  domain.VolatilityClassPerformance,
		PollSuccess:      result.Err == nil,
		ExpectedInterval: task.ExpectedInterval,
		Timestamp:        completedAt,
	})
	p.publishSubscribedDetailDelta(task.Device)
	return completedAt
}

func (p *PipelineOrchestrator) publishSubscribedDetailDelta(device domain.Device) {
	if p.hub == nil || p.stateStore == nil {
		return
	}

	subscribers := p.hub.DetailSubscribers(device.ID)
	if len(subscribers) == 0 {
		return
	}

	deviceState, ok := p.stateStore.GetDevice(device.ID)
	if !ok {
		return
	}

	delta := buildDeviceDetailDelta(device, deviceState)
	for _, client := range subscribers {
		p.hub.SendTo(client, ws.Message{
			Type:    ws.MessageTypeSnapshotDelta,
			Payload: delta,
		})
	}
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
	if p.cache == nil || p.stateStore == nil || p.hub == nil {
		<-ctx.Done()
		return
	}

	stateChanges := p.stateStore.Changes()
	p.broadcastOnce(ctx)
	drainBroadcastLoopInputs(stateChanges, p.deviceChangeNotify, p.linkChangeNotify, p.topologyNotify, p.alertNotify)

	flushTimer := time.NewTimer(time.Hour)
	if !flushTimer.Stop() {
		select {
		case <-flushTimer.C:
		default:
		}
	}
	defer flushTimer.Stop()

	fullResyncTicker := time.NewTicker(p.fullResyncInterval)
	defer fullResyncTicker.Stop()

	flushScheduled := false
	dirtyDevices := make(map[uuid.UUID]struct{})
	topologyDirty := false
	alertsDirty := false
	forceFull := false

	scheduleFlush := func() {
		if flushScheduled {
			return
		}
		flushTimer.Reset(p.broadcastCoalesceWindow)
		flushScheduled = true
	}

	resetDirtyState := func() {
		clear(dirtyDevices)
		topologyDirty = false
		alertsDirty = false
		forceFull = false
		flushScheduled = false
	}

	for {
		select {
		case <-ctx.Done():
			return
		case ids, ok := <-stateChanges:
			if !ok {
				stateChanges = nil
				continue
			}
			addDirtyDeviceIDs(dirtyDevices, ids)
			scheduleFlush()
		case change, ok := <-p.deviceChangeNotify:
			if !ok {
				p.deviceChangeNotify = nil
				continue
			}
			switch change.Kind {
			case domain.ChangeKindCreated, domain.ChangeKindDeleted:
				topologyDirty = true
			case domain.ChangeKindUpdated:
				if change.DeviceID != uuid.Nil {
					dirtyDevices[change.DeviceID] = struct{}{}
				}
			}
			scheduleFlush()
		case _, ok := <-p.linkChangeNotify:
			if !ok {
				p.linkChangeNotify = nil
				continue
			}
			topologyDirty = true
			scheduleFlush()
		case <-p.topologyNotify:
			topologyDirty = true
			scheduleFlush()
		case <-p.alertNotify:
			alertsDirty = true
			scheduleFlush()
		case <-fullResyncTicker.C:
			forceFull = true
			scheduleFlush()
		case <-flushTimer.C:
			flushScheduled = false
			if err := p.broadcastDirty(ctx, dirtyDevices, alertsDirty, topologyDirty, forceFull); err != nil {
				log.Printf("pipeline: event-driven broadcast failed: %v", err)
			}
			resetDirtyState()
		}
	}
}

func (p *PipelineOrchestrator) broadcastOnce(context.Context) {
	if p.cache == nil || p.stateStore == nil || p.hub == nil {
		return
	}

	devices, err := p.cache.GetDevices()
	if err != nil {
		log.Printf("pipeline: failed to load devices for broadcast: %v", err)
		return
	}

	links, err := p.cache.GetLinks()
	if err != nil {
		log.Printf("pipeline: failed to load links for broadcast: %v", err)
		links = nil
	}

	p.snapshotMu.RLock()
	hostnames := cloneHostnameOverrides(p.hostnames)
	alerts := cloneAlertGroups(p.alerts)
	previousHashes := p.prevHashes
	p.snapshotMu.RUnlock()

	startedAt := time.Now()
	snapshot := buildPipelineSnapshot(devices, links, p.stateStore.Snapshot(), alerts, hostnames)
	observability.Default().ObserveRefreshSnapshotBuild(refreshSnapshotModeFull, time.Since(startedAt), true)
	currentHashes := computeSnapshotHashes(snapshot)
	drainedTopology := drainTopologyNotify(p.topologyNotify)

	p.snapshotMu.Lock()
	p.lastSnapshot = snapshot
	p.prevHashes = currentHashes
	p.snapshotMu.Unlock()

	switch {
	case previousHashes == nil:
		observability.Default().IncRefreshTopologyReload(refreshReloadReasonStartup)
		p.hub.Broadcast(ws.Message{
			Type:    ws.MessageTypeSnapshot,
			Payload: snapshot,
		})
	default:
		delta := buildDelta(snapshot, currentHashes, previousHashes)
		if delta == nil && drainedTopology {
			observability.Default().IncRefreshTopologyReload(refreshReloadReasonTopologyDrainFallback)
			p.hub.Broadcast(ws.Message{
				Type:    ws.MessageTypeSnapshot,
				Payload: snapshot,
			})
		} else if delta != nil {
			p.hub.Broadcast(ws.Message{
				Type:    ws.MessageTypeSnapshotDelta,
				Payload: delta,
			})
		}
	}

	if drainedTopology {
		p.hub.Broadcast(ws.Message{
			Type:    ws.MessageTypeTopologyChanged,
			Payload: nil,
		})
	}
}

func (p *PipelineOrchestrator) broadcastDirty(ctx context.Context, dirtyDevices map[uuid.UUID]struct{}, alertsDirty bool, topologyDirty bool, forceFull bool) error {
	if p.cache == nil || p.stateStore == nil || p.hub == nil {
		return nil
	}

	if resyncReason, ok := p.consumeResyncRequired(); ok {
		p.hub.Broadcast(ws.Message{
			Type: ws.MessageTypeResyncRequired,
			Payload: ws.ResyncRequiredPayload{
				Scope:  ws.ResyncScopeOverview,
				Reason: resyncReason,
			},
		})
		return p.broadcastFullSnapshot(ctx, reloadReasonForResync(resyncReason), topologyDirty)
	}
	if topologyDirty {
		return p.broadcastFullSnapshot(ctx, refreshReloadReasonTopologyDirty, true)
	}
	if forceFull {
		return p.broadcastFullSnapshot(ctx, refreshReloadReasonFullResync, false)
	}
	if len(dirtyDevices) == 0 && !alertsDirty {
		return nil
	}

	startedAt := time.Now()
	delta, requireFullSnapshot, err := p.buildDirtyOverviewDelta(dirtyDevices, alertsDirty)
	observability.Default().ObserveRefreshSnapshotBuild(refreshSnapshotModeDirty, time.Since(startedAt), err == nil)
	if err != nil {
		return err
	}
	if requireFullSnapshot {
		return p.broadcastFullSnapshot(ctx, refreshReloadReasonDirtyDeltaFallback, false)
	}
	if delta == nil {
		return nil
	}

	p.snapshotMu.Lock()
	merged := mergeSnapshotPayload(p.lastSnapshot, delta)
	p.lastSnapshot = merged
	p.prevHashes = computeSnapshotHashes(merged)
	p.snapshotMu.Unlock()

	p.hub.Broadcast(ws.Message{
		Type:    ws.MessageTypeSnapshotDelta,
		Payload: delta,
	})

	return nil
}

func (p *PipelineOrchestrator) broadcastFullSnapshot(_ context.Context, reason string, topologyChanged bool) error {
	observability.Default().IncRefreshTopologyReload(reason)
	snapshot, err := p.buildFullOverviewSnapshot()
	if err != nil {
		return err
	}

	p.snapshotMu.Lock()
	p.lastSnapshot = snapshot
	p.prevHashes = computeSnapshotHashes(snapshot)
	p.snapshotMu.Unlock()

	p.hub.Broadcast(ws.Message{
		Type:    ws.MessageTypeSnapshot,
		Payload: snapshot,
	})

	if topologyChanged {
		p.hub.Broadcast(ws.Message{
			Type:    ws.MessageTypeTopologyChanged,
			Payload: nil,
		})
	}

	return nil
}

func (p *PipelineOrchestrator) buildFullOverviewSnapshot() (_ *ws.SnapshotPayload, err error) {
	startedAt := time.Now()
	defer func() {
		observability.Default().ObserveRefreshSnapshotBuild(refreshSnapshotModeFull, time.Since(startedAt), err == nil)
	}()

	devices, err := p.cache.GetDevices()
	if err != nil {
		return nil, err
	}

	links, err := p.cache.GetLinks()
	if err != nil {
		return nil, err
	}

	p.snapshotMu.RLock()
	hostnames := cloneHostnameOverrides(p.hostnames)
	alerts := cloneAlertGroups(p.alerts)
	p.snapshotMu.RUnlock()

	return buildPipelineSnapshot(devices, links, p.stateStore.Snapshot(), alerts, hostnames), nil
}

func (p *PipelineOrchestrator) buildDirtyOverviewDelta(dirtyDevices map[uuid.UUID]struct{}, alertsDirty bool) (*ws.SnapshotPayload, bool, error) {
	if len(dirtyDevices) == 0 && !alertsDirty {
		return nil, false, nil
	}

	delta := ws.EmptySnapshot()
	if len(dirtyDevices) > 0 {
		devices, err := p.cache.GetDevices()
		if err != nil {
			return nil, false, err
		}

		links, err := p.cache.GetLinks()
		if err != nil {
			return nil, false, err
		}

		p.snapshotMu.RLock()
		hostnames := cloneHostnameOverrides(p.hostnames)
		p.snapshotMu.RUnlock()

		filteredDevices := filterDevicesByID(devices, dirtyDevices)
		filteredLinks := filterLinksByDeviceID(links, dirtyDevices)
		if len(filteredDevices) > 0 {
			partial := buildPipelineSnapshot(filteredDevices, filteredLinks, p.stateStore.Snapshot(), nil, hostnames)
			delta.DeviceMetrics = partial.DeviceMetrics
			delta.LinkMetrics = partial.LinkMetrics
			delta.DeviceStatuses = partial.DeviceStatuses

			for _, device := range filteredDevices {
				deviceKey := device.ID.String()

				hostname, ok := partial.DeviceHostnames[deviceKey]
				if !ok {
					hostname = ""
				}
				delta.DeviceHostnames[deviceKey] = hostname

				model, ok := partial.DeviceModels[deviceKey]
				if !ok {
					model = ""
				}
				delta.DeviceModels[deviceKey] = model
			}
		}
	}

	if alertsDirty {
		p.snapshotMu.RLock()
		alerts := cloneAlertGroups(p.alerts)
		previousAlerts := append([]ws.AlertDTO(nil), p.lastSnapshot.Alerts...)
		p.snapshotMu.RUnlock()

		currentAlerts := ws.AlertsToDTOs(flattenAlerts(alerts))
		if len(currentAlerts) == 0 && len(previousAlerts) > 0 {
			return nil, true, nil
		}
		delta.Alerts = currentAlerts
	}

	if snapshotPayloadEmpty(delta) {
		return nil, false, nil
	}

	return delta, false, nil
}

func (p *PipelineOrchestrator) setAlerts(next map[uuid.UUID][]domain.AlertState) {
	p.snapshotMu.Lock()
	previous := ws.AlertsToDTOs(flattenAlerts(cloneAlertGroups(p.alerts)))
	current := ws.AlertsToDTOs(flattenAlerts(cloneAlertGroups(next)))
	changed := !reflect.DeepEqual(previous, current)
	p.alerts = next
	p.snapshotMu.Unlock()

	if changed {
		select {
		case p.alertNotify <- struct{}{}:
		default:
		}
	}
}

func (p *PipelineOrchestrator) publishPrometheusStatus(status ws.PrometheusStatusPayload) {
	p.snapshotMu.Lock()
	changed := p.promStatus != status
	p.promStatus = status
	p.snapshotMu.Unlock()

	if !changed || p.hub == nil {
		return
	}

	p.hub.Broadcast(ws.Message{
		Type:    ws.MessageTypePrometheusStatus,
		Payload: status,
	})
}

func (p *PipelineOrchestrator) recordPrometheusHostname(deviceID uuid.UUID, hostname string) {
	if hostname == "" {
		return
	}

	p.snapshotMu.Lock()
	defer p.snapshotMu.Unlock()
	p.hostnames[deviceID] = hostname
	p.hostnameObservedAt[deviceID] = p.clockNow()
}

func (p *PipelineOrchestrator) clearPrometheusHostnames() {
	p.snapshotMu.Lock()
	defer p.snapshotMu.Unlock()
	clear(p.hostnames)
	clear(p.hostnameObservedAt)
}

func (p *PipelineOrchestrator) prunePrometheusHostnames() {
	cutoff := p.clockNow().Add(-prometheusEnrichmentRetention)

	p.snapshotMu.Lock()
	defer p.snapshotMu.Unlock()
	for deviceID, observedAt := range p.hostnameObservedAt {
		if observedAt.After(cutoff) {
			continue
		}
		delete(p.hostnameObservedAt, deviceID)
		delete(p.hostnames, deviceID)
	}
}

func (p *PipelineOrchestrator) clockNow() time.Time {
	if p != nil && p.now != nil {
		return p.now().UTC()
	}
	return time.Now().UTC()
}

func (p *PipelineOrchestrator) consumeResyncRequired() (string, bool) {
	if p.stateStore != nil && p.stateStore.ConsumeOverflowed() {
		return ws.ResyncReasonStateChangesDrop, true
	}
	if p.hub != nil && p.hub.ConsumeBroadcastOverflow() {
		return ws.ResyncReasonHubBufferFull, true
	}
	return "", false
}

func reloadReasonForResync(reason string) string {
	switch reason {
	case ws.ResyncReasonStateChangesDrop:
		return refreshReloadReasonStateChangesDropped
	case ws.ResyncReasonHubBufferFull:
		return refreshReloadReasonHubBufferFull
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

func (p *PipelineOrchestrator) snmpTimeout() time.Duration {
	if p.settingsRepo == nil {
		return 10 * time.Second
	}

	value, err := p.settingsRepo.Get(domain.SettingSNMPTimeout)
	if err != nil {
		return 10 * time.Second
	}

	seconds, err := strconv.Atoi(value)
	if err != nil || seconds <= 0 {
		return 10 * time.Second
	}

	return time.Duration(seconds) * time.Second
}

func (p *PipelineOrchestrator) snmpRetries() int {
	if p.settingsRepo == nil {
		return 2
	}

	value, err := p.settingsRepo.Get(domain.SettingSNMPRetries)
	if err != nil {
		return 2
	}

	retries, err := strconv.Atoi(value)
	if err != nil || retries < 0 {
		return 2
	}

	return retries
}

func (p *PipelineOrchestrator) topologyDiscoveryMode(device domain.Device) domain.TopologyDiscoveryMode {
	defaultMode := domain.TopologyDiscoveryModeLLDPCDP
	if p.settingsRepo != nil {
		if value, err := p.settingsRepo.Get(domain.SettingTopologyDiscoveryDefaultMode); err == nil {
			defaultMode = domain.NormalizeTopologyDiscoveryMode(domain.TopologyDiscoveryMode(value), domain.TopologyDiscoveryModeLLDPCDP)
		}
	}

	// Regular static polling must never reopen bootstrap-once discovery windows.
	// One-shot topology discovery is handled explicitly by DeviceService on add,
	// manual runs, settings changes, and delayed reprobe follow-ups.
	mode := domain.NormalizeTopologyDiscoveryMode(device.TopologyDiscoveryMode, domain.TopologyDiscoveryModeInherit)
	if mode == domain.TopologyDiscoveryModeInherit {
		mode = defaultMode
	}
	if mode == domain.TopologyDiscoveryModeBootstrapOnce {
		return domain.TopologyDiscoveryModeOff
	}
	return mode
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
		len(payload.Alerts) == 0 &&
		len(payload.DeviceStatuses) == 0 &&
		len(payload.DeviceHostnames) == 0 &&
		len(payload.DeviceModels) == 0
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
	for key, value := range delta.DeviceHostnames {
		merged.DeviceHostnames[key] = value
	}
	for key, value := range delta.DeviceModels {
		merged.DeviceModels[key] = value
	}
	if delta.Alerts != nil {
		merged.Alerts = append([]ws.AlertDTO(nil), delta.Alerts...)
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
