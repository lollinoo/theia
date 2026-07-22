package service

// This file defines device service service behavior and domain orchestration rules.

import (
	"context"
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
	RedueEssentialTask(device domain.Device, changedAt time.Time)
	ReconcileDeviceTasks(device domain.Device, changedAt time.Time)
}

type bootstrapScheduler interface {
	ScheduleBootstrap(device domain.Device, dueAt time.Time) bool
}

type runtimeResetter interface {
	ResetDeviceRuntime(deviceID uuid.UUID)
}

// DeviceUpdate holds optional fields for partial device updates.
type DeviceUpdate struct {
	Hostname              *string
	IP                    *string
	Addresses             *[]domain.DeviceAddress
	ProbePorts            *[]int
	Notes                 **string
	Tags                  *map[string]string
	SNMPCredentials       *domain.SNMPCredentials
	Vendor                *string
	MetricsSource         *domain.MetricsSource
	PrometheusLabelName   *string
	PrometheusLabelValue  *string
	TopologyDiscoveryMode *domain.TopologyDiscoveryMode
	PollingEnabled        *bool
	PollIntervalOverride  **int        // nil=not set, *nil=clear, **value=set
	AreaIDs               *[]uuid.UUID // nil=not set, non-nil=replace all area assignments
}

// DeviceDraftInput holds the named inputs used to construct a normalized device draft.
type DeviceDraftInput struct {
	IP                    string
	Hostname              string
	DeviceType            domain.DeviceType
	SNMPCredentials       domain.SNMPCredentials
	Tags                  map[string]string
	Vendor                string
	MetricsSource         domain.MetricsSource
	PrometheusLabelName   string
	PrometheusLabelValue  string
	TopologyDiscoveryMode domain.TopologyDiscoveryMode
	AreaIDs               []uuid.UUID
	ProbePorts            []int
	Addresses             []domain.DeviceAddress
	Notes                 *string
}

// DeviceService orchestrates device management, combining SNMP discovery
// with persistence through repositories.
type DeviceService struct {
	deviceRepo         domain.DeviceRepository
	linkRepo           domain.LinkRepository
	topologyStore      topology.ObservationStore
	settingsRepo       domain.SettingsRepository
	networkProbe       func(context.Context, string, time.Duration, []int) error
	mutation           *deviceMutationService
	discovery          *deviceDiscoveryCoordinator
	discoverFunc       DiscoverFunc
	pollRescheduler    pollRescheduler
	bootstrapScheduler bootstrapScheduler
	runtimeResetter    runtimeResetter
	now                func() time.Time
	scheduleFunc       func(time.Duration, func())
	delayedReprobe     func(context.Context, uuid.UUID) error
	reprobeDelay       time.Duration
	reprobeCooldown    time.Duration
	reprobeWindow      time.Duration
	reprobeMu          sync.Mutex
	reprobeBooked      map[uuid.UUID]time.Time
	reprobeInFlight    atomic.Int32

	lifecycleParent context.Context
	lifecycleCtx    context.Context
	lifecycleCancel context.CancelFunc
	stopOnce        sync.Once
	asyncMu         sync.Mutex
	probeWg         sync.WaitGroup
	TopologyNotify  chan struct{} // signaled when probeDevice creates new links
}

// DeviceServiceOption represents device service option data used by the service orchestration.
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
		networkProbe:    ProbeTCPReachability,
		discoverFunc:    discoverFunc,
		now:             time.Now,
		scheduleFunc:    func(delay time.Duration, fn func()) { time.AfterFunc(delay, fn) },
		reprobeDelay:    incompleteLinkReprobeDelay,
		reprobeCooldown: incompleteLinkReprobeCooldown,
		reprobeWindow:   incompleteLinkReprobeWindow,
		reprobeBooked:   make(map[uuid.UUID]time.Time),
		lifecycleParent: context.Background(),
		TopologyNotify:  topologyNotify,
	}
	for _, option := range options {
		if option != nil {
			option(svc)
		}
	}
	if svc.lifecycleParent == nil {
		svc.lifecycleParent = context.Background()
	}
	svc.lifecycleCtx, svc.lifecycleCancel = context.WithCancel(svc.lifecycleParent)
	svc.mutation = newDeviceMutationService(svc)
	svc.discovery = newDeviceDiscoveryCoordinator(svc)
	if svc.delayedReprobe == nil {
		svc.delayedReprobe = svc.discovery.runDelayedReprobe
	}
	return svc
}

