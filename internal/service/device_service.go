package service

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/domain"
	"github.com/lollinoo/theia/internal/snmp"
)

// DiscoverFunc performs SNMP discovery on a target device and returns the result.
// This abstraction allows injecting mocks for testing.
// The vendor registry is used for device detection, model extraction, and vendor identification.
type DiscoverFunc func(target string, creds domain.SNMPCredentials) (*snmp.DiscoveryResult, error)

// SNMPPollFunc polls a single device via SNMP for live metrics.
// vendorName is used to resolve vendor-specific SNMP OIDs.
type SNMPPollFunc func(target string, creds domain.SNMPCredentials, vendorName string) (domain.DeviceMetrics, error)

type pollRescheduler interface {
	ReduePerformanceTask(device domain.Device, changedAt time.Time)
}

// DeviceUpdate holds optional fields for partial device updates.
type DeviceUpdate struct {
	Hostname             *string
	IP                   *string
	Notes                **string
	Tags                 *map[string]string
	SNMPCredentials      *domain.SNMPCredentials
	Vendor               *string
	MetricsSource        *domain.MetricsSource
	PrometheusLabelName  *string
	PrometheusLabelValue *string
	PollIntervalOverride **int        // nil=not set, *nil=clear, **value=set
	AreaIDs              *[]uuid.UUID // nil=not set, non-nil=replace all area assignments
}

// DeviceService orchestrates device management, combining SNMP discovery
// with persistence through repositories.
type DeviceService struct {
	deviceRepo      domain.DeviceRepository
	linkRepo        domain.LinkRepository
	settingsRepo    domain.SettingsRepository
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

	probeWg        sync.WaitGroup
	TopologyNotify chan struct{} // signaled when probeDevice creates new links
}

const (
	incompleteLinkReprobeDelay    = 20 * time.Second
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
	svc.delayedReprobe = svc.ReprobeDevice
	return svc
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
	areaIDs []uuid.UUID,
	notes ...*string,
) (*domain.Device, error) {
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

	// Virtual devices without an IP don't use any metrics source — they are
	// status-inert nodes on the canvas whose state is never overridden by
	// Prometheus or SNMP polling.
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

	device := &domain.Device{
		ID:                   uuid.New(),
		Hostname:             hostname,
		IP:                   ip,
		Notes:                normalizedNotes,
		SNMPCredentials:      creds,
		DeviceType:           deviceType,
		PollClass:            domain.ClassifyPollClass(deviceType),
		Status:               initialStatus,
		Vendor:               vendor,
		Managed:              true,
		Tags:                 tags,
		MetricsSource:        metricsSource,
		PrometheusLabelName:  prometheusLabelName,
		PrometheusLabelValue: prometheusLabelValue,
		AreaIDs:              areaIDs,
	}
	domain.NormalizeVirtualNoIPDevice(device)

	if err := s.deviceRepo.Create(device); err != nil {
		return nil, fmt.Errorf("creating device: %w", err)
	}

	if deviceType == domain.DeviceTypeVirtual {
		// Virtual devices are not SNMP-probed. Status starts as "unknown".
		// For virtual devices WITH an IP, the MetricsCollector will update
		// status via probe_success on its next cycle (per D-05/D-07).
		// For virtual devices WITHOUT an IP, status stays "unknown" permanently (per D-06).
		return device, nil
	}

	// Launch async probe for non-virtual devices
	s.probeWg.Add(1)
	go func() {
		defer s.probeWg.Done()
		s.probeDevice(device)
	}()

	return device, nil
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

func (s *DeviceService) incompleteLLDPPeerIDs(deviceID uuid.UUID) []uuid.UUID {
	links, err := s.linkRepo.GetByDeviceID(deviceID)
	if err != nil {
		log.Printf("Failed to inspect LLDP peers for delayed re-probe on %s: %v", deviceID, err)
		return nil
	}

	seen := make(map[uuid.UUID]struct{})
	peerIDs := make([]uuid.UUID, 0, len(links))
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
		peerIDs = append(peerIDs, peerID)
	}

	return peerIDs
}

