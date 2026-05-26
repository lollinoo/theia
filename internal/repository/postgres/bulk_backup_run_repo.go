package postgres

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/domain"
)

// BulkBackupRunRepo implements domain.BulkBackupRunRepository using PostgreSQL.
type BulkBackupRunRepo struct {
	db *DB
}

func NewBulkBackupRunRepo(db *sql.DB) *BulkBackupRunRepo {
	return &BulkBackupRunRepo{db: wrapDB(db)}
}

func (r *BulkBackupRunRepo) CreateRun(run *domain.BulkBackupRun, items []domain.BulkBackupRunItem) error {
	if run.ID == uuid.Nil {
		run.ID = uuid.New()
	}
	if run.BatchSize <= 0 {
		run.BatchSize = 10
	}
	if run.Status == "" {
		run.Status = domain.BulkBackupRunStatusRunning
	}
	if run.CreatedAt.IsZero() {
		run.CreatedAt = time.Now().UTC()
	}
	if run.StartedAt == nil {
		startedAt := run.CreatedAt
		run.StartedAt = &startedAt
	}
	run.TotalCount = len(items)

	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(
		`INSERT INTO backup_bulk_runs (
			id, status, batch_size, total_count, queued_count, success_count, failed_count,
			skipped_count, cancelled_count, error_message, cancel_requested, created_by,
			created_at, started_at, completed_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		run.ID.String(),
		string(run.Status),
		run.BatchSize,
		run.TotalCount,
		run.QueuedCount,
		run.SuccessCount,
		run.FailedCount,
		run.SkippedCount,
		run.CancelledCount,
		run.ErrorMessage,
		run.CancelRequested,
		run.CreatedBy,
		run.CreatedAt,
		run.StartedAt,
		run.CompletedAt,
	); err != nil {
		return err
	}

	for i := range items {
		item := items[i]
		if item.ID == uuid.Nil {
			item.ID = uuid.New()
		}
		item.RunID = run.ID
		if item.Status == "" {
			item.Status = domain.BulkBackupRunItemStatusChecking
		}
		if item.CreatedAt.IsZero() {
			item.CreatedAt = run.CreatedAt
		}
		if item.UpdatedAt.IsZero() {
			item.UpdatedAt = item.CreatedAt
		}
		if _, err := tx.Exec(
			`INSERT INTO backup_bulk_run_items (
				id, run_id, device_id, device_name, status, reason, backup_job_id,
				created_at, updated_at, completed_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			item.ID.String(),
			item.RunID.String(),
			item.DeviceID.String(),
			item.DeviceName,
			string(item.Status),
			item.Reason,
			nullableUUIDString(item.BackupJobID),
			item.CreatedAt,
			item.UpdatedAt,
			item.CompletedAt,
		); err != nil {
			return err
		}
		items[i] = item
	}

	if err := tx.Commit(); err != nil {
		return err
	}
	run.Items = items
	return nil
}

func (r *BulkBackupRunRepo) GetRun(id uuid.UUID) (*domain.BulkBackupRun, error) {
	run, err := r.getRunByQuery(
		`SELECT id, status, batch_size, total_count, queued_count, success_count, failed_count,
			skipped_count, cancelled_count, error_message, cancel_requested, created_by,
			created_at, started_at, completed_at
		 FROM backup_bulk_runs WHERE id = ?`,
		id.String(),
	)
	if err != nil || run == nil {
		return run, err
	}
	items, err := r.ListRunItems(run.ID)
	if err != nil {
		return nil, err
	}
	run.Items = items
	return run, nil
}

func (r *BulkBackupRunRepo) GetLatestRun() (*domain.BulkBackupRun, error) {
	run, err := r.getRunByQuery(
		`SELECT id, status, batch_size, total_count, queued_count, success_count, failed_count,
			skipped_count, cancelled_count, error_message, cancel_requested, created_by,
			created_at, started_at, completed_at
		 FROM backup_bulk_runs ORDER BY created_at DESC LIMIT 1`,
	)
	if err != nil || run == nil {
		return run, err
	}
	items, err := r.ListRunItems(run.ID)
	if err != nil {
		return nil, err
	}
	run.Items = items
	return run, nil
}

