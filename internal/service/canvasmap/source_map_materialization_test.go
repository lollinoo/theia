package canvasmap

// This file exercises source map materialization behavior so refactors preserve the documented contract.

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/domain"
)

// TestReplaceMaterializedMembershipFromSourceMapProjectsAndCopiesPositions characterizes source-map materialization orchestration.
func TestReplaceMaterializedMembershipFromSourceMapProjectsAndCopiesPositions(t *testing.T) {
	mapID := uuid.New()
	sourceMapID := uuid.New()
	areaID := uuid.New()
	baseID := uuid.New()
	ghostID := uuid.New()
	prunedID := uuid.New()
	linkID := uuid.New()
	order := []string{}
	visualColor := "#112233"
	maps := &fakeSourceMapMaterializationMapRepo{
		order: &order,
		memberships: map[uuid.UUID]domain.CanvasMapMembership{
			sourceMapID: {
				Devices: []domain.CanvasMapDeviceMembership{
					{DeviceID: baseID, Role: domain.CanvasMapDeviceRoleBase, AreaIDs: []uuid.UUID{areaID}, VisualColor: &visualColor},
					{DeviceID: ghostID, Role: domain.CanvasMapDeviceRoleGhost},
				},
				LinkIDs: []uuid.UUID{linkID},
				Areas: []domain.CanvasMapAreaMembership{
					{AreaID: areaID, Name: "Source Snapshot", Description: "source-local", Color: "#FFAB00"},
				},
			},
		},
	}
	positions := &fakeSourceMapMaterializationPositionRepo{
		order: &order,
		positions: map[uuid.UUID][]domain.DevicePosition{
			sourceMapID: {
				{DeviceID: baseID, X: 10, Y: 20, Pinned: true},
				{DeviceID: ghostID, X: 30, Y: 40},
				{DeviceID: prunedID, X: 50, Y: 60},
			},
		},
	}

	err := ReplaceMaterializedMembershipFromSourceMap(context.Background(), mapID, sourceMapID, domain.CanvasMapFilter{
		DeviceIDs:             []uuid.UUID{baseID},
		IncludeCrossAreaLinks: true,
		IncludeGhostDevices:   true,
	}, SourceMapMaterializationDeps{
		Maps:      maps,
		Positions: positions,
		Devices: &fakeSourceMapMaterializationDeviceService{
			order:   &order,
			devices: []domain.Device{{ID: baseID, AreaIDs: []uuid.UUID{areaID}}, {ID: ghostID}},
		},
		Links: &fakeSourceMapMaterializationLinkRepo{
			order: &order,
			links: []domain.Link{{ID: linkID, SourceDeviceID: baseID, TargetDeviceID: ghostID}},
		},
		Areas: &fakeMaterializationAreaRepo{order: &order},
	})

	if err != nil {
		t.Fatalf("ReplaceMaterializedMembershipFromSourceMap() error = %v", err)
	}
	if got, want := order, []string{"membership", "devices", "links", "replace", "positions", "save_positions"}; !stringSlicesEqual(got, want) {
		t.Fatalf("operation order = %v, want %v", got, want)
	}
	if maps.replacedMapID != mapID {
		t.Fatalf("replaced map ID = %s, want %s", maps.replacedMapID, mapID)
	}
	if len(maps.replacedMembership.Areas) != 1 || maps.replacedMembership.Areas[0].Name != "Source Snapshot" {
		t.Fatalf("areas = %+v, want source snapshot", maps.replacedMembership.Areas)
	}
	if got, want := maps.replacedMembership.LinkIDs, []uuid.UUID{linkID}; !uuidSlicesEqual(got, want) {
		t.Fatalf("link IDs = %v, want %v", got, want)
	}
	if len(maps.replacedMembership.Devices) != 2 || maps.replacedMembership.Devices[1].Role != domain.CanvasMapDeviceRoleGhost {
		t.Fatalf("devices = %+v, want base plus ghost", maps.replacedMembership.Devices)
	}
	if got, want := devicePositionIDs(positions.savedPositions), []uuid.UUID{baseID, ghostID}; !uuidSlicesEqual(got, want) {
		t.Fatalf("saved position IDs = %v, want %v", got, want)
	}
}

