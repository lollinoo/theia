package service

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/domain"
)

type deviceMutationService struct {
	parent          *DeviceService
	deviceRepo      domain.DeviceRepository
	linkRepo        domain.LinkRepository
	settingsRepo    domain.SettingsRepository
	discoverFunc    DiscoverFunc
	pollRescheduler *pollRescheduler
	runtimeResetter *runtimeResetter
	now             func() time.Time
	probeWg         *sync.WaitGroup
}

func newDeviceMutationService(parent *DeviceService) *deviceMutationService {
	return &deviceMutationService{
		parent:          parent,
		deviceRepo:      parent.deviceRepo,
		linkRepo:        parent.linkRepo,
		settingsRepo:    parent.settingsRepo,
		discoverFunc:    parent.discoverFunc,
		pollRescheduler: &parent.pollRescheduler,
		runtimeResetter: &parent.runtimeResetter,
		now:             parent.now,
		probeWg:         &parent.probeWg,
	}
}

func (m *deviceMutationService) AddDevice(
	ctx context.Context,
	ip, hostname string,
	deviceType domain.DeviceType,
	creds domain.SNMPCredentials,
	tags map[string]string,
	vendor string,
	metricsSource domain.MetricsSource,
	prometheusLabelName string,
	prometheusLabelValue string,
	topologyDiscoveryMode domain.TopologyDiscoveryMode,
	areaIDs []uuid.UUID,
	notes ...*string,
) (*domain.Device, error) {
	_ = ctx
	if tags == nil {
		tags = map[string]string{}
	}
	if deviceType == "" {
		deviceType = domain.DeviceTypeUnknown
	}
	if metricsSource == "" {
		metricsSource = domain.MetricsSourcePrometheus
	}
	if prometheusLabelName == "" {
		prometheusLabelName = "instance"
	}
	if prometheusLabelValue == "" {
		prometheusLabelValue = ip
	}

	if deviceType == domain.DeviceTypeVirtual && ip == "" {
		metricsSource = domain.MetricsSourceNone
	}

	var normalizedNotes *string
	if len(notes) > 0 {
		normalizedNotes = domain.NormalizeDeviceNotes(notes[0])
	}

	initialStatus := domain.DeviceStatusProbing
	if deviceType == domain.DeviceTypeVirtual {
		initialStatus = domain.DeviceStatusUnknown
	}
	pollingEnabled := true
	pollIntervalOverride := initialPollIntervalOverride(m.settingsRepo, deviceType)

	device := &domain.Device{
		ID:                     uuid.New(),
		Hostname:               hostname,
		IP:                     ip,
		Notes:                  normalizedNotes,
		SNMPCredentials:        creds,
		DeviceType:             deviceType,
		PollClass:              domain.ClassifyPollClass(deviceType),
		PollIntervalOverride:   pollIntervalOverride,
		PollingEnabled:         &pollingEnabled,
		Status:                 initialStatus,
		Vendor:                 vendor,
		Managed:                true,
		Tags:                   tags,
		MetricsSource:          metricsSource,
		PrometheusLabelName:    prometheusLabelName,
		PrometheusLabelValue:   prometheusLabelValue,
		TopologyDiscoveryMode:  topologyDiscoveryMode,
		TopologyBootstrapState: domain.TopologyBootstrapStateIdle,
		AreaIDs:                areaIDs,
	}
	device.TopologyDiscoveryMode = domain.NormalizeTopologyDiscoveryMode(device.TopologyDiscoveryMode, domain.TopologyDiscoveryModeInherit)
	if domain.ResolveTopologyDiscoveryMode(device, m.parent.defaultTopologyDiscoveryMode()) == domain.TopologyDiscoveryModeBootstrapOnce {
		device.TopologyBootstrapState = domain.TopologyBootstrapStatePending
	}
	domain.NormalizeVirtualNoIPDevice(device)

	if err := m.deviceRepo.Create(device); err != nil {
		return nil, fmt.Errorf("creating device: %w", err)
	}

	if deviceType == domain.DeviceTypeVirtual {
		m.parent.populateEffectiveTopologyDiscoveryMode(device)
		return device, nil
	}

	if m.parent.bootstrapScheduler != nil &&
		device.TopologyBootstrapState == domain.TopologyBootstrapStatePending &&
		strings.TrimSpace(device.IP) != "" &&
		device.MetricsSource != domain.MetricsSourcePrometheus &&
		device.MetricsSource != domain.MetricsSourceNone &&
		m.parent.bootstrapScheduler.ScheduleBootstrap(*device, m.now().UTC()) {
		m.parent.populateEffectiveTopologyDiscoveryMode(device)
		return device, nil
	}

	m.probeWg.Add(1)
	go func() {
		defer m.probeWg.Done()
		m.parent.probeDevice(device)
	}()

	m.parent.populateEffectiveTopologyDiscoveryMode(device)
	return device, nil
}