func (r *BulkBackupRunRepo) GetActiveRun() (*domain.BulkBackupRun, error) {
	run, err := r.getRunByQuery(
		`SELECT id, status, batch_size, total_count, queued_count, success_count, failed_count,
			skipped_count, cancelled_count, error_message, cancel_requested, created_by,
			created_at, started_at, completed_at
		 FROM backup_bulk_runs WHERE status IN ('running', 'cancelling')
		 ORDER BY created_at ASC LIMIT 1`,
	)
	if err != nil || run == nil {
		return run, err
	}
	items, err := r.ListRunItems(run.ID)
	if err != nil {
		return nil, err
	}
	run.Items = items
	return run, nil
}

func (r *BulkBackupRunRepo) ListResumableRuns() ([]domain.BulkBackupRun, error) {
	rows, err := r.db.Query(
		`SELECT id, status, batch_size, total_count, queued_count, success_count, failed_count,
			skipped_count, cancelled_count, error_message, cancel_requested, created_by,
			created_at, started_at, completed_at
		 FROM backup_bulk_runs WHERE status IN ('running', 'cancelling')
		 ORDER BY created_at ASC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var runs []domain.BulkBackupRun
	for rows.Next() {
		run, err := scanBulkBackupRunRows(rows)
		if err != nil {
			return nil, err
		}
		runs = append(runs, *run)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i := range runs {
		items, err := r.ListRunItems(runs[i].ID)
		if err != nil {
			return nil, err
		}
		runs[i].Items = items
	}
	return runs, nil
}

func (r *BulkBackupRunRepo) UpdateRun(run *domain.BulkBackupRun) error {
	res, err := r.db.Exec(
		`UPDATE backup_bulk_runs
		 SET status = ?, batch_size = ?, total_count = ?, queued_count = ?, success_count = ?,
			failed_count = ?, skipped_count = ?, cancelled_count = ?, error_message = ?,
			cancel_requested = ?, created_by = ?, started_at = ?, completed_at = ?
		 WHERE id = ?`,
		string(run.Status),
		run.BatchSize,
		run.TotalCount,
		run.QueuedCount,
		run.SuccessCount,
		run.FailedCount,
		run.SkippedCount,
		run.CancelledCount,
		run.ErrorMessage,
		run.CancelRequested,
		run.CreatedBy,
		run.StartedAt,
		run.CompletedAt,
		run.ID.String(),
	)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("bulk backup run %s not found", run.ID)
	}
	return nil
}

func (r *BulkBackupRunRepo) ListRunItems(runID uuid.UUID) ([]domain.BulkBackupRunItem, error) {
	rows, err := r.db.Query(
		`SELECT id, run_id, device_id, device_name, status, reason, backup_job_id,
			created_at, updated_at, completed_at
		 FROM backup_bulk_run_items WHERE run_id = ? ORDER BY created_at ASC, id ASC`,
		runID.String(),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []domain.BulkBackupRunItem
	for rows.Next() {
		item, err := scanBulkBackupRunItemRows(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, *item)
	}
	return items, rows.Err()
}

func (r *BulkBackupRunRepo) UpdateRunItem(item *domain.BulkBackupRunItem) error {
	if item.UpdatedAt.IsZero() {
		item.UpdatedAt = time.Now().UTC()
	}
	res, err := r.db.Exec(
		`UPDATE backup_bulk_run_items
		 SET device_name = ?, status = ?, reason = ?, backup_job_id = ?, updated_at = ?, completed_at = ?
		 WHERE id = ?`,
		item.DeviceName,
		string(item.Status),
		item.Reason,
		nullableUUIDString(item.BackupJobID),
		item.UpdatedAt,
		item.CompletedAt,
		item.ID.String(),
	)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("bulk backup run item %s not found", item.ID)
	}
	return nil
}

func (r *BulkBackupRunRepo) RecalculateRunCounters(runID uuid.UUID) (*domain.BulkBackupRun, error) {
	items, err := r.ListRunItems(runID)
	if err != nil {
		return nil, err
	}
	run, err := r.GetRun(runID)
	if err != nil || run == nil {
		return run, err
	}
	run.TotalCount = len(items)
	run.QueuedCount = 0
	run.SuccessCount = 0
	run.FailedCount = 0
	run.SkippedCount = 0
	run.CancelledCount = 0
	for _, item := range items {
		switch item.Status {
		case domain.BulkBackupRunItemStatusQueued, domain.BulkBackupRunItemStatusRunning:
			run.QueuedCount++
		case domain.BulkBackupRunItemStatusSuccess:
			run.SuccessCount++
		case domain.BulkBackupRunItemStatusFailed:
			run.FailedCount++
		case domain.BulkBackupRunItemStatusSkipped:
			run.SkippedCount++
		case domain.BulkBackupRunItemStatusCancelled:
			run.CancelledCount++
		}
	}
	run.Items = items
	if err := r.UpdateRun(run); err != nil {
		return nil, err
	}
	return run, nil
}

func (r *BulkBackupRunRepo) getRunByQuery(query string, args ...interface{}) (*domain.BulkBackupRun, error) {
	row := r.db.QueryRow(query, args...)
	return scanBulkBackupRunRow(row)
}

func scanBulkBackupRunRow(row *sql.Row) (*domain.BulkBackupRun, error) {
	var idStr, status string
	var startedAt, completedAt sql.NullTime
	var run domain.BulkBackupRun
	if err := row.Scan(
		&idStr,
		&status,
		&run.BatchSize,
		&run.TotalCount,
		&run.QueuedCount,
		&run.SuccessCount,
		&run.FailedCount,
		&run.SkippedCount,
		&run.CancelledCount,
		&run.ErrorMessage,
		&run.CancelRequested,
		&run.CreatedBy,
		&run.CreatedAt,
		&startedAt,
		&completedAt,
	); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	id, err := uuid.Parse(idStr)
	if err != nil {
		return nil, fmt.Errorf("invalid bulk backup run id: %w", err)
	}
	run.ID = id
	run.Status = domain.BulkBackupRunStatus(status)
	if startedAt.Valid {
		run.StartedAt = &startedAt.Time
	}
	if completedAt.Valid {
		run.CompletedAt = &completedAt.Time
	}
	return &run, nil
}

func scanBulkBackupRunRows(rows *sql.Rows) (*domain.BulkBackupRun, error) {
	var idStr, status string
	var startedAt, completedAt sql.NullTime
	var run domain.BulkBackupRun
	if err := rows.Scan(
		&idStr,
		&status,
		&run.BatchSize,
		&run.TotalCount,
		&run.QueuedCount,
		&run.SuccessCount,
		&run.FailedCount,
		&run.SkippedCount,
		&run.CancelledCount,
		&run.ErrorMessage,
		&run.CancelRequested,
		&run.CreatedBy,
		&run.CreatedAt,
		&startedAt,
		&completedAt,
	); err != nil {
		return nil, err
	}
	id, err := uuid.Parse(idStr)
	if err != nil {
		return nil, fmt.Errorf("invalid bulk backup run id: %w", err)
	}
	run.ID = id
	run.Status = domain.BulkBackupRunStatus(status)
	if startedAt.Valid {
		run.StartedAt = &startedAt.Time
	}
	if completedAt.Valid {
		run.CompletedAt = &completedAt.Time
	}
	return &run, nil
}

func scanBulkBackupRunItemRows(rows *sql.Rows) (*domain.BulkBackupRunItem, error) {
	var idStr, runIDStr, deviceIDStr, status string
	var backupJobID sql.NullString
	var completedAt sql.NullTime
	var item domain.BulkBackupRunItem
	if err := rows.Scan(
		&idStr,
		&runIDStr,
		&deviceIDStr,
		&item.DeviceName,
		&status,
		&item.Reason,
		&backupJobID,
		&item.CreatedAt,
		&item.UpdatedAt,
		&completedAt,
	); err != nil {
		return nil, err
	}
	id, err := uuid.Parse(idStr)
	if err != nil {
		return nil, fmt.Errorf("invalid bulk backup run item id: %w", err)
	}
	runID, err := uuid.Parse(runIDStr)
	if err != nil {
		return nil, fmt.Errorf("invalid bulk backup run id: %w", err)
	}
	deviceID, err := uuid.Parse(deviceIDStr)
	if err != nil {
		return nil, fmt.Errorf("invalid bulk backup run item device id: %w", err)
	}
	item.ID = id
	item.RunID = runID
	item.DeviceID = deviceID
	item.Status = domain.BulkBackupRunItemStatus(status)
	if completedAt.Valid {
		item.CompletedAt = &completedAt.Time
	}
	if backupJobID.Valid && backupJobID.String != "" {
		parsedJobID, err := uuid.Parse(backupJobID.String)
		if err != nil {
			return nil, fmt.Errorf("invalid bulk backup run item backup job id: %w", err)
		}
		item.BackupJobID = &parsedJobID
	}
	return &item, nil
}
