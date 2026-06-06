package service

import (
	"errors"
	"fmt"
)

// BulkOperationLimits holds defensive quotas for bulk backup and download requests.
type BulkOperationLimits struct {
	BulkBackupMaxDevices              int
	BulkBackupMaxQueuedJobs           int
	BulkDownloadMaxDevices            int
	BulkDownloadMaxFiles              int
	BulkDownloadMaxBytes              int64
	BulkDownloadMaxConcurrentPerActor int
	BulkDownloadMaxConcurrentGlobal   int
}

// DefaultBulkOperationLimits bounds bulk workflows while preserving ordinary fleet use.
var DefaultBulkOperationLimits = BulkOperationLimits{
	BulkBackupMaxDevices:              100,
	BulkBackupMaxQueuedJobs:           100,
	BulkDownloadMaxDevices:            100,
	BulkDownloadMaxFiles:              500,
	BulkDownloadMaxBytes:              512 << 20,
	BulkDownloadMaxConcurrentPerActor: 1,
	BulkDownloadMaxConcurrentGlobal:   4,
}

// BulkLimitError reports a request that exceeds a configured bulk quota.
type BulkLimitError struct {
	Operation string
	Limit     string
	Max       int64
	Actual    int64
}

func (e *BulkLimitError) Error() string {
	return fmt.Sprintf("%s exceeds %s limit: requested %d, maximum %d", e.Operation, e.Limit, e.Actual, e.Max)
}

// IsBulkLimitError reports whether err is a bulk quota rejection.
func IsBulkLimitError(err error) bool {
	var target *BulkLimitError
	return errors.As(err, &target)
}

// BulkPathError reports an unsafe backup file path or zip entry name.
type BulkPathError struct {
	Path   string
	Reason string
}

func (e *BulkPathError) Error() string {
	if e.Path == "" {
		return e.Reason
	}
	return fmt.Sprintf("%s: %s", e.Reason, e.Path)
}

// IsBulkPathError reports whether err is an unsafe bulk download path rejection.
func IsBulkPathError(err error) bool {
	var target *BulkPathError
	return errors.As(err, &target)
}

func normalizeBulkOperationLimits(limits BulkOperationLimits) BulkOperationLimits {
	defaults := DefaultBulkOperationLimits
	if limits.BulkBackupMaxDevices <= 0 {
		limits.BulkBackupMaxDevices = defaults.BulkBackupMaxDevices
	}
	if limits.BulkBackupMaxQueuedJobs <= 0 {
		limits.BulkBackupMaxQueuedJobs = defaults.BulkBackupMaxQueuedJobs
	}
	if limits.BulkDownloadMaxDevices <= 0 {
		limits.BulkDownloadMaxDevices = defaults.BulkDownloadMaxDevices
	}
	if limits.BulkDownloadMaxFiles <= 0 {
		limits.BulkDownloadMaxFiles = defaults.BulkDownloadMaxFiles
	}
	if limits.BulkDownloadMaxBytes <= 0 {
		limits.BulkDownloadMaxBytes = defaults.BulkDownloadMaxBytes
	}
	if limits.BulkDownloadMaxConcurrentPerActor <= 0 {
		limits.BulkDownloadMaxConcurrentPerActor = defaults.BulkDownloadMaxConcurrentPerActor
	}
	if limits.BulkDownloadMaxConcurrentGlobal <= 0 {
		limits.BulkDownloadMaxConcurrentGlobal = defaults.BulkDownloadMaxConcurrentGlobal
	}
	return limits
}
