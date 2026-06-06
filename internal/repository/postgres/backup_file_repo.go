package postgres

// This file defines backup file repo persistence behavior, ordering guarantees, and not-found conventions.

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/domain"
)

// BackupFileRepo implements domain.BackupFileRepository using PostgreSQL.
type BackupFileRepo struct {
	db *DB
}

// NewBackupFileRepo creates a new BackupFileRepo.
func NewBackupFileRepo(db *sql.DB) *BackupFileRepo {
	return &BackupFileRepo{db: wrapDB(db)}
}

// Create inserts a new backup file record.
func (r *BackupFileRepo) Create(file *domain.BackupFile) error {
	if file.ID == uuid.Nil {
		file.ID = uuid.New()
	}
	if file.CreatedAt.IsZero() {
		file.CreatedAt = time.Now().UTC()
	}

	_, err := r.db.Exec(
		`INSERT INTO backup_files (id, job_id, file_type, file_name, file_path, file_hash, size_bytes, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		file.ID.String(), file.JobID.String(), file.FileType, file.FileName, file.FilePath, file.FileHash, file.SizeBytes, file.CreatedAt,
	)
	return err
}

// GetByJobID returns all files for a backup job.
func (r *BackupFileRepo) GetByJobID(jobID uuid.UUID) ([]domain.BackupFile, error) {
	rows, err := r.db.Query(
		`SELECT id, job_id, file_type, file_name, file_path, file_hash, size_bytes, created_at
		 FROM backup_files WHERE job_id = ? ORDER BY file_type`,
		jobID.String(),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var files []domain.BackupFile
	for rows.Next() {
		var idStr, jobIDStr string
		var f domain.BackupFile
		if err := rows.Scan(&idStr, &jobIDStr, &f.FileType, &f.FileName, &f.FilePath, &f.FileHash, &f.SizeBytes, &f.CreatedAt); err != nil {
			return nil, err
		}
		id, err := uuid.Parse(idStr)
		if err != nil {
			return nil, fmt.Errorf("invalid file id: %w", err)
		}
		jID, err := uuid.Parse(jobIDStr)
		if err != nil {
			return nil, fmt.Errorf("invalid job id: %w", err)
		}
		f.ID = id
		f.JobID = jID
		files = append(files, f)
	}
	return files, rows.Err()
}

// GetByID returns a single backup file by ID.
func (r *BackupFileRepo) GetByID(id uuid.UUID) (*domain.BackupFile, error) {
	var idStr, jobIDStr string
	var f domain.BackupFile
	err := r.db.QueryRow(
		`SELECT id, job_id, file_type, file_name, file_path, file_hash, size_bytes, created_at
		 FROM backup_files WHERE id = ?`,
		id.String(),
	).Scan(&idStr, &jobIDStr, &f.FileType, &f.FileName, &f.FilePath, &f.FileHash, &f.SizeBytes, &f.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	fID, err := uuid.Parse(idStr)
	if err != nil {
		return nil, fmt.Errorf("invalid file id: %w", err)
	}
	jID, err := uuid.Parse(jobIDStr)
	if err != nil {
		return nil, fmt.Errorf("invalid job id: %w", err)
	}
	f.ID = fID
	f.JobID = jID
	return &f, nil
}

// DeleteByJobID removes all files for a backup job.
func (r *BackupFileRepo) DeleteByJobID(jobID uuid.UUID) error {
	_, err := r.db.Exec(`DELETE FROM backup_files WHERE job_id = ?`, jobID.String())
	return err
}