func (m *deviceMutationService) UpdateDevice(ctx context.Context, id uuid.UUID, update DeviceUpdate) error {
	device, err := m.deviceRepo.GetByID(id)
	if err != nil {
		return fmt.Errorf("getting device: %w", err)
	}
	previousIP := device.IP
	previousOverride := clonePollIntervalOverride(device.PollIntervalOverride)
	previousPollingEnabled := domain.DevicePollingEnabled(*device)
	defaultTopologyMode := m.parent.defaultTopologyDiscoveryMode()
	previousConfiguredMode := domain.NormalizeTopologyDiscoveryMode(device.TopologyDiscoveryMode, domain.TopologyDiscoveryModeInherit)
	previousEffectiveMode := domain.ResolveTopologyDiscoveryMode(device, defaultTopologyMode)
	previousBootstrapState := domain.NormalizeTopologyBootstrapState(device.TopologyBootstrapState)
	shouldTriggerTopologyProbe := false

	if update.Hostname != nil {
		device.Hostname = *update.Hostname
	}
	if update.IP != nil {
		device.IP = *update.IP
	}
	if update.Notes != nil {
		device.Notes = domain.NormalizeDeviceNotes(*update.Notes)
	}
	if update.Tags != nil {
		device.Tags = *update.Tags
	}
	if update.SNMPCredentials != nil {
		device.SNMPCredentials = *update.SNMPCredentials
	}
	if update.Vendor != nil {
		device.Vendor = *update.Vendor
	}
	if update.MetricsSource != nil {
		device.MetricsSource = *update.MetricsSource
	}
	if update.PrometheusLabelName != nil {
		device.PrometheusLabelName = *update.PrometheusLabelName
	}
	if update.PrometheusLabelValue != nil {
		device.PrometheusLabelValue = *update.PrometheusLabelValue
	}
	if update.TopologyDiscoveryMode != nil {
		device.TopologyDiscoveryMode = *update.TopologyDiscoveryMode
		if device.TopologyDiscoveryMode == "" {
			device.TopologyDiscoveryMode = domain.TopologyDiscoveryModeInherit
		}
		if device.TopologyDiscoveryMode == domain.TopologyDiscoveryModeBootstrapOnce {
			device.TopologyBootstrapState = domain.TopologyBootstrapStatePending
		}

		newConfiguredMode := domain.NormalizeTopologyDiscoveryMode(device.TopologyDiscoveryMode, domain.TopologyDiscoveryModeInherit)
		newEffectiveMode := domain.ResolveTopologyDiscoveryMode(device, defaultTopologyMode)
		newBootstrapState := domain.NormalizeTopologyBootstrapState(device.TopologyBootstrapState)
		discoveryModeChanged :=
			newConfiguredMode != previousConfiguredMode ||
				newEffectiveMode != previousEffectiveMode ||
				newBootstrapState != previousBootstrapState
		if discoveryModeChanged &&
			newEffectiveMode != domain.TopologyDiscoveryModeOff &&
			strings.TrimSpace(device.IP) != "" &&
			device.DeviceType != domain.DeviceTypeVirtual &&
			device.MetricsSource != domain.MetricsSourcePrometheus &&
			device.MetricsSource != domain.MetricsSourceNone {
			shouldTriggerTopologyProbe = true
		}
	}
	if update.PollIntervalOverride != nil {
		device.PollIntervalOverride = *update.PollIntervalOverride
	}
	if update.PollingEnabled != nil {
		device.PollingEnabled = update.PollingEnabled
	}
	if update.AreaIDs != nil {
		device.AreaIDs = *update.AreaIDs
	}
	domain.NormalizeDevicePollingEnabled(device)
	domain.NormalizeVirtualNoIPDevice(device)

	if err := m.deviceRepo.Update(device); err != nil {
		return err
	}

	changedAt := m.now().UTC()
	ipChanged := update.IP != nil && previousIP != device.IP
	if ipChanged {
		if resetter := *m.runtimeResetter; resetter != nil {
			resetter.ResetDeviceRuntime(device.ID)
		}
	}

	if rescheduler := *m.pollRescheduler; rescheduler != nil {
		if update.PollingEnabled != nil && previousPollingEnabled != domain.DevicePollingEnabled(*device) {
			rescheduler.ReconcileDeviceTasks(*device, changedAt)
		}
		pollIntervalChanged := update.PollIntervalOverride != nil && !pollIntervalOverridesEqual(previousOverride, device.PollIntervalOverride)
		if (ipChanged || pollIntervalChanged) && domain.DevicePollingEnabled(*device) {
			rescheduler.ReduePerformanceTask(*device, changedAt)
		}
	}

	if !shouldTriggerTopologyProbe {
		return nil
	}
	return m.parent.ReprobeDevice(ctx, id)
}

