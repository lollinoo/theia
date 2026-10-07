package postgres

// This file defines bulk operation lease repo persistence behavior, ordering guarantees, and not-found conventions.

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/binary"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/lollinoo/theia/internal/domain"
)

// BulkOperationLeaseRepo coordinates bounded bulk operations with PostgreSQL advisory locks.
type BulkOperationLeaseRepo struct {
	db *sql.DB
}

// NewBulkOperationLeaseRepo constructs bulk operation lease repo state for the persistence boundary.
func NewBulkOperationLeaseRepo(db *sql.DB) *BulkOperationLeaseRepo {
	return &BulkOperationLeaseRepo{db: db}
}

func (r *BulkOperationLeaseRepo) TryAcquireBulkOperationLease(ctx context.Context, key string) (domain.BulkOperationLease, bool, error) {
	lease, blocked, err := r.TryAcquireBulkOperationLeases(ctx, []string{key})
	return lease, blocked == -1 && err == nil, err
}

// TryAcquireBulkOperationLeases acquires all keys on one connection. The index
// of a busy key is returned, or -1 on success; failed acquisition releases all keys.
func (r *BulkOperationLeaseRepo) TryAcquireBulkOperationLeases(ctx context.Context, keys []string) (domain.BulkOperationLease, int, error) {
	if len(keys) == 0 {
		return nil, 0, fmt.Errorf("bulk operation lease keys are required")
	}
	if r == nil || r.db == nil {
		return nil, 0, fmt.Errorf("bulk operation lease repository is not configured")
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, 0, err
	}
	for i, key := range keys {
		key = strings.TrimSpace(key)
		if key == "" {
			tx.Rollback()
			return nil, i, fmt.Errorf("bulk operation lease key is required")
		}
		var acquired bool
		if err := tx.QueryRowContext(ctx, rebindQuery(`SELECT pg_try_advisory_xact_lock(?)`), bulkOperationAdvisoryLockID(key)).Scan(&acquired); err != nil {
			tx.Rollback()
			return nil, i, err
		}
		if !acquired {
			tx.Rollback()
			return nil, i, nil
		}
	}

	return &bulkOperationAdvisoryLease{
		tx: tx,
	}, -1, nil
}

type bulkOperationAdvisoryLease struct {
	tx   *sql.Tx
	once sync.Once
}

func (l *bulkOperationAdvisoryLease) Release() error {
	var releaseErr error
	l.once.Do(func() {
		if l == nil || l.tx == nil {
			return
		}
		if err := l.tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
			releaseErr = err
		}
	})
	return releaseErr
}

func bulkOperationAdvisoryLockID(key string) int64 {
	sum := sha256.Sum256([]byte(key))
	return int64(binary.BigEndian.Uint64(sum[:8]))
}
