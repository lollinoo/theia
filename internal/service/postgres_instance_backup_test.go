package service

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"database/sql"
	"database/sql/driver"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
)

type postgresServiceTestSetup struct {
	db                *sql.DB
	svc               *InstanceBackupService
	instanceBackupDir string
	deviceBackupDir   string
	knownHostsPath    string
	stateDir          string
	dbDSN             string
	encryptionKey     []byte
}

func setupPostgresServiceTest(t *testing.T) *postgresServiceTestSetup {
	t.Helper()

	tmpDir := t.TempDir()
	db, err := openMigrationVersionTestDB()
	if err != nil {
		t.Fatalf("opening migration version db: %v", err)
	}

	repo := newInstanceBackupCancelTestRepo()
	instanceBackupDir := filepath.Join(tmpDir, "instance-backups")
	deviceBackupDir := filepath.Join(tmpDir, "device-backups")
	knownHostsPath := filepath.Join(tmpDir, "known_hosts")
	stateDir := filepath.Join(tmpDir, "runtime")
	if err := os.MkdirAll(instanceBackupDir, 0o755); err != nil {
		t.Fatalf("creating instance backup dir: %v", err)
	}
	if err := os.MkdirAll(deviceBackupDir, 0o755); err != nil {
		t.Fatalf("creating device backup dir: %v", err)
	}
	if err := os.MkdirAll(stateDir, 0o700); err != nil {
		t.Fatalf("creating state dir: %v", err)
	}
	encryptionKey := sha256.Sum256([]byte("postgres-backup-test-key"))
	dbDSN := "postgres://theia:strong-password@localhost:5432/theia?sslmode=disable"

	svc := NewInstanceBackupService(
		db,
		repo,
		nil,
		instanceBackupDir,
		deviceBackupDir,
		knownHostsPath,
		stateDir,
		dbDSN,
		encryptionKey[:],
	)

	t.Cleanup(func() {
		_ = db.Close()
	})

	return &postgresServiceTestSetup{
		db:                db,
		svc:               svc,
		instanceBackupDir: instanceBackupDir,
		deviceBackupDir:   deviceBackupDir,
		knownHostsPath:    knownHostsPath,
		stateDir:          stateDir,
		dbDSN:             dbDSN,
		encryptionKey:     encryptionKey[:],
	}
}

const migrationVersionTestDriverName = "theia_postgres_migration_version_test"

var registerMigrationVersionTestDriver sync.Once

func openMigrationVersionTestDB() (*sql.DB, error) {
	registerMigrationVersionTestDriver.Do(func() {
		sql.Register(migrationVersionTestDriverName, migrationVersionTestDriver{})
	})
	return sql.Open(migrationVersionTestDriverName, "")
}

type migrationVersionTestDriver struct{}

func (migrationVersionTestDriver) Open(name string) (driver.Conn, error) {
	return migrationVersionTestConn{}, nil
}

type migrationVersionTestConn struct{}

func (migrationVersionTestConn) Prepare(query string) (driver.Stmt, error) {
	return nil, driver.ErrSkip
}

func (migrationVersionTestConn) Close() error {
	return nil
}

func (migrationVersionTestConn) Begin() (driver.Tx, error) {
	return nil, driver.ErrSkip
}

func (migrationVersionTestConn) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	if strings.Contains(strings.ToLower(query), "schema_migrations") {
		return &singleIntRows{value: 15}, nil
	}
	return nil, fmt.Errorf("unexpected query %q", query)
}

type singleIntRows struct {
	value int
	done  bool
}

func (r *singleIntRows) Columns() []string {
	return []string{"version"}
}

func (r *singleIntRows) Close() error {
	return nil
}

func (r *singleIntRows) Next(dest []driver.Value) error {
	if r.done {
		return io.EOF
	}
	dest[0] = int64(r.value)
	r.done = true
	return nil
}

func stubExternalCommands(t *testing.T, runner func(context.Context, string, ...string) ([]byte, error)) {
	t.Helper()

	stubExternalCommandsWithEnv(t, func(ctx context.Context, _ []string, name string, args ...string) ([]byte, error) {
		return runner(ctx, name, args...)
	})
}

func stubExternalCommandsWithEnv(t *testing.T, runner func(context.Context, []string, string, ...string) ([]byte, error)) {
	t.Helper()

	originalRunner := runExternalCommand
	originalRunnerWithEnv := runExternalCommandWithEnv
	originalLookup := lookupExternalCommand
	runExternalCommand = func(ctx context.Context, name string, args ...string) ([]byte, error) {
		return runner(ctx, nil, name, args...)
	}
	runExternalCommandWithEnv = runner
	lookupExternalCommand = func(name string) (string, error) {
		return "/usr/bin/" + name, nil
	}
	t.Cleanup(func() {
		runExternalCommand = originalRunner
		runExternalCommandWithEnv = originalRunnerWithEnv
		lookupExternalCommand = originalLookup
	})
}

func writePostgresArchive(t *testing.T, archivePath string, manifest backupManifest, entries map[string][]byte) {
	t.Helper()

	f, err := os.Create(archivePath)
	if err != nil {
		t.Fatalf("creating archive: %v", err)
	}
	defer f.Close()

	gw := gzip.NewWriter(f)
	defer gw.Close()

	tw := tar.NewWriter(gw)
	defer tw.Close()

	manifestJSON, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		t.Fatalf("marshaling manifest: %v", err)
	}
	if err := tw.WriteHeader(&tar.Header{Name: "manifest.json", Size: int64(len(manifestJSON)), Mode: 0o644}); err != nil {
		t.Fatalf("writing manifest header: %v", err)
	}
	if _, err := tw.Write(manifestJSON); err != nil {
		t.Fatalf("writing manifest: %v", err)
	}

	for name, data := range entries {
		if err := tw.WriteHeader(&tar.Header{Name: name, Size: int64(len(data)), Mode: 0o644}); err != nil {
			t.Fatalf("writing header for %s: %v", name, err)
		}
		if _, err := tw.Write(data); err != nil {
			t.Fatalf("writing entry %s: %v", name, err)
		}
	}
}

func readArchiveEntries(t *testing.T, archivePath string) map[string][]byte {
	t.Helper()

	f, err := os.Open(archivePath)
	if err != nil {
		t.Fatalf("opening archive: %v", err)
	}
	defer f.Close()

	gr, err := gzip.NewReader(f)
	if err != nil {
		t.Fatalf("creating gzip reader: %v", err)
	}
	defer gr.Close()

	tr := tar.NewReader(gr)
	entries := map[string][]byte{}
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("reading tar entry: %v", err)
		}
		data, err := io.ReadAll(tr)
		if err != nil {
			t.Fatalf("reading tar entry %s: %v", hdr.Name, err)
		}
		entries[hdr.Name] = data
	}
	return entries
}

func manifestKeyHash(key []byte) string {
	sum := sha256.Sum256(key[:8])
	return hex.EncodeToString(sum[:])
}

func manifestMigrationVersion(t *testing.T, db *sql.DB) int {
	t.Helper()

	var version int
	if err := db.QueryRow("SELECT version FROM schema_migrations").Scan(&version); err != nil {
		t.Fatalf("query migration version: %v", err)
	}
	return version
}

func commandFlagValue(args []string, flag string) string {
	for i := 0; i < len(args)-1; i++ {
		if args[i] == flag {
			return args[i+1]
		}
	}
	return ""
}

func commandArgsEqual(args []string, want ...string) bool {
	if len(args) != len(want) {
		return false
	}
	for i := range args {
		if args[i] != want[i] {
			return false
		}
	}
	return true
}

