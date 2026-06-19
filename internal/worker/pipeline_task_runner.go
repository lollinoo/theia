package worker

// This file defines pipeline task runner worker behavior, background lifecycle, and runtime state updates.

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/collector"
	"github.com/lollinoo/theia/internal/domain"
	"github.com/lollinoo/theia/internal/observability"
	"github.com/lollinoo/theia/internal/polling"
	"github.com/lollinoo/theia/internal/scheduler"
	"github.com/lollinoo/theia/internal/service"
	"github.com/lollinoo/theia/internal/snmp"
	"github.com/lollinoo/theia/internal/state"
	"github.com/lollinoo/theia/internal/ws"
)

type pipelineTaskRunner struct {
	pipeline *PipelineOrchestrator
}

func (r *pipelineTaskRunner) runWorker(ctx context.Context) {
	p := r.pipeline
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
			r.runTask(ctx, task)
		}
	}
}

func (r *pipelineTaskRunner) runTask(ctx context.Context, task scheduler.PollTask) {
	p := r.pipeline
	finishedAt := time.Now().UTC()
	defer func() {
		if p.scheduler != nil {
			p.scheduler.Complete(scheduler.Completion{
				RunID:      task.RunID,
				Key:        task.Key,
				FinishedAt: time.Now().UTC(),
			})
		}
	}()

	if task.Kind == polling.TaskKindEssential {
		if p.essential == nil || p.stateStore == nil {
			return
		}

		profile := r.timeoutProfile(polling.LaneEssential)
		result := p.essential.Poll(ctx, task.Device, profile.Timeout, profile.Retries, r.networkProbePorts(task.Device))
		finishedAt = completionTime(result.CollectedAt)
		observability.Default().IncPollResult(domain.VolatilityClassPerformance, result.Err == nil)

		update := result.ToStoreUpdate(task.ExpectedInterval, task.DeadlineMissed)
		if task.DeadlineMissed && update.Essential != nil {
			update.Essential.Overloaded = p.PollingHealth().EssentialOverloaded
		}
		p.stateStore.Update(update)
		r.publishSubscribedDetailDelta(task.Device)
		return
	}

	if task.Kind == polling.TaskKindBootstrap {
		if p.staticCollector == nil || p.stateStore == nil {
			return
		}

		profile := r.timeoutProfile(polling.LaneBootstrap)
		result := p.staticCollector.Poll(ctx, task.Device, profile.Timeout, profile.Retries, r.bootstrapTopologyDiscoveryMode(task.Device))
		finishedAt = completionTime(result.CollectedAt)
		observability.Default().IncPollResult(domain.VolatilityClassStatic, result.Err == nil)
		p.stateStore.Update(state.StateUpdate{
			DeviceID:         task.Device.ID,
			VolatilityClass:  domain.VolatilityClassStatic,
			Metrics:          staticResultMetrics(result),
			PollSuccess:      result.Err == nil,
			ExpectedInterval: task.ExpectedInterval,
			Timestamp:        finishedAt,
		})
		r.publishSubscribedDetailDelta(task.Device)
		if result.Err != nil || p.topologyService == nil {
			return
		}

		r.persistStaticDiscoveryForced(task.Device, result)
		return
	}

	if task.Device.DeviceType == domain.DeviceTypeVirtual {
		finishedAt = r.runVirtualTask(ctx, task)
		return
	}

	switch task.VolatilityClass {
	case domain.VolatilityClassPerformance:
		if p.performance == nil || p.stateStore == nil {
			return
		}
		if r.knownSNMPUnreachable(task.Device.ID) {
			return
		}

		profile := r.timeoutProfile(polling.LaneBackground)
		result := p.performance.PollWithOptions(ctx, task.Device, profile.Timeout, profile.Retries, collector.PerformancePollOptions{
			ExpectedInterval: task.ExpectedInterval,
			CounterCooldown:  p.runtime,
		})
		finishedAt = completionTime(result.CollectedAt)
		observability.Default().IncPollResult(task.VolatilityClass, result.Err == nil)

		update := result.ToStoreUpdate(task.ExpectedInterval)
		if result.Err == nil {
			if p.prometheus != nil && p.GetPrometheusStatus().Enabled {
				enrichment, err := p.prometheus.CollectDeviceEnrichment(ctx, task.Device)
				if err == nil && enrichment.Hostname != "" {
					p.prometheusMonitor.recordHostname(task.Device.ID, enrichment.Hostname)
				}
			}

			if len(result.Counters) > 0 {
				p.runtime.mu.Lock()
				linkMetrics, next := collector.ComputeCounterRates(
					result.Counters,
					p.runtime.prevCounters[task.Device.ID],
					completionTime(result.CollectedAt),
					task.ExpectedInterval,
				)
				p.runtime.prevCounters[task.Device.ID] = next
				p.runtime.mu.Unlock()
				update.LinkMetrics = linkMetrics
			}
		}

		p.stateStore.Update(update)
		r.publishSubscribedDetailDelta(task.Device)

	case domain.VolatilityClassOperational:
		if p.operational == nil || p.stateStore == nil {
			return
		}
		if r.knownSNMPUnreachable(task.Device.ID) {
			return
		}

		profile := r.timeoutProfile(polling.LaneBackground)
		result := p.operational.Poll(ctx, task.Device, profile.Timeout, profile.Retries)
		finishedAt = completionTime(result.CollectedAt)
		observability.Default().IncPollResult(task.VolatilityClass, result.Err == nil)
		p.stateStore.Update(result.ToStoreUpdate(task.ExpectedInterval))
		r.publishSubscribedDetailDelta(task.Device)

	case domain.VolatilityClassStatic:
		if p.staticCollector == nil || p.stateStore == nil {
			return
		}

		profile := r.timeoutProfile(polling.LaneBackground)
		result := p.staticCollector.Poll(ctx, task.Device, profile.Timeout, profile.Retries, r.topologyDiscoveryMode(task.Device))
		finishedAt = completionTime(result.CollectedAt)
		observability.Default().IncPollResult(task.VolatilityClass, result.Err == nil)
		p.stateStore.Update(state.StateUpdate{
			DeviceID:         task.Device.ID,
			VolatilityClass:  domain.VolatilityClassStatic,
			Metrics:          staticResultMetrics(result),
			PollSuccess:      result.Err == nil,
			ExpectedInterval: task.ExpectedInterval,
			Timestamp:        completionTime(result.CollectedAt),
		})
		r.publishSubscribedDetailDelta(task.Device)
		if result.Err != nil || p.topologyService == nil {
			return
		}

		r.persistStaticDiscovery(task.Device, result)
	}
}

