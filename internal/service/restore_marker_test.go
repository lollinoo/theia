package service

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

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

	got, exists, err := readRestoreMarker(markerPath)
	if err != nil {
		t.Fatalf("readRestoreMarker() error = %v", err)
	}
	if !exists {
		t.Fatal("readRestoreMarker() exists = false, want true")
	}
	if *got != marker {
		t.Fatalf("readRestoreMarker() = %#v, want %#v", *got, marker)
	}

	if err := removeRestoreMarker(markerPath); err != nil {
		t.Fatalf("removeRestoreMarker() error = %v", err)
	}
	if _, err := os.Stat(markerPath); !os.IsNotExist(err) {
		t.Fatalf("marker should be removed, stat err = %v", err)
	}
}

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
