package service_test

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lollinoo/theia/internal/domain"
	"github.com/lollinoo/theia/internal/repository/sqlite"
	"github.com/lollinoo/theia/internal/service"

	_ "github.com/mattn/go-sqlite3"
)

// backupManifest mirrors the unexported manifest type for test deserialization.
type backupManifest struct {
	Version           int    `json:"version"`
	AppVersion        string `json:"app_version"`
	GitCommit         string `json:"git_commit"`
	MigrationVersion  int    `json:"migration_version"`
	CreatedAt         string `json:"created_at"`
	DBSHA256          string `json:"db_sha256"`
	BackupFileCount   int    `json:"backup_file_count"`
	TotalSizeBytes    int64  `json:"total_size_bytes"`
	EncryptionKeyHash string `json:"encryption_key_hash"`
}

// testSetup holds all resources for an instance backup service test.
type testSetup struct {
	db                *sql.DB
	repo              *sqlite.InstanceBackupRepo
	settingsRepo      *sqlite.SettingsRepo
	svc               *service.InstanceBackupService
	tmpDir            string
	dbPath            string
	instanceBackupDir string
	deviceBackupDir   string
	knownHostsPath    string
	encryptionKey     []byte
}

func setupInstanceBackupTest(t *testing.T) *testSetup {
	t.Helper()

	tmpDir := t.TempDir()

	// VACUUM INTO requires a file-based source database
	dbPath := filepath.Join(tmpDir, "test.db")
	db, err := sql.Open("sqlite3", dbPath+"?_journal_mode=WAL&_busy_timeout=5000&_foreign_keys=on")
	if err != nil {
		t.Fatalf("opening test db: %v", err)
	}

	if err := sqlite.RunMigrations(db); err != nil {
		t.Fatalf("running migrations: %v", err)
	}

	repo := sqlite.NewInstanceBackupRepo(db)
	settingsRepo := sqlite.NewSettingsRepo(db)

	instanceBackupDir := filepath.Join(tmpDir, "instance-backups")
	deviceBackupDir := filepath.Join(tmpDir, "device-backups")
	knownHostsPath := filepath.Join(tmpDir, "known_hosts")

	if err := os.MkdirAll(instanceBackupDir, 0755); err != nil {
		t.Fatalf("creating instance backup dir: %v", err)
	}
	if err := os.MkdirAll(deviceBackupDir, 0755); err != nil {
		t.Fatalf("creating device backup dir: %v", err)
	}

	encryptionKey := sha256.Sum256([]byte("test-encryption-key"))

	svc := service.NewInstanceBackupService(
		db,
		repo,
		settingsRepo,
		instanceBackupDir,
		deviceBackupDir,
		knownHostsPath,
		dbPath,
		"",
		encryptionKey[:],
	)

	t.Cleanup(func() { db.Close() })

	return &testSetup{
		db:                db,
		repo:              repo,
		settingsRepo:      settingsRepo,
		svc:               svc,
		tmpDir:            tmpDir,
		dbPath:            dbPath,
		instanceBackupDir: instanceBackupDir,
		deviceBackupDir:   deviceBackupDir,
		knownHostsPath:    knownHostsPath,
		encryptionKey:     encryptionKey[:],
	}
}

// readArchiveEntries reads a .tar.gz file and returns a map of entry names to their content.
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
	entries := make(map[string][]byte)
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
			t.Fatalf("reading entry %s data: %v", hdr.Name, err)
		}
		entries[hdr.Name] = data
	}
	return entries
}

// computeSHA256 computes the SHA-256 hash of a file using streaming.
func computeSHA256(t *testing.T, path string) string {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("opening file for hash: %v", err)
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		t.Fatalf("hashing file: %v", err)
	}
	return hex.EncodeToString(h.Sum(nil))
}

// createTestArchive builds a .tar.gz archive with a manifest and a copy of the given DB.
// The manifest encryption_key_hash is computed from the given key.
// Returns the path to the created archive.
func createTestArchive(t *testing.T, dbPath string, encryptionKey []byte, overrides map[string]interface{}) string {
	t.Helper()

	// Read the real DB for the archive
	dbData, err := os.ReadFile(dbPath)
	if err != nil {
		t.Fatalf("reading db for test archive: %v", err)
	}
	dbHash := sha256.Sum256(dbData)
	dbHashStr := hex.EncodeToString(dbHash[:])

	// Read migration version from the DB
	db, err := sql.Open("sqlite3", dbPath+"?mode=ro")
	if err != nil {
		t.Fatalf("opening db for migration version: %v", err)
	}
	var migVer int
	if err := db.QueryRow("SELECT version FROM schema_migrations").Scan(&migVer); err != nil {
		t.Fatalf("reading migration version: %v", err)
	}
	db.Close()

	// Compute encryption key hash (SHA-256 of first 8 bytes)
	keyPrefix := encryptionKey[:8]
	keyHash := sha256.Sum256(keyPrefix)
	keyHashStr := hex.EncodeToString(keyHash[:])

	manifest := map[string]interface{}{
		"version":             1,
		"app_version":         "dev",
		"git_commit":          "test",
		"migration_version":   migVer,
		"created_at":          "2026-04-05T00:00:00Z",
		"db_sha256":           dbHashStr,
		"backup_file_count":   0,
		"total_size_bytes":    int64(len(dbData)),
		"encryption_key_hash": keyHashStr,
	}

	// Apply overrides
	for k, v := range overrides {
		manifest[k] = v
	}

	manifestJSON, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		t.Fatalf("marshaling manifest: %v", err)
	}

	archivePath := filepath.Join(t.TempDir(), "test-backup.tar.gz")
	f, err := os.Create(archivePath)
	if err != nil {
		t.Fatalf("creating archive: %v", err)
	}
	defer f.Close()

	gw := gzip.NewWriter(f)
	defer gw.Close()

	tw := tar.NewWriter(gw)
	defer tw.Close()

	// Add manifest.json
	if err := tw.WriteHeader(&tar.Header{
		Name: "manifest.json",
		Size: int64(len(manifestJSON)),
		Mode: 0644,
	}); err != nil {
		t.Fatalf("writing manifest header: %v", err)
	}
	if _, err := tw.Write(manifestJSON); err != nil {
		t.Fatalf("writing manifest data: %v", err)
	}

	// Add theia.db (use the real test DB data, or overridden data)
	if err := tw.WriteHeader(&tar.Header{
		Name: "theia.db",
		Size: int64(len(dbData)),
		Mode: 0644,
	}); err != nil {
		t.Fatalf("writing db header: %v", err)
	}
	if _, err := tw.Write(dbData); err != nil {
		t.Fatalf("writing db data: %v", err)
	}

	return archivePath
}

