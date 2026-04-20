package service

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/domain"
	"github.com/lollinoo/theia/internal/pollingbudget"
	"github.com/lollinoo/theia/internal/snmp"
	"github.com/lollinoo/theia/internal/topology"
)

// DiscoverFunc performs SNMP discovery on a target device and returns the result.
// This abstraction allows injecting mocks for testing.
// The vendor registry is used for device detection, model extraction, and vendor identification.
type DiscoverFunc func(target string, creds domain.SNMPCredentials, topologyMode domain.TopologyDiscoveryMode) (*snmp.DiscoveryResult, error)

// SNMPPollFunc polls a single device via SNMP for live metrics.
// vendorName is used to resolve vendor-specific SNMP OIDs.
type SNMPPollFunc func(target string, creds domain.SNMPCredentials, vendorName string) (domain.DeviceMetrics, error)

type pollRescheduler interface {
	ReduePerformanceTask(device domain.Device, changedAt time.Time)
}

// DeviceUpdate holds optional fields for partial device updates.
type DeviceUpdate struct {
	Hostname              *string
	IP                    *string
	Notes                 **string
	Tags                  *map[string]string
	SNMPCredentials       *domain.SNMPCredentials
	Vendor                *string
	MetricsSource         *domain.MetricsSource
	PrometheusLabelName   *string
	PrometheusLabelValue  *string
	TopologyDiscoveryMode *domain.TopologyDiscoveryMode
	PollIntervalOverride  **int        // nil=not set, *nil=clear, **value=set
	AreaIDs               *[]uuid.UUID // nil=not set, non-nil=replace all area assignments
}

// DeviceService orchestrates device management, combining SNMP discovery
// with persistence through repositories.
type DeviceService struct {
	deviceRepo      domain.DeviceRepository
	linkRepo        domain.LinkRepository
	topologyStore   topology.ObservationStore
	settingsRepo    domain.SettingsRepository
	mutation        *deviceMutationService
	discoverFunc    DiscoverFunc
	pollRescheduler pollRescheduler
	now             func() time.Time
	scheduleFunc    func(time.Duration, func())
	delayedReprobe  func(context.Context, uuid.UUID) error
	reprobeDelay    time.Duration
	reprobeCooldown time.Duration
	reprobeWindow   time.Duration
	reprobeMu       sync.Mutex
	reprobeBooked   map[uuid.UUID]time.Time
	reprobeInFlight atomic.Int32

	probeWg        sync.WaitGroup
	TopologyNotify chan struct{} // signaled when probeDevice creates new links
}

type DeviceServiceOption func(*DeviceService)

const (
	incompleteLinkReprobeDelay    = 20 * time.Second
	incompleteLinkReprobeRetry    = 5 * time.Second
	incompleteLinkReprobeCooldown = 45 * time.Second
	incompleteLinkReprobeWindow   = 5 * time.Minute
)

// NewDeviceService creates a new DeviceService with the given dependencies.
// topologyNotify is an optional buffered channel signaled when probeDevice creates
// new LLDP/CDP links so the MetricsCollector can broadcast a topology_changed event.
func NewDeviceService(
	deviceRepo domain.DeviceRepository,
	linkRepo domain.LinkRepository,
	settingsRepo domain.SettingsRepository,
	discoverFunc DiscoverFunc,
	topologyNotify chan struct{},
	options ...DeviceServiceOption,
) *DeviceService {
	svc := &DeviceService{
		deviceRepo:      deviceRepo,
		linkRepo:        linkRepo,
		settingsRepo:    settingsRepo,
		discoverFunc:    discoverFunc,
		now:             time.Now,
		scheduleFunc:    func(delay time.Duration, fn func()) { time.AfterFunc(delay, fn) },
		reprobeDelay:    incompleteLinkReprobeDelay,
		reprobeCooldown: incompleteLinkReprobeCooldown,
		reprobeWindow:   incompleteLinkReprobeWindow,
		reprobeBooked:   make(map[uuid.UUID]time.Time),
		TopologyNotify:  topologyNotify,
	}
	svc.delayedReprobe = svc.runDelayedReprobe
	for _, option := range options {
		if option != nil {
			option(svc)
		}
	}
	svc.mutation = newDeviceMutationService(svc)
	return svc
}

func WithTopologyObservationStore(store topology.ObservationStore) DeviceServiceOption {
	return func(s *DeviceService) {
		s.topologyStore = store
	}
}

