package canvasmap

// This file defines source map materialization canvas-map service behavior and topology ownership rules.

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/domain"
)

// SourceMapMaterializationStage identifies the dependency step that failed while materializing from a saved map.
type SourceMapMaterializationStage string

const (
	SourceMapMaterializationStageMembership    SourceMapMaterializationStage = "membership"
	SourceMapMaterializationStageDevices       SourceMapMaterializationStage = "devices"
	SourceMapMaterializationStageLinks         SourceMapMaterializationStage = "links"
	SourceMapMaterializationStageAreas         SourceMapMaterializationStage = "areas"
	SourceMapMaterializationStageReplace       SourceMapMaterializationStage = "replace"
	SourceMapMaterializationStagePositions     SourceMapMaterializationStage = "positions"
	SourceMapMaterializationStageSavePositions SourceMapMaterializationStage = "save_positions"
)

// SourceMapMaterializationError preserves the failed source-map stage for HTTP adapter mapping.
type SourceMapMaterializationError struct {
	Stage SourceMapMaterializationStage
	Err   error
}

// Error includes the failing stage so source-map materialization failures remain distinguishable in logs.
func (e SourceMapMaterializationError) Error() string {
	if e.Err == nil {
		return string(e.Stage)
	}
	return fmt.Sprintf("%s: %v", e.Stage, e.Err)
}

// Unwrap exposes the underlying repository/service error for errors.Is/As checks.
func (e SourceMapMaterializationError) Unwrap() error {
	return e.Err
}

// SourceMapMaterializationMapRepository is the map persistence surface needed to copy saved-map membership.
type SourceMapMaterializationMapRepository interface {
	GetMembership(uuid.UUID) (domain.CanvasMapMembership, error)
	ReplaceMembership(uuid.UUID, domain.CanvasMapMembership) error
}

// SourceMapMaterializationPositionRepository is the map-position surface needed to copy saved-map positions.
type SourceMapMaterializationPositionRepository interface {
	GetAllForMap(uuid.UUID) ([]domain.DevicePosition, error)
	SaveAllForMap(uuid.UUID, []domain.DevicePosition) error
}

// SourceMapMaterializationDeviceService is the device reader used to hydrate source-map members.
type SourceMapMaterializationDeviceService interface {
	GetDevicesByIDs(context.Context, []uuid.UUID) ([]domain.Device, error)
}

// SourceMapMaterializationDeps groups collaborators required to materialize a map from another saved map.
type SourceMapMaterializationDeps struct {
	Maps      SourceMapMaterializationMapRepository
	Positions SourceMapMaterializationPositionRepository
	Devices   SourceMapMaterializationDeviceService
	Links     VirtualIsolationLinkRepository
	Areas     MaterializationAreaRepository
}

// ReplaceMaterializedMembershipFromSourceMap projects source-map membership into a target map and copies valid positions.
func ReplaceMaterializedMembershipFromSourceMap(
	ctx context.Context,
	mapID uuid.UUID,
	sourceMapID uuid.UUID,
	filter domain.CanvasMapFilter,
	deps SourceMapMaterializationDeps,
) error {
	sourceMembership, err := deps.Maps.GetMembership(sourceMapID)
	if err != nil {
		return wrapSourceMapMaterializationError(SourceMapMaterializationStageMembership, err)
	}
	devices, err := deps.Devices.GetDevicesByIDs(ctx, MembershipDeviceIDs(sourceMembership.Devices))
	if err != nil {
		return wrapSourceMapMaterializationError(SourceMapMaterializationStageDevices, err)
	}
	links, err := LoadLinksByIDs(deps.Links, sourceMembership.LinkIDs)
	if err != nil {
		return wrapSourceMapMaterializationError(SourceMapMaterializationStageLinks, err)
	}

	fallbackAreas := []domain.AreaWithCount{}
	if len(sourceMembership.Areas) == 0 {
		fallbackAreas, err = deps.Areas.GetAllWithDeviceCount()
		if err != nil {
			return wrapSourceMapMaterializationError(SourceMapMaterializationStageAreas, err)
		}
	}

	plan := PlanSourceMapMaterialization(devices, links, sourceMembership, fallbackAreas, filter, nil)
	if err := deps.Maps.ReplaceMembership(mapID, plan.Membership); err != nil {
		return wrapSourceMapMaterializationError(SourceMapMaterializationStageReplace, err)
	}

	sourcePositions, err := deps.Positions.GetAllForMap(sourceMapID)
	if err != nil {
		return wrapSourceMapMaterializationError(SourceMapMaterializationStagePositions, err)
	}
	positionPlan := PlanSourceMapMaterialization(devices, links, sourceMembership, fallbackAreas, filter, sourcePositions)
	if positionPlan.ShouldSavePositions {
		if err := deps.Positions.SaveAllForMap(mapID, positionPlan.Positions); err != nil {
			return wrapSourceMapMaterializationError(SourceMapMaterializationStageSavePositions, err)
		}
	}
	return nil
}

// wrapSourceMapMaterializationError annotates lower-level source-map failures with the adapter stage.
func wrapSourceMapMaterializationError(stage SourceMapMaterializationStage, err error) error {
	if err == nil {
		return nil
	}
	return SourceMapMaterializationError{Stage: stage, Err: err}
}
