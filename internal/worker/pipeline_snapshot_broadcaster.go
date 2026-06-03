package worker

import (
	"context"
	"fmt"
	"log"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/lollinoo/theia/internal/domain"
	"github.com/lollinoo/theia/internal/logging"
	"github.com/lollinoo/theia/internal/observability"
	"github.com/lollinoo/theia/internal/polling"
	"github.com/lollinoo/theia/internal/state"
	"github.com/lollinoo/theia/internal/ws"
)

type pipelineSnapshotBroadcaster struct {
	pipeline *PipelineOrchestrator
}

var (
	snapshotAllPipelineState = func(store *state.Store) map[uuid.UUID]state.DeviceState {
		return store.Snapshot()
	}
	snapshotPipelineStateFor = func(store *state.Store, ids []uuid.UUID) map[uuid.UUID]state.DeviceState {
		return store.SnapshotFor(ids)
	}
)

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
	previousSnapshot := ws.CloneSnapshot(p.runtime.lastSnapshot)
	p.runtime.mu.RUnlock()

	startedAt := time.Now()
	snapshot := buildPipelineSnapshot(devices, links, snapshotAllPipelineState(p.stateStore), alerts, promStatus)
	observability.Default().ObserveRefreshSnapshotBuild(refreshSnapshotModeFull, time.Since(startedAt), true)
	currentHashes := computeSnapshotHashes(snapshot)
	drainedTopology := drainTopologyNotify(p.topologyNotify)

	p.runtime.mu.Lock()
	p.runtime.lastSnapshot = snapshot
	p.runtime.prevHashes = currentHashes
	p.runtime.previousAlertRuntime = alertRuntimeSummaryFromSnapshot(snapshot)
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
			patch := buildRuntimeDeltaPatch(delta, previousSnapshot)
			if patch != nil {
				p.runtime.mu.Lock()
				baseVersion := p.runtime.overviewVersion
				p.runtime.overviewVersion = baseVersion + 1
				version := p.runtime.overviewVersion
				p.runtime.mu.Unlock()
				p.hub.BroadcastOverviewDelta(patch, baseVersion, version, snapshot)
			}
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

	p.runtime.mu.RLock()
	missingDirtyDeviceRuntimeBase := len(dirtyDevices) > 0 && snapshotPayloadEmpty(p.runtime.lastSnapshot)
	p.runtime.mu.RUnlock()
	if missingDirtyDeviceRuntimeBase {
		if err := b.broadcastFullSnapshot(ctx, refreshReloadReasonDirtyDeltaFallback, false); err != nil {
			return err
		}
		b.broadcastAlertsIfDirty(alertsDirty)
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
	previousSnapshotForPatch := p.runtime.lastSnapshot
	if previousSnapshotForPatch == nil && alertsDirty {
		previousSnapshotForPatch = previousAlertRuntimePatchBase(delta, p.runtime.previousAlertRuntime)
	}
	patch := buildRuntimeDeltaPatch(delta, previousSnapshotForPatch)
	if patch == nil {
		p.runtime.mu.Unlock()
		b.broadcastAlertsIfDirty(alertsDirty)
		return nil
	}
	baseVersion := p.runtime.overviewVersion
	merged := mergeSnapshotPayload(p.runtime.lastSnapshot, delta)
	p.runtime.lastSnapshot = merged
	p.runtime.prevHashes = computeSnapshotHashes(merged)
	p.runtime.previousAlertRuntime = alertRuntimeSummaryFromSnapshot(merged)
	p.runtime.overviewVersion++
	version := p.runtime.overviewVersion
	p.runtime.mu.Unlock()

	p.hub.BroadcastOverviewDelta(patch, baseVersion, version, merged)
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
	now := p.clockNow()
	p.runtime.mu.Lock()
	changed := !pollingHealthEqual(health, p.runtime.lastPollingHealth)
	if !changed && health.ActiveWorkers != p.runtime.lastPollingHealth.ActiveWorkers {
		changed = pollingHealthActiveWorkerHeartbeatDue(p.runtime.lastPollingHealthAt, now)
	}
	if changed {
		p.runtime.lastPollingHealth = clonePollingHealth(health)
		p.runtime.lastPollingHealthAt = now
	}
	p.runtime.mu.Unlock()

	if changed {
		logging.Debugf(
			"polling health changed status=%s essential_overloaded=%t degraded_risk=%t essential_lag=%.1fs deadline_miss_total=%d active_workers=%d/%d queues=%s warnings=%d",
			health.Status(),
			health.EssentialOverloaded,
			health.DegradedRisk,
			health.EssentialQueueLagSeconds,
			health.DeadlineMissTotal,
			health.ActiveWorkers,
			health.ConfiguredWorkers,
			pollingHealthQueueSummary(health.Queues),
			len(health.Warnings),
		)
		p.hub.Broadcast(ws.NewPollingHealthChangedMessage(health))
	}
}

