package domain

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

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

type CanvasMapDeviceRole string

const (
	CanvasMapDeviceRoleBase  CanvasMapDeviceRole = "base"
	CanvasMapDeviceRoleGhost CanvasMapDeviceRole = "ghost"
)

func (role CanvasMapDeviceRole) IsValid() bool {
	return role == CanvasMapDeviceRoleBase || role == CanvasMapDeviceRoleGhost
}

type CanvasMapDeviceMembership struct {
	DeviceID    uuid.UUID
	Role        CanvasMapDeviceRole
	AreaIDs     []uuid.UUID
	VisualColor *string
}

type CanvasMapAreaMembership struct {
	AreaID      uuid.UUID
	Name        string
	Description string
	Color       string
}

type CanvasMapMembership struct {
	Devices []CanvasMapDeviceMembership
	LinkIDs []uuid.UUID
	Areas   []CanvasMapAreaMembership
}

type CanvasMap struct {
	ID                     uuid.UUID
	Name                   string
	Description            string
	SourceAreaID           *uuid.UUID
	FilterJSON             string
	IsDefault              bool
	MembershipMaterialized bool
	DeviceCount            int
	LinkCount              int
	PositionCount          int
	CreatedAt              time.Time
	UpdatedAt              time.Time
}

type CanvasMapCreate struct {
	Name         string
	Description  string
	SourceAreaID *uuid.UUID
	Filter       CanvasMapFilter
	IsDefault    bool
}

type CanvasMapUpdate struct {
	Name            *string
	Description     *string
	SourceAreaID    *uuid.UUID
	SourceAreaIDSet bool
	Filter          *CanvasMapFilter
}

type CanvasMapRepository interface {
	Create(input CanvasMapCreate) (CanvasMap, error)
	GetByID(id uuid.UUID) (CanvasMap, error)
	GetDefault() (CanvasMap, error)
	List() ([]CanvasMap, error)
	Update(id uuid.UUID, input CanvasMapUpdate) (CanvasMap, error)
	SetPrimary(id uuid.UUID) (CanvasMap, error)
	Delete(id uuid.UUID) error
	Duplicate(id uuid.UUID, name string) (CanvasMap, error)
	GetMembership(id uuid.UUID) (CanvasMapMembership, error)
	ReplaceMembership(id uuid.UUID, membership CanvasMapMembership) error
	UpdateDeviceVisualColor(id uuid.UUID, deviceID uuid.UUID, visualColor *string) error
	RemoveDevice(id uuid.UUID, deviceID uuid.UUID) error
	RemoveLink(id uuid.UUID, linkID uuid.UUID) error
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
	if utf8.RuneCountInString(name) > CanvasMapNameMaxLength {
		return fmt.Errorf("map name must be %d characters or fewer", CanvasMapNameMaxLength)
	}
	return nil
}

func ValidateCanvasMapDescription(description string) error {
	if utf8.RuneCountInString(description) > CanvasMapDescriptionMaxLength {
		return fmt.Errorf("map description must be %d characters or fewer", CanvasMapDescriptionMaxLength)
	}
	return nil
}

func CanonicalCanvasMapFilterJSON(filter CanvasMapFilter) (string, error) {
	filter.DeviceIDs = canonicalCanvasMapDeviceIDs(filter.DeviceIDs)
	if filter.Tags == nil {
		filter.Tags = map[string]string{}
	}
	payload, err := json.Marshal(filter)
	if err != nil {
		return "", fmt.Errorf("encoding canvas map filter: %w", err)
	}
	return string(payload), nil
}

func canonicalCanvasMapDeviceIDs(deviceIDs []uuid.UUID) []uuid.UUID {
	if len(deviceIDs) == 0 {
		return []uuid.UUID{}
	}

	canonical := append([]uuid.UUID(nil), deviceIDs...)
	sort.Slice(canonical, func(i, j int) bool {
		return canonical[i].String() < canonical[j].String()
	})

	deduplicated := canonical[:0]
	for _, deviceID := range canonical {
		if len(deduplicated) == 0 || deduplicated[len(deduplicated)-1] != deviceID {
			deduplicated = append(deduplicated, deviceID)
		}
	}
	return deduplicated
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
