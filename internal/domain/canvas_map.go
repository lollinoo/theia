package domain

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	CanvasMapNameMaxLength        = 80
	CanvasMapDescriptionMaxLength = 500
)

type CanvasMapFilter struct {
	AreaID                *uuid.UUID        `json:"area_id,omitempty"`
	DeviceIDs             []uuid.UUID       `json:"device_ids,omitempty"`
	IncludeCrossAreaLinks bool              `json:"include_cross_area_links"`
	IncludeGhostDevices   bool              `json:"include_ghost_devices"`
	Tags                  map[string]string `json:"tags,omitempty"`
}

type CanvasMap struct {
	ID            uuid.UUID
	Name          string
	Description   string
	SourceAreaID  *uuid.UUID
	FilterJSON    string
	IsDefault     bool
	DeviceCount   int
	LinkCount     int
	PositionCount int
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

type CanvasMapCreate struct {
	Name         string
	Description  string
	SourceAreaID *uuid.UUID
	Filter       CanvasMapFilter
	IsDefault    bool
}

type CanvasMapUpdate struct {
	Name        *string
	Description *string
	Filter      *CanvasMapFilter
}

type CanvasMapRepository interface {
	Create(input CanvasMapCreate) (CanvasMap, error)
	GetByID(id uuid.UUID) (CanvasMap, error)
	GetDefault() (CanvasMap, error)
	List() ([]CanvasMap, error)
	Update(id uuid.UUID, input CanvasMapUpdate) (CanvasMap, error)
	Delete(id uuid.UUID) error
	Duplicate(id uuid.UUID, name string) (CanvasMap, error)
}

type CanvasMapPositionRepository interface {
	GetAllForMap(mapID uuid.UUID) ([]DevicePosition, error)
	SaveAllForMap(mapID uuid.UUID, positions []DevicePosition) error
	DeleteByDeviceID(deviceID uuid.UUID) error
}

func ValidateCanvasMapName(name string) error {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return fmt.Errorf("map name is required")
	}
	if len(trimmed) > CanvasMapNameMaxLength {
		return fmt.Errorf("map name must be %d characters or fewer", CanvasMapNameMaxLength)
	}
	return nil
}

func ValidateCanvasMapDescription(description string) error {
	if len(description) > CanvasMapDescriptionMaxLength {
		return fmt.Errorf("map description must be %d characters or fewer", CanvasMapDescriptionMaxLength)
	}
	return nil
}

func CanonicalCanvasMapFilterJSON(filter CanvasMapFilter) (string, error) {
	if filter.DeviceIDs == nil {
		filter.DeviceIDs = []uuid.UUID{}
	}
	if filter.Tags == nil {
		filter.Tags = map[string]string{}
	}
	payload, err := json.Marshal(filter)
	if err != nil {
		return "", fmt.Errorf("encoding canvas map filter: %w", err)
	}
	return string(payload), nil
}

func ParseCanvasMapFilter(raw string) (CanvasMapFilter, error) {
	if strings.TrimSpace(raw) == "" {
		return CanvasMapFilter{DeviceIDs: []uuid.UUID{}, Tags: map[string]string{}}, nil
	}
	var filter CanvasMapFilter
	if err := json.Unmarshal([]byte(raw), &filter); err != nil {
		return CanvasMapFilter{}, fmt.Errorf("decoding canvas map filter: %w", err)
	}
	if filter.DeviceIDs == nil {
		filter.DeviceIDs = []uuid.UUID{}
	}
	if filter.Tags == nil {
		filter.Tags = map[string]string{}
	}
	return filter, nil
}