func (s *DeviceService) shouldScheduleIncompleteLinkReprobe(deviceID uuid.UUID) bool {
	device, err := s.deviceRepo.GetByID(deviceID)
	if err != nil {
		log.Printf("Failed to inspect device %s for delayed LLDP re-probe: %v", deviceID, err)
		return false
	}
	if device.Status != domain.DeviceStatusProbing {
		return false
	}
	if device.CreatedAt.IsZero() {
		return false
	}
	age := s.now().Sub(device.CreatedAt)
	if age < 0 || age > s.reprobeWindow {
		return false
	}

	return s.hasIncompleteLLDPLinks(deviceID)
}

func (s *DeviceService) reserveIncompleteLinkReprobe(deviceID uuid.UUID) bool {
	s.reprobeMu.Lock()
	defer s.reprobeMu.Unlock()

	last, ok := s.reprobeBooked[deviceID]
	if ok && s.now().Sub(last) < s.reprobeCooldown {
		return false
	}
	s.reprobeBooked[deviceID] = s.now()
	return true
}

func (s *DeviceService) scheduleIncompleteLinkReprobe(deviceID uuid.UUID, deviceIP string) {
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
		label := peer.IP
		if label == "" {
			label = peer.Hostname
		}
		targets = append(targets, reprobeTarget{id: peerID, label: label})
	}

	for _, target := range targets {
		if !s.reserveIncompleteLinkReprobe(target.id) {
			continue
		}

		targetID := target.id
		targetLabel := target.label
		log.Printf("Scheduling delayed LLDP re-probe for %s in %s to resolve incomplete ports", targetLabel, s.reprobeDelay)
		s.scheduleFunc(s.reprobeDelay, func() {
			if !s.hasIncompleteLLDPLinks(targetID) {
				return
			}
			if err := s.delayedReprobe(context.Background(), targetID); err != nil {
				log.Printf("Delayed LLDP re-probe failed for %s: %v", targetLabel, err)
			}
		})
	}
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

	result, err := s.discoverFunc(deviceIP, device.SNMPCredentials)
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
	if s.shouldScheduleIncompleteLinkReprobe(deviceID) {
		s.scheduleIncompleteLinkReprobe(deviceID, deviceIP)
	}

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
	device, err := s.deviceRepo.GetByID(id)
	if err != nil {
		return fmt.Errorf("getting device: %w", err)
	}
	previousOverride := clonePollIntervalOverride(device.PollIntervalOverride)

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
	if update.PollIntervalOverride != nil {
		device.PollIntervalOverride = *update.PollIntervalOverride
	}
	if update.AreaIDs != nil {
		device.AreaIDs = *update.AreaIDs
	}
	domain.NormalizeVirtualNoIPDevice(device)

	if err := s.deviceRepo.Update(device); err != nil {
		return err
	}
	if update.PollIntervalOverride == nil || s.pollRescheduler == nil {
		return nil
	}
	if pollIntervalOverridesEqual(previousOverride, device.PollIntervalOverride) {
		return nil
	}

	s.pollRescheduler.ReduePerformanceTask(*device, time.Now().UTC())
	return nil
}

// DeleteDevice removes a device and all associated links.
func (s *DeviceService) DeleteDevice(ctx context.Context, id uuid.UUID) error {
	// Delete associated links first
	links, err := s.linkRepo.GetByDeviceID(id)
	if err != nil {
		return fmt.Errorf("getting links for device: %w", err)
	}
	for _, link := range links {
		if err := s.linkRepo.Delete(link.ID); err != nil {
			log.Printf("Warning: failed to delete link %s: %v", link.ID, err)
		}
	}

	// Delete the device (cascading delete handles interfaces in SQLite)
	return s.deviceRepo.Delete(id)
}

// GetDevice retrieves a single device by ID.
func (s *DeviceService) GetDevice(ctx context.Context, id uuid.UUID) (*domain.Device, error) {
	device, err := s.deviceRepo.GetByID(id)
	if err != nil {
		return nil, err
	}
	domain.NormalizeVirtualNoIPDevice(device)
	return device, nil
}

// GetAllDevices retrieves all devices with their interfaces.
func (s *DeviceService) GetAllDevices(ctx context.Context) ([]domain.Device, error) {
	devices, err := s.deviceRepo.GetAll()
	if err != nil {
		return nil, err
	}
	for i := range devices {
		domain.NormalizeVirtualNoIPDevice(&devices[i])
	}
	return devices, nil
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

	discoveryResult, err := s.discoverFunc(device.IP, device.SNMPCredentials)
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
