package worker

import (
	"context"
	"log"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"

	"github.com/lollinoo/theia/internal/cache"
	"github.com/lollinoo/theia/internal/collector"
	"github.com/lollinoo/theia/internal/domain"
	"github.com/lollinoo/theia/internal/scheduler"
	"github.com/lollinoo/theia/internal/service"
	"github.com/lollinoo/theia/internal/snmp"
	"github.com/lollinoo/theia/internal/state"
	"github.com/lollinoo/theia/internal/ws"
)

const pipelineBroadcastInterval = 5 * time.Second

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
	settingsRepo   domain.SettingsRepository
	topologyNotify chan struct{}
	running        atomic.Bool
	cancel         context.CancelFunc
	done           chan struct{}
	healthDone     chan struct{}
	snapshotMu     sync.RWMutex
	lastSnapshot   *ws.SnapshotPayload
	promAvailable  bool
	promErr        string
	hostnames      map[uuid.UUID]string
	alerts         map[uuid.UUID][]domain.AlertState
	prevCounters   map[uuid.UUID]map[string]collector.CounterBaseline
	prevHashes     *sectionHashes
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
) *PipelineOrchestrator {
	return &PipelineOrchestrator{
		scheduler:       sched,
		stateStore:      stateStore,
		cache:           cache,
		hub:             hub,
		performance:     performance,
		operational:     operational,
		staticCollector: staticCollector,
		prometheus:      prometheus,
		topologyService: topologyService,
		settingsRepo:    settingsRepo,
		topologyNotify:  topologyNotify,
		done:            make(chan struct{}),
		healthDone:      make(chan struct{}),
		lastSnapshot:    ws.EmptySnapshot(),
		promAvailable:   true,
		hostnames:       make(map[uuid.UUID]string),
		alerts:          make(map[uuid.UUID][]domain.AlertState),
		prevCounters:    make(map[uuid.UUID]map[string]collector.CounterBaseline),
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
	return p.promAvailable
}

func (p *PipelineOrchestrator) Status() string {
	if p.running.Load() {
		return "running"
	}
	return "stopped"
}

func (p *PipelineOrchestrator) workerCount() int {
	if p.settingsRepo == nil {
		return 5
	}

	value, err := p.settingsRepo.Get(domain.SettingSNMPWorkerPoolSize)
	if err != nil {
		return 5
	}

	count, err := strconv.Atoi(value)
	if err != nil || count <= 0 {
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

	switch task.VolatilityClass {
	case domain.VolatilityClassPerformance:
		if p.performance == nil || p.stateStore == nil {
			return
		}

		result := p.performance.Poll(ctx, task.Device, p.snmpTimeout(), p.snmpRetries())
		finishedAt = completionTime(result.CollectedAt)

		update := result.ToStoreUpdate(task.ExpectedInterval)
		if result.Err == nil {
			if p.prometheus != nil {
				enrichment, err := p.prometheus.CollectDeviceEnrichment(ctx, task.Device)
				if err == nil && enrichment.Hostname != "" {
					p.snapshotMu.Lock()
					p.hostnames[task.Device.ID] = enrichment.Hostname
					p.snapshotMu.Unlock()
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
		p.stateStore.Update(result.ToStoreUpdate(task.ExpectedInterval))
		p.publishSubscribedDetailDelta(task.Device)

	case domain.VolatilityClassStatic:
		if p.staticCollector == nil || p.stateStore == nil {
			return
		}

		result := p.staticCollector.Poll(ctx, task.Device, p.snmpTimeout(), p.snmpRetries())
		finishedAt = completionTime(result.CollectedAt)
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

	devices, err := p.cache.GetDevices()
	if err != nil {
		p.publishPrometheusStatus(false, err)
		return
	}

	alerts, err := p.prometheus.CollectAlerts(ctx, devices)
	if err != nil {
		p.publishPrometheusStatus(false, err)
		return
	}

	p.snapshotMu.Lock()
	p.alerts = alerts
	p.snapshotMu.Unlock()
	p.publishPrometheusStatus(true, nil)
}

func (p *PipelineOrchestrator) broadcastLoop(ctx context.Context) {
	ticker := time.NewTicker(pipelineBroadcastInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			p.broadcastOnce(ctx)
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

	snapshot := buildPipelineSnapshot(devices, links, p.stateStore.Snapshot(), alerts, hostnames)
	currentHashes := computeSnapshotHashes(snapshot)
	drainedTopology := drainTopologyNotify(p.topologyNotify)

	p.snapshotMu.Lock()
	p.lastSnapshot = snapshot
	p.prevHashes = currentHashes
	p.snapshotMu.Unlock()

	switch {
	case previousHashes == nil:
		p.hub.Broadcast(ws.Message{
			Type:    ws.MessageTypeSnapshot,
			Payload: snapshot,
		})
	default:
		delta := buildDelta(snapshot, currentHashes, previousHashes)
		if delta == nil && drainedTopology {
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

func (p *PipelineOrchestrator) publishPrometheusStatus(available bool, err error) {
	p.snapshotMu.Lock()
	changed := p.promAvailable != available
	p.promAvailable = available
	if err != nil {
		p.promErr = err.Error()
	} else {
		p.promErr = ""
	}
	p.snapshotMu.Unlock()

	if !changed || p.hub == nil {
		return
	}

	payload := ws.PrometheusStatusPayload{Available: available}
	if !available && err != nil {
		payload.Error = err.Error()
	}
	p.hub.Broadcast(ws.Message{
		Type:    ws.MessageTypePrometheusStatus,
		Payload: payload,
	})
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
