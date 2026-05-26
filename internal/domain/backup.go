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

type BulkBackupRunStatus string

const (
	BulkBackupRunStatusRunning    BulkBackupRunStatus = "running"
	BulkBackupRunStatusPausing    BulkBackupRunStatus = "pausing"
	BulkBackupRunStatusPaused     BulkBackupRunStatus = "paused"
	BulkBackupRunStatusCancelling BulkBackupRunStatus = "cancelling"
	BulkBackupRunStatusSuccess    BulkBackupRunStatus = "success"
	BulkBackupRunStatusPartial    BulkBackupRunStatus = "partial"
	BulkBackupRunStatusFailed     BulkBackupRunStatus = "failed"
	BulkBackupRunStatusCancelled  BulkBackupRunStatus = "cancelled"
)

type BulkBackupRunItemStatus string

const (
	BulkBackupRunItemStatusChecking  BulkBackupRunItemStatus = "checking"
	BulkBackupRunItemStatusSkipped   BulkBackupRunItemStatus = "skipped"
	BulkBackupRunItemStatusActive    BulkBackupRunItemStatus = "active"
	BulkBackupRunItemStatusQueued    BulkBackupRunItemStatus = "queued"
	BulkBackupRunItemStatusRunning   BulkBackupRunItemStatus = "running"
	BulkBackupRunItemStatusSuccess   BulkBackupRunItemStatus = "success"
	BulkBackupRunItemStatusFailed    BulkBackupRunItemStatus = "failed"
	BulkBackupRunItemStatusCancelled BulkBackupRunItemStatus = "cancelled"
)

// BulkBackupRun tracks one durable multi-device backup orchestration.
type BulkBackupRun struct {
	ID              uuid.UUID
	Status          BulkBackupRunStatus
	BatchSize       int
	TotalCount      int
	QueuedCount     int
	SuccessCount    int
	FailedCount     int
	SkippedCount    int
	CancelledCount  int
	ErrorMessage    string
	CancelRequested bool
	CreatedBy       string
	CreatedAt       time.Time
	StartedAt       *time.Time
	CompletedAt     *time.Time
	Items           []BulkBackupRunItem
}

// BulkBackupRunItem tracks one device within a bulk backup run.
type BulkBackupRunItem struct {
	ID          uuid.UUID
	RunID       uuid.UUID
	DeviceID    uuid.UUID
	DeviceName  string
	Status      BulkBackupRunItemStatus
	Reason      string
	BackupJobID *uuid.UUID
	CreatedAt   time.Time
	UpdatedAt   time.Time
	CompletedAt *time.Time
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

	// ListSuccessfulByDeviceOldest returns all successful backup jobs for a device,
	// ordered oldest first (ascending by created_at). Used for retention sweep.
	ListSuccessfulByDeviceOldest(deviceID uuid.UUID) ([]BackupJob, error)

	// ListAllDeviceIDs returns distinct device IDs that have at least one backup job.
	// Used by retention sweep to iterate over all devices.
	ListAllDeviceIDs() ([]uuid.UUID, error)

	// DeleteFailedOlderThan removes failed backup job records older than the given time.
	// Returns the count of deleted records.
	DeleteFailedOlderThan(cutoff time.Time) (int, error)
}

// BulkBackupRunRepository defines durable orchestration operations for bulk backup runs.
type BulkBackupRunRepository interface {
	CreateRun(run *BulkBackupRun, items []BulkBackupRunItem) error
	GetRun(id uuid.UUID) (*BulkBackupRun, error)
	GetLatestRun() (*BulkBackupRun, error)
	GetActiveRun() (*BulkBackupRun, error)
	ListResumableRuns() ([]BulkBackupRun, error)
	UpdateRun(run *BulkBackupRun) error
	ListRunItems(runID uuid.UUID) ([]BulkBackupRunItem, error)
	UpdateRunItem(item *BulkBackupRunItem) error
	RecalculateRunCounters(runID uuid.UUID) (*BulkBackupRun, error)
}

// BackupFileRepository defines persistence operations for backup files.
type BackupFileRepository interface {
	Create(file *BackupFile) error
	GetByJobID(jobID uuid.UUID) ([]BackupFile, error)
	GetByID(id uuid.UUID) (*BackupFile, error)
	DeleteByJobID(jobID uuid.UUID) error
}
