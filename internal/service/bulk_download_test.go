package service

// This file exercises bulk download behavior so refactors preserve the documented contract.

import "testing"

func TestSafeBulkDownloadZipPathNormalizesSafeEntries(t *testing.T) {
	zipPath, err := safeBulkDownloadZipPath("edge/core", `configs\running.rsc`)
	if err != nil {
		t.Fatalf("safeBulkDownloadZipPath returned error: %v", err)
	}
	if zipPath != "edge_core/configs/running.rsc" {
		t.Fatalf("zipPath = %q, want %q", zipPath, "edge_core/configs/running.rsc")
	}
}

func TestSafeBulkDownloadZipPathRejectsUnsafeEntries(t *testing.T) {
	tests := []string{
		"",
		"../escape.rsc",
		"/absolute.rsc",
		`C:\absolute.rsc`,
	}

	for _, fileName := range tests {
		t.Run(fileName, func(t *testing.T) {
			if _, err := safeBulkDownloadZipPath("device", fileName); err == nil {
				t.Fatal("safeBulkDownloadZipPath error = nil, want bulk path error")
			} else if !IsBulkPathError(err) {
				t.Fatalf("safeBulkDownloadZipPath error = %v, want bulk path error", err)
			}
		})
	}
}

func TestSaturatedInt64SumCapsOverflow(t *testing.T) {
	if got := saturatedInt64Sum(10, 5); got != 15 {
		t.Fatalf("saturatedInt64Sum(10, 5) = %d, want 15", got)
	}
	if got := saturatedInt64Sum(1<<63-2, 10); got != 1<<63-1 {
		t.Fatalf("saturatedInt64Sum overflow = %d, want MaxInt64", got)
	}
}
