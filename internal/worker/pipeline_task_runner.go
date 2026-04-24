package worker

import (
	"context"
	"log"
	"strconv"
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
				FinishedAt: finishedAt,
			})
		}
	}()

	if task.Kind == polling.TaskKindEssential {
		if p.essential == nil || p.stateStore == nil {
			return
		}

		profile := r.timeoutProfile(polling.LaneEssential)
		result := p.essential.Poll(ctx, task.Device, profile.Timeout, profile.Retries)
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

	if task.Device.DeviceType == domain.DeviceTypeVirtual {
		finishedAt = r.runVirtualTask(ctx, task)
		return
	}

	switch task.VolatilityClass {
	case domain.VolatilityClassPerformance:
		if p.performance == nil || p.stateStore == nil {
			return
		}

		result := p.performance.Poll(ctx, task.Device, r.snmpTimeout(), r.snmpRetries())
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

		result := p.operational.Poll(ctx, task.Device, r.snmpTimeout(), r.snmpRetries())
		finishedAt = completionTime(result.CollectedAt)
		observability.Default().IncPollResult(task.VolatilityClass, result.Err == nil)
		p.stateStore.Update(result.ToStoreUpdate(task.ExpectedInterval))
		r.publishSubscribedDetailDelta(task.Device)

	case domain.VolatilityClassStatic:
		if p.staticCollector == nil || p.stateStore == nil {
			return
		}

		result := p.staticCollector.Poll(ctx, task.Device, r.snmpTimeout(), r.snmpRetries(), r.topologyDiscoveryMode(task.Device))
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
				p.stateStore.Update(result.ToStoreUpdate(task.ExpectedInterval))
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

	if err := service.ProbeVirtualReachability(ctx, task.Device.IP, r.snmpTimeout()); err != nil {
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
	r.publishSubscribedDetailDelta(task.Device)
	return completedAt
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

	seconds, err := strconv.Atoi(value)
	if err != nil || seconds <= 0 {
		return 10 * time.Second
	}

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

	retries, err := strconv.Atoi(value)
	if err != nil || retries < 0 {
		return 2
	}

	return retries
}

func (r *pipelineTaskRunner) timeoutProfile(lane polling.Lane) polling.TimeoutProfile {
	policy, _ := polling.PolicyFromSettings(r.pipeline.settingsRepo, 0, 300*time.Millisecond, 0)
	if profile, ok := policy.Timeouts[lane]; ok {
		return profile
	}
	return polling.TimeoutProfile{Timeout: r.snmpTimeout(), Retries: r.snmpRetries()}
}

func (r *pipelineTaskRunner) topologyDiscoveryMode(device domain.Device) domain.TopologyDiscoveryMode {
	p := r.pipeline
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

func (p *PipelineOrchestrator) runTask(ctx context.Context, task scheduler.PollTask) {
	p.taskRunner.runTask(ctx, task)
}

func (p *PipelineOrchestrator) publishSubscribedDetailDelta(device domain.Device) {
	p.taskRunner.publishSubscribedDetailDelta(device)
}
