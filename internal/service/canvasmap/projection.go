package canvasmap

import (
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/domain"
)

// TopologyProjection is the saved-map structural view after applying a map
// filter or materialized membership.
type TopologyProjection struct {
	Devices      []domain.Device
	Links        []domain.Link
	GhostDevices []domain.Device
}

type CreatePlan struct {
	Filter                domain.CanvasMapFilter
	PersistedSourceAreaID *uuid.UUID
	SourceMapID           *uuid.UUID
	CreateEmptyMembership bool
}

var ErrDefaultMapDelete = errors.New("cannot delete default canvas map")

// ValidateDelete rejects attempts to delete the default saved map.
func ValidateDelete(canvasMap domain.CanvasMap) error {
	if canvasMap.IsDefault {
		return ErrDefaultMapDelete
	}
	return nil
}

// ProjectionFilterForMap returns the filter used to project a persisted canvas map.
func ProjectionFilterForMap(canvasMap domain.CanvasMap) domain.CanvasMapFilter {
	filter, err := domain.ParseCanvasMapFilter(canvasMap.FilterJSON)
	if err != nil {
		filter, _ = domain.ParseCanvasMapFilter("")
	}
	if filter.AreaID == nil && canvasMap.SourceAreaID != nil {
		areaID := *canvasMap.SourceAreaID
		filter.AreaID = &areaID
	}
	return filter
}

// MaterializationFilter returns the filter persisted for a newly materialized map.
func MaterializationFilter(filter domain.CanvasMapFilter, sourceAreaID *uuid.UUID) domain.CanvasMapFilter {
	if filter.AreaID == nil && sourceAreaID != nil {
		areaID := *sourceAreaID
		filter.AreaID = &areaID
	}
	return filter
}

// PlanCreate captures saved-map materialization decisions for a create request.
func PlanCreate(filter domain.CanvasMapFilter, sourceAreaID *uuid.UUID, sourceMapID *uuid.UUID) CreatePlan {
	materializationFilter := MaterializationFilter(filter, sourceAreaID)
	persistedSourceAreaID := sourceAreaID
	if sourceMapID != nil {
		persistedSourceAreaID = nil
	}
	return CreatePlan{
		Filter:                materializationFilter,
		PersistedSourceAreaID: persistedSourceAreaID,
		SourceMapID:           sourceMapID,
		CreateEmptyMembership: ShouldCreateEmptyMembership(filter, sourceAreaID),
	}
}

// ShouldCreateEmptyMembership reports whether map creation should persist an
// intentionally empty materialized membership instead of projecting all devices.
func ShouldCreateEmptyMembership(filter domain.CanvasMapFilter, sourceAreaID *uuid.UUID) bool {
	return sourceAreaID == nil &&
		filter.AreaID == nil &&
		len(filter.DeviceIDs) == 0 &&
		len(filter.Tags) == 0
}

