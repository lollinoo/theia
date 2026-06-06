package service

// This file defines bulk operation lease service behavior and domain orchestration rules.

import (
	"context"
	"fmt"
	"log"
	"sync"

	"github.com/lollinoo/theia/internal/domain"
	"github.com/lollinoo/theia/internal/observability"
)

type bulkOperationGate struct {
	mu     sync.Mutex
	active bool
}

func (g *bulkOperationGate) TryAcquire() (domain.BulkOperationLease, bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.active {
		return nil, false
	}
	g.active = true
	return &releaseFuncBulkOperationLease{release: func() error {
		g.mu.Lock()
		defer g.mu.Unlock()
		g.active = false
		return nil
	}}, true
}

type releaseFuncBulkOperationLease struct {
	once    sync.Once
	release func() error
}

func (l *releaseFuncBulkOperationLease) Release() error {
	var err error
	l.once.Do(func() {
		if l.release != nil {
			err = l.release()
		}
	})
	return err
}

type compositeBulkOperationLease struct {
	once   sync.Once
	leases []domain.BulkOperationLease
}

func (l *compositeBulkOperationLease) Release() error {
	var releaseErr error
	l.once.Do(func() {
		for i := len(l.leases) - 1; i >= 0; i-- {
			if l.leases[i] == nil {
				continue
			}
			if err := l.leases[i].Release(); err != nil && releaseErr == nil {
				releaseErr = err
			}
		}
	})
	return releaseErr
}

func (s *BackupService) acquireLegacyBulkBackupLease(ctx context.Context) (domain.BulkOperationLease, error) {
	if err := contextError(ctx); err != nil {
		return nil, err
	}
	localLease, acquired := s.legacyBulkBackupGate.TryAcquire()
	if !acquired {
		return nil, ErrBulkBackupAlreadyActive
	}
	localLease = newBulkOperationMetricLease(localLease, legacyBulkBackupMetricOperation, "local")
	if s.bulkOperationLeaseRepo == nil {
		return localLease, nil
	}

	distributedLease, acquired, err := s.bulkOperationLeaseRepo.TryAcquireBulkOperationLease(ctx, legacyBulkBackupLeaseKey)
	if err != nil {
		if releaseErr := localLease.Release(); releaseErr != nil {
			log.Printf("Warning: failed to release local bulk backup gate after distributed limiter error: %v", releaseErr)
		}
		return nil, fmt.Errorf("%w: %v", ErrBulkOperationLimiterUnavailable, err)
	}
	if !acquired {
		if releaseErr := localLease.Release(); releaseErr != nil {
			log.Printf("Warning: failed to release local bulk backup gate after distributed limiter rejection: %v", releaseErr)
		}
		return nil, ErrBulkBackupAlreadyActive
	}
	distributedLease = newBulkOperationMetricLease(distributedLease, legacyBulkBackupMetricOperation, "distributed")

	return &compositeBulkOperationLease{leases: []domain.BulkOperationLease{localLease, distributedLease}}, nil
}

func recordLegacyBulkBackupLocalConcurrencyLimit() {
	observability.Default().SetBulkOperationConcurrencyLimit(legacyBulkBackupMetricOperation, "global", "local", 1)
}

func recordLegacyBulkBackupDistributedConcurrencyLimit() {
	observability.Default().SetBulkOperationConcurrencyLimit(legacyBulkBackupMetricOperation, "global", "distributed", 1)
}

func newBulkOperationMetricLease(lease domain.BulkOperationLease, operation, source string) domain.BulkOperationLease {
	if lease == nil {
		return nil
	}
	observability.Default().SetBulkOperationInFlight(operation, source, 1)
	return &bulkOperationMetricLease{
		lease:     lease,
		operation: operation,
		source:    source,
	}
}

type bulkOperationMetricLease struct {
	lease     domain.BulkOperationLease
	operation string
	source    string
}

func (l *bulkOperationMetricLease) Release() error {
	err := l.lease.Release()
	observability.Default().SetBulkOperationInFlight(l.operation, l.source, 0)
	return err
}

func releaseBulkOperationLease(label string, lease domain.BulkOperationLease) {
	if lease == nil {
		return
	}
	if err := lease.Release(); err != nil {
		log.Printf("Warning: failed to release %s bulk operation lease: %v", label, err)
	}
}
