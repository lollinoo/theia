package service

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	reposqlite "github.com/lollinoo/theia/internal/repository/sqlite"

	_ "github.com/mattn/go-sqlite3"
)

type postgresServiceTestSetup struct {
	db                *sql.DB
	svc               *InstanceBackupService
	instanceBackupDir string
	deviceBackupDir   string
	knownHostsPath    string
	dbPath            string
	encryptionKey     []byte
}

func setupPostgresServiceTest(t *testing.T) *postgresServiceTestSetup {
	t.Helper()

	tmpDir := t.TempDir()
	metadataPath := filepath.Join(tmpDir, "metadata.db")
	db, err := sql.Open("sqlite3", metadataPath)
	if err != nil {
		t.Fatalf("opening metadata db: %v", err)
	}
	if err := reposqlite.RunMigrations(db); err != nil {
		t.Fatalf("running metadata migrations: %v", err)
	}

	repo := reposqlite.NewInstanceBackupRepo(db)
	settingsRepo := reposqlite.NewSettingsRepo(db)
	instanceBackupDir := filepath.Join(tmpDir, "instance-backups")
	deviceBackupDir := filepath.Join(tmpDir, "device-backups")
	knownHostsPath := filepath.Join(tmpDir, "known_hosts")
	dbPath := filepath.Join(tmpDir, "theia.db")
	if err := os.MkdirAll(instanceBackupDir, 0o755); err != nil {
		t.Fatalf("creating instance backup dir: %v", err)
	}
	if err := os.MkdirAll(deviceBackupDir, 0o755); err != nil {
		t.Fatalf("creating device backup dir: %v", err)
	}
	encryptionKey := sha256.Sum256([]byte("postgres-backup-test-key"))

	svc := NewInstanceBackupService(
		db,
		repo,
		settingsRepo,
		instanceBackupDir,
		deviceBackupDir,
		knownHostsPath,
		dbPath,
		"postgres://theia:theia@localhost:5432/theia?sslmode=disable",
		encryptionKey[:],
	)
	svc.dialect = reposqlite.DialectPostgres

	t.Cleanup(func() {
		_ = db.Close()
	})

	return &postgresServiceTestSetup{
		db:                db,
		svc:               svc,
		instanceBackupDir: instanceBackupDir,
		deviceBackupDir:   deviceBackupDir,
		knownHostsPath:    knownHostsPath,
		dbPath:            dbPath,
		encryptionKey:     encryptionKey[:],
	}
}

