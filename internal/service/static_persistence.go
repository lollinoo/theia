package service

import (
	"fmt"
	"log"
	"sort"

	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/domain"
	"github.com/lollinoo/theia/internal/snmp"
)

type StaticDiscoveryInput struct {
	SysName       string
	SysDescr      string
	SysObjectID   string
	HardwareModel string
	Vendor        string
	DeviceType    domain.DeviceType
	Interfaces    []domain.Interface
	Neighbors     []snmp.NeighborInfo
}

type StaticPersistenceResult struct {
	TopologyChanged bool
	LinksCreated    int
}

type linkUpsertReporter interface {
	UpsertDetailed(link *domain.Link) (domain.LinkUpsertResult, error)
}

func (s *DeviceService) ApplyStaticDiscovery(deviceID uuid.UUID, input StaticDiscoveryInput) (StaticPersistenceResult, error) {
	fresh, err := s.deviceRepo.GetByID(deviceID)
	if err != nil {
		return StaticPersistenceResult{}, fmt.Errorf("re-fetch device: %w", err)
	}

	interfaceChanged := staticInterfaceSetChanged(fresh.Interfaces, input.Interfaces)

	fresh.SysName = input.SysName
	fresh.SysDescr = input.SysDescr
	fresh.SysObjectID = input.SysObjectID
	fresh.HardwareModel = input.HardwareModel
	if fresh.Vendor == "" || fresh.Vendor == "default" {
		fresh.Vendor = input.Vendor
	}
	fresh.DeviceType = input.DeviceType
	fresh.Interfaces = append([]domain.Interface(nil), input.Interfaces...)
	if fresh.PollIntervalOverride == nil {
		fresh.PollClass = domain.ClassifyPollClass(fresh.DeviceType)
	}

	if err := s.deviceRepo.Update(fresh); err != nil {
		return StaticPersistenceResult{}, fmt.Errorf("update device: %w", err)
	}

	result := StaticPersistenceResult{TopologyChanged: interfaceChanged}
	for _, neighbor := range dedupePreferredDiscoveredNeighbors(input.Neighbors) {
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
		upsertResult := domain.LinkUpsertResult{}
		if reporter, ok := s.linkRepo.(linkUpsertReporter); ok {
			upsertResult, err = reporter.UpsertDetailed(link)
		} else {
			var created bool
			created, err = s.linkRepo.Upsert(link)
			upsertResult = domain.LinkUpsertResult{Created: created, Changed: created}
		}
		if err != nil {
			log.Printf("Failed to upsert link %s:%s <-> %s:%s: %v",
				fresh.SysName, neighbor.LocalIfName,
				neighbor.RemoteSysName, neighbor.RemotePortID, err)
			continue
		}
		if upsertResult.Created {
			result.LinksCreated++
		}
		if upsertResult.Changed {
			result.TopologyChanged = true
		}
		log.Printf("Auto-linked %s:%s <-> %s:%s via %s",
			fresh.SysName, neighbor.LocalIfName,
			neighbor.RemoteSysName, neighbor.RemotePortID,
			string(neighbor.Protocol))
	}

	return result, nil
}

type interfaceMaterialSignature struct {
	IfName  string
	IfDescr string
	Speed   int64
}

func staticInterfaceSetChanged(before, after []domain.Interface) bool {
	if len(before) != len(after) {
		return true
	}

	beforeSignatures := materialInterfaceSignatures(before)
	afterSignatures := materialInterfaceSignatures(after)
	for i := range beforeSignatures {
		if beforeSignatures[i] != afterSignatures[i] {
			return true
		}
	}
	return false
}

func materialInterfaceSignatures(interfaces []domain.Interface) []interfaceMaterialSignature {
	signatures := make([]interfaceMaterialSignature, 0, len(interfaces))
	for _, iface := range interfaces {
		signatures = append(signatures, interfaceMaterialSignature{
			IfName:  iface.IfName,
			IfDescr: iface.IfDescr,
			Speed:   iface.Speed,
		})
	}
	sort.Slice(signatures, func(i, j int) bool {
		if signatures[i].IfName != signatures[j].IfName {
			return signatures[i].IfName < signatures[j].IfName
		}
		if signatures[i].IfDescr != signatures[j].IfDescr {
			return signatures[i].IfDescr < signatures[j].IfDescr
		}
		return signatures[i].Speed < signatures[j].Speed
	})
	return signatures
}