func (s *DeviceService) defaultTopologyDiscoveryMode() domain.TopologyDiscoveryMode {
	if s.settingsRepo == nil {
		return domain.TopologyDiscoveryModeLLDPCDP
	}
	value, err := s.settingsRepo.Get(domain.SettingTopologyDiscoveryDefaultMode)
	if err != nil {
		return domain.TopologyDiscoveryModeLLDPCDP
	}
	return domain.NormalizeTopologyDiscoveryMode(domain.TopologyDiscoveryMode(value), domain.TopologyDiscoveryModeLLDPCDP)
}

func (s *DeviceService) populateEffectiveTopologyDiscoveryMode(device *domain.Device) {
	if device == nil {
		return
	}
	device.TopologyDiscoveryMode = domain.NormalizeTopologyDiscoveryMode(device.TopologyDiscoveryMode, domain.TopologyDiscoveryModeInherit)
	device.TopologyBootstrapState = domain.NormalizeTopologyBootstrapState(device.TopologyBootstrapState)
	device.EffectiveTopologyDiscoveryMode = domain.ResolveTopologyDiscoveryMode(device, s.defaultTopologyDiscoveryMode())
}

func (s *DeviceService) SetPollRescheduler(rescheduler pollRescheduler) {
	s.pollRescheduler = rescheduler
}

// AddDevice creates a new device and triggers an async SNMP probe for
// non-virtual devices. Virtual devices skip probing and start with status
// "unknown". The device is returned immediately before any probe completes.
func (s *DeviceService) AddDevice(
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
	return s.mutation.AddDevice(ctx, ip, hostname, deviceType, creds, tags, vendor, metricsSource, prometheusLabelName, prometheusLabelValue, topologyDiscoveryMode, areaIDs, notes...)
}

// probeDevice performs SNMP discovery and updates the device in the repository.
// It re-fetches the device from the repo to avoid racing on the pointer
// that was returned to the caller of AddDevice.
func (s *DeviceService) updateDeviceStatus(deviceID uuid.UUID, status domain.DeviceStatus) error {
	fresh, err := s.deviceRepo.GetByID(deviceID)
	if err != nil {
		return err
	}
	fresh.Status = status
	return s.deviceRepo.Update(fresh)
}

func (s *DeviceService) markDeviceStatus(deviceID uuid.UUID, deviceIP string, status domain.DeviceStatus) {
	if err := s.updateDeviceStatus(deviceID, status); err != nil {
		log.Printf("Failed to update device %s status to %s: %v", deviceIP, string(status), err)
	}
}

func (s *DeviceService) updateTopologyDiscoveryState(deviceID uuid.UUID, mutate func(*domain.Device)) {
	device, err := s.deviceRepo.GetByID(deviceID)
	if err != nil {
		log.Printf("Failed to load device %s for topology discovery state update: %v", deviceID, err)
		return
	}
	if mutate != nil {
		mutate(device)
	}
	device.TopologyDiscoveryMode = domain.NormalizeTopologyDiscoveryMode(device.TopologyDiscoveryMode, domain.TopologyDiscoveryModeInherit)
	device.TopologyBootstrapState = domain.NormalizeTopologyBootstrapState(device.TopologyBootstrapState)
	if err := s.deviceRepo.Update(device); err != nil {
		log.Printf("Failed to update topology discovery state for %s: %v", deviceID, err)
	}
}

func topologyDiscoveryResultLabel(neighborCount int, followupScheduled bool) string {
	switch {
	case followupScheduled:
		return "ports_pending"
	case neighborCount > 0:
		return "neighbors_found"
	default:
		return "no_neighbors"
	}
}

func (s *DeviceService) canTemporarilyBootstrapPeer(device *domain.Device) bool {
	if device == nil {
		return false
	}

	configuredMode := domain.NormalizeTopologyDiscoveryMode(device.TopologyDiscoveryMode, domain.TopologyDiscoveryModeInherit)
	if configuredMode == domain.TopologyDiscoveryModeOff {
		return false
	}

	effectiveMode := domain.ResolveTopologyDiscoveryMode(device, s.defaultTopologyDiscoveryMode())
	return effectiveMode == domain.TopologyDiscoveryModeOff
}

