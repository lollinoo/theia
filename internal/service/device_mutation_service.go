package service

// This file defines device mutation service service behavior and domain orchestration rules.

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
	if err := m.parent.lifecycleErr(); err != nil {
		return nil, err
	}
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

	if deviceType == domain.DeviceTypeVirtual {
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
	domain.NormalizeVirtualDevice(device)
	if err := m.ensureNoPhysicalVirtualIPConflict(*device, uuid.Nil); err != nil {
		return nil, err
	}

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

	m.parent.startLifecycleProbe(device)

	m.parent.populateEffectiveTopologyDiscoveryMode(device)
	return device, nil
}

// UpdateDevice updates device data through the service orchestration.
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
	domain.NormalizeVirtualDevice(device)
	if err := m.ensureNoPhysicalVirtualIPConflict(*device, device.ID); err != nil {
		return err
	}

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

func (m *deviceMutationService) ensureNoPhysicalVirtualIPConflict(candidate domain.Device, excludeID uuid.UUID) error {
	address := strings.TrimSpace(candidate.IP)
	if address == "" {
		return nil
	}

	conflict, err := m.deviceRepo.FindPhysicalVirtualIPConflict(address, candidate.DeviceType, excludeID)
	if err != nil {
		return fmt.Errorf("checking device IP conflict: %w", err)
	}
	if conflict != nil {
		return fmt.Errorf("device IP conflict: %s is already used by a %s device", address, conflict.DeviceType)
	}
	return nil
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

// DeleteDevice deletes device data through the service orchestration.
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

	if err := m.deviceRepo.Delete(id); err != nil {
		return err
	}
	if resetter := *m.runtimeResetter; resetter != nil {
		resetter.ResetDeviceRuntime(id)
	}
	return nil
}

// GetDevice retrieves device data from the service orchestration.
func (m *deviceMutationService) GetDevice(ctx context.Context, id uuid.UUID) (*domain.Device, error) {
	_ = ctx
	device, err := m.deviceRepo.GetByID(id)
	if err != nil {
		return nil, err
	}
	domain.NormalizeDevicePollingEnabled(device)
	domain.NormalizeVirtualDevice(device)
	m.parent.populateEffectiveTopologyDiscoveryMode(device)
	return device, nil
}

// GetAllDevices retrieves all devices data from the service orchestration.
func (m *deviceMutationService) GetAllDevices(ctx context.Context) ([]domain.Device, error) {
	_ = ctx
	devices, err := m.deviceRepo.GetAll()
	if err != nil {
		return nil, err
	}
	for i := range devices {
		domain.NormalizeDevicePollingEnabled(&devices[i])
		domain.NormalizeVirtualDevice(&devices[i])
		m.parent.populateEffectiveTopologyDiscoveryMode(&devices[i])
	}
	return devices, nil
}

type orphanDeviceRepository interface {
	GetOrphans() ([]domain.Device, error)
}

// GetOrphanDevices retrieves orphan devices data from the service orchestration.
func (m *deviceMutationService) GetOrphanDevices(ctx context.Context) ([]domain.Device, error) {
	_ = ctx
	orphanRepo, ok := m.deviceRepo.(orphanDeviceRepository)
	if !ok {
		return nil, fmt.Errorf("device repository does not support orphan device listing")
	}
	devices, err := orphanRepo.GetOrphans()
	if err != nil {
		return nil, err
	}
	for i := range devices {
		domain.NormalizeDevicePollingEnabled(&devices[i])
		domain.NormalizeVirtualDevice(&devices[i])
		m.parent.populateEffectiveTopologyDiscoveryMode(&devices[i])
	}
	return devices, nil
}

// GetDevicesByIDs retrieves devices by ids data from the service orchestration.
func (m *deviceMutationService) GetDevicesByIDs(ctx context.Context, ids []uuid.UUID) ([]domain.Device, error) {
	_ = ctx
	if len(ids) == 0 {
		return []domain.Device{}, nil
	}

	type deviceBatchRepository interface {
		GetByIDs([]uuid.UUID) ([]domain.Device, error)
	}

	batchRepo, ok := m.deviceRepo.(deviceBatchRepository)
	var devices []domain.Device
	var err error
	if ok {
		devices, err = batchRepo.GetByIDs(ids)
	} else {
		devices, err = m.deviceRepo.GetAll()
		if err == nil {
			requested := make(map[uuid.UUID]struct{}, len(ids))
			for _, id := range ids {
				requested[id] = struct{}{}
			}
			filtered := devices[:0]
			for _, device := range devices {
				if _, include := requested[device.ID]; include {
					filtered = append(filtered, device)
				}
			}
			devices = filtered
		}
	}
	if err != nil {
		return nil, err
	}

	for i := range devices {
		domain.NormalizeDevicePollingEnabled(&devices[i])
		domain.NormalizeVirtualDevice(&devices[i])
		m.parent.populateEffectiveTopologyDiscoveryMode(&devices[i])
	}
	return devices, nil
}

// GetTopologyDevicesByIDs retrieves topology devices by ids data from the service orchestration.
func (m *deviceMutationService) GetTopologyDevicesByIDs(ctx context.Context, ids []uuid.UUID) ([]domain.Device, error) {
	_ = ctx
	if len(ids) == 0 {
		return []domain.Device{}, nil
	}

	type topologyDeviceBatchRepository interface {
		GetByIDsForTopology([]uuid.UUID) ([]domain.Device, error)
	}

	if topologyRepo, ok := m.deviceRepo.(topologyDeviceBatchRepository); ok {
		devices, err := topologyRepo.GetByIDsForTopology(ids)
		if err != nil {
			return nil, err
		}
		m.normalizeTopologyDevices(devices)
		return devices, nil
	}

	devices, err := m.GetDevicesByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	for i := range devices {
		devices[i].SNMPCredentials = domain.SNMPCredentials{}
	}
	return devices, nil
}

func (m *deviceMutationService) normalizeTopologyDevices(devices []domain.Device) {
	for i := range devices {
		domain.NormalizeDevicePollingEnabled(&devices[i])
		domain.NormalizeVirtualDevice(&devices[i])
		m.parent.populateEffectiveTopologyDiscoveryMode(&devices[i])
	}
}
