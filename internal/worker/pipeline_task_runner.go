package worker

// This file defines pipeline task runner worker behavior, background lifecycle, and runtime state updates.

import (
	"context"
	"log"
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
			PollSuccess:      result.Err == nil,
			ExpectedInterval: task.ExpectedInterval,
			Timestamp:        finishedAt,
		})
		r.publishSubscribedDetailDelta(task.Device)
		if result.Err != nil || p.topologyService == nil {
			return
		}

		r.persistStaticDiscovery(task.Device, result)
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

		profile := r.timeoutProfile(polling.LaneBackground)
		result := p.performance.Poll(ctx, task.Device, profile.Timeout, profile.Retries)
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

		p.stateStore.Update(update)
		r.publishSubscribedDetailDelta(task.Device)

	case domain.VolatilityClassOperational:
		if p.operational == nil || p.stateStore == nil {
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

func (r *pipelineTaskRunner) persistStaticDiscovery(device domain.Device, result collector.StaticResult) {
	p := r.pipeline
	persisted, err := p.topologyService.ApplyStaticDiscovery(device.ID, service.StaticDiscoveryInput{
		SysName:                    result.SysName,
		SysDescr:                   result.SysDescr,
		SysObjectID:                result.SysObjectID,
		HardwareModel:              result.HardwareModel,
		OSVersion:                  result.OSVersion,
		Vendor:                     result.Vendor,
		DeviceType:                 result.DeviceType,
		Interfaces:                 append([]domain.Interface(nil), result.Interfaces...),
		Neighbors:                  append([]snmp.NeighborInfo(nil), result.Neighbors...),
		NeighborDiscoveryProtocols: append([]domain.DiscoveryProtocol(nil), result.NeighborDiscoveryProtocols...),
		NeighborDiscoveryFailures:  append([]snmp.NeighborDiscoveryFailure(nil), result.NeighborDiscoveryFailures...),
	})
	if err != nil {
		log.Printf("pipeline: static persistence failed for %s: %v", device.ID, err)
		return
	}
	if persisted.TopologyChanged && p.topologyNotify != nil {
		select {
		case p.topologyNotify <- struct{}{}:
		default:
		}
	}
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
	return r.resolvedTopologyDiscoveryMode(device)
}

func (r *pipelineTaskRunner) resolvedTopologyDiscoveryMode(device domain.Device) domain.TopologyDiscoveryMode {
	p := r.pipeline
	defaultMode := domain.TopologyDiscoveryModeLLDPCDP
	if p.settingsRepo != nil {
		if value, err := p.settingsRepo.Get(domain.SettingTopologyDiscoveryDefaultMode); err == nil {
			defaultMode = domain.NormalizeTopologyDiscoveryMode(domain.TopologyDiscoveryMode(value), domain.TopologyDiscoveryModeLLDPCDP)
		}
	}

	mode := domain.NormalizeTopologyDiscoveryMode(device.TopologyDiscoveryMode, domain.TopologyDiscoveryModeInherit)
	if mode == domain.TopologyDiscoveryModeInherit {
		mode = defaultMode
	}
	return mode
}

func (p *PipelineOrchestrator) runTask(ctx context.Context, task scheduler.PollTask) {
	p.taskRunner.runTask(ctx, task)
}

func (p *PipelineOrchestrator) publishSubscribedDetailDelta(device domain.Device) {
	p.taskRunner.publishSubscribedDetailDelta(device)
}
