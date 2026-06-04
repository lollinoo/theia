package canvasmap

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/domain"
)

func TestIsolateVirtualDevicesClonesSharedVirtualDeviceAndRemapsMembershipLinksAndPositions(t *testing.T) {
	ctx := context.Background()
	mapID := uuid.New()
	otherMapID := uuid.New()
	virtualID := uuid.New()
	baseID := uuid.New()
	cloneID := uuid.New()
	sourceLinkID := uuid.New()
	clonedLinkID := uuid.New()
	areaID := uuid.New()
	color := "#112233"
	pollOverride := 45
	pollingEnabled := false
	notes := "source note"
	sourceMembership := domain.CanvasMapMembership{
		Devices: []domain.CanvasMapDeviceMembership{
			{
				DeviceID:    virtualID,
				Role:        domain.CanvasMapDeviceRoleBase,
				AreaIDs:     []uuid.UUID{areaID},
				VisualColor: &color,
			},
			{DeviceID: baseID, Role: domain.CanvasMapDeviceRoleBase},
		},
		LinkIDs: []uuid.UUID{sourceLinkID},
		Areas: []domain.CanvasMapAreaMembership{
			{AreaID: areaID, Name: "Core", Description: "core", Color: "#00E676"},
		},
	}
	maps := &fakeVirtualIsolationMapRepo{
		maps: []domain.CanvasMap{
			{ID: mapID},
			{ID: otherMapID},
		},
		memberships: map[uuid.UUID]domain.CanvasMapMembership{
			mapID: sourceMembership,
			otherMapID: {
				Devices: []domain.CanvasMapDeviceMembership{
					{DeviceID: virtualID, Role: domain.CanvasMapDeviceRoleBase},
				},
			},
		},
	}
	devices := &fakeVirtualIsolationDeviceService{
		devices: []domain.Device{
			{
				ID:                    virtualID,
				IP:                    "10.0.0.10",
				Hostname:              "virtual-edge",
				Notes:                 &notes,
				DeviceType:            domain.DeviceTypeVirtual,
				Tags:                  map[string]string{"role": "edge"},
				Vendor:                "virtual-vendor",
				MetricsSource:         domain.MetricsSourceSNMP,
				PrometheusLabelName:   "device",
				PrometheusLabelValue:  "virtual-edge",
				TopologyDiscoveryMode: domain.TopologyDiscoveryModeOff,
				PollIntervalOverride:  &pollOverride,
				PollingEnabled:        &pollingEnabled,
			},
			{ID: baseID, DeviceType: domain.DeviceTypeRouter},
		},
		clone: domain.Device{ID: cloneID, DeviceType: domain.DeviceTypeVirtual},
	}
	links := &fakeVirtualIsolationLinkRepo{
		links: []domain.Link{
			{
				ID:                sourceLinkID,
				SourceDeviceID:    virtualID,
				SourceIfName:      "eth0",
				TargetDeviceID:    baseID,
				TargetIfName:      "eth1",
				DiscoveryProtocol: domain.DiscoveryProtocolManual,
			},
		},
		nextID: clonedLinkID,
	}
	positions := &fakeVirtualIsolationPositionRepo{
		positions: map[uuid.UUID][]domain.DevicePosition{
			mapID: {
				{DeviceID: virtualID, X: 10, Y: 20, Pinned: true},
				{DeviceID: baseID, X: 30, Y: 40},
			},
		},
	}

	err := IsolateVirtualDevices(ctx, mapID, VirtualIsolationDeps{
		Maps:      maps,
		Positions: positions,
		Devices:   devices,
		Links:     links,
	})

	if err != nil {
		t.Fatalf("IsolateVirtualDevices() error = %v", err)
	}
	if len(devices.added) != 1 {
		t.Fatalf("added clones = %d, want 1", len(devices.added))
	}
	added := devices.added[0]
	if added.ip != "10.0.0.10" || added.hostname != "virtual-edge" || added.deviceType != domain.DeviceTypeVirtual {
		t.Fatalf("added clone = %+v, want source virtual device details", added)
	}
	if added.metricsSource != domain.MetricsSourceNone {
		t.Fatalf("added clone metrics source = %s, want none", added.metricsSource)
	}
	devices.devices[0].Tags["role"] = "mutated"
	if added.tags["role"] != "edge" {
		t.Fatalf("added clone tags = %#v, want copied tags", added.tags)
	}
	if len(added.notes) != 1 || added.notes[0] == nil || *added.notes[0] != notes || added.notes[0] == &notes {
		t.Fatalf("added clone notes = %#v, want copied source note", added.notes)
	}
	if devices.update.PollIntervalOverride == nil || *devices.update.PollIntervalOverride == nil || **devices.update.PollIntervalOverride != pollOverride {
		t.Fatalf("clone poll override update = %#v, want %d", devices.update.PollIntervalOverride, pollOverride)
	}
	if devices.update.PollingEnabled == nil || *devices.update.PollingEnabled != pollingEnabled {
		t.Fatalf("clone polling update = %#v, want %v", devices.update.PollingEnabled, pollingEnabled)
	}
	if len(links.created) != 1 {
		t.Fatalf("created links = %d, want 1", len(links.created))
	}
	createdLink := links.created[0]
	if createdLink.SourceDeviceID != cloneID || createdLink.TargetDeviceID != baseID ||
		createdLink.SourceIfName != "eth0" || createdLink.TargetIfName != "eth1" ||
		createdLink.DiscoveryProtocol != domain.DiscoveryProtocolManual {
		t.Fatalf("created link = %+v, want remapped source with link details preserved", createdLink)
	}
	gotMembership := maps.replaced[mapID]
	if len(gotMembership.Devices) != 2 || gotMembership.Devices[0].DeviceID != cloneID || gotMembership.Devices[1].DeviceID != baseID {
		t.Fatalf("replaced membership devices = %+v, want clone then base", gotMembership.Devices)
	}
	if gotMembership.Devices[0].VisualColor == nil || *gotMembership.Devices[0].VisualColor != color {
		t.Fatalf("clone visual color = %v, want %s", gotMembership.Devices[0].VisualColor, color)
	}
	if !uuidSlicesEqual(gotMembership.LinkIDs, []uuid.UUID{clonedLinkID}) {
		t.Fatalf("replaced membership links = %v, want cloned link %s", gotMembership.LinkIDs, clonedLinkID)
	}
	if len(gotMembership.Areas) != 1 || gotMembership.Areas[0].AreaID != areaID {
		t.Fatalf("replaced membership areas = %+v, want original area", gotMembership.Areas)
	}
	gotPositions := positions.saved[mapID]
	if len(gotPositions) != 2 || gotPositions[0].DeviceID != cloneID || gotPositions[1].DeviceID != baseID {
		t.Fatalf("saved positions = %+v, want virtual position remapped to clone and base kept", gotPositions)
	}
}

