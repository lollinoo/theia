package service

import (
	"fmt"
	"os"
	"path/filepath"
)

func validateRestoreStagingLayout(marker restoreMarker, stagingDir string) error {
	if err := validateRestoreStagingDir(stagingDir); err != nil {
		return err
	}

	expectedStagedDB := filepath.Join(stagingDir, "database.dump")
	if marker.StagedDB != expectedStagedDB {
		return fmt.Errorf("restore marker staged db path does not match runtime staging path")
	}
	if err := validateStagedDBFile(marker.StagedDB); err != nil {
		return err
	}

	expectedStagedBackups := filepath.Join(stagingDir, "backups")
	if marker.StagedBackups != "" {
		if marker.StagedBackups != expectedStagedBackups {
			return fmt.Errorf("restore marker staged backups path does not match runtime staging path")
		}
		if err := validateOptionalStagedBackupDir(marker.StagedBackups); err != nil {
			return err
		}
	}

	expectedStagedKnownHosts := filepath.Join(stagingDir, "known_hosts")
	if marker.StagedKnownHosts != "" {
		if marker.StagedKnownHosts != expectedStagedKnownHosts {
			return fmt.Errorf("restore marker staged known_hosts path does not match runtime staging path")
		}
		if err := validateOptionalStagedKnownHosts(marker.StagedKnownHosts); err != nil {
			return err
		}
	}

	return nil
}

func validateStagedDBFile(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("stat staged db: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return fmt.Errorf("staged db must be a regular file")
	}
	return nil
}

func validateRestoreStagingDir(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("stat restore staging dir: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("restore staging dir must not be a symlink")
	}
	if !info.IsDir() {
		return fmt.Errorf("restore staging dir must be a directory")
	}
	return nil
}

func validateOptionalStagedBackupDir(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("stat staged backup dir: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("staged backup dir must not be a symlink")
	}
	if !info.IsDir() {
		return fmt.Errorf("staged backup dir must be a directory")
	}

	return filepath.WalkDir(path, func(entryPath string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 || (!info.IsDir() && !info.Mode().IsRegular()) {
			return fmt.Errorf("staged backup entry must be a regular file or directory: %s", entryPath)
		}
		return nil
	})
}

func validateOptionalStagedKnownHosts(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("stat staged known_hosts: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return fmt.Errorf("staged known_hosts must be a regular file")
	}
	return nil
}

func validateRetryStagedDBDestination(stagedDB string, stagingDir string) error {
	if err := validateRestoreStagingDir(stagingDir); err != nil {
		return err
	}

	expectedStagedDB := filepath.Join(stagingDir, "database.dump")
	if stagedDB != expectedStagedDB {
		return fmt.Errorf("restore marker staged db path does not match runtime staging path")
	}

	info, err := os.Lstat(stagedDB)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("stat staged db retry destination: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return fmt.Errorf("staged db retry destination must be a regular file")
	}

	return nil
}