// ProjectTopologyForFilter applies a canvas map filter to a device/link topology.
func ProjectTopologyForFilter(
	devices []domain.Device,
	links []domain.Link,
	filter domain.CanvasMapFilter,
) TopologyProjection {
	knownDeviceIDs := make(map[uuid.UUID]struct{}, len(devices))
	baseDeviceIDs := make(map[uuid.UUID]struct{}, len(devices))
	selectedDeviceIDs := make(map[uuid.UUID]struct{}, len(filter.DeviceIDs))
	for _, deviceID := range filter.DeviceIDs {
		selectedDeviceIDs[deviceID] = struct{}{}
	}

	projection := TopologyProjection{
		Devices: make([]domain.Device, 0, len(devices)),
		Links:   make([]domain.Link, 0, len(links)),
	}
	for _, device := range devices {
		knownDeviceIDs[device.ID] = struct{}{}

		baseDevice := false
		switch {
		case len(selectedDeviceIDs) > 0:
			_, baseDevice = selectedDeviceIDs[device.ID]
		case filter.AreaID != nil:
			baseDevice = deviceHasArea(device, *filter.AreaID)
		default:
			baseDevice = true
		}
		if !baseDevice || !deviceMatchesTags(device, filter.Tags) {
			continue
		}

		projection.Devices = append(projection.Devices, device)
		baseDeviceIDs[device.ID] = struct{}{}
	}

	ghostDeviceIDs := make(map[uuid.UUID]struct{})
	for _, link := range links {
		_, sourceKnown := knownDeviceIDs[link.SourceDeviceID]
		_, targetKnown := knownDeviceIDs[link.TargetDeviceID]
		if !sourceKnown || !targetKnown {
			continue
		}

		_, sourceIsBase := baseDeviceIDs[link.SourceDeviceID]
		_, targetIsBase := baseDeviceIDs[link.TargetDeviceID]
		includeLink := sourceIsBase && targetIsBase
		if filter.IncludeCrossAreaLinks && (sourceIsBase || targetIsBase) {
			includeLink = true
		}
		if !includeLink {
			continue
		}

		projection.Links = append(projection.Links, link)
		if !filter.IncludeGhostDevices {
			continue
		}
		if sourceIsBase && !targetIsBase {
			ghostDeviceIDs[link.TargetDeviceID] = struct{}{}
		}
		if targetIsBase && !sourceIsBase {
			ghostDeviceIDs[link.SourceDeviceID] = struct{}{}
		}
	}

	if filter.IncludeGhostDevices && len(ghostDeviceIDs) > 0 {
		projection.GhostDevices = make([]domain.Device, 0, len(ghostDeviceIDs))
		for _, device := range devices {
			if _, isBase := baseDeviceIDs[device.ID]; isBase {
				continue
			}
			if _, isGhost := ghostDeviceIDs[device.ID]; isGhost {
				projection.GhostDevices = append(projection.GhostDevices, device)
			}
		}
	}

	return projection
}

// MaterializeMembership builds the persisted map membership for a source topology.
func MaterializeMembership(
	devices []domain.Device,
	links []domain.Link,
	areas []domain.AreaWithCount,
	filter domain.CanvasMapFilter,
) domain.CanvasMapMembership {
	projection := ProjectTopologyForFilter(devices, links, filter)
	areaMemberships := areasForMembership(areas, projection.Devices, filter)
	includedAreaIDs := areaWithCountIDSet(areaMemberships)
	membership := domain.CanvasMapMembership{
		Devices: make([]domain.CanvasMapDeviceMembership, 0, len(projection.Devices)+len(projection.GhostDevices)),
		LinkIDs: make([]uuid.UUID, 0, len(projection.Links)),
		Areas:   make([]domain.CanvasMapAreaMembership, 0, len(areas)),
	}

	for _, device := range projection.Devices {
		membership.Devices = append(membership.Devices, domain.CanvasMapDeviceMembership{
			DeviceID: device.ID,
			Role:     domain.CanvasMapDeviceRoleBase,
			AreaIDs:  filterDeviceAreaIDs(device.AreaIDs, includedAreaIDs),
		})
	}
	for _, device := range projection.GhostDevices {
		membership.Devices = append(membership.Devices, domain.CanvasMapDeviceMembership{
			DeviceID: device.ID,
			Role:     domain.CanvasMapDeviceRoleGhost,
		})
	}
	for _, link := range projection.Links {
		membership.LinkIDs = append(membership.LinkIDs, link.ID)
	}
	for _, area := range areaMemberships {
		membership.Areas = append(membership.Areas, domain.CanvasMapAreaMembership{
			AreaID:      area.ID,
			Name:        area.Name,
			Description: area.Description,
			Color:       area.Color,
		})
	}

	return membership
}