func (s *DeviceService) reopenBootstrapWindow(deviceID uuid.UUID) bool {
	updated := false
	s.updateTopologyDiscoveryState(deviceID, func(device *domain.Device) {
		if !s.canTemporarilyBootstrapPeer(device) {
			return
		}
		device.TopologyBootstrapState = domain.TopologyBootstrapStatePending
		updated = true
	})
	return updated
}

func (s *DeviceService) hasIncompleteLLDPLinks(deviceID uuid.UUID) bool {
	links, err := s.linkRepo.GetByDeviceID(deviceID)
	if err != nil {
		log.Printf("Failed to inspect links for delayed LLDP re-probe on %s: %v", deviceID, err)
		return false
	}

	for _, link := range links {
		if link.DiscoveryProtocol != domain.DiscoveryProtocolLLDP {
			continue
		}
		if strings.TrimSpace(link.SourceIfName) == "" || strings.TrimSpace(link.TargetIfName) == "" {
			return true
		}
	}

	return false
}

func (s *DeviceService) lldpPeerIDs(deviceID uuid.UUID) []uuid.UUID {
	links, err := s.linkRepo.GetByDeviceID(deviceID)
	if err != nil {
		log.Printf("Failed to inspect LLDP peers for topology bootstrap reconciliation on %s: %v", deviceID, err)
		return nil
	}

	seen := make(map[uuid.UUID]struct{})
	peerIDs := make([]uuid.UUID, 0, len(links))
	for _, link := range links {
		if link.DiscoveryProtocol != domain.DiscoveryProtocolLLDP {
			continue
		}

		peerID := link.SourceDeviceID
		if peerID == deviceID {
			peerID = link.TargetDeviceID
		}
		if peerID == uuid.Nil || peerID == deviceID {
			continue
		}
		if _, exists := seen[peerID]; exists {
			continue
		}
		seen[peerID] = struct{}{}
		peerIDs = append(peerIDs, peerID)
	}

	return peerIDs
}

func (s *DeviceService) incompleteLLDPPeerIDs(deviceID uuid.UUID) []uuid.UUID {
	links, err := s.linkRepo.GetByDeviceID(deviceID)
	if err != nil {
		log.Printf("Failed to inspect LLDP peers for delayed re-probe on %s: %v", deviceID, err)
		return nil
	}

	seen := make(map[uuid.UUID]struct{})
	incomplete := make([]uuid.UUID, 0, len(links))
	for _, link := range links {
		if link.DiscoveryProtocol != domain.DiscoveryProtocolLLDP {
			continue
		}
		if strings.TrimSpace(link.SourceIfName) != "" && strings.TrimSpace(link.TargetIfName) != "" {
			continue
		}

		peerID := link.SourceDeviceID
		if peerID == deviceID {
			peerID = link.TargetDeviceID
		}
		if peerID == uuid.Nil || peerID == deviceID {
			continue
		}
		if _, exists := seen[peerID]; exists {
			continue
		}
		seen[peerID] = struct{}{}
		incomplete = append(incomplete, peerID)
	}

	return incomplete
}

func (s *DeviceService) reconcileResolvedBootstrapPeers(deviceID uuid.UUID) {
	for _, peerID := range s.lldpPeerIDs(deviceID) {
		if s.hasIncompleteLLDPLinks(peerID) {
			continue
		}

		s.updateTopologyDiscoveryState(peerID, func(device *domain.Device) {
			switch domain.NormalizeTopologyBootstrapState(device.TopologyBootstrapState) {
			case domain.TopologyBootstrapStatePending, domain.TopologyBootstrapStateFollowupScheduled:
				device.TopologyBootstrapState = domain.TopologyBootstrapStateCompleted
				if device.LastTopologyDiscoveryResult == "ports_pending" {
					device.LastTopologyDiscoveryResult = "neighbors_found"
				}
			}
		})
	}
}

func (s *DeviceService) shouldScheduleIncompleteLinkReprobe(deviceID uuid.UUID) bool {
	device, err := s.deviceRepo.GetByID(deviceID)
	if err != nil {
		log.Printf("Failed to inspect device %s for delayed LLDP re-probe: %v", deviceID, err)
		return false
	}
	if device.CreatedAt.IsZero() {
		return false
	}
	mode := domain.ResolveTopologyDiscoveryMode(device, s.defaultTopologyDiscoveryMode())
	if mode == domain.TopologyDiscoveryModeOff {
		return false
	}
	bootstrapPending := mode == domain.TopologyDiscoveryModeBootstrapOnce &&
		device.TopologyBootstrapState == domain.TopologyBootstrapStatePending
	if !bootstrapPending {
		age := s.now().Sub(device.CreatedAt)
		if age < 0 || age > s.reprobeWindow {
			return false
		}
	}
	if device.Status != domain.DeviceStatusProbing && !bootstrapPending {
		return false
	}
	if mode == domain.TopologyDiscoveryModeBootstrapOnce &&
		device.TopologyBootstrapState != domain.TopologyBootstrapStatePending {
		return false
	}

	return s.hasIncompleteLLDPLinks(deviceID)
}