func commandArgExists(args []string, want string) bool {
	for _, arg := range args {
		if arg == want {
			return true
		}
	}
	return false
}

func commandEnvValue(env []string, key string) string {
	prefix := key + "="
	for _, item := range env {
		if strings.HasPrefix(item, prefix) {
			return strings.TrimPrefix(item, prefix)
		}
	}
	return ""
}

func TestInstanceBackupServiceCreate_PostgresArchive(t *testing.T) {
	ts := setupPostgresServiceTest(t)

	stubExternalCommandsWithEnv(t, func(ctx context.Context, env []string, name string, args ...string) ([]byte, error) {
		switch name {
		case "pg_dump":
			if commandArgsEqual(args, "--version") {
				return []byte("pg_dump (PostgreSQL) 17.4\n"), nil
			}
			if commandEnvValue(env, "PGPASSWORD") != "strong-password" {
				t.Fatal("pg_dump PGPASSWORD env does not match DSN password")
			}
			connInfo := commandFlagValue(args, "--dbname")
			if strings.Contains(connInfo, "password") || strings.Contains(connInfo, "strong-password") {
				t.Fatalf("pg_dump conninfo leaked password material: %q", connInfo)
			}
			if strings.Contains(connInfo, "postgres://") {
				t.Fatalf("pg_dump conninfo should not use raw URL dsn: %q", connInfo)
			}
			dest := commandFlagValue(args, "--file")
			if dest == "" {
				t.Fatal("pg_dump missing --file argument")
			}
			if err := os.WriteFile(dest, []byte("postgres-dump-data"), 0o600); err != nil {
				t.Fatalf("writing fake dump: %v", err)
			}
			return nil, nil
		default:
			return nil, fmt.Errorf("unexpected command %s", name)
		}
	})

	backup, err := ts.svc.Create(context.Background())
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	entries := readArchiveEntries(t, backup.FilePath)
	if _, ok := entries[postgresArchiveDBEntry]; !ok {
		t.Fatalf("archive missing %s", postgresArchiveDBEntry)
	}

	var manifest backupManifest
	if err := json.Unmarshal(entries["manifest.json"], &manifest); err != nil {
		t.Fatalf("unmarshal manifest: %v", err)
	}
	if manifest.DBEntryName != postgresArchiveDBEntry {
		t.Fatalf("manifest.DBEntryName = %q, want %q", manifest.DBEntryName, postgresArchiveDBEntry)
	}
	if backup.Status != "success" {
		t.Fatalf("backup.Status = %q, want success", backup.Status)
	}
}

func TestInstanceBackupServiceCreate_PostgresRejectsUnsupportedPgDumpVersionBeforeDump(t *testing.T) {
	ts := setupPostgresServiceTest(t)
	dumpExecuted := false

	stubExternalCommandsWithEnv(t, func(ctx context.Context, env []string, name string, args ...string) ([]byte, error) {
		if name != "pg_dump" {
			return nil, fmt.Errorf("unexpected command %s", name)
		}
		if commandArgsEqual(args, "--version") {
			return []byte("pg_dump (PostgreSQL) 16.10\n"), nil
		}
		if commandArgExists(args, "--format=custom") {
			dumpExecuted = true
			dest := commandFlagValue(args, "--file")
			if dest == "" {
				t.Fatal("pg_dump missing --file argument")
			}
			if err := os.WriteFile(dest, []byte("postgres-dump-data"), 0o600); err != nil {
				t.Fatalf("writing fake dump: %v", err)
			}
			return nil, nil
		}
		return nil, fmt.Errorf("unexpected pg_dump args: %v", args)
	})

	_, err := ts.svc.Create(context.Background())
	if err == nil {
		t.Fatal("Create() error = nil, want unsupported pg_dump version")
	}
	assertPostgresCLIActionableError(t, err.Error(), "pg_dump")
	if dumpExecuted {
		t.Fatal("pg_dump --format=custom executed before rejecting unsupported version")
	}

	backups, listErr := ts.svc.List(context.Background())
	if listErr != nil {
		t.Fatalf("List() error = %v", listErr)
	}
	if len(backups) != 1 {
		t.Fatalf("backup count = %d, want 1", len(backups))
	}
	if backups[0].Status != "failed" {
		t.Fatalf("backup status = %q, want failed", backups[0].Status)
	}
	assertPostgresCLIActionableError(t, backups[0].ErrorMessage, "pg_dump")
}

func TestInstanceBackupServiceCreate_PostgresFailureRedactsCommandSecrets(t *testing.T) {
	ts := setupPostgresServiceTest(t)
	const sensitive = "should-not-appear"
	ts.svc.dbDSN = "postgres://theia:" + sensitive + "@localhost:5432/theia?sslmode=disable"

	stubExternalCommandsWithEnv(t, func(ctx context.Context, env []string, name string, args ...string) ([]byte, error) {
		if name != "pg_dump" {
			return nil, fmt.Errorf("unexpected command %s", name)
		}
		if commandArgsEqual(args, "--version") {
			return []byte("pg_dump (PostgreSQL) 17.4\n"), nil
		}
		if commandEnvValue(env, "PGPASSWORD") != sensitive {
			t.Fatal("pg_dump PGPASSWORD env does not match DSN password")
		}
		output := []byte("FATAL: password=" + sensitive)
		return output, externalCommandError(name, args, errors.New("exit status 1"), output)
	})

	_, err := ts.svc.Create(context.Background())
	if err == nil {
		t.Fatal("Create() error = nil, want pg_dump failure")
	}

	for _, message := range []string{err.Error()} {
		if strings.Contains(message, sensitive) {
			t.Fatal("returned error leaked sensitive DSN password")
		}
		if strings.Contains(message, "postgres://theia:"+sensitive+"@localhost") {
			t.Fatal("returned error leaked raw postgres DSN")
		}
		if !strings.Contains(message, "--dbname [redacted]") {
			t.Fatalf("returned error missing redacted command context: %q", message)
		}
	}

	backups, listErr := ts.svc.List(context.Background())
	if listErr != nil {
		t.Fatalf("List() error = %v", listErr)
	}
	if len(backups) != 1 {
		t.Fatalf("backup count = %d, want 1", len(backups))
	}
	if backups[0].Status != "failed" {
		t.Fatalf("backup status = %q, want failed", backups[0].Status)
	}
	if strings.Contains(backups[0].ErrorMessage, sensitive) {
		t.Fatal("stored backup error message leaked sensitive DSN password")
	}
	if !strings.Contains(backups[0].ErrorMessage, "--dbname [redacted]") {
		t.Fatalf("stored backup error missing redacted command context: %q", backups[0].ErrorMessage)
	}
}

