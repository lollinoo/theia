package service

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestRestoreMarkerReadWriteRemoveRoundTrip characterizes restore marker persistence and cleanup.
func TestRestoreMarkerReadWriteRemoveRoundTrip(t *testing.T) {
	stateDir := t.TempDir()
	markerPath := restoreMarkerFilePath(stateDir)
	marker := newRestoreMarker(
		filepath.Join(stateDir, ".restore-staging", postgresArchiveDBEntry),
		filepath.Join(stateDir, ".restore-staging", "backups"),
		filepath.Join(stateDir, ".restore-staging", "known_hosts"),
		stateDir,
		filepath.Join(stateDir, "device-backups"),
		filepath.Join(stateDir, "known_hosts"),
		"2026-04-23T00:00:00Z",
	)

	if err := writeRestoreMarker(markerPath, marker); err != nil {
		t.Fatalf("writeRestoreMarker() error = %v", err)
	}

	info, err := os.Stat(markerPath)
	if err != nil {
		t.Fatalf("stat marker: %v", err)
	}
	if mode := info.Mode().Perm(); mode != 0o600 {
		t.Fatalf("marker mode = %v, want 0600", mode)
	}
	assertRestoreMarkerOperationJSON(t, markerPath, "validation_passed")

	got, exists, err := readRestoreMarker(markerPath)
	if err != nil {
		t.Fatalf("readRestoreMarker() error = %v", err)
	}
	if !exists {
		t.Fatal("readRestoreMarker() exists = false, want true")
	}
	if got.StagedDB != marker.StagedDB ||
		got.StagedBackups != marker.StagedBackups ||
		got.StagedKnownHosts != marker.StagedKnownHosts ||
		got.StateDir != marker.StateDir ||
		got.DeviceBackupDir != marker.DeviceBackupDir ||
		got.KnownHostsPath != marker.KnownHostsPath ||
		got.Timestamp != marker.Timestamp {
		t.Fatalf("readRestoreMarker() = %#v, want restore paths from %#v", *got, marker)
	}

	if err := removeRestoreMarker(markerPath); err != nil {
		t.Fatalf("removeRestoreMarker() error = %v", err)
	}
	if _, err := os.Stat(markerPath); !os.IsNotExist(err) {
		t.Fatalf("marker should be removed, stat err = %v", err)
	}
}

func TestWriteRestoreMarkerDoesNotLeavePartialJSONOrTempFiles(t *testing.T) {
	stateDir := t.TempDir()
	markerPath := restoreMarkerFilePath(stateDir)
	marker := newRestoreMarker(
		filepath.Join(stateDir, ".restore-staging", postgresArchiveDBEntry),
		"",
		"",
		stateDir,
		filepath.Join(stateDir, "device-backups"),
		filepath.Join(stateDir, "known_hosts"),
		"2026-04-23T00:00:00Z",
	)

	if err := writeRestoreMarker(markerPath, marker); err != nil {
		t.Fatalf("writeRestoreMarker() error = %v", err)
	}

	data, err := os.ReadFile(markerPath)
	if err != nil {
		t.Fatalf("reading marker: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("marker contains partial or malformed JSON: %v", err)
	}
	if decoded["staged_db"] == "" {
		t.Fatal("marker missing staged_db after atomic publication")
	}

	entries, err := os.ReadDir(stateDir)
	if err != nil {
		t.Fatalf("reading state dir: %v", err)
	}
	for _, entry := range entries {
		if strings.Contains(entry.Name(), ".tmp") || strings.Contains(entry.Name(), "restore-pending-") {
			t.Fatalf("atomic marker write left temp file %q", entry.Name())
		}
	}
}

func assertRestoreMarkerOperationJSON(t *testing.T, markerPath string, wantPhase string) {
	t.Helper()
	data, err := os.ReadFile(markerPath)
	if err != nil {
		t.Fatalf("reading marker json: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal marker json: %v", err)
	}
	if got, ok := decoded["operation_id"].(string); !ok || got == "" {
		t.Fatalf("marker operation_id = %#v, want non-empty string", decoded["operation_id"])
	}
	if got, ok := decoded["phase"].(string); !ok || got != wantPhase {
		t.Fatalf("marker phase = %#v, want %q", decoded["phase"], wantPhase)
	}
	if got, ok := decoded["attempt_count"].(float64); !ok || got != 0 {
		t.Fatalf("marker attempt_count = %#v, want 0", decoded["attempt_count"])
	}
	if got, ok := decoded["created_at"].(string); !ok || got == "" {
		t.Fatalf("marker created_at = %#v, want non-empty string", decoded["created_at"])
	}
	if got, ok := decoded["updated_at"].(string); !ok || got == "" {
		t.Fatalf("marker updated_at = %#v, want non-empty string", decoded["updated_at"])
	}
}

// TestReadRestoreMarkerMissingIsNoOp preserves missing-marker startup as a safe no-op.
func TestReadRestoreMarkerMissingIsNoOp(t *testing.T) {
	got, exists, err := readRestoreMarker(restoreMarkerFilePath(t.TempDir()))
	if err != nil {
		t.Fatalf("readRestoreMarker() error = %v", err)
	}
	if exists {
		t.Fatal("readRestoreMarker() exists = true, want false")
	}
	if got != nil {
		t.Fatalf("readRestoreMarker() marker = %#v, want nil", got)
	}
}

// TestParseRestoreMarkerRejectsMalformedAndIncompleteMarkers preserves marker parse fail-closed errors.
func TestParseRestoreMarkerRejectsMalformedAndIncompleteMarkers(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		wantErr string
	}{
		{
			name:    "malformed json",
			body:    "{",
			wantErr: "parse restore marker:",
		},
		{
			name:    "missing staged db",
			body:    `{"state_dir":"/tmp/theia"}`,
			wantErr: "restore marker missing staged_db",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseRestoreMarker([]byte(tt.body))
			if err == nil {
				t.Fatalf("parseRestoreMarker() error = nil, want %q", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("parseRestoreMarker() error = %q, want %q", err.Error(), tt.wantErr)
			}
		})
	}
}

