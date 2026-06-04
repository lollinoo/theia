package canvasmap

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/domain"
)

// TestReplaceMaterializedMembershipProjectsAndPersistsCurrentTopology characterizes the current handler materialization workflow.
func TestReplaceMaterializedMembershipProjectsAndPersistsCurrentTopology(t *testing.T) {
	mapID := uuid.New()
	areaID := uuid.New()
	otherAreaID := uuid.New()
	baseID := uuid.New()
	ghostID := uuid.New()
	excludedID := uuid.New()
	linkID := uuid.New()
	excludedLinkID := uuid.New()
	order := []string{}
	maps := &fakeMaterializationMapRepo{order: &order}

	err := ReplaceMaterializedMembership(context.Background(), mapID, domain.CanvasMapFilter{
		AreaID:                &areaID,
		IncludeCrossAreaLinks: true,
		IncludeGhostDevices:   true,
	}, MaterializationDeps{
		Maps: maps,
		Devices: &fakeMaterializationDeviceService{
			order: &order,
			devices: []domain.Device{
				{ID: baseID, AreaIDs: []uuid.UUID{areaID, otherAreaID}},
				{ID: ghostID, AreaIDs: []uuid.UUID{otherAreaID}},
				{ID: excludedID, AreaIDs: []uuid.UUID{otherAreaID}},
			},
		},
		Links: &fakeMaterializationLinkRepo{
			order: &order,
			links: []domain.Link{
				{ID: linkID, SourceDeviceID: baseID, TargetDeviceID: ghostID},
				{ID: excludedLinkID, SourceDeviceID: ghostID, TargetDeviceID: excludedID},
			},
		},
		Areas: &fakeMaterializationAreaRepo{
			order: &order,
			areas: []domain.AreaWithCount{
				{Area: domain.Area{ID: areaID, Name: "Core", Description: "Core area", Color: "#00E676"}},
				{Area: domain.Area{ID: otherAreaID, Name: "Edge", Description: "Edge area", Color: "#2979FF"}},
			},
		},
	})

	if err != nil {
		t.Fatalf("ReplaceMaterializedMembership() error = %v", err)
	}
	if got, want := order, []string{"devices", "links", "areas", "replace"}; !stringSlicesEqual(got, want) {
		t.Fatalf("operation order = %v, want %v", got, want)
	}
	if maps.replacedMapID != mapID {
		t.Fatalf("replaced map ID = %s, want %s", maps.replacedMapID, mapID)
	}
	assertMaterializedMembership(t, maps.replacedMembership, baseID, ghostID, linkID, areaID)
}

// TestReplaceMaterializedMembershipWrapsEachIOStage keeps adapter error mapping stable after extraction.
func TestReplaceMaterializedMembershipWrapsEachIOStage(t *testing.T) {
	assertMaterializationStageError(t, MaterializationStageDevices, errors.New("devices unavailable"))
	assertMaterializationStageError(t, MaterializationStageLinks, errors.New("links unavailable"))
	assertMaterializationStageError(t, MaterializationStageAreas, errors.New("areas unavailable"))
	assertMaterializationStageError(t, MaterializationStageReplace, errors.New("replace unavailable"))
}

// assertMaterializationStageError verifies that a failing dependency is wrapped with the expected stage and original error.
func assertMaterializationStageError(t *testing.T, stage MaterializationStage, wantErr error) {
	t.Helper()
	mapID := uuid.New()
	order := []string{}
	maps := &fakeMaterializationMapRepo{order: &order}
	devices := &fakeMaterializationDeviceService{order: &order, devices: []domain.Device{{ID: uuid.New()}}}
	links := &fakeMaterializationLinkRepo{order: &order, links: []domain.Link{}}
	areas := &fakeMaterializationAreaRepo{order: &order, areas: []domain.AreaWithCount{}}

	switch stage {
	case MaterializationStageDevices:
		devices.err = wantErr
	case MaterializationStageLinks:
		links.err = wantErr
	case MaterializationStageAreas:
		areas.err = wantErr
	case MaterializationStageReplace:
		maps.err = wantErr
	default:
		t.Fatalf("unsupported materialization stage %s", stage)
	}

	err := ReplaceMaterializedMembership(context.Background(), mapID, domain.CanvasMapFilter{}, MaterializationDeps{
		Maps:    maps,
		Devices: devices,
		Links:   links,
		Areas:   areas,
	})

	var materializationErr MaterializationError
	if !errors.As(err, &materializationErr) {
		t.Fatalf("ReplaceMaterializedMembership() error = %T %[1]v, want MaterializationError", err)
	}
	if materializationErr.Stage != stage {
		t.Fatalf("ReplaceMaterializedMembership() stage = %s, want %s", materializationErr.Stage, stage)
	}
	if !errors.Is(err, wantErr) {
		t.Fatalf("ReplaceMaterializedMembership() error = %v, want wrapped %v", err, wantErr)
	}
}