func TestInstanceBackupServiceValidateAndStageRestore_Postgres(t *testing.T) {
	ts := setupPostgresServiceTest(t)

	stubExternalCommands(t, func(ctx context.Context, name string, args ...string) ([]byte, error) {
		switch name {
		case "pg_restore":
			if commandArgsEqual(args, "--version") {
				return []byte("pg_restore (PostgreSQL) 17.4\n"), nil
			}
			if len(args) < 2 || args[0] != "--list" {
				return nil, fmt.Errorf("unexpected pg_restore args: %v", args)
			}
			return []byte("archive listing"), nil
		case "pg_dump":
			if commandArgsEqual(args, "--version") {
				return []byte("pg_dump (PostgreSQL) 17.4\n"), nil
			}
			return nil, fmt.Errorf("unexpected pg_dump args: %v", args)
		case "psql":
			if commandArgsEqual(args, "--version") {
				return []byte("psql (PostgreSQL) 17.4\n"), nil
			}
			return nil, fmt.Errorf("unexpected psql args: %v", args)
		default:
			return nil, fmt.Errorf("unexpected command %s", name)
		}
	})

	dumpData := []byte("postgres-dump-data")
	dumpHash := sha256.Sum256(dumpData)
	archivePath := filepath.Join(t.TempDir(), "postgres-backup.tar.gz")
	manifest := backupManifest{
		Version:           1,
		AppVersion:        "dev",
		GitCommit:         "test",
		DBEntryName:       postgresArchiveDBEntry,
		MigrationVersion:  manifestMigrationVersion(t, ts.db),
		CreatedAt:         "2026-04-23T00:00:00Z",
		DBSHA256:          hex.EncodeToString(dumpHash[:]),
		BackupFileCount:   1,
		TotalSizeBytes:    int64(len(dumpData)),
		EncryptionKeyHash: manifestKeyHash(ts.encryptionKey),
	}
	writePostgresArchive(t, archivePath, manifest, map[string][]byte{
		postgresArchiveDBEntry:      dumpData,
		"backups/router/config.rsc": []byte("backup-data"),
		"known_hosts":               []byte("example-host ssh-ed25519 AAAA\n"),
	})

	report, err := ts.svc.ValidateAndStageRestore(archivePath, false)
	if err != nil {
		t.Fatalf("ValidateAndStageRestore() error = %v", err)
	}
	if !report.Valid {
		t.Fatal("expected report.Valid to be true")
	}

	stagingDir := filepath.Join(ts.stateDir, ".restore-staging")
	if _, err := os.Stat(filepath.Join(stagingDir, postgresArchiveDBEntry)); err != nil {
		t.Fatalf("staged postgres dump missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(stagingDir, "backups", "router", "config.rsc")); err != nil {
		t.Fatalf("staged backup file missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(stagingDir, "known_hosts")); err != nil {
		t.Fatalf("staged known_hosts missing: %v", err)
	}

	markerBytes, err := os.ReadFile(filepath.Join(ts.stateDir, ".theia-restore-pending"))
	if err != nil {
		t.Fatalf("reading restore marker: %v", err)
	}
	var marker restoreMarker
	if err := json.Unmarshal(markerBytes, &marker); err != nil {
		t.Fatalf("unmarshal restore marker: %v", err)
	}
	if !strings.HasSuffix(marker.StagedDB, postgresArchiveDBEntry) {
		t.Fatalf("marker.StagedDB = %q, want suffix %q", marker.StagedDB, postgresArchiveDBEntry)
	}
	assertRestoreStatusFilePhase(t, ts.stateDir, "staged_restart_pending", 0, "")
}

func TestMoveOrCopyFileForRestoreStagingRenamesOnSameFilesystem(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "extracted", "database.dump")
	dst := filepath.Join(root, "staging", "database.dump")
	if err := os.MkdirAll(filepath.Dir(src), 0o755); err != nil {
		t.Fatalf("creating source dir: %v", err)
	}
	if err := os.WriteFile(src, []byte("postgres-dump"), 0o644); err != nil {
		t.Fatalf("writing source file: %v", err)
	}

	if err := moveOrCopyFileForRestoreStagingContext(context.Background(), src, dst); err != nil {
		t.Fatalf("moveOrCopyFileForRestoreStagingContext: %v", err)
	}

	if _, err := os.Stat(src); !os.IsNotExist(err) {
		t.Fatalf("source should be moved after same-filesystem staging, stat err = %v", err)
	}
	data, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("reading staged file: %v", err)
	}
	if string(data) != "postgres-dump" {
		t.Fatalf("staged file = %q, want postgres-dump", string(data))
	}
	info, err := os.Stat(dst)
	if err != nil {
		t.Fatalf("stat staged file: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("staged file mode = %o, want 600", got)
	}
}

func TestMoveOrCopyFileForRestoreStagingFallsBackOnCrossDeviceRename(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "extracted", "known_hosts")
	dst := filepath.Join(root, "staging", "known_hosts")
	if err := os.MkdirAll(filepath.Dir(src), 0o755); err != nil {
		t.Fatalf("creating source dir: %v", err)
	}
	if err := os.WriteFile(src, []byte("host-key"), 0o644); err != nil {
		t.Fatalf("writing source file: %v", err)
	}

	originalRename := renameRestoreStagingPath
	renameRestoreStagingPath = func(oldPath, newPath string) error {
		return &os.LinkError{Op: "rename", Old: oldPath, New: newPath, Err: syscall.EXDEV}
	}
	t.Cleanup(func() {
		renameRestoreStagingPath = originalRename
	})

	if err := moveOrCopyFileForRestoreStagingContext(context.Background(), src, dst); err != nil {
		t.Fatalf("moveOrCopyFileForRestoreStagingContext: %v", err)
	}

	if _, err := os.Stat(src); err != nil {
		t.Fatalf("source should remain after copy fallback: %v", err)
	}
	data, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("reading staged file: %v", err)
	}
	if string(data) != "host-key" {
		t.Fatalf("staged file = %q, want host-key", string(data))
	}
}

func TestInstanceBackupServiceValidateAndStageRestore_RejectsLegacySQLiteArchiveWithActionableError(t *testing.T) {
	ts := setupPostgresServiceTest(t)

	dbData := []byte("sqlite-database-data")
	dbHash := sha256.Sum256(dbData)
	archivePath := filepath.Join(t.TempDir(), "sqlite-backup.tar.gz")
	manifest := backupManifest{
		Version:           1,
		AppVersion:        "1.7.0",
		GitCommit:         "test",
		DBEntryName:       "theia.db",
		MigrationVersion:  manifestMigrationVersion(t, ts.db),
		CreatedAt:         "2026-04-23T00:00:00Z",
		DBSHA256:          hex.EncodeToString(dbHash[:]),
		BackupFileCount:   0,
		TotalSizeBytes:    int64(len(dbData)),
		EncryptionKeyHash: manifestKeyHash(ts.encryptionKey),
	}
	writePostgresArchive(t, archivePath, manifest, map[string][]byte{
		"theia.db": dbData,
	})

	_, err := ts.svc.ValidateAndStageRestore(archivePath, true)
	if err == nil {
		t.Fatal("ValidateAndStageRestore() error = nil, want legacy SQLite archive error")
	}
	if !strings.Contains(err.Error(), "legacy SQLite instance backup") {
		t.Fatalf("ValidateAndStageRestore() error = %v, want legacy SQLite compatibility guidance", err)
	}
	if strings.Contains(err.Error(), "disallowed restore archive entry") {
		t.Fatalf("ValidateAndStageRestore() error = %v, want actionable legacy archive error", err)
	}
}

