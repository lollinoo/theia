package service

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

type restoreMarker struct {
	StagedDB         string `json:"staged_db"`
	StagedBackups    string `json:"staged_backups"`
	StagedKnownHosts string `json:"staged_known_hosts"`
	DBPath           string `json:"db_path"`
	DeviceBackupDir  string `json:"device_backup_dir"`
	KnownHostsPath   string `json:"known_hosts_path"`
	Timestamp        string `json:"timestamp"`
}

func newRestoreMarker(
	stagedDB string,
	stagedBackups string,
	stagedKnownHosts string,
	dbPath string,
	deviceBackupDir string,
	knownHostsPath string,
	timestamp string,
) restoreMarker {
	return restoreMarker{
		StagedDB:         stagedDB,
		StagedBackups:    stagedBackups,
		StagedKnownHosts: stagedKnownHosts,
		DBPath:           dbPath,
		DeviceBackupDir:  deviceBackupDir,
		KnownHostsPath:   knownHostsPath,
		Timestamp:        timestamp,
	}
}

type RestoreCoordinator struct {
	dbPath          string
	deviceBackupDir string
	knownHostsPath  string
}

func NewRestoreCoordinator(dbPath, deviceBackupDir, knownHostsPath string) *RestoreCoordinator {
	return &RestoreCoordinator{
		dbPath:          dbPath,
		deviceBackupDir: deviceBackupDir,
		knownHostsPath:  knownHostsPath,
	}
}

func (c *RestoreCoordinator) ApplyPendingRestore() (bool, error) {
	markerPath := filepath.Join(filepath.Dir(c.dbPath), ".theia-restore-pending")
	markerData, err := os.ReadFile(markerPath)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("read restore marker: %w", err)
	}

	var marker restoreMarker
	if err := json.Unmarshal(markerData, &marker); err != nil {
		return false, fmt.Errorf("parse restore marker: %w", err)
	}
	if marker.StagedDB == "" {
		return false, fmt.Errorf("restore marker missing staged_db")
	}

	if filepath.Clean(marker.DBPath) != filepath.Clean(c.dbPath) ||
		filepath.Clean(marker.DeviceBackupDir) != filepath.Clean(c.deviceBackupDir) ||
		filepath.Clean(marker.KnownHostsPath) != filepath.Clean(c.knownHostsPath) {
		return false, fmt.Errorf("restore marker targets do not match runtime paths")
	}

	if _, err := os.Stat(marker.StagedDB); err != nil {
		return false, fmt.Errorf("stat staged db: %w", err)
	}

	if err := c.backupLiveDB(); err != nil {
		return false, err
	}

	if _, err := os.Stat(c.dbPath); err != nil && !os.IsNotExist(err) {
		return false, fmt.Errorf("stat live db: %w", err)
	}

	if err := os.Remove(c.dbPath); err != nil && !os.IsNotExist(err) {
		return false, fmt.Errorf("remove live db: %w", err)
	}
	if err := os.Remove(c.dbPath + "-wal"); err != nil && !os.IsNotExist(err) {
		return false, fmt.Errorf("remove live db wal: %w", err)
	}
	if err := os.Remove(c.dbPath + "-shm"); err != nil && !os.IsNotExist(err) {
		return false, fmt.Errorf("remove live db shm: %w", err)
	}
	if err := os.Rename(marker.StagedDB, c.dbPath); err != nil {
		return false, fmt.Errorf("activate staged db: %w", err)
	}
	if err := os.Chmod(c.dbPath, 0600); err != nil {
		return false, fmt.Errorf("chmod restored db: %w", err)
	}

	if marker.StagedBackups != "" && marker.DeviceBackupDir != "" {
		if info, err := os.Stat(marker.StagedBackups); err == nil {
			if info.IsDir() {
				if err := replaceDirForRestore(marker.StagedBackups, marker.DeviceBackupDir); err != nil {
					return false, c.restoreRetryableError(marker.StagedDB, fmt.Errorf("activate staged backup dir: %w", err))
				}
			}
		} else if !os.IsNotExist(err) {
			return false, c.restoreRetryableError(marker.StagedDB, fmt.Errorf("stat staged backup dir: %w", err))
		}
	}

	if marker.StagedKnownHosts != "" && marker.KnownHostsPath != "" {
		if _, err := os.Stat(marker.StagedKnownHosts); err == nil {
			if err := replaceFileForRestore(marker.StagedKnownHosts, marker.KnownHostsPath); err != nil {
				return false, c.restoreRetryableError(marker.StagedDB, fmt.Errorf("activate staged known_hosts: %w", err))
			}
		} else if !os.IsNotExist(err) {
			return false, c.restoreRetryableError(marker.StagedDB, fmt.Errorf("stat staged known_hosts: %w", err))
		}
	}

	if err := os.Remove(markerPath); err != nil && !os.IsNotExist(err) {
		return false, fmt.Errorf("remove restore marker: %w", err)
	}

	stagingDir := filepath.Dir(marker.StagedDB)
	if stagingDir != "" && stagingDir != "." {
		if err := os.RemoveAll(stagingDir); err != nil {
			return false, fmt.Errorf("remove staging dir: %w", err)
		}
	}

	return true, nil
}