// MaterializeMembershipFromSourceMap builds a persisted membership from another saved map.
func MaterializeMembershipFromSourceMap(
	devices []domain.Device,
	links []domain.Link,
	sourceMembership domain.CanvasMapMembership,
	areas []domain.CanvasMapAreaMembership,
	filter domain.CanvasMapFilter,
) domain.CanvasMapMembership {
	sourceProjection := ProjectTopologyForMembership(devices, links, sourceMembership)
	sourceDevices := append([]domain.Device{}, sourceProjection.Devices...)
	sourceDevices = append(sourceDevices, sourceProjection.GhostDevices...)
	projection := ProjectTopologyForFilter(sourceDevices, sourceProjection.Links, filter)
	areaMemberships := areaSnapshotsForMembership(areas, projection.Devices, filter)
	includedAreaIDs := areaSnapshotIDSet(areaMemberships)
	membership := domain.CanvasMapMembership{
		Devices: make([]domain.CanvasMapDeviceMembership, 0, len(projection.Devices)+len(projection.GhostDevices)),
		LinkIDs: make([]uuid.UUID, 0, len(projection.Links)),
		Areas:   make([]domain.CanvasMapAreaMembership, 0, len(areas)),
	}

	for _, device := range projection.Devices {
		membership.Devices = append(membership.Devices, domain.CanvasMapDeviceMembership{
			DeviceID: device.ID,
			Role:     domain.CanvasMapDeviceRoleBase,
			AreaIDs:  filterDeviceAreaIDs(device.AreaIDs, includedAreaIDs),
		})
	}
	for _, device := range projection.GhostDevices {
		membership.Devices = append(membership.Devices, domain.CanvasMapDeviceMembership{
			DeviceID: device.ID,
			Role:     domain.CanvasMapDeviceRoleGhost,
		})
	}
	for _, link := range projection.Links {
		membership.LinkIDs = append(membership.LinkIDs, link.ID)
	}
	membership.Areas = append(membership.Areas, areaMemberships...)

	return membership
}

// AreasWithCountToMembership converts global area rows into saved-map snapshots.
func AreasWithCountToMembership(areas []domain.AreaWithCount) []domain.CanvasMapAreaMembership {
	areaRows := make([]domain.Area, 0, len(areas))
	for _, area := range areas {
		areaRows = append(areaRows, area.Area)
	}
	return AreasToMembership(areaRows)
}

// AreasToMembership converts area rows into saved-map snapshots.
func AreasToMembership(areas []domain.Area) []domain.CanvasMapAreaMembership {
	snapshots := make([]domain.CanvasMapAreaMembership, 0, len(areas))
	for _, area := range areas {
		snapshots = append(snapshots, domain.CanvasMapAreaMembership{
			AreaID:      area.ID,
			Name:        area.Name,
			Description: area.Description,
			Color:       area.Color,
		})
	}
	return snapshots
}

// BaseDeviceMembership returns the saved-map membership row for an added base device.
func BaseDeviceMembership(device domain.Device) domain.CanvasMapDeviceMembership {
	return domain.CanvasMapDeviceMembership{
		DeviceID: device.ID,
		Role:     domain.CanvasMapDeviceRoleBase,
		AreaIDs:  append([]uuid.UUID(nil), device.AreaIDs...),
	}
}

// ShouldCopyDefaultPositions reports whether a new materialized map should copy
// positions from the current default map.
func ShouldCopyDefaultPositions(mapID uuid.UUID, defaultMapID uuid.UUID) bool {
	return mapID != defaultMapID
}

// DefaultPositionCandidates returns default-map positions, falling back to
// legacy canvas positions only when the default map has no saved positions.
func DefaultPositionCandidates(defaultPositions []domain.DevicePosition, legacyPositions []domain.DevicePosition) []domain.DevicePosition {
	if len(defaultPositions) > 0 {
		return defaultPositions
	}
	return legacyPositions
}

// DefaultPositionsForMembership returns copyable default positions for map members.
func DefaultPositionsForMembership(
	candidates []domain.DevicePosition,
	devices []domain.CanvasMapDeviceMembership,
) []domain.DevicePosition {
	return FilterPositionsForMemberDevices(candidates, devices)
}

