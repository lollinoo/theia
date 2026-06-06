package domain

// This file defines position domain contracts and lifecycle invariants.

import (
	"time"

	"github.com/google/uuid"
)

// DevicePosition stores a persisted canvas coordinate for a device.
type DevicePosition struct {
	DeviceID  uuid.UUID `json:"device_id"`
	X         float64   `json:"x"`
	Y         float64   `json:"y"`
	Pinned    bool      `json:"pinned"`
	UpdatedAt time.Time `json:"updated_at"`
}

// PositionRepository defines persistence operations for device layout positions.
type PositionRepository interface {
	GetAll() ([]DevicePosition, error)
	SaveAll(positions []DevicePosition) error
	DeleteByDeviceID(deviceID uuid.UUID) error
}
