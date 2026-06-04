package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

const restoreMarkerFileName = ".theia-restore-pending"

var errRestoreMarkerTargetMismatch = errors.New("restore marker targets do not match runtime paths")

var errRestoreMarkerMissingStagedDB = errors.New("restore marker missing staged_db")

// restoreMarkerFilePath returns the pending-restore marker location for a state directory.
func restoreMarkerFilePath(stateDir string) string {
	return filepath.Join(stateDir, restoreMarkerFileName)
}

// readRestoreMarker loads a pending restore marker while treating absence as a no-op.
func readRestoreMarker(path string) (*restoreMarker, bool, error) {
	markerData, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("read restore marker: %w", err)
	}

	marker, err := parseRestoreMarker(markerData)
	if err != nil {
		if errors.Is(err, errRestoreMarkerMissingStagedDB) {
			return nil, true, err
		}
		return nil, false, err
	}
	return marker, true, nil
}

// parseRestoreMarker decodes and validates the required marker fields before restore activation.
func parseRestoreMarker(markerData []byte) (*restoreMarker, error) {
	var marker restoreMarker
	if err := json.Unmarshal(markerData, &marker); err != nil {
		return nil, fmt.Errorf("parse restore marker: %w", err)
	}
	if marker.StagedDB == "" {
		return nil, errRestoreMarkerMissingStagedDB
	}
	return &marker, nil
}

// writeRestoreMarker persists a pending restore marker with owner-only permissions.
func writeRestoreMarker(path string, marker restoreMarker) error {
	markerJSON, err := json.MarshalIndent(marker, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling marker JSON: %w", err)
	}
	if err := os.WriteFile(path, markerJSON, 0600); err != nil {
		return fmt.Errorf("writing restore marker: %w", err)
	}
	if err := os.Chmod(path, 0600); err != nil {
		return fmt.Errorf("restricting restore marker permissions: %w", err)
	}
	return nil
}

// removeRestoreMarker clears a pending restore marker while preserving missing-marker no-op behavior.
func removeRestoreMarker(path string) error {
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove restore marker: %w", err)
	}
	return nil
}

// validatePendingRestoreMarker checks runtime ownership and staged artifact layout before restore side effects.
func validatePendingRestoreMarker(marker restoreMarker, stateDir string, deviceBackupDir string, knownHostsPath string, stagingDir string) error {
	if err := validateRestoreMarkerRuntimeTargets(marker, stateDir, deviceBackupDir, knownHostsPath); err != nil {
		return err
	}
	return validateRestoreStagingLayout(marker, stagingDir)
}

// validateRestoreMarkerRuntimeTargets ensures the marker belongs to the current runtime paths.
func validateRestoreMarkerRuntimeTargets(marker restoreMarker, stateDir string, deviceBackupDir string, knownHostsPath string) error {
	if filepath.Clean(marker.StateDir) != filepath.Clean(stateDir) ||
		filepath.Clean(marker.DeviceBackupDir) != filepath.Clean(deviceBackupDir) ||
		filepath.Clean(marker.KnownHostsPath) != filepath.Clean(knownHostsPath) {
		return errRestoreMarkerTargetMismatch
	}
	return nil
}
