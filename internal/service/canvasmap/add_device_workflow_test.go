package canvasmap

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/domain"
)

// TestAddDeviceToMaterializedMembershipAddsOnlyMissingLinksForExistingMember characterizes existing-member link inclusion.
func TestAddDeviceToMaterializedMembershipAddsOnlyMissingLinksForExistingMember(t *testing.T) {
	mapID := uuid.New()
	deviceID := uuid.New()
	baseID := uuid.New()
	existingLinkID := uuid.New()
	missingLinkID := uuid.New()
	areaID := uuid.New()
	order := []string{}
	maps := &fakeAddDeviceMapRepo{
		order: &order,
		membership: domain.CanvasMapMembership{
			Devices: []domain.CanvasMapDeviceMembership{
				{DeviceID: deviceID, Role: domain.CanvasMapDeviceRoleGhost},
				{DeviceID: baseID, Role: domain.CanvasMapDeviceRoleBase},
			},
			LinkIDs: []uuid.UUID{existingLinkID},
			Areas:   []domain.CanvasMapAreaMembership{{AreaID: areaID, Name: "Existing", Color: "#00E676"}},
		},
	}

	err := AddDeviceToMaterializedMembership(context.Background(), mapID, deviceID, true, AddDeviceMembershipDeps{
		Maps:    maps,
		Devices: &fakeAddDeviceService{order: &order, device: &domain.Device{ID: deviceID}},
		Links: &fakeAddDeviceLinkRepo{
			order: &order,
			links: []domain.Link{
				{ID: existingLinkID, SourceDeviceID: baseID, TargetDeviceID: deviceID},
				{ID: missingLinkID, SourceDeviceID: deviceID, TargetDeviceID: baseID},
			},
		},
		Areas: &fakeAddDeviceAreaRepo{order: &order},
	})

	if err != nil {
		t.Fatalf("AddDeviceToMaterializedMembership() error = %v", err)
	}
	if got, want := order, []string{"device", "membership", "links", "add"}; !stringSlicesEqual(got, want) {
		t.Fatalf("operation order = %v, want %v", got, want)
	}
	if maps.addedDevice.DeviceID != deviceID || maps.addedDevice.Role != domain.CanvasMapDeviceRoleGhost {
		t.Fatalf("added device = %+v, want existing ghost member", maps.addedDevice)
	}
	if got, want := maps.addedLinkIDs, []uuid.UUID{missingLinkID}; !uuidSlicesEqual(got, want) {
		t.Fatalf("added link IDs = %v, want %v", got, want)
	}
	if len(maps.addedAreas) != 1 || maps.addedAreas[0].AreaID != areaID {
		t.Fatalf("added areas = %+v, want existing map areas", maps.addedAreas)
	}
}

// TestAddDeviceToMaterializedMembershipRejectsDuplicateAddressBeforeLoadingLinks preserves duplicate short-circuit ordering.
func TestAddDeviceToMaterializedMembershipRejectsDuplicateAddressBeforeLoadingLinks(t *testing.T) {
	mapID := uuid.New()
	deviceID := uuid.New()
	order := []string{}
	maps := &fakeAddDeviceMapRepo{order: &order, membership: domain.CanvasMapMembership{}}

	err := AddDeviceToMaterializedMembership(context.Background(), mapID, deviceID, true, AddDeviceMembershipDeps{
		Maps: maps,
		Devices: &fakeAddDeviceService{
			order:         &order,
			device:        &domain.Device{ID: deviceID, IP: " Router.EXAMPLE.com "},
			memberDevices: []domain.Device{{ID: uuid.New(), IP: "router.example.com"}},
		},
		Links: &fakeAddDeviceLinkRepo{order: &order},
		Areas: &fakeAddDeviceAreaRepo{order: &order},
	})

	var duplicateErr DuplicateDeviceAddressError
	if !errors.As(err, &duplicateErr) {
		t.Fatalf("AddDeviceToMaterializedMembership() error = %T %[1]v, want DuplicateDeviceAddressError", err)
	}
	if got, want := order, []string{"device", "membership", "member_devices"}; !stringSlicesEqual(got, want) {
		t.Fatalf("operation order = %v, want %v", got, want)
	}
	if maps.addCalls != 0 {
		t.Fatalf("add calls = %d, want 0 after duplicate conflict", maps.addCalls)
	}
}

