package sqlite

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/lollinoo/theia/internal/domain"
	"github.com/lollinoo/theia/internal/topology"
)

func TestTopologyObservationRepo_UpsertAndListObservations(t *testing.T) {
	db := openTestDB(t)
	if err := RunMigrations(db); err != nil {
		t.Fatalf("RunMigrations failed: %v", err)
	}

	deviceRepo := NewDeviceRepo(db, nil, nil)
	local := &domain.Device{ID: uuid.New(), Hostname: "local", IP: "192.0.2.10", SysName: "local", Managed: true}
	remote := &domain.Device{ID: uuid.New(), Hostname: "remote", IP: "192.0.2.11", SysName: "remote", Managed: true}
	if err := deviceRepo.Create(local); err != nil {
		t.Fatalf("Create local failed: %v", err)
	}
	if err := deviceRepo.Create(remote); err != nil {
		t.Fatalf("Create remote failed: %v", err)
	}

	repo := NewTopologyObservationRepo(db)
	firstSeen := time.Date(2026, 4, 17, 10, 0, 0, 0, time.UTC)
	if err := repo.UpsertObservation(&topology.Observation{
		LocalDeviceID:   local.ID,
		RemoteIdentity:  topology.NormalizeRemoteIdentity(remote.SysName),
		RemoteDeviceID:  remote.ID,
		LocalPort:       "ether1",
		RemotePort:      "ether2",
		Protocol:        domain.DiscoveryProtocolLLDP,
		LastObservedAt:  firstSeen,
		FirstObservedAt: firstSeen,
	}); err != nil {
		t.Fatalf("UpsertObservation failed: %v", err)
	}

	if err := repo.UpsertObservation(&topology.Observation{
		LocalDeviceID:  local.ID,
		RemoteIdentity: topology.NormalizeRemoteIdentity(remote.SysName),
		RemoteDeviceID: remote.ID,
		LocalPort:      "ether1",
		RemotePort:     "ether2",
		Protocol:       domain.DiscoveryProtocolLLDP,
		LastObservedAt: firstSeen.Add(5 * time.Minute),
	}); err != nil {
		t.Fatalf("second UpsertObservation failed: %v", err)
	}

	observations, err := repo.ListObservationsForDevices([]uuid.UUID{local.ID, remote.ID})
	if err != nil {
		t.Fatalf("ListObservationsForDevices failed: %v", err)
	}
	if len(observations) != 1 {
		t.Fatalf("expected 1 observation row, got %d", len(observations))
	}
	if !observations[0].FirstObservedAt.Equal(firstSeen) {
		t.Fatalf("FirstObservedAt = %s, want %s", observations[0].FirstObservedAt, firstSeen)
	}
	if observations[0].RemoteDeviceID != remote.ID {
		t.Fatalf("RemoteDeviceID = %s, want %s", observations[0].RemoteDeviceID, remote.ID)
	}
}

func TestTopologyObservationRepo_UnresolvedNeighborLifecycle(t *testing.T) {
	db := openTestDB(t)
	if err := RunMigrations(db); err != nil {
		t.Fatalf("RunMigrations failed: %v", err)
	}

	deviceRepo := NewDeviceRepo(db, nil, nil)
	local := &domain.Device{ID: uuid.New(), Hostname: "local", IP: "192.0.2.12", SysName: "local", Managed: true}
	if err := deviceRepo.Create(local); err != nil {
		t.Fatalf("Create local failed: %v", err)
	}

	repo := NewTopologyObservationRepo(db)
	now := time.Date(2026, 4, 17, 11, 0, 0, 0, time.UTC)
	if err := repo.UpsertUnresolvedNeighbor(&topology.UnresolvedNeighbor{
		LocalDeviceID:   local.ID,
		RemoteIdentity:  "missing-edge",
		Protocol:        domain.DiscoveryProtocolLLDP,
		Occurrences:     1,
		FirstObservedAt: now,
		LastObservedAt:  now,
	}); err != nil {
		t.Fatalf("UpsertUnresolvedNeighbor failed: %v", err)
	}
	if err := repo.UpsertUnresolvedNeighbor(&topology.UnresolvedNeighbor{
		LocalDeviceID:  local.ID,
		RemoteIdentity: "missing-edge",
		Protocol:       domain.DiscoveryProtocolLLDP,
		Occurrences:    1,
		LastObservedAt: now.Add(5 * time.Minute),
	}); err != nil {
		t.Fatalf("second UpsertUnresolvedNeighbor failed: %v", err)
	}

	unresolved, err := repo.GetUnresolvedNeighborsByDeviceID(local.ID)
	if err != nil {
		t.Fatalf("GetUnresolvedNeighborsByDeviceID failed: %v", err)
	}
	if len(unresolved) != 1 {
		t.Fatalf("expected 1 unresolved neighbor, got %d", len(unresolved))
	}
	if unresolved[0].Occurrences != 2 {
		t.Fatalf("Occurrences = %d, want 2", unresolved[0].Occurrences)
	}

	if err := repo.ResolveUnresolvedNeighbor(local.ID, "missing-edge", domain.DiscoveryProtocolLLDP, now.Add(10*time.Minute)); err != nil {
		t.Fatalf("ResolveUnresolvedNeighbor failed: %v", err)
	}
	unresolved, err = repo.GetUnresolvedNeighborsByDeviceID(local.ID)
	if err != nil {
		t.Fatalf("GetUnresolvedNeighborsByDeviceID after resolve failed: %v", err)
	}
	if len(unresolved) != 0 {
		t.Fatalf("expected unresolved neighbor to disappear after resolve, got %d record(s)", len(unresolved))
	}
}
