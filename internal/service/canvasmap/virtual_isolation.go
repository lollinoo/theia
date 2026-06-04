package canvasmap

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/domain"
)

type VirtualIsolationMapRepository interface {
	List() ([]domain.CanvasMap, error)
	GetMembership(uuid.UUID) (domain.CanvasMapMembership, error)
	ReplaceMembership(uuid.UUID, domain.CanvasMapMembership) error
}

type VirtualIsolationPositionRepository interface {
	GetAllForMap(uuid.UUID) ([]domain.DevicePosition, error)
	SaveAllForMap(uuid.UUID, []domain.DevicePosition) error
}

type VirtualDeviceCloneUpdate struct {
	PollIntervalOverride **int
	PollingEnabled       *bool
}

type VirtualIsolationDeviceService interface {
	GetDevicesByIDs(context.Context, []uuid.UUID) ([]domain.Device, error)
	AddDevice(
		context.Context,
		string,
		string,
		domain.DeviceType,
		domain.SNMPCredentials,
		map[string]string,
		string,
		domain.MetricsSource,
		string,
		string,
		domain.TopologyDiscoveryMode,
		[]uuid.UUID,
		...*string,
	) (*domain.Device, error)
	UpdateClonedVirtualDevice(context.Context, uuid.UUID, VirtualDeviceCloneUpdate) error
	GetDevice(context.Context, uuid.UUID) (*domain.Device, error)
}

type VirtualIsolationLinkRepository interface {
	Create(*domain.Link) error
	GetAll() ([]domain.Link, error)
}

type VirtualIsolationDeps struct {
	Maps      VirtualIsolationMapRepository
	Positions VirtualIsolationPositionRepository
	Devices   VirtualIsolationDeviceService
	Links     VirtualIsolationLinkRepository
}

func IsolateVirtualDevices(
	ctx context.Context,
	mapID uuid.UUID,
	deps VirtualIsolationDeps,
) error {
	membership, err := deps.Maps.GetMembership(mapID)
	if err != nil {
		return fmt.Errorf("loading canvas map membership: %w", err)
	}
	if len(membership.Devices) == 0 {
		return nil
	}
	if deps.Devices == nil || deps.Links == nil {
		return fmt.Errorf("canvas map virtual device isolation dependencies unavailable")
	}

	devices, err := deps.Devices.GetDevicesByIDs(ctx, MembershipDeviceIDs(membership.Devices))
	if err != nil {
		return fmt.Errorf("loading canvas map devices: %w", err)
	}
	virtualMemberIDs, err := VirtualMemberDeviceIDs(membership, devices)
	if err != nil {
		return err
	}
	if len(virtualMemberIDs) == 0 {
		return nil
	}
	sharedVirtualIDs, err := sharedDeviceIDs(deps.Maps, mapID, virtualMemberIDs)
	if err != nil {
		return err
	}
	if len(sharedVirtualIDs) == 0 {
		return nil
	}

	cloneCandidates, err := VirtualDeviceCloneCandidates(membership, devices, sharedVirtualIDs)
	if err != nil {
		return err
	}
	if len(cloneCandidates) == 0 {
		return nil
	}

	clonedDeviceIDs := make(map[uuid.UUID]uuid.UUID, len(cloneCandidates))
	for _, device := range cloneCandidates {
		clone, err := cloneVirtualDevice(ctx, deps.Devices, device)
		if err != nil {
			return err
		}
		clonedDeviceIDs[device.ID] = clone.ID
	}
	if len(clonedDeviceIDs) == 0 {
		return nil
	}
	nextMembership := MembershipWithDeviceClones(membership, clonedDeviceIDs)

	links, err := LoadLinksByIDs(deps.Links, membership.LinkIDs)
	if err != nil {
		return fmt.Errorf("loading canvas map links: %w", err)
	}
	nextMembership.LinkIDs = make([]uuid.UUID, 0, len(links))
	for _, link := range links {
		nextLinkID, err := cloneLinkForVirtualDevices(deps.Links, link, clonedDeviceIDs)
		if err != nil {
			return err
		}
		nextMembership.LinkIDs = append(nextMembership.LinkIDs, nextLinkID)
	}

	positions, err := deps.Positions.GetAllForMap(mapID)
	if err != nil {
		return fmt.Errorf("loading canvas map positions: %w", err)
	}
	nextPositions := RemapPositionsForDeviceClones(
		positions,
		clonedDeviceIDs,
		nextMembership.Devices,
	)

	if err := deps.Maps.ReplaceMembership(mapID, nextMembership); err != nil {
		return fmt.Errorf("replacing canvas map membership with cloned virtual devices: %w", err)
	}
	if len(nextPositions) > 0 {
		if err := deps.Positions.SaveAllForMap(mapID, nextPositions); err != nil {
			return fmt.Errorf("saving cloned virtual device positions: %w", err)
		}
	}
	return nil
}

