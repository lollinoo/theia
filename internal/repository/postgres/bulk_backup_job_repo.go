package postgres

import (
	"fmt"
	"github.com/lollinoo/theia/internal/domain"
	"time"
)

// CreateBulkRunJob creates the job and attaches it to an active item in one
// transaction. A failed association never leaves a pending, unexecutable job.
func (r *BulkBackupRunRepo) CreateBulkRunJob(item *domain.BulkBackupRunItem, job *domain.BackupJob) error {
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	now := time.Now().UTC()
	if _, err := tx.Exec(`INSERT INTO backup_jobs (id,device_id,status,error_message,created_at) VALUES (?,?,?,?,?)`, job.ID.String(), job.DeviceID.String(), string(job.Status), job.ErrorMessage, now); err != nil {
		return fmt.Errorf("creating bulk job: %w", err)
	}
	result, err := tx.Exec(`UPDATE backup_bulk_run_items SET backup_job_id=?, device_name=?, updated_at=?
  WHERE id=? AND run_id=? AND device_id=? AND status='active' AND backup_job_id IS NULL`, job.ID.String(), item.DeviceName, now, item.ID.String(), item.RunID.String(), job.DeviceID.String())
	if err != nil {
		return fmt.Errorf("attaching bulk job: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return fmt.Errorf("bulk item %s is no longer claimable", item.ID)
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	job.CreatedAt = now
	item.BackupJobID = &job.ID
	item.UpdatedAt = now
	return nil
}
