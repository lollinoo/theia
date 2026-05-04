package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/lollinoo/theia/internal/repository/sqlite"
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
	DBDriver         string `json:"db_driver,omitempty"`
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
	dbDSN           string
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

func NewRestoreCoordinatorWithDSN(dbPath, dbDSN, deviceBackupDir, knownHostsPath string) *RestoreCoordinator {
	return &RestoreCoordinator{
		dbPath:          dbPath,
		dbDSN:           strings.TrimSpace(dbDSN),
		deviceBackupDir: deviceBackupDir,
		knownHostsPath:  knownHostsPath,
	}
}

func (c *RestoreCoordinator) ApplyPendingRestore() (bool, error) {
	ctx := context.Background()
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

	dialect := sqlite.DialectSQLite
	if strings.TrimSpace(marker.DBDriver) != "" {
		dialect, err = sqlite.NormalizeDialect(marker.DBDriver)
		if err != nil {
			return false, fmt.Errorf("parse restore marker database driver: %w", err)
		}
	}

	if filepath.Clean(marker.DBPath) != filepath.Clean(c.dbPath) ||
		filepath.Clean(marker.DeviceBackupDir) != filepath.Clean(c.deviceBackupDir) ||
		filepath.Clean(marker.KnownHostsPath) != filepath.Clean(c.knownHostsPath) {
		return false, fmt.Errorf("restore marker targets do not match runtime paths")
	}

	stagingDir := filepath.Join(filepath.Dir(c.dbPath), ".restore-staging")
	if err := c.validateStagedRestoreArtifacts(marker, dialect, stagingDir); err != nil {
		return false, err
	}

	if err := c.backupLiveDB(dialect); err != nil {
		return false, err
	}

	switch dialect {
	case sqlite.DialectPostgres:
		if err := c.applyPostgresRestore(ctx, marker.StagedDB); err != nil {
			return false, err
		}
	default:
		if err := c.applySQLiteRestore(marker.StagedDB); err != nil {
			return false, err
		}
	}

	if restoreCoordinatorAfterDBActivationHook != nil {
		if err := restoreCoordinatorAfterDBActivationHook(); err != nil {
			return false, c.restoreRetryableError(marker.StagedDB, dialect, fmt.Errorf("after db activation hook: %w", err))
		}
	}

	if marker.StagedBackups != "" && marker.DeviceBackupDir != "" {
		if info, err := os.Stat(marker.StagedBackups); err == nil {
			if !info.IsDir() {
				return false, c.restoreRetryableError(marker.StagedDB, dialect, fmt.Errorf("validate staged backup dir: staged backup dir must be a directory"))
			}
			if err := validateOptionalStagedBackupDir(marker.StagedBackups); err != nil {
				return false, c.restoreRetryableError(marker.StagedDB, dialect, fmt.Errorf("validate staged backup dir: %w", err))
			}
			if err := replaceDirForRestore(marker.StagedBackups, marker.DeviceBackupDir); err != nil {
				return false, c.restoreRetryableError(marker.StagedDB, dialect, fmt.Errorf("activate staged backup dir: %w", err))
			}
		} else if !os.IsNotExist(err) {
			return false, c.restoreRetryableError(marker.StagedDB, dialect, fmt.Errorf("stat staged backup dir: %w", err))
		}
	}

	if marker.StagedKnownHosts != "" && marker.KnownHostsPath != "" {
		if _, err := os.Stat(marker.StagedKnownHosts); err == nil {
			if err := validateOptionalStagedKnownHosts(marker.StagedKnownHosts); err != nil {
				return false, c.restoreRetryableError(marker.StagedDB, dialect, fmt.Errorf("validate staged known_hosts: %w", err))
			}
			if err := replaceFileForRestore(marker.StagedKnownHosts, marker.KnownHostsPath); err != nil {
				return false, c.restoreRetryableError(marker.StagedDB, dialect, fmt.Errorf("activate staged known_hosts: %w", err))
			}
		} else if !os.IsNotExist(err) {
			return false, c.restoreRetryableError(marker.StagedDB, dialect, fmt.Errorf("stat staged known_hosts: %w", err))
		}
	}

	if err := os.Remove(markerPath); err != nil && !os.IsNotExist(err) {
		return false, fmt.Errorf("remove restore marker: %w", err)
	}

	if stagingDir != "" && stagingDir != "." {
		if err := os.RemoveAll(stagingDir); err != nil {
			return false, fmt.Errorf("remove staging dir: %w", err)
		}
	}

	return true, nil
}

func (c *RestoreCoordinator) validateStagedRestoreArtifacts(marker restoreMarker, dialect sqlite.Dialect, stagingDir string) error {
	if err := validateRestoreStagingDir(stagingDir); err != nil {
		return err
	}

	expectedStagedDB := filepath.Join(stagingDir, "theia.db")
	if dialect == sqlite.DialectPostgres {
		expectedStagedDB = filepath.Join(stagingDir, "database.dump")
	}
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

func (c *RestoreCoordinator) applySQLiteRestore(stagedDB string) error {
	if err := validateStagedDBFile(stagedDB); err != nil {
		return err
	}

	if _, err := os.Stat(c.dbPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("stat live db: %w", err)
	}

	if err := os.Remove(c.dbPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove live db: %w", err)
	}
	if err := os.Remove(c.dbPath + "-wal"); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove live db wal: %w", err)
	}
	if err := os.Remove(c.dbPath + "-shm"); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove live db shm: %w", err)
	}
	if err := os.Rename(stagedDB, c.dbPath); err != nil {
		return fmt.Errorf("activate staged db: %w", err)
	}
	if err := os.Chmod(c.dbPath, 0600); err != nil {
		return fmt.Errorf("chmod restored db: %w", err)
	}
	return nil
}

func (c *RestoreCoordinator) applyPostgresRestore(ctx context.Context, stagedDB string) error {
	if strings.TrimSpace(c.dbDSN) == "" {
		return fmt.Errorf("postgres restore requires db_dsn")
	}
	if err := ensureExternalCommand("pg_restore"); err != nil {
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
		"pg_restore",
		"--clean",
		"--if-exists",
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

func (c *RestoreCoordinator) backupLiveDB(dialect sqlite.Dialect) error {
	if dialect == sqlite.DialectPostgres {
		bakPath := c.dbPath + ".pre-restore.dump"
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

func (c *RestoreCoordinator) dumpLivePostgresDatabase(ctx context.Context, destPath string) error {
	if strings.TrimSpace(c.dbDSN) == "" {
		return fmt.Errorf("postgres backup requires db_dsn")
	}
	if err := ensureExternalCommand("pg_dump"); err != nil {
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

func (c *RestoreCoordinator) restoreRetryableError(stagedDB string, dialect sqlite.Dialect, activationErr error) error {
	if dialect == sqlite.DialectPostgres {
		if err := c.dumpLivePostgresDatabase(context.Background(), stagedDB); err != nil {
			return fmt.Errorf("%w (restore staged db for retry: %v)", activationErr, err)
		}
		return activationErr
	}

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