func (c *RestoreCoordinator) backupLiveDB() error {
	if _, err := os.Stat(c.dbPath); err == nil {
		bakPath := c.dbPath + ".pre-restore.bak"
		if _, err := os.Stat(bakPath); err == nil {
			return nil
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("stat existing db backup: %w", err)
		}

		if err := copyFileForRestore(c.dbPath, bakPath); err != nil {
			return fmt.Errorf("backup current db: %w", err)
		}
		return nil
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("stat live db: %w", err)
	}

	return nil
}

func (c *RestoreCoordinator) restoreRetryableError(stagedDB string, activationErr error) error {
	if err := copyFileForRestore(c.dbPath, stagedDB); err != nil {
		return fmt.Errorf("%w (restore staged db for retry: %v)", activationErr, err)
	}

	return activationErr
}

func copyFileForRestore(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	if err := os.MkdirAll(filepath.Dir(dst), 0700); err != nil {
		return err
	}

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}

	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}

	if err := out.Close(); err != nil {
		return err
	}

	if err := os.Chmod(dst, 0600); err != nil {
		return err
	}

	return nil
}

func copyDirForRestore(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}

		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, 0700)
		}

		return copyFileForRestore(path, target)
	})
}

func replaceFileForRestore(src, dst string) error {
	tmpPath := dst + ".restore-tmp"
	backupPath := dst + ".restore-old"

	if err := os.Remove(tmpPath); err != nil && !os.IsNotExist(err) {
		return err
	}
	if err := os.Remove(backupPath); err != nil && !os.IsNotExist(err) {
		return err
	}
	if err := copyFileForRestore(src, tmpPath); err != nil {
		return err
	}

	movedExisting := false
	if _, err := os.Stat(dst); err == nil {
		if err := os.Rename(dst, backupPath); err != nil {
			_ = os.Remove(tmpPath)
			return err
		}
		movedExisting = true
	} else if !os.IsNotExist(err) {
		_ = os.Remove(tmpPath)
		return err
	}

	if err := os.Rename(tmpPath, dst); err != nil {
		if movedExisting {
			if restoreErr := os.Rename(backupPath, dst); restoreErr != nil {
				return fmt.Errorf("activate staged restore file: %w (restore previous file: %v)", err, restoreErr)
			}
		}
		_ = os.Remove(tmpPath)
		return err
	}

	if movedExisting {
		_ = os.Remove(backupPath)
	}

	return nil
}

func replaceDirForRestore(src, dst string) error {
	tmpPath := dst + ".restore-tmp"
	backupPath := dst + ".restore-old"

	if err := os.RemoveAll(tmpPath); err != nil {
		return err
	}
	if err := os.RemoveAll(backupPath); err != nil {
		return err
	}
	if err := copyDirForRestore(src, tmpPath); err != nil {
		return err
	}

	movedExisting := false
	if _, err := os.Stat(dst); err == nil {
		if err := os.Rename(dst, backupPath); err != nil {
			_ = os.RemoveAll(tmpPath)
			return err
		}
		movedExisting = true
	} else if !os.IsNotExist(err) {
		_ = os.RemoveAll(tmpPath)
		return err
	}

	if err := os.Rename(tmpPath, dst); err != nil {
		if movedExisting {
			if restoreErr := os.Rename(backupPath, dst); restoreErr != nil {
				return fmt.Errorf("activate staged restore dir: %w (restore previous dir: %v)", err, restoreErr)
			}
		}
		_ = os.RemoveAll(tmpPath)
		return err
	}

	if movedExisting {
		_ = os.RemoveAll(backupPath)
	}

	return nil
}
