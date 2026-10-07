package postgres

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
)

// ReconcileBulkRunBatch projects job state in one bounded batch. Unchanged jobs
// cause no item/counter writes; ownership fencing still applies to transitions.
func (r *BulkBackupRunRepo) ReconcileBulkRunBatch(runID uuid.UUID, ids []uuid.UUID) (bool, error) {
	return r.ReconcileBulkRunBatchContext(context.Background(), runID, ids)
}

// ReconcileBulkRunBatchContext bounds pool and ownership-lock waits by the processor deadline.
func (r *BulkBackupRunRepo) ReconcileBulkRunBatchContext(ctx context.Context, runID uuid.UUID, ids []uuid.UUID) (bool, error) {
	if len(ids) == 0 {
		return true, nil
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	if err := r.lockProcessorContext(ctx, tx, runID); err != nil {
		return false, err
	}
	args := []interface{}{runID.String()}
	marks := make([]string, len(ids))
	for i, id := range ids {
		marks[i] = "?"
		args = append(args, id.String())
	}
	filter := "i.run_id=? AND i.id IN (" + strings.Join(marks, ",") + ")"
	result, err := tx.ExecContext(ctx, `WITH next AS (
 SELECT i.id,
 CASE WHEN j.id IS NULL THEN 'failed' ELSE j.status END AS status,
 CASE WHEN j.id IS NULL THEN 'backup job disappeared' ELSE j.error_message END AS reason
 FROM backup_bulk_run_items i LEFT JOIN backup_jobs j ON j.id=i.backup_job_id
 WHERE `+filter+` AND i.status IN ('active','queued','running')
 AND (j.id IS NULL OR j.status IN ('running','success','failed'))
)
UPDATE backup_bulk_run_items i SET status=n.status, reason=CASE WHEN n.status='failed' THEN n.reason ELSE '' END,
 updated_at=NOW(), completed_at=CASE WHEN n.status IN ('success','failed') THEN NOW() ELSE NULL END
FROM next n WHERE i.id=n.id AND i.status IS DISTINCT FROM n.status`, args...)
	if err != nil {
		return false, err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	if n > 0 {
		if _, err := tx.ExecContext(ctx, bulkCountersSQL, runID.String(), runID.String()); err != nil {
			return false, err
		}
	}
	var total, remaining int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*),COUNT(*) FILTER(WHERE i.status NOT IN ('success','failed','skipped','cancelled'))
 FROM backup_bulk_run_items i WHERE `+filter, args...).Scan(&total, &remaining); err != nil {
		return false, err
	}
	if total != len(ids) {
		return false, fmt.Errorf("bulk batch items disappeared")
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return remaining == 0, nil
}

const bulkCountersSQL = `UPDATE backup_bulk_runs r SET total_count=c.total, queued_count=c.queued,
 success_count=c.success,failed_count=c.failed,skipped_count=c.skipped,cancelled_count=c.cancelled
 FROM (SELECT COUNT(*) AS total,
 COUNT(*) FILTER(WHERE status IN ('active','queued','running')) AS queued,
 COUNT(*) FILTER(WHERE status='success') AS success,COUNT(*) FILTER(WHERE status='failed') AS failed,
 COUNT(*) FILTER(WHERE status='skipped') AS skipped,COUNT(*) FILTER(WHERE status='cancelled') AS cancelled
 FROM backup_bulk_run_items WHERE run_id=?) c WHERE r.id=?`