// pollingHealthEqual compares fields that should trigger immediate health broadcasts.
func pollingHealthEqual(a, b polling.HealthSnapshot) bool {
	if a.EssentialOverloaded != b.EssentialOverloaded ||
		a.DegradedRisk != b.DegradedRisk ||
		pollingHealthLagBucket(a.EssentialQueueLagSeconds) != pollingHealthLagBucket(b.EssentialQueueLagSeconds) ||
		a.DeadlineMissTotal != b.DeadlineMissTotal ||
		a.ConfiguredWorkers != b.ConfiguredWorkers ||
		!pollingHealthQueuesEqual(a.Queues, b.Queues) ||
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

func pollingHealthQueuesEqual(a, b map[string]polling.QueueSnapshot) bool {
	if len(a) != len(b) {
		return false
	}
	for key, aQueue := range a {
		bQueue, ok := b[key]
		if !ok {
			return false
		}
		if aQueue.ReadyDepth != bQueue.ReadyDepth ||
			pollingHealthLagBucket(aQueue.LagSeconds) != pollingHealthLagBucket(bQueue.LagSeconds) ||
			aQueue.ConfiguredWorkers != bQueue.ConfiguredWorkers {
			return false
		}
	}
	return true
}

const pollingHealthLagBucketSeconds = 5
const pollingHealthActiveWorkerHeartbeatInterval = time.Minute

func pollingHealthActiveWorkerHeartbeatDue(lastBroadcastAt time.Time, now time.Time) bool {
	if lastBroadcastAt.IsZero() {
		return true
	}
	return !now.Before(lastBroadcastAt.Add(pollingHealthActiveWorkerHeartbeatInterval))
}

func pollingHealthLagBucket(lagSeconds float64) int64 {
	if lagSeconds <= 0 {
		return 0
	}
	return int64(lagSeconds / pollingHealthLagBucketSeconds)
}

func pollingHealthQueueSummary(queues map[string]polling.QueueSnapshot) string {
	if len(queues) == 0 {
		return "-"
	}

	keys := make([]string, 0, len(queues))
	for key := range queues {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		queue := queues[key]
		parts = append(parts, fmt.Sprintf(
			"%s ready=%d lag=%.1fs active=%d/%d",
			key,
			queue.ReadyDepth,
			queue.LagSeconds,
			queue.ActiveWorkers,
			queue.ConfiguredWorkers,
		))
	}
	return strings.Join(parts, " ")
}

func clonePollingHealth(health polling.HealthSnapshot) polling.HealthSnapshot {
	health.Warnings = append([]polling.CapacityWarning(nil), health.Warnings...)
	if health.Queues != nil {
		queues := health.Queues
		health.Queues = make(map[string]polling.QueueSnapshot, len(queues))
		for key, queue := range queues {
			health.Queues[key] = queue
		}
	}
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
	p.runtime.previousAlertRuntime = alertRuntimeSummaryFromSnapshot(snapshot)
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
	p.runtime.previousAlertRuntime = alertRuntimeSummaryFromSnapshot(snapshot)
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

	return buildPipelineSnapshot(devices, links, snapshotAllPipelineState(p.stateStore), alerts, promStatus), nil
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
	previousAlertRuntime := cloneAlertRuntimeSummary(p.runtime.previousAlertRuntime)
	var previousSnapshotForHash *ws.SnapshotPayload
	previousSnapshotAvailable := false
	previousAlertDeviceIDs := make(map[uuid.UUID]struct{})
	if alertsDirty {
		previousSnapshotAvailable = p.runtime.lastSnapshot != nil
		if previousSnapshotAvailable {
			addPreviousAlertRuntimeDeviceIDs(previousAlertDeviceIDs, p.runtime.lastSnapshot)
			if previousHashes == nil {
				previousSnapshotForHash = ws.CloneSnapshot(p.runtime.lastSnapshot)
			}
		} else if len(dirtyDevices) == 0 {
			addPreviousAlertRuntimeSummaryDeviceIDs(previousAlertDeviceIDs, previousAlertRuntime)
			previousSnapshotAvailable = len(previousAlertDeviceIDs) > 0
		}
	}
	p.runtime.mu.RUnlock()

	if alertsDirty {
		if !previousSnapshotAvailable {
			return nil, true, nil
		}
		if previousHashes == nil && previousSnapshotForHash != nil {
			previousHashes = computeSnapshotHashes(previousSnapshotForHash)
		}
	}

	delta := ws.EmptySnapshot()
	filteredDevices := filterDevicesByID(devices, dirtyDevices)
	filteredLinks := filterLinksByDeviceID(links, dirtyDevices)
	contextIDs := make(map[uuid.UUID]struct{}, len(dirtyDevices)+len(filteredLinks))
	if len(filteredDevices) > 0 {
		for id := range dirtyDevices {
			contextIDs[id] = struct{}{}
		}
		for _, link := range filteredLinks {
			contextIDs[link.SourceDeviceID] = struct{}{}
			contextIDs[link.TargetDeviceID] = struct{}{}
		}
	}

	alertDeviceIDs := make(map[uuid.UUID]struct{})
	if alertsDirty {
		addCurrentAlertRuntimeDeviceIDs(alertDeviceIDs, alerts)
		for id := range previousAlertDeviceIDs {
			alertDeviceIDs[id] = struct{}{}
		}
		for id := range alertDeviceIDs {
			contextIDs[id] = struct{}{}
		}
	}

	if len(contextIDs) == 0 {
		return nil, false, nil
	}

	states := snapshotPipelineStateFor(p.stateStore, deviceIDSetToSlice(contextIDs))
	partial := buildPipelineSnapshot(filterDevicesByID(devices, contextIDs), filteredLinks, states, alerts, promStatus)

	if len(filteredDevices) > 0 {
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
	if alertsDirty {
		partialHashes := computeSnapshotHashes(partial)
		for id := range alertDeviceIDs {
			deviceID := id.String()
			currentHash, ok := partialHashes.devices[deviceID]
			if !ok {
				continue
			}
			if previousHashes == nil {
				if alertRuntimeFieldsChanged(partial.Devices[deviceID], previousAlertRuntime[id]) {
					delta.Devices[deviceID] = partial.Devices[deviceID]
				}
				continue
			}
			if previousHash, ok := previousHashes.devices[deviceID]; !ok || previousHash != currentHash {
				delta.Devices[deviceID] = partial.Devices[deviceID]
			}
		}
	}

	if snapshotPayloadEmpty(delta) {
		return nil, false, nil
	}

	return delta, false, nil
}

func deviceIDSetToSlice(ids map[uuid.UUID]struct{}) []uuid.UUID {
	out := make([]uuid.UUID, 0, len(ids))
	for id := range ids {
		if id == uuid.Nil {
			continue
		}
		out = append(out, id)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].String() < out[j].String()
	})
	return out
}

