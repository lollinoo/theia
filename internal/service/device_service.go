package service

import (
	"context"
	"fmt"
	"log"
	"sync"

	"github.com/azmin/mikrotik-theia/internal/domain"
	"github.com/azmin/mikrotik-theia/internal/snmp"
	"github.com/google/uuid"
)

// DiscoverFunc performs SNMP discovery on a target device and returns the result.
// This abstraction allows injecting mocks for testing.
type DiscoverFunc func(target string, creds domain.SNMPCredentials) (*snmp.DiscoveryResult, error)

// DeviceUpdate holds optional fields for partial device updates.
type DeviceUpdate struct {
	Hostname             *string
	IP                   *string
	Tags                 *map[string]string
	SNMPCredentials      *domain.SNMPCredentials
	MetricsSource        *domain.MetricsSource
	PrometheusLabelName  *string
	PrometheusLabelValue *string
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
	metricsSource domain.MetricsSource,
	prometheusLabelName string,
	prometheusLabelValue string,
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
		Managed:              true,
		Tags:                 tags,
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
		log.Printf("DEBUG: probeDevice launched with SNMP Config - Version: %s, V2c: %+v, V3: %+v", device.SNMPCredentials.Version, device.SNMPCredentials.V2c, device.SNMPCredentials.V3)
		s.probeDevice(device)
	}()

	return device, nil
}

// probeDevice performs SNMP discovery and updates the device in the repository.
// It re-fetches the device from the repo to avoid racing on the pointer
// that was returned to the caller of AddDevice.
func (s *DeviceService) probeDevice(device *domain.Device) {
	// Capture immutable values needed for probe
	deviceID := device.ID
	deviceIP := device.IP
	creds := device.SNMPCredentials

	result, err := s.discoverFunc(deviceIP, creds)

	// Re-fetch from repo to get a fresh copy (avoids data race with caller)
	fresh, fetchErr := s.deviceRepo.GetByID(deviceID)
	if fetchErr != nil {
		log.Printf("Failed to re-fetch device %s for probe update: %v", deviceIP, fetchErr)
		return
	}

	if err != nil {
		log.Printf("SNMP discovery failed for %s: %v", deviceIP, err)
		fresh.Status = domain.DeviceStatusDown
		if updateErr := s.deviceRepo.Update(fresh); updateErr != nil {
			log.Printf("Failed to update device %s status to down: %v", deviceIP, updateErr)
		}
		return
	}

	// Update device fields from discovery
	fresh.SysName = result.SysName
	fresh.SysDescr = result.SysDescr
	fresh.SysObjectID = result.SysObjectID
	fresh.HardwareModel = result.HardwareModel
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
	if update.MetricsSource != nil {
		device.MetricsSource = *update.MetricsSource
	}
	if update.PrometheusLabelName != nil {
		device.PrometheusLabelName = *update.PrometheusLabelName
	}
	if update.PrometheusLabelValue != nil {
		device.PrometheusLabelValue = *update.PrometheusLabelValue
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

// WaitForProbes blocks until all in-flight probes complete.
// Useful for testing to ensure async operations finish.
func (s *DeviceService) WaitForProbes() {
	s.probeWg.Wait()
}