func initialPollIntervalOverride(settingsRepo domain.SettingsRepository, deviceType domain.DeviceType) *int {
	if deviceType == domain.DeviceTypeVirtual {
		return nil
	}
	seconds := configuredPollingIntervalSeconds(settingsRepo)
	return &seconds
}

func configuredPollingIntervalSeconds(settingsRepo domain.SettingsRepository) int {
	const fallback = 60
	if settingsRepo == nil {
		return fallback
	}
	value, err := settingsRepo.Get(domain.SettingPollingInterval)
	if err != nil {
		return fallback
	}
	seconds, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || seconds <= 0 {
		return fallback
	}
	return seconds
}

func (m *deviceMutationService) DeleteDevice(ctx context.Context, id uuid.UUID) error {
	_ = ctx
	links, err := m.linkRepo.GetByDeviceID(id)
	if err != nil {
		return fmt.Errorf("getting links for device: %w", err)
	}
	for _, link := range links {
		if err := m.linkRepo.Delete(link.ID); err != nil {
			log.Printf("Warning: failed to delete link %s: %v", link.ID, err)
		}
	}

	return m.deviceRepo.Delete(id)
}

func (m *deviceMutationService) GetDevice(ctx context.Context, id uuid.UUID) (*domain.Device, error) {
	_ = ctx
	device, err := m.deviceRepo.GetByID(id)
	if err != nil {
		return nil, err
	}
	domain.NormalizeDevicePollingEnabled(device)
	domain.NormalizeVirtualNoIPDevice(device)
	m.parent.populateEffectiveTopologyDiscoveryMode(device)
	return device, nil
}

func (m *deviceMutationService) GetAllDevices(ctx context.Context) ([]domain.Device, error) {
	_ = ctx
	devices, err := m.deviceRepo.GetAll()
	if err != nil {
		return nil, err
	}
	for i := range devices {
		domain.NormalizeDevicePollingEnabled(&devices[i])
		domain.NormalizeVirtualNoIPDevice(&devices[i])
		m.parent.populateEffectiveTopologyDiscoveryMode(&devices[i])
	}
	return devices, nil
}