func (s *DeviceService) reserveIncompleteLinkReprobe(deviceID uuid.UUID) (time.Time, bool) {
	s.reprobeMu.Lock()
	defer s.reprobeMu.Unlock()

	last, ok := s.reprobeBooked[deviceID]
	if ok && s.now().Sub(last) < s.reprobeCooldown {
		return last, false
	}
	bookedAt := s.now()
	s.reprobeBooked[deviceID] = bookedAt
	return bookedAt, true
}

func (s *DeviceService) syncTopologyDiscoveryMetadata(deviceID uuid.UUID, neighborCount int, followupScheduled bool) {
	recordedAt := s.now().UTC()
	hasIncomplete := s.hasIncompleteLLDPLinks(deviceID)

	s.updateTopologyDiscoveryState(deviceID, func(fresh *domain.Device) {
		mode := domain.ResolveTopologyDiscoveryMode(fresh, s.defaultTopologyDiscoveryMode())
		if mode == domain.TopologyDiscoveryModeOff {
			return
		}

		fresh.LastTopologyDiscoveryAt = &recordedAt
		fresh.LastTopologyDiscoveryResult = topologyDiscoveryResultLabel(
			neighborCount,
			followupScheduled || hasIncomplete,
		)

		if mode != domain.TopologyDiscoveryModeBootstrapOnce {
			return
		}
		if followupScheduled {
			fresh.TopologyBootstrapState = domain.TopologyBootstrapStateFollowupScheduled
			return
		}
		if hasIncomplete {
			if fresh.TopologyBootstrapState == domain.TopologyBootstrapStateIdle {
				fresh.TopologyBootstrapState = domain.TopologyBootstrapStatePending
			}
			return
		}
		fresh.TopologyBootstrapState = domain.TopologyBootstrapStateCompleted
	})
}

func (s *DeviceService) finalizeBootstrapWindowIfExhausted(deviceID uuid.UUID, followupScheduled bool) {
	if followupScheduled || !s.hasIncompleteLLDPLinks(deviceID) {
		return
	}

	s.updateTopologyDiscoveryState(deviceID, func(device *domain.Device) {
		mode := domain.ResolveTopologyDiscoveryMode(device, s.defaultTopologyDiscoveryMode())
		if mode != domain.TopologyDiscoveryModeBootstrapOnce {
			return
		}

		switch device.TopologyBootstrapState {
		case domain.TopologyBootstrapStatePending, domain.TopologyBootstrapStateFollowupScheduled:
			device.TopologyBootstrapState = domain.TopologyBootstrapStateCompleted
		}
	})
}

func (s *DeviceService) runDelayedReprobe(_ context.Context, id uuid.UUID) error {
	device, err := s.deviceRepo.GetByID(id)
	if err != nil {
		return fmt.Errorf("getting device: %w", err)
	}

	s.probeDevice(device)
	return nil
}

func (s *DeviceService) scheduleIncompleteLinkReprobeAttempt(
	targetID uuid.UUID,
	targetLabel string,
	bookedAt time.Time,
	delay time.Duration,
) {
	s.scheduleFunc(delay, func() {
		if !s.hasIncompleteLLDPLinks(targetID) {
			return
		}

		limit := s.staticReprobeBudget()
		if limit <= 0 {
			return
		}
		if int(s.reprobeInFlight.Add(1)) > limit {
			s.reprobeInFlight.Add(-1)
			if s.now().Sub(bookedAt) < s.reprobeCooldown {
				log.Printf(
					"Retrying delayed LLDP re-probe for %s in %s: static reprobe budget exhausted",
					targetLabel,
					incompleteLinkReprobeRetry,
				)
				s.scheduleIncompleteLinkReprobeAttempt(
					targetID,
					targetLabel,
					bookedAt,
					incompleteLinkReprobeRetry,
				)
				return
			}
			log.Printf("Skipping delayed LLDP re-probe for %s: static reprobe budget exhausted", targetLabel)
			return
		}
		defer s.reprobeInFlight.Add(-1)

		if err := s.delayedReprobe(context.Background(), targetID); err != nil {
			log.Printf("Delayed LLDP re-probe failed for %s: %v", targetLabel, err)
		}
	})
}

