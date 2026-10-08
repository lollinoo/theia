package postgres

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/domain"
	"github.com/lollinoo/theia/internal/topology"
)

func TestTopologyDiscoveryBatchKeepsIdentityAndOccurrenceSemantics(t *testing.T) {
	db := setupTestDB(t)
	devices := NewDeviceRepo(db, nil, nil)
	repo := NewTopologyObservationRepo(db)
	local := &domain.Device{ID: uuid.New(), IP: "192.0.2.81", Hostname: "local"}
	remote := &domain.Device{ID: uuid.New(), IP: "192.0.2.82", Hostname: "remote", SysName: "EDGE.example.test"}
	for _, device := range []*domain.Device{local, remote} {
		if err := devices.Create(device); err != nil {
			t.Fatal(err)
		}
	}
	lookup, err := devices.LookupDeviceIDsBySysNames([]string{" edge.elsewhere.test. ", "EDGE", "missing"})
	if err != nil || lookup["EDGE"] != remote.ID || lookup[" edge.elsewhere.test. "] != remote.ID || lookup["missing"] != uuid.Nil {
		t.Fatalf("identity results=%#v error=%v", lookup, err)
	}
	now := time.Now().UTC().Truncate(time.Microsecond)
	observation := topology.Observation{LocalDeviceID: local.ID, RemoteIdentity: "missing", LocalPort: "one", RemotePort: "two", Protocol: domain.DiscoveryProtocolLLDP, LastObservedAt: now}
	if err := repo.UpsertDiscoveryObservations([]topology.Observation{observation, observation}); err != nil {
		t.Fatal(err)
	}
	stored, err := repo.ListObservationsForDevices([]uuid.UUID{local.ID})
	if err != nil || len(stored) != 1 || !stored[0].FirstObservedAt.Equal(now) {
		t.Fatalf("stored=%#v error=%v", stored, err)
	}
	originalID := stored[0].ID
	unknown, err := repo.GetUnresolvedNeighborsByDeviceID(local.ID)
	if err != nil || len(unknown) != 1 || unknown[0].Occurrences != 2 {
		t.Fatalf("unknown=%#v error=%v", unknown, err)
	}
	observation.RemoteDeviceID = remote.ID
	observation.LastObservedAt = now.Add(time.Minute)
	if err := repo.UpsertDiscoveryObservations([]topology.Observation{observation}); err != nil {
		t.Fatal(err)
	}
	stored, err = repo.ListObservationsForDevices([]uuid.UUID{local.ID})
	if err != nil || stored[0].ID != originalID || stored[0].RemoteDeviceID != remote.ID || !stored[0].FirstObservedAt.Equal(now) {
		t.Fatalf("updated=%#v error=%v", stored, err)
	}
	unknown, err = repo.GetUnresolvedNeighborsByDeviceID(local.ID)
	if err != nil || len(unknown) != 0 {
		t.Fatalf("resolved unknown=%#v error=%v", unknown, err)
	}
	observation.RemoteDeviceID = uuid.Nil
	if err := repo.UpsertDiscoveryObservations([]topology.Observation{observation}); err != nil {
		t.Fatal(err)
	}
	unknown, err = repo.GetUnresolvedNeighborsByDeviceID(local.ID)
	if err != nil || len(unknown) != 1 || unknown[0].Occurrences != 3 {
		t.Fatalf("rediscovered unknown=%#v error=%v", unknown, err)
	}
}

func TestTopologyDiscoveryBatchRollsBackAcrossStatementBoundaries(t *testing.T) {
	db := setupTestDB(t)
	devices := NewDeviceRepo(db, nil, nil)
	repo := NewTopologyObservationRepo(db)
	local := &domain.Device{ID: uuid.New(), IP: "192.0.2.83", Hostname: "local"}
	if err := devices.Create(local); err != nil {
		t.Fatal(err)
	}
	observations := make([]topology.Observation, discoveryBatchSize+1)
	for i := range observations {
		observations[i] = topology.Observation{LocalDeviceID: local.ID, RemoteIdentity: fmt.Sprintf("neighbor-%d", i), LocalPort: "one", RemotePort: "two", Protocol: domain.DiscoveryProtocolLLDP}
	}
	observations[len(observations)-1].LocalDeviceID = uuid.MustParse("ffffffff-ffff-ffff-ffff-ffffffffffff")
	if err := repo.UpsertDiscoveryObservations(observations); err == nil {
		t.Fatal("invalid second batch accepted")
	}
	if count := importTestCount(t, db, "SELECT COUNT(*) FROM topology_observations"); count != 0 {
		t.Fatalf("partial discovery committed: %d", count)
	}
	observations[len(observations)-1].LocalDeviceID = local.ID
	if err := repo.UpsertDiscoveryObservations(observations); err != nil {
		t.Fatal(err)
	}
	if count := importTestCount(t, db, "SELECT COUNT(*) FROM topology_observations"); count != len(observations) {
		t.Fatalf("multi-statement discovery count=%d", count)
	}
}

