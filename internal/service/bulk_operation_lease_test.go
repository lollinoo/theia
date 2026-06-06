package service

import (
	"fmt"
	"testing"

	"github.com/lollinoo/theia/internal/domain"
)

type recordingBulkOperationLease struct {
	name     string
	releases *[]string
}

func (l recordingBulkOperationLease) Release() error {
	*l.releases = append(*l.releases, l.name)
	return nil
}

func TestBulkOperationGateRejectsConcurrentAcquisitionUntilReleased(t *testing.T) {
	var gate bulkOperationGate

	lease, acquired := gate.TryAcquire()
	if !acquired {
		t.Fatal("first TryAcquire acquired = false, want true")
	}
	if _, acquired := gate.TryAcquire(); acquired {
		t.Fatal("second TryAcquire acquired = true, want false while lease is active")
	}
	if err := lease.Release(); err != nil {
		t.Fatalf("Release returned error: %v", err)
	}
	if _, acquired := gate.TryAcquire(); !acquired {
		t.Fatal("third TryAcquire acquired = false, want true after release")
	}
}

func TestCompositeBulkOperationLeaseReleasesReverseOrderOnce(t *testing.T) {
	releases := []string{}
	lease := &compositeBulkOperationLease{
		leases: []domain.BulkOperationLease{
			recordingBulkOperationLease{name: "local", releases: &releases},
			recordingBulkOperationLease{name: "distributed", releases: &releases},
		},
	}

	if err := lease.Release(); err != nil {
		t.Fatalf("Release returned error: %v", err)
	}
	if err := lease.Release(); err != nil {
		t.Fatalf("second Release returned error: %v", err)
	}

	if got := fmt.Sprint(releases); got != "[distributed local]" {
		t.Fatalf("release order = %s, want [distributed local]", got)
	}
}