func stubExternalCommands(t *testing.T, runner func(context.Context, string, ...string) ([]byte, error)) {
	t.Helper()

	originalRunner := runExternalCommand
	originalLookup := lookupExternalCommand
	runExternalCommand = runner
	lookupExternalCommand = func(name string) (string, error) {
		return "/usr/bin/" + name, nil
	}
	t.Cleanup(func() {
		runExternalCommand = originalRunner
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

func TestInstanceBackupServiceCreate_PostgresArchive(t *testing.T) {
	ts := setupPostgresServiceTest(t)

	stubExternalCommands(t, func(ctx context.Context, name string, args ...string) ([]byte, error) {
		switch name {
		case "pg_dump":
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
	if _, ok := entries[sqliteArchiveDBEntry]; ok {
		t.Fatalf("archive unexpectedly contains %s", sqliteArchiveDBEntry)
	}

	var manifest backupManifest
	if err := json.Unmarshal(entries["manifest.json"], &manifest); err != nil {
		t.Fatalf("unmarshal manifest: %v", err)
	}
	if manifest.DBDriver != string(reposqlite.DialectPostgres) {
		t.Fatalf("manifest.DBDriver = %q, want %q", manifest.DBDriver, reposqlite.DialectPostgres)
	}
	if manifest.DBEntryName != postgresArchiveDBEntry {
		t.Fatalf("manifest.DBEntryName = %q, want %q", manifest.DBEntryName, postgresArchiveDBEntry)
	}
	if backup.Status != "success" {
		t.Fatalf("backup.Status = %q, want success", backup.Status)
	}
}

func TestInstanceBackupServiceValidateAndStageRestore_Postgres(t *testing.T) {
	ts := setupPostgresServiceTest(t)

	stubExternalCommands(t, func(ctx context.Context, name string, args ...string) ([]byte, error) {
		if name != "pg_restore" {
			return nil, fmt.Errorf("unexpected command %s", name)
		}
		if len(args) < 2 || args[0] != "--list" {
			return nil, fmt.Errorf("unexpected pg_restore args: %v", args)
		}
		return []byte("archive listing"), nil
	})

	dumpData := []byte("postgres-dump-data")
	dumpHash := sha256.Sum256(dumpData)
	archivePath := filepath.Join(t.TempDir(), "postgres-backup.tar.gz")
	manifest := backupManifest{
		Version:           1,
		AppVersion:        "dev",
		GitCommit:         "test",
		DBDriver:          string(reposqlite.DialectPostgres),
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

	stagingDir := filepath.Join(filepath.Dir(ts.dbPath), ".restore-staging")
	if _, err := os.Stat(filepath.Join(stagingDir, postgresArchiveDBEntry)); err != nil {
		t.Fatalf("staged postgres dump missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(stagingDir, "backups", "router", "config.rsc")); err != nil {
		t.Fatalf("staged backup file missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(stagingDir, "known_hosts")); err != nil {
		t.Fatalf("staged known_hosts missing: %v", err)
	}

	markerBytes, err := os.ReadFile(filepath.Join(filepath.Dir(ts.dbPath), ".theia-restore-pending"))
	if err != nil {
		t.Fatalf("reading restore marker: %v", err)
	}
	var marker restoreMarker
	if err := json.Unmarshal(markerBytes, &marker); err != nil {
		t.Fatalf("unmarshal restore marker: %v", err)
	}
	if marker.DBDriver != string(reposqlite.DialectPostgres) {
		t.Fatalf("marker.DBDriver = %q, want %q", marker.DBDriver, reposqlite.DialectPostgres)
	}
	if !strings.HasSuffix(marker.StagedDB, postgresArchiveDBEntry) {
		t.Fatalf("marker.StagedDB = %q, want suffix %q", marker.StagedDB, postgresArchiveDBEntry)
	}
}

func TestRestoreCoordinatorApplyPendingRestore_Postgres(t *testing.T) {
	runtimeDir := t.TempDir()
	dbPath := filepath.Join(runtimeDir, "theia.db")
	deviceBackupDir := filepath.Join(runtimeDir, "device-backups")
	knownHostsPath := filepath.Join(runtimeDir, "known_hosts")
	stagingDir := filepath.Join(runtimeDir, ".restore-staging")
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

	stubExternalCommands(t, func(ctx context.Context, name string, args ...string) ([]byte, error) {
		switch name {
		case "pg_dump":
			dest := commandFlagValue(args, "--file")
			if dest == "" {
				t.Fatal("pg_dump missing --file argument")
			}
			if err := os.WriteFile(dest, []byte("pre-restore-pg-dump"), 0o600); err != nil {
				t.Fatalf("writing pre-restore dump: %v", err)
			}
			return nil, nil
		case "pg_restore":
			if got := args[len(args)-1]; got != stagedDump {
				t.Fatalf("pg_restore target = %q, want %q", got, stagedDump)
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
		dbPath,
		deviceBackupDir,
		knownHostsPath,
		"2026-04-23T00:00:00Z",
	)
	marker.DBDriver = string(reposqlite.DialectPostgres)
	markerJSON, err := json.Marshal(marker)
	if err != nil {
		t.Fatalf("marshal marker: %v", err)
	}
	if err := os.WriteFile(filepath.Join(runtimeDir, ".theia-restore-pending"), markerJSON, 0o600); err != nil {
		t.Fatalf("write marker: %v", err)
	}

	coordinator := NewRestoreCoordinatorWithDSN(dbPath, "postgres://theia:theia@localhost:5432/theia?sslmode=disable", deviceBackupDir, knownHostsPath)
	applied, err := coordinator.ApplyPendingRestore()
	if err != nil {
		t.Fatalf("ApplyPendingRestore() error = %v", err)
	}
	if !applied {
		t.Fatal("ApplyPendingRestore() applied = false, want true")
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
	if _, err := os.Stat(dbPath + ".pre-restore.dump"); err != nil {
		t.Fatalf("pre-restore dump missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(runtimeDir, ".theia-restore-pending")); !os.IsNotExist(err) {
		t.Fatalf("restore marker should be removed, stat err = %v", err)
	}
	if _, err := os.Stat(stagingDir); !os.IsNotExist(err) {
		t.Fatalf("staging dir should be removed, stat err = %v", err)
	}
}