func LoadLinksByIDs(repo VirtualIsolationLinkRepository, ids []uuid.UUID) ([]domain.Link, error) {
	if len(ids) == 0 {
		return []domain.Link{}, nil
	}

	type linkBatchRepository interface {
		GetByIDs([]uuid.UUID) ([]domain.Link, error)
	}
	if batchRepo, ok := repo.(linkBatchRepository); ok {
		return batchRepo.GetByIDs(ids)
	}

	links, err := repo.GetAll()
	if err != nil {
		return nil, err
	}
	requested := make(map[uuid.UUID]struct{}, len(ids))
	for _, id := range ids {
		requested[id] = struct{}{}
	}
	filtered := links[:0]
	for _, link := range links {
		if _, ok := requested[link.ID]; ok {
			filtered = append(filtered, link)
		}
	}
	return filtered, nil
}

func sharedDeviceIDs(
	repo VirtualIsolationMapRepository,
	mapID uuid.UUID,
	deviceIDs map[uuid.UUID]struct{},
) (map[uuid.UUID]struct{}, error) {
	canvasMaps, err := repo.List()
	if err != nil {
		return nil, fmt.Errorf("listing canvas maps for virtual isolation: %w", err)
	}
	shared := make(map[uuid.UUID]struct{})
	for _, canvasMap := range canvasMaps {
		if canvasMap.ID == mapID {
			continue
		}
		membership, err := repo.GetMembership(canvasMap.ID)
		if err != nil {
			return nil, fmt.Errorf("loading canvas map %s membership for virtual isolation: %w", canvasMap.ID, err)
		}
		for _, member := range membership.Devices {
			if _, ok := deviceIDs[member.DeviceID]; ok {
				shared[member.DeviceID] = struct{}{}
			}
		}
	}
	return shared, nil
}

func cloneVirtualDevice(
	ctx context.Context,
	devices VirtualIsolationDeviceService,
	device domain.Device,
) (*domain.Device, error) {
	notes := cloneOptionalString(device.Notes)
	clone, err := devices.AddDevice(
		ctx,
		device.IP,
		device.Hostname,
		domain.DeviceTypeVirtual,
		domain.SNMPCredentials{},
		cloneStringMap(device.Tags),
		device.Vendor,
		domain.MetricsSourceNone,
		device.PrometheusLabelName,
		device.PrometheusLabelValue,
		device.TopologyDiscoveryMode,
		nil,
		notes,
	)
	if err != nil {
		return nil, fmt.Errorf("cloning virtual device %s: %w", device.ID, err)
	}

	update := VirtualDeviceCloneUpdate{PollIntervalOverride: clonePollIntervalOverrideForUpdate(device.PollIntervalOverride)}
	if device.PollingEnabled != nil {
		value := *device.PollingEnabled
		update.PollingEnabled = &value
	}
	if err := devices.UpdateClonedVirtualDevice(ctx, clone.ID, update); err != nil {
		return nil, fmt.Errorf("updating cloned virtual device %s: %w", clone.ID, err)
	}

	reloaded, err := devices.GetDevice(ctx, clone.ID)
	if err != nil {
		return nil, fmt.Errorf("loading cloned virtual device %s: %w", clone.ID, err)
	}
	return reloaded, nil
}

func cloneLinkForVirtualDevices(
	repo VirtualIsolationLinkRepository,
	link domain.Link,
	clonedDeviceIDs map[uuid.UUID]uuid.UUID,
) (uuid.UUID, error) {
	remapped, cloned := RemapLinkForDeviceClones(link, clonedDeviceIDs)
	if !cloned {
		return link.ID, nil
	}

	nextLink := &domain.Link{
		SourceDeviceID:    remapped.SourceDeviceID,
		SourceIfName:      remapped.SourceIfName,
		TargetDeviceID:    remapped.TargetDeviceID,
		TargetIfName:      remapped.TargetIfName,
		DiscoveryProtocol: remapped.DiscoveryProtocol,
	}
	if err := repo.Create(nextLink); err != nil {
		return uuid.Nil, fmt.Errorf("cloning canvas map link %s: %w", link.ID, err)
	}
	return nextLink.ID, nil
}

func clonePollIntervalOverrideForUpdate(value *int) **int {
	var sourceOverride *int
	if value != nil {
		copied := *value
		sourceOverride = &copied
	}
	return &sourceOverride
}

func cloneStringMap(values map[string]string) map[string]string {
	if values == nil {
		return nil
	}
	cloned := make(map[string]string, len(values))
	for key, value := range values {
		cloned[key] = value
	}
	return cloned
}

func cloneOptionalString(value *string) *string {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}
