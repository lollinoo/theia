package canvasmap

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/domain"
)

type readonlyTopologyMapRepo struct{ *fakeTopologyMapRepo }

func (*readonlyTopologyMapRepo) List() ([]domain.CanvasMap, error) {
	panic("topology GET scanned maps for isolation")
}
func (*readonlyTopologyMapRepo) ReplaceMembership(uuid.UUID, domain.CanvasMapMembership) error {
	panic("topology GET mutated membership")
}

func TestTopologyLoadDoesNotIsolateVirtualDevices(t *testing.T) {
	id, deviceID := uuid.New(), uuid.New()
	maps := &readonlyTopologyMapRepo{&fakeTopologyMapRepo{byID: map[uuid.UUID]domain.CanvasMap{id: {ID: id, MembershipMaterialized: true}}, memberships: map[uuid.UUID]domain.CanvasMapMembership{id: {Devices: []domain.CanvasMapDeviceMembership{{DeviceID: deviceID, Role: domain.CanvasMapDeviceRoleBase}}}}}}
	_, err := LoadTopology(context.Background(), id, TopologyLoadDeps{Maps: maps, Positions: &fakeTopologyPositionRepo{}, Devices: &fakeTopologyDeviceService{devices: []domain.Device{{ID: deviceID, DeviceType: domain.DeviceTypeVirtual}}}, Links: &fakeTopologyLinkRepo{}})
	if err != nil {
		t.Fatal(err)
	}
}