// TestReplaceMaterializedMembershipFromSourceMapWrapsEachIOStage keeps HTTP mapping independent from service internals.
func TestReplaceMaterializedMembershipFromSourceMapWrapsEachIOStage(t *testing.T) {
	assertSourceMapMaterializationStageError(t, SourceMapMaterializationStageMembership, errors.New("membership unavailable"))
	assertSourceMapMaterializationStageError(t, SourceMapMaterializationStageDevices, errors.New("devices unavailable"))
	assertSourceMapMaterializationStageError(t, SourceMapMaterializationStageLinks, errors.New("links unavailable"))
	assertSourceMapMaterializationStageError(t, SourceMapMaterializationStageAreas, errors.New("areas unavailable"))
	assertSourceMapMaterializationStageError(t, SourceMapMaterializationStageReplace, errors.New("replace unavailable"))
	assertSourceMapMaterializationStageError(t, SourceMapMaterializationStagePositions, errors.New("positions unavailable"))
	assertSourceMapMaterializationStageError(t, SourceMapMaterializationStageSavePositions, errors.New("save unavailable"))
}

// assertSourceMapMaterializationStageError verifies stage wrapping for one source-map materialization dependency failure.
func assertSourceMapMaterializationStageError(t *testing.T, stage SourceMapMaterializationStage, wantErr error) {
	t.Helper()
	mapID := uuid.New()
	sourceMapID := uuid.New()
	baseID := uuid.New()
	linkID := uuid.New()
	order := []string{}
	maps := &fakeSourceMapMaterializationMapRepo{
		order: &order,
		memberships: map[uuid.UUID]domain.CanvasMapMembership{
			sourceMapID: {
				Devices: []domain.CanvasMapDeviceMembership{{DeviceID: baseID, Role: domain.CanvasMapDeviceRoleBase}},
				LinkIDs: []uuid.UUID{linkID},
				Areas:   []domain.CanvasMapAreaMembership{{AreaID: uuid.New(), Name: "Source", Color: "#00E676"}},
			},
		},
	}
	devices := &fakeSourceMapMaterializationDeviceService{order: &order, devices: []domain.Device{{ID: baseID}}}
	links := &fakeSourceMapMaterializationLinkRepo{
		order: &order,
		links: []domain.Link{{ID: linkID, SourceDeviceID: baseID, TargetDeviceID: baseID}},
	}
	areas := &fakeMaterializationAreaRepo{order: &order}
	positions := &fakeSourceMapMaterializationPositionRepo{
		order: &order,
		positions: map[uuid.UUID][]domain.DevicePosition{
			sourceMapID: {{DeviceID: baseID, X: 10, Y: 20}},
		},
	}

	switch stage {
	case SourceMapMaterializationStageMembership:
		maps.membershipErr = wantErr
	case SourceMapMaterializationStageDevices:
		devices.err = wantErr
	case SourceMapMaterializationStageLinks:
		links.err = wantErr
	case SourceMapMaterializationStageAreas:
		maps.memberships[sourceMapID] = domain.CanvasMapMembership{
			Devices: []domain.CanvasMapDeviceMembership{{DeviceID: baseID, Role: domain.CanvasMapDeviceRoleBase}},
			LinkIDs: []uuid.UUID{linkID},
		}
		areas.err = wantErr
	case SourceMapMaterializationStageReplace:
		maps.replaceErr = wantErr
	case SourceMapMaterializationStagePositions:
		positions.loadErr = wantErr
	case SourceMapMaterializationStageSavePositions:
		positions.saveErr = wantErr
	default:
		t.Fatalf("unsupported source-map materialization stage %s", stage)
	}

	err := ReplaceMaterializedMembershipFromSourceMap(context.Background(), mapID, sourceMapID, domain.CanvasMapFilter{}, SourceMapMaterializationDeps{
		Maps:      maps,
		Positions: positions,
		Devices:   devices,
		Links:     links,
		Areas:     areas,
	})

	var sourceErr SourceMapMaterializationError
	if !errors.As(err, &sourceErr) {
		t.Fatalf("ReplaceMaterializedMembershipFromSourceMap() error = %T %[1]v, want SourceMapMaterializationError", err)
	}
	if sourceErr.Stage != stage {
		t.Fatalf("ReplaceMaterializedMembershipFromSourceMap() stage = %s, want %s", sourceErr.Stage, stage)
	}
	if !errors.Is(err, wantErr) {
		t.Fatalf("ReplaceMaterializedMembershipFromSourceMap() error = %v, want wrapped %v", err, wantErr)
	}
}