// TestValidateRestoreMarkerRuntimeTargetsFailsClosed keeps runtime target mismatches fail-closed.
func TestValidateRestoreMarkerRuntimeTargetsFailsClosed(t *testing.T) {
	stateDir := t.TempDir()
	marker := newRestoreMarker(
		filepath.Join(stateDir, ".restore-staging", postgresArchiveDBEntry),
		"",
		"",
		stateDir,
		filepath.Join(stateDir, "device-backups"),
		filepath.Join(stateDir, "known_hosts"),
		"2026-04-23T00:00:00Z",
	)

	if err := validateRestoreMarkerRuntimeTargets(
		marker,
		stateDir+string(os.PathSeparator),
		filepath.Join(stateDir, "device-backups"),
		filepath.Join(stateDir, "known_hosts"),
	); err != nil {
		t.Fatalf("validateRestoreMarkerRuntimeTargets() equivalent paths error = %v", err)
	}

	err := validateRestoreMarkerRuntimeTargets(
		marker,
		stateDir,
		filepath.Join(stateDir, "other-device-backups"),
		filepath.Join(stateDir, "known_hosts"),
	)
	if err == nil {
		t.Fatal("validateRestoreMarkerRuntimeTargets() error = nil, want target mismatch")
	}
	if !errors.Is(err, errRestoreMarkerTargetMismatch) {
		t.Fatalf("validateRestoreMarkerRuntimeTargets() error = %v, want errRestoreMarkerTargetMismatch", err)
	}
	if got := err.Error(); got != "restore marker targets do not match runtime paths" {
		t.Fatalf("validateRestoreMarkerRuntimeTargets() error = %q, want stable API error", got)
	}
}

// TestValidatePendingRestoreMarkerChecksRuntimeTargetsBeforeStagingLayout locks the safety preflight order.
func TestValidatePendingRestoreMarkerChecksRuntimeTargetsBeforeStagingLayout(t *testing.T) {
	stateDir := t.TempDir()
	stagingDir := filepath.Join(stateDir, ".restore-staging")
	marker := newRestoreMarker(
		filepath.Join(stagingDir, postgresArchiveDBEntry),
		"",
		"",
		stateDir,
		filepath.Join(stateDir, "device-backups"),
		filepath.Join(stateDir, "known_hosts"),
		"2026-04-23T00:00:00Z",
	)

	err := validatePendingRestoreMarker(
		marker,
		stateDir,
		filepath.Join(stateDir, "other-device-backups"),
		filepath.Join(stateDir, "known_hosts"),
		stagingDir,
	)
	if err == nil {
		t.Fatal("validatePendingRestoreMarker() error = nil, want target mismatch")
	}
	if !errors.Is(err, errRestoreMarkerTargetMismatch) {
		t.Fatalf("validatePendingRestoreMarker() error = %v, want errRestoreMarkerTargetMismatch", err)
	}
}
