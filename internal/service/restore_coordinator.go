package service

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

var terminatePostgresConnections = func(ctx context.Context, dsn string) error {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return fmt.Errorf("open postgres restore connection: %w", err)
	}
	defer db.Close()

	if _, err := db.ExecContext(ctx, `
		SELECT pg_terminate_backend(pid)
		FROM pg_stat_activity
		WHERE datname = current_database()
		  AND pid <> pg_backend_pid()
	`); err != nil {
		return fmt.Errorf("terminate postgres sessions: %w", err)
	}
	return nil
}

var restoreCoordinatorAfterDBActivationHook func() error

type restoreMarker struct {
	StagedDB         string `json:"staged_db"`
	StagedBackups    string `json:"staged_backups"`
	StagedKnownHosts string `json:"staged_known_hosts"`
	StateDir         string `json:"state_dir"`
	DeviceBackupDir  string `json:"device_backup_dir"`
	KnownHostsPath   string `json:"known_hosts_path"`
	Timestamp        string `json:"timestamp"`
}

func newRestoreMarker(
	stagedDB string,
	stagedBackups string,
	stagedKnownHosts string,
	stateDir string,
	deviceBackupDir string,
	knownHostsPath string,
	timestamp string,
) restoreMarker {
	return restoreMarker{
		StagedDB:         stagedDB,
		StagedBackups:    stagedBackups,
		StagedKnownHosts: stagedKnownHosts,
		StateDir:         stateDir,
		DeviceBackupDir:  deviceBackupDir,
		KnownHostsPath:   knownHostsPath,
		Timestamp:        timestamp,
	}
}

type RestoreCoordinator struct {
	stateDir        string
	dbDSN           string
	deviceBackupDir string
	knownHostsPath  string
}

func NewRestoreCoordinatorWithDSN(stateDir, dbDSN, deviceBackupDir, knownHostsPath string) *RestoreCoordinator {
	return &RestoreCoordinator{
		stateDir:        stateDir,
		dbDSN:           strings.TrimSpace(dbDSN),
		deviceBackupDir: deviceBackupDir,
		knownHostsPath:  knownHostsPath,
	}
}

// ApplyPendingRestore applies a staged restore after process restart.
// Recovery semantics are marker-driven: no marker is a no-op; preflight failures
// leave live targets unchanged and keep the marker and staging dir; post-DB
// activation optional artifact failures keep the marker/staging and refresh the
// staged DB for retry when the retry destination remains valid and safe; success
// removes both the marker and staging dir.
func (c *RestoreCoordinator) ApplyPendingRestore() (bool, error) {
	ctx := context.Background()
	markerPath := restoreMarkerFilePath(c.stateDir)
	marker, exists, err := readRestoreMarker(markerPath)
	if err != nil {
		return false, err
	}
	if !exists {
		return false, nil
	}

	if err := validateRestoreMarkerRuntimeTargets(*marker, c.stateDir, c.deviceBackupDir, c.knownHostsPath); err != nil {
		return false, err
	}

	stagingDir := filepath.Join(c.stateDir, ".restore-staging")
	if err := validateRestoreStagingLayout(*marker, stagingDir); err != nil {
		return false, err
	}

	if err := ensureSupportedPostgresCLITools(ctx, "pg_dump", "pg_restore", "psql"); err != nil {
		return false, err
	}

	if err := c.backupLiveDB(); err != nil {
		return false, err
	}

	if err := c.applyPostgresRestore(ctx, marker.StagedDB); err != nil {
		return false, err
	}

	if restoreCoordinatorAfterDBActivationHook != nil {
		if err := restoreCoordinatorAfterDBActivationHook(); err != nil {
			return false, c.restoreRetryableError(marker.StagedDB, stagingDir, fmt.Errorf("after db activation hook: %w", err))
		}
	}

	if marker.StagedBackups != "" || marker.StagedKnownHosts != "" {
		if err := validateRestoreStagingDir(stagingDir); err != nil {
			return false, c.restoreRetryableError(marker.StagedDB, stagingDir, fmt.Errorf("validate restore staging dir: %w", err))
		}
	}

	if marker.StagedBackups != "" && marker.DeviceBackupDir != "" {
		if _, err := os.Lstat(marker.StagedBackups); err == nil {
			if err := validateOptionalStagedBackupDir(marker.StagedBackups); err != nil {
				return false, c.restoreRetryableError(marker.StagedDB, stagingDir, fmt.Errorf("validate staged backup dir: %w", err))
			}
			if err := replaceDirForRestore(marker.StagedBackups, marker.DeviceBackupDir); err != nil {
				return false, c.restoreRetryableError(marker.StagedDB, stagingDir, fmt.Errorf("activate staged backup dir: %w", err))
			}
		} else if !os.IsNotExist(err) {
			return false, c.restoreRetryableError(marker.StagedDB, stagingDir, fmt.Errorf("stat staged backup dir: %w", err))
		}
	}

	if marker.StagedKnownHosts != "" && marker.KnownHostsPath != "" {
		if _, err := os.Lstat(marker.StagedKnownHosts); err == nil {
			if err := validateOptionalStagedKnownHosts(marker.StagedKnownHosts); err != nil {
				return false, c.restoreRetryableError(marker.StagedDB, stagingDir, fmt.Errorf("validate staged known_hosts: %w", err))
			}
			if err := replaceFileForRestore(marker.StagedKnownHosts, marker.KnownHostsPath); err != nil {
				return false, c.restoreRetryableError(marker.StagedDB, stagingDir, fmt.Errorf("activate staged known_hosts: %w", err))
			}
		} else if !os.IsNotExist(err) {
			return false, c.restoreRetryableError(marker.StagedDB, stagingDir, fmt.Errorf("stat staged known_hosts: %w", err))
		}
	}

	if err := removeRestoreMarker(markerPath); err != nil {
		return false, err
	}

	if stagingDir != "" && stagingDir != "." {
		if err := os.RemoveAll(stagingDir); err != nil {
			return false, fmt.Errorf("remove staging dir: %w", err)
		}
	}

	return true, nil
}