func staticResultMetrics(result collector.StaticResult) *domain.DeviceMetrics {
	if result.Err != nil {
		return nil
	}
	metrics := result.Metrics
	return &metrics
}

func (r *pipelineTaskRunner) knownSNMPUnreachable(deviceID uuid.UUID) bool {
	p := r.pipeline
	if p == nil || p.stateStore == nil || deviceID == uuid.Nil {
		return false
	}
	deviceState, ok := p.stateStore.GetDevice(deviceID)
	return ok && deviceState.SNMPReachable == polling.TriStateFalse
}

func (r *pipelineTaskRunner) persistStaticDiscovery(device domain.Device, result collector.StaticResult) {
	r.persistStaticDiscoveryWithPolicy(device, result, false)
}

func (r *pipelineTaskRunner) persistStaticDiscoveryForced(device domain.Device, result collector.StaticResult) {
	r.persistStaticDiscoveryWithPolicy(device, result, true)
}

func (r *pipelineTaskRunner) persistStaticDiscoveryWithPolicy(device domain.Device, result collector.StaticResult, force bool) {
	p := r.pipeline
	force = force || staticPersistenceRequiresBootstrapFollowup(device)
	fingerprint := staticDiscoveryFingerprint(device, result)
	topologyFingerprint := staticDiscoveryTopologyFingerprint(result)
	if !force {
		shouldPersist, skipReason := p.shouldPersistStaticDiscovery(device.ID, fingerprint)
		if !shouldPersist {
			if skipReason == "" {
				skipReason = "unchanged"
			}
			observability.Default().IncStaticPersistenceSkip(skipReason)
			return
		}
	}
	skipTopologyMaterialization := !force && p.staticTopologyFingerprintUnchanged(device.ID, topologyFingerprint)

	persisted, err := p.topologyService.ApplyStaticDiscovery(device.ID, service.StaticDiscoveryInput{
		SysName:                     result.SysName,
		SysDescr:                    result.SysDescr,
		SysObjectID:                 result.SysObjectID,
		HardwareModel:               result.HardwareModel,
		OSVersion:                   result.OSVersion,
		Vendor:                      result.Vendor,
		DeviceType:                  result.DeviceType,
		Interfaces:                  append([]domain.Interface(nil), result.Interfaces...),
		Neighbors:                   append([]snmp.NeighborInfo(nil), result.Neighbors...),
		NeighborDiscoveryProtocols:  append([]domain.DiscoveryProtocol(nil), result.NeighborDiscoveryProtocols...),
		NeighborDiscoveryFailures:   append([]snmp.NeighborDiscoveryFailure(nil), result.NeighborDiscoveryFailures...),
		SkipTopologyMaterialization: skipTopologyMaterialization,
	})
	if err != nil {
		log.Printf("pipeline: static persistence failed for %s: %v", device.ID, err)
		return
	}
	if skipTopologyMaterialization {
		observability.Default().IncTopologyMaterializationSkip("unchanged")
	}
	p.rememberStaticDiscoveryPersistence(device.ID, fingerprint, topologyFingerprint, persisted)
	if persisted.TopologyChanged && p.topologyNotify != nil {
		select {
		case p.topologyNotify <- struct{}{}:
		default:
		}
	}
}

