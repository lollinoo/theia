package worker

import (
	"context"
	"log"
	"time"

	"github.com/google/uuid"

	"github.com/lollinoo/theia/internal/domain"
	"github.com/lollinoo/theia/internal/observability"
	"github.com/lollinoo/theia/internal/polling"
	"github.com/lollinoo/theia/internal/ws"
)

type pipelineSnapshotBroadcaster struct {
	pipeline *PipelineOrchestrator
}

func (b *pipelineSnapshotBroadcaster) broadcastLoop(ctx context.Context) {
	p := b.pipeline
	if p.cache == nil || p.stateStore == nil || p.hub == nil {
		<-ctx.Done()
		return
	}

	stateChanges := p.stateStore.Changes()
	drainBroadcastLoopInputs(stateChanges, p.deviceChangeNotify, p.linkChangeNotify, p.topologyNotify, p.alertNotify)
	b.broadcastOnce(ctx)

	flushTimer := time.NewTimer(time.Hour)
	if !flushTimer.Stop() {
		select {
		case <-flushTimer.C:
		default:
		}
	}
	defer flushTimer.Stop()

	var fullResyncTicker *time.Ticker
	var fullResyncC <-chan time.Time
	if p.fullResyncInterval > 0 {
		fullResyncTicker = time.NewTicker(p.fullResyncInterval)
		fullResyncC = fullResyncTicker.C
		defer fullResyncTicker.Stop()
	}

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
		case <-fullResyncC:
			forceFull = true
			scheduleFlush()
		case <-flushTimer.C:
			flushScheduled = false
			if err := b.broadcastDirty(ctx, dirtyDevices, alertsDirty, topologyDirty, forceFull); err != nil {
				log.Printf("pipeline: event-driven broadcast failed: %v", err)
			}
			b.broadcastPollingHealthIfChanged()
			resetDirtyState()
		}
	}
}

func (b *pipelineSnapshotBroadcaster) broadcastOnce(context.Context) {
	p := b.pipeline
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

	p.runtime.mu.RLock()
	alerts := cloneAlertGroups(p.runtime.alerts)
	promStatus := p.runtime.promStatus
	previousHashes := p.runtime.prevHashes
	p.runtime.mu.RUnlock()

	startedAt := time.Now()
	snapshot := buildPipelineSnapshot(devices, links, p.stateStore.Snapshot(), alerts, promStatus)
	observability.Default().ObserveRefreshSnapshotBuild(refreshSnapshotModeFull, time.Since(startedAt), true)
	currentHashes := computeSnapshotHashes(snapshot)
	drainedTopology := drainTopologyNotify(p.topologyNotify)

	p.runtime.mu.Lock()
	p.runtime.lastSnapshot = snapshot
	p.runtime.prevHashes = currentHashes
	currentVersion := p.runtime.overviewVersion
	p.runtime.mu.Unlock()

	switch {
	case previousHashes == nil:
		p.runtime.mu.Lock()
		p.runtime.overviewVersion = currentVersion + 1
		version := p.runtime.overviewVersion
		p.runtime.mu.Unlock()
		observability.Default().IncRefreshTopologyReload(refreshReloadReasonStartup)
		p.hub.BroadcastOverviewSnapshot(snapshot, version)
	default:
		delta := buildDelta(snapshot, currentHashes, previousHashes)
		if delta != nil {
			p.runtime.mu.Lock()
			baseVersion := p.runtime.overviewVersion
			p.runtime.overviewVersion = baseVersion + 1
			version := p.runtime.overviewVersion
			p.runtime.mu.Unlock()
			p.hub.BroadcastOverviewDelta(delta, baseVersion, version, snapshot)
		}
	}

	if drainedTopology {
		b.broadcastTopologyInvalidation(refreshReloadReasonTopologyDrainFallback)
	}
}

