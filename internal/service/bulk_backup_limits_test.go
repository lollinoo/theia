package service

import (
	"errors"
	"testing"
)

func TestNormalizeBulkOperationLimitsFillsDefaultsAndPreservesOverrides(t *testing.T) {
	limits := normalizeBulkOperationLimits(BulkOperationLimits{
		BulkBackupMaxDevices:              12,
		BulkDownloadMaxFiles:              34,
		BulkDownloadMaxConcurrentPerActor: 2,
	})

	if limits.BulkBackupMaxDevices != 12 {
		t.Fatalf("BulkBackupMaxDevices = %d, want 12", limits.BulkBackupMaxDevices)
	}
	if limits.BulkDownloadMaxFiles != 34 {
		t.Fatalf("BulkDownloadMaxFiles = %d, want 34", limits.BulkDownloadMaxFiles)
	}
	if limits.BulkDownloadMaxConcurrentPerActor != 2 {
		t.Fatalf("BulkDownloadMaxConcurrentPerActor = %d, want 2", limits.BulkDownloadMaxConcurrentPerActor)
	}
	if limits.BulkBackupMaxQueuedJobs != DefaultBulkOperationLimits.BulkBackupMaxQueuedJobs {
		t.Fatalf("BulkBackupMaxQueuedJobs = %d, want default %d", limits.BulkBackupMaxQueuedJobs, DefaultBulkOperationLimits.BulkBackupMaxQueuedJobs)
	}
	if limits.BulkDownloadMaxBytes != DefaultBulkOperationLimits.BulkDownloadMaxBytes {
		t.Fatalf("BulkDownloadMaxBytes = %d, want default %d", limits.BulkDownloadMaxBytes, DefaultBulkOperationLimits.BulkDownloadMaxBytes)
	}
}

func TestBulkErrorPredicatesRecognizeWrappedErrors(t *testing.T) {
	limitErr := &BulkLimitError{Operation: "bulk download", Limit: "files", Max: 1, Actual: 2}
	if !IsBulkLimitError(errors.Join(errors.New("outer"), limitErr)) {
		t.Fatal("IsBulkLimitError returned false for wrapped bulk limit error")
	}

	pathErr := &BulkPathError{Path: "../escape", Reason: "backup file name contains traversal"}
	if !IsBulkPathError(errors.Join(errors.New("outer"), pathErr)) {
		t.Fatal("IsBulkPathError returned false for wrapped bulk path error")
	}
}
