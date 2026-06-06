package canvasmap

// This file exercises default position copy behavior so refactors preserve the documented contract.

import (
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/domain"
)

// TestCopyDefaultPositionsForMaterializedMembershipCopiesMemberPositions preserves default-map position copying.
func TestCopyDefaultPositionsForMaterializedMembershipCopiesMemberPositions(t *testing.T) {
	mapID := uuid.New()
	defaultMapID := uuid.New()
	memberID := uuid.New()
	prunedID := uuid.New()
	order := []string{}
	positions := &fakeDefaultPositionCopyPositionRepo{
		order: &order,
		positions: map[uuid.UUID][]domain.DevicePosition{
			defaultMapID: {
				{DeviceID: memberID, X: 10, Y: 20, Pinned: true},
				{DeviceID: prunedID, X: 30, Y: 40},
			},
		},
	}

	err := CopyDefaultPositionsForMaterializedMembership(mapID, DefaultPositionCopyDeps{
		Maps: &fakeDefaultPositionCopyMapRepo{
			order:      &order,
			defaultMap: domain.CanvasMap{ID: defaultMapID},
			membership: domain.CanvasMapMembership{
				Devices: []domain.CanvasMapDeviceMembership{{DeviceID: memberID, Role: domain.CanvasMapDeviceRoleBase}},
			},
		},
		Positions: positions,
	})

	if err != nil {
		t.Fatalf("CopyDefaultPositionsForMaterializedMembership() error = %v", err)
	}
	if got, want := order, []string{"default", "membership", "positions", "save_positions"}; !stringSlicesEqual(got, want) {
		t.Fatalf("operation order = %v, want %v", got, want)
	}
	if got, want := devicePositionIDs(positions.savedPositions), []uuid.UUID{memberID}; !uuidSlicesEqual(got, want) {
		t.Fatalf("saved position IDs = %v, want %v", got, want)
	}
}

// TestCopyDefaultPositionsForMaterializedMembershipSkipsDefaultMap preserves the default-map no-op path.
func TestCopyDefaultPositionsForMaterializedMembershipSkipsDefaultMap(t *testing.T) {
	mapID := uuid.New()
	order := []string{}

	err := CopyDefaultPositionsForMaterializedMembership(mapID, DefaultPositionCopyDeps{
		Maps: &fakeDefaultPositionCopyMapRepo{
			order:      &order,
			defaultMap: domain.CanvasMap{ID: mapID},
		},
		Positions: &fakeDefaultPositionCopyPositionRepo{order: &order},
	})

	if err != nil {
		t.Fatalf("CopyDefaultPositionsForMaterializedMembership() error = %v", err)
	}
	if got, want := order, []string{"default"}; !stringSlicesEqual(got, want) {
		t.Fatalf("operation order = %v, want %v", got, want)
	}
}

// TestCopyDefaultPositionsForMaterializedMembershipFallsBackToLegacy preserves legacy position fallback behavior.
func TestCopyDefaultPositionsForMaterializedMembershipFallsBackToLegacy(t *testing.T) {
	mapID := uuid.New()
	defaultMapID := uuid.New()
	memberID := uuid.New()
	order := []string{}
	positions := &fakeDefaultPositionCopyPositionRepo{
		order:     &order,
		positions: map[uuid.UUID][]domain.DevicePosition{defaultMapID: {}},
	}

	err := CopyDefaultPositionsForMaterializedMembership(mapID, DefaultPositionCopyDeps{
		Maps: &fakeDefaultPositionCopyMapRepo{
			order:      &order,
			defaultMap: domain.CanvasMap{ID: defaultMapID},
			membership: domain.CanvasMapMembership{
				Devices: []domain.CanvasMapDeviceMembership{{DeviceID: memberID, Role: domain.CanvasMapDeviceRoleBase}},
			},
		},
		Positions: positions,
		LegacyPositions: &fakeDefaultPositionCopyLegacyRepo{
			order:     &order,
			positions: []domain.DevicePosition{{DeviceID: memberID, X: 50, Y: 60}},
		},
	})

	if err != nil {
		t.Fatalf("CopyDefaultPositionsForMaterializedMembership() error = %v", err)
	}
	if got, want := order, []string{"default", "membership", "positions", "legacy", "save_positions"}; !stringSlicesEqual(got, want) {
		t.Fatalf("operation order = %v, want %v", got, want)
	}
	if got, want := devicePositionIDs(positions.savedPositions), []uuid.UUID{memberID}; !uuidSlicesEqual(got, want) {
		t.Fatalf("saved position IDs = %v, want %v", got, want)
	}
}

// TestCopyDefaultPositionsForMaterializedMembershipWrapsEachIOStage keeps HTTP mapping stable after extraction.
func TestCopyDefaultPositionsForMaterializedMembershipWrapsEachIOStage(t *testing.T) {
	assertDefaultPositionCopyStageError(t, DefaultPositionCopyStageDefaultMap, errors.New("default unavailable"))
	assertDefaultPositionCopyStageError(t, DefaultPositionCopyStageMembership, errors.New("membership unavailable"))
	assertDefaultPositionCopyStageError(t, DefaultPositionCopyStagePositions, errors.New("positions unavailable"))
	assertDefaultPositionCopyStageError(t, DefaultPositionCopyStageLegacyPositions, errors.New("legacy unavailable"))
	assertDefaultPositionCopyStageError(t, DefaultPositionCopyStageSavePositions, errors.New("save unavailable"))
}

