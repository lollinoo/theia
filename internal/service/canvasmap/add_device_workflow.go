package canvasmap

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/domain"
)

// AddDeviceMembershipStage identifies the dependency step that failed while adding a device to a saved map.
type AddDeviceMembershipStage string

const (
	AddDeviceMembershipStageDevice        AddDeviceMembershipStage = "device"
	AddDeviceMembershipStageMembership    AddDeviceMembershipStage = "membership"
	AddDeviceMembershipStageMemberDevices AddDeviceMembershipStage = "member_devices"
	AddDeviceMembershipStageLinks         AddDeviceMembershipStage = "links"
	AddDeviceMembershipStageAreas         AddDeviceMembershipStage = "areas"
	AddDeviceMembershipStagePersist       AddDeviceMembershipStage = "persist"
)

// AddDeviceMembershipError preserves the failed add-device stage for HTTP adapter mapping.
type AddDeviceMembershipError struct {
	Stage AddDeviceMembershipStage
	Err   error
}

// Error includes the failing stage so add-device workflow failures remain distinguishable in logs.
func (e AddDeviceMembershipError) Error() string {
	if e.Err == nil {
		return string(e.Stage)
	}
	return fmt.Sprintf("%s: %v", e.Stage, e.Err)
}

// Unwrap exposes the underlying repository/service error for errors.Is/As checks.
func (e AddDeviceMembershipError) Unwrap() error {
	return e.Err
}

// AddDeviceMembershipMapRepository is the map persistence surface needed for incremental membership changes.
type AddDeviceMembershipMapRepository interface {
	GetMembership(uuid.UUID) (domain.CanvasMapMembership, error)
	AddDeviceMembership(uuid.UUID, domain.CanvasMapDeviceMembership, []uuid.UUID, []domain.CanvasMapAreaMembership) error
}

// AddDeviceMembershipDeviceService is the device reader surface used by add-device membership orchestration.
type AddDeviceMembershipDeviceService interface {
	GetDevice(context.Context, uuid.UUID) (*domain.Device, error)
	GetDevicesByIDs(context.Context, []uuid.UUID) ([]domain.Device, error)
}

// AddDeviceMembershipLinkRepository is the link reader surface used for include-connected-links.
type AddDeviceMembershipLinkRepository interface {
	GetByDeviceID(uuid.UUID) ([]domain.Link, error)
}

// AddDeviceMembershipAreaRepository is the area reader surface used to snapshot new-device areas.
type AddDeviceMembershipAreaRepository interface {
	GetByID(uuid.UUID) (*domain.Area, error)
}

// AddDeviceMembershipDeps groups collaborators required to add saved-map device membership.
type AddDeviceMembershipDeps struct {
	Maps    AddDeviceMembershipMapRepository
	Devices AddDeviceMembershipDeviceService
	Links   AddDeviceMembershipLinkRepository
	Areas   AddDeviceMembershipAreaRepository
}

// AddDeviceToMaterializedMembership plans and persists a saved-map device membership addition.
func AddDeviceToMaterializedMembership(
	ctx context.Context,
	mapID uuid.UUID,
	deviceID uuid.UUID,
	includeConnectedLinks bool,
	deps AddDeviceMembershipDeps,
) error {
	device, err := deps.Devices.GetDevice(ctx, deviceID)
	if err != nil {
		return wrapAddDeviceMembershipError(AddDeviceMembershipStageDevice, err)
	}
	membership, err := deps.Maps.GetMembership(mapID)
	if err != nil {
		return wrapAddDeviceMembershipError(AddDeviceMembershipStageMembership, err)
	}

	memberDevices := []domain.Device{}
	areas := []domain.CanvasMapAreaMembership{}
	_, existingMember := MemberByDeviceID(membership, deviceID)
	if !existingMember && NormalizeDeviceAddress(device.IP) != "" {
		memberDevices, err = deps.Devices.GetDevicesByIDs(ctx, MembershipDeviceIDs(membership.Devices))
		if err != nil {
			return wrapAddDeviceMembershipError(AddDeviceMembershipStageMemberDevices, err)
		}
		if _, err := PlanAddDeviceMembership(*device, membership, memberDevices, nil, nil, false); err != nil {
			return err
		}
	}

	links := []domain.Link{}
	if includeConnectedLinks {
		links, err = deps.Links.GetByDeviceID(deviceID)
		if err != nil {
			return wrapAddDeviceMembershipError(AddDeviceMembershipStageLinks, err)
		}
	}

	if !existingMember {
		areas, err = AreaMembershipsForDevice(deps.Areas, device)
		if err != nil {
			return wrapAddDeviceMembershipError(AddDeviceMembershipStageAreas, err)
		}
	}
	plan, err := PlanAddDeviceMembership(*device, membership, memberDevices, links, areas, includeConnectedLinks)
	if err != nil {
		return err
	}
	if err := deps.Maps.AddDeviceMembership(mapID, plan.Device, plan.LinkIDs, plan.Areas); err != nil {
		return wrapAddDeviceMembershipError(AddDeviceMembershipStagePersist, err)
	}
	return nil
}

// AreaMembershipsForDevice loads global area rows and converts them to saved-map snapshots for a device.
func AreaMembershipsForDevice(
	repo AddDeviceMembershipAreaRepository,
	device *domain.Device,
) ([]domain.CanvasMapAreaMembership, error) {
	if device == nil || len(device.AreaIDs) == 0 {
		return []domain.CanvasMapAreaMembership{}, nil
	}
	areas := make([]domain.Area, 0, len(device.AreaIDs))
	for _, areaID := range device.AreaIDs {
		area, err := repo.GetByID(areaID)
		if err != nil {
			return nil, err
		}
		areas = append(areas, *area)
	}
	return AreasToMembership(areas), nil
}

// wrapAddDeviceMembershipError annotates lower-level add-device failures with the adapter stage.
func wrapAddDeviceMembershipError(stage AddDeviceMembershipStage, err error) error {
	if err == nil {
		return nil
	}
	return AddDeviceMembershipError{Stage: stage, Err: err}
}