// createTestArchiveWithCorruptDB builds an archive where the DB content does not match db_sha256.
func createTestArchiveWithCorruptDB(t *testing.T, dbPath string, encryptionKey []byte) string {
	t.Helper()

	// Use correct hash in manifest but put wrong data in the DB entry
	dbData, err := os.ReadFile(dbPath)
	if err != nil {
		t.Fatalf("reading db: %v", err)
	}
	dbHash := sha256.Sum256(dbData)
	dbHashStr := hex.EncodeToString(dbHash[:])

	db, err := sql.Open("sqlite3", dbPath+"?mode=ro")
	if err != nil {
		t.Fatalf("opening db: %v", err)
	}
	var migVer int
	if err := db.QueryRow("SELECT version FROM schema_migrations").Scan(&migVer); err != nil {
		t.Fatalf("reading migration version: %v", err)
	}
	db.Close()

	keyPrefix := encryptionKey[:8]
	keyHash := sha256.Sum256(keyPrefix)
	keyHashStr := hex.EncodeToString(keyHash[:])

	manifest := map[string]interface{}{
		"version":             1,
		"app_version":         "dev",
		"git_commit":          "test",
		"migration_version":   migVer,
		"created_at":          "2026-04-05T00:00:00Z",
		"db_sha256":           dbHashStr,
		"backup_file_count":   0,
		"total_size_bytes":    int64(len(dbData)),
		"encryption_key_hash": keyHashStr,
	}

	manifestJSON, _ := json.MarshalIndent(manifest, "", "  ")

	// Corrupt the DB data
	corruptData := []byte("this is not a valid sqlite database")

	archivePath := filepath.Join(t.TempDir(), "corrupt-backup.tar.gz")
	f, err := os.Create(archivePath)
	if err != nil {
		t.Fatalf("creating archive: %v", err)
	}
	defer f.Close()

	gw := gzip.NewWriter(f)
	defer gw.Close()

	tw := tar.NewWriter(gw)
	defer tw.Close()

	tw.WriteHeader(&tar.Header{Name: "manifest.json", Size: int64(len(manifestJSON)), Mode: 0644})
	tw.Write(manifestJSON)
	tw.WriteHeader(&tar.Header{Name: "theia.db", Size: int64(len(corruptData)), Mode: 0644})
	tw.Write(corruptData)

	return archivePath
}

func addFileToTestArchive(archivePath, name string, data []byte) error {
	return addTarEntryToTestArchive(archivePath, &tar.Header{Name: name, Mode: 0644, Size: int64(len(data))}, data)
}

func addTarEntryToTestArchive(archivePath string, newHeader *tar.Header, data []byte) error {
	original, err := os.ReadFile(archivePath)
	if err != nil {
		return err
	}

	tmpPath := archivePath + ".tmp"
	f, err := os.Create(tmpPath)
	if err != nil {
		return err
	}

	gzWriter := gzip.NewWriter(f)
	tarWriter := tar.NewWriter(gzWriter)

	reader, err := gzip.NewReader(bytes.NewReader(original))
	if err != nil {
		_ = tarWriter.Close()
		_ = gzWriter.Close()
		_ = f.Close()
		_ = os.Remove(tmpPath)
		return err
	}
	tarReader := tar.NewReader(reader)
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			_ = reader.Close()
			_ = tarWriter.Close()
			_ = gzWriter.Close()
			_ = f.Close()
			_ = os.Remove(tmpPath)
			return err
		}
		payload, err := io.ReadAll(tarReader)
		if err != nil {
			_ = reader.Close()
			_ = tarWriter.Close()
			_ = gzWriter.Close()
			_ = f.Close()
			_ = os.Remove(tmpPath)
			return err
		}
		if err := tarWriter.WriteHeader(header); err != nil {
			_ = reader.Close()
			_ = tarWriter.Close()
			_ = gzWriter.Close()
			_ = f.Close()
			_ = os.Remove(tmpPath)
			return err
		}
		if _, err := tarWriter.Write(payload); err != nil {
			_ = reader.Close()
			_ = tarWriter.Close()
			_ = gzWriter.Close()
			_ = f.Close()
			_ = os.Remove(tmpPath)
			return err
		}
	}
	if err := reader.Close(); err != nil {
		_ = tarWriter.Close()
		_ = gzWriter.Close()
		_ = f.Close()
		_ = os.Remove(tmpPath)
		return err
	}

	if err := tarWriter.WriteHeader(newHeader); err != nil {
		_ = tarWriter.Close()
		_ = gzWriter.Close()
		_ = f.Close()
		_ = os.Remove(tmpPath)
		return err
	}
	if _, err := tarWriter.Write(data); err != nil {
		_ = tarWriter.Close()
		_ = gzWriter.Close()
		_ = f.Close()
		_ = os.Remove(tmpPath)
		return err
	}
	if err := tarWriter.Close(); err != nil {
		_ = gzWriter.Close()
		_ = f.Close()
		_ = os.Remove(tmpPath)
		return err
	}
	if err := gzWriter.Close(); err != nil {
		_ = f.Close()
		_ = os.Remove(tmpPath)
		return err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}

	if err := os.Rename(tmpPath, archivePath); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}

	return nil
}

func requireRestoreLimitError(t *testing.T, err error) {
	t.Helper()
	var limitErr *service.RestoreLimitError
	if !errors.As(err, &limitErr) {
		t.Fatalf("error = %v, want RestoreLimitError", err)
	}
}

func requireDisallowedRestoreArchiveEntry(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("expected disallowed archive entry error")
	}
	if !strings.Contains(err.Error(), "disallowed restore archive entry") {
		t.Fatalf("error = %q, want disallowed restore archive entry", err.Error())
	}
}