// assertDefaultPositionCopyStageError verifies stage wrapping for one default-position copy dependency failure.
func assertDefaultPositionCopyStageError(t *testing.T, stage DefaultPositionCopyStage, wantErr error) {
	t.Helper()
	mapID := uuid.New()
	defaultMapID := uuid.New()
	memberID := uuid.New()
	order := []string{}
	maps := &fakeDefaultPositionCopyMapRepo{
		order:      &order,
		defaultMap: domain.CanvasMap{ID: defaultMapID},
		membership: domain.CanvasMapMembership{
			Devices: []domain.CanvasMapDeviceMembership{{DeviceID: memberID, Role: domain.CanvasMapDeviceRoleBase}},
		},
	}
	positions := &fakeDefaultPositionCopyPositionRepo{
		order: &order,
		positions: map[uuid.UUID][]domain.DevicePosition{
			defaultMapID: {{DeviceID: memberID, X: 10, Y: 20}},
		},
	}
	legacy := &fakeDefaultPositionCopyLegacyRepo{order: &order}

	switch stage {
	case DefaultPositionCopyStageDefaultMap:
		maps.defaultErr = wantErr
	case DefaultPositionCopyStageMembership:
		maps.membershipErr = wantErr
	case DefaultPositionCopyStagePositions:
		positions.loadErr = wantErr
	case DefaultPositionCopyStageLegacyPositions:
		positions.positions[defaultMapID] = []domain.DevicePosition{}
		legacy.err = wantErr
	case DefaultPositionCopyStageSavePositions:
		positions.saveErr = wantErr
	default:
		t.Fatalf("unsupported default-position copy stage %s", stage)
	}

	err := CopyDefaultPositionsForMaterializedMembership(mapID, DefaultPositionCopyDeps{
		Maps:            maps,
		Positions:       positions,
		LegacyPositions: legacy,
	})

	var copyErr DefaultPositionCopyError
	if !errors.As(err, &copyErr) {
		t.Fatalf("CopyDefaultPositionsForMaterializedMembership() error = %T %[1]v, want DefaultPositionCopyError", err)
	}
	if copyErr.Stage != stage {
		t.Fatalf("CopyDefaultPositionsForMaterializedMembership() stage = %s, want %s", copyErr.Stage, stage)
	}
	if !errors.Is(err, wantErr) {
		t.Fatalf("CopyDefaultPositionsForMaterializedMembership() error = %v, want wrapped %v", err, wantErr)
	}
}

// fakeDefaultPositionCopyMapRepo records default map and membership reads.
type fakeDefaultPositionCopyMapRepo struct {
	order         *[]string
	defaultMap    domain.CanvasMap
	membership    domain.CanvasMapMembership
	defaultErr    error
	membershipErr error
}

func (r *fakeDefaultPositionCopyMapRepo) GetDefault() (domain.CanvasMap, error) {
	*r.order = append(*r.order, "default")
	if r.defaultErr != nil {
		return domain.CanvasMap{}, r.defaultErr
	}
	return r.defaultMap, nil
}

func (r *fakeDefaultPositionCopyMapRepo) GetMembership(uuid.UUID) (domain.CanvasMapMembership, error) {
	*r.order = append(*r.order, "membership")
	if r.membershipErr != nil {
		return domain.CanvasMapMembership{}, r.membershipErr
	}
	return r.membership, nil
}

// fakeDefaultPositionCopyPositionRepo records default position reads and target position writes.
type fakeDefaultPositionCopyPositionRepo struct {
	order          *[]string
	positions      map[uuid.UUID][]domain.DevicePosition
	savedPositions []domain.DevicePosition
	loadErr        error
	saveErr        error
}

func (r *fakeDefaultPositionCopyPositionRepo) GetAllForMap(mapID uuid.UUID) ([]domain.DevicePosition, error) {
	*r.order = append(*r.order, "positions")
	if r.loadErr != nil {
		return nil, r.loadErr
	}
	return append([]domain.DevicePosition(nil), r.positions[mapID]...), nil
}

// SaveAllForMap records target position saves or returns injected save error.
func (r *fakeDefaultPositionCopyPositionRepo) SaveAllForMap(_ uuid.UUID, positions []domain.DevicePosition) error {
	*r.order = append(*r.order, "save_positions")
	if r.saveErr != nil {
		return r.saveErr
	}
	r.savedPositions = append([]domain.DevicePosition(nil), positions...)
	return nil
}

// fakeDefaultPositionCopyLegacyRepo records legacy position fallback reads.
type fakeDefaultPositionCopyLegacyRepo struct {
	order     *[]string
	positions []domain.DevicePosition
	err       error
}

func (r *fakeDefaultPositionCopyLegacyRepo) GetAll() ([]domain.DevicePosition, error) {
	*r.order = append(*r.order, "legacy")
	if r.err != nil {
		return nil, r.err
	}
	return append([]domain.DevicePosition(nil), r.positions...), nil
}
