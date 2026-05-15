package service

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/domain"
	"github.com/lollinoo/theia/internal/snmp"
)

type deviceDiscoveryCoordinator struct {
	parent *DeviceService
}

func newDeviceDiscoveryCoordinator(parent *DeviceService) *deviceDiscoveryCoordinator {
	return &deviceDiscoveryCoordinator{parent: parent}
}

func (d *deviceDiscoveryCoordinator) runDelayedReprobe(_ context.Context, id uuid.UUID) error {
	device, err := d.parent.deviceRepo.GetByID(id)
	if err != nil {
		return fmt.Errorf("getting device: %w", err)
	}

	d.probeDevice(device)
	return nil
}

func (d *deviceDiscoveryCoordinator) scheduleIncompleteLinkReprobeAttempt(
	targetID uuid.UUID,
	targetLabel string,
	bookedAt time.Time,
	delay time.Duration,
) {
	s := d.parent
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
				d.scheduleIncompleteLinkReprobeAttempt(
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

func (d *deviceDiscoveryCoordinator) scheduleIncompleteLinkReprobe(deviceID uuid.UUID, deviceIP string) bool {
	s := d.parent
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
		d.scheduleIncompleteLinkReprobeAttempt(target.id, targetLabel, bookedAt, s.reprobeDelay)
	}

	return scheduled
}

func (d *deviceDiscoveryCoordinator) probeDevice(device *domain.Device) {
	s := d.parent
	deviceID := device.ID
	deviceIP := device.IP

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
		SysName:                   result.SysName,
		SysDescr:                  result.SysDescr,
		SysObjectID:               result.SysObjectID,
		HardwareModel:             result.HardwareModel,
		OSVersion:                 result.OSVersion,
		Vendor:                    result.Vendor,
		DeviceType:                result.DeviceType,
		Interfaces:                result.Interfaces,
		Neighbors:                 result.Neighbors,
		NeighborDiscoveryFailures: result.NeighborDiscoveryFailures,
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
		followupScheduled = d.scheduleIncompleteLinkReprobe(deviceID, deviceIP)
	}
	s.syncTopologyDiscoveryMetadata(deviceID, len(result.Neighbors), followupScheduled, snmp.HasCriticalNeighborDiscoveryFailure(result.NeighborDiscoveryFailures))
	s.finalizeBootstrapWindowIfExhausted(deviceID, followupScheduled)

	if err := s.updateDeviceStatus(deviceID, domain.DeviceStatusUp); err != nil {
		log.Printf("Failed to update device %s status to up: %v", deviceIP, err)
		return
	}

	if persisted.TopologyChanged && s.TopologyNotify != nil {
		select {
		case s.TopologyNotify <- struct{}{}:
		default:
		}
	}
}

func (d *deviceDiscoveryCoordinator) ProbeDevice(ctx context.Context, id uuid.UUID) error {
	device, err := d.parent.deviceRepo.GetByID(id)
	if err != nil {
		return fmt.Errorf("getting device: %w", err)
	}

	device.Status = domain.DeviceStatusProbing
	if err := d.parent.deviceRepo.Update(device); err != nil {
		return fmt.Errorf("updating device status: %w", err)
	}

	d.parent.probeWg.Add(1)
	go func() {
		defer d.parent.probeWg.Done()
		d.probeDevice(device)
	}()

	return nil
}

func (d *deviceDiscoveryCoordinator) ReprobeDevice(ctx context.Context, id uuid.UUID) error {
	device, err := d.parent.deviceRepo.GetByID(id)
	if err != nil {
		return fmt.Errorf("getting device: %w", err)
	}

	d.parent.probeWg.Add(1)
	go func() {
		defer d.parent.probeWg.Done()
		d.probeDevice(device)
	}()

	return nil
}

func (d *deviceDiscoveryCoordinator) RunTopologyDiscoveryNow(ctx context.Context, id uuid.UUID) error {
	device, err := d.parent.deviceRepo.GetByID(id)
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
	if err := d.parent.deviceRepo.Update(device); err != nil {
		return fmt.Errorf("updating topology discovery state: %w", err)
	}
	return d.ReprobeDevice(ctx, id)
}

func (d *deviceDiscoveryCoordinator) PingVirtualDevice(ctx context.Context, id uuid.UUID, timeout time.Duration) error {
	device, err := d.parent.deviceRepo.GetByID(id)
	if err != nil {
		return fmt.Errorf("getting device: %w", err)
	}

	if device.IP == "" {
		if domain.NormalizeVirtualNoIPDevice(device) {
			return d.parent.deviceRepo.Update(device)
		}
		return nil
	}

	newStatus := domain.DeviceStatusDown
	if err := ProbeVirtualReachability(ctx, device.IP, timeout); err == nil {
		newStatus = domain.DeviceStatusUp
	}

	if device.Status != newStatus {
		d.parent.markDeviceStatus(device.ID, device.IP, newStatus)
	}

	return nil
}

func (d *deviceDiscoveryCoordinator) WaitForProbes() {
	d.parent.probeWg.Wait()
}

func (d *deviceDiscoveryCoordinator) TestSNMP(ctx context.Context, id uuid.UUID) (*SNMPTestResult, error) {
	device, err := d.parent.deviceRepo.GetByID(id)
	if err != nil {
		return nil, fmt.Errorf("getting device: %w", err)
	}

	result := &SNMPTestResult{
		TargetIP:    device.IP,
		SNMPVersion: string(device.SNMPCredentials.Version),
	}

	discoveryResult, err := d.parent.discoverFunc(device.IP, device.SNMPCredentials, domain.TopologyDiscoveryModeOff)
	if err != nil {
		result.Error = err.Error()
		return result, nil
	}

	result.Success = true
	result.SysName = discoveryResult.SysName
	result.SysDescr = discoveryResult.SysDescr
	return result, nil
}
