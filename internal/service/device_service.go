package service

import (
	"context"
	"fmt"
	"log"
	"sync"

	"github.com/lollinoo/theia/internal/domain"
	"github.com/lollinoo/theia/internal/snmp"
	"github.com/google/uuid"
)

// DiscoverFunc performs SNMP discovery on a target device and returns the result.
// This abstraction allows injecting mocks for testing.
// The vendor registry is used for device detection, model extraction, and vendor identification.
type DiscoverFunc func(target string, creds domain.SNMPCredentials) (*snmp.DiscoveryResult, error)

// SNMPPollFunc polls a single device via SNMP for live metrics.
// vendorName is used to resolve vendor-specific SNMP OIDs.
type SNMPPollFunc func(target string, creds domain.SNMPCredentials, vendorName string) (domain.DeviceMetrics, error)

// DeviceUpdate holds optional fields for partial device updates.
type DeviceUpdate struct {
	Hostname             *string
	IP                   *string
	Tags                 *map[string]string
	SNMPCredentials      *domain.SNMPCredentials
	Vendor               *string
	MetricsSource        *domain.MetricsSource
	PrometheusLabelName  *string
	PrometheusLabelValue *string
	SSHProfileID         **uuid.UUID   // double pointer: nil=not set, *nil=unassign, **=set
	AreaIDs              *[]uuid.UUID // nil=not set, non-nil=replace all area assignments
}

// DeviceService orchestrates device management, combining SNMP discovery
// with persistence through repositories.
type DeviceService struct {
	deviceRepo   domain.DeviceRepository
	linkRepo     domain.LinkRepository
	settingsRepo domain.SettingsRepository
	discoverFunc DiscoverFunc

	probeWg sync.WaitGroup
}

// NewDeviceService creates a new DeviceService with the given dependencies.
func NewDeviceService(
	deviceRepo domain.DeviceRepository,
	linkRepo domain.LinkRepository,
	settingsRepo domain.SettingsRepository,
	discoverFunc DiscoverFunc,
) *DeviceService {
	return &DeviceService{
		deviceRepo:   deviceRepo,
		linkRepo:     linkRepo,
		settingsRepo: settingsRepo,
		discoverFunc: discoverFunc,
	}
}