func (s *DeviceService) scheduleIncompleteLinkReprobe(deviceID uuid.UUID, deviceIP string) bool {
	type reprobeTarget struct {
		id    uuid.UUID
		label string
	}

	targets := []reprobeTarget{{id: deviceID, label: deviceIP}}
	for _, peerID := range s.incompleteLLDPPeerIDs(deviceID) {
		peer, err := s.deviceRepo.GetByID(peerID)
		if err != nil {
			log.Printf("Failed to inspect LLDP peer %s for delayed re-probe: %v", peerID, err)
			continue
		}
		if !peer.Managed || peer.IP == "" || peer.DeviceType == domain.DeviceTypeVirtual {
			continue
		}
		if peer.MetricsSource == domain.MetricsSourcePrometheus || peer.MetricsSource == domain.MetricsSourceNone {
			continue
		}
		if domain.ResolveTopologyDiscoveryMode(peer, s.defaultTopologyDiscoveryMode()) == domain.TopologyDiscoveryModeOff {
			if !s.reopenBootstrapWindow(peerID) {
				continue
			}
		}
		label := peer.IP
		if label == "" {
			label = peer.Hostname
		}
		targets = append(targets, reprobeTarget{id: peerID, label: label})
	}

	scheduled := false
	for _, target := range targets {
		bookedAt, reserved := s.reserveIncompleteLinkReprobe(target.id)
		if !reserved {
			continue
		}

		scheduled = true
		targetLabel := target.label
		log.Printf("Scheduling delayed LLDP re-probe for %s in %s to resolve incomplete ports", targetLabel, s.reprobeDelay)
		s.scheduleIncompleteLinkReprobeAttempt(target.id, targetLabel, bookedAt, s.reprobeDelay)
	}

	return scheduled
}

func (s *DeviceService) staticReprobeBudget() int {
	budgets := pollingbudget.Resolve(s.settingsRepo)
	return budgets[domain.VolatilityClassStatic]
}

func (s *DeviceService) probeDevice(device *domain.Device) {
	deviceID := device.ID
	deviceIP := device.IP

	// Virtual devices are never SNMP-probed.
	if device.DeviceType == domain.DeviceTypeVirtual {
		if device.IP != "" {
			s.markDeviceStatus(deviceID, deviceIP, domain.DeviceStatusUp)
			log.Printf("Virtual device %s has IP; marked up (probe_success will refine)", deviceIP)
		} else {
			s.markDeviceStatus(deviceID, "(virtual-no-ip)", domain.DeviceStatusUnknown)
			log.Printf("Virtual device %s has no IP; status remains unknown", deviceID)
		}
		return
	}

	// Prometheus-only devices never touch gosnmp — mark up and return.
	if device.MetricsSource == domain.MetricsSourcePrometheus {
		fresh, err := s.deviceRepo.GetByID(deviceID)
		if err != nil {
			log.Printf("Failed to re-fetch device %s for prometheus probe: %v", deviceIP, err)
			return
		}
		fresh.Status = domain.DeviceStatusUp
		if fresh.PollIntervalOverride == nil {
			fresh.PollClass = domain.ClassifyPollClass(fresh.DeviceType)
		}
		if err := s.deviceRepo.Update(fresh); err != nil {
			log.Printf("Failed to update device %s status to up: %v", deviceIP, err)
			return
		}
		log.Printf("Skipped SNMP probe for %s (metrics_source=prometheus); marked up", deviceIP)
		return
	}

	topologyMode := domain.ResolveTopologyDiscoveryMode(device, s.defaultTopologyDiscoveryMode())

	result, err := s.discoverFunc(deviceIP, device.SNMPCredentials, topologyMode)
	if err != nil {
		log.Printf("SNMP discovery failed for %s: %v", deviceIP, err)
		s.markDeviceStatus(deviceID, deviceIP, domain.DeviceStatusDown)
		return
	}

	persisted, err := s.ApplyStaticDiscovery(deviceID, StaticDiscoveryInput{
		SysName:       result.SysName,
		SysDescr:      result.SysDescr,
		SysObjectID:   result.SysObjectID,
		HardwareModel: result.HardwareModel,
		OSVersion:     result.OSVersion,
		Vendor:        result.Vendor,
		DeviceType:    result.DeviceType,
		Interfaces:    result.Interfaces,
		Neighbors:     result.Neighbors,
	})
	if err != nil {
		if statusErr := s.updateDeviceStatus(deviceID, domain.DeviceStatusUp); statusErr != nil {
			log.Printf("Failed to update device %s status to up after discovery persistence failure: %v", deviceIP, statusErr)
		}
		log.Printf("Failed to persist static discovery for %s: %v", deviceIP, err)
		return
	}
	followupScheduled := false
	if s.shouldScheduleIncompleteLinkReprobe(deviceID) {
		followupScheduled = s.scheduleIncompleteLinkReprobe(deviceID, deviceIP)
	}
	s.syncTopologyDiscoveryMetadata(deviceID, len(result.Neighbors), followupScheduled)
	s.finalizeBootstrapWindowIfExhausted(deviceID, followupScheduled)

	if err := s.updateDeviceStatus(deviceID, domain.DeviceStatusUp); err != nil {
		log.Printf("Failed to update device %s status to up: %v", deviceIP, err)
		return
	}

	// Signal topology change when persisted topology changed.
	// Non-blocking send: if the channel is full the next collection cycle will
	// pick up the topology change, so dropping the signal is safe (T-33-02).
	if persisted.TopologyChanged && s.TopologyNotify != nil {
		select {
		case s.TopologyNotify <- struct{}{}:
		default:
			// Channel full — skip; MetricsCollector will pick up change next cycle
		}
	}
}

