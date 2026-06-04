package service

import (
	"fmt"
	"os"
)

func activateOptionalRestoreArtifacts(marker restoreMarker, stagingDir string) error {
	if marker.StagedBackups == "" && marker.StagedKnownHosts == "" {
		return nil
	}
	if err := validateRestoreStagingDir(stagingDir); err != nil {
		return fmt.Errorf("validate restore staging dir: %w", err)
	}
	if err := activateOptionalStagedBackupDir(marker); err != nil {
		return err
	}
	return activateOptionalStagedKnownHosts(marker)
}

func activateOptionalStagedBackupDir(marker restoreMarker) error {
	if marker.StagedBackups == "" || marker.DeviceBackupDir == "" {
		return nil
	}
	if _, err := os.Lstat(marker.StagedBackups); err == nil {
		if err := validateOptionalStagedBackupDir(marker.StagedBackups); err != nil {
			return fmt.Errorf("validate staged backup dir: %w", err)
		}
		if err := replaceDirForRestore(marker.StagedBackups, marker.DeviceBackupDir); err != nil {
			return fmt.Errorf("activate staged backup dir: %w", err)
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("stat staged backup dir: %w", err)
	}
	return nil
}

func activateOptionalStagedKnownHosts(marker restoreMarker) error {
	if marker.StagedKnownHosts == "" || marker.KnownHostsPath == "" {
		return nil
	}
	if _, err := os.Lstat(marker.StagedKnownHosts); err == nil {
		if err := validateOptionalStagedKnownHosts(marker.StagedKnownHosts); err != nil {
			return fmt.Errorf("validate staged known_hosts: %w", err)
		}
		if err := replaceFileForRestore(marker.StagedKnownHosts, marker.KnownHostsPath); err != nil {
			return fmt.Errorf("activate staged known_hosts: %w", err)
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("stat staged known_hosts: %w", err)
	}
	return nil
}