func TestValidateAndStageRestore(t *testing.T) {
	t.Run("valid archive returns RestoreReport with Valid=true", func(t *testing.T) {
		ts := setupInstanceBackupTest(t)

		archivePath := createTestArchive(t, ts.dbPath, ts.encryptionKey, nil)
		report, err := ts.svc.ValidateAndStageRestore(archivePath, true)
		if err != nil {
			t.Fatalf("ValidateAndStageRestore: %v", err)
		}
		if !report.Valid {
			t.Error("expected Valid=true")
		}
		if report.AppVersion == "" {
			t.Error("AppVersion is empty")
		}
		if report.MigrationVersion == 0 {
			t.Error("MigrationVersion is 0")
		}
		if report.CreatedAt == "" {
			t.Error("CreatedAt is empty")
		}
	})

	t.Run("dryRun=true does not create staging or marker", func(t *testing.T) {
		ts := setupInstanceBackupTest(t)

		archivePath := createTestArchive(t, ts.dbPath, ts.encryptionKey, nil)
		_, err := ts.svc.ValidateAndStageRestore(archivePath, true)
		if err != nil {
			t.Fatalf("ValidateAndStageRestore: %v", err)
		}

		stagingDir := filepath.Join(filepath.Dir(ts.dbPath), ".restore-staging")
		if _, err := os.Stat(stagingDir); !os.IsNotExist(err) {
			t.Error("staging dir should not exist after dry run")
		}
		markerPath := filepath.Join(filepath.Dir(ts.dbPath), ".theia-restore-pending")
		if _, err := os.Stat(markerPath); !os.IsNotExist(err) {
			t.Error("marker file should not exist after dry run")
		}
	})

	t.Run("wrong encryption key returns error", func(t *testing.T) {
		ts := setupInstanceBackupTest(t)

		// Create archive with a different encryption key
		wrongKey := sha256.Sum256([]byte("wrong-key"))
		archivePath := createTestArchive(t, ts.dbPath, wrongKey[:], nil)

		_, err := ts.svc.ValidateAndStageRestore(archivePath, true)
		if err == nil {
			t.Fatal("expected error for wrong encryption key")
		}
		if !strings.Contains(err.Error(), "encryption key mismatch") {
			t.Errorf("error = %q, want to contain 'encryption key mismatch'", err.Error())
		}
	})

	t.Run("corrupted DB returns checksum mismatch error", func(t *testing.T) {
		ts := setupInstanceBackupTest(t)

		archivePath := createTestArchiveWithCorruptDB(t, ts.dbPath, ts.encryptionKey)

		_, err := ts.svc.ValidateAndStageRestore(archivePath, true)
		if err == nil {
			t.Fatal("expected error for corrupted DB")
		}
		if !strings.Contains(err.Error(), "database checksum mismatch") {
			t.Errorf("error = %q, want to contain 'database checksum mismatch'", err.Error())
		}
	})

	t.Run("dryRun=false creates staging dir and marker file", func(t *testing.T) {
		ts := setupInstanceBackupTest(t)

		archivePath := createTestArchive(t, ts.dbPath, ts.encryptionKey, nil)
		report, err := ts.svc.ValidateAndStageRestore(archivePath, false)
		if err != nil {
			t.Fatalf("ValidateAndStageRestore: %v", err)
		}
		if !report.Valid {
			t.Error("expected Valid=true")
		}

		stagingDir := filepath.Join(filepath.Dir(ts.dbPath), ".restore-staging")
		if _, err := os.Stat(stagingDir); os.IsNotExist(err) {
			t.Error("staging dir should exist after non-dry-run")
		}
		// Verify staged DB exists
		if _, err := os.Stat(filepath.Join(stagingDir, "theia.db")); os.IsNotExist(err) {
			t.Error("staged theia.db should exist")
		}

		markerPath := filepath.Join(filepath.Dir(ts.dbPath), ".theia-restore-pending")
		if _, err := os.Stat(markerPath); os.IsNotExist(err) {
			t.Error("marker file should exist after non-dry-run")
		}

		// Verify marker content is valid JSON
		markerData, err := os.ReadFile(markerPath)
		if err != nil {
			t.Fatalf("reading marker: %v", err)
		}
		var markerJSON map[string]interface{}
		if err := json.Unmarshal(markerData, &markerJSON); err != nil {
			t.Fatalf("parsing marker JSON: %v", err)
		}
		if _, ok := markerJSON["staged_db"]; !ok {
			t.Error("marker missing staged_db field")
		}
	})

	t.Run("staged marker is applied by restore coordinator without translation", func(t *testing.T) {
		ts := setupInstanceBackupTest(t)

		archivePath := createTestArchive(t, ts.dbPath, ts.encryptionKey, nil)
		if err := addFileToTestArchive(archivePath, "backups/router/backup.rsc", []byte("staged-backup")); err != nil {
			t.Fatalf("adding staged backup to archive: %v", err)
		}
		if err := addFileToTestArchive(archivePath, "known_hosts", []byte("staged-known-hosts")); err != nil {
			t.Fatalf("adding staged known_hosts to archive: %v", err)
		}

		deviceFile := filepath.Join(ts.deviceBackupDir, "router", "backup.rsc")
		if err := os.MkdirAll(filepath.Dir(deviceFile), 0755); err != nil {
			t.Fatalf("creating live device dir: %v", err)
		}
		if err := os.WriteFile(deviceFile, []byte("live-backup"), 0644); err != nil {
			t.Fatalf("writing live backup: %v", err)
		}
		if err := os.WriteFile(ts.knownHostsPath, []byte("live-known-hosts"), 0644); err != nil {
			t.Fatalf("writing live known_hosts: %v", err)
		}

		report, err := ts.svc.ValidateAndStageRestore(archivePath, false)
		if err != nil {
			t.Fatalf("ValidateAndStageRestore: %v", err)
		}
		if !report.Valid {
			t.Fatal("expected Valid=true")
		}

		if err := os.WriteFile(ts.dbPath+"-wal", []byte("live-wal"), 0644); err != nil {
			t.Fatalf("writing live wal: %v", err)
		}
		if err := os.WriteFile(ts.dbPath+"-shm", []byte("live-shm"), 0644); err != nil {
			t.Fatalf("writing live shm: %v", err)
		}

		coordinator := service.NewRestoreCoordinator(ts.dbPath, ts.deviceBackupDir, ts.knownHostsPath)
		applied, err := coordinator.ApplyPendingRestore()
		if err != nil {
			t.Fatalf("ApplyPendingRestore: %v", err)
		}
		if !applied {
			t.Fatal("expected ApplyPendingRestore to apply staged restore")
		}

		dbBytes, err := os.ReadFile(ts.dbPath)
		if err != nil {
			t.Fatalf("reading applied db: %v", err)
		}
		archiveEntries := readArchiveEntries(t, archivePath)
		if string(dbBytes) != string(archiveEntries["theia.db"]) {
			t.Fatal("applied db content does not match staged archive db")
		}
		backupBytes, err := os.ReadFile(deviceFile)
		if err != nil {
			t.Fatalf("reading applied backup: %v", err)
		}
		if string(backupBytes) != "staged-backup" {
			t.Fatalf("applied backup = %q, want staged-backup", string(backupBytes))
		}
		knownHostsBytes, err := os.ReadFile(ts.knownHostsPath)
		if err != nil {
			t.Fatalf("reading applied known_hosts: %v", err)
		}
		if string(knownHostsBytes) != "staged-known-hosts" {
			t.Fatalf("applied known_hosts = %q, want staged-known-hosts", string(knownHostsBytes))
		}
		if _, err := os.Stat(filepath.Join(filepath.Dir(ts.dbPath), ".theia-restore-pending")); !os.IsNotExist(err) {
			t.Fatalf("marker file should be removed after apply, stat err = %v", err)
		}
		if _, err := os.Stat(filepath.Join(filepath.Dir(ts.dbPath), ".restore-staging")); !os.IsNotExist(err) {
			t.Fatalf("staging dir should be removed after apply, stat err = %v", err)
		}
	})

	t.Run("archive with newer migration version returns error", func(t *testing.T) {
		ts := setupInstanceBackupTest(t)

		// Create archive with migration version far in the future
		archivePath := createTestArchive(t, ts.dbPath, ts.encryptionKey, map[string]interface{}{
			"migration_version": 99999,
		})

		_, err := ts.svc.ValidateAndStageRestore(archivePath, true)
		if err == nil {
			t.Fatal("expected error for newer migration version")
		}
		if !strings.Contains(err.Error(), "newer migration version") {
			t.Errorf("error = %q, want to contain 'newer migration version'", err.Error())
		}
	})

	t.Run("SC-3: older archive triggers migration and stages successfully", func(t *testing.T) {
		ts := setupInstanceBackupTest(t)

		// Read current migration version from the live DB
		liveDB, err := sql.Open("sqlite3", ts.dbPath+"?mode=ro")
		if err != nil {
			t.Fatalf("opening live db: %v", err)
		}
		var currentVersion int
		if err := liveDB.QueryRow("SELECT version FROM schema_migrations").Scan(&currentVersion); err != nil {
			t.Fatalf("reading current migration version: %v", err)
		}
		liveDB.Close()

		if currentVersion < 2 {
			t.Skip("current migration version too low to test older-archive path")
		}

		// Build an archive whose manifest reports an older migration version.
		// The archive DB itself is already fully migrated — RunMigrations will
		// treat it as ErrNoChange (idempotent).
		olderVersion := currentVersion - 1
		archivePath := createTestArchive(t, ts.dbPath, ts.encryptionKey, map[string]interface{}{
			"migration_version": olderVersion,
		})

		// dryRun=true: NeedsMigration should be reported but no staging occurs
		dryReport, err := ts.svc.ValidateAndStageRestore(archivePath, true)
		if err != nil {
			t.Fatalf("ValidateAndStageRestore(dryRun=true): %v", err)
		}
		if !dryReport.NeedsMigration {
			t.Errorf("expected NeedsMigration=true for archive at version %d vs current %d",
				olderVersion, currentVersion)
		}
		if dryReport.MigrationVersion != olderVersion {
			t.Errorf("MigrationVersion = %d, want %d", dryReport.MigrationVersion, olderVersion)
		}

		// Staging dir must NOT exist after dry run
		stagingDir := filepath.Join(filepath.Dir(ts.dbPath), ".restore-staging")
		if _, err := os.Stat(stagingDir); !os.IsNotExist(err) {
			t.Error("staging dir must not exist after dry run")
		}

		// dryRun=false: RunMigrations is called on extracted DB then files are staged
		report, err := ts.svc.ValidateAndStageRestore(archivePath, false)
		if err != nil {
			t.Fatalf("ValidateAndStageRestore(dryRun=false): %v", err)
		}
		if !report.Valid {
			t.Error("expected Valid=true after successful migration and staging")
		}
		if !report.NeedsMigration {
			t.Errorf("report.NeedsMigration should be true for older archive")
		}

		// Staged DB must exist
		if _, err := os.Stat(filepath.Join(stagingDir, "theia.db")); os.IsNotExist(err) {
			t.Error("staged theia.db must exist after non-dry-run with migration")
		}

		// Marker file must exist
		markerPath := filepath.Join(filepath.Dir(ts.dbPath), ".theia-restore-pending")
		if _, err := os.Stat(markerPath); os.IsNotExist(err) {
			t.Error("marker file must exist after non-dry-run")
		}
	})
}

