package postgres

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/domain"
)

// InstanceBackupRepo implements domain.InstanceBackupRepository using PostgreSQL.
type InstanceBackupRepo struct {
	db *DB
}

// NewInstanceBackupRepo creates a new InstanceBackupRepo.
func NewInstanceBackupRepo(db *sql.DB) *InstanceBackupRepo {
	return &InstanceBackupRepo{db: wrapDB(db)}
}

// Create inserts a new instance backup record.
// If backup.ID is uuid.Nil, a new UUID is auto-generated.
// If backup.CreatedAt is zero, it is set to time.Now().UTC().
// If backup.Trigger is empty, it defaults to "manual".
func (r *InstanceBackupRepo) Create(backup *domain.InstanceBackup) error {
	if backup.ID == uuid.Nil {
		backup.ID = uuid.New()
	}
	if backup.CreatedAt.IsZero() {
		backup.CreatedAt = time.Now().UTC()
	}
	trigger := string(backup.Trigger)
	if trigger == "" {
		trigger = string(domain.InstanceBackupTriggerManual)
		backup.Trigger = domain.InstanceBackupTriggerManual
	}

	_, err := r.db.Exec(
		`INSERT INTO instance_backups (id, file_name, file_path, size_bytes, sha256, app_version, migration_version, status, error_message, trigger_type, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		backup.ID.String(),
		backup.FileName,
		backup.FilePath,
		backup.SizeBytes,
		backup.SHA256,
		backup.AppVersion,
		backup.MigrationVersion,
		string(backup.Status),
		backup.ErrorMessage,
		trigger,
		backup.CreatedAt,
	)
	return err
}

// GetByID returns an instance backup by ID, or nil,nil if not found.
func (r *InstanceBackupRepo) GetByID(id uuid.UUID) (*domain.InstanceBackup, error) {
	row := r.db.QueryRow(
		`SELECT id, file_name, file_path, size_bytes, sha256, app_version, migration_version, status, error_message, trigger_type, created_at
		 FROM instance_backups WHERE id = ?`,
		id.String(),
	)
	return scanInstanceBackupRow(row)
}

// List returns all instance backups ordered by created_at DESC.
func (r *InstanceBackupRepo) List() ([]domain.InstanceBackup, error) {
	rows, err := r.db.Query(
		`SELECT id, file_name, file_path, size_bytes, sha256, app_version, migration_version, status, error_message, trigger_type, created_at
		 FROM instance_backups ORDER BY created_at DESC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanInstanceBackupRows(rows)
}

// Update updates all mutable fields of an instance backup.
// Returns an error if no row with the given ID exists.
func (r *InstanceBackupRepo) Update(backup *domain.InstanceBackup) error {
	trigger := string(backup.Trigger)
	if trigger == "" {
		trigger = string(domain.InstanceBackupTriggerManual)
	}

	res, err := r.db.Exec(
		`UPDATE instance_backups
		 SET file_name = ?, file_path = ?, size_bytes = ?, sha256 = ?, app_version = ?, migration_version = ?, status = ?, error_message = ?, trigger_type = ?
		 WHERE id = ?`,
		backup.FileName,
		backup.FilePath,
		backup.SizeBytes,
		backup.SHA256,
		backup.AppVersion,
		backup.MigrationVersion,
		string(backup.Status),
		backup.ErrorMessage,
		trigger,
		backup.ID.String(),
	)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("instance backup %s not found", backup.ID)
	}
	return nil
}

// Delete removes an instance backup by ID.
// Returns an error if no row with the given ID exists.
func (r *InstanceBackupRepo) Delete(id uuid.UUID) error {
	res, err := r.db.Exec(`DELETE FROM instance_backups WHERE id = ?`, id.String())
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("instance backup %s not found", id)
	}
	return nil
}

// ListSuccessfulOldest returns all successful backups ordered by created_at ASC (oldest first).
func (r *InstanceBackupRepo) ListSuccessfulOldest() ([]domain.InstanceBackup, error) {
	rows, err := r.db.Query(
		`SELECT id, file_name, file_path, size_bytes, sha256, app_version, migration_version, status, error_message, trigger_type, created_at
		 FROM instance_backups WHERE status = 'success' ORDER BY created_at ASC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanInstanceBackupRows(rows)
}

// DeleteFailedOlderThan deletes failed backup records with created_at before cutoff
// and returns the number of records deleted.
func (r *InstanceBackupRepo) DeleteFailedOlderThan(cutoff time.Time) (int, error) {
	res, err := r.db.Exec(
		`DELETE FROM instance_backups WHERE status = 'failed' AND created_at < ?`, cutoff,
	)
	if err != nil {
		return 0, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, err
	}
	return int(n), nil
}

// --- Helpers ---

func scanInstanceBackupRow(row *sql.Row) (*domain.InstanceBackup, error) {
	var idStr, status, triggerType string
	var b domain.InstanceBackup
	if err := row.Scan(
		&idStr, &b.FileName, &b.FilePath, &b.SizeBytes, &b.SHA256,
		&b.AppVersion, &b.MigrationVersion, &status, &b.ErrorMessage, &triggerType, &b.CreatedAt,
	); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	id, err := uuid.Parse(idStr)
	if err != nil {
		return nil, fmt.Errorf("invalid instance backup id: %w", err)
	}
	b.ID = id
	b.Status = domain.InstanceBackupStatus(status)
	b.Trigger = domain.InstanceBackupTrigger(triggerType)
	return &b, nil
}

func scanInstanceBackupRows(rows *sql.Rows) ([]domain.InstanceBackup, error) {
	var backups []domain.InstanceBackup
	for rows.Next() {
		var idStr, status, triggerType string
		var b domain.InstanceBackup
		if err := rows.Scan(
			&idStr, &b.FileName, &b.FilePath, &b.SizeBytes, &b.SHA256,
			&b.AppVersion, &b.MigrationVersion, &status, &b.ErrorMessage, &triggerType, &b.CreatedAt,
		); err != nil {
			return nil, err
		}
		id, err := uuid.Parse(idStr)
		if err != nil {
			return nil, fmt.Errorf("invalid instance backup id: %w", err)
		}
		b.ID = id
		b.Status = domain.InstanceBackupStatus(status)
		b.Trigger = domain.InstanceBackupTrigger(triggerType)
		backups = append(backups, b)
	}
	return backups, rows.Err()
}