type fakeVirtualIsolationMapRepo struct {
	maps        []domain.CanvasMap
	memberships map[uuid.UUID]domain.CanvasMapMembership
	replaced    map[uuid.UUID]domain.CanvasMapMembership
}

func (r *fakeVirtualIsolationMapRepo) List() ([]domain.CanvasMap, error) {
	return append([]domain.CanvasMap(nil), r.maps...), nil
}

func (r *fakeVirtualIsolationMapRepo) GetMembership(id uuid.UUID) (domain.CanvasMapMembership, error) {
	return r.memberships[id], nil
}

func (r *fakeVirtualIsolationMapRepo) ReplaceMembership(id uuid.UUID, membership domain.CanvasMapMembership) error {
	if r.replaced == nil {
		r.replaced = map[uuid.UUID]domain.CanvasMapMembership{}
	}
	r.replaced[id] = membership
	return nil
}

type fakeVirtualIsolationDeviceService struct {
	devices []domain.Device
	clone   domain.Device
	added   []fakeVirtualIsolationAddedDevice
	update  VirtualDeviceCloneUpdate
}

type fakeVirtualIsolationAddedDevice struct {
	ip                    string
	hostname              string
	deviceType            domain.DeviceType
	tags                  map[string]string
	vendor                string
	metricsSource         domain.MetricsSource
	prometheusLabelName   string
	prometheusLabelValue  string
	topologyDiscoveryMode domain.TopologyDiscoveryMode
	areaIDs               []uuid.UUID
	notes                 []*string
}

func (s *fakeVirtualIsolationDeviceService) GetDevicesByIDs(context.Context, []uuid.UUID) ([]domain.Device, error) {
	return append([]domain.Device(nil), s.devices...), nil
}

func (s *fakeVirtualIsolationDeviceService) AddDevice(
	_ context.Context,
	ip string,
	hostname string,
	deviceType domain.DeviceType,
	_ domain.SNMPCredentials,
	tags map[string]string,
	vendor string,
	metricsSource domain.MetricsSource,
	prometheusLabelName string,
	prometheusLabelValue string,
	topologyDiscoveryMode domain.TopologyDiscoveryMode,
	areaIDs []uuid.UUID,
	notes ...*string,
) (*domain.Device, error) {
	s.added = append(s.added, fakeVirtualIsolationAddedDevice{
		ip:                    ip,
		hostname:              hostname,
		deviceType:            deviceType,
		tags:                  tags,
		vendor:                vendor,
		metricsSource:         metricsSource,
		prometheusLabelName:   prometheusLabelName,
		prometheusLabelValue:  prometheusLabelValue,
		topologyDiscoveryMode: topologyDiscoveryMode,
		areaIDs:               areaIDs,
		notes:                 notes,
	})
	return &s.clone, nil
}

func (s *fakeVirtualIsolationDeviceService) UpdateClonedVirtualDevice(_ context.Context, _ uuid.UUID, update VirtualDeviceCloneUpdate) error {
	s.update = update
	return nil
}

func (s *fakeVirtualIsolationDeviceService) GetDevice(context.Context, uuid.UUID) (*domain.Device, error) {
	return &s.clone, nil
}

type fakeVirtualIsolationLinkRepo struct {
	links   []domain.Link
	nextID  uuid.UUID
	created []domain.Link
}

func (r *fakeVirtualIsolationLinkRepo) GetAll() ([]domain.Link, error) {
	return append([]domain.Link(nil), r.links...), nil
}

func (r *fakeVirtualIsolationLinkRepo) Create(link *domain.Link) error {
	link.ID = r.nextID
	r.created = append(r.created, *link)
	return nil
}

type fakeVirtualIsolationPositionRepo struct {
	positions map[uuid.UUID][]domain.DevicePosition
	saved     map[uuid.UUID][]domain.DevicePosition
}

func (r *fakeVirtualIsolationPositionRepo) GetAllForMap(mapID uuid.UUID) ([]domain.DevicePosition, error) {
	return append([]domain.DevicePosition(nil), r.positions[mapID]...), nil
}

func (r *fakeVirtualIsolationPositionRepo) SaveAllForMap(mapID uuid.UUID, positions []domain.DevicePosition) error {
	if r.saved == nil {
		r.saved = map[uuid.UUID][]domain.DevicePosition{}
	}
	r.saved[mapID] = append([]domain.DevicePosition(nil), positions...)
	return nil
}
