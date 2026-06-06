package scheduler

// This file exercises map membership source behavior so refactors preserve the documented contract.

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/lollinoo/theia/internal/domain"
)

func TestSavedMapDeviceSourceGetDevicesFiltersOrphanDevices(t *testing.T) {
	memberID := uuid.MustParse("30000000-0000-0000-0000-000000000001")
	otherMemberID := uuid.MustParse("30000000-0000-0000-0000-000000000002")
	orphanID := uuid.MustParse("30000000-0000-0000-0000-000000000003")
	mapID := uuid.MustParse("40000000-0000-0000-0000-000000000001")

	source := NewSavedMapDeviceSource(
		&fakeDeviceSource{
			devices: []domain.Device{
				{ID: memberID, Hostname: "member"},
				{ID: otherMemberID, Hostname: "other-member"},
				{ID: orphanID, Hostname: "orphan"},
			},
		},
		fakeCanvasMapMembershipSource{
			maps: []domain.CanvasMap{{ID: mapID}},
			memberships: map[uuid.UUID]domain.CanvasMapMembership{
				mapID: {
					Devices: []domain.CanvasMapDeviceMembership{
						{DeviceID: memberID, Role: domain.CanvasMapDeviceRoleBase},
						{DeviceID: otherMemberID, Role: domain.CanvasMapDeviceRoleGhost},
					},
				},
			},
		},
	)

	devices, err := source.GetDevices()
	if err != nil {
		t.Fatalf("GetDevices() error = %v", err)
	}

	if len(devices) != 2 {
		t.Fatalf("len(devices) = %d, want 2; devices = %#v", len(devices), devices)
	}
	if devices[0].ID != memberID || devices[1].ID != otherMemberID {
		t.Fatalf("devices = %#v, want only saved-map members in source order", devices)
	}
}

func TestSavedMapDeviceSourceGetDevicesRefreshesMemberships(t *testing.T) {
	deviceID := uuid.MustParse("30000000-0000-0000-0000-000000000011")
	mapID := uuid.MustParse("40000000-0000-0000-0000-000000000011")
	maps := &mutableCanvasMapMembershipSource{
		maps: []domain.CanvasMap{{ID: mapID}},
	}
	source := NewSavedMapDeviceSource(
		&fakeDeviceSource{devices: []domain.Device{{ID: deviceID, Hostname: "router"}}},
		maps,
	)

	devices, err := source.GetDevices()
	if err != nil {
		t.Fatalf("first GetDevices() error = %v", err)
	}
	if len(devices) != 0 {
		t.Fatalf("first len(devices) = %d, want 0", len(devices))
	}

	maps.membership = domain.CanvasMapMembership{
		Devices: []domain.CanvasMapDeviceMembership{
			{DeviceID: deviceID, Role: domain.CanvasMapDeviceRoleBase},
		},
	}

	devices, err = source.GetDevices()
	if err != nil {
		t.Fatalf("second GetDevices() error = %v", err)
	}
	if len(devices) != 1 || devices[0].ID != deviceID {
		t.Fatalf("second devices = %#v, want device %s", devices, deviceID)
	}
}

func TestSavedMapDeviceSourceGetDevicesPropagatesMembershipErrors(t *testing.T) {
	wantErr := errors.New("membership unavailable")
	source := NewSavedMapDeviceSource(
		&fakeDeviceSource{devices: []domain.Device{{ID: uuid.New(), Hostname: "router"}}},
		fakeCanvasMapMembershipSource{listErr: wantErr},
	)

	if _, err := source.GetDevices(); !errors.Is(err, wantErr) {
		t.Fatalf("GetDevices() error = %v, want %v", err, wantErr)
	}
}

func TestSchedulerRefreshRemovesTasksWhenDeviceLosesLastSavedMapMembership(t *testing.T) {
	deviceID := uuid.MustParse("30000000-0000-0000-0000-000000000021")
	mapID := uuid.MustParse("40000000-0000-0000-0000-000000000021")
	maps := &mutableCanvasMapMembershipSource{
		maps: []domain.CanvasMap{{ID: mapID}},
		membership: domain.CanvasMapMembership{
			Devices: []domain.CanvasMapDeviceMembership{
				{DeviceID: deviceID, Role: domain.CanvasMapDeviceRoleBase},
			},
		},
	}
	source := NewSavedMapDeviceSource(
		&fakeDeviceSource{
			devices: []domain.Device{
				{ID: deviceID, Hostname: "router", Managed: true, PollClass: domain.PollClassStandard},
			},
		},
		maps,
	)
	scheduler := NewScheduler(source, nil)
	now := time.Unix(1_700_000_000, 0)

	if err := scheduler.refreshDevices(now); err != nil {
		t.Fatalf("initial refreshDevices() error = %v", err)
	}
	if got := len(scheduler.items); got != 4 {
		t.Fatalf("initial len(items) = %d, want 4", got)
	}

	maps.membership = domain.CanvasMapMembership{}
	if err := scheduler.refreshDevices(now.Add(time.Minute)); err != nil {
		t.Fatalf("second refreshDevices() error = %v", err)
	}
	if got := len(scheduler.items); got != 0 {
		t.Fatalf("len(items) after orphaning device = %d, want 0", got)
	}
}