func staticPersistenceRequiresBootstrapFollowup(device domain.Device) bool {
	switch domain.NormalizeTopologyBootstrapState(device.TopologyBootstrapState) {
	case domain.TopologyBootstrapStatePending, domain.TopologyBootstrapStateFollowupScheduled:
		return true
	default:
		return false
	}
}

func (p *PipelineOrchestrator) shouldPersistStaticDiscovery(deviceID uuid.UUID, fingerprint string) (bool, string) {
	if p == nil || deviceID == uuid.Nil || fingerprint == "" {
		return true, ""
	}
	now := p.staticPersistenceClock().UTC()
	maxAge := p.staticPersistenceMaxAge
	if maxAge <= 0 {
		maxAge = staticPersistenceSelfHealInterval
	}
	spread := p.staticPersistenceSelfHealSpread
	if spread <= 0 {
		spread = staticPersistenceSelfHealSpread
	}

	p.staticPersistenceMu.Lock()
	defer p.staticPersistenceMu.Unlock()
	entry, ok := p.staticPersistenceCache[deviceID]
	if !ok || entry.fingerprint != fingerprint || entry.persistedAt.IsZero() {
		return true, ""
	}
	selfHealEligibleAt := entry.persistedAt.Add(maxAge)
	if now.Before(selfHealEligibleAt) {
		return false, "unchanged"
	}
	selfHealDeadline := selfHealEligibleAt.Add(staticPersistenceSelfHealJitter(deviceID, spread))
	if now.Before(selfHealDeadline) {
		return false, "self_heal_deferred"
	}
	return true, ""
}

func (p *PipelineOrchestrator) staticTopologyFingerprintUnchanged(deviceID uuid.UUID, topologyFingerprint string) bool {
	if p == nil || deviceID == uuid.Nil || topologyFingerprint == "" {
		return false
	}
	now := p.staticPersistenceClock().UTC()
	maxAge := p.staticPersistenceMaxAge
	if maxAge <= 0 {
		maxAge = staticPersistenceSelfHealInterval
	}
	spread := p.staticPersistenceSelfHealSpread
	if spread <= 0 {
		spread = staticPersistenceSelfHealSpread
	}

	p.staticPersistenceMu.Lock()
	defer p.staticPersistenceMu.Unlock()
	entry, ok := p.staticPersistenceCache[deviceID]
	if !ok ||
		entry.topologyFingerprint != topologyFingerprint ||
		entry.topologyMaterializedAt.IsZero() ||
		entry.topologyUnresolvedNeighbors > 0 {
		return false
	}
	selfHealDeadline := entry.topologyMaterializedAt.
		Add(maxAge).
		Add(staticPersistenceSelfHealJitter(deviceID, spread))
	return now.Before(selfHealDeadline)
}

func staticPersistenceSelfHealJitter(deviceID uuid.UUID, spread time.Duration) time.Duration {
	if deviceID == uuid.Nil || spread <= 0 {
		return 0
	}
	sum := sha256.Sum256(deviceID[:])
	return time.Duration(binary.BigEndian.Uint64(sum[:8]) % uint64(spread))
}