// TestAddDeviceToMaterializedMembershipWrapsDependencyStages keeps HTTP error mapping stable.
func TestAddDeviceToMaterializedMembershipWrapsDependencyStages(t *testing.T) {
	assertAddDeviceMembershipStageError(t, AddDeviceMembershipStageDevice, errors.New("device unavailable"))
	assertAddDeviceMembershipStageError(t, AddDeviceMembershipStageMembership, errors.New("membership unavailable"))
	assertAddDeviceMembershipStageError(t, AddDeviceMembershipStageMemberDevices, errors.New("member devices unavailable"))
	assertAddDeviceMembershipStageError(t, AddDeviceMembershipStageLinks, errors.New("links unavailable"))
	assertAddDeviceMembershipStageError(t, AddDeviceMembershipStageAreas, errors.New("areas unavailable"))
	assertAddDeviceMembershipStageError(t, AddDeviceMembershipStagePersist, errors.New("persist unavailable"))
}

// assertAddDeviceMembershipStageError verifies stage wrapping for one add-device dependency failure.
func assertAddDeviceMembershipStageError(t *testing.T, stage AddDeviceMembershipStage, wantErr error) {
	t.Helper()
	mapID := uuid.New()
	deviceID := uuid.New()
	baseID := uuid.New()
	areaID := uuid.New()
	linkID := uuid.New()
	order := []string{}
	maps := &fakeAddDeviceMapRepo{
		order: &order,
		membership: domain.CanvasMapMembership{
			Devices: []domain.CanvasMapDeviceMembership{{DeviceID: baseID, Role: domain.CanvasMapDeviceRoleBase}},
		},
	}
	devices := &fakeAddDeviceService{
		order:         &order,
		device:        &domain.Device{ID: deviceID, IP: "", AreaIDs: []uuid.UUID{areaID}},
		memberDevices: []domain.Device{{ID: baseID, IP: "10.0.0.1"}},
	}
	links := &fakeAddDeviceLinkRepo{
		order: &order,
		links: []domain.Link{{ID: linkID, SourceDeviceID: baseID, TargetDeviceID: deviceID}},
	}
	areas := &fakeAddDeviceAreaRepo{
		order: &order,
		areas: map[uuid.UUID]domain.Area{
			areaID: {ID: areaID, Name: "Area", Color: "#00E676"},
		},
	}
	includeConnectedLinks := false

	switch stage {
	case AddDeviceMembershipStageDevice:
		devices.getErr = wantErr
	case AddDeviceMembershipStageMembership:
		maps.membershipErr = wantErr
	case AddDeviceMembershipStageMemberDevices:
		devices.device.IP = "10.0.0.2"
		devices.byIDsErr = wantErr
	case AddDeviceMembershipStageLinks:
		maps.membership.Devices = append(maps.membership.Devices, domain.CanvasMapDeviceMembership{DeviceID: deviceID, Role: domain.CanvasMapDeviceRoleBase})
		includeConnectedLinks = true
		links.err = wantErr
	case AddDeviceMembershipStageAreas:
		areas.err = wantErr
	case AddDeviceMembershipStagePersist:
		maps.addErr = wantErr
	default:
		t.Fatalf("unsupported add-device stage %s", stage)
	}

	err := AddDeviceToMaterializedMembership(context.Background(), mapID, deviceID, includeConnectedLinks, AddDeviceMembershipDeps{
		Maps:    maps,
		Devices: devices,
		Links:   links,
		Areas:   areas,
	})

	var addErr AddDeviceMembershipError
	if !errors.As(err, &addErr) {
		t.Fatalf("AddDeviceToMaterializedMembership() error = %T %[1]v, want AddDeviceMembershipError", err)
	}
	if addErr.Stage != stage {
		t.Fatalf("AddDeviceToMaterializedMembership() stage = %s, want %s", addErr.Stage, stage)
	}
	if !errors.Is(err, wantErr) {
		t.Fatalf("AddDeviceToMaterializedMembership() error = %v, want wrapped %v", err, wantErr)
	}
}