// UpdateDevice applies partial updates to an existing device without re-probing.
func (s *DeviceService) UpdateDevice(ctx context.Context, id uuid.UUID, update DeviceUpdate) error {
	return s.mutation.UpdateDevice(ctx, id, update)
}

// DeleteDevice removes a device and all associated links.
func (s *DeviceService) DeleteDevice(ctx context.Context, id uuid.UUID) error {
	return s.mutation.DeleteDevice(ctx, id)
}

// GetDevice retrieves a single device by ID.
func (s *DeviceService) GetDevice(ctx context.Context, id uuid.UUID) (*domain.Device, error) {
	return s.mutation.GetDevice(ctx, id)
}

// GetAllDevices retrieves all devices with their interfaces.
func (s *DeviceService) GetAllDevices(ctx context.Context) ([]domain.Device, error) {
	return s.mutation.GetAllDevices(ctx)
}

// ProbeDevice triggers a re-probe of an existing device.
func (s *DeviceService) ProbeDevice(ctx context.Context, id uuid.UUID) error {
	device, err := s.deviceRepo.GetByID(id)
	if err != nil {
		return fmt.Errorf("getting device: %w", err)
	}

	device.Status = domain.DeviceStatusProbing
	if err := s.deviceRepo.Update(device); err != nil {
		return fmt.Errorf("updating device status: %w", err)
	}

	s.probeWg.Add(1)
	go func() {
		defer s.probeWg.Done()
		s.probeDevice(device)
	}()

	return nil
}

// ReprobeDevice triggers a re-probe of an existing device without
// transitioning through the "probing" status. This avoids status flapping
// when the Poller re-probes all devices every cycle -- the current status
// (up/down) is kept until the probe result is known.
func (s *DeviceService) ReprobeDevice(ctx context.Context, id uuid.UUID) error {
	device, err := s.deviceRepo.GetByID(id)
	if err != nil {
		return fmt.Errorf("getting device: %w", err)
	}

	// Do NOT set device.Status = "probing" or update the repo here.
	// The existing status stays visible until probeDevice() completes
	// and writes the final up/down result.

	s.probeWg.Add(1)
	go func() {
		defer s.probeWg.Done()
		s.probeDevice(device)
	}()

	return nil
}

