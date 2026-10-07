package postgres

import (
	"database/sql"
	"fmt"
	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/domain"
)

// ForProcessor returns an immutable repository scope for one lease owner. Every
// processor mutation locks the run row and verifies ownership before writing.
func (r *BulkBackupRunRepo) ForProcessor(runID uuid.UUID, owner string) domain.BulkBackupRunRepository {
	return &BulkBackupRunRepo{db: r.db, processorRunID: runID, processorOwner: owner}
}

func (r *BulkBackupRunRepo) lockProcessor(tx *Tx, runID uuid.UUID) error {
	if r.processorOwner == "" {
		return nil
	}
	if runID != r.processorRunID {
		return domain.ErrBulkBackupProcessorLeaseLost
	}
	var id string
	err := tx.QueryRow(`SELECT id FROM backup_bulk_runs WHERE id=? AND processing_owner=?
  AND processing_lease_expires_at>NOW() FOR UPDATE`, runID.String(), r.processorOwner).Scan(&id)
	if err == sql.ErrNoRows {
		return domain.ErrBulkBackupProcessorLeaseLost
	}
	if err != nil {
		return fmt.Errorf("checking bulk processor ownership: %w", err)
	}
	return nil
}

func (r *BulkBackupRunRepo) execProcessorMutation(runID uuid.UUID, query string, args ...interface{}) (sql.Result, error) {
	if r.processorOwner == "" {
		return r.db.Exec(query, args...)
	}
	tx, err := r.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if err := r.lockProcessor(tx, runID); err != nil {
		return nil, err
	}
	result, err := tx.Exec(query, args...)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return result, nil
}

// RecoverBulkRun resets interrupted work only after acquiring and checking the
// processor lease. Job and item repairs commit atomically with the run transition.
func (r *BulkBackupRunRepo) RecoverBulkRun(runID uuid.UUID) error {
	if r.processorOwner == "" {
		return domain.ErrBulkBackupProcessorLeaseLost
	}
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := r.lockProcessor(tx, runID); err != nil {
		return err
	}
	if _, err := tx.Exec(`UPDATE backup_jobs SET status='failed',error_message='interrupted by server restart'
  WHERE status IN ('pending','running') AND id IN (SELECT backup_job_id FROM backup_bulk_run_items
    WHERE run_id=? AND status NOT IN ('success','failed','skipped','cancelled'))`, runID.String()); err != nil {
		return err
	}
	if _, err := tx.Exec(`UPDATE backup_bulk_run_items SET status='checking',reason='',backup_job_id=NULL,completed_at=NULL,updated_at=NOW()
  WHERE run_id=? AND status NOT IN ('success','failed','skipped','cancelled')`, runID.String()); err != nil {
		return err
	}
	if _, err := tx.Exec(`UPDATE backup_bulk_runs SET status=CASE WHEN status='pausing' THEN 'paused' ELSE status END,
  cancel_requested=CASE WHEN status='pausing' THEN FALSE ELSE cancel_requested END WHERE id=?`, runID.String()); err != nil {
		return err
	}
	return tx.Commit()
}
