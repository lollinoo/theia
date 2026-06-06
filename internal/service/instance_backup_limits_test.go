package service

import (
	"errors"
	"testing"
	"time"
)

func TestNormalizeRestoreArchiveLimitsFillsDefaultsAndPreservesOverrides(t *testing.T) {
	limits := normalizeRestoreArchiveLimits(RestoreArchiveLimits{
		MaxCompressedBytes: 12,
		MaxFileEntries:     34,
	})

	if limits.MaxCompressedBytes != 12 {
		t.Fatalf("MaxCompressedBytes = %d, want 12", limits.MaxCompressedBytes)
	}
	if limits.MaxFileEntries != 34 {
		t.Fatalf("MaxFileEntries = %d, want 34", limits.MaxFileEntries)
	}
	if limits.MaxTotalBytes != DefaultRestoreArchiveLimits.MaxTotalBytes {
		t.Fatalf("MaxTotalBytes = %d, want default %d", limits.MaxTotalBytes, DefaultRestoreArchiveLimits.MaxTotalBytes)
	}
	if limits.MaxEntryBytes != DefaultRestoreArchiveLimits.MaxEntryBytes {
		t.Fatalf("MaxEntryBytes = %d, want default %d", limits.MaxEntryBytes, DefaultRestoreArchiveLimits.MaxEntryBytes)
	}
}

func TestNormalizeBackupArchiveLimitsFillsDefaultsAndPreservesOverrides(t *testing.T) {
	limits := normalizeBackupArchiveLimits(BackupArchiveLimits{
		MaxTotalBytes:  12,
		MaxFileEntries: 34,
		MaxDuration:    time.Second,
	})

	if limits.MaxTotalBytes != 12 {
		t.Fatalf("MaxTotalBytes = %d, want 12", limits.MaxTotalBytes)
	}
	if limits.MaxFileEntries != 34 {
		t.Fatalf("MaxFileEntries = %d, want 34", limits.MaxFileEntries)
	}
	if limits.MaxDuration != time.Second {
		t.Fatalf("MaxDuration = %v, want 1s", limits.MaxDuration)
	}
	if limits.MaxEntryBytes != DefaultBackupArchiveLimits.MaxEntryBytes {
		t.Fatalf("MaxEntryBytes = %d, want default %d", limits.MaxEntryBytes, DefaultBackupArchiveLimits.MaxEntryBytes)
	}
}

func TestNewRestoreLimitErrorCanBeMatchedByType(t *testing.T) {
	err := newRestoreLimitError("restore limit %d", 7)

	var limitErr *RestoreLimitError
	if !errors.As(err, &limitErr) {
		t.Fatalf("error = %T %v, want RestoreLimitError", err, err)
	}
	if err.Error() != "restore limit 7" {
		t.Fatalf("error message = %q, want restore limit 7", err.Error())
	}
}
