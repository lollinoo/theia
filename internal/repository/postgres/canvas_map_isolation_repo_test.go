package postgres

import (
	"context"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/domain"
)

func TestCanvasIsolationRollsBackAndPreservesCloneMetadata(t *testing.T) {
	db := setupTestDB(t)
	devices := NewDeviceRepo(db, testKeyring, nil)
	v := &domain.Device{ID: uuid.New(), Hostname: "virtual", IP: "192.0.2.8", DeviceType: domain.DeviceTypeVirtual, MetricsSource: domain.MetricsSourceNone, ProbePorts: []int{22, 443}}
	if err := devices.Create(v); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("UPDATE device_addresses SET probe_ports='161,162' WHERE device_id=$1", v.ID); err != nil {
		t.Fatal(err)
	}
	peer := &domain.Device{ID: uuid.New(), Hostname: "peer", DeviceType: domain.DeviceTypeSwitch}
	if err := devices.Create(peer); err != nil {
		t.Fatal(err)
	}
	maps := NewCanvasMapRepo(db)
	first, err := maps.Create(domain.CanvasMapCreate{Name: "First"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := maps.Create(domain.CanvasMapCreate{Name: "Second"})
	if err != nil {
		t.Fatal(err)
	}
	link := &domain.Link{ID: uuid.New(), SourceDeviceID: v.ID, TargetDeviceID: peer.ID, DiscoveryProtocol: domain.DiscoveryProtocolManual}
	if err := NewLinkRepo(db, nil).Create(link); err != nil {
		t.Fatal(err)
	}
	membership := domain.CanvasMapMembership{Devices: []domain.CanvasMapDeviceMembership{{DeviceID: v.ID, Role: domain.CanvasMapDeviceRoleBase}, {DeviceID: peer.ID, Role: domain.CanvasMapDeviceRoleBase}}, LinkIDs: []uuid.UUID{link.ID}}
	for _, id := range []uuid.UUID{first.ID, second.ID} {
		if err := maps.ReplaceMembership(id, membership); err != nil {
			t.Fatal(err)
		}
	}
	if err := NewCanvasMapPositionRepo(db).SaveAllForMap(first.ID, []domain.DevicePosition{{DeviceID: v.ID, X: 8, Y: 9, Pinned: true}}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE FUNCTION audit_reject_position() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'injected failure'; END $$`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TRIGGER audit_position BEFORE UPDATE ON canvas_map_positions FOR EACH ROW EXECUTE FUNCTION audit_reject_position()`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		db.Exec("DROP TRIGGER IF EXISTS audit_position ON canvas_map_positions")
		db.Exec("DROP FUNCTION IF EXISTS audit_reject_position()")
	})
	if err := maps.IsolateVirtualDevices(context.Background(), first.ID); err == nil {
		t.Fatal("injected failure ignored")
	}
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM devices").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("orphaned clones: %d devices", count)
	}
	if _, err := db.Exec("DROP TRIGGER audit_position ON canvas_map_positions"); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	errs := make(chan error, 3)
	for range 3 {
		wg.Add(1)
		go func() { defer wg.Done(); errs <- maps.IsolateVirtualDevices(context.Background(), first.ID) }()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if err := db.QueryRow("SELECT COUNT(*) FROM devices").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 3 {
		t.Fatalf("concurrent isolation created %d devices", count)
	}
	got, err := maps.GetMembership(first.ID)
	if err != nil {
		t.Fatal(err)
	}
	var cloneID uuid.UUID
	for _, member := range got.Devices {
		if member.DeviceID != peer.ID {
			cloneID = member.DeviceID
		}
	}
	clone, err := devices.GetByID(cloneID)
	if err != nil {
		t.Fatal(err)
	}
	if clone.ID == v.ID || len(clone.ProbePorts) != 2 || len(clone.Addresses) != 1 || clone.Addresses[0].Address != v.IP || len(clone.Addresses[0].ProbePorts) != 2 {
		t.Fatalf("clone lost metadata: %+v", clone)
	}
	positions, err := NewCanvasMapPositionRepo(db).GetAllForMap(first.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(positions) != 1 || positions[0].DeviceID != cloneID || positions[0].X != 8 {
		t.Fatalf("positions=%+v", positions)
	}
	clonedLink, err := NewLinkRepo(db, nil).GetByID(got.LinkIDs[0])
	if err != nil {
		t.Fatal(err)
	}
	if clonedLink.SourceDeviceID != cloneID {
		t.Fatal("link was not remapped")
	}
	other, err := maps.GetMembership(second.ID)
	if err != nil {
		t.Fatal(err)
	}
	if other.LinkIDs[0] != link.ID {
		t.Fatal("isolation changed the other map")
	}
}