func (p *PipelineOrchestrator) rememberStaticDiscoveryPersistence(
	deviceID uuid.UUID,
	fingerprint string,
	topologyFingerprint string,
	result service.StaticPersistenceResult,
) {
	if p == nil || deviceID == uuid.Nil || fingerprint == "" {
		return
	}
	now := p.staticPersistenceClock().UTC()
	p.staticPersistenceMu.Lock()
	defer p.staticPersistenceMu.Unlock()
	if p.staticPersistenceCache == nil {
		p.staticPersistenceCache = make(map[uuid.UUID]staticPersistenceCacheEntry)
	}
	entry := p.staticPersistenceCache[deviceID]
	if result.TopologyMaterialized {
		entry.topologyFingerprint = topologyFingerprint
		entry.topologyMaterializedAt = now
		entry.topologyUnresolvedNeighbors = result.UnresolvedNeighbors
	} else if entry.topologyFingerprint != topologyFingerprint {
		entry.topologyFingerprint = topologyFingerprint
		entry.topologyMaterializedAt = time.Time{}
		entry.topologyUnresolvedNeighbors = 0
	}
	entry.fingerprint = fingerprint
	entry.persistedAt = now
	p.staticPersistenceCache[deviceID] = entry
}

func (p *PipelineOrchestrator) staticPersistenceClock() time.Time {
	if p != nil && p.staticPersistenceNow != nil {
		return p.staticPersistenceNow()
	}
	return time.Now()
}

type staticDiscoveryFingerprintPayload struct {
	SysName                    string
	SysDescr                   string
	SysObjectID                string
	HardwareModel              string
	OSVersion                  string
	Vendor                     string
	DeviceType                 domain.DeviceType
	Interfaces                 []staticDiscoveryInterfaceFingerprint
	Neighbors                  []staticDiscoveryNeighborFingerprint
	NeighborDiscoveryProtocols []domain.DiscoveryProtocol
	NeighborDiscoveryFailures  []snmp.NeighborDiscoveryFailure
}

type staticDiscoveryTopologyFingerprintPayload struct {
	Neighbors                  []staticDiscoveryNeighborFingerprint
	NeighborDiscoveryProtocols []domain.DiscoveryProtocol
	NeighborDiscoveryFailures  []staticDiscoveryNeighborFailureFingerprint
}

type staticDiscoveryInterfaceFingerprint struct {
	IfIndex     int
	IfName      string
	IfDescr     string
	Speed       int64
	AdminStatus string
	OperStatus  string
}

type staticDiscoveryNeighborFingerprint struct {
	RemoteChassisID string
	RemotePortID    string
	RemoteSysName   string
	LocalIfIndex    int
	LocalIfName     string
	Protocol        domain.DiscoveryProtocol
}

type staticDiscoveryNeighborFailureFingerprint struct {
	Protocol domain.DiscoveryProtocol
	OID      string
	Critical bool
}

func staticDiscoveryFingerprint(device domain.Device, result collector.StaticResult) string {
	payload := staticDiscoveryFingerprintPayload{
		SysName:                    strings.TrimSpace(result.SysName),
		SysDescr:                   strings.TrimSpace(result.SysDescr),
		SysObjectID:                strings.TrimSpace(result.SysObjectID),
		HardwareModel:              strings.TrimSpace(result.HardwareModel),
		OSVersion:                  strings.TrimSpace(result.OSVersion),
		Vendor:                     strings.TrimSpace(result.Vendor),
		DeviceType:                 result.DeviceType,
		Interfaces:                 staticDiscoveryInterfaceFingerprints(device.Interfaces, result.Interfaces),
		Neighbors:                  staticDiscoveryNeighborFingerprints(result.Neighbors),
		NeighborDiscoveryProtocols: append([]domain.DiscoveryProtocol(nil), result.NeighborDiscoveryProtocols...),
		NeighborDiscoveryFailures:  append([]snmp.NeighborDiscoveryFailure(nil), result.NeighborDiscoveryFailures...),
	}
	sort.Slice(payload.NeighborDiscoveryProtocols, func(i, j int) bool {
		return payload.NeighborDiscoveryProtocols[i] < payload.NeighborDiscoveryProtocols[j]
	})
	sort.Slice(payload.NeighborDiscoveryFailures, func(i, j int) bool {
		left := payload.NeighborDiscoveryFailures[i]
		right := payload.NeighborDiscoveryFailures[j]
		if left.Protocol != right.Protocol {
			return left.Protocol < right.Protocol
		}
		if left.OID != right.OID {
			return left.OID < right.OID
		}
		if left.Critical != right.Critical {
			return !left.Critical && right.Critical
		}
		return left.Error < right.Error
	})

	encoded, err := json.Marshal(payload)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:])
}

