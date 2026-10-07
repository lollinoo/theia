package service

import (
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/domain"
	"github.com/lollinoo/theia/internal/snmp"
	"github.com/lollinoo/theia/internal/topology"
)

type discoveryIdentityRepo struct {
	*mockDeviceRepo
	ids   map[string]uuid.UUID
	names []string
	calls int
	err   error
}

func (r *discoveryIdentityRepo) LookupDeviceIDsBySysNames(names []string) (map[string]uuid.UUID, error) {
	r.calls++
	r.names = names
	return r.ids, r.err
}

type discoveryBatchStore struct {
	recordingTopologyObservationStore
	batch []topology.Observation
	calls int
	err   error
}

func (r *discoveryBatchStore) UpsertDiscoveryObservations(observations []topology.Observation) error {
	r.calls++
	r.batch = observations
	return r.err
}

func TestStaticDiscoveryUsesBatchIdentitiesAndObservationWrites(t *testing.T) {
	local, remote := uuid.New(), uuid.New()
	repo := &discoveryIdentityRepo{mockDeviceRepo: newMockDeviceRepo(), ids: map[string]uuid.UUID{"remote": remote}}
	store := &discoveryBatchStore{}
	svc := NewDeviceService(repo, newMockLinkRepo(), newMockSettingsRepo(), nil, nil, WithTopologyObservationStore(store))
	neighbors := []snmp.NeighborInfo{
		{RemoteSysName: "remote", LocalIfName: "one", RemotePortID: "two", Protocol: domain.DiscoveryProtocolLLDP},
		{RemoteSysName: "remote", LocalIfName: "three", RemotePortID: "four", Protocol: domain.DiscoveryProtocolLLDP},
		{RemoteSysName: "missing", LocalIfName: "five", RemotePortID: "six", Protocol: domain.DiscoveryProtocolLLDP},
	}
	result, _, _, err := svc.applyDiscoveryViaObservationStore(domain.Device{ID: local}, neighbors, []domain.DiscoveryProtocol{domain.DiscoveryProtocolLLDP}, nil, false)
	if err != nil || result.UnresolvedNeighbors != 1 || repo.calls != 1 || len(repo.names) != 2 || store.calls != 1 || len(store.batch) != 3 || store.upsertObservations != 0 || store.upsertUnresolved != 0 || store.resolveUnresolved != 0 {
		t.Fatalf("result=%#v error=%v lookup=%d batch=%d", result, err, repo.calls, store.calls)
	}
	if store.batch[0].RemoteDeviceID != remote || store.batch[2].RemoteDeviceID != uuid.Nil {
		t.Fatal("batch changed resolved/unresolved identities")
	}
	store.err = errors.New("batch failed")
	prunes := store.pruneObservations
	if _, _, _, err := svc.applyDiscoveryViaObservationStore(domain.Device{ID: local}, neighbors, nil, nil, false); !errors.Is(err, store.err) || store.pruneObservations != prunes {
		t.Fatalf("batch failure did not stop pruning: %v", err)
	}
	repo.err = errors.New("lookup failed")
	writes := store.calls
	if _, _, _, err := svc.applyDiscoveryViaObservationStore(domain.Device{ID: local}, neighbors, nil, nil, false); !errors.Is(err, repo.err) || store.calls != writes {
		t.Fatalf("lookup failure wrote observations: %v", err)
	}
}

func TestStoredTopologyReconciliationBatchesResolvedObservations(t *testing.T) {
	repo := newMockDeviceRepo()
	local, remote := uuid.New(), uuid.New()
	for _, device := range []*domain.Device{{ID: local, IP: "192.0.2.95", SysName: "local"}, {ID: remote, IP: "192.0.2.96", SysName: "remote"}} {
		if err := repo.Create(device); err != nil {
			t.Fatal(err)
		}
	}
	store := &discoveryBatchStore{recordingTopologyObservationStore: recordingTopologyObservationStore{observations: []topology.Observation{
		{LocalDeviceID: local, RemoteIdentity: "remote", LocalPort: "one", RemotePort: "two", Protocol: domain.DiscoveryProtocolLLDP},
		{LocalDeviceID: local, RemoteIdentity: "remote", LocalPort: "three", RemotePort: "four", Protocol: domain.DiscoveryProtocolLLDP},
	}}}
	svc := NewDeviceService(repo, newMockLinkRepo(), newMockSettingsRepo(), nil, nil, WithTopologyObservationStore(store))
	if _, err := svc.ReconcileStoredTopology([]uuid.UUID{local, remote}); err != nil {
		t.Fatal(err)
	}
	if store.calls != 1 || len(store.batch) != 2 || store.upsertObservations != 0 || store.resolveUnresolved != 0 {
		t.Fatalf("reconciliation writes batch=%d rows=%d singles=%d/%d", store.calls, len(store.batch), store.upsertObservations, store.resolveUnresolved)
	}
}
