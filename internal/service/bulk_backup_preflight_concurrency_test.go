package service

import (
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/lollinoo/theia/internal/domain"
)

func TestBulkBackupPreflightsBoundConcurrencyAndPreserveResults(t *testing.T) {
	backups := make([]queuedDeviceBackup, 10)
	for i := range backups {
		backups[i].device = domain.Device{IP: "unused-primary", Addresses: []domain.DeviceAddress{
			{Address: fmt.Sprintf("backup-%d", i), Role: domain.DeviceAddressRoleBackup},
		}}
		backups[i].profile = &domain.CredentialProfile{Port: 2222}
	}
	started := make(chan string, len(backups))
	release := make(chan struct{})
	released := false
	defer func() {
		if !released {
			close(release)
		}
	}()
	var active, peak atomic.Int32
	unreachable := errors.New("unreachable")
	finished := make(chan []error, 1)
	go func() {
		finished <- checkBulkBackupReachability(backups, func(target string, port int, timeout time.Duration) error {
			if port != 2222 || timeout != 5*time.Second {
				t.Errorf("probe parameters = %d, %s", port, timeout)
			}
			current := active.Add(1)
			for previous := peak.Load(); current > previous; previous = peak.Load() {
				if peak.CompareAndSwap(previous, current) {
					break
				}
			}
			started <- target
			<-release
			active.Add(-1)
			if target == "backup-2" || target == "backup-8" {
				return unreachable
			}
			return nil
		})
	}()

	seen := make(map[string]int)
	for range defaultBulkBackupWorkerCount {
		select {
		case target := <-started:
			seen[target]++
		case <-time.After(5 * time.Second):
			t.Fatal("independent preflights did not start concurrently")
		}
	}
	select {
	case target := <-started:
		t.Fatalf("probe %s exceeded the concurrency bound", target)
	default:
	}
	close(release)
	released = true
	var results []error
	select {
	case results = <-finished:
	case <-time.After(5 * time.Second):
		t.Fatal("preflights did not finish")
	}
	for len(seen) < len(backups) {
		select {
		case target := <-started:
			seen[target]++
		default:
			t.Fatalf("only %d unique targets checked", len(seen))
		}
	}
	if peak.Load() != int32(defaultBulkBackupWorkerCount) || active.Load() != 0 {
		t.Fatalf("peak=%d active=%d", peak.Load(), active.Load())
	}
	for i, result := range results {
		if seen[fmt.Sprintf("backup-%d", i)] != 1 {
			t.Fatalf("target %d was checked %d times", i, seen[fmt.Sprintf("backup-%d", i)])
		}
		if (i == 2 || i == 8) != errors.Is(result, unreachable) {
			t.Fatalf("result %d = %v", i, result)
		}
	}
}

func TestBulkBackupPreflightsEmptyBatch(t *testing.T) {
	results := checkBulkBackupReachability(nil, func(string, int, time.Duration) error {
		t.Fatal("empty batch started a probe")
		return nil
	})
	if len(results) != 0 {
		t.Fatalf("unexpected results: %v", results)
	}
}