// fakeAddDeviceMapRepo records membership reads and incremental membership writes.
type fakeAddDeviceMapRepo struct {
	order         *[]string
	membership    domain.CanvasMapMembership
	membershipErr error
	addErr        error
	addCalls      int
	addedDevice   domain.CanvasMapDeviceMembership
	addedLinkIDs  []uuid.UUID
	addedAreas    []domain.CanvasMapAreaMembership
}

// GetMembership returns configured map membership or injected load error.
func (r *fakeAddDeviceMapRepo) GetMembership(uuid.UUID) (domain.CanvasMapMembership, error) {
	*r.order = append(*r.order, "membership")
	if r.membershipErr != nil {
		return domain.CanvasMapMembership{}, r.membershipErr
	}
	return r.membership, nil
}

// AddDeviceMembership records the incremental mutation inputs or returns injected persistence error.
func (r *fakeAddDeviceMapRepo) AddDeviceMembership(
	_ uuid.UUID,
	device domain.CanvasMapDeviceMembership,
	linkIDs []uuid.UUID,
	areas []domain.CanvasMapAreaMembership,
) error {
	*r.order = append(*r.order, "add")
	r.addCalls++
	if r.addErr != nil {
		return r.addErr
	}
	r.addedDevice = device
	r.addedLinkIDs = append([]uuid.UUID(nil), linkIDs...)
	r.addedAreas = append([]domain.CanvasMapAreaMembership(nil), areas...)
	return nil
}

// fakeAddDeviceService records device reads used by the add-device workflow.
type fakeAddDeviceService struct {
	order         *[]string
	device        *domain.Device
	memberDevices []domain.Device
	getErr        error
	byIDsErr      error
}

// GetDevice returns the requested device or injected load error.
func (s *fakeAddDeviceService) GetDevice(context.Context, uuid.UUID) (*domain.Device, error) {
	*s.order = append(*s.order, "device")
	if s.getErr != nil {
		return nil, s.getErr
	}
	return s.device, nil
}

// GetDevicesByIDs returns current member devices for duplicate-address checks.
func (s *fakeAddDeviceService) GetDevicesByIDs(context.Context, []uuid.UUID) ([]domain.Device, error) {
	*s.order = append(*s.order, "member_devices")
	if s.byIDsErr != nil {
		return nil, s.byIDsErr
	}
	return append([]domain.Device(nil), s.memberDevices...), nil
}

// fakeAddDeviceLinkRepo records connected-link reads for add-device requests.
type fakeAddDeviceLinkRepo struct {
	order *[]string
	links []domain.Link
	err   error
}

// GetByDeviceID returns connected links or injected link load error.
func (r *fakeAddDeviceLinkRepo) GetByDeviceID(uuid.UUID) ([]domain.Link, error) {
	*r.order = append(*r.order, "links")
	if r.err != nil {
		return nil, r.err
	}
	return append([]domain.Link(nil), r.links...), nil
}

// fakeAddDeviceAreaRepo records area snapshot reads for new device members.
type fakeAddDeviceAreaRepo struct {
	order *[]string
	areas map[uuid.UUID]domain.Area
	err   error
}

// GetByID returns an area snapshot or injected area load error.
func (r *fakeAddDeviceAreaRepo) GetByID(id uuid.UUID) (*domain.Area, error) {
	*r.order = append(*r.order, "areas")
	if r.err != nil {
		return nil, r.err
	}
	area := r.areas[id]
	return &area, nil
}