func addCurrentAlertRuntimeDeviceIDs(target map[uuid.UUID]struct{}, alerts map[uuid.UUID][]domain.AlertState) {
	for id, grouped := range alerts {
		if id == uuid.Nil || !alertStatesAffectRuntime(grouped) {
			continue
		}
		target[id] = struct{}{}
	}
}

func addPreviousAlertRuntimeDeviceIDs(target map[uuid.UUID]struct{}, snapshot *ws.SnapshotPayload) {
	if snapshot == nil {
		return
	}

	for id, runtime := range snapshot.Devices {
		if !deviceRuntimeHadAlertEffect(runtime) {
			continue
		}
		deviceID, err := uuid.Parse(id)
		if err != nil || deviceID == uuid.Nil {
			continue
		}
		target[deviceID] = struct{}{}
	}
}

func alertStatesAffectRuntime(alerts []domain.AlertState) bool {
	status, firingCount := summarizeAlerts(alerts)
	return firingCount > 0 || status != domain.AlertStatusNormal
}

func deviceRuntimeHadAlertEffect(runtime ws.DeviceRuntimeDTO) bool {
	if runtime.FiringAlertCount > 0 {
		return true
	}
	return runtime.AlertStatus != "" && !strings.EqualFold(runtime.AlertStatus, string(domain.AlertStatusNormal))
}