// RemapPositionsForDeviceClones moves positions from original device IDs to
// their clone IDs and keeps only positions still present in membership.
func RemapPositionsForDeviceClones(
	positions []domain.DevicePosition,
	clonedDeviceIDs map[uuid.UUID]uuid.UUID,
	members []domain.CanvasMapDeviceMembership,
) []domain.DevicePosition {
	memberIDs := make(map[uuid.UUID]struct{}, len(members))
	for _, member := range members {
		memberIDs[member.DeviceID] = struct{}{}
	}

	nextPositions := make([]domain.DevicePosition, 0, len(positions))
	for _, position := range positions {
		if cloneID, ok := clonedDeviceIDs[position.DeviceID]; ok {
			position.DeviceID = cloneID
		}
		if _, ok := memberIDs[position.DeviceID]; ok {
			nextPositions = append(nextPositions, position)
		}
	}
	return nextPositions
}

// RemapLinkForDeviceClones returns a link with cloned endpoint IDs applied.
func RemapLinkForDeviceClones(
	link domain.Link,
	clonedDeviceIDs map[uuid.UUID]uuid.UUID,
) (domain.Link, bool) {
	cloned := false
	if cloneID, ok := clonedDeviceIDs[link.SourceDeviceID]; ok {
		link.SourceDeviceID = cloneID
		cloned = true
	}
	if cloneID, ok := clonedDeviceIDs[link.TargetDeviceID]; ok {
		link.TargetDeviceID = cloneID
		cloned = true
	}
	return link, cloned
}

// ProjectTopologyForMembership applies a materialized map membership to a topology.
func ProjectTopologyForMembership(
	devices []domain.Device,
	links []domain.Link,
	membership domain.CanvasMapMembership,
) TopologyProjection {
	deviceRoles := make(map[uuid.UUID]domain.CanvasMapDeviceRole, len(membership.Devices))
	deviceAreaIDs := make(map[uuid.UUID][]uuid.UUID, len(membership.Devices))
	for _, device := range membership.Devices {
		deviceRoles[device.DeviceID] = device.Role
		deviceAreaIDs[device.DeviceID] = append([]uuid.UUID(nil), device.AreaIDs...)
	}

	linkIDs := make(map[uuid.UUID]struct{}, len(membership.LinkIDs))
	for _, linkID := range membership.LinkIDs {
		linkIDs[linkID] = struct{}{}
	}

	projection := TopologyProjection{
		Devices:      []domain.Device{},
		Links:        []domain.Link{},
		GhostDevices: []domain.Device{},
	}
	for _, device := range devices {
		role, ok := deviceRoles[device.ID]
		if !ok {
			continue
		}
		device.AreaIDs = append([]uuid.UUID(nil), deviceAreaIDs[device.ID]...)
		if role == domain.CanvasMapDeviceRoleGhost {
			projection.GhostDevices = append(projection.GhostDevices, device)
			continue
		}
		projection.Devices = append(projection.Devices, device)
	}

	for _, link := range links {
		if _, ok := linkIDs[link.ID]; !ok {
			continue
		}
		if _, ok := deviceRoles[link.SourceDeviceID]; !ok {
			continue
		}
		if _, ok := deviceRoles[link.TargetDeviceID]; !ok {
			continue
		}
		projection.Links = append(projection.Links, link)
	}

	return projection
}

// MembershipDeviceIDs returns device IDs from materialized membership rows.
func MembershipDeviceIDs(devices []domain.CanvasMapDeviceMembership) []uuid.UUID {
	if len(devices) == 0 {
		return []uuid.UUID{}
	}
	ids := make([]uuid.UUID, 0, len(devices))
	for _, device := range devices {
		ids = append(ids, device.DeviceID)
	}
	return ids
}