func (s *DeviceService) RunTopologyDiscoveryNow(ctx context.Context, id uuid.UUID) error {
	device, err := s.deviceRepo.GetByID(id)
	if err != nil {
		return fmt.Errorf("getting device: %w", err)
	}
	if device.DeviceType == domain.DeviceTypeVirtual || strings.TrimSpace(device.IP) == "" {
		return fmt.Errorf("topology discovery requires a non-virtual device with an IP")
	}
	if device.MetricsSource == domain.MetricsSourcePrometheus {
		return fmt.Errorf("topology discovery requires SNMP-capable metrics source")
	}

	device.TopologyBootstrapState = domain.TopologyBootstrapStatePending
	if err := s.deviceRepo.Update(device); err != nil {
		return fmt.Errorf("updating topology discovery state: %w", err)
	}
	return s.ReprobeDevice(ctx, id)
}

// PingVirtualDevice performs a lightweight TCP reachability check for a
// virtual device that has an IP address. It tries common TCP ports and
// marks the device as "up" if any port is reachable, or "down" if all
// fail. Virtual devices without an IP are left at "unknown".
func (s *DeviceService) PingVirtualDevice(ctx context.Context, id uuid.UUID, timeout time.Duration) error {
	device, err := s.deviceRepo.GetByID(id)
	if err != nil {
		return fmt.Errorf("getting device: %w", err)
	}

	if device.IP == "" {
		// No IP — keep the virtual node inert/unknown.
		if domain.NormalizeVirtualNoIPDevice(device) {
			return s.deviceRepo.Update(device)
		}
		return nil
	}

	newStatus := domain.DeviceStatusDown
	if err := ProbeVirtualReachability(ctx, device.IP, timeout); err == nil {
		newStatus = domain.DeviceStatusUp
	}

	// Only update if status actually changed, to avoid unnecessary DB writes.
	if device.Status != newStatus {
		s.markDeviceStatus(device.ID, device.IP, newStatus)
	}

	return nil
}

// WaitForProbes blocks until all in-flight probes complete.
// Useful for testing to ensure async operations finish.
func (s *DeviceService) WaitForProbes() {
	s.probeWg.Wait()
}

// SNMPTestResult holds diagnostic info from an SNMP connectivity test.
type SNMPTestResult struct {
	Success     bool   `json:"success"`
	SysName     string `json:"sys_name,omitempty"`
	SysDescr    string `json:"sys_descr,omitempty"`
	Error       string `json:"error,omitempty"`
	TargetIP    string `json:"target_ip"`
	SNMPVersion string `json:"snmp_version"`
}

// TestSNMP attempts an SNMP connection to a device and returns diagnostic info.
func (s *DeviceService) TestSNMP(ctx context.Context, id uuid.UUID) (*SNMPTestResult, error) {
	device, err := s.deviceRepo.GetByID(id)
	if err != nil {
		return nil, fmt.Errorf("getting device: %w", err)
	}

	result := &SNMPTestResult{
		TargetIP:    device.IP,
		SNMPVersion: string(device.SNMPCredentials.Version),
	}

	discoveryResult, err := s.discoverFunc(device.IP, device.SNMPCredentials, domain.TopologyDiscoveryModeOff)
	if err != nil {
		result.Error = err.Error()
		return result, nil
	}

	result.Success = true
	result.SysName = discoveryResult.SysName
	result.SysDescr = discoveryResult.SysDescr
	return result, nil
}

func clonePollIntervalOverride(override *int) *int {
	if override == nil {
		return nil
	}
	cloned := *override
	return &cloned
}

func pollIntervalOverridesEqual(left, right *int) bool {
	switch {
	case left == nil && right == nil:
		return true
	case left == nil || right == nil:
		return false
	default:
		return *left == *right
	}
}

func dedupePreferredDiscoveredNeighbors(neighbors []snmp.NeighborInfo) []snmp.NeighborInfo {
	result := make([]snmp.NeighborInfo, 0, len(neighbors))
	for _, candidate := range neighbors {
		if shouldDropDiscoveredNeighbor(candidate, neighbors) {
			continue
		}
		result = append(result, candidate)
	}
	return result
}

