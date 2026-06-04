package canvasmap

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/domain"
)

// TestLoadTopologyBuildsMaterializedResponsePlan characterizes the saved-map topology projection used by the API handler.
func TestLoadTopologyBuildsMaterializedResponsePlan(t *testing.T) {
	mapID := uuid.New()
	baseID := uuid.New()
	ghostID := uuid.New()
	linkID := uuid.New()
	areaID := uuid.New()
	visualColor := "#112233"
	maps := &fakeTopologyMapRepo{
		byID: map[uuid.UUID]domain.CanvasMap{
			mapID: {ID: mapID, MembershipMaterialized: true},
		},
		memberships: map[uuid.UUID]domain.CanvasMapMembership{
			mapID: {
				Devices: []domain.CanvasMapDeviceMembership{
					{
						DeviceID:    baseID,
						Role:        domain.CanvasMapDeviceRoleBase,
						AreaIDs:     []uuid.UUID{areaID},
						VisualColor: &visualColor,
					},
					{DeviceID: ghostID, Role: domain.CanvasMapDeviceRoleGhost},
				},
				LinkIDs: []uuid.UUID{linkID},
				Areas: []domain.CanvasMapAreaMembership{
					{AreaID: areaID, Name: "Core", Description: "core", Color: "#00E676"},
				},
			},
		},
	}
	devices := &fakeTopologyDeviceService{
		devices: []domain.Device{
			{ID: baseID, DeviceType: domain.DeviceTypeRouter},
			{ID: ghostID, DeviceType: domain.DeviceTypeSwitch},
		},
	}
	links := &fakeTopologyLinkRepo{
		links: []domain.Link{{ID: linkID, SourceDeviceID: baseID, TargetDeviceID: ghostID}},
	}
	positions := &fakeTopologyPositionRepo{
		positions: map[uuid.UUID][]domain.DevicePosition{
			mapID: {
				{DeviceID: baseID, X: 10, Y: 20, Pinned: true},
				{DeviceID: ghostID, X: 30, Y: 40},
				{DeviceID: uuid.New(), X: 50, Y: 60},
			},
		},
	}

	loaded, err := LoadTopology(context.Background(), mapID, TopologyLoadDeps{
		Maps:      maps,
		Positions: positions,
		Devices:   devices,
		Links:     links,
	})

	if err != nil {
		t.Fatalf("LoadTopology() error = %v", err)
	}
	if loaded.Map.ID != mapID {
		t.Fatalf("loaded map = %s, want %s", loaded.Map.ID, mapID)
	}
	if loaded.Plan.DeviceCount != 1 || loaded.Plan.LinkCount != 1 || loaded.Plan.PositionCount != 2 {
		t.Fatalf("loaded counts = devices %d links %d positions %d, want 1/1/2", loaded.Plan.DeviceCount, loaded.Plan.LinkCount, loaded.Plan.PositionCount)
	}
	if got, want := deviceIDs(loaded.Plan.Devices), []uuid.UUID{baseID, ghostID}; !uuidSlicesEqual(got, want) {
		t.Fatalf("loaded devices = %v, want %v", got, want)
	}
	if len(loaded.Plan.Areas) != 1 || loaded.Plan.Areas[0].DeviceCount != 1 {
		t.Fatalf("loaded areas = %+v, want core count for base device", loaded.Plan.Areas)
	}
	if loaded.Plan.VisualColors[baseID] != visualColor {
		t.Fatalf("visual color = %q, want %q", loaded.Plan.VisualColors[baseID], visualColor)
	}
}

// TestLoadTopologyReturnsEmptyPlanForUnmaterializedMapAfterLoadingPositions preserves the existing load order for legacy maps.
func TestLoadTopologyReturnsEmptyPlanForUnmaterializedMapAfterLoadingPositions(t *testing.T) {
	mapID := uuid.New()
	maps := &fakeTopologyMapRepo{
		byID: map[uuid.UUID]domain.CanvasMap{
			mapID: {ID: mapID, MembershipMaterialized: false},
		},
		memberships: map[uuid.UUID]domain.CanvasMapMembership{mapID: {}},
	}
	positions := &fakeTopologyPositionRepo{
		positions: map[uuid.UUID][]domain.DevicePosition{
			mapID: {{DeviceID: uuid.New(), X: 10, Y: 20}},
		},
	}

	loaded, err := LoadTopology(context.Background(), mapID, TopologyLoadDeps{
		Maps:      maps,
		Positions: positions,
		Devices:   &fakeTopologyDeviceService{},
		Links:     &fakeTopologyLinkRepo{},
	})

	if err != nil {
		t.Fatalf("LoadTopology() error = %v", err)
	}
	if positions.getAllCalls != 1 {
		t.Fatalf("position loads = %d, want 1 to preserve existing load order", positions.getAllCalls)
	}
	if loaded.Plan.DeviceCount != 0 || loaded.Plan.LinkCount != 0 || loaded.Plan.PositionCount != 0 {
		t.Fatalf("empty counts = devices %d links %d positions %d, want zero", loaded.Plan.DeviceCount, loaded.Plan.LinkCount, loaded.Plan.PositionCount)
	}
	if len(loaded.Plan.Devices) != 0 || len(loaded.Plan.Links) != 0 || len(loaded.Plan.Positions) != 0 || len(loaded.Plan.Areas) != 0 {
		t.Fatalf("empty plan = %+v, want empty slices", loaded.Plan)
	}
}

