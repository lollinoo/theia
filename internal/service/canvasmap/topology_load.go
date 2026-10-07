package canvasmap

// This file defines topology load canvas-map service behavior and topology ownership rules.

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/domain"
)

// TopologyLoadStage identifies the IO step that failed while loading saved-map topology.
type TopologyLoadStage string

const (
	TopologyLoadStageIsolate    TopologyLoadStage = "isolate"
	TopologyLoadStageMap        TopologyLoadStage = "map"
	TopologyLoadStagePositions  TopologyLoadStage = "positions"
	TopologyLoadStageMembership TopologyLoadStage = "membership"
	TopologyLoadStageDevices    TopologyLoadStage = "devices"
	TopologyLoadStageLinks      TopologyLoadStage = "links"
)

// TopologyLoadError preserves the failed loader stage for HTTP adapter error mapping.
type TopologyLoadError struct {
	Stage TopologyLoadStage
	Err   error
}

// Error includes the failing stage so logs can distinguish loader failures.
func (e TopologyLoadError) Error() string {
	if e.Err == nil {
		return string(e.Stage)
	}
	return fmt.Sprintf("%s: %v", e.Stage, e.Err)
}

// Unwrap exposes the underlying repository/service error for errors.Is/As.
func (e TopologyLoadError) Unwrap() error {
	return e.Err
}

// TopologyMapRepository is the narrow map persistence surface needed for topology loading.
type TopologyMapRepository interface {
	GetMembership(uuid.UUID) (domain.CanvasMapMembership, error)
	GetByID(uuid.UUID) (domain.CanvasMap, error)
}

// TopologyLoadDeps groups the persistence and device/link operations needed to load saved-map topology.
type TopologyLoadDeps struct {
	Maps      TopologyMapRepository
	Positions interface {
		GetAllForMap(uuid.UUID) ([]domain.DevicePosition, error)
	}
	Devices interface {
		GetDevicesByIDs(context.Context, []uuid.UUID) ([]domain.Device, error)
	}
	Links TopologyLinkRepository
}

// TopologyLoadResult returns the fresh map row and the domain-level response plan for the API adapter.
type TopologyLoadResult struct {
	Map  domain.CanvasMap
	Plan TopologyResponsePlan
}

// LoadTopology performs saved-map structural loading before the HTTP layer
// formats JSON: map loading, member projection,
// link filtering, position pruning, counts, and visual metadata.
func LoadTopology(
	ctx context.Context,
	mapID uuid.UUID,
	deps TopologyLoadDeps,
) (TopologyLoadResult, error) {

	canvasMap, err := deps.Maps.GetByID(mapID)
	if err != nil {
		return TopologyLoadResult{}, wrapTopologyLoadError(TopologyLoadStageMap, err)
	}

	return LoadTopologyForMap(ctx, canvasMap, deps)
}

// LoadTopologyForMap reuses the map already validated by the HTTP adapter.
// Unmaterialized maps have no projected positions, membership or links to load.
func LoadTopologyForMap(ctx context.Context, canvasMap domain.CanvasMap, deps TopologyLoadDeps) (TopologyLoadResult, error) {
	mapID := canvasMap.ID
	if !canvasMap.MembershipMaterialized {
		return TopologyLoadResult{Map: canvasMap, Plan: EmptyTopologyResponsePlan()}, nil
	}
	positions, err := deps.Positions.GetAllForMap(mapID)
	if err != nil {
		return TopologyLoadResult{}, wrapTopologyLoadError(TopologyLoadStagePositions, err)
	}

	loadMembership := deps.Maps.GetMembership
	if existing, ok := deps.Maps.(interface {
		GetMembershipForExistingMap(uuid.UUID) (domain.CanvasMapMembership, error)
	}); ok {
		loadMembership = existing.GetMembershipForExistingMap
	}
	membership, err := loadMembership(mapID)
	if err != nil {
		return TopologyLoadResult{}, wrapTopologyLoadError(TopologyLoadStageMembership, err)
	}
	devices, err := deps.Devices.GetDevicesByIDs(ctx, MembershipDeviceIDs(membership.Devices))
	if err != nil {
		return TopologyLoadResult{}, wrapTopologyLoadError(TopologyLoadStageDevices, err)
	}
	links, err := LoadLinksByIDs(deps.Links, membership.LinkIDs)
	if err != nil {
		return TopologyLoadResult{}, wrapTopologyLoadError(TopologyLoadStageLinks, err)
	}

	return TopologyLoadResult{
		Map:  canvasMap,
		Plan: BuildMaterializedTopologyResponsePlan(membership, devices, links, positions),
	}, nil
}

// wrapTopologyLoadError annotates lower-level failures with the adapter stage.
func wrapTopologyLoadError(stage TopologyLoadStage, err error) error {
	if err == nil {
		return nil
	}
	return TopologyLoadError{Stage: stage, Err: err}
}