func TestValidateAndStageRestoreWritesPrivateMarkerAndStagingTree(t *testing.T) {
	ts := setupInstanceBackupTest(t)

	archivePath := createTestArchive(t, ts.dbPath, ts.encryptionKey, nil)
	if err := addFileToTestArchive(archivePath, "known_hosts", []byte("example-host ssh-ed25519 AAAA\n")); err != nil {
		t.Fatalf("adding known_hosts to archive: %v", err)
	}

	if _, err := ts.svc.ValidateAndStageRestore(archivePath, false); err != nil {
		t.Fatalf("ValidateAndStageRestore: %v", err)
	}

	stagingDir := filepath.Join(filepath.Dir(ts.dbPath), ".restore-staging")
	assertPathMode(t, stagingDir, 0700)
	assertPathMode(t, filepath.Join(stagingDir, "theia.db"), 0600)
	assertPathMode(t, filepath.Join(stagingDir, "known_hosts"), 0600)
	assertPathMode(t, filepath.Join(filepath.Dir(ts.dbPath), ".theia-restore-pending"), 0600)
}

func TestValidateAndStageRestoreTightensExistingMarkerPermissions(t *testing.T) {
	ts := setupInstanceBackupTest(t)

	archivePath := createTestArchive(t, ts.dbPath, ts.encryptionKey, nil)
	markerPath := filepath.Join(filepath.Dir(ts.dbPath), ".theia-restore-pending")
	if err := os.WriteFile(markerPath, []byte("old marker"), 0o644); err != nil {
		t.Fatalf("WriteFile(existing marker): %v", err)
	}

	if _, err := ts.svc.ValidateAndStageRestore(archivePath, false); err != nil {
		t.Fatalf("ValidateAndStageRestore: %v", err)
	}

	assertPathMode(t, markerPath, 0600)
}