func (c *RestoreCoordinator) applyPostgresRestore(ctx context.Context, stagedDB string) error {
	if strings.TrimSpace(c.dbDSN) == "" {
		return fmt.Errorf("postgres restore requires db_dsn")
	}
	if err := ensureSupportedPostgresCLITools(ctx, "pg_restore", "psql"); err != nil {
		return err
	}
	conn, err := postgresCLIConnInfo(c.dbDSN)
	if err != nil {
		return fmt.Errorf("build postgres conninfo: %w", err)
	}
	if err := terminatePostgresConnections(ctx, c.dbDSN); err != nil {
		return err
	}
	if err := validateStagedDBFile(stagedDB); err != nil {
		return err
	}
	if _, err := runExternalCommandWithEnv(
		ctx,
		conn.env,
		"psql",
		"--set", "ON_ERROR_STOP=1",
		"--single-transaction",
		"--dbname", conn.connInfo,
		"--command", "DROP SCHEMA IF EXISTS public CASCADE; CREATE SCHEMA public;",
	); err != nil {
		return fmt.Errorf("clean postgres schema before restore: %w", err)
	}
	if _, err := runExternalCommandWithEnv(
		ctx,
		conn.env,
		"pg_restore",
		"--no-owner",
		"--no-privileges",
		"--single-transaction",
		"--exit-on-error",
		"--dbname", conn.connInfo,
		stagedDB,
	); err != nil {
		return fmt.Errorf("restore postgres database: %w", err)
	}
	return nil
}

func (c *RestoreCoordinator) backupLiveDB() error {
	bakPath := filepath.Join(c.stateDir, "postgres.pre-restore.dump")
	if _, err := os.Stat(bakPath); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("stat existing db backup: %w", err)
	}
	if err := c.dumpLivePostgresDatabase(context.Background(), bakPath); err != nil {
		return fmt.Errorf("backup current db: %w", err)
	}
	return nil
}

func (c *RestoreCoordinator) dumpLivePostgresDatabase(ctx context.Context, destPath string) error {
	if strings.TrimSpace(c.dbDSN) == "" {
		return fmt.Errorf("postgres backup requires db_dsn")
	}
	if err := ensureSupportedPostgresCLITools(ctx, "pg_dump"); err != nil {
		return err
	}
	conn, err := postgresCLIConnInfo(c.dbDSN)
	if err != nil {
		return fmt.Errorf("build postgres conninfo: %w", err)
	}
	if _, err := runExternalCommandWithEnv(
		ctx,
		conn.env,
		"pg_dump",
		"--format=custom",
		"--no-owner",
		"--no-privileges",
		"--file", destPath,
		"--dbname", conn.connInfo,
	); err != nil {
		return fmt.Errorf("pg_dump failed: %w", err)
	}
	return nil
}

func (c *RestoreCoordinator) restoreRetryableError(stagedDB string, stagingDir string, activationErr error) error {
	if err := validateRetryStagedDBDestination(stagedDB, stagingDir); err != nil {
		return fmt.Errorf("%w (skip restore staged db for retry: %v)", activationErr, err)
	}

	if err := c.dumpLivePostgresDatabase(context.Background(), stagedDB); err != nil {
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
		if info.Mode()&os.ModeSymlink != 0 || (!info.IsDir() && !info.Mode().IsRegular()) {
			return fmt.Errorf("restore directory entry must be a regular file or directory: %s", path)
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
	if err := validateOptionalStagedKnownHosts(src); err != nil {
		return err
	}

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
	if err := validateOptionalStagedBackupDir(src); err != nil {
		return err
	}

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
