package service

import (
	"fmt"
	"log"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/domain"
	"github.com/lollinoo/theia/internal/observability"
	"github.com/lollinoo/theia/internal/snmp"
	"github.com/lollinoo/theia/internal/topology"
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

func (s *DeviceService) ApplyStaticDiscovery(deviceID uuid.UUID, input StaticDiscoveryInput) (result StaticPersistenceResult, err error) {
	startedAt := time.Now()
	defer func() {
		observability.Default().ObserveTopologyMaterialization(time.Since(startedAt), err == nil)
	}()

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

	neighbors := dedupePreferredDiscoveredNeighbors(input.Neighbors)
	observability.Default().SetDiscoveryNeighborCounts(fresh.ID, countNeighborsByProtocol(neighbors))

	result = StaticPersistenceResult{TopologyChanged: interfaceChanged}
	unknownNeighbors := make(map[unknownNeighborKey]int)
	unknownByProtocol := make(map[domain.DiscoveryProtocol]int)
	if s.topologyStore != nil {
		materialized, currentUnknowns, currentTotals, materializeErr := s.applyDiscoveryViaObservationStore(*fresh, neighbors)
		if materializeErr != nil {
			return StaticPersistenceResult{}, materializeErr
		}
		result.TopologyChanged = result.TopologyChanged || materialized.TopologyChanged
		result.LinksCreated += materialized.LinksCreated
		unknownNeighbors = currentUnknowns
		unknownByProtocol = currentTotals
	} else {
		for _, neighbor := range neighbors {
			if neighbor.RemoteSysName == "" {
				continue
			}

			remoteDevice, err := s.deviceRepo.GetBySysName(neighbor.RemoteSysName)
			if err != nil {
				log.Printf("Error looking up neighbor %s: %v", neighbor.RemoteSysName, err)
				continue
			}
			if remoteDevice == nil {
				unknownNeighbors[unknownNeighborKey{
					RemoteSysName: neighbor.RemoteSysName,
					Protocol:      neighbor.Protocol,
				}]++
				unknownByProtocol[neighbor.Protocol]++
				continue
			}

			link := &domain.Link{
				SourceDeviceID:    fresh.ID,
				SourceIfName:      neighbor.LocalIfName,
				TargetDeviceID:    remoteDevice.ID,
				TargetIfName:      neighbor.RemotePortID,
				DiscoveryProtocol: neighbor.Protocol,
			}
			upsertResult, upsertErr := upsertLinkDetailed(s.linkRepo, link)
			if upsertErr != nil {
				log.Printf("Failed to upsert link %s:%s <-> %s:%s: %v",
					fresh.SysName, neighbor.LocalIfName,
					neighbor.RemoteSysName, neighbor.RemotePortID, upsertErr)
				continue
			}
			if upsertResult.Created {
				result.LinksCreated++
			}
			if upsertResult.Changed {
				result.TopologyChanged = true
			}
			if shouldLogAutoLink(upsertResult, fresh.ID == remoteDevice.ID) {
				log.Printf("Auto-linked %s:%s <-> %s:%s via %s (%s)",
					fresh.SysName, neighbor.LocalIfName,
					neighbor.RemoteSysName, neighbor.RemotePortID,
					string(neighbor.Protocol), upsertResult.Kind)
			}
		}
	}
	logUnknownNeighborSummary(fresh.ID, fresh.SysName, unknownNeighbors, unknownByProtocol)

	return result, nil
}

func (s *DeviceService) applyDiscoveryViaObservationStore(
	fresh domain.Device,
	neighbors []snmp.NeighborInfo,
) (StaticPersistenceResult, map[unknownNeighborKey]int, map[domain.DiscoveryProtocol]int, error) {
	if s.topologyStore == nil {
		return StaticPersistenceResult{}, nil, nil, nil
	}

	now := time.Now().UTC()
	affectedDeviceIDs := map[uuid.UUID]struct{}{
		fresh.ID: {},
	}
	unknownNeighbors := make(map[unknownNeighborKey]int)
	unknownByProtocol := make(map[domain.DiscoveryProtocol]int)

	for _, neighbor := range neighbors {
		if neighbor.RemoteSysName == "" {
			continue
		}

		normalizedIdentity := topology.NormalizeRemoteIdentity(neighbor.RemoteSysName)
		remoteDevice, err := s.deviceRepo.GetBySysName(neighbor.RemoteSysName)
		if err != nil {
			log.Printf("Error looking up neighbor %s: %v", neighbor.RemoteSysName, err)
			continue
		}

		observation := &topology.Observation{
			LocalDeviceID:  fresh.ID,
			RemoteIdentity: normalizedIdentity,
			LocalPort:      neighbor.LocalIfName,
			RemotePort:     neighbor.RemotePortID,
			Protocol:       neighbor.Protocol,
			LastObservedAt: now,
			SelfNeighbor:   remoteDevice != nil && remoteDevice.ID == fresh.ID,
		}
		if remoteDevice != nil {
			observation.RemoteDeviceID = remoteDevice.ID
		}
		if err := s.topologyStore.UpsertObservation(observation); err != nil {
			return StaticPersistenceResult{}, nil, nil, fmt.Errorf("upserting topology observation: %w", err)
		}

		if remoteDevice == nil {
			unknownNeighbors[unknownNeighborKey{
				RemoteSysName: neighbor.RemoteSysName,
				Protocol:      neighbor.Protocol,
			}]++
			unknownByProtocol[neighbor.Protocol]++
			if err := s.topologyStore.UpsertUnresolvedNeighbor(&topology.UnresolvedNeighbor{
				LocalDeviceID:   fresh.ID,
				RemoteIdentity:  normalizedIdentity,
				Protocol:        neighbor.Protocol,
				Occurrences:     1,
				LastObservedAt:  now,
				FirstObservedAt: now,
			}); err != nil {
				return StaticPersistenceResult{}, nil, nil, fmt.Errorf("upserting unresolved neighbor: %w", err)
			}
			continue
		}

		affectedDeviceIDs[remoteDevice.ID] = struct{}{}
		if err := s.topologyStore.ResolveUnresolvedNeighbor(fresh.ID, normalizedIdentity, neighbor.Protocol, now); err != nil {
			return StaticPersistenceResult{}, nil, nil, fmt.Errorf("resolving unresolved neighbor: %w", err)
		}
	}

	deviceIDs := make([]uuid.UUID, 0, len(affectedDeviceIDs))
	for deviceID := range affectedDeviceIDs {
		deviceIDs = append(deviceIDs, deviceID)
	}

	observations, err := s.topologyStore.ListObservationsForDevices(deviceIDs)
	if err != nil {
		return StaticPersistenceResult{}, nil, nil, fmt.Errorf("listing topology observations: %w", err)
	}

	applied, err := topology.ApplyObservations(observations, linkWriterAdapter{repo: s.linkRepo})
	if err != nil {
		return StaticPersistenceResult{}, nil, nil, fmt.Errorf("materializing canonical links: %w", err)
	}

	nameCache := map[uuid.UUID]string{
		fresh.ID: fresh.SysName,
	}
	for _, event := range applied.Events {
		if !shouldLogAutoLink(event.Result, event.Link.SourceDeviceID == event.Link.TargetDeviceID) {
			continue
		}
		sourceName := s.lookupDeviceLabel(event.Link.SourceDeviceID, nameCache)
		targetName := s.lookupDeviceLabel(event.Link.TargetDeviceID, nameCache)
		log.Printf("Auto-linked %s:%s <-> %s:%s via %s (%s)",
			sourceName, event.Link.SourceIfName,
			targetName, event.Link.TargetIfName,
			string(event.Link.DiscoveryProtocol), event.Result.Kind)
	}

	return StaticPersistenceResult{
		TopologyChanged: applied.TopologyChanged,
		LinksCreated:    applied.LinksCreated,
	}, unknownNeighbors, unknownByProtocol, nil
}

func (s *DeviceService) lookupDeviceLabel(deviceID uuid.UUID, cache map[uuid.UUID]string) string {
	if value := strings.TrimSpace(cache[deviceID]); value != "" {
		return value
	}
	device, err := s.deviceRepo.GetByID(deviceID)
	if err != nil || device == nil {
		return deviceID.String()
	}
	label := strings.TrimSpace(device.SysName)
	if label == "" {
		label = strings.TrimSpace(device.Hostname)
	}
	if label == "" {
		label = deviceID.String()
	}
	cache[deviceID] = label
	return label
}

type linkWriterAdapter struct {
	repo domain.LinkRepository
}

func (a linkWriterAdapter) UpsertDetailed(link *domain.Link) (domain.LinkUpsertResult, error) {
	return upsertLinkDetailed(a.repo, link)
}

func upsertLinkDetailed(repo domain.LinkRepository, link *domain.Link) (domain.LinkUpsertResult, error) {
	if reporter, ok := repo.(linkUpsertReporter); ok {
		return reporter.UpsertDetailed(link)
	}

	created, err := repo.Upsert(link)
	kind := domain.LinkUpsertKindNoop
	if created {
		kind = domain.LinkUpsertKindCreated
	}
	return domain.LinkUpsertResult{Created: created, Changed: created, Kind: kind}, err
}

type unknownNeighborKey struct {
	RemoteSysName string
	Protocol      domain.DiscoveryProtocol
}

func countNeighborsByProtocol(neighbors []snmp.NeighborInfo) map[domain.DiscoveryProtocol]int {
	counts := make(map[domain.DiscoveryProtocol]int)
	for _, neighbor := range neighbors {
		if neighbor.RemoteSysName == "" {
			continue
		}
		counts[neighbor.Protocol]++
	}
	return counts
}

func shouldLogAutoLink(result domain.LinkUpsertResult, selfLink bool) bool {
	switch result.Kind {
	case domain.LinkUpsertKindCreated, domain.LinkUpsertKindEnriched, domain.LinkUpsertKindReoriented:
		return true
	case domain.LinkUpsertKindUpdated:
		return selfLink
	default:
		return false
	}
}

func logUnknownNeighborSummary(deviceID uuid.UUID, localSysName string, unknowns map[unknownNeighborKey]int, totals map[domain.DiscoveryProtocol]int) {
	if len(unknowns) == 0 {
		return
	}

	var protocolTotals []string
	for protocol, count := range totals {
		observability.Default().AddUnknownNeighbors(deviceID, protocol, count)
		protocolTotals = append(protocolTotals, fmt.Sprintf("%s=%d", protocol, count))
	}
	sort.Strings(protocolTotals)

	type detail struct {
		key   unknownNeighborKey
		count int
	}
	var details []detail
	for key, count := range unknowns {
		details = append(details, detail{key: key, count: count})
	}
	sort.Slice(details, func(i, j int) bool {
		if details[i].key.Protocol != details[j].key.Protocol {
			return details[i].key.Protocol < details[j].key.Protocol
		}
		return details[i].key.RemoteSysName < details[j].key.RemoteSysName
	})

	parts := make([]string, 0, len(details))
	for index, item := range details {
		if index == 5 {
			parts = append(parts, fmt.Sprintf("... +%d more", len(details)-index))
			break
		}
		parts = append(parts, fmt.Sprintf("%s(%s)x%d", item.key.RemoteSysName, item.key.Protocol, item.count))
	}

	log.Printf("Static discovery for %s skipped unresolved neighbors [%s]: %s",
		localSysName, strings.Join(protocolTotals, ", "), strings.Join(parts, ", "))
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
