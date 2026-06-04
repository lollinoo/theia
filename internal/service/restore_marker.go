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

func restoreMarkerFilePath(stateDir string) string {
	return filepath.Join(stateDir, restoreMarkerFileName)
}

func readRestoreMarker(path string) (*restoreMarker, bool, error) {
	markerData, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("read restore marker: %w", err)
	}

	var marker restoreMarker
	if err := json.Unmarshal(markerData, &marker); err != nil {
		return nil, false, fmt.Errorf("parse restore marker: %w", err)
	}
	if marker.StagedDB == "" {
		return nil, true, fmt.Errorf("restore marker missing staged_db")
	}
	return &marker, true, nil
}

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

func removeRestoreMarker(path string) error {
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove restore marker: %w", err)
	}
	return nil
}

func validateRestoreMarkerRuntimeTargets(marker restoreMarker, stateDir string, deviceBackupDir string, knownHostsPath string) error {
	if filepath.Clean(marker.StateDir) != filepath.Clean(stateDir) ||
		filepath.Clean(marker.DeviceBackupDir) != filepath.Clean(deviceBackupDir) ||
		filepath.Clean(marker.KnownHostsPath) != filepath.Clean(knownHostsPath) {
		return errRestoreMarkerTargetMismatch
	}
	return nil
}