// WithNetworkReachabilityProbe overrides the TCP reachability probe used by diagnostics.
func WithNetworkReachabilityProbe(probe func(context.Context, string, time.Duration, []int) error) DeviceServiceOption {
	return func(s *DeviceService) {
		if probe != nil {
			s.networkProbe = probe
		}
	}
}

// WithLifecycleContext binds async device probes and delayed reprobes to a service parent context.
func WithLifecycleContext(ctx context.Context) DeviceServiceOption {
	return func(s *DeviceService) {
		if ctx != nil {
			s.lifecycleParent = ctx
		}
	}
}

func WithTopologyObservationStore(store topology.ObservationStore) DeviceServiceOption {
	return func(s *DeviceService) {
		s.topologyStore = store
	}
}

func WithBootstrapScheduler(scheduler bootstrapScheduler) DeviceServiceOption {
	return func(s *DeviceService) {
		s.bootstrapScheduler = scheduler
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

func (s *DeviceService) SetBootstrapScheduler(scheduler bootstrapScheduler) {
	s.bootstrapScheduler = scheduler
}

func (s *DeviceService) SetRuntimeResetter(resetter runtimeResetter) {
	s.runtimeResetter = resetter
}

func (s *DeviceService) lifecycleErr() error {
	if s.lifecycleCtx == nil {
		return nil
	}
	select {
	case <-s.lifecycleCtx.Done():
		return s.lifecycleCtx.Err()
	default:
		return nil
	}
}

func (s *DeviceService) beginLifecycleWork() (context.Context, func(), bool) {
	if err := s.lifecycleErr(); err != nil {
		return nil, nil, false
	}

	s.asyncMu.Lock()
	defer s.asyncMu.Unlock()

	if err := s.lifecycleErr(); err != nil {
		return nil, nil, false
	}
	s.probeWg.Add(1)
	workCtx := s.lifecycleCtx
	if workCtx == nil {
		workCtx = context.Background()
	}
	return workCtx, s.probeWg.Done, true
}

func (s *DeviceService) startLifecycleProbe(device *domain.Device) bool {
	workCtx, done, ok := s.beginLifecycleWork()
	if !ok {
		return false
	}
	go func() {
		defer done()
		if workCtx.Err() != nil {
			return
		}
		s.probeDevice(device)
	}()
	return true
}

// Stop cancels service-owned async probe work and waits for tracked probes to drain.
func (s *DeviceService) Stop() {
	s.stopOnce.Do(func() {
		if s.lifecycleCancel != nil {
			s.lifecycleCancel()
		}
	})
	s.asyncMu.Lock()
	s.asyncMu.Unlock()
	s.WaitForProbes()
}

// BuildDeviceDraft applies AddDevice defaults and normalization without persistence or probes.
func (s *DeviceService) BuildDeviceDraft(input DeviceDraftInput) (*domain.Device, error) {
	tags := input.Tags
	if tags == nil {
		tags = map[string]string{}
	}

	deviceType := input.DeviceType
	if deviceType == "" {
		deviceType = domain.DeviceTypeUnknown
	}

	metricsSource := input.MetricsSource
	if metricsSource == "" {
		metricsSource = domain.MetricsSourcePrometheus
	}

	prometheusLabelName := input.PrometheusLabelName
	if prometheusLabelName == "" {
		prometheusLabelName = "instance"
	}
	prometheusLabelValue := input.PrometheusLabelValue
	if prometheusLabelValue == "" {
		prometheusLabelValue = input.IP
	}

	if deviceType == domain.DeviceTypeVirtual {
		metricsSource = domain.MetricsSourceNone
	}

	initialStatus := domain.DeviceStatusProbing
	if deviceType == domain.DeviceTypeVirtual {
		initialStatus = domain.DeviceStatusUnknown
	}
	pollingEnabled := true
	pollIntervalOverride := initialPollIntervalOverride(s.settingsRepo, deviceType)
	normalizedProbePorts, err := domain.NormalizeProbePorts(input.ProbePorts)
	if err != nil {
		return nil, err
	}

	device := &domain.Device{
		ID:                     uuid.New(),
		Hostname:               input.Hostname,
		IP:                     input.IP,
		Notes:                  domain.NormalizeDeviceNotes(input.Notes),
		SNMPCredentials:        input.SNMPCredentials,
		DeviceType:             deviceType,
		PollClass:              domain.ClassifyPollClass(deviceType),
		PollIntervalOverride:   pollIntervalOverride,
		PollingEnabled:         &pollingEnabled,
		Status:                 initialStatus,
		Vendor:                 input.Vendor,
		Managed:                true,
		Tags:                   tags,
		MetricsSource:          metricsSource,
		PrometheusLabelName:    prometheusLabelName,
		PrometheusLabelValue:   prometheusLabelValue,
		TopologyDiscoveryMode:  input.TopologyDiscoveryMode,
		TopologyBootstrapState: domain.TopologyBootstrapStateIdle,
		AreaIDs:                input.AreaIDs,
		ProbePorts:             normalizedProbePorts,
		Addresses:              append([]domain.DeviceAddress(nil), input.Addresses...),
	}
	domain.NormalizeDeviceAddresses(device)
	device.TopologyDiscoveryMode = domain.NormalizeTopologyDiscoveryMode(device.TopologyDiscoveryMode, domain.TopologyDiscoveryModeInherit)
	if domain.ResolveTopologyDiscoveryMode(device, s.defaultTopologyDiscoveryMode()) == domain.TopologyDiscoveryModeBootstrapOnce {
		device.TopologyBootstrapState = domain.TopologyBootstrapStatePending
	}
	domain.NormalizeVirtualDevice(device)

	return device, nil
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
	return s.mutation.AddDevice(ctx, ip, hostname, deviceType, creds, tags, vendor, metricsSource, prometheusLabelName, prometheusLabelValue, topologyDiscoveryMode, areaIDs, nil, nil, notes...)
}

// AddDeviceWithAddresses creates a device with an explicit address collection
// while preserving AddDevice's legacy single-IP signature for existing callers.
func (s *DeviceService) AddDeviceWithAddresses(
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
	probePorts []int,
	addresses []domain.DeviceAddress,
	notes ...*string,
) (*domain.Device, error) {
	return s.mutation.AddDevice(ctx, ip, hostname, deviceType, creds, tags, vendor, metricsSource, prometheusLabelName, prometheusLabelValue, topologyDiscoveryMode, areaIDs, probePorts, addresses, notes...)
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
	beforeDevice := *device
	beforeDevice.TopologyDiscoveryMode = domain.NormalizeTopologyDiscoveryMode(beforeDevice.TopologyDiscoveryMode, domain.TopologyDiscoveryModeInherit)
	beforeDevice.TopologyBootstrapState = domain.NormalizeTopologyBootstrapState(beforeDevice.TopologyBootstrapState)
	before := topologyDiscoveryStateSignatureFor(beforeDevice)
	if mutate != nil {
		mutate(device)
	}
	device.TopologyDiscoveryMode = domain.NormalizeTopologyDiscoveryMode(device.TopologyDiscoveryMode, domain.TopologyDiscoveryModeInherit)
	device.TopologyBootstrapState = domain.NormalizeTopologyBootstrapState(device.TopologyBootstrapState)
	if topologyDiscoveryStateSignatureFor(*device) == before {
		return
	}
	if err := s.deviceRepo.Update(device); err != nil {
		log.Printf("Failed to update topology discovery state for %s: %v", deviceID, err)
	}
}

type topologyDiscoveryStateSignature struct {
	Mode      domain.TopologyDiscoveryMode
	Bootstrap domain.TopologyBootstrapState
	LastAtSet bool
	LastAt    time.Time
	Result    string
}

func topologyDiscoveryStateSignatureFor(device domain.Device) topologyDiscoveryStateSignature {
	signature := topologyDiscoveryStateSignature{
		Mode:      device.TopologyDiscoveryMode,
		Bootstrap: device.TopologyBootstrapState,
		Result:    device.LastTopologyDiscoveryResult,
	}
	if device.LastTopologyDiscoveryAt != nil {
		signature.LastAtSet = true
		signature.LastAt = *device.LastTopologyDiscoveryAt
	}
	return signature
}

func topologyDiscoveryResultLabel(neighborCount int, followupScheduled bool, criticalFailure bool) string {
	switch {
	case criticalFailure && (neighborCount > 0 || followupScheduled):
		return "partial_discovery_failed"
	case criticalFailure:
		return "discovery_failed"
	case neighborCount > 0 && followupScheduled:
		return "ports_pending"
	case neighborCount > 0:
		return "neighbors_found"
	case followupScheduled:
		return "ports_pending"
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

func (s *DeviceService) syncTopologyDiscoveryMetadata(deviceID uuid.UUID, neighborCount int, followupScheduled bool, criticalFailure bool) {
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
			criticalFailure,
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

func (s *DeviceService) staticReprobeBudget() int {
	budgets := pollingbudget.Resolve(s.settingsRepo)
	return budgets[domain.VolatilityClassStatic]
}

func (s *DeviceService) probeDevice(device *domain.Device) {
	s.discovery.probeDevice(device)
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

// GetOrphanDevices retrieves global devices that are not in any saved map.
func (s *DeviceService) GetOrphanDevices(ctx context.Context) ([]domain.Device, error) {
	return s.mutation.GetOrphanDevices(ctx)
}

// GetDevicesByIDs retrieves selected devices with their interfaces.
func (s *DeviceService) GetDevicesByIDs(ctx context.Context, ids []uuid.UUID) ([]domain.Device, error) {
	return s.mutation.GetDevicesByIDs(ctx, ids)
}

// GetTopologyDevicesByIDs retrieves selected devices without decrypting sensitive credentials.
func (s *DeviceService) GetTopologyDevicesByIDs(ctx context.Context, ids []uuid.UUID) ([]domain.Device, error) {
	return s.mutation.GetTopologyDevicesByIDs(ctx, ids)
}

// ProbeDevice triggers a re-probe of an existing device.
func (s *DeviceService) ProbeDevice(ctx context.Context, id uuid.UUID) error {
	return s.discovery.ProbeDevice(ctx, id)
}

// ReprobeDevice triggers a re-probe of an existing device without
// transitioning through the "probing" status. This avoids status flapping
// when the Poller re-probes all devices every cycle -- the current status
// (up/down) is kept until the probe result is known.
func (s *DeviceService) ReprobeDevice(ctx context.Context, id uuid.UUID) error {
	return s.discovery.ReprobeDevice(ctx, id)
}

func (s *DeviceService) RunTopologyDiscoveryNow(ctx context.Context, id uuid.UUID) error {
	return s.discovery.RunTopologyDiscoveryNow(ctx, id)
}

// PingVirtualDevice performs a lightweight TCP reachability check for a
// virtual device that has an IP address. It tries common TCP ports and
// marks the device as "up" if any port is reachable, or "down" if all
// fail. Virtual devices without an IP are left at "unknown".
func (s *DeviceService) PingVirtualDevice(ctx context.Context, id uuid.UUID, timeout time.Duration) error {
	return s.discovery.PingVirtualDevice(ctx, id, timeout)
}

// WaitForProbes blocks until all in-flight probes complete.
// Useful for testing to ensure async operations finish.
func (s *DeviceService) WaitForProbes() {
	s.discovery.WaitForProbes()
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
	return s.discovery.TestSNMP(ctx, id)
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

func (s *DeviceService) scheduleIncompleteLinkReprobe(deviceID uuid.UUID, deviceIP string) bool {
	return s.discovery.scheduleIncompleteLinkReprobe(deviceID, deviceIP)
}