func TestInstanceBackupServiceValidateAndStageRestore_PostgresRejectsUnsupportedPgRestoreVersion(t *testing.T) {
	ts := setupPostgresServiceTest(t)
	listExecuted := false

	stubExternalCommands(t, func(ctx context.Context, name string, args ...string) ([]byte, error) {
		if name != "pg_restore" {
			return nil, fmt.Errorf("unexpected command %s", name)
		}
		if commandArgsEqual(args, "--version") {
			return []byte("pg_restore (PostgreSQL) 16.10\n"), nil
		}
		if len(args) >= 2 && args[0] == "--list" {
			listExecuted = true
			return []byte("archive listing"), nil
		}
		return nil, fmt.Errorf("unexpected pg_restore args: %v", args)
	})

	dumpData := []byte("postgres-dump-data")
	dumpHash := sha256.Sum256(dumpData)
	archivePath := filepath.Join(t.TempDir(), "postgres-backup.tar.gz")
	manifest := backupManifest{
		Version:           1,
		AppVersion:        "dev",
		GitCommit:         "test",
		DBEntryName:       postgresArchiveDBEntry,
		MigrationVersion:  manifestMigrationVersion(t, ts.db),
		CreatedAt:         "2026-04-23T00:00:00Z",
		DBSHA256:          hex.EncodeToString(dumpHash[:]),
		BackupFileCount:   1,
		TotalSizeBytes:    int64(len(dumpData)),
		EncryptionKeyHash: manifestKeyHash(ts.encryptionKey),
	}
	writePostgresArchive(t, archivePath, manifest, map[string][]byte{
		postgresArchiveDBEntry: dumpData,
	})

	_, err := ts.svc.ValidateAndStageRestore(archivePath, false)
	if err == nil {
		t.Fatal("ValidateAndStageRestore() error = nil, want unsupported pg_restore version")
	}
	assertPostgresCLIActionableError(t, err.Error(), "pg_restore")
	if listExecuted {
		t.Fatal("pg_restore --list executed before rejecting unsupported version")
	}
}

func TestInstanceBackupServiceValidateAndStageRestore_PostgresRejectsUnsupportedPgDumpBeforeStaging(t *testing.T) {
	ts := setupPostgresServiceTest(t)

	stubExternalCommands(t, func(ctx context.Context, name string, args ...string) ([]byte, error) {
		switch name {
		case "pg_restore":
			if commandArgsEqual(args, "--version") {
				return []byte("pg_restore (PostgreSQL) 17.4\n"), nil
			}
			if len(args) >= 2 && args[0] == "--list" {
				return []byte("archive listing"), nil
			}
			return nil, fmt.Errorf("unexpected pg_restore args: %v", args)
		case "pg_dump":
			if commandArgsEqual(args, "--version") {
				return []byte("pg_dump (PostgreSQL) 16.10\n"), nil
			}
			return nil, fmt.Errorf("unexpected pg_dump args: %v", args)
		default:
			return nil, fmt.Errorf("unexpected command %s", name)
		}
	})

	dumpData := []byte("postgres-dump-data")
	dumpHash := sha256.Sum256(dumpData)
	archivePath := filepath.Join(t.TempDir(), "postgres-backup.tar.gz")
	manifest := backupManifest{
		Version:           1,
		AppVersion:        "dev",
		GitCommit:         "test",
		DBEntryName:       postgresArchiveDBEntry,
		MigrationVersion:  manifestMigrationVersion(t, ts.db),
		CreatedAt:         "2026-04-23T00:00:00Z",
		DBSHA256:          hex.EncodeToString(dumpHash[:]),
		BackupFileCount:   1,
		TotalSizeBytes:    int64(len(dumpData)),
		EncryptionKeyHash: manifestKeyHash(ts.encryptionKey),
	}
	writePostgresArchive(t, archivePath, manifest, map[string][]byte{
		postgresArchiveDBEntry: dumpData,
	})

	_, err := ts.svc.ValidateAndStageRestore(archivePath, false)
	if err == nil {
		t.Fatal("ValidateAndStageRestore() error = nil, want unsupported pg_dump version")
	}
	assertPostgresCLIActionableError(t, err.Error(), "pg_dump")

	stagingDir := filepath.Join(ts.stateDir, ".restore-staging")
	if _, err := os.Stat(stagingDir); !os.IsNotExist(err) {
		t.Fatalf("restore staging dir should not exist, stat err = %v", err)
	}
	markerPath := filepath.Join(ts.stateDir, ".theia-restore-pending")
	if _, err := os.Stat(markerPath); !os.IsNotExist(err) {
		t.Fatalf("restore marker should not exist, stat err = %v", err)
	}
}