func TestTopologyDiscoveryBatchCountsConcurrentSightings(t *testing.T) {
	db := setupTestDB(t)
	devices := NewDeviceRepo(db, nil, nil)
	repo := NewTopologyObservationRepo(db)
	local := &domain.Device{ID: uuid.New(), IP: "192.0.2.84", Hostname: "local"}
	if err := devices.Create(local); err != nil {
		t.Fatal(err)
	}
	const count = 16
	var group sync.WaitGroup
	failures := make(chan error, count)
	for i := 0; i < count; i++ {
		group.Add(1)
		go func() {
			defer group.Done()
			failures <- repo.UpsertDiscoveryObservations([]topology.Observation{{LocalDeviceID: local.ID, RemoteIdentity: "missing", LocalPort: "one", RemotePort: "two", Protocol: domain.DiscoveryProtocolLLDP}})
		}()
	}
	group.Wait()
	close(failures)
	for err := range failures {
		if err != nil {
			t.Fatal(err)
		}
	}
	unknown, err := repo.GetUnresolvedNeighborsByDeviceID(local.ID)
	if err != nil || len(unknown) != 1 || unknown[0].Occurrences != count {
		t.Fatalf("concurrent occurrences=%#v error=%v", unknown, err)
	}
}

func TestTopologyDiscoveryBatchesLockMixedNeighborChangesInOrder(t *testing.T) {
	db := setupTestDB(t)
	devices := NewDeviceRepo(db, nil, nil)
	repo := NewTopologyObservationRepo(db)
	local, remote := uuid.New(), uuid.New()
	for _, device := range []*domain.Device{{ID: local, IP: "192.0.2.97"}, {ID: remote, IP: "192.0.2.98"}} {
		if err := devices.Create(device); err != nil {
			t.Fatal(err)
		}
	}
	seed := []topology.Observation{{LocalDeviceID: local, RemoteIdentity: "a", LocalPort: "seed", Protocol: domain.DiscoveryProtocolLLDP}, {LocalDeviceID: local, RemoteIdentity: "b", LocalPort: "seed", Protocol: domain.DiscoveryProtocolLLDP}}
	if err := repo.UpsertDiscoveryObservations(seed); err != nil {
		t.Fatal(err)
	}
	var group sync.WaitGroup
	failures := make(chan error, 16)
	for i := 0; i < 16; i++ {
		group.Add(1)
		go func(i int) {
			defer group.Done()
			batch := []topology.Observation{{LocalDeviceID: local, RemoteIdentity: "a", LocalPort: fmt.Sprintf("port-%d", i), Protocol: domain.DiscoveryProtocolLLDP}, {LocalDeviceID: local, RemoteIdentity: "b", LocalPort: fmt.Sprintf("port-%d", i), Protocol: domain.DiscoveryProtocolLLDP}}
			batch[i%2].RemoteDeviceID = remote
			if i%2 == 0 {
				batch[0], batch[1] = batch[1], batch[0]
			}
			failures <- repo.UpsertDiscoveryObservations(batch)
		}(i)
	}
	group.Wait()
	close(failures)
	for err := range failures {
		if err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{"a", "b"} {
		var count int
		if err := db.QueryRow("SELECT occurrences FROM unresolved_neighbors WHERE local_device_id=$1 AND remote_identity=$2", local.String(), name).Scan(&count); err != nil || count != 9 {
			t.Fatalf("%s occurrences=%d error=%v", name, count, err)
		}
	}
}