func TestSchedulerDirectSchedulingIgnoresSavedMapOrphanDevice(t *testing.T) {
	deviceID := uuid.MustParse("30000000-0000-0000-0000-000000000031")
	mapID := uuid.MustParse("40000000-0000-0000-0000-000000000031")
	device := domain.Device{
		ID:                     deviceID,
		Hostname:               "orphan-router",
		IP:                     "192.0.2.31",
		Managed:                true,
		PollClass:              domain.PollClassCore,
		TopologyBootstrapState: domain.TopologyBootstrapStatePending,
	}
	source := NewSavedMapDeviceSource(
		&fakeDeviceSource{devices: []domain.Device{device}},
		&mutableCanvasMapMembershipSource{maps: []domain.CanvasMap{{ID: mapID}}},
	)
	scheduler := NewScheduler(source, nil)
	scheduler.running.Store(true)
	now := time.Unix(1_700_000_000, 0)

	scheduler.ReduePerformanceTask(device, now)
	select {
	case request := <-scheduler.redueRequests:
		t.Fatalf("unexpected redue request for saved-map orphan: %+v", request.device)
	default:
	}

	if scheduler.ScheduleBootstrap(device, now) {
		t.Fatal("ScheduleBootstrap returned true for saved-map orphan")
	}

	scheduler.ReconcileDeviceTasks(device, now)
	if got := len(scheduler.items); got != 0 {
		t.Fatalf("len(items) after orphan reconcile = %d, want 0", got)
	}
}

func TestSchedulerHandleReduePerformanceTaskIgnoresSavedMapOrphanDevice(t *testing.T) {
	deviceID := uuid.MustParse("30000000-0000-0000-0000-000000000041")
	mapID := uuid.MustParse("40000000-0000-0000-0000-000000000041")
	device := domain.Device{
		ID:        deviceID,
		Hostname:  "queued-orphan",
		IP:        "192.0.2.41",
		Managed:   true,
		PollClass: domain.PollClassCore,
	}
	source := NewSavedMapDeviceSource(
		&fakeDeviceSource{devices: []domain.Device{device}},
		&mutableCanvasMapMembershipSource{maps: []domain.CanvasMap{{ID: mapID}}},
	)
	scheduler := NewScheduler(source, nil)

	scheduler.handleReduePerformanceTask(reduePerformanceTaskRequest{
		device:    device,
		changedAt: time.Unix(1_700_000_000, 0),
	})

	if got := len(scheduler.items); got != 0 {
		t.Fatalf("len(items) after queued orphan redue = %d, want 0", got)
	}
}

type fakeCanvasMapMembershipSource struct {
	maps        []domain.CanvasMap
	memberships map[uuid.UUID]domain.CanvasMapMembership
	listErr     error
	memberErr   error
}

func (s fakeCanvasMapMembershipSource) List() ([]domain.CanvasMap, error) {
	if s.listErr != nil {
		return nil, s.listErr
	}
	return append([]domain.CanvasMap(nil), s.maps...), nil
}

func (s fakeCanvasMapMembershipSource) GetMembership(id uuid.UUID) (domain.CanvasMapMembership, error) {
	if s.memberErr != nil {
		return domain.CanvasMapMembership{}, s.memberErr
	}
	return s.memberships[id], nil
}

type mutableCanvasMapMembershipSource struct {
	maps       []domain.CanvasMap
	membership domain.CanvasMapMembership
}

func (s *mutableCanvasMapMembershipSource) List() ([]domain.CanvasMap, error) {
	return append([]domain.CanvasMap(nil), s.maps...), nil
}

func (s *mutableCanvasMapMembershipSource) GetMembership(uuid.UUID) (domain.CanvasMapMembership, error) {
	return s.membership, nil
}
