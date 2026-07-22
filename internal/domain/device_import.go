package domain

import (
	"context"
	"errors"

	"github.com/google/uuid"
)

var (
	// ErrDeviceImportAddressConflict reports that an imported address is already owned.
	ErrDeviceImportAddressConflict = errors.New("device import address conflict")
	// ErrDeviceImportDestinationChanged reports that the selected map or map-local area disappeared.
	ErrDeviceImportDestinationChanged = errors.New("device import destination changed")
	// ErrDeviceImportStoreUnavailable reports that persistence could not complete safely.
	ErrDeviceImportStoreUnavailable = errors.New("device import store unavailable")
)

// DeviceImportPlacement identifies the saved map and optional map-local area for one imported device.
type DeviceImportPlacement struct {
	MapID  uuid.UUID
	AreaID *uuid.UUID
}

// DeviceImportStore persists imported devices and delays publication until a completed batch is ready.
type DeviceImportStore interface {
	ExistingCanonicalAddresses(
		ctx context.Context,
		addresses []string,
	) (map[string]struct{}, error)
	CreateDeviceInMap(
		ctx context.Context,
		device *Device,
		placement DeviceImportPlacement,
	) error
	PublishCreatedDevices(deviceIDs []uuid.UUID)
}