func (b *pipelineSnapshotBroadcaster) broadcastDirty(ctx context.Context, dirtyDevices map[uuid.UUID]struct{}, alertsDirty bool, topologyDirty bool, forceFull bool) error {
	p := b.pipeline
	if p.cache == nil || p.stateStore == nil || p.hub == nil {
		return nil
	}

	if resyncReason, ok := p.consumeResyncRequired(); ok {
		if err := b.broadcastFullSnapshotWithResync(ctx, reloadReasonForResync(resyncReason), resyncReason, topologyDirty); err != nil {
			return err
		}
		b.broadcastAlertsIfDirty(alertsDirty)
		return nil
	}
	if topologyDirty {
		observability.Default().IncRefreshTopologyReload(refreshReloadReasonTopologyDirty)
		b.broadcastTopologyInvalidation(refreshReloadReasonTopologyDirty)
		b.broadcastAlertsIfDirty(alertsDirty)
		return nil
	}
	if forceFull {
		if err := b.broadcastFullSnapshot(ctx, refreshReloadReasonFullResync, false); err != nil {
			return err
		}
		b.broadcastAlertsIfDirty(alertsDirty)
		return nil
	}
	if len(dirtyDevices) == 0 && !alertsDirty {
		return nil
	}

	startedAt := time.Now()
	delta, requireFullSnapshot, err := b.buildDirtyOverviewDelta(dirtyDevices, alertsDirty)
	observability.Default().ObserveRefreshSnapshotBuild(refreshSnapshotModeDirty, time.Since(startedAt), err == nil)
	if err != nil {
		return err
	}
	if requireFullSnapshot {
		if err := b.broadcastFullSnapshot(ctx, refreshReloadReasonDirtyDeltaFallback, false); err != nil {
			return err
		}
		b.broadcastAlertsIfDirty(alertsDirty)
		return nil
	}
	if delta == nil {
		b.broadcastAlertsIfDirty(alertsDirty)
		return nil
	}

	p.runtime.mu.Lock()
	baseVersion := p.runtime.overviewVersion
	merged := mergeSnapshotPayload(p.runtime.lastSnapshot, delta)
	p.runtime.lastSnapshot = merged
	p.runtime.prevHashes = computeSnapshotHashes(merged)
	p.runtime.overviewVersion++
	version := p.runtime.overviewVersion
	p.runtime.mu.Unlock()

	p.hub.BroadcastOverviewDelta(delta, baseVersion, version, merged)
	b.broadcastAlertsIfDirty(alertsDirty)

	return nil
}

func (b *pipelineSnapshotBroadcaster) broadcastAlertsIfDirty(alertsDirty bool) {
	p := b.pipeline
	if !alertsDirty || p.hub == nil {
		return
	}

	p.hub.Broadcast(ws.Message{
		Type:    ws.MessageTypeAlert,
		Payload: p.runtime.getAlerts(),
	})
}

func (b *pipelineSnapshotBroadcaster) broadcastPollingHealthIfChanged() {
	p := b.pipeline
	if p == nil || p.runtime == nil || p.hub == nil {
		return
	}

	health := p.PollingHealth()
	p.runtime.mu.Lock()
	changed := !pollingHealthEqual(health, p.runtime.lastPollingHealth)
	if changed {
		p.runtime.lastPollingHealth = clonePollingHealth(health)
	}
	p.runtime.mu.Unlock()

	if changed {
		p.hub.Broadcast(ws.NewPollingHealthChangedMessage(health))
	}
}

func pollingHealthEqual(a, b polling.HealthSnapshot) bool {
	if a.EssentialOverloaded != b.EssentialOverloaded ||
		a.DegradedRisk != b.DegradedRisk ||
		a.EssentialQueueLagSeconds != b.EssentialQueueLagSeconds ||
		a.DeadlineMissTotal != b.DeadlineMissTotal ||
		a.ActiveWorkers != b.ActiveWorkers ||
		a.ConfiguredWorkers != b.ConfiguredWorkers ||
		len(a.Warnings) != len(b.Warnings) {
		return false
	}

	for i := range a.Warnings {
		if a.Warnings[i] != b.Warnings[i] {
			return false
		}
	}
	return true
}

func clonePollingHealth(health polling.HealthSnapshot) polling.HealthSnapshot {
	health.Warnings = append([]polling.CapacityWarning(nil), health.Warnings...)
	return health
}

func (b *pipelineSnapshotBroadcaster) broadcastFullSnapshot(_ context.Context, reason string, topologyChanged bool) error {
	p := b.pipeline
	observability.Default().IncRefreshTopologyReload(reason)
	snapshot, err := b.buildFullOverviewSnapshot()
	if err != nil {
		return err
	}

	p.runtime.mu.Lock()
	p.runtime.lastSnapshot = snapshot
	p.runtime.prevHashes = computeSnapshotHashes(snapshot)
	p.runtime.overviewVersion++
	version := p.runtime.overviewVersion
	p.runtime.mu.Unlock()

	p.hub.BroadcastOverviewSnapshot(snapshot, version)

	if topologyChanged {
		b.broadcastTopologyInvalidation(refreshReloadReasonTopologyDirty)
	}

	return nil
}