// ConnectedBaseLinkIDs returns links connecting an added device to base members.
func ConnectedBaseLinkIDs(
	deviceID uuid.UUID,
	membership domain.CanvasMapMembership,
	links []domain.Link,
) []uuid.UUID {
	baseDeviceIDs := make(map[uuid.UUID]struct{}, len(membership.Devices)+1)
	for _, member := range membership.Devices {
		if member.Role == domain.CanvasMapDeviceRoleBase {
			baseDeviceIDs[member.DeviceID] = struct{}{}
		}
	}
	baseDeviceIDs[deviceID] = struct{}{}

	seen := make(map[uuid.UUID]struct{}, len(links))
	linkIDs := make([]uuid.UUID, 0, len(links))
	for _, link := range links {
		if _, ok := baseDeviceIDs[link.SourceDeviceID]; !ok {
			continue
		}
		if _, ok := baseDeviceIDs[link.TargetDeviceID]; !ok {
			continue
		}
		if _, ok := seen[link.ID]; ok {
			continue
		}
		seen[link.ID] = struct{}{}
		linkIDs = append(linkIDs, link.ID)
	}
	return linkIDs
}

// MissingLinkIDs returns candidate links that are not already present.
func MissingLinkIDs(existing []uuid.UUID, candidates []uuid.UUID) []uuid.UUID {
	if len(candidates) == 0 {
		return []uuid.UUID{}
	}
	known := make(map[uuid.UUID]struct{}, len(existing))
	for _, id := range existing {
		known[id] = struct{}{}
	}
	missing := make([]uuid.UUID, 0, len(candidates))
	for _, id := range candidates {
		if _, ok := known[id]; ok {
			continue
		}
		missing = append(missing, id)
	}
	return missing
}

// HasDuplicateDeviceAddress reports whether another map member already has the
// device address being added.
func HasDuplicateDeviceAddress(device domain.Device, existing []domain.Device) bool {
	address := NormalizeDeviceAddress(device.IP)
	if address == "" {
		return false
	}
	for _, existingDevice := range existing {
		if existingDevice.ID == device.ID {
			continue
		}
		if NormalizeDeviceAddress(existingDevice.IP) == address {
			return true
		}
	}
	return false
}

func NormalizeDeviceAddress(address string) string {
	return strings.ToLower(strings.TrimSpace(address))
}

func DuplicateDeviceAddressMessage(address string) string {
	address = strings.TrimSpace(address)
	if address == "" {
		return "a device with that address already exists in this map"
	}
	return fmt.Sprintf("a device with IP/host %q already exists in this map", address)
}

// AreaMembershipToAreas converts saved-map area snapshots to area rows with map-local counts.
func AreaMembershipToAreas(
	areas []domain.CanvasMapAreaMembership,
	baseDevices []domain.Device,
) []domain.AreaWithCount {
	response := make([]domain.AreaWithCount, 0, len(areas))
	for _, area := range areas {
		response = append(response, domain.AreaWithCount{
			Area: domain.Area{
				ID:          area.AreaID,
				Name:        area.Name,
				Description: area.Description,
				Color:       area.Color,
			},
			DeviceCount: areaDeviceCount(area.AreaID, baseDevices),
		})
	}
	return response
}

// FilterPositionsForDevices keeps positions for the supplied devices.
func FilterPositionsForDevices(
	positions []domain.DevicePosition,
	devices []domain.Device,
) []domain.DevicePosition {
	if len(positions) == 0 || len(devices) == 0 {
		return []domain.DevicePosition{}
	}

	deviceIDs := make(map[uuid.UUID]struct{}, len(devices))
	for _, device := range devices {
		deviceIDs[device.ID] = struct{}{}
	}

	filtered := make([]domain.DevicePosition, 0, len(positions))
	for _, position := range positions {
		if _, ok := deviceIDs[position.DeviceID]; ok {
			filtered = append(filtered, position)
		}
	}
	return filtered
}