func staticDiscoveryTopologyFingerprint(result collector.StaticResult) string {
	payload := staticDiscoveryTopologyFingerprintPayload{
		Neighbors:                  staticDiscoveryNeighborFingerprints(result.Neighbors),
		NeighborDiscoveryProtocols: append([]domain.DiscoveryProtocol(nil), result.NeighborDiscoveryProtocols...),
		NeighborDiscoveryFailures:  staticDiscoveryNeighborFailureFingerprints(result.NeighborDiscoveryFailures),
	}
	sort.Slice(payload.NeighborDiscoveryProtocols, func(i, j int) bool {
		return payload.NeighborDiscoveryProtocols[i] < payload.NeighborDiscoveryProtocols[j]
	})

	encoded, err := json.Marshal(payload)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:])
}

func staticDiscoveryInterfaceFingerprints(existing []domain.Interface, observed []domain.Interface) []staticDiscoveryInterfaceFingerprint {
	interfaces := canonicalStaticDiscoveryInterfaces(existing, observed)
	if len(interfaces) == 0 {
		return nil
	}
	fingerprints := make([]staticDiscoveryInterfaceFingerprint, 0, len(interfaces))
	for _, iface := range interfaces {
		fingerprints = append(fingerprints, staticDiscoveryInterfaceFingerprint{
			IfIndex:     iface.IfIndex,
			IfName:      strings.TrimSpace(iface.IfName),
			IfDescr:     strings.TrimSpace(iface.IfDescr),
			Speed:       iface.Speed,
			AdminStatus: strings.TrimSpace(iface.AdminStatus),
			OperStatus:  strings.TrimSpace(iface.OperStatus),
		})
	}
	sort.Slice(fingerprints, func(i, j int) bool {
		left := fingerprints[i]
		right := fingerprints[j]
		if left.IfIndex != right.IfIndex {
			return left.IfIndex < right.IfIndex
		}
		if left.IfName != right.IfName {
			return left.IfName < right.IfName
		}
		return left.IfDescr < right.IfDescr
	})
	return fingerprints
}

func canonicalStaticDiscoveryInterfaces(existing []domain.Interface, observed []domain.Interface) []domain.Interface {
	if len(existing) == 0 {
		return append([]domain.Interface(nil), observed...)
	}
	if len(observed) == 0 {
		return append([]domain.Interface(nil), existing...)
	}

	merged := append([]domain.Interface(nil), existing...)
	indexByKey := make(map[string]int, len(existing)*3)
	for index, iface := range merged {
		for _, key := range staticDiscoveryInterfaceIdentityKeys(iface) {
			if _, exists := indexByKey[key]; !exists {
				indexByKey[key] = index
			}
		}
	}

	for _, iface := range observed {
		matchIndex := -1
		for _, key := range staticDiscoveryInterfaceIdentityKeys(iface) {
			if index, ok := indexByKey[key]; ok {
				matchIndex = index
				break
			}
		}
		if matchIndex >= 0 {
			merged[matchIndex] = canonicalStaticDiscoveryInterface(merged[matchIndex], iface)
			for _, key := range staticDiscoveryInterfaceIdentityKeys(merged[matchIndex]) {
				if _, exists := indexByKey[key]; !exists {
					indexByKey[key] = matchIndex
				}
			}
			continue
		}
		merged = append(merged, iface)
		newIndex := len(merged) - 1
		for _, key := range staticDiscoveryInterfaceIdentityKeys(iface) {
			if _, exists := indexByKey[key]; !exists {
				indexByKey[key] = newIndex
			}
		}
	}
	return merged
}

func canonicalStaticDiscoveryInterface(existing domain.Interface, observed domain.Interface) domain.Interface {
	merged := existing
	if observed.IfIndex != 0 {
		merged.IfIndex = observed.IfIndex
	}
	if strings.TrimSpace(observed.IfName) != "" {
		merged.IfName = observed.IfName
	}
	if strings.TrimSpace(observed.IfDescr) != "" {
		merged.IfDescr = observed.IfDescr
	}
	if observed.Speed > 0 {
		merged.Speed = observed.Speed
	}
	if strings.TrimSpace(observed.AdminStatus) != "" {
		merged.AdminStatus = observed.AdminStatus
	}
	if strings.TrimSpace(observed.OperStatus) != "" {
		merged.OperStatus = observed.OperStatus
	}
	return merged
}

