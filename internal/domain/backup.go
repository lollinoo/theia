package domain

import (
	"time"

	"github.com/google/uuid"
)

// SSHAuthMethod indicates how to authenticate over SSH.
type SSHAuthMethod string

const (
	SSHAuthPassword SSHAuthMethod = "password"
	SSHAuthKey      SSHAuthMethod = "key"
)

// BackupStatus represents the current state of a backup job.
type BackupStatus string

const (
	BackupStatusPending BackupStatus = "pending"
	BackupStatusRunning BackupStatus = "running"
	BackupStatusSuccess BackupStatus = "success"
	BackupStatusFailed  BackupStatus = "failed"
)

// BackupJob represents a single backup operation that produces multiple files.
type BackupJob struct {
	ID           uuid.UUID    `json:"id"`
	DeviceID     uuid.UUID    `json:"device_id"`
	Status       BackupStatus `json:"status"`
	ErrorMessage string       `json:"error_message"`
	CreatedAt    time.Time    `json:"created_at"`
	Files        []BackupFile `json:"files,omitempty"`
}

// BackupFile represents a single file produced by a backup job.
type BackupFile struct {
	ID        uuid.UUID `json:"id"`
	JobID     uuid.UUID `json:"job_id"`
	FileType  string    `json:"file_type"` // "running", "compact", "verbose", "binary"
	FileName  string    `json:"file_name"`
	FilePath  string    `json:"-"` // disk path, not exposed to API
	FileHash  string    `json:"file_hash"`
	SizeBytes int       `json:"size_bytes"`
	CreatedAt time.Time `json:"created_at"`
}

// BackupJobRepository defines persistence operations for backup jobs.
type BackupJobRepository interface {
	Create(job *BackupJob) error
	GetByID(id uuid.UUID) (*BackupJob, error)
	GetByDeviceID(deviceID uuid.UUID) ([]BackupJob, error)
	GetLatestByDeviceID(deviceID uuid.UUID) (*BackupJob, error)
	Update(job *BackupJob) error
	Delete(id uuid.UUID) error
	DeleteByDeviceID(deviceID uuid.UUID) error
}

// BackupFileRepository defines persistence operations for backup files.
type BackupFileRepository interface {
	Create(file *BackupFile) error
	GetByJobID(jobID uuid.UUID) ([]BackupFile, error)
	GetByID(id uuid.UUID) (*BackupFile, error)
	DeleteByJobID(jobID uuid.UUID) error
}