// assertMaterializedMembership checks the projected membership shape persisted by the orchestrator.
func assertMaterializedMembership(
	t *testing.T,
	membership domain.CanvasMapMembership,
	baseID uuid.UUID,
	ghostID uuid.UUID,
	linkID uuid.UUID,
	areaID uuid.UUID,
) {
	t.Helper()
	if got, want := len(membership.Devices), 2; got != want {
		t.Fatalf("device membership count = %d, want %d", got, want)
	}
	if membership.Devices[0].DeviceID != baseID || membership.Devices[0].Role != domain.CanvasMapDeviceRoleBase {
		t.Fatalf("first member = %+v, want base device %s", membership.Devices[0], baseID)
	}
	if got, want := membership.Devices[0].AreaIDs, []uuid.UUID{areaID}; !uuidSlicesEqual(got, want) {
		t.Fatalf("base area IDs = %v, want %v", got, want)
	}
	if membership.Devices[1].DeviceID != ghostID || membership.Devices[1].Role != domain.CanvasMapDeviceRoleGhost {
		t.Fatalf("second member = %+v, want ghost device %s", membership.Devices[1], ghostID)
	}
	if got, want := membership.LinkIDs, []uuid.UUID{linkID}; !uuidSlicesEqual(got, want) {
		t.Fatalf("link IDs = %v, want %v", got, want)
	}
	if len(membership.Areas) != 1 || membership.Areas[0].AreaID != areaID || membership.Areas[0].Name != "Core" {
		t.Fatalf("areas = %+v, want Core area snapshot", membership.Areas)
	}
}

// stringSlicesEqual compares operation-order traces without importing extra assertion helpers.
func stringSlicesEqual(a []string, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for idx := range a {
		if a[idx] != b[idx] {
			return false
		}
	}
	return true
}

// fakeMaterializationMapRepo records ReplaceMembership calls and can inject persistence errors.
type fakeMaterializationMapRepo struct {
	order              *[]string
	err                error
	replacedMapID      uuid.UUID
	replacedMembership domain.CanvasMapMembership
}

// ReplaceMembership captures the materialized membership passed to persistence.
func (r *fakeMaterializationMapRepo) ReplaceMembership(id uuid.UUID, membership domain.CanvasMapMembership) error {
	*r.order = append(*r.order, "replace")
	if r.err != nil {
		return r.err
	}
	r.replacedMapID = id
	r.replacedMembership = membership
	return nil
}

// fakeMaterializationDeviceService supplies device rows for materialization tests.
type fakeMaterializationDeviceService struct {
	order   *[]string
	devices []domain.Device
	err     error
}

// GetAllDevices records device loading and returns the configured fake device set.
func (s *fakeMaterializationDeviceService) GetAllDevices(context.Context) ([]domain.Device, error) {
	*s.order = append(*s.order, "devices")
	if s.err != nil {
		return nil, s.err
	}
	return append([]domain.Device(nil), s.devices...), nil
}

// fakeMaterializationLinkRepo supplies link rows for materialization tests.
type fakeMaterializationLinkRepo struct {
	order *[]string
	links []domain.Link
	err   error
}

// GetAll records link loading and returns the configured fake link set.
func (r *fakeMaterializationLinkRepo) GetAll() ([]domain.Link, error) {
	*r.order = append(*r.order, "links")
	if r.err != nil {
		return nil, r.err
	}
	return append([]domain.Link(nil), r.links...), nil
}

// fakeMaterializationAreaRepo supplies area snapshots for materialization tests.
type fakeMaterializationAreaRepo struct {
	order *[]string
	areas []domain.AreaWithCount
	err   error
}

// GetAllWithDeviceCount records area loading and returns the configured fake area set.
func (r *fakeMaterializationAreaRepo) GetAllWithDeviceCount() ([]domain.AreaWithCount, error) {
	*r.order = append(*r.order, "areas")
	if r.err != nil {
		return nil, r.err
	}
	return append([]domain.AreaWithCount(nil), r.areas...), nil
}