func staticDiscoveryInterfaceIdentityKeys(iface domain.Interface) []string {
	keys := make([]string, 0, 3)
	if iface.IfIndex > 0 {
		keys = append(keys, fmt.Sprintf("index:%d", iface.IfIndex))
	}
	if key := normalizedStaticDiscoveryInterfaceName(iface.IfName); key != "" {
		keys = append(keys, "name:"+key)
	}
	if key := normalizedStaticDiscoveryInterfaceName(iface.IfDescr); key != "" {
		keys = append(keys, "name:"+key)
	}
	return keys
}

func normalizedStaticDiscoveryInterfaceName(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}

func staticDiscoveryNeighborFingerprints(neighbors []snmp.NeighborInfo) []staticDiscoveryNeighborFingerprint {
	if len(neighbors) == 0 {
		return nil
	}
	fingerprints := make([]staticDiscoveryNeighborFingerprint, 0, len(neighbors))
	for _, neighbor := range neighbors {
		fingerprints = append(fingerprints, staticDiscoveryNeighborFingerprint{
			RemoteChassisID: strings.TrimSpace(neighbor.RemoteChassisID),
			RemotePortID:    strings.TrimSpace(neighbor.RemotePortID),
			RemoteSysName:   strings.TrimSpace(neighbor.RemoteSysName),
			LocalIfIndex:    neighbor.LocalIfIndex,
			LocalIfName:     strings.TrimSpace(neighbor.LocalIfName),
			Protocol:        neighbor.Protocol,
		})
	}
	sort.Slice(fingerprints, func(i, j int) bool {
		left := fingerprints[i]
		right := fingerprints[j]
		if left.Protocol != right.Protocol {
			return left.Protocol < right.Protocol
		}
		if left.LocalIfIndex != right.LocalIfIndex {
			return left.LocalIfIndex < right.LocalIfIndex
		}
		if left.LocalIfName != right.LocalIfName {
			return left.LocalIfName < right.LocalIfName
		}
		if left.RemoteSysName != right.RemoteSysName {
			return left.RemoteSysName < right.RemoteSysName
		}
		if left.RemoteChassisID != right.RemoteChassisID {
			return left.RemoteChassisID < right.RemoteChassisID
		}
		return left.RemotePortID < right.RemotePortID
	})
	return fingerprints
}

func staticDiscoveryNeighborFailureFingerprints(failures []snmp.NeighborDiscoveryFailure) []staticDiscoveryNeighborFailureFingerprint {
	if len(failures) == 0 {
		return nil
	}
	fingerprints := make([]staticDiscoveryNeighborFailureFingerprint, 0, len(failures))
	for _, failure := range failures {
		fingerprints = append(fingerprints, staticDiscoveryNeighborFailureFingerprint{
			Protocol: failure.Protocol,
			OID:      strings.TrimSpace(failure.OID),
			Critical: failure.Critical,
		})
	}
	sort.Slice(fingerprints, func(i, j int) bool {
		left := fingerprints[i]
		right := fingerprints[j]
		if left.Protocol != right.Protocol {
			return left.Protocol < right.Protocol
		}
		if left.OID != right.OID {
			return left.OID < right.OID
		}
		if left.Critical != right.Critical {
			return !left.Critical && right.Critical
		}
		return false
	})
	return fingerprints
}

func (r *pipelineTaskRunner) runVirtualTask(ctx context.Context, task scheduler.PollTask) time.Time {
	if domain.IsVirtualNoIPDevice(task.Device) {
		return time.Now().UTC()
	}

	switch task.VolatilityClass {
	case domain.VolatilityClassOperational:
		return r.runVirtualOperationalTask(ctx, task)
	case domain.VolatilityClassPerformance, domain.VolatilityClassStatic:
		return time.Now().UTC()
	default:
		return time.Now().UTC()
	}
}

