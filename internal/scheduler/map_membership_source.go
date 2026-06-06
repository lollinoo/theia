package scheduler

// This file defines map membership source scheduling behavior, timing policy, and queue ownership.

import (
	"fmt"

	"github.com/google/uuid"

	"github.com/lollinoo/theia/internal/domain"
)

type canvasMapMembershipSource interface {
	List() ([]domain.CanvasMap, error)
	GetMembership(id uuid.UUID) (domain.CanvasMapMembership, error)
}

type savedMapMemberDeviceIDSource interface {
	ListMemberDeviceIDs() ([]uuid.UUID, error)
}

type deviceMembershipGate interface {
	IncludesDevice(id uuid.UUID) (bool, error)
}

type savedMapDeviceSource struct {
	source  DeviceSource
	mapRepo canvasMapMembershipSource
}

// NewSavedMapDeviceSource wraps a device source so recurring polling only sees
// devices that belong to at least one saved map.
func NewSavedMapDeviceSource(source DeviceSource, mapRepo canvasMapMembershipSource) DeviceSource {
	return savedMapDeviceSource{source: source, mapRepo: mapRepo}
}

// GetDevices retrieves devices data from the scheduler.
func (s savedMapDeviceSource) GetDevices() ([]domain.Device, error) {
	devices, err := s.source.GetDevices()
	if err != nil {
		return nil, err
	}
	if s.mapRepo == nil {
		return devices, nil
	}

	memberIDs, err := s.savedMapMemberIDs()
	if err != nil {
		return nil, err
	}
	if len(memberIDs) == 0 || len(devices) == 0 {
		return []domain.Device{}, nil
	}

	filtered := make([]domain.Device, 0, len(devices))
	for _, device := range devices {
		if _, ok := memberIDs[device.ID]; ok {
			filtered = append(filtered, device)
		}
	}
	return filtered, nil
}

func (s savedMapDeviceSource) IncludesDevice(id uuid.UUID) (bool, error) {
	if s.mapRepo == nil {
		return true, nil
	}
	memberIDs, err := s.savedMapMemberIDs()
	if err != nil {
		return false, err
	}
	_, ok := memberIDs[id]
	return ok, nil
}

func (s savedMapDeviceSource) savedMapMemberIDs() (map[uuid.UUID]struct{}, error) {
	if source, ok := s.mapRepo.(savedMapMemberDeviceIDSource); ok {
		deviceIDs, err := source.ListMemberDeviceIDs()
		if err != nil {
			return nil, err
		}
		memberIDs := make(map[uuid.UUID]struct{}, len(deviceIDs))
		for _, deviceID := range deviceIDs {
			if deviceID != uuid.Nil {
				memberIDs[deviceID] = struct{}{}
			}
		}
		return memberIDs, nil
	}

	maps, err := s.mapRepo.List()
	if err != nil {
		return nil, err
	}

	memberIDs := make(map[uuid.UUID]struct{})
	for _, canvasMap := range maps {
		membership, err := s.mapRepo.GetMembership(canvasMap.ID)
		if err != nil {
			return nil, fmt.Errorf("loading saved map membership %s: %w", canvasMap.ID, err)
		}
		for _, device := range membership.Devices {
			if device.DeviceID != uuid.Nil {
				memberIDs[device.DeviceID] = struct{}{}
			}
		}
	}
	return memberIDs, nil
}
