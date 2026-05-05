package domain

import (
	"time"

	"github.com/google/uuid"
)

// InstanceBackupStatus represents the current state of an instance backup.
type InstanceBackupStatus string

const (
	// InstanceBackupStatusRunning indicates the backup is currently in progress.
	InstanceBackupStatusRunning InstanceBackupStatus = "running"
	// InstanceBackupStatusSuccess indicates the backup completed successfully.
	InstanceBackupStatusSuccess InstanceBackupStatus = "success"
	// InstanceBackupStatusFailed indicates the backup failed.
	InstanceBackupStatusFailed InstanceBackupStatus = "failed"
	// InstanceBackupStatusCancelled indicates the backup was cancelled before completion.
	InstanceBackupStatusCancelled InstanceBackupStatus = "cancelled"
)

// InstanceBackupTrigger represents what initiated a backup.
type InstanceBackupTrigger string

const (
	// InstanceBackupTriggerManual indicates a user-initiated backup.
	InstanceBackupTriggerManual InstanceBackupTrigger = "manual"
	// InstanceBackupTriggerScheduled indicates a scheduler-initiated backup.
	InstanceBackupTriggerScheduled InstanceBackupTrigger = "scheduled"
)

// InstanceBackup represents a full Theia instance backup (database + config files).
type InstanceBackup struct {
	ID               uuid.UUID             `json:"id"`
	FileName         string                `json:"file_name"`
	FilePath         string                `json:"-"` // disk path, not exposed to API
	SizeBytes        int64                 `json:"size_bytes"`
	SHA256           string                `json:"sha256"`
	AppVersion       string                `json:"app_version"`
	MigrationVersion int                   `json:"migration_version"`
	Status           InstanceBackupStatus  `json:"status"`
	ErrorMessage     string                `json:"error_message,omitempty"`
	Trigger          InstanceBackupTrigger `json:"trigger"`
	CreatedAt        time.Time             `json:"created_at"`
}

// InstanceBackupProgress describes best-effort progress for a running instance backup.
type InstanceBackupProgress struct {
	Phase   string `json:"phase"`
	Message string `json:"message"`
	Current int64  `json:"current"`
	Total   int64  `json:"total"`
}

// InstanceBackupRepository defines persistence operations for instance backups.
type InstanceBackupRepository interface {
	Create(backup *InstanceBackup) error
	GetByID(id uuid.UUID) (*InstanceBackup, error)
	List() ([]InstanceBackup, error)
	Update(backup *InstanceBackup) error
	Delete(id uuid.UUID) error
	// ListSuccessfulOldest returns all successful backups ordered by created_at ASC (oldest first).
	ListSuccessfulOldest() ([]InstanceBackup, error)
	// DeleteFailedOlderThan deletes failed backup records with created_at before cutoff
	// and returns the number of records deleted.
	DeleteFailedOlderThan(cutoff time.Time) (int, error)
}