func (r *pipelineTaskRunner) runVirtualOperationalTask(ctx context.Context, task scheduler.PollTask) time.Time {
	p := r.pipeline
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
				p.prometheusMonitor.recordHostname(task.Device.ID, enrichment.Hostname)
			}
			if enrichment.ProbeReachable != nil {
				result.Reachable = *enrichment.ProbeReachable
				completedAt := completionTime(result.CollectedAt)
				p.stateStore.Update(virtualReachabilityStoreUpdate(task, result))
				p.stateStore.Update(state.StateUpdate{
					DeviceID:         task.Device.ID,
					VolatilityClass:  domain.VolatilityClassPerformance,
					PollSuccess:      true,
					ExpectedInterval: task.ExpectedInterval,
					Timestamp:        completedAt,
				})
				r.publishSubscribedDetailDelta(task.Device)
				return completedAt
			}
		}
	}

	profile := r.timeoutProfile(polling.LaneBackground)
	if err := service.ProbeVirtualReachability(ctx, task.Device.IP, profile.Timeout, r.networkProbePorts(task.Device)); err != nil {
		result.Err = err
	} else {
		result.Reachable = true
	}
	observability.Default().IncPollResult(task.VolatilityClass, result.Err == nil)

	completedAt := completionTime(result.CollectedAt)
	p.stateStore.Update(virtualReachabilityStoreUpdate(task, result))
	// Virtual nodes only run the operational tier, so stamp freshness metadata
	// explicitly to keep the UI footer out of the "waiting for first poll" state.
	p.stateStore.Update(state.StateUpdate{
		DeviceID:         task.Device.ID,
		VolatilityClass:  domain.VolatilityClassPerformance,
		PollSuccess:      result.Err == nil,
		ExpectedInterval: task.ExpectedInterval,
		Timestamp:        completedAt,
	})
	r.publishSubscribedDetailDelta(task.Device)
	return completedAt
}

func virtualReachabilityStoreUpdate(task scheduler.PollTask, result collector.OperationalResult) state.StateUpdate {
	networkReachable := polling.TriStateFalse
	pollStatus := polling.PollStatusFailed
	if result.Reachable {
		networkReachable = polling.TriStateTrue
		pollStatus = polling.PollStatusComplete
	}

	return state.StateUpdate{
		DeviceID:         task.Device.ID,
		VolatilityClass:  domain.VolatilityClassOperational,
		PollSuccess:      result.Reachable,
		ExpectedInterval: task.ExpectedInterval,
		Timestamp:        completionTime(result.CollectedAt),
		Essential: &state.EssentialUpdate{
			PollStatus:       pollStatus,
			NetworkReachable: networkReachable,
			SNMPReachable:    polling.TriStateUnknown,
			Uptime:           polling.FieldStateMissing,
			CPU:              polling.FieldStateMissing,
			Memory:           polling.FieldStateMissing,
		},
	}
}

func (r *pipelineTaskRunner) publishSubscribedDetailDelta(device domain.Device) {
	p := r.pipeline
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

	p.runtime.mu.RLock()
	promStatus := p.runtime.promStatus
	alerts := cloneAlertGroups(p.runtime.alerts)
	version := p.runtime.overviewVersion
	p.runtime.mu.RUnlock()

	states := p.stateStore.Snapshot()
	devicesByID := map[uuid.UUID]domain.Device{device.ID: device}
	var linkRuntimes []ws.LinkRuntimeDTO
	if p.cache != nil {
		if cachedDevices, err := p.cache.GetDevices(); err == nil {
			for _, cachedDevice := range cachedDevices {
				devicesByID[cachedDevice.ID] = cachedDevice
			}
		}
		if links, err := p.cache.GetLinks(); err == nil {
			linkRuntimes = buildDeviceLinkRuntimeDTOs(device, deviceState, devicesByID, states, links, promStatus)
		}
	}

	delta := buildDeviceDetailDeltaWithLinks(device, deviceState, linkRuntimes, alerts[device.ID], promStatus)
	for _, client := range subscribers {
		p.hub.SendTo(client, ws.NewSnapshotDeltaMessage(delta, version, version))
	}
}

func (r *pipelineTaskRunner) snmpTimeout() time.Duration {
	p := r.pipeline
	if p.settingsRepo == nil {
		return 10 * time.Second
	}

	value, err := p.settingsRepo.Get(domain.SettingSNMPTimeout)
	if err != nil {
		return 10 * time.Second
	}

	seconds := domain.CoerceConstrainedInt(domain.SettingSNMPTimeout, value, 10)
	return time.Duration(seconds) * time.Second
}

