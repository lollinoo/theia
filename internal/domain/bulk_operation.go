package domain

import "context"

// BulkOperationLease represents a distributed lock held for a bounded bulk operation.
type BulkOperationLease interface {
	Release() error
}

// BulkOperationLeaseRepository coordinates bulk operations across processes.
type BulkOperationLeaseRepository interface {
	TryAcquireBulkOperationLease(ctx context.Context, key string) (BulkOperationLease, bool, error)
}
