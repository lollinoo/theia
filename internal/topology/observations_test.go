package topology_test

import (
	"database/sql"
	"testing"
	"time"

	"github.com/google/uuid"
	_ "github.com/mattn/go-sqlite3"

	"github.com/lollinoo/theia/internal/domain"
	sqliterepo "github.com/lollinoo/theia/internal/repository/sqlite"
	"github.com/lollinoo/theia/internal/topology"
)

func newMaterializerRepos(t *testing.T) (*sqliterepo.DeviceRepo, *sqliterepo.LinkRepo) {
	t.Helper()
	db, err := sql.Open("sqlite3", ":memory:?_foreign_keys=on")
	if err != nil {
		t.Fatalf("opening sqlite db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if err := sqliterepo.RunMigrations(db); err != nil {
		t.Fatalf("RunMigrations failed: %v", err)
	}
	return sqliterepo.NewDeviceRepo(db, nil, nil), sqliterepo.NewLinkRepo(db, nil)
}

func seedMaterializerDevices(t *testing.T, repo *sqliterepo.DeviceRepo, ids ...uuid.UUID) {
	t.Helper()
	for _, id := range ids {
		device := &domain.Device{
			ID:       id,
			Hostname: id.String(),
			IP:       "192.0.2." + id.String()[len(id.String())-2:],
			SysName:  id.String(),
			Managed:  true,
			Status:   domain.DeviceStatusUp,
		}
		if err := repo.Create(device); err != nil {
			t.Fatalf("Create device %s failed: %v", id, err)
		}
	}
}

func TestApplyObservations_SameDirectionEnrich(t *testing.T) {
	deviceRepo, repo := newMaterializerRepos(t)
	localID := uuid.MustParse("61000000-0000-0000-0000-000000000001")
	remoteID := uuid.MustParse("61000000-0000-0000-0000-000000000002")
	now := time.Date(2026, 4, 17, 12, 0, 0, 0, time.UTC)
	seedMaterializerDevices(t, deviceRepo, localID, remoteID)

	result, err := topology.ApplyObservations([]topology.Observation{
		{LocalDeviceID: localID, RemoteDeviceID: remoteID, RemoteIdentity: "remote", LocalPort: "", RemotePort: "ether2", Protocol: domain.DiscoveryProtocolLLDP, LastObservedAt: now},
		{LocalDeviceID: localID, RemoteDeviceID: remoteID, RemoteIdentity: "remote", LocalPort: "ether1", RemotePort: "ether2", Protocol: domain.DiscoveryProtocolLLDP, LastObservedAt: now.Add(time.Minute)},
	}, repo)
	if err != nil {
		t.Fatalf("ApplyObservations failed: %v", err)
	}
	if !result.TopologyChanged {
		t.Fatal("expected topology to change")
	}
	if result.LinksCreated != 1 {
		t.Fatalf("LinksCreated = %d, want 1", result.LinksCreated)
	}

	links, err := repo.GetAll()
	if err != nil {
		t.Fatalf("GetAll failed: %v", err)
	}
	if len(links) != 1 {
		t.Fatalf("expected 1 link, got %d", len(links))
	}
	if links[0].SourceIfName != "ether1" || links[0].TargetIfName != "ether2" {
		t.Fatalf("unexpected link endpoints: %+v", links[0])
	}
}

func TestApplyObservations_ReverseDirectionMerge(t *testing.T) {
	deviceRepo, repo := newMaterializerRepos(t)
	deviceA := uuid.MustParse("62000000-0000-0000-0000-000000000001")
	deviceB := uuid.MustParse("62000000-0000-0000-0000-000000000002")
	seedMaterializerDevices(t, deviceRepo, deviceA, deviceB)

	_, err := topology.ApplyObservations([]topology.Observation{
		{LocalDeviceID: deviceA, RemoteDeviceID: deviceB, RemoteIdentity: "b", LocalPort: "ether1", RemotePort: "ether2", Protocol: domain.DiscoveryProtocolLLDP},
		{LocalDeviceID: deviceB, RemoteDeviceID: deviceA, RemoteIdentity: "a", LocalPort: "ether2", RemotePort: "ether1", Protocol: domain.DiscoveryProtocolLLDP},
	}, repo)
	if err != nil {
		t.Fatalf("ApplyObservations failed: %v", err)
	}

	links, err := repo.GetAll()
	if err != nil {
		t.Fatalf("GetAll failed: %v", err)
	}
	if len(links) != 1 {
		t.Fatalf("expected reverse observations to converge to 1 link, got %d", len(links))
	}
}

func TestApplyObservations_ReorientsReverseIncompleteObservation(t *testing.T) {
	deviceRepo, repo := newMaterializerRepos(t)
	deviceA := uuid.MustParse("63000000-0000-0000-0000-000000000001")
	deviceB := uuid.MustParse("63000000-0000-0000-0000-000000000002")
	seedMaterializerDevices(t, deviceRepo, deviceA, deviceB)

	if _, err := topology.ApplyObservations([]topology.Observation{
		{LocalDeviceID: deviceB, RemoteDeviceID: deviceA, RemoteIdentity: "a", LocalPort: "", RemotePort: "ether1", Protocol: domain.DiscoveryProtocolLLDP},
	}, repo); err != nil {
		t.Fatalf("seeding reverse incomplete observation failed: %v", err)
	}
	result, err := topology.ApplyObservations([]topology.Observation{
		{LocalDeviceID: deviceB, RemoteDeviceID: deviceA, RemoteIdentity: "a", LocalPort: "", RemotePort: "ether1", Protocol: domain.DiscoveryProtocolLLDP},
		{LocalDeviceID: deviceA, RemoteDeviceID: deviceB, RemoteIdentity: "b", LocalPort: "ether1", RemotePort: "", Protocol: domain.DiscoveryProtocolLLDP},
	}, repo)
	if err != nil {
		t.Fatalf("ApplyObservations failed: %v", err)
	}

	reoriented := false
	for _, event := range result.Events {
		if event.Result.Kind == domain.LinkUpsertKindReoriented {
			reoriented = true
		}
	}
	if !reoriented {
		t.Fatal("expected a reverse incomplete observation to be reoriented")
	}
}

func TestApplyObservations_SelfLinkObservation(t *testing.T) {
	deviceRepo, repo := newMaterializerRepos(t)
	deviceID := uuid.MustParse("64000000-0000-0000-0000-000000000001")
	seedMaterializerDevices(t, deviceRepo, deviceID)

	_, err := topology.ApplyObservations([]topology.Observation{
		{
			LocalDeviceID:  deviceID,
			RemoteDeviceID: deviceID,
			RemoteIdentity: "self-device",
			LocalPort:      "ether1",
			RemotePort:     "ether9",
			Protocol:       domain.DiscoveryProtocolLLDP,
			SelfNeighbor:   true,
		},
	}, repo)
	if err != nil {
		t.Fatalf("ApplyObservations failed: %v", err)
	}

	links, err := repo.GetAll()
	if err != nil {
		t.Fatalf("GetAll failed: %v", err)
	}
	if len(links) != 1 {
		t.Fatalf("expected 1 self link, got %d", len(links))
	}
	if links[0].SourceDeviceID != deviceID || links[0].TargetDeviceID != deviceID {
		t.Fatalf("expected self-link devices to match %s, got %+v", deviceID, links[0])
	}
}