func (r *pipelineTaskRunner) snmpRetries() int {
	p := r.pipeline
	if p.settingsRepo == nil {
		return 2
	}

	value, err := p.settingsRepo.Get(domain.SettingSNMPRetries)
	if err != nil {
		return 2
	}

	return domain.CoerceConstrainedInt(domain.SettingSNMPRetries, value, 2)
}

func (r *pipelineTaskRunner) timeoutProfile(lane polling.Lane) polling.TimeoutProfile {
	policy, _ := polling.PolicyFromSettings(r.pipeline.settingsRepo, 0, 300*time.Millisecond, 0)
	if profile, ok := policy.Timeouts[lane]; ok {
		return profile
	}
	return polling.TimeoutProfile{Timeout: r.snmpTimeout(), Retries: r.snmpRetries()}
}

func (r *pipelineTaskRunner) networkProbePorts(device domain.Device) []int {
	return domain.ResolveProbePorts(r.primaryAddressProbePorts(device), device.ProbePorts, r.globalNetworkProbePorts())
}

func (r *pipelineTaskRunner) primaryAddressProbePorts(device domain.Device) []int {
	primary := domain.PrimaryAddress(device)
	if primary == "" {
		return nil
	}
	normalizedPrimary := domain.NormalizeDeviceAddressValue(primary)
	for _, address := range device.Addresses {
		if normalizedPrimary == "" || domain.NormalizeDeviceAddressValue(address.Address) != normalizedPrimary {
			continue
		}
		return address.ProbePorts
	}
	return nil
}

func (r *pipelineTaskRunner) globalNetworkProbePorts() []int {
	if r == nil || r.pipeline == nil || r.pipeline.settingsRepo == nil {
		return domain.CoerceNetworkProbePortsCSV("")
	}
	value, err := r.pipeline.settingsRepo.Get(domain.SettingNetworkProbePorts)
	if err != nil {
		return domain.CoerceNetworkProbePortsCSV("")
	}
	return domain.CoerceNetworkProbePortsCSV(strings.TrimSpace(value))
}

func (r *pipelineTaskRunner) topologyDiscoveryMode(device domain.Device) domain.TopologyDiscoveryMode {
	// Regular static polling must never reopen bootstrap-once discovery windows.
	// One-shot topology discovery is handled explicitly by DeviceService on add,
	// manual runs, settings changes, and delayed reprobe follow-ups.
	mode := r.resolvedTopologyDiscoveryMode(device)
	if mode == domain.TopologyDiscoveryModeBootstrapOnce {
		return domain.TopologyDiscoveryModeOff
	}
	return mode
}

func (r *pipelineTaskRunner) bootstrapTopologyDiscoveryMode(device domain.Device) domain.TopologyDiscoveryMode {
	defaultMode := r.defaultTopologyDiscoveryMode()
	return domain.ResolveTopologyDiscoveryMode(&device, defaultMode)
}

func (r *pipelineTaskRunner) resolvedTopologyDiscoveryMode(device domain.Device) domain.TopologyDiscoveryMode {
	defaultMode := r.defaultTopologyDiscoveryMode()
	mode := domain.NormalizeTopologyDiscoveryMode(device.TopologyDiscoveryMode, domain.TopologyDiscoveryModeInherit)
	if mode == domain.TopologyDiscoveryModeInherit {
		mode = defaultMode
	}
	return mode
}

func (r *pipelineTaskRunner) defaultTopologyDiscoveryMode() domain.TopologyDiscoveryMode {
	p := r.pipeline
	defaultMode := domain.TopologyDiscoveryModeLLDPCDP
	if p != nil && p.settingsRepo != nil {
		if value, err := p.settingsRepo.Get(domain.SettingTopologyDiscoveryDefaultMode); err == nil {
			defaultMode = domain.NormalizeTopologyDiscoveryMode(domain.TopologyDiscoveryMode(value), domain.TopologyDiscoveryModeLLDPCDP)
		}
	}
	return defaultMode
}

func (p *PipelineOrchestrator) runTask(ctx context.Context, task scheduler.PollTask) {
	p.taskRunner.runTask(ctx, task)
}

func (p *PipelineOrchestrator) publishSubscribedDetailDelta(device domain.Device) {
	p.taskRunner.publishSubscribedDetailDelta(device)
}
