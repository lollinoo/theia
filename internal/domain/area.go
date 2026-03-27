package domain

import (
	"time"

	"github.com/google/uuid"
)

// Area represents a logical grouping of network devices (e.g., OSPF area).
type Area struct {
	ID          uuid.UUID `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Color       string    `json:"color"` // hex color e.g. "#00E676"
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// AreaWithCount extends Area with the number of devices assigned to it.
type AreaWithCount struct {
	Area
	DeviceCount int `json:"device_count"`
}

// AreaRepository defines persistence operations for areas.
type AreaRepository interface {
	Create(area *Area) error
	GetByID(id uuid.UUID) (*Area, error)
	GetAll() ([]Area, error)
	GetAllWithDeviceCount() ([]AreaWithCount, error)
	Update(area *Area) error
	Delete(id uuid.UUID) error
}
