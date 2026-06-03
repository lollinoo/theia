package postgres

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

func NewBulkOperationLeaseRepo(db *sql.DB) *BulkOperationLeaseRepo {
	return &BulkOperationLeaseRepo{db: db}
}

func (r *BulkOperationLeaseRepo) TryAcquireBulkOperationLease(ctx context.Context, key string) (domain.BulkOperationLease, bool, error) {
	key = strings.TrimSpace(key)
	if key == "" {
		return nil, false, fmt.Errorf("bulk operation lease key is required")
	}
	if r == nil || r.db == nil {
		return nil, false, fmt.Errorf("bulk operation lease repository is not configured")
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, false, err
	}

	lockID := bulkOperationAdvisoryLockID(key)
	var acquired bool
	if err := tx.QueryRowContext(ctx, rebindQuery(`SELECT pg_try_advisory_xact_lock(?)`), lockID).Scan(&acquired); err != nil {
		_ = tx.Rollback()
		return nil, false, err
	}
	if !acquired {
		_ = tx.Rollback()
		return nil, false, nil
	}

	return &bulkOperationAdvisoryLease{
		tx: tx,
	}, true, nil
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
