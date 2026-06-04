package canvasmap

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/domain"
)

// MaterializationStage identifies the dependency step that failed while materializing current topology.
type MaterializationStage string

const (
	MaterializationStageDevices MaterializationStage = "devices"
	MaterializationStageLinks   MaterializationStage = "links"
	MaterializationStageAreas   MaterializationStage = "areas"
	MaterializationStageReplace MaterializationStage = "replace"
)

// MaterializationError preserves the failed materialization stage for the HTTP adapter.
type MaterializationError struct {
	Stage MaterializationStage
	Err   error
}

// Error includes the failing stage so logs can distinguish materialization dependency failures.
func (e MaterializationError) Error() string {
	if e.Err == nil {
		return string(e.Stage)
	}
	return fmt.Sprintf("%s: %v", e.Stage, e.Err)
}

// Unwrap exposes the underlying repository/service error for errors.Is/As checks.
func (e MaterializationError) Unwrap() error {
	return e.Err
}

// MaterializationMapRepository is the narrow persistence surface needed to replace map membership.
type MaterializationMapRepository interface {
	ReplaceMembership(uuid.UUID, domain.CanvasMapMembership) error
}

// MaterializationDeviceService is the narrow device reader used by current-topology materialization.
type MaterializationDeviceService interface {
	GetAllDevices(context.Context) ([]domain.Device, error)
}

// MaterializationLinkRepository is the narrow link reader used by current-topology materialization.
type MaterializationLinkRepository interface {
	GetAll() ([]domain.Link, error)
}

// MaterializationAreaRepository is the narrow area reader used by current-topology materialization.
type MaterializationAreaRepository interface {
	GetAllWithDeviceCount() ([]domain.AreaWithCount, error)
}

// MaterializationDeps groups the repository/service operations needed to persist materialized topology.
type MaterializationDeps struct {
	Maps    MaterializationMapRepository
	Devices MaterializationDeviceService
	Links   MaterializationLinkRepository
	Areas   MaterializationAreaRepository
}

// ReplaceMaterializedMembership loads current topology, projects it through the map filter, and persists membership.
func ReplaceMaterializedMembership(
	ctx context.Context,
	mapID uuid.UUID,
	filter domain.CanvasMapFilter,
	deps MaterializationDeps,
) error {
	devices, err := deps.Devices.GetAllDevices(ctx)
	if err != nil {
		return wrapMaterializationError(MaterializationStageDevices, err)
	}
	links, err := deps.Links.GetAll()
	if err != nil {
		return wrapMaterializationError(MaterializationStageLinks, err)
	}
	areas, err := deps.Areas.GetAllWithDeviceCount()
	if err != nil {
		return wrapMaterializationError(MaterializationStageAreas, err)
	}

	membership := MaterializeMembership(devices, links, areas, filter)
	if err := deps.Maps.ReplaceMembership(mapID, membership); err != nil {
		return wrapMaterializationError(MaterializationStageReplace, err)
	}
	return nil
}

// wrapMaterializationError annotates lower-level materialization failures with the adapter stage.
func wrapMaterializationError(stage MaterializationStage, err error) error {
	if err == nil {
		return nil
	}
	return MaterializationError{Stage: stage, Err: err}
}