func (b *pipelineSnapshotBroadcaster) broadcastFullSnapshotWithResync(_ context.Context, reason string, resyncReason string, topologyChanged bool) error {
	p := b.pipeline
	observability.Default().IncRefreshTopologyReload(reason)
	snapshot, err := b.buildFullOverviewSnapshot()
	if err != nil {
		return err
	}

	p.runtime.mu.Lock()
	p.runtime.lastSnapshot = snapshot
	p.runtime.prevHashes = computeSnapshotHashes(snapshot)
	p.runtime.overviewVersion++
	version := p.runtime.overviewVersion
	p.runtime.mu.Unlock()

	p.hub.BroadcastOverviewResync(resyncReason, snapshot, version)

	if topologyChanged {
		b.broadcastTopologyInvalidation(refreshReloadReasonTopologyDirty)
	}

	return nil
}

func (b *pipelineSnapshotBroadcaster) broadcastTopologyInvalidation(reason string) {
	p := b.pipeline
	if p == nil || p.runtime == nil || p.hub == nil {
		return
	}

	p.runtime.mu.Lock()
	p.runtime.topologyVersion++
	version := p.runtime.topologyVersion
	p.runtime.mu.Unlock()

	p.hub.Broadcast(ws.NewTopologyChangedMessage(version, reason))
}

func (b *pipelineSnapshotBroadcaster) buildFullOverviewSnapshot() (_ *ws.SnapshotPayload, err error) {
	p := b.pipeline
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

	p.runtime.mu.RLock()
	alerts := cloneAlertGroups(p.runtime.alerts)
	promStatus := p.runtime.promStatus
	p.runtime.mu.RUnlock()

	return buildPipelineSnapshot(devices, links, p.stateStore.Snapshot(), alerts, promStatus), nil
}

func (b *pipelineSnapshotBroadcaster) buildDirtyOverviewDelta(dirtyDevices map[uuid.UUID]struct{}, alertsDirty bool) (*ws.SnapshotPayload, bool, error) {
	p := b.pipeline
	if len(dirtyDevices) == 0 && !alertsDirty {
		return nil, false, nil
	}

	devices, err := p.cache.GetDevices()
	if err != nil {
		return nil, false, err
	}

	links, err := p.cache.GetLinks()
	if err != nil {
		return nil, false, err
	}

	p.runtime.mu.RLock()
	alerts := cloneAlertGroups(p.runtime.alerts)
	promStatus := p.runtime.promStatus
	previousHashes := p.runtime.prevHashes
	p.runtime.mu.RUnlock()
	states := p.stateStore.Snapshot()

	if alertsDirty {
		if previousHashes == nil {
			return nil, true, nil
		}
		current := buildPipelineSnapshot(devices, links, states, alerts, promStatus)
		currentHashes := computeSnapshotHashes(current)
		return buildDelta(current, currentHashes, previousHashes), false, nil
	}

	delta := ws.EmptySnapshot()
	filteredDevices := filterDevicesByID(devices, dirtyDevices)
	filteredLinks := filterLinksByDeviceID(links, dirtyDevices)
	if len(filteredDevices) > 0 {
		contextIDs := make(map[uuid.UUID]struct{}, len(dirtyDevices)+len(filteredLinks))
		for id := range dirtyDevices {
			contextIDs[id] = struct{}{}
		}
		for _, link := range filteredLinks {
			contextIDs[link.SourceDeviceID] = struct{}{}
			contextIDs[link.TargetDeviceID] = struct{}{}
		}

		partial := buildPipelineSnapshot(filterDevicesByID(devices, contextIDs), filteredLinks, states, alerts, promStatus)
		for id := range dirtyDevices {
			deviceRuntime, ok := partial.Devices[id.String()]
			if !ok {
				continue
			}
			delta.Devices[id.String()] = deviceRuntime
		}
		for id, linkRuntime := range partial.Links {
			delta.Links[id] = linkRuntime
		}
	}

	if snapshotPayloadEmpty(delta) {
		return nil, false, nil
	}

	return delta, false, nil
}
