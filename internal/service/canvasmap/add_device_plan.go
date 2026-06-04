package canvasmap

import (
	"errors"

	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/domain"
)

var ErrDeviceAlreadyInCanvasMap = errors.New("device already exists in this map")

// DuplicateDeviceAddressError preserves the user-visible duplicate-address message.
type DuplicateDeviceAddressError struct {
	Address string
}

// Error formats the duplicate-address conflict using the API-compatible text.
func (e DuplicateDeviceAddressError) Error() string {
	return DuplicateDeviceAddressMessage(e.Address)
}

// AddDeviceMembershipPlan describes the map-local membership rows to persist.
type AddDeviceMembershipPlan struct {
	Device  domain.CanvasMapDeviceMembership
	LinkIDs []uuid.UUID
	Areas   []domain.CanvasMapAreaMembership
}

// PlanAddDeviceMembership chooses whether to add a new base member or only missing links for an existing member.
func PlanAddDeviceMembership(
	device domain.Device,
	membership domain.CanvasMapMembership,
	memberDevices []domain.Device,
	connectedLinks []domain.Link,
	areas []domain.CanvasMapAreaMembership,
	includeConnectedLinks bool,
) (AddDeviceMembershipPlan, error) {
	if member, exists := MemberByDeviceID(membership, device.ID); exists {
		if !includeConnectedLinks {
			return AddDeviceMembershipPlan{}, ErrDeviceAlreadyInCanvasMap
		}
		linkIDs := ConnectedBaseLinkIDs(device.ID, membership, connectedLinks)
		missingLinkIDs := MissingLinkIDs(membership.LinkIDs, linkIDs)
		if len(missingLinkIDs) == 0 {
			return AddDeviceMembershipPlan{}, ErrDeviceAlreadyInCanvasMap
		}
		return AddDeviceMembershipPlan{
			Device:  member,
			LinkIDs: missingLinkIDs,
			Areas:   append([]domain.CanvasMapAreaMembership(nil), membership.Areas...),
		}, nil
	}

	if HasDuplicateDeviceAddress(device, memberDevices) {
		return AddDeviceMembershipPlan{}, DuplicateDeviceAddressError{Address: device.IP}
	}

	linkIDs := []uuid.UUID{}
	if includeConnectedLinks {
		linkIDs = ConnectedBaseLinkIDs(device.ID, membership, connectedLinks)
	}
	return AddDeviceMembershipPlan{
		Device:  BaseDeviceMembership(device),
		LinkIDs: linkIDs,
		Areas:   append([]domain.CanvasMapAreaMembership(nil), areas...),
	}, nil
}

// MemberByDeviceID returns a defensive copy of an existing map membership row.
func MemberByDeviceID(
	membership domain.CanvasMapMembership,
	deviceID uuid.UUID,
) (domain.CanvasMapDeviceMembership, bool) {
	for _, member := range membership.Devices {
		if member.DeviceID != deviceID {
			continue
		}
		return domain.CanvasMapDeviceMembership{
			DeviceID:    member.DeviceID,
			Role:        member.Role,
			AreaIDs:     append([]uuid.UUID(nil), member.AreaIDs...),
			VisualColor: copyOptionalString(member.VisualColor),
		}, true
	}
	return domain.CanvasMapDeviceMembership{}, false
}