func devicePositionIDs(positions []domain.DevicePosition) []uuid.UUID {
	ids := make([]uuid.UUID, 0, len(positions))
	for _, position := range positions {
		ids = append(ids, position.DeviceID)
	}
	return ids
}

// fakeSourceMapMaterializationMapRepo records source membership reads and target membership writes.
type fakeSourceMapMaterializationMapRepo struct {
	order              *[]string
	memberships        map[uuid.UUID]domain.CanvasMapMembership
	membershipErr      error
	replaceErr         error
	replacedMapID      uuid.UUID
	replacedMembership domain.CanvasMapMembership
}

func (r *fakeSourceMapMaterializationMapRepo) GetMembership(id uuid.UUID) (domain.CanvasMapMembership, error) {
	*r.order = append(*r.order, "membership")
	if r.membershipErr != nil {
		return domain.CanvasMapMembership{}, r.membershipErr
	}
	return r.memberships[id], nil
}

// ReplaceMembership captures the target membership or returns the injected replacement error.
func (r *fakeSourceMapMaterializationMapRepo) ReplaceMembership(id uuid.UUID, membership domain.CanvasMapMembership) error {
	*r.order = append(*r.order, "replace")
	if r.replaceErr != nil {
		return r.replaceErr
	}
	r.replacedMapID = id
	r.replacedMembership = membership
	return nil
}

// fakeSourceMapMaterializationDeviceService supplies source member devices.
type fakeSourceMapMaterializationDeviceService struct {
	order   *[]string
	devices []domain.Device
	err     error
}

// GetDevicesByIDs records source-device loading and returns the configured devices.
func (s *fakeSourceMapMaterializationDeviceService) GetDevicesByIDs(context.Context, []uuid.UUID) ([]domain.Device, error) {
	*s.order = append(*s.order, "devices")
	if s.err != nil {
		return nil, s.err
	}
	return append([]domain.Device(nil), s.devices...), nil
}

// fakeSourceMapMaterializationLinkRepo supplies source member links.
type fakeSourceMapMaterializationLinkRepo struct {
	order *[]string
	links []domain.Link
	err   error
}

// Create satisfies the shared link repository interface without exercising link cloning.
func (r *fakeSourceMapMaterializationLinkRepo) Create(*domain.Link) error {
	return nil
}

// GetAll records source-link loading and returns the configured links.
func (r *fakeSourceMapMaterializationLinkRepo) GetAll() ([]domain.Link, error) {
	*r.order = append(*r.order, "links")
	if r.err != nil {
		return nil, r.err
	}
	return append([]domain.Link(nil), r.links...), nil
}

// fakeSourceMapMaterializationPositionRepo records source-position reads and target-position writes.
type fakeSourceMapMaterializationPositionRepo struct {
	order          *[]string
	positions      map[uuid.UUID][]domain.DevicePosition
	savedPositions []domain.DevicePosition
	loadErr        error
	saveErr        error
}

// GetAllForMap records source-position loading and returns configured positions.
func (r *fakeSourceMapMaterializationPositionRepo) GetAllForMap(mapID uuid.UUID) ([]domain.DevicePosition, error) {
	*r.order = append(*r.order, "positions")
	if r.loadErr != nil {
		return nil, r.loadErr
	}
	return append([]domain.DevicePosition(nil), r.positions[mapID]...), nil
}

// SaveAllForMap records target-position saves and can inject persistence failures.
func (r *fakeSourceMapMaterializationPositionRepo) SaveAllForMap(_ uuid.UUID, positions []domain.DevicePosition) error {
	*r.order = append(*r.order, "save_positions")
	if r.saveErr != nil {
		return r.saveErr
	}
	r.savedPositions = append([]domain.DevicePosition(nil), positions...)
	return nil
}
