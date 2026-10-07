package postgres

import (
	"github.com/lollinoo/theia/internal/domain"
	"sync"
	"testing"
)

func TestConcurrentReverseDiscoveriesCreateOnePhysicalLink(t *testing.T) {
	db := setupTestDB(t)
	a, b := createTestDevicePair(t, NewDeviceRepo(db, testKeyring, nil))
	repo := NewLinkRepo(db, nil)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := range 12 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			link := &domain.Link{SourceDeviceID: a, SourceIfName: "eth0", TargetDeviceID: b, TargetIfName: "eth1", DiscoveryProtocol: domain.DiscoveryProtocolLLDP}
			if i%2 != 0 {
				link.SourceDeviceID, link.TargetDeviceID = b, a
				link.SourceIfName, link.TargetIfName = "eth1", "eth0"
			}
			if _, err := repo.Upsert(link); err != nil {
				t.Error(err)
			}
		}()
	}
	close(start)
	wg.Wait()
	links, err := repo.GetByDeviceID(a)
	if err != nil {
		t.Fatal(err)
	}
	if len(links) != 1 {
		t.Fatalf("physical links=%d, want 1", len(links))
	}
	if _, err := repo.Upsert(&domain.Link{SourceDeviceID: a, SourceIfName: "eth2", TargetDeviceID: b, TargetIfName: "eth3", DiscoveryProtocol: domain.DiscoveryProtocolLLDP}); err != nil {
		t.Fatal(err)
	}
	links, err = repo.GetByDeviceID(a)
	if err != nil {
		t.Fatal(err)
	}
	if len(links) != 2 {
		t.Fatalf("parallel links=%d, want 2", len(links))
	}
}
