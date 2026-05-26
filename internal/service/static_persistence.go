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
	SysName                    string
	SysDescr                   string
	SysObjectID                string
	HardwareModel              string
	OSVersion                  string
	Vendor                     string
	DeviceType                 domain.DeviceType
	Interfaces                 []domain.Interface
	Neighbors                  []snmp.NeighborInfo
	NeighborDiscoveryProtocols []domain.DiscoveryProtocol
	NeighborDiscoveryFailures  []snmp.NeighborDiscoveryFailure
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
	if shouldPromoteDiscoveredHostname(fresh.Hostname, fresh.IP, input.SysName) {
		fresh.Hostname = strings.TrimSpace(input.SysName)
	}
	fresh.SysDescr = input.SysDescr
	fresh.SysObjectID = input.SysObjectID
	fresh.HardwareModel = input.HardwareModel
	if input.OSVersion != "" {
		fresh.OSVersion = input.OSVersion
	}
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
		materialized, currentUnknowns, currentTotals, materializeErr := s.applyDiscoveryViaObservationStore(
			*fresh,
			neighbors,
			input.NeighborDiscoveryProtocols,
			input.NeighborDiscoveryFailures,
		)
		if materializeErr != nil {
			return StaticPersistenceResult{}, materializeErr
		}
		result.TopologyChanged = result.TopologyChanged || materialized.TopologyChanged
		result.LinksCreated += materialized.LinksCreated
		unknownNeighbors = currentUnknowns
		unknownByProtocol = currentTotals
	} else {
		for _, neighbor := range neighbors {
			normalizedIdentity := discoveredNeighborRemoteIdentity(neighbor)
			if normalizedIdentity == "" {
				continue
			}

			var remoteDevice *domain.Device
			if strings.TrimSpace(neighbor.RemoteSysName) != "" {
				var lookupErr error
				remoteDevice, lookupErr = s.deviceRepo.GetBySysName(neighbor.RemoteSysName)
				if lookupErr != nil {
					log.Printf("Error looking up neighbor %s: %v", neighbor.RemoteSysName, lookupErr)
					continue
				}
			}
			if remoteDevice == nil {
				unknownNeighbors[unknownNeighborKey{
					RemoteIdentity: unknownNeighborIdentity(neighbor, normalizedIdentity),
					Protocol:       neighbor.Protocol,
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
	s.syncTopologyDiscoveryMetadata(fresh.ID, len(neighbors), false, snmp.HasCriticalNeighborDiscoveryFailure(input.NeighborDiscoveryFailures))
	s.reconcileResolvedBootstrapPeers(fresh.ID)

	return result, nil
}

func shouldPromoteDiscoveredHostname(currentHostname string, deviceIP string, discoveredSysName string) bool {
	discovered := strings.TrimSpace(discoveredSysName)
	if discovered == "" {
		return false
	}
	current := strings.TrimSpace(currentHostname)
	if current == "" {
		return true
	}
	return current == strings.TrimSpace(deviceIP)
}

func (s *DeviceService) applyDiscoveryViaObservationStore(
	fresh domain.Device,
	neighbors []snmp.NeighborInfo,
	attemptedProtocols []domain.DiscoveryProtocol,
	failures []snmp.NeighborDiscoveryFailure,
) (StaticPersistenceResult, map[unknownNeighborKey]int, map[domain.DiscoveryProtocol]int, error) {
	if s.topologyStore == nil {
		return StaticPersistenceResult{}, nil, nil, nil
	}

	now := time.Now().UTC()
	reconciledProtocols := reconciledNeighborDiscoveryProtocols(attemptedProtocols, neighbors, failures)
	affectedDeviceIDs := map[uuid.UUID]struct{}{
		fresh.ID: {},
	}
	unknownNeighbors := make(map[unknownNeighborKey]int)
	unknownByProtocol := make(map[domain.DiscoveryProtocol]int)
	currentObservations := make([]topology.Observation, 0, len(neighbors))

	for _, neighbor := range neighbors {
		normalizedIdentity := discoveredNeighborRemoteIdentity(neighbor)
		if normalizedIdentity == "" {
			continue
		}

		var remoteDevice *domain.Device
		if strings.TrimSpace(neighbor.RemoteSysName) != "" {
			var lookupErr error
			remoteDevice, lookupErr = s.deviceRepo.GetBySysName(neighbor.RemoteSysName)
			if lookupErr != nil {
				return StaticPersistenceResult{}, nil, nil, fmt.Errorf("looking up neighbor %s: %w", neighbor.RemoteSysName, lookupErr)
			}
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
		currentObservations = append(currentObservations, *observation)

		if remoteDevice == nil {
			unknownNeighbors[unknownNeighborKey{
				RemoteIdentity: unknownNeighborIdentity(neighbor, normalizedIdentity),
				Protocol:       neighbor.Protocol,
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

	pruneProtocols := pruneSafeNeighborDiscoveryProtocols(reconciledProtocols, currentObservations)

	prunedObservations := 0
	if len(pruneProtocols) > 0 {
		var pruneErr error
		prunedObservations, pruneErr = s.topologyStore.PruneLocalObservations(
			fresh.ID,
			pruneProtocols,
			currentObservations,
		)
		if pruneErr != nil {
			return StaticPersistenceResult{}, nil, nil, fmt.Errorf("pruning stale topology observations: %w", pruneErr)
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

	applied, err := topology.ApplyObservations(
		materializableTopologyObservations(observations),
		linkWriterAdapter{repo: s.linkRepo},
	)
	if err != nil {
		return StaticPersistenceResult{}, nil, nil, fmt.Errorf("materializing canonical links: %w", err)
	}

	deletedStaleLinks, err := s.deleteStaleAutoDiscoveredLinks(
		fresh.ID,
		pruneProtocols,
		applied.Events,
		currentObservations,
	)
	if err != nil {
		return StaticPersistenceResult{}, nil, nil, err
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
		TopologyChanged: applied.TopologyChanged || prunedObservations > 0 || deletedStaleLinks > 0,
		LinksCreated:    applied.LinksCreated,
	}, unknownNeighbors, unknownByProtocol, nil
}

func reconciledNeighborDiscoveryProtocols(
	attemptedProtocols []domain.DiscoveryProtocol,
	neighbors []snmp.NeighborInfo,
	failures []snmp.NeighborDiscoveryFailure,
) []domain.DiscoveryProtocol {
	attempted := make(map[domain.DiscoveryProtocol]struct{})
	for _, protocol := range attemptedProtocols {
		if isReconcilableDiscoveryProtocol(protocol) {
			attempted[protocol] = struct{}{}
		}
	}
	if len(attempted) == 0 {
		for _, neighbor := range neighbors {
			if isReconcilableDiscoveryProtocol(neighbor.Protocol) {
				attempted[neighbor.Protocol] = struct{}{}
			}
		}
	}

	failed := make(map[domain.DiscoveryProtocol]struct{})
	for _, failure := range failures {
		if isReconcilableDiscoveryProtocol(failure.Protocol) {
			failed[failure.Protocol] = struct{}{}
		}
	}

	protocols := make([]domain.DiscoveryProtocol, 0, 2)
	for _, protocol := range []domain.DiscoveryProtocol{domain.DiscoveryProtocolLLDP, domain.DiscoveryProtocolCDP} {
		if _, ok := attempted[protocol]; !ok {
			continue
		}
		if _, ok := failed[protocol]; ok {
			continue
		}
		protocols = append(protocols, protocol)
	}
	return protocols
}

func pruneSafeNeighborDiscoveryProtocols(
	protocols []domain.DiscoveryProtocol,
	currentObservations []topology.Observation,
) []domain.DiscoveryProtocol {
	unsafe := make(map[domain.DiscoveryProtocol]struct{})
	for _, observation := range currentObservations {
		if !isAmbiguousResolvedDiscoveryObservation(observation) {
			continue
		}
		unsafe[observation.Protocol] = struct{}{}
	}
	if len(unsafe) == 0 {
		return protocols
	}

	filtered := make([]domain.DiscoveryProtocol, 0, len(protocols))
	for _, protocol := range protocols {
		if _, ok := unsafe[protocol]; ok {
			continue
		}
		filtered = append(filtered, protocol)
	}
	return filtered
}

func materializableTopologyObservations(observations []topology.Observation) []topology.Observation {
	filtered := make([]topology.Observation, 0, len(observations))
	for _, observation := range observations {
		if isAmbiguousResolvedDiscoveryObservation(observation) {
			continue
		}
		filtered = append(filtered, observation)
	}
	return filtered
}

func isAmbiguousResolvedDiscoveryObservation(observation topology.Observation) bool {
	return observation.RemoteDeviceID != uuid.Nil &&
		isReconcilableDiscoveryProtocol(observation.Protocol) &&
		strings.TrimSpace(observation.LocalPort) == "" &&
		strings.TrimSpace(observation.RemotePort) == ""
}

func isReconcilableDiscoveryProtocol(protocol domain.DiscoveryProtocol) bool {
	return protocol == domain.DiscoveryProtocolLLDP || protocol == domain.DiscoveryProtocolCDP
}

func (s *DeviceService) deleteStaleAutoDiscoveredLinks(
	localDeviceID uuid.UUID,
	reconciledProtocols []domain.DiscoveryProtocol,
	materializedEvents []topology.ApplyEvent,
	currentObservations []topology.Observation,
) (int, error) {
	if s.linkRepo == nil || len(reconciledProtocols) == 0 {
		return 0, nil
	}

	protocolSet := make(map[domain.DiscoveryProtocol]struct{}, len(reconciledProtocols))
	for _, protocol := range reconciledProtocols {
		if isReconcilableDiscoveryProtocol(protocol) {
			protocolSet[protocol] = struct{}{}
		}
	}
	if len(protocolSet) == 0 {
		return 0, nil
	}

	supportedLinkIDs := make(map[uuid.UUID]struct{})
	supportedLinks := make(map[string]struct{})
	for _, event := range materializedEvents {
		link := event.Link
		if !isReconcilableDiscoveryProtocol(link.DiscoveryProtocol) {
			continue
		}
		if link.ID != uuid.Nil {
			supportedLinkIDs[link.ID] = struct{}{}
		}
		supportedLinks[physicalLinkKey(link)] = struct{}{}
	}

	links, err := s.linkRepo.GetByDeviceID(localDeviceID)
	if err != nil {
		return 0, fmt.Errorf("listing links for stale discovery reconciliation: %w", err)
	}

	deleted := 0
	for _, link := range links {
		if link.DiscoveryProtocol == domain.DiscoveryProtocolManual {
			continue
		}
		if _, ok := protocolSet[link.DiscoveryProtocol]; !ok {
			continue
		}
		if _, ok := supportedLinkIDs[link.ID]; ok {
			continue
		}
		if _, ok := supportedLinks[physicalLinkKey(link)]; ok {
			continue
		}
		if currentUnresolvedObservationSupportsLink(localDeviceID, link, currentObservations) {
			continue
		}
		if err := s.linkRepo.Delete(link.ID); err != nil {
			return deleted, fmt.Errorf("deleting stale auto-discovered link %s: %w", link.ID, err)
		}
		deleted++
	}
	return deleted, nil
}

func currentUnresolvedObservationSupportsLink(
	localDeviceID uuid.UUID,
	link domain.Link,
	currentObservations []topology.Observation,
) bool {
	localPort, remotePort, ok := linkPortsForLocalEndpoint(localDeviceID, link)
	if !ok || (strings.TrimSpace(localPort) == "" && strings.TrimSpace(remotePort) == "") {
		return false
	}

	for _, observation := range currentObservations {
		if observation.LocalDeviceID != localDeviceID {
			continue
		}
		if observation.RemoteDeviceID != uuid.Nil {
			continue
		}
		if observation.Protocol != link.DiscoveryProtocol {
			continue
		}
		if strings.TrimSpace(observation.LocalPort) != "" {
			if strings.TrimSpace(localPort) == "" || !sameDiscoveryPort(observation.LocalPort, localPort) {
				continue
			}
			if strings.TrimSpace(observation.RemotePort) == "" {
				return true
			}
			if sameDiscoveryPort(observation.RemotePort, remotePort) {
				return true
			}
			continue
		}
		if strings.TrimSpace(observation.RemotePort) != "" &&
			strings.TrimSpace(remotePort) != "" &&
			sameDiscoveryPort(observation.RemotePort, remotePort) {
			return true
		}
	}
	return false
}

func linkPortsForLocalEndpoint(localDeviceID uuid.UUID, link domain.Link) (string, string, bool) {
	if link.SourceDeviceID == localDeviceID {
		return link.SourceIfName, link.TargetIfName, true
	}
	if link.TargetDeviceID == localDeviceID {
		return link.TargetIfName, link.SourceIfName, true
	}
	return "", "", false
}

func sameDiscoveryPort(left, right string) bool {
	return strings.EqualFold(strings.TrimSpace(left), strings.TrimSpace(right))
}

func physicalLinkKey(link domain.Link) string {
	source := physicalEndpointKey(link.SourceDeviceID, link.SourceIfName)
	target := physicalEndpointKey(link.TargetDeviceID, link.TargetIfName)
	if target < source {
		source, target = target, source
	}
	return source + "<->" + target
}

func physicalEndpointKey(deviceID uuid.UUID, port string) string {
	return deviceID.String() + "|" + strings.ToLower(strings.TrimSpace(port))
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
	RemoteIdentity string
	Protocol       domain.DiscoveryProtocol
}

func countNeighborsByProtocol(neighbors []snmp.NeighborInfo) map[domain.DiscoveryProtocol]int {
	counts := make(map[domain.DiscoveryProtocol]int)
	for _, neighbor := range neighbors {
		if discoveredNeighborRemoteIdentity(neighbor) == "" {
			continue
		}
		counts[neighbor.Protocol]++
	}
	return counts
}

func discoveredNeighborRemoteIdentity(neighbor snmp.NeighborInfo) string {
	if strings.TrimSpace(neighbor.RemoteSysName) != "" {
		return topology.NormalizeRemoteIdentity(neighbor.RemoteSysName)
	}
	return strings.ToLower(strings.TrimSpace(neighbor.RemoteChassisID))
}

func unknownNeighborIdentity(neighbor snmp.NeighborInfo, normalizedIdentity string) string {
	if strings.TrimSpace(neighbor.RemoteSysName) != "" {
		return neighbor.RemoteSysName
	}
	return normalizedIdentity
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
		return details[i].key.RemoteIdentity < details[j].key.RemoteIdentity
	})

	parts := make([]string, 0, len(details))
	for index, item := range details {
		if index == 5 {
			parts = append(parts, fmt.Sprintf("... +%d more", len(details)-index))
			break
		}
		parts = append(parts, fmt.Sprintf("%s(%s)x%d", item.key.RemoteIdentity, item.key.Protocol, item.count))
	}

	log.Printf("Static discovery for %s observed off-map neighbors [%s]: %s",
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
