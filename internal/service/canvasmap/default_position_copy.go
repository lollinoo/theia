package canvasmap

// This file defines default position copy canvas-map service behavior and topology ownership rules.

import (
	"fmt"

	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/domain"
)

// DefaultPositionCopyStage identifies the dependency step that failed while copying default positions.
type DefaultPositionCopyStage string

const (
	DefaultPositionCopyStageDefaultMap      DefaultPositionCopyStage = "default_map"
	DefaultPositionCopyStageMembership      DefaultPositionCopyStage = "membership"
	DefaultPositionCopyStagePositions       DefaultPositionCopyStage = "positions"
	DefaultPositionCopyStageLegacyPositions DefaultPositionCopyStage = "legacy_positions"
	DefaultPositionCopyStageSavePositions   DefaultPositionCopyStage = "save_positions"
)

// DefaultPositionCopyError preserves the failed position-copy stage for HTTP adapter mapping.
type DefaultPositionCopyError struct {
	Stage DefaultPositionCopyStage
	Err   error
}

// Error includes the failing stage so default-position copy failures remain distinguishable in logs.
func (e DefaultPositionCopyError) Error() string {
	if e.Err == nil {
		return string(e.Stage)
	}
	return fmt.Sprintf("%s: %v", e.Stage, e.Err)
}

// Unwrap exposes the underlying repository error for errors.Is/As checks.
func (e DefaultPositionCopyError) Unwrap() error {
	return e.Err
}

// DefaultPositionCopyMapRepository is the map persistence surface needed to find source and target membership.
type DefaultPositionCopyMapRepository interface {
	GetDefault() (domain.CanvasMap, error)
	GetMembership(uuid.UUID) (domain.CanvasMapMembership, error)
}

// DefaultPositionCopyPositionRepository is the map-position surface needed to load and save copied positions.
type DefaultPositionCopyPositionRepository interface {
	GetAllForMap(uuid.UUID) ([]domain.DevicePosition, error)
	SaveAllForMap(uuid.UUID, []domain.DevicePosition) error
}

// DefaultPositionCopyLegacyRepository is the legacy canvas position reader used only for fallback.
type DefaultPositionCopyLegacyRepository interface {
	GetAll() ([]domain.DevicePosition, error)
}

// DefaultPositionCopyDeps groups collaborators required to copy default map positions.
type DefaultPositionCopyDeps struct {
	Maps            DefaultPositionCopyMapRepository
	Positions       DefaultPositionCopyPositionRepository
	LegacyPositions DefaultPositionCopyLegacyRepository
}

// CopyDefaultPositionsForMaterializedMembership copies valid default-map positions for a newly materialized map.
func CopyDefaultPositionsForMaterializedMembership(mapID uuid.UUID, deps DefaultPositionCopyDeps) error {
	defaultMap, err := deps.Maps.GetDefault()
	if err != nil {
		return wrapDefaultPositionCopyError(DefaultPositionCopyStageDefaultMap, err)
	}
	if !ShouldCopyDefaultPositions(mapID, defaultMap.ID) {
		return nil
	}

	membership, err := deps.Maps.GetMembership(mapID)
	if err != nil {
		return wrapDefaultPositionCopyError(DefaultPositionCopyStageMembership, err)
	}
	sourcePositions, err := deps.Positions.GetAllForMap(defaultMap.ID)
	if err != nil {
		return wrapDefaultPositionCopyError(DefaultPositionCopyStagePositions, err)
	}

	legacyPositions := []domain.DevicePosition{}
	if len(sourcePositions) == 0 && deps.LegacyPositions != nil {
		legacyPositions, err = deps.LegacyPositions.GetAll()
		if err != nil {
			return wrapDefaultPositionCopyError(DefaultPositionCopyStageLegacyPositions, err)
		}
	}

	copyPlan := PlanDefaultPositionCopy(
		mapID,
		defaultMap.ID,
		sourcePositions,
		legacyPositions,
		membership.Devices,
	)
	if !copyPlan.ShouldSave {
		return nil
	}
	if err := deps.Positions.SaveAllForMap(mapID, copyPlan.Positions); err != nil {
		return wrapDefaultPositionCopyError(DefaultPositionCopyStageSavePositions, err)
	}
	return nil
}

// wrapDefaultPositionCopyError annotates lower-level position-copy failures with the adapter stage.
func wrapDefaultPositionCopyError(stage DefaultPositionCopyStage, err error) error {
	if err == nil {
		return nil
	}
	return DefaultPositionCopyError{Stage: stage, Err: err}
}