func TestValidateAndStageRestoreRejectsUnsafeArchiveEntries(t *testing.T) {
	t.Run("path traversal entry", func(t *testing.T) {
		ts := setupInstanceBackupTest(t)
		archivePath := createTestArchive(t, ts.dbPath, ts.encryptionKey, nil)
		if err := addFileToTestArchive(archivePath, "backups/../evil.txt", []byte("evil")); err != nil {
			t.Fatalf("adding traversal entry: %v", err)
		}

		_, err := ts.svc.ValidateAndStageRestore(archivePath, true)

		if err == nil {
			t.Fatal("expected traversal error")
		}
		if !strings.Contains(err.Error(), "path traversal") {
			t.Fatalf("error = %q, want path traversal error", err.Error())
		}
	})

	t.Run("absolute path entry", func(t *testing.T) {
		ts := setupInstanceBackupTest(t)
		archivePath := createTestArchive(t, ts.dbPath, ts.encryptionKey, nil)
		if err := addFileToTestArchive(archivePath, "/tmp/evil.txt", []byte("evil")); err != nil {
			t.Fatalf("adding absolute entry: %v", err)
		}

		_, err := ts.svc.ValidateAndStageRestore(archivePath, true)

		if err == nil {
			t.Fatal("expected absolute path error")
		}
		if !strings.Contains(err.Error(), "absolute path") {
			t.Fatalf("error = %q, want absolute path error", err.Error())
		}
	})

	t.Run("symlink and hardlink entries", func(t *testing.T) {
		for _, tc := range []struct {
			name     string
			typeflag byte
		}{
			{name: "symlink", typeflag: tar.TypeSymlink},
			{name: "hardlink", typeflag: tar.TypeLink},
		} {
			t.Run(tc.name, func(t *testing.T) {
				ts := setupInstanceBackupTest(t)
				archivePath := createTestArchive(t, ts.dbPath, ts.encryptionKey, nil)
				err := addTarEntryToTestArchive(archivePath, &tar.Header{
					Name:     "backups/router/link",
					Typeflag: tc.typeflag,
					Linkname: "manifest.json",
					Mode:     0644,
				}, nil)
				if err != nil {
					t.Fatalf("adding link entry: %v", err)
				}

				_, err = ts.svc.ValidateAndStageRestore(archivePath, true)

				if err == nil {
					t.Fatal("expected link entry error")
				}
				if !strings.Contains(err.Error(), "disallowed link entry") {
					t.Fatalf("error = %q, want disallowed link entry error", err.Error())
				}
			})
		}
	})

	t.Run("unsupported fifo entry", func(t *testing.T) {
		ts := setupInstanceBackupTest(t)
		archivePath := createTestArchive(t, ts.dbPath, ts.encryptionKey, nil)
		err := addTarEntryToTestArchive(archivePath, &tar.Header{
			Name:     "backups/router/fifo",
			Typeflag: tar.TypeFifo,
			Mode:     0600,
		}, nil)
		if err != nil {
			t.Fatalf("adding fifo entry: %v", err)
		}

		_, err = ts.svc.ValidateAndStageRestore(archivePath, true)

		if err == nil {
			t.Fatal("expected unsupported entry type error")
		}
		if !strings.Contains(err.Error(), "unsupported restore archive entry type") {
			t.Fatalf("error = %q, want unsupported entry type error", err.Error())
		}
	})
}

func TestValidateAndStageRestoreRejectsSingletonPrefixArchiveEntries(t *testing.T) {
	for _, entryName := range []string{
		"known_hosts/child",
		"known_hosts_extra",
		"theia.db.bak",
		"manifest.json.bak",
	} {
		t.Run(entryName, func(t *testing.T) {
			ts := setupInstanceBackupTest(t)
			archivePath := createTestArchive(t, ts.dbPath, ts.encryptionKey, nil)
			if err := addFileToTestArchive(archivePath, entryName, []byte("unexpected")); err != nil {
				t.Fatalf("adding singleton-prefix entry: %v", err)
			}

			_, err := ts.svc.ValidateAndStageRestore(archivePath, true)

			requireDisallowedRestoreArchiveEntry(t, err)
		})
	}
}

func TestValidateAndStageRestoreRejectsTypeMismatchedArchiveEntries(t *testing.T) {
	t.Run("regular file named backups", func(t *testing.T) {
		ts := setupInstanceBackupTest(t)
		archivePath := createTestArchive(t, ts.dbPath, ts.encryptionKey, nil)
		if err := addFileToTestArchive(archivePath, "backups", []byte("not a directory")); err != nil {
			t.Fatalf("adding backups file entry: %v", err)
		}

		_, err := ts.svc.ValidateAndStageRestore(archivePath, true)

		requireDisallowedRestoreArchiveEntry(t, err)
	})

	for _, entryName := range []string{
		"known_hosts/",
		"database.dump/",
	} {
		t.Run("directory "+entryName, func(t *testing.T) {
			ts := setupInstanceBackupTest(t)
			archivePath := createTestArchive(t, ts.dbPath, ts.encryptionKey, nil)
			err := addTarEntryToTestArchive(archivePath, &tar.Header{
				Name:     entryName,
				Typeflag: tar.TypeDir,
				Mode:     0700,
			}, nil)
			if err != nil {
				t.Fatalf("adding singleton directory entry: %v", err)
			}

			_, err = ts.svc.ValidateAndStageRestore(archivePath, true)

			requireDisallowedRestoreArchiveEntry(t, err)
		})
	}
}