func TestRestoreCoordinatorApplyPendingRestore_Postgres(t *testing.T) {
	const dbDSN = "postgres://theia:strong-password@localhost:5432/theia?sslmode=disable"

	stateDir := t.TempDir()
	deviceBackupDir := filepath.Join(stateDir, "device-backups")
	knownHostsPath := filepath.Join(stateDir, "known_hosts")
	stagingDir := filepath.Join(stateDir, ".restore-staging")
	stagedDump := filepath.Join(stagingDir, postgresArchiveDBEntry)
	if err := os.MkdirAll(filepath.Join(stagingDir, "backups", "router"), 0o755); err != nil {
		t.Fatalf("creating staging dir: %v", err)
	}
	if err := os.WriteFile(stagedDump, []byte("staged-pg-dump"), 0o600); err != nil {
		t.Fatalf("writing staged dump: %v", err)
	}
	if err := os.WriteFile(filepath.Join(stagingDir, "backups", "router", "config.rsc"), []byte("restored-backup"), 0o600); err != nil {
		t.Fatalf("writing staged backup file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(stagingDir, "known_hosts"), []byte("restored-known-hosts"), 0o600); err != nil {
		t.Fatalf("writing staged known_hosts: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(deviceBackupDir, "router"), 0o755); err != nil {
		t.Fatalf("creating device backup dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(deviceBackupDir, "router", "config.rsc"), []byte("live-backup"), 0o644); err != nil {
		t.Fatalf("writing live backup: %v", err)
	}
	if err := os.WriteFile(knownHostsPath, []byte("live-known-hosts"), 0o644); err != nil {
		t.Fatalf("writing live known_hosts: %v", err)
	}

	originalTerminate := terminatePostgresConnections
	terminatePostgresConnections = func(ctx context.Context, dsn string) error { return nil }
	t.Cleanup(func() { terminatePostgresConnections = originalTerminate })

	cleanSchemaExecuted := false
	stubExternalCommandsWithEnv(t, func(ctx context.Context, env []string, name string, args ...string) ([]byte, error) {
		switch name {
		case "pg_dump":
			if commandArgsEqual(args, "--version") {
				return []byte("pg_dump (PostgreSQL) 17.4\n"), nil
			}
			if commandEnvValue(env, "PGPASSWORD") != "strong-password" {
				t.Fatal("pg_dump PGPASSWORD env does not match DSN password")
			}
			connInfo := commandFlagValue(args, "--dbname")
			if strings.Contains(connInfo, "password") || strings.Contains(connInfo, "strong-password") {
				t.Fatalf("pg_dump conninfo leaked password material: %q", connInfo)
			}
			dest := commandFlagValue(args, "--file")
			if dest == "" {
				t.Fatal("pg_dump missing --file argument")
			}
			if err := os.WriteFile(dest, []byte("pre-restore-pg-dump"), 0o600); err != nil {
				t.Fatalf("writing pre-restore dump: %v", err)
			}
			return nil, nil
		case "pg_restore":
			if commandArgsEqual(args, "--version") {
				return []byte("pg_restore (PostgreSQL) 17.4\n"), nil
			}
			if !cleanSchemaExecuted {
				t.Fatal("pg_restore executed before PostgreSQL schema cleanup")
			}
			if commandEnvValue(env, "PGPASSWORD") != "strong-password" {
				t.Fatal("pg_restore PGPASSWORD env does not match DSN password")
			}
			connInfo := commandFlagValue(args, "--dbname")
			if strings.Contains(connInfo, "password") || strings.Contains(connInfo, "strong-password") {
				t.Fatalf("pg_restore conninfo leaked password material: %q", connInfo)
			}
			if strings.Contains(connInfo, "postgres://") {
				t.Fatalf("pg_restore conninfo should not use raw URL dsn: %q", connInfo)
			}
			if got := args[len(args)-1]; got != stagedDump {
				t.Fatalf("pg_restore target = %q, want %q", got, stagedDump)
			}
			if commandArgExists(args, "--clean") {
				t.Fatal("pg_restore should not use --clean after explicit schema cleanup")
			}
			return nil, nil
		case "psql":
			if commandArgsEqual(args, "--version") {
				return []byte("psql (PostgreSQL) 17.4\n"), nil
			}
			if commandEnvValue(env, "PGPASSWORD") != "strong-password" {
				t.Fatal("psql PGPASSWORD env does not match DSN password")
			}
			connInfo := commandFlagValue(args, "--dbname")
			if strings.Contains(connInfo, "password") || strings.Contains(connInfo, "strong-password") {
				t.Fatalf("psql conninfo leaked password material: %q", connInfo)
			}
			command := commandFlagValue(args, "--command")
			if !strings.Contains(command, "DROP SCHEMA IF EXISTS public CASCADE") {
				t.Fatalf("psql schema cleanup command = %q, want DROP SCHEMA public CASCADE", command)
			}
			if !strings.Contains(command, "CREATE SCHEMA public") {
				t.Fatalf("psql schema cleanup command = %q, want CREATE SCHEMA public", command)
			}
			cleanSchemaExecuted = true
			return nil, nil
		default:
			return nil, fmt.Errorf("unexpected command %s", name)
		}
	})

	marker := newRestoreMarker(
		stagedDump,
		filepath.Join(stagingDir, "backups"),
		filepath.Join(stagingDir, "known_hosts"),
		stateDir,
		deviceBackupDir,
		knownHostsPath,
		"2026-04-23T00:00:00Z",
	)
	markerJSON, err := json.Marshal(marker)
	if err != nil {
		t.Fatalf("marshal marker: %v", err)
	}
	if err := os.WriteFile(filepath.Join(stateDir, ".theia-restore-pending"), markerJSON, 0o600); err != nil {
		t.Fatalf("write marker: %v", err)
	}

	coordinator := NewRestoreCoordinatorWithDSN(stateDir, dbDSN, deviceBackupDir, knownHostsPath)
	applied, err := coordinator.ApplyPendingRestore()
	if err != nil {
		t.Fatalf("ApplyPendingRestore() error = %v", err)
	}
	if !applied {
		t.Fatal("ApplyPendingRestore() applied = false, want true")
	}
	if !cleanSchemaExecuted {
		t.Fatal("PostgreSQL schema cleanup was not executed")
	}

	backupBytes, err := os.ReadFile(filepath.Join(deviceBackupDir, "router", "config.rsc"))
	if err != nil {
		t.Fatalf("reading restored backup file: %v", err)
	}
	if string(backupBytes) != "restored-backup" {
		t.Fatalf("restored backup = %q, want restored-backup", string(backupBytes))
	}
	knownHostsBytes, err := os.ReadFile(knownHostsPath)
	if err != nil {
		t.Fatalf("reading restored known_hosts: %v", err)
	}
	if string(knownHostsBytes) != "restored-known-hosts" {
		t.Fatalf("restored known_hosts = %q, want restored-known-hosts", string(knownHostsBytes))
	}
	if _, err := os.Stat(filepath.Join(stateDir, "postgres.pre-restore.dump")); err != nil {
		t.Fatalf("pre-restore dump missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(stateDir, ".theia-restore-pending")); !os.IsNotExist(err) {
		t.Fatalf("restore marker should be removed, stat err = %v", err)
	}
	if _, err := os.Stat(stagingDir); !os.IsNotExist(err) {
		t.Fatalf("staging dir should be removed, stat err = %v", err)
	}
	assertRestoreStatusFilePhase(t, stateDir, "completed", 1, "")
}

// TestRestoreCoordinatorApplyPendingRestore_PostgresRestoreFailureKeepsRetryState preserves retry safety after DB activation fails.
func TestRestoreCoordinatorApplyPendingRestore_PostgresRestoreFailureKeepsRetryState(t *testing.T) {
	const dbDSN = "postgres://theia:strong-password@localhost:5432/theia?sslmode=disable"

	stateDir := t.TempDir()
	deviceBackupDir := filepath.Join(stateDir, "device-backups")
	knownHostsPath := filepath.Join(stateDir, "known_hosts")
	stagingDir := filepath.Join(stateDir, ".restore-staging")
	stagedDump := filepath.Join(stagingDir, postgresArchiveDBEntry)
	if err := os.MkdirAll(filepath.Join(stagingDir, "backups", "router"), 0o755); err != nil {
		t.Fatalf("creating staging backup dir: %v", err)
	}
	if err := os.WriteFile(stagedDump, []byte("staged-pg-dump"), 0o600); err != nil {
		t.Fatalf("writing staged dump: %v", err)
	}
	if err := os.WriteFile(filepath.Join(stagingDir, "backups", "router", "config.rsc"), []byte("restored-backup"), 0o600); err != nil {
		t.Fatalf("writing staged backup file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(stagingDir, "known_hosts"), []byte("restored-known-hosts"), 0o600); err != nil {
		t.Fatalf("writing staged known_hosts: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(deviceBackupDir, "router"), 0o755); err != nil {
		t.Fatalf("creating live backup dir: %v", err)
	}
	liveBackupPath := filepath.Join(deviceBackupDir, "router", "config.rsc")
	if err := os.WriteFile(liveBackupPath, []byte("live-backup"), 0o644); err != nil {
		t.Fatalf("writing live backup: %v", err)
	}
	if err := os.WriteFile(knownHostsPath, []byte("live-known-hosts"), 0o644); err != nil {
		t.Fatalf("writing live known_hosts: %v", err)
	}

	originalTerminate := terminatePostgresConnections
	terminatePostgresConnections = func(ctx context.Context, dsn string) error { return nil }
	t.Cleanup(func() { terminatePostgresConnections = originalTerminate })

	pgRestoreExecuted := false
	stubExternalCommandsWithEnv(t, func(ctx context.Context, env []string, name string, args ...string) ([]byte, error) {
		switch name {
		case "pg_dump":
			if commandArgsEqual(args, "--version") {
				return []byte("pg_dump (PostgreSQL) 17.4\n"), nil
			}
			dest := commandFlagValue(args, "--file")
			if dest == "" {
				t.Fatal("pg_dump missing --file argument")
			}
			if err := os.WriteFile(dest, []byte("pre-restore-pg-dump"), 0o600); err != nil {
				t.Fatalf("writing pre-restore dump: %v", err)
			}
			return nil, nil
		case "pg_restore":
			if commandArgsEqual(args, "--version") {
				return []byte("pg_restore (PostgreSQL) 17.4\n"), nil
			}
			pgRestoreExecuted = true
			return nil, fmt.Errorf("restore failed")
		case "psql":
			if commandArgsEqual(args, "--version") {
				return []byte("psql (PostgreSQL) 17.4\n"), nil
			}
			return nil, nil
		default:
			return nil, fmt.Errorf("unexpected command %s", name)
		}
	})

	marker := newRestoreMarker(
		stagedDump,
		filepath.Join(stagingDir, "backups"),
		filepath.Join(stagingDir, "known_hosts"),
		stateDir,
		deviceBackupDir,
		knownHostsPath,
		"2026-04-23T00:00:00Z",
	)
	markerJSON, err := json.Marshal(marker)
	if err != nil {
		t.Fatalf("marshal marker: %v", err)
	}
	markerPath := filepath.Join(stateDir, ".theia-restore-pending")
	if err := os.WriteFile(markerPath, markerJSON, 0o600); err != nil {
		t.Fatalf("write marker: %v", err)
	}

	coordinator := NewRestoreCoordinatorWithDSN(stateDir, dbDSN, deviceBackupDir, knownHostsPath)
	applied, err := coordinator.ApplyPendingRestore()
	if err == nil {
		t.Fatal("ApplyPendingRestore() error = nil, want pg_restore failure")
	}
	if applied {
		t.Fatal("ApplyPendingRestore() applied = true, want false")
	}
	if !pgRestoreExecuted {
		t.Fatal("pg_restore was not executed")
	}
	if _, err := os.Stat(markerPath); err != nil {
		t.Fatalf("restore marker should remain for retry: %v", err)
	}
	if _, err := os.Stat(stagingDir); err != nil {
		t.Fatalf("restore staging dir should remain for retry: %v", err)
	}
	assertRestoreStatusFilePhase(t, stateDir, "failed_retryable", 1, "restore failed")
	if _, err := os.Stat(filepath.Join(stateDir, "postgres.pre-restore.dump")); err != nil {
		t.Fatalf("pre-restore dump should remain after failed restore: %v", err)
	}
	backupBytes, err := os.ReadFile(liveBackupPath)
	if err != nil {
		t.Fatalf("reading live backup after failed restore: %v", err)
	}
	if string(backupBytes) != "live-backup" {
		t.Fatalf("live backup = %q, want unchanged live-backup", string(backupBytes))
	}
	knownHostsBytes, err := os.ReadFile(knownHostsPath)
	if err != nil {
		t.Fatalf("reading live known_hosts after failed restore: %v", err)
	}
	if string(knownHostsBytes) != "live-known-hosts" {
		t.Fatalf("live known_hosts = %q, want unchanged live-known-hosts", string(knownHostsBytes))
	}
}

func TestRestoreCoordinatorApplyPendingRestoreRunsCompletionVerifierBeforeCompletion(t *testing.T) {
	const dbDSN = "postgres://theia:strong-password@localhost:5432/theia?sslmode=disable"

	stateDir := t.TempDir()
	deviceBackupDir := filepath.Join(stateDir, "device-backups")
	knownHostsPath := filepath.Join(stateDir, "known_hosts")
	stagingDir := filepath.Join(stateDir, ".restore-staging")
	stagedDump := filepath.Join(stagingDir, postgresArchiveDBEntry)
	prepareMinimalPendingPostgresRestore(t, stateDir, deviceBackupDir, knownHostsPath, stagedDump)
	stubSuccessfulPendingPostgresRestoreCommands(t)

	coordinator := NewRestoreCoordinatorWithDSN(stateDir, dbDSN, deviceBackupDir, knownHostsPath)
	coordinator.SetCompletionVerifier(func(ctx context.Context, reportPhase func(restoreOperationPhase) error) error {
		if err := reportPhase(restorePhaseVerifyingKeyring); err != nil {
			return err
		}
		assertRestoreStatusFilePhase(t, stateDir, "verifying_keyring", 1, "")
		if err := reportPhase(restorePhaseRunningCredentialRewrap); err != nil {
			return err
		}
		assertRestoreStatusFilePhase(t, stateDir, "running_credential_rewrap", 1, "")
		return nil
	})

	applied, err := coordinator.ApplyPendingRestore()
	if err != nil {
		t.Fatalf("ApplyPendingRestore() error = %v", err)
	}
	if !applied {
		t.Fatal("ApplyPendingRestore() applied = false, want true")
	}
	assertRestoreStatusFilePhase(t, stateDir, "completed", 1, "")
}

func TestRestoreCoordinatorApplyPendingRestoreMissingKeyBlocksCompletionWithOperatorStatus(t *testing.T) {
	const dbDSN = "postgres://theia:strong-password@localhost:5432/theia?sslmode=disable"

	stateDir := t.TempDir()
	deviceBackupDir := filepath.Join(stateDir, "device-backups")
	knownHostsPath := filepath.Join(stateDir, "known_hosts")
	stagingDir := filepath.Join(stateDir, ".restore-staging")
	stagedDump := filepath.Join(stagingDir, postgresArchiveDBEntry)
	prepareMinimalPendingPostgresRestore(t, stateDir, deviceBackupDir, knownHostsPath, stagedDump)
	stubSuccessfulPendingPostgresRestoreCommands(t)

	coordinator := NewRestoreCoordinatorWithDSN(stateDir, dbDSN, deviceBackupDir, knownHostsPath)
	coordinator.SetCompletionVerifier(func(ctx context.Context, reportPhase func(restoreOperationPhase) error) error {
		if err := reportPhase(restorePhaseVerifyingKeyring); err != nil {
			return err
		}
		return fmt.Errorf(`credential lifecycle check failed: archive or ciphertext requires encryption key id "kid-old", but it is not configured`)
	})

	applied, err := coordinator.ApplyPendingRestore()
	if err == nil {
		t.Fatal("ApplyPendingRestore() error = nil, want missing key error")
	}
	if applied {
		t.Fatal("ApplyPendingRestore() applied = true, want false")
	}
	if got := err.Error(); !strings.Contains(got, "kid-old") || !strings.Contains(got, "THEIA_ENCRYPTION_KEYS") {
		t.Fatalf("ApplyPendingRestore() error = %q, want missing-key operator guidance", got)
	}
	if _, err := os.Stat(restoreMarkerFilePath(stateDir)); err != nil {
		t.Fatalf("restore marker should remain for operator recovery: %v", err)
	}
	if _, err := os.Stat(stagingDir); err != nil {
		t.Fatalf("restore staging dir should remain for operator recovery: %v", err)
	}

	status := readRestoreStatusMap(t, stateDir)
	if got := status["phase"]; got != "failed_operator_action_required" {
		t.Fatalf("restore status phase = %#v, want failed_operator_action_required", got)
	}
	if got := status["missing_key_id"]; got != "kid-old" {
		t.Fatalf("restore status missing_key_id = %#v, want kid-old", got)
	}
	if got, _ := status["last_error"].(string); !strings.Contains(got, "THEIA_ENCRYPTION_KEYS") {
		t.Fatalf("restore status last_error = %q, want THEIA_ENCRYPTION_KEYS guidance", got)
	}
}

// TestRestoreCoordinatorApplyPendingRestore_OptionalArtifactFailureRefreshesStagedDBForRetry preserves post-DB retry semantics.
func TestRestoreCoordinatorApplyPendingRestore_OptionalArtifactFailureRefreshesStagedDBForRetry(t *testing.T) {
	const dbDSN = "postgres://theia:strong-password@localhost:5432/theia?sslmode=disable"

	stateDir := t.TempDir()
	deviceBackupDir := filepath.Join(stateDir, "device-backups")
	knownHostsPath := filepath.Join(stateDir, "known_hosts")
	stagingDir := filepath.Join(stateDir, ".restore-staging")
	stagedDump := filepath.Join(stagingDir, postgresArchiveDBEntry)
	stagedBackups := filepath.Join(stagingDir, "backups")
	if err := os.MkdirAll(filepath.Join(stagedBackups, "router"), 0o755); err != nil {
		t.Fatalf("creating staged backup dir: %v", err)
	}
	if err := os.WriteFile(stagedDump, []byte("staged-pg-dump"), 0o600); err != nil {
		t.Fatalf("writing staged dump: %v", err)
	}
	if err := os.WriteFile(filepath.Join(stagedBackups, "router", "config.rsc"), []byte("restored-backup"), 0o600); err != nil {
		t.Fatalf("writing staged backup file: %v", err)
	}
	liveBackupTarget := filepath.Join(stateDir, "live-backup-target")
	if err := os.Mkdir(liveBackupTarget, 0o755); err != nil {
		t.Fatalf("creating live backup target: %v", err)
	}
	if err := os.Symlink(liveBackupTarget, deviceBackupDir); err != nil {
		t.Fatalf("creating live backup symlink: %v", err)
	}

	originalTerminate := terminatePostgresConnections
	terminatePostgresConnections = func(ctx context.Context, dsn string) error { return nil }
	t.Cleanup(func() { terminatePostgresConnections = originalTerminate })

	restoreExecuted := false
	retryDumpExecuted := false
	stubExternalCommandsWithEnv(t, func(ctx context.Context, env []string, name string, args ...string) ([]byte, error) {
		switch name {
		case "pg_dump":
			if commandArgsEqual(args, "--version") {
				return []byte("pg_dump (PostgreSQL) 17.4\n"), nil
			}
			dest := commandFlagValue(args, "--file")
			if dest == "" {
				t.Fatal("pg_dump missing --file argument")
			}
			body := []byte("pre-restore-pg-dump")
			if dest == stagedDump {
				body = []byte("retry-pg-dump")
				retryDumpExecuted = true
			}
			if err := os.WriteFile(dest, body, 0o600); err != nil {
				t.Fatalf("writing pg_dump destination: %v", err)
			}
			return nil, nil
		case "pg_restore":
			if commandArgsEqual(args, "--version") {
				return []byte("pg_restore (PostgreSQL) 17.4\n"), nil
			}
			restoreExecuted = true
			return nil, nil
		case "psql":
			if commandArgsEqual(args, "--version") {
				return []byte("psql (PostgreSQL) 17.4\n"), nil
			}
			return nil, nil
		default:
			return nil, fmt.Errorf("unexpected command %s", name)
		}
	})

	marker := newRestoreMarker(
		stagedDump,
		stagedBackups,
		"",
		stateDir,
		deviceBackupDir,
		knownHostsPath,
		"2026-04-23T00:00:00Z",
	)
	markerJSON, err := json.Marshal(marker)
	if err != nil {
		t.Fatalf("marshal marker: %v", err)
	}
	markerPath := filepath.Join(stateDir, ".theia-restore-pending")
	if err := os.WriteFile(markerPath, markerJSON, 0o600); err != nil {
		t.Fatalf("write marker: %v", err)
	}

	coordinator := NewRestoreCoordinatorWithDSN(stateDir, dbDSN, deviceBackupDir, knownHostsPath)
	applied, err := coordinator.ApplyPendingRestore()
	if err == nil {
		t.Fatal("ApplyPendingRestore() error = nil, want optional artifact activation failure")
	}
	if applied {
		t.Fatal("ApplyPendingRestore() applied = true, want false")
	}
	if !strings.Contains(err.Error(), "validate live backup dir") {
		t.Fatalf("ApplyPendingRestore() error = %q, want live backup validation failure", err.Error())
	}
	if !restoreExecuted {
		t.Fatal("pg_restore was not executed before optional artifact failure")
	}
	if !retryDumpExecuted {
		t.Fatal("retry pg_dump to staged database was not executed")
	}
	if _, err := os.Stat(markerPath); err != nil {
		t.Fatalf("restore marker should remain for retry: %v", err)
	}
	if _, err := os.Stat(stagingDir); err != nil {
		t.Fatalf("restore staging dir should remain for retry: %v", err)
	}
	stagedBytes, err := os.ReadFile(stagedDump)
	if err != nil {
		t.Fatalf("reading refreshed staged dump: %v", err)
	}
	if string(stagedBytes) != "retry-pg-dump" {
		t.Fatalf("staged dump = %q, want retry-pg-dump", string(stagedBytes))
	}
	assertRestoreStatusFilePhase(t, stateDir, "failed_retryable", 1, "validate live backup dir")
	if info, err := os.Lstat(deviceBackupDir); err != nil {
		t.Fatalf("lstat live backup symlink: %v", err)
	} else if info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("live backup destination mode = %v, want unchanged symlink", info.Mode())
	}
}

func TestRestoreCoordinatorApplyPendingRestore_PostgresChecksPgDumpAndPgRestoreBeforeSideEffects(t *testing.T) {
	const dbDSN = "postgres://theia:strong-password@localhost:5432/theia?sslmode=disable"

	stateDir := t.TempDir()
	deviceBackupDir := filepath.Join(stateDir, "device-backups")
	knownHostsPath := filepath.Join(stateDir, "known_hosts")
	stagingDir := filepath.Join(stateDir, ".restore-staging")
	stagedDump := filepath.Join(stagingDir, postgresArchiveDBEntry)
	if err := os.MkdirAll(stagingDir, 0o755); err != nil {
		t.Fatalf("creating staging dir: %v", err)
	}
	if err := os.WriteFile(stagedDump, []byte("staged-pg-dump"), 0o600); err != nil {
		t.Fatalf("writing staged dump: %v", err)
	}

	terminateCalled := false
	originalTerminate := terminatePostgresConnections
	terminatePostgresConnections = func(ctx context.Context, dsn string) error {
		terminateCalled = true
		return nil
	}
	t.Cleanup(func() { terminatePostgresConnections = originalTerminate })

	preRestoreDumpExecuted := false
	actualRestoreExecuted := false
	stubExternalCommandsWithEnv(t, func(ctx context.Context, env []string, name string, args ...string) ([]byte, error) {
		switch name {
		case "pg_dump":
			if commandArgsEqual(args, "--version") {
				return []byte("pg_dump (PostgreSQL) 17.4\n"), nil
			}
			if commandArgExists(args, "--format=custom") {
				preRestoreDumpExecuted = true
				dest := commandFlagValue(args, "--file")
				if dest == "" {
					t.Fatal("pg_dump missing --file argument")
				}
				if err := os.WriteFile(dest, []byte("pre-restore-pg-dump"), 0o600); err != nil {
					t.Fatalf("writing pre-restore dump: %v", err)
				}
				return nil, nil
			}
			return nil, fmt.Errorf("unexpected pg_dump args: %v", args)
		case "pg_restore":
			if commandArgsEqual(args, "--version") {
				return []byte("pg_restore (PostgreSQL) 16.10\n"), nil
			}
			actualRestoreExecuted = true
			return nil, nil
		default:
			return nil, fmt.Errorf("unexpected command %s", name)
		}
	})

	marker := newRestoreMarker(
		stagedDump,
		"",
		"",
		stateDir,
		deviceBackupDir,
		knownHostsPath,
		"2026-04-23T00:00:00Z",
	)
	markerJSON, err := json.Marshal(marker)
	if err != nil {
		t.Fatalf("marshal marker: %v", err)
	}
	markerPath := filepath.Join(stateDir, ".theia-restore-pending")
	if err := os.WriteFile(markerPath, markerJSON, 0o600); err != nil {
		t.Fatalf("write marker: %v", err)
	}

	coordinator := NewRestoreCoordinatorWithDSN(stateDir, dbDSN, deviceBackupDir, knownHostsPath)
	applied, err := coordinator.ApplyPendingRestore()
	if err == nil {
		t.Fatal("ApplyPendingRestore() error = nil, want unsupported pg_restore version")
	}
	if applied {
		t.Fatal("ApplyPendingRestore() applied = true, want false")
	}
	assertPostgresCLIActionableError(t, err.Error(), "pg_restore")
	if terminateCalled {
		t.Fatal("terminatePostgresConnections called before PostgreSQL tool preflight completed")
	}
	if preRestoreDumpExecuted {
		t.Fatal("pg_dump pre-restore dump executed before PostgreSQL tool preflight completed")
	}
	if actualRestoreExecuted {
		t.Fatal("actual pg_restore executed after unsupported version")
	}
	if _, err := os.Stat(filepath.Join(stateDir, "postgres.pre-restore.dump")); !os.IsNotExist(err) {
		t.Fatalf("pre-restore dump should not exist, stat err = %v", err)
	}
	if _, err := os.Stat(markerPath); err != nil {
		t.Fatalf("restore marker should remain: %v", err)
	}
}

func TestPostgresCLIConnInfo_RewritesURLDSNWithSpecialPassword(t *testing.T) {
	const sensitive = "should-not-appear"
	conn, err := postgresCLIConnInfo("postgres://theia:" + sensitive + "@postgres:5432/theia?sslmode=disable&application_name=theia")
	if err != nil {
		t.Fatalf("postgresCLIConnInfo() error = %v", err)
	}

	connInfo := conn.connInfo
	checks := []string{
		"host='postgres'",
		"port='5432'",
		"user='theia'",
		"dbname='theia'",
		"sslmode='disable'",
		"application_name='theia'",
	}
	for _, want := range checks {
		if !strings.Contains(connInfo, want) {
			t.Fatalf("connInfo = %q, want substring %q", connInfo, want)
		}
	}
	if strings.Contains(connInfo, "postgres://") {
		t.Fatalf("connInfo should not contain raw URL dsn: %q", connInfo)
	}
	if strings.Contains(connInfo, "password") || strings.Contains(connInfo, sensitive) {
		t.Fatalf("connInfo leaked password material: %q", connInfo)
	}
	if commandEnvValue(conn.env, "PGPASSWORD") != sensitive {
		t.Fatal("PGPASSWORD env does not match URL DSN password")
	}
}

func TestPostgresCLIConnInfo_MovesKeywordPasswordToEnvironment(t *testing.T) {
	const sensitive = "should-not-appear"
	conn, err := postgresCLIConnInfo("host=postgres user=theia password='" + sensitive + "' dbname=theia sslmode=disable")
	if err != nil {
		t.Fatalf("postgresCLIConnInfo() error = %v", err)
	}

	connInfo := conn.connInfo
	for _, want := range []string{
		"host='postgres'",
		"user='theia'",
		"dbname='theia'",
		"sslmode='disable'",
	} {
		if !strings.Contains(connInfo, want) {
			t.Fatalf("connInfo = %q, want substring %q", connInfo, want)
		}
	}
	if strings.Contains(connInfo, "password") || strings.Contains(connInfo, sensitive) {
		t.Fatalf("connInfo leaked password material: %q", connInfo)
	}
	if commandEnvValue(conn.env, "PGPASSWORD") != sensitive {
		t.Fatal("PGPASSWORD env does not match keyword DSN password")
	}
}

func assertRestoreStatusFilePhase(t *testing.T, stateDir, wantPhase string, wantAttempts int, wantErrorSubstring string) {
	t.Helper()
	status := readRestoreStatusMap(t, stateDir)
	if got, ok := status["operation_id"].(string); !ok || got == "" {
		t.Fatalf("restore status operation_id = %#v, want non-empty string", status["operation_id"])
	}
	if got, ok := status["phase"].(string); !ok || got != wantPhase {
		t.Fatalf("restore status phase = %#v, want %q", status["phase"], wantPhase)
	}
	if got, ok := status["attempt_count"].(float64); !ok || int(got) != wantAttempts {
		t.Fatalf("restore status attempt_count = %#v, want %d", status["attempt_count"], wantAttempts)
	}
	if got, ok := status["created_at"].(string); !ok || got == "" {
		t.Fatalf("restore status created_at = %#v, want non-empty string", status["created_at"])
	}
	if got, ok := status["updated_at"].(string); !ok || got == "" {
		t.Fatalf("restore status updated_at = %#v, want non-empty string", status["updated_at"])
	}
	lastError, _ := status["last_error"].(string)
	if wantErrorSubstring == "" {
		if lastError != "" {
			t.Fatalf("restore status last_error = %q, want empty", lastError)
		}
		return
	}
	if !strings.Contains(lastError, wantErrorSubstring) {
		t.Fatalf("restore status last_error = %q, want substring %q", lastError, wantErrorSubstring)
	}
}

func readRestoreStatusMap(t *testing.T, stateDir string) map[string]any {
	t.Helper()
	statusPath := filepath.Join(stateDir, ".theia-restore-status.json")
	data, err := os.ReadFile(statusPath)
	if err != nil {
		t.Fatalf("reading restore status file: %v", err)
	}
	var status map[string]any
	if err := json.Unmarshal(data, &status); err != nil {
		t.Fatalf("unmarshal restore status file: %v", err)
	}
	return status
}

func prepareMinimalPendingPostgresRestore(t *testing.T, stateDir, deviceBackupDir, knownHostsPath, stagedDump string) {
	t.Helper()
	stagingDir := filepath.Dir(stagedDump)
	if err := os.MkdirAll(stagingDir, 0o700); err != nil {
		t.Fatalf("creating staging dir: %v", err)
	}
	if err := os.WriteFile(stagedDump, []byte("staged-pg-dump"), 0o600); err != nil {
		t.Fatalf("writing staged dump: %v", err)
	}
	marker := newRestoreMarker(
		stagedDump,
		"",
		"",
		stateDir,
		deviceBackupDir,
		knownHostsPath,
		"2026-04-23T00:00:00Z",
	)
	if err := writeRestoreMarker(restoreMarkerFilePath(stateDir), marker); err != nil {
		t.Fatalf("writing restore marker: %v", err)
	}
}

func stubSuccessfulPendingPostgresRestoreCommands(t *testing.T) {
	t.Helper()
	originalTerminate := terminatePostgresConnections
	terminatePostgresConnections = func(ctx context.Context, dsn string) error { return nil }
	t.Cleanup(func() { terminatePostgresConnections = originalTerminate })

	stubExternalCommandsWithEnv(t, func(ctx context.Context, env []string, name string, args ...string) ([]byte, error) {
		switch name {
		case "pg_dump":
			if commandArgsEqual(args, "--version") {
				return []byte("pg_dump (PostgreSQL) 17.4\n"), nil
			}
			dest := commandFlagValue(args, "--file")
			if dest == "" {
				t.Fatal("pg_dump missing --file argument")
			}
			if err := os.WriteFile(dest, []byte("pre-restore-pg-dump"), 0o600); err != nil {
				t.Fatalf("writing pg_dump destination: %v", err)
			}
			return nil, nil
		case "pg_restore":
			if commandArgsEqual(args, "--version") {
				return []byte("pg_restore (PostgreSQL) 17.4\n"), nil
			}
			return nil, nil
		case "psql":
			if commandArgsEqual(args, "--version") {
				return []byte("psql (PostgreSQL) 17.4\n"), nil
			}
			return nil, nil
		default:
			return nil, fmt.Errorf("unexpected command %s", name)
		}
	})
}