// AddDevice creates a new device with status "probing" and triggers an async
// SNMP probe. The device is returned immediately before the probe completes.
func (s *DeviceService) AddDevice(
	ctx context.Context,
	ip, hostname string,
	creds domain.SNMPCredentials,
	tags map[string]string,
	vendor string,
	metricsSource domain.MetricsSource,
	prometheusLabelName string,
	prometheusLabelValue string,
	sshProfileID *uuid.UUID,
) (*domain.Device, error) {
	if ip == "" {
		return nil, fmt.Errorf("IP address is required")
	}
	if tags == nil {
		tags = map[string]string{}
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

	device := &domain.Device{
		ID:                   uuid.New(),
		Hostname:             hostname,
		IP:                   ip,
		SNMPCredentials:      creds,
		DeviceType:           domain.DeviceTypeUnknown,
		Status:               domain.DeviceStatusProbing,
		Vendor:               vendor,
		Managed:              true,
		Tags:                 tags,
		SSHProfileID:         sshProfileID,
		MetricsSource:        metricsSource,
		PrometheusLabelName:  prometheusLabelName,
		PrometheusLabelValue: prometheusLabelValue,
	}

	if err := s.deviceRepo.Create(device); err != nil {
		return nil, fmt.Errorf("creating device: %w", err)
	}

	// Launch async probe
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
func (s *DeviceService) markDeviceStatus(deviceID uuid.UUID, deviceIP string, status domain.DeviceStatus) {
	fresh, err := s.deviceRepo.GetByID(deviceID)
	if err != nil {
		log.Printf("Failed to re-fetch device %s for status update: %v", deviceIP, err)
		return
	}
	fresh.Status = status
	if err := s.deviceRepo.Update(fresh); err != nil {
		log.Printf("Failed to update device %s status to %s: %v", deviceIP, string(status), err)
	}
}

func (s *DeviceService) probeDevice(device *domain.Device) {
	deviceID := device.ID
	deviceIP := device.IP

	// Prometheus-only devices never touch gosnmp — mark up and return.
	if device.MetricsSource == domain.MetricsSourcePrometheus {
		s.markDeviceStatus(deviceID, deviceIP, domain.DeviceStatusUp)
		log.Printf("Skipped SNMP probe for %s (metrics_source=prometheus); marked up", deviceIP)
		return
	}

	result, err := s.discoverFunc(deviceIP, device.SNMPCredentials)
	if err != nil {
		log.Printf("SNMP discovery failed for %s: %v", deviceIP, err)
		s.markDeviceStatus(deviceID, deviceIP, domain.DeviceStatusDown)
		return
	}

	// Re-fetch from repo to get a fresh copy (avoids data race with caller)
	fresh, fetchErr := s.deviceRepo.GetByID(deviceID)
	if fetchErr != nil {
		log.Printf("Failed to re-fetch device %s for probe update: %v", deviceIP, fetchErr)
		return
	}

	// Update device fields from discovery
	fresh.SysName = result.SysName
	fresh.SysDescr = result.SysDescr
	fresh.SysObjectID = result.SysObjectID
	fresh.HardwareModel = result.HardwareModel
	// Only overwrite vendor if the user didn't manually tag one
	if fresh.Vendor == "" || fresh.Vendor == "default" {
		fresh.Vendor = result.Vendor
	}
	fresh.DeviceType = result.DeviceType
	fresh.Status = domain.DeviceStatusUp
	fresh.Interfaces = result.Interfaces

	if err := s.deviceRepo.Update(fresh); err != nil {
		log.Printf("Failed to update device %s after probe: %v", deviceIP, err)
		return
	}

	// Auto-create links from LLDP/CDP neighbors
	for _, neighbor := range result.Neighbors {
		if neighbor.RemoteSysName == "" {
			continue
		}
		remoteDevice, err := s.deviceRepo.GetBySysName(neighbor.RemoteSysName)
		if err != nil {
			log.Printf("Error looking up neighbor %s: %v", neighbor.RemoteSysName, err)
			continue
		}
		if remoteDevice == nil {
			log.Printf("Skipping neighbor %s: device not found in system", neighbor.RemoteSysName)
			continue
		}

		link := &domain.Link{
			SourceDeviceID:    fresh.ID,
			SourceIfName:      neighbor.LocalIfName,
			TargetDeviceID:    remoteDevice.ID,
			TargetIfName:      neighbor.RemotePortID,
			DiscoveryProtocol: neighbor.Protocol,
		}
		if err := s.linkRepo.Upsert(link); err != nil {
			log.Printf("Failed to upsert link %s:%s <-> %s:%s: %v",
				fresh.SysName, neighbor.LocalIfName,
				neighbor.RemoteSysName, neighbor.RemotePortID, err)
			continue
		}
		log.Printf("Auto-linked %s:%s <-> %s:%s via %s",
			fresh.SysName, neighbor.LocalIfName,
			neighbor.RemoteSysName, neighbor.RemotePortID,
			string(neighbor.Protocol))
	}
}

// UpdateDevice applies partial updates to an existing device without re-probing.
func (s *DeviceService) UpdateDevice(ctx context.Context, id uuid.UUID, update DeviceUpdate) error {
	device, err := s.deviceRepo.GetByID(id)
	if err != nil {
		return fmt.Errorf("getting device: %w", err)
	}

	if update.Hostname != nil {
		device.Hostname = *update.Hostname
	}
	if update.IP != nil {
		device.IP = *update.IP
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
	if update.SSHProfileID != nil {
		device.SSHProfileID = *update.SSHProfileID
	}
	if update.AreaIDs != nil {
		device.AreaIDs = *update.AreaIDs
	}

	return s.deviceRepo.Update(device)
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
	return s.deviceRepo.GetByID(id)
}

// GetAllDevices retrieves all devices with their interfaces.
func (s *DeviceService) GetAllDevices(ctx context.Context) ([]domain.Device, error) {
	return s.deviceRepo.GetAll()
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