func TestValidateAndStageRestoreRejectsArchiveQuotaViolations(t *testing.T) {
	t.Run("compressed size limit", func(t *testing.T) {
		ts := setupInstanceBackupTest(t)
		archivePath := createTestArchive(t, ts.dbPath, ts.encryptionKey, nil)
		info, err := os.Stat(archivePath)
		if err != nil {
			t.Fatalf("stat archive: %v", err)
		}
		limits := service.DefaultRestoreArchiveLimits
		limits.MaxCompressedBytes = info.Size() - 1
		ts.svc.SetRestoreArchiveLimitsForTest(limits)

		_, err = ts.svc.ValidateAndStageRestore(archivePath, true)

		if err == nil {
			t.Fatal("expected compressed quota error")
		}
		requireRestoreLimitError(t, err)
		if !strings.Contains(err.Error(), "compressed archive exceeds") {
			t.Fatalf("error = %q, want compressed archive quota error", err.Error())
		}
	})

	t.Run("per-entry expanded size limit", func(t *testing.T) {
		ts := setupInstanceBackupTest(t)
		archivePath := createTestArchive(t, ts.dbPath, ts.encryptionKey, nil)
		dbInfo, err := os.Stat(ts.dbPath)
		if err != nil {
			t.Fatalf("stat db: %v", err)
		}
		largeBackup := bytes.Repeat([]byte("x"), int(dbInfo.Size()+1))
		if err := addFileToTestArchive(archivePath, "backups/router/large.rsc", largeBackup); err != nil {
			t.Fatalf("adding backup entry: %v", err)
		}
		limits := service.DefaultRestoreArchiveLimits
		limits.MaxEntryBytes = dbInfo.Size()
		ts.svc.SetRestoreArchiveLimitsForTest(limits)

		_, err = ts.svc.ValidateAndStageRestore(archivePath, true)

		if err == nil {
			t.Fatal("expected per-entry quota error")
		}
		requireRestoreLimitError(t, err)
		if !strings.Contains(err.Error(), "archive entry backups/router/large.rsc exceeds") {
			t.Fatalf("error = %q, want per-entry quota error", err.Error())
		}
	})

	t.Run("total expanded size limit", func(t *testing.T) {
		ts := setupInstanceBackupTest(t)
		archivePath := createTestArchive(t, ts.dbPath, ts.encryptionKey, nil)
		baseEntries := readArchiveEntries(t, archivePath)
		var baseTotal int64
		for _, data := range baseEntries {
			baseTotal += int64(len(data))
		}
		if err := addFileToTestArchive(archivePath, "backups/router/a.rsc", bytes.Repeat([]byte("a"), 8)); err != nil {
			t.Fatalf("adding first backup entry: %v", err)
		}
		if err := addFileToTestArchive(archivePath, "backups/router/b.rsc", bytes.Repeat([]byte("b"), 8)); err != nil {
			t.Fatalf("adding second backup entry: %v", err)
		}
		limits := service.DefaultRestoreArchiveLimits
		limits.MaxTotalBytes = baseTotal + 8
		ts.svc.SetRestoreArchiveLimitsForTest(limits)

		_, err := ts.svc.ValidateAndStageRestore(archivePath, true)

		if err == nil {
			t.Fatal("expected total expanded quota error")
		}
		requireRestoreLimitError(t, err)
		if !strings.Contains(err.Error(), "expanded archive exceeds") {
			t.Fatalf("error = %q, want total expanded quota error", err.Error())
		}
	})

	t.Run("file count limit", func(t *testing.T) {
		ts := setupInstanceBackupTest(t)
		archivePath := createTestArchive(t, ts.dbPath, ts.encryptionKey, nil)
		baseEntries := readArchiveEntries(t, archivePath)
		if err := addFileToTestArchive(archivePath, "backups/router/a.rsc", []byte("a")); err != nil {
			t.Fatalf("adding first backup entry: %v", err)
		}
		if err := addFileToTestArchive(archivePath, "backups/router/b.rsc", []byte("b")); err != nil {
			t.Fatalf("adding second backup entry: %v", err)
		}
		limits := service.DefaultRestoreArchiveLimits
		limits.MaxFileEntries = len(baseEntries) + 1
		ts.svc.SetRestoreArchiveLimitsForTest(limits)

		_, err := ts.svc.ValidateAndStageRestore(archivePath, true)

		if err == nil {
			t.Fatal("expected file count quota error")
		}
		requireRestoreLimitError(t, err)
		if !strings.Contains(err.Error(), "archive file count exceeds") {
			t.Fatalf("error = %q, want file count quota error", err.Error())
		}
	})

	t.Run("directory entry count limit", func(t *testing.T) {
		ts := setupInstanceBackupTest(t)
		archivePath := createTestArchive(t, ts.dbPath, ts.encryptionKey, nil)
		baseEntries := readArchiveEntries(t, archivePath)
		for _, name := range []string{"backups/router", "backups/router/nested"} {
			err := addTarEntryToTestArchive(archivePath, &tar.Header{
				Name:     name,
				Typeflag: tar.TypeDir,
				Mode:     0700,
			}, nil)
			if err != nil {
				t.Fatalf("adding directory entry %s: %v", name, err)
			}
		}
		limits := service.DefaultRestoreArchiveLimits
		limits.MaxFileEntries = len(baseEntries) + 1
		ts.svc.SetRestoreArchiveLimitsForTest(limits)

		_, err := ts.svc.ValidateAndStageRestore(archivePath, true)

		if err == nil {
			t.Fatal("expected directory entry count quota error")
		}
		requireRestoreLimitError(t, err)
		if !strings.Contains(err.Error(), "archive entry count exceeds") {
			t.Fatalf("error = %q, want entry count quota error", err.Error())
		}
	})

	t.Run("unknown regular file size cannot bypass quota", func(t *testing.T) {
		ts := setupInstanceBackupTest(t)
		archivePath := createTestArchive(t, ts.dbPath, ts.encryptionKey, nil)
		dbInfo, err := os.Stat(ts.dbPath)
		if err != nil {
			t.Fatalf("stat db: %v", err)
		}
		unknownPayload := bytes.Repeat([]byte("u"), int(dbInfo.Size()+1))
		if err := addFileToTestArchive(archivePath, "unexpected.bin", unknownPayload); err != nil {
			t.Fatalf("adding unknown entry: %v", err)
		}
		limits := service.DefaultRestoreArchiveLimits
		limits.MaxEntryBytes = dbInfo.Size()
		ts.svc.SetRestoreArchiveLimitsForTest(limits)

		_, err = ts.svc.ValidateAndStageRestore(archivePath, true)

		if err == nil {
			t.Fatal("expected unknown entry quota error")
		}
		requireRestoreLimitError(t, err)
		if !strings.Contains(err.Error(), "archive entry unexpected.bin exceeds") {
			t.Fatalf("error = %q, want unknown entry quota error", err.Error())
		}
	})
}

func assertPathMode(t *testing.T, path string, want os.FileMode) {
	t.Helper()

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	if got := info.Mode().Perm(); got != want {
		t.Fatalf("mode for %s = %04o, want %04o", path, got, want)
	}
}