// FilterPositionsForMemberDevices keeps positions for the supplied membership devices.
func FilterPositionsForMemberDevices(
	positions []domain.DevicePosition,
	devices []domain.CanvasMapDeviceMembership,
) []domain.DevicePosition {
	if len(positions) == 0 || len(devices) == 0 {
		return []domain.DevicePosition{}
	}

	deviceIDs := make(map[uuid.UUID]struct{}, len(devices))
	for _, device := range devices {
		deviceIDs[device.DeviceID] = struct{}{}
	}

	filtered := make([]domain.DevicePosition, 0, len(positions))
	for _, position := range positions {
		if _, ok := deviceIDs[position.DeviceID]; ok {
			filtered = append(filtered, position)
		}
	}
	return filtered
}

func areaWithCountIDSet(areas []domain.AreaWithCount) map[uuid.UUID]struct{} {
	ids := make(map[uuid.UUID]struct{}, len(areas))
	for _, area := range areas {
		ids[area.ID] = struct{}{}
	}
	return ids
}

func areaSnapshotIDSet(areas []domain.CanvasMapAreaMembership) map[uuid.UUID]struct{} {
	ids := make(map[uuid.UUID]struct{}, len(areas))
	for _, area := range areas {
		ids[area.AreaID] = struct{}{}
	}
	return ids
}

func filterDeviceAreaIDs(areaIDs []uuid.UUID, included map[uuid.UUID]struct{}) []uuid.UUID {
	if len(areaIDs) == 0 || len(included) == 0 {
		return []uuid.UUID{}
	}
	filtered := make([]uuid.UUID, 0, len(areaIDs))
	for _, areaID := range areaIDs {
		if _, ok := included[areaID]; ok {
			filtered = append(filtered, areaID)
		}
	}
	return filtered
}

func areasForMembership(
	areas []domain.AreaWithCount,
	baseDevices []domain.Device,
	filter domain.CanvasMapFilter,
) []domain.AreaWithCount {
	includedAreaIDs := make(map[uuid.UUID]struct{})
	if filter.AreaID != nil {
		includedAreaIDs[*filter.AreaID] = struct{}{}
	} else {
		for _, device := range baseDevices {
			for _, areaID := range device.AreaIDs {
				includedAreaIDs[areaID] = struct{}{}
			}
		}
	}

	filtered := make([]domain.AreaWithCount, 0, len(includedAreaIDs))
	for _, area := range areas {
		if _, ok := includedAreaIDs[area.ID]; ok {
			filtered = append(filtered, area)
		}
	}
	return filtered
}

func areaSnapshotsForMembership(
	areas []domain.CanvasMapAreaMembership,
	baseDevices []domain.Device,
	filter domain.CanvasMapFilter,
) []domain.CanvasMapAreaMembership {
	includedAreaIDs := make(map[uuid.UUID]struct{})
	if filter.AreaID != nil {
		includedAreaIDs[*filter.AreaID] = struct{}{}
	} else {
		for _, device := range baseDevices {
			for _, areaID := range device.AreaIDs {
				includedAreaIDs[areaID] = struct{}{}
			}
		}
	}

	filtered := make([]domain.CanvasMapAreaMembership, 0, len(includedAreaIDs))
	for _, area := range areas {
		if _, ok := includedAreaIDs[area.AreaID]; ok {
			filtered = append(filtered, area)
		}
	}
	return filtered
}

func areaDeviceCount(areaID uuid.UUID, devices []domain.Device) int {
	count := 0
	for _, device := range devices {
		if deviceHasArea(device, areaID) {
			count++
		}
	}
	return count
}

func deviceHasArea(device domain.Device, areaID uuid.UUID) bool {
	for _, deviceAreaID := range device.AreaIDs {
		if deviceAreaID == areaID {
			return true
		}
	}
	return false
}

func deviceMatchesTags(device domain.Device, tags map[string]string) bool {
	for key, expected := range tags {
		actual, ok := device.Tags[key]
		if !ok || actual != expected {
			return false
		}
	}
	return true
}