func shouldDropDiscoveredNeighbor(candidate snmp.NeighborInfo, neighbors []snmp.NeighborInfo) bool {
	candidateRemote := normalizeDiscoveredNeighborIdentity(candidate.RemoteSysName)
	if candidateRemote == "" {
		candidateRemote = normalizeDiscoveredNeighborIdentity(candidate.RemoteChassisID)
	}
	if candidateRemote == "" {
		return false
	}

	candidateIsCompletePhysical := isCompletePhysicalDiscoveredNeighbor(candidate)
	candidateHasPhysicalRemotePort := isLikelyPhysicalDiscoveredInterface(candidate.RemotePortID)
	for _, other := range neighbors {
		otherRemote := normalizeDiscoveredNeighborIdentity(other.RemoteSysName)
		if otherRemote == "" {
			otherRemote = normalizeDiscoveredNeighborIdentity(other.RemoteChassisID)
		}
		if otherRemote != candidateRemote {
			continue
		}
		if compareDiscoveredNeighborPreference(other, candidate) <= 0 {
			continue
		}
		if isCompletePhysicalDiscoveredNeighbor(other) {
			if candidateIsCompletePhysical {
				continue
			}
			return true
		}
		if candidateHasPhysicalRemotePort {
			continue
		}
		if isLikelyPhysicalDiscoveredInterface(other.RemotePortID) {
			return true
		}
	}
	return false
}

func isCompletePhysicalDiscoveredNeighbor(neighbor snmp.NeighborInfo) bool {
	return discoveredPhysicalInterfaceAnchor(neighbor.LocalIfName) != "" && discoveredPhysicalInterfaceAnchor(neighbor.RemotePortID) != ""
}

func compareDiscoveredNeighborPreference(candidate, existing snmp.NeighborInfo) int {
	candidateScore := discoveredNeighborPreferenceScore(candidate)
	existingScore := discoveredNeighborPreferenceScore(existing)
	if candidateScore > existingScore {
		return 1
	}
	if candidateScore < existingScore {
		return -1
	}
	return 0
}

func discoveredNeighborPreferenceScore(neighbor snmp.NeighborInfo) int {
	score := 0
	if neighbor.Protocol == domain.DiscoveryProtocolLLDP {
		score += 100
	}
	if isLikelyPhysicalDiscoveredInterface(neighbor.LocalIfName) {
		score += 50
	}
	if isLikelyPhysicalDiscoveredInterface(neighbor.RemotePortID) {
		score += 40
	}
	if neighbor.LocalIfName != "" {
		score += 20
	}
	if neighbor.RemotePortID != "" {
		score += 20
	}
	if neighbor.RemoteSysName != "" {
		score += 10
	}
	if neighbor.RemoteChassisID != "" {
		score += 5
	}
	return score
}

func discoveredPhysicalInterfaceAnchor(name string) string {
	normalized := normalizeDiscoveredNeighborIdentity(name)
	if normalized == "" {
		return ""
	}
	return extractDiscoveredPhysicalInterfaceAnchor(normalized)
}

func isLikelyPhysicalDiscoveredInterface(name string) bool {
	normalized := normalizeDiscoveredNeighborIdentity(name)
	if normalized == "" {
		return false
	}
	return extractDiscoveredPhysicalInterfaceAnchor(normalized) != ""
}

func extractDiscoveredPhysicalInterfaceAnchor(normalized string) string {
	virtualHints := []string{
		"vlan", "vrf", "vpn", "bridge", "br-", "bond", "loopback", "lo",
		"gre", "eoip", "wg", "wireguard", "pppoe", "ppp", "sstp", "ovpn",
		"l2tp", "vxlan", "veth", "tap", "tun",
	}
	for _, hint := range virtualHints {
		if strings.Contains(normalized, hint) {
			return ""
		}
	}

	physicalPatterns := []string{
		"ether", "eth", "sfp-sfpplus", "sfp", "qsfp", "ens", "eno", "enp",
		"gigabitethernet", "tengigabitethernet", "fastethernet", "ge-", "xe-", "et-",
	}
	for _, pattern := range physicalPatterns {
		if idx := strings.Index(normalized, pattern); idx >= 0 {
			anchor := normalized[idx:]
			for i, r := range anchor {
				if !((r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '/') {
					anchor = anchor[:i]
					break
				}
			}
			anchor = strings.Trim(anchor, "- /")
			if discoveredHasDigit(anchor) {
				return anchor
			}
		}
	}

	shortPrefixes := []string{"gi", "te", "fo", "port"}
	for _, prefix := range shortPrefixes {
		if strings.HasPrefix(normalized, prefix) && discoveredHasDigit(normalized) {
			return normalized
		}
	}

	return ""
}

func discoveredHasDigit(value string) bool {
	for _, r := range value {
		if r >= '0' && r <= '9' {
			return true
		}
	}
	return false
}

func normalizeDiscoveredNeighborIdentity(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}