func TestInstanceBackupService(t *testing.T) {
	t.Run("Create produces tar.gz at expected path", func(t *testing.T) {
		ts := setupInstanceBackupTest(t)

		ctx := t.Context()
		backup, err := ts.svc.Create(ctx)
		if err != nil {
			t.Fatalf("Create: %v", err)
		}

		if backup == nil {
			t.Fatal("Create returned nil backup")
		}

		// File should exist
		if _, err := os.Stat(backup.FilePath); os.IsNotExist(err) {
			t.Fatalf("archive file does not exist at %s", backup.FilePath)
		}

		// Path should be under instanceBackupDir/{uuid}/
		if !strings.HasPrefix(backup.FilePath, ts.instanceBackupDir) {
			t.Errorf("FilePath %q does not start with %q", backup.FilePath, ts.instanceBackupDir)
		}

		// Should end with .tar.gz
		if !strings.HasSuffix(backup.FilePath, ".tar.gz") {
			t.Errorf("FilePath %q does not end with .tar.gz", backup.FilePath)
		}
	})

	t.Run("Archive contains manifest.json", func(t *testing.T) {
		ts := setupInstanceBackupTest(t)

		backup, err := ts.svc.Create(t.Context())
		if err != nil {
			t.Fatalf("Create: %v", err)
		}

		entries := readArchiveEntries(t, backup.FilePath)
		if _, ok := entries["manifest.json"]; !ok {
			t.Fatal("archive does not contain manifest.json")
		}
	})

	t.Run("Archive contains theia.db", func(t *testing.T) {
		ts := setupInstanceBackupTest(t)

		backup, err := ts.svc.Create(t.Context())
		if err != nil {
			t.Fatalf("Create: %v", err)
		}

		entries := readArchiveEntries(t, backup.FilePath)
		if _, ok := entries["theia.db"]; !ok {
			t.Fatal("archive does not contain theia.db")
		}

		// DB entry should be non-empty
		if len(entries["theia.db"]) == 0 {
			t.Fatal("theia.db entry is empty")
		}
	})

	t.Run("Archive contains backups/ when device backup files exist", func(t *testing.T) {
		ts := setupInstanceBackupTest(t)

		// Create fake device backup files
		deviceDir := filepath.Join(ts.deviceBackupDir, "device-uuid-1")
		if err := os.MkdirAll(deviceDir, 0755); err != nil {
			t.Fatalf("creating device dir: %v", err)
		}
		if err := os.WriteFile(filepath.Join(deviceDir, "backup.rsc"), []byte("# config export"), 0644); err != nil {
			t.Fatalf("writing device backup: %v", err)
		}

		backup, err := ts.svc.Create(t.Context())
		if err != nil {
			t.Fatalf("Create: %v", err)
		}

		entries := readArchiveEntries(t, backup.FilePath)

		// Should have at least one entry under backups/
		found := false
		for name := range entries {
			if strings.HasPrefix(name, "backups/") {
				found = true
				break
			}
		}
		if !found {
			t.Fatal("archive does not contain any entries under backups/")
		}
	})

	t.Run("Archive contains known_hosts when file exists", func(t *testing.T) {
		ts := setupInstanceBackupTest(t)

		// Create known_hosts file
		if err := os.WriteFile(ts.knownHostsPath, []byte("192.168.1.1 ssh-rsa AAAA...\n"), 0644); err != nil {
			t.Fatalf("writing known_hosts: %v", err)
		}

		backup, err := ts.svc.Create(t.Context())
		if err != nil {
			t.Fatalf("Create: %v", err)
		}

		entries := readArchiveEntries(t, backup.FilePath)
		if _, ok := entries["known_hosts"]; !ok {
			t.Fatal("archive does not contain known_hosts")
		}
	})

	t.Run("Manifest has all required fields", func(t *testing.T) {
		ts := setupInstanceBackupTest(t)

		backup, err := ts.svc.Create(t.Context())
		if err != nil {
			t.Fatalf("Create: %v", err)
		}

		entries := readArchiveEntries(t, backup.FilePath)
		manifestData, ok := entries["manifest.json"]
		if !ok {
			t.Fatal("no manifest.json in archive")
		}

		var manifest backupManifest
		if err := json.Unmarshal(manifestData, &manifest); err != nil {
			t.Fatalf("parsing manifest: %v", err)
		}

		if manifest.Version != 1 {
			t.Errorf("manifest.version = %d, want 1", manifest.Version)
		}
		if manifest.AppVersion == "" {
			t.Error("manifest.app_version is empty")
		}
		if manifest.GitCommit == "" {
			t.Error("manifest.git_commit is empty")
		}
		if manifest.MigrationVersion == 0 {
			t.Error("manifest.migration_version is 0")
		}
		if manifest.CreatedAt == "" {
			t.Error("manifest.created_at is empty")
		}
		if manifest.DBSHA256 == "" {
			t.Error("manifest.db_sha256 is empty")
		}
		if manifest.EncryptionKeyHash == "" {
			t.Error("manifest.encryption_key_hash is empty")
		}
	})

	t.Run("db_sha256 matches actual SHA-256 of theia.db in archive", func(t *testing.T) {
		ts := setupInstanceBackupTest(t)

		backup, err := ts.svc.Create(t.Context())
		if err != nil {
			t.Fatalf("Create: %v", err)
		}

		entries := readArchiveEntries(t, backup.FilePath)
		var manifest backupManifest
		if err := json.Unmarshal(entries["manifest.json"], &manifest); err != nil {
			t.Fatalf("parsing manifest: %v", err)
		}

		// Compute SHA-256 of the theia.db entry
		dbData := entries["theia.db"]
		h := sha256.Sum256(dbData)
		actualHash := hex.EncodeToString(h[:])

		if manifest.DBSHA256 != actualHash {
			t.Errorf("manifest.db_sha256 = %q, actual theia.db hash = %q", manifest.DBSHA256, actualHash)
		}
	})

	t.Run("encryption_key_hash is SHA-256 of first 8 bytes of key", func(t *testing.T) {
		ts := setupInstanceBackupTest(t)

		backup, err := ts.svc.Create(t.Context())
		if err != nil {
			t.Fatalf("Create: %v", err)
		}

		entries := readArchiveEntries(t, backup.FilePath)
		var manifest backupManifest
		if err := json.Unmarshal(entries["manifest.json"], &manifest); err != nil {
			t.Fatalf("parsing manifest: %v", err)
		}

		// Expected: SHA-256 of first 8 bytes of encryption key
		keyPrefix := ts.encryptionKey[:8]
		expected := sha256.Sum256(keyPrefix)
		expectedHash := hex.EncodeToString(expected[:])

		if manifest.EncryptionKeyHash != expectedHash {
			t.Errorf("encryption_key_hash = %q, want %q", manifest.EncryptionKeyHash, expectedHash)
		}
	})

	t.Run("Sidecar sha256 file exists alongside archive", func(t *testing.T) {
		ts := setupInstanceBackupTest(t)

		backup, err := ts.svc.Create(t.Context())
		if err != nil {
			t.Fatalf("Create: %v", err)
		}

		sidecarPath := backup.FilePath + ".sha256"
		if _, err := os.Stat(sidecarPath); os.IsNotExist(err) {
			t.Fatalf("sidecar .sha256 file does not exist at %s", sidecarPath)
		}
	})

	t.Run("Sidecar sha256 content matches actual archive hash", func(t *testing.T) {
		ts := setupInstanceBackupTest(t)

		backup, err := ts.svc.Create(t.Context())
		if err != nil {
			t.Fatalf("Create: %v", err)
		}

		// Read sidecar
		sidecarData, err := os.ReadFile(backup.FilePath + ".sha256")
		if err != nil {
			t.Fatalf("reading sidecar: %v", err)
		}

		// Parse sidecar: format is "{hash}  {basename}\n"
		parts := strings.Fields(string(sidecarData))
		if len(parts) < 1 {
			t.Fatalf("sidecar file is empty or malformed: %q", string(sidecarData))
		}
		sidecarHash := parts[0]

		// Compute actual archive hash
		actualHash := computeSHA256(t, backup.FilePath)

		if sidecarHash != actualHash {
			t.Errorf("sidecar hash = %q, actual archive hash = %q", sidecarHash, actualHash)
		}
	})

	t.Run("Archive and sidecar are written with private permissions", func(t *testing.T) {
		ts := setupInstanceBackupTest(t)

		backup, err := ts.svc.Create(t.Context())
		if err != nil {
			t.Fatalf("Create: %v", err)
		}

		assertPathMode(t, backup.FilePath, 0600)
		assertPathMode(t, backup.FilePath+".sha256", 0600)
	})

	t.Run("Backup record has status success after Create", func(t *testing.T) {
		ts := setupInstanceBackupTest(t)

		backup, err := ts.svc.Create(t.Context())
		if err != nil {
			t.Fatalf("Create: %v", err)
		}

		if backup.Status != domain.InstanceBackupStatusSuccess {
			t.Errorf("status = %q, want %q", backup.Status, domain.InstanceBackupStatusSuccess)
		}

		// Verify via repo as well
		got, err := ts.repo.GetByID(backup.ID)
		if err != nil {
			t.Fatalf("GetByID: %v", err)
		}
		if got.Status != domain.InstanceBackupStatusSuccess {
			t.Errorf("repo status = %q, want %q", got.Status, domain.InstanceBackupStatusSuccess)
		}
	})

	t.Run("Create with missing device backup dir still succeeds", func(t *testing.T) {
		ts := setupInstanceBackupTest(t)

		// Remove the device backup directory
		os.RemoveAll(ts.deviceBackupDir)

		backup, err := ts.svc.Create(t.Context())
		if err != nil {
			t.Fatalf("Create: %v", err)
		}

		entries := readArchiveEntries(t, backup.FilePath)

		// Should not have any backups/ entries
		for name := range entries {
			if strings.HasPrefix(name, "backups/") {
				t.Errorf("unexpected backups/ entry when device backup dir missing: %s", name)
			}
		}

		// Manifest should have backup_file_count=0
		var manifest backupManifest
		if err := json.Unmarshal(entries["manifest.json"], &manifest); err != nil {
			t.Fatalf("parsing manifest: %v", err)
		}
		if manifest.BackupFileCount != 0 {
			t.Errorf("backup_file_count = %d, want 0", manifest.BackupFileCount)
		}
	})

	t.Run("Create with missing known_hosts still succeeds", func(t *testing.T) {
		ts := setupInstanceBackupTest(t)

		// Don't create known_hosts file (it doesn't exist by default)
		backup, err := ts.svc.Create(t.Context())
		if err != nil {
			t.Fatalf("Create: %v", err)
		}

		entries := readArchiveEntries(t, backup.FilePath)
		if _, ok := entries["known_hosts"]; ok {
			t.Error("archive should not contain known_hosts when file does not exist")
		}
	})

	t.Run("List delegates to repo", func(t *testing.T) {
		ts := setupInstanceBackupTest(t)

		// Create two backups
		_, err := ts.svc.Create(t.Context())
		if err != nil {
			t.Fatalf("Create 1: %v", err)
		}
		_, err = ts.svc.Create(t.Context())
		if err != nil {
			t.Fatalf("Create 2: %v", err)
		}

		list, err := ts.svc.List(t.Context())
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if len(list) != 2 {
			t.Errorf("List returned %d items, want 2", len(list))
		}
	})

	t.Run("GetByID delegates to repo", func(t *testing.T) {
		ts := setupInstanceBackupTest(t)

		backup, err := ts.svc.Create(t.Context())
		if err != nil {
			t.Fatalf("Create: %v", err)
		}

		got, err := ts.svc.GetByID(t.Context(), backup.ID)
		if err != nil {
			t.Fatalf("GetByID: %v", err)
		}
		if got == nil {
			t.Fatal("GetByID returned nil")
		}
		if got.ID != backup.ID {
			t.Errorf("ID = %v, want %v", got.ID, backup.ID)
		}
	})

	t.Run("Delete removes archive files and repo record", func(t *testing.T) {
		ts := setupInstanceBackupTest(t)

		backup, err := ts.svc.Create(t.Context())
		if err != nil {
			t.Fatalf("Create: %v", err)
		}

		archivePath := backup.FilePath
		sidecarPath := archivePath + ".sha256"
		backupDir := filepath.Dir(archivePath)

		// Verify files exist before delete
		if _, err := os.Stat(archivePath); os.IsNotExist(err) {
			t.Fatalf("archive not found before delete")
		}

		if err := ts.svc.Delete(t.Context(), backup.ID); err != nil {
			t.Fatalf("Delete: %v", err)
		}

		// Archive and sidecar should be gone (entire UUID directory removed)
		if _, err := os.Stat(archivePath); !os.IsNotExist(err) {
			t.Error("archive file still exists after delete")
		}
		if _, err := os.Stat(sidecarPath); !os.IsNotExist(err) {
			t.Error("sidecar file still exists after delete")
		}
		if _, err := os.Stat(backupDir); !os.IsNotExist(err) {
			t.Error("backup UUID directory still exists after delete")
		}

		// Repo record should be gone
		got, err := ts.repo.GetByID(backup.ID)
		if err != nil {
			t.Fatalf("GetByID after delete: %v", err)
		}
		if got != nil {
			t.Error("backup record still exists in repo after delete")
		}
	})
}
