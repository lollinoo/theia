package sqlite

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/domain"
)

// BackupJobRepo implements domain.BackupJobRepository using SQLite.
type BackupJobRepo struct {
	db *DB
}

// NewBackupJobRepo creates a new BackupJobRepo.
func NewBackupJobRepo(db *sql.DB) *BackupJobRepo {
	return &BackupJobRepo{db: wrapDB(db)}
}

// Create inserts a new backup job.
func (r *BackupJobRepo) Create(job *domain.BackupJob) error {
	if job.ID == uuid.Nil {
		job.ID = uuid.New()
	}
	if job.CreatedAt.IsZero() {
		job.CreatedAt = time.Now().UTC()
	}

	_, err := r.db.Exec(
		`INSERT INTO backup_jobs (id, device_id, status, error_message, created_at)
		 VALUES (?, ?, ?, ?, ?)`,
		job.ID.String(), job.DeviceID.String(), string(job.Status), job.ErrorMessage, job.CreatedAt,
	)
	return err
}

// GetByID returns a backup job by ID.
func (r *BackupJobRepo) GetByID(id uuid.UUID) (*domain.BackupJob, error) {
	row := r.db.QueryRow(
		`SELECT id, device_id, status, error_message, created_at FROM backup_jobs WHERE id = ?`,
		id.String(),
	)
	return scanJobRow(row)
}

// GetByDeviceID returns all backup jobs for a device, newest first.
func (r *BackupJobRepo) GetByDeviceID(deviceID uuid.UUID) ([]domain.BackupJob, error) {
	rows, err := r.db.Query(
		`SELECT id, device_id, status, error_message, created_at
		 FROM backup_jobs WHERE device_id = ? ORDER BY created_at DESC`,
		deviceID.String(),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var jobs []domain.BackupJob
	for rows.Next() {
		j, err := scanJobRows(rows)
		if err != nil {
			return nil, err
		}
		jobs = append(jobs, *j)
	}
	return jobs, rows.Err()
}

// GetLatestByDeviceID returns the most recent successful backup job.
func (r *BackupJobRepo) GetLatestByDeviceID(deviceID uuid.UUID) (*domain.BackupJob, error) {
	row := r.db.QueryRow(
		`SELECT id, device_id, status, error_message, created_at
		 FROM backup_jobs WHERE device_id = ? AND status = 'success'
		 ORDER BY created_at DESC LIMIT 1`,
		deviceID.String(),
	)
	return scanJobRow(row)
}

// Update updates a backup job's status and error message.
func (r *BackupJobRepo) Update(job *domain.BackupJob) error {
	res, err := r.db.Exec(
		`UPDATE backup_jobs SET status = ?, error_message = ? WHERE id = ?`,
		string(job.Status), job.ErrorMessage, job.ID.String(),
	)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("backup job %s not found", job.ID)
	}
	return nil
}

// Delete removes a backup job by ID.
func (r *BackupJobRepo) Delete(id uuid.UUID) error {
	res, err := r.db.Exec(`DELETE FROM backup_jobs WHERE id = ?`, id.String())
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("backup job %s not found", id)
	}
	return nil
}

// DeleteByDeviceID removes all backup jobs for a device.
func (r *BackupJobRepo) DeleteByDeviceID(deviceID uuid.UUID) error {
	_, err := r.db.Exec(`DELETE FROM backup_jobs WHERE device_id = ?`, deviceID.String())
	return err
}

// ListSuccessfulByDeviceOldest returns all successful backup jobs for a device, oldest first.
func (r *BackupJobRepo) ListSuccessfulByDeviceOldest(deviceID uuid.UUID) ([]domain.BackupJob, error) {
	rows, err := r.db.Query(
		`SELECT id, device_id, status, error_message, created_at
		 FROM backup_jobs WHERE device_id = ? AND status = 'success'
		 ORDER BY created_at ASC`,
		deviceID.String(),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var jobs []domain.BackupJob
	for rows.Next() {
		j, err := scanJobRows(rows)
		if err != nil {
			return nil, err
		}
		jobs = append(jobs, *j)
	}
	return jobs, rows.Err()
}

// ListAllDeviceIDs returns distinct device IDs from backup_jobs table.
func (r *BackupJobRepo) ListAllDeviceIDs() ([]uuid.UUID, error) {
	rows, err := r.db.Query(`SELECT DISTINCT device_id FROM backup_jobs`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []uuid.UUID
	for rows.Next() {
		var idStr string
		if err := rows.Scan(&idStr); err != nil {
			return nil, err
		}
		id, err := uuid.Parse(idStr)
		if err != nil {
			return nil, fmt.Errorf("invalid device id: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// DeleteFailedOlderThan removes failed backup job records older than cutoff.
func (r *BackupJobRepo) DeleteFailedOlderThan(cutoff time.Time) (int, error) {
	res, err := r.db.Exec(
		`DELETE FROM backup_jobs WHERE status = 'failed' AND created_at < ?`,
		cutoff,
	)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

func scanJobRow(row *sql.Row) (*domain.BackupJob, error) {
	var idStr, deviceIDStr, status string
	var j domain.BackupJob
	if err := row.Scan(&idStr, &deviceIDStr, &status, &j.ErrorMessage, &j.CreatedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	id, err := uuid.Parse(idStr)
	if err != nil {
		return nil, fmt.Errorf("invalid job id: %w", err)
	}
	deviceID, err := uuid.Parse(deviceIDStr)
	if err != nil {
		return nil, fmt.Errorf("invalid device id: %w", err)
	}
	j.ID = id
	j.DeviceID = deviceID
	j.Status = domain.BackupStatus(status)
	return &j, nil
}

func scanJobRows(rows *sql.Rows) (*domain.BackupJob, error) {
	var idStr, deviceIDStr, status string
	var j domain.BackupJob
	if err := rows.Scan(&idStr, &deviceIDStr, &status, &j.ErrorMessage, &j.CreatedAt); err != nil {
		return nil, err
	}
	id, err := uuid.Parse(idStr)
	if err != nil {
		return nil, fmt.Errorf("invalid job id: %w", err)
	}
	deviceID, err := uuid.Parse(deviceIDStr)
	if err != nil {
		return nil, fmt.Errorf("invalid device id: %w", err)
	}
	j.ID = id
	j.DeviceID = deviceID
	j.Status = domain.BackupStatus(status)
	return &j, nil
}