func alertRuntimeSummaryFromSnapshot(snapshot *ws.SnapshotPayload) map[uuid.UUID]ws.DeviceRuntimeDTO {
	summary := make(map[uuid.UUID]ws.DeviceRuntimeDTO)
	if snapshot == nil {
		return summary
	}
	for id, runtime := range snapshot.Devices {
		if !deviceRuntimeHadAlertEffect(runtime) {
			continue
		}
		deviceID, err := uuid.Parse(id)
		if err != nil || deviceID == uuid.Nil {
			continue
		}
		summary[deviceID] = ws.DeviceRuntimeDTO{
			DeviceID:         runtime.DeviceID,
			AlertStatus:      runtime.AlertStatus,
			FiringAlertCount: runtime.FiringAlertCount,
		}
	}
	return summary
}

func cloneAlertRuntimeSummary(summary map[uuid.UUID]ws.DeviceRuntimeDTO) map[uuid.UUID]ws.DeviceRuntimeDTO {
	if len(summary) == 0 {
		return nil
	}
	cloned := make(map[uuid.UUID]ws.DeviceRuntimeDTO, len(summary))
	for id, runtime := range summary {
		cloned[id] = runtime
	}
	return cloned
}

func addPreviousAlertRuntimeSummaryDeviceIDs(target map[uuid.UUID]struct{}, summary map[uuid.UUID]ws.DeviceRuntimeDTO) {
	for id, runtime := range summary {
		if id == uuid.Nil || !deviceRuntimeHadAlertEffect(runtime) {
			continue
		}
		target[id] = struct{}{}
	}
}

func alertRuntimeFieldsChanged(current, previous ws.DeviceRuntimeDTO) bool {
	return current.AlertStatus != previous.AlertStatus || current.FiringAlertCount != previous.FiringAlertCount
}

func previousAlertRuntimePatchBase(delta *ws.SnapshotPayload, summary map[uuid.UUID]ws.DeviceRuntimeDTO) *ws.SnapshotPayload {
	if delta == nil || len(summary) == 0 {
		return nil
	}
	previous := ws.EmptySnapshot()
	for id, current := range delta.Devices {
		deviceID, err := uuid.Parse(id)
		if err != nil || deviceID == uuid.Nil {
			continue
		}
		alertRuntime, ok := summary[deviceID]
		if !ok {
			continue
		}
		base := current
		base.AlertStatus = alertRuntime.AlertStatus
		base.FiringAlertCount = alertRuntime.FiringAlertCount
		previous.Devices[id] = base
	}
	if len(previous.Devices) == 0 {
		return nil
	}
	return previous
}
