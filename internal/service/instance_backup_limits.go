package service

// This file defines instance backup limits backup and restore service behavior, including filesystem safety and cleanup expectations.

import (
	"fmt"
	"time"
)

// RestoreArchiveLimits defines defensive quotas for uploaded restore archives.
type RestoreArchiveLimits struct {
	MaxCompressedBytes int64
	MaxTotalBytes      int64
	MaxEntryBytes      int64
	MaxFileEntries     int
}

// RestoreLimitError marks restore validation failures caused by defensive quota limits.
type RestoreLimitError struct {
	message string
}

// Error returns the restore quota failure message used by API error mapping.
func (e *RestoreLimitError) Error() string {
	return e.message
}

// newRestoreLimitError wraps a formatted message in the restore quota error type.
func newRestoreLimitError(format string, args ...interface{}) error {
	return &RestoreLimitError{message: fmt.Sprintf(format, args...)}
}

// DefaultRestoreArchiveLimits caps restore uploads and extracted archive content.
var DefaultRestoreArchiveLimits = RestoreArchiveLimits{
	MaxCompressedBytes: 256 << 20, // 256 MiB uploaded .tar.gz
	MaxTotalBytes:      1 << 30,   // 1 GiB expanded regular-file content
	MaxEntryBytes:      512 << 20, // 512 MiB per regular file
	MaxFileEntries:     25000,
}

// BackupArchiveLimits defines defensive quotas for instance backup archive creation.
type BackupArchiveLimits struct {
	MaxTotalBytes  int64
	MaxEntryBytes  int64
	MaxFileEntries int
	MaxDuration    time.Duration
}

// DefaultBackupArchiveLimits caps instance backup archive creation work.
var DefaultBackupArchiveLimits = BackupArchiveLimits{
	MaxTotalBytes:  2 << 30, // 2 GiB expanded archive content
	MaxEntryBytes:  1 << 30, // 1 GiB per archived file
	MaxFileEntries: 50000,
	MaxDuration:    30 * time.Minute,
}

// SetRestoreArchiveLimitsForTest overrides restore archive quotas in focused tests.
func (s *InstanceBackupService) SetRestoreArchiveLimitsForTest(limits RestoreArchiveLimits) {
	s.SetRestoreArchiveLimits(limits)
}

// SetRestoreArchiveLimits overrides restore archive quotas.
func (s *InstanceBackupService) SetRestoreArchiveLimits(limits RestoreArchiveLimits) {
	s.restoreLimits = normalizeRestoreArchiveLimits(limits)
}

// RestoreArchiveLimits returns the normalized current restore archive limits.
func (s *InstanceBackupService) RestoreArchiveLimits() RestoreArchiveLimits {
	return normalizeRestoreArchiveLimits(s.restoreLimits)
}

// SetBackupArchiveLimitsForTest overrides backup archive quotas in focused tests.
func (s *InstanceBackupService) SetBackupArchiveLimitsForTest(limits BackupArchiveLimits) {
	s.SetBackupArchiveLimits(limits)
}

// SetBackupArchiveLimits overrides backup archive creation quotas.
func (s *InstanceBackupService) SetBackupArchiveLimits(limits BackupArchiveLimits) {
	s.backupLimits = normalizeBackupArchiveLimits(limits)
}

// BackupArchiveLimits returns the normalized current backup archive limits.
func (s *InstanceBackupService) BackupArchiveLimits() BackupArchiveLimits {
	return normalizeBackupArchiveLimits(s.backupLimits)
}

// normalizeRestoreArchiveLimits fills missing restore limits with defensive defaults.
func normalizeRestoreArchiveLimits(limits RestoreArchiveLimits) RestoreArchiveLimits {
	defaults := DefaultRestoreArchiveLimits
	if limits.MaxCompressedBytes <= 0 {
		limits.MaxCompressedBytes = defaults.MaxCompressedBytes
	}
	if limits.MaxTotalBytes <= 0 {
		limits.MaxTotalBytes = defaults.MaxTotalBytes
	}
	if limits.MaxEntryBytes <= 0 {
		limits.MaxEntryBytes = defaults.MaxEntryBytes
	}
	if limits.MaxFileEntries <= 0 {
		limits.MaxFileEntries = defaults.MaxFileEntries
	}
	return limits
}

// normalizeBackupArchiveLimits fills missing backup limits with defensive defaults.
func normalizeBackupArchiveLimits(limits BackupArchiveLimits) BackupArchiveLimits {
	defaults := DefaultBackupArchiveLimits
	if limits.MaxTotalBytes <= 0 {
		limits.MaxTotalBytes = defaults.MaxTotalBytes
	}
	if limits.MaxEntryBytes <= 0 {
		limits.MaxEntryBytes = defaults.MaxEntryBytes
	}
	if limits.MaxFileEntries <= 0 {
		limits.MaxFileEntries = defaults.MaxFileEntries
	}
	if limits.MaxDuration <= 0 {
		limits.MaxDuration = defaults.MaxDuration
	}
	return limits
}