// TestLoadTopologyWrapsPositionErrorsWithStage keeps HTTP error mapping independent from repository error text.
func TestLoadTopologyWrapsPositionErrorsWithStage(t *testing.T) {
	mapID := uuid.New()
	wantErr := errors.New("positions unavailable")

	_, err := LoadTopology(context.Background(), mapID, TopologyLoadDeps{
		Maps: &fakeTopologyMapRepo{
			byID:        map[uuid.UUID]domain.CanvasMap{mapID: {ID: mapID}},
			memberships: map[uuid.UUID]domain.CanvasMapMembership{mapID: {}},
		},
		Positions: &fakeTopologyPositionRepo{err: wantErr},
		Devices:   &fakeTopologyDeviceService{},
		Links:     &fakeTopologyLinkRepo{},
	})

	var loadErr TopologyLoadError
	if !errors.As(err, &loadErr) {
		t.Fatalf("LoadTopology() error = %T %[1]v, want TopologyLoadError", err)
	}
	if loadErr.Stage != TopologyLoadStagePositions {
		t.Fatalf("LoadTopology() stage = %s, want %s", loadErr.Stage, TopologyLoadStagePositions)
	}
	if !errors.Is(err, wantErr) {
		t.Fatalf("LoadTopology() error = %v, want wrapped %v", err, wantErr)
	}
}

type fakeTopologyMapRepo struct {
	byID        map[uuid.UUID]domain.CanvasMap
	memberships map[uuid.UUID]domain.CanvasMapMembership
}

// List returns the fake map set used by virtual-device isolation.
func (r *fakeTopologyMapRepo) List() ([]domain.CanvasMap, error) {
	maps := make([]domain.CanvasMap, 0, len(r.byID))
	for _, canvasMap := range r.byID {
		maps = append(maps, canvasMap)
	}
	return maps, nil
}

// GetByID returns the post-isolation map snapshot.
func (r *fakeTopologyMapRepo) GetByID(id uuid.UUID) (domain.CanvasMap, error) {
	return r.byID[id], nil
}

// GetMembership returns materialized membership for isolation and topology projection.
func (r *fakeTopologyMapRepo) GetMembership(id uuid.UUID) (domain.CanvasMapMembership, error) {
	return r.memberships[id], nil
}

// ReplaceMembership records isolation rewrites when a test triggers them.
func (r *fakeTopologyMapRepo) ReplaceMembership(id uuid.UUID, membership domain.CanvasMapMembership) error {
	r.memberships[id] = membership
	return nil
}

type fakeTopologyDeviceService struct {
	devices []domain.Device
}

// GetDevicesByIDs returns fake devices for both isolation checks and topology projection.
func (s *fakeTopologyDeviceService) GetDevicesByIDs(context.Context, []uuid.UUID) ([]domain.Device, error) {
	return append([]domain.Device(nil), s.devices...), nil
}

// AddDevice satisfies the isolation dependency without exercising clone behavior in topology tests.
func (s *fakeTopologyDeviceService) AddDevice(
	context.Context,
	string,
	string,
	domain.DeviceType,
	domain.SNMPCredentials,
	map[string]string,
	string,
	domain.MetricsSource,
	string,
	string,
	domain.TopologyDiscoveryMode,
	[]uuid.UUID,
	...*string,
) (*domain.Device, error) {
	return &domain.Device{ID: uuid.New(), DeviceType: domain.DeviceTypeVirtual}, nil
}

// UpdateClonedVirtualDevice satisfies the isolation dependency for non-cloning topology tests.
func (s *fakeTopologyDeviceService) UpdateClonedVirtualDevice(context.Context, uuid.UUID, VirtualDeviceCloneUpdate) error {
	return nil
}

// GetDevice satisfies the isolation dependency for non-cloning topology tests.
func (s *fakeTopologyDeviceService) GetDevice(context.Context, uuid.UUID) (*domain.Device, error) {
	return &domain.Device{ID: uuid.New(), DeviceType: domain.DeviceTypeVirtual}, nil
}

type fakeTopologyLinkRepo struct {
	links []domain.Link
}

// GetAll provides link rows for LoadLinksByIDs fallback behavior.
func (r *fakeTopologyLinkRepo) GetAll() ([]domain.Link, error) {
	return append([]domain.Link(nil), r.links...), nil
}

// Create satisfies the isolation dependency when tests do not need cloned links.
func (r *fakeTopologyLinkRepo) Create(*domain.Link) error {
	return nil
}

type fakeTopologyPositionRepo struct {
	positions   map[uuid.UUID][]domain.DevicePosition
	err         error
	getAllCalls int
}

// GetAllForMap records position-load ordering and can inject stage-specific failures.
func (r *fakeTopologyPositionRepo) GetAllForMap(mapID uuid.UUID) ([]domain.DevicePosition, error) {
	r.getAllCalls++
	if r.err != nil {
		return nil, r.err
	}
	return append([]domain.DevicePosition(nil), r.positions[mapID]...), nil
}

// SaveAllForMap satisfies the isolation dependency when tests do not save positions.
func (r *fakeTopologyPositionRepo) SaveAllForMap(uuid.UUID, []domain.DevicePosition) error {
	return nil
}
