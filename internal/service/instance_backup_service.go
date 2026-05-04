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
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/lollinoo/theia/internal/domain"
	"github.com/lollinoo/theia/internal/repository/sqlite"
	"github.com/lollinoo/theia/internal/version"
)

// InstanceBackupService orchestrates full Theia instance backups (database + config files).
type InstanceBackupService struct {
	db              *sql.DB
	dialect         sqlite.Dialect
	repo            domain.InstanceBackupRepository
	settingsRepo    domain.SettingsRepository
	backupDir       string // THEIA_INSTANCE_BACKUP_DIR
	deviceBackupDir string // THEIA_BACKUP_DIR (device config files)
	knownHostsPath  string // SSH known hosts file path
	dbPath          string // live DB path
	dbDSN           string // live DB DSN for postgres backups/restores
	encryptionKey   []byte // for key hash in manifest
	restoreLimits   RestoreArchiveLimits
}

// backupManifest describes the contents and metadata of an instance backup archive.
type backupManifest struct {
	Version           int    `json:"version"`
	AppVersion        string `json:"app_version"`
	GitCommit         string `json:"git_commit"`
	DBDriver          string `json:"db_driver,omitempty"`
	DBEntryName       string `json:"db_entry_name,omitempty"`
	MigrationVersion  int    `json:"migration_version"`
	CreatedAt         string `json:"created_at"`
	DBSHA256          string `json:"db_sha256"`
	BackupFileCount   int    `json:"backup_file_count"`
	TotalSizeBytes    int64  `json:"total_size_bytes"`
	EncryptionKeyHash string `json:"encryption_key_hash"`
}

const (
	sqliteArchiveDBEntry   = "theia.db"
	postgresArchiveDBEntry = "database.dump"
)

// RestoreArchiveLimits defines defensive quotas for uploaded restore archives.
type RestoreArchiveLimits struct {
	MaxCompressedBytes int64
	MaxTotalBytes      int64
	MaxEntryBytes      int64
	MaxFileEntries     int
}

// DefaultRestoreArchiveLimits caps restore uploads and extracted archive content.
var DefaultRestoreArchiveLimits = RestoreArchiveLimits{
	MaxCompressedBytes: 256 << 20, // 256 MiB uploaded .tar.gz
	MaxTotalBytes:      1 << 30,   // 1 GiB expanded regular-file content
	MaxEntryBytes:      512 << 20, // 512 MiB per regular file
	MaxFileEntries:     25000,
}

type databaseBackupArtifact struct {
	tempPath         string
	archiveEntryName string
	migrationVersion int
}

// NewInstanceBackupService creates a new InstanceBackupService.
func NewInstanceBackupService(
	db *sql.DB,
	repo domain.InstanceBackupRepository,
	settingsRepo domain.SettingsRepository,
	backupDir string,
	deviceBackupDir string,
	knownHostsPath string,
	dbPath string,
	dbDSN string,
	encryptionKey []byte,
) *InstanceBackupService {
	return &InstanceBackupService{
		db:              db,
		dialect:         sqlite.DetectDialect(db),
		repo:            repo,
		settingsRepo:    settingsRepo,
		backupDir:       backupDir,
		deviceBackupDir: deviceBackupDir,
		knownHostsPath:  knownHostsPath,
		dbPath:          dbPath,
		dbDSN:           strings.TrimSpace(dbDSN),
		encryptionKey:   encryptionKey,
		restoreLimits:   DefaultRestoreArchiveLimits,
	}
}

// SetRestoreArchiveLimitsForTest overrides restore archive quotas in focused tests.
func (s *InstanceBackupService) SetRestoreArchiveLimitsForTest(limits RestoreArchiveLimits) {
	s.restoreLimits = normalizeRestoreArchiveLimits(limits)
}

func normalizeRestoreArchiveLimits(limits RestoreArchiveLimits) RestoreArchiveLimits {
	defaults := DefaultRestoreArchiveLimits
	if limits.MaxCompressedBytes <= 0 {
		limits.MaxCompressedBytes = defaults.MaxCompressedBytes
	}
	if limits.MaxTotalBytes <= 0 {
		limits.MaxTotalBytes = defaults.MaxTotalBytes
	}
	if limits.MaxEntryBytes <= 0 {
		limits.MaxEntryBytes = defaults.MaxEntryBytes
	}
	if limits.MaxFileEntries <= 0 {
		limits.MaxFileEntries = defaults.MaxFileEntries
	}
	return limits
}

// Create produces a full instance backup archive with trigger set to "manual".
func (s *InstanceBackupService) Create(ctx context.Context) (*domain.InstanceBackup, error) {
	return s.CreateWithTrigger(ctx, domain.InstanceBackupTriggerManual)
}

// CreateWithTrigger produces a full instance backup archive containing the database,
// device config files, SSH known_hosts, and a manifest with integrity metadata.
// The trigger field records what initiated the backup (manual or scheduled).
func (s *InstanceBackupService) CreateWithTrigger(ctx context.Context, trigger domain.InstanceBackupTrigger) (*domain.InstanceBackup, error) {
	backupID := uuid.New()
	now := time.Now().UTC()

	// Build filename: theia-backup-{YYYYMMDD}-{HHMMSS}-v{version}.tar.gz
	fileName := fmt.Sprintf("theia-backup-%s-v%s.tar.gz",
		now.Format("20060102-150405"),
		version.Version,
	)

	// Create backup subdirectory: {backupDir}/{backupID}/
	backupSubDir := filepath.Join(s.backupDir, backupID.String())
	if err := os.MkdirAll(backupSubDir, 0700); err != nil {
		return nil, fmt.Errorf("creating backup directory: %w", err)
	}

	// Create initial DB record with status "running"
	backup := &domain.InstanceBackup{
		ID:       backupID,
		FileName: fileName,
		Status:   domain.InstanceBackupStatusRunning,
		Trigger:  trigger,
	}
	if err := s.repo.Create(backup); err != nil {
		os.RemoveAll(backupSubDir)
		return nil, fmt.Errorf("creating backup record: %w", err)
	}

	// On error, mark as failed and clean up
	var cleanupOnError = func(errMsg string) {
		backup.Status = domain.InstanceBackupStatusFailed
		backup.ErrorMessage = errMsg
		if updateErr := s.repo.Update(backup); updateErr != nil {
			log.Printf("Failed to update backup record to failed: %v", updateErr)
		}
		os.RemoveAll(backupSubDir)
	}

	// Step 1: Create a dialect-appropriate database snapshot/dump.
	dbArtifact, err := s.backupDatabase(ctx, backupSubDir)
	if err != nil {
		cleanupOnError(fmt.Sprintf("backing up database: %v", err))
		return nil, fmt.Errorf("backing up database: %w", err)
	}
	defer os.Remove(dbArtifact.tempPath)

	// Step 2: Compute SHA-256 of the database copy
	dbHash, err := computeFileHash(dbArtifact.tempPath)
	if err != nil {
		cleanupOnError(fmt.Sprintf("computing DB hash: %v", err))
		return nil, fmt.Errorf("computing DB hash: %w", err)
	}

	// Step 4: Collect device backup files
	backupFileCount := 0
	var deviceBackupFiles []struct {
		archiveName string
		diskPath    string
	}
	if info, err := os.Stat(s.deviceBackupDir); err == nil && info.IsDir() {
		err := filepath.Walk(s.deviceBackupDir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return nil // skip files we can't read
			}
			if info.IsDir() {
				// Skip the instance backup directory to prevent circular inclusion (T-15-10)
				cleanInstanceBackupDir := filepath.Clean(s.backupDir)
				cleanPath := filepath.Clean(path)
				if cleanPath == cleanInstanceBackupDir || strings.HasPrefix(cleanPath, cleanInstanceBackupDir+string(filepath.Separator)) {
					return filepath.SkipDir
				}
				return nil
			}

			rel, err := filepath.Rel(s.deviceBackupDir, path)
			if err != nil {
				return nil
			}

			// Validate archive entry name: no absolute paths, no traversal (T-15-05)
			archiveName := filepath.ToSlash(filepath.Join("backups", rel))
			if strings.HasPrefix(archiveName, "/") || strings.Contains(archiveName, "..") {
				return nil
			}

			deviceBackupFiles = append(deviceBackupFiles, struct {
				archiveName string
				diskPath    string
			}{archiveName: archiveName, diskPath: path})
			backupFileCount++
			return nil
		})
		if err != nil {
			log.Printf("Warning: error walking device backup dir: %v", err)
		}
	}

	// Step 5: Build manifest
	manifest := backupManifest{
		Version:           1,
		AppVersion:        version.Version,
		GitCommit:         version.GitCommit,
		DBDriver:          string(s.dialect),
		DBEntryName:       dbArtifact.archiveEntryName,
		MigrationVersion:  dbArtifact.migrationVersion,
		CreatedAt:         now.Format(time.RFC3339),
		DBSHA256:          dbHash,
		BackupFileCount:   backupFileCount,
		TotalSizeBytes:    0, // will be updated after archiving
		EncryptionKeyHash: computeEncryptionKeyHash(s.encryptionKey),
	}

	// Step 6: Create archive at temp path
	finalPath := filepath.Join(backupSubDir, fileName)
	tempArchivePath := finalPath + ".tmp"

	totalSize, err := s.createArchive(tempArchivePath, dbArtifact, deviceBackupFiles, &manifest)
	if err != nil {
		cleanupOnError(fmt.Sprintf("creating archive: %v", err))
		os.Remove(tempArchivePath)
		return nil, fmt.Errorf("creating archive: %w", err)
	}
	manifest.TotalSizeBytes = totalSize

	// Step 7: Rename temp archive to final path
	if err := os.Rename(tempArchivePath, finalPath); err != nil {
		cleanupOnError(fmt.Sprintf("renaming archive: %v", err))
		os.Remove(tempArchivePath)
		return nil, fmt.Errorf("renaming archive: %w", err)
	}
	if err := os.Chmod(finalPath, 0600); err != nil {
		cleanupOnError(fmt.Sprintf("restricting archive permissions: %v", err))
		return nil, fmt.Errorf("restricting archive permissions: %w", err)
	}

	// Step 8: Compute archive SHA-256 and write sidecar
	archiveHash, err := computeFileHash(finalPath)
	if err != nil {
		cleanupOnError(fmt.Sprintf("computing archive hash: %v", err))
		return nil, fmt.Errorf("computing archive hash: %w", err)
	}

	sidecarContent := fmt.Sprintf("%s  %s\n", archiveHash, filepath.Base(finalPath))
	sidecarPath := finalPath + ".sha256"
	if err := os.WriteFile(sidecarPath, []byte(sidecarContent), 0600); err != nil {
		cleanupOnError(fmt.Sprintf("writing sidecar: %v", err))
		return nil, fmt.Errorf("writing sidecar: %w", err)
	}

	// Step 9: Get archive file size
	archiveInfo, err := os.Stat(finalPath)
	if err != nil {
		cleanupOnError(fmt.Sprintf("statting archive: %v", err))
		return nil, fmt.Errorf("statting archive: %w", err)
	}

	// Step 10: Update DB record to success
	backup.FileName = fileName
	backup.FilePath = finalPath
	backup.SizeBytes = archiveInfo.Size()
	backup.SHA256 = archiveHash
	backup.AppVersion = version.Version
	backup.MigrationVersion = dbArtifact.migrationVersion
	backup.Status = domain.InstanceBackupStatusSuccess
	backup.ErrorMessage = ""

	if err := s.repo.Update(backup); err != nil {
		return nil, fmt.Errorf("updating backup record: %w", err)
	}

	return backup, nil
}

// createArchive builds a .tar.gz archive containing manifest, database, device backups, and known_hosts.
// Returns the total size of all archived file data.
func (s *InstanceBackupService) createArchive(
	archivePath string,
	dbArtifact databaseBackupArtifact,
	deviceBackupFiles []struct {
		archiveName string
		diskPath    string
	},
	manifest *backupManifest,
) (int64, error) {
	f, err := os.OpenFile(archivePath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return 0, fmt.Errorf("creating archive file: %w", err)
	}
	defer f.Close()

	gw := gzip.NewWriter(f)
	defer gw.Close()

	tw := tar.NewWriter(gw)
	defer tw.Close()

	var totalSize int64

	// Add manifest.json
	manifestJSON, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return 0, fmt.Errorf("marshaling manifest: %w", err)
	}
	if err := addBytesToTar(tw, "manifest.json", manifestJSON, time.Now().UTC()); err != nil {
		return 0, fmt.Errorf("adding manifest to archive: %w", err)
	}
	totalSize += int64(len(manifestJSON))

	// Add the database payload (SQLite file or PostgreSQL dump).
	dbSize, err := addFileToTar(tw, dbArtifact.archiveEntryName, dbArtifact.tempPath)
	if err != nil {
		return 0, fmt.Errorf("adding database to archive: %w", err)
	}
	totalSize += dbSize

	// Add device backup files under backups/
	for _, bf := range deviceBackupFiles {
		size, err := addFileToTar(tw, bf.archiveName, bf.diskPath)
		if err != nil {
			log.Printf("Warning: skipping device backup file %s: %v", bf.diskPath, err)
			continue
		}
		totalSize += size
	}

	// Add known_hosts if it exists
	if info, err := os.Stat(s.knownHostsPath); err == nil && !info.IsDir() {
		size, err := addFileToTar(tw, "known_hosts", s.knownHostsPath)
		if err != nil {
			log.Printf("Warning: failed to add known_hosts to archive: %v", err)
		} else {
			totalSize += size
		}
	}

	return totalSize, nil
}

// backupDatabase creates a clean copy or logical dump of the live database.
func (s *InstanceBackupService) backupDatabase(ctx context.Context, backupSubDir string) (databaseBackupArtifact, error) {
	switch s.dialect {
	case sqlite.DialectPostgres:
		return s.backupPostgresDatabase(ctx, filepath.Join(backupSubDir, postgresArchiveDBEntry+".tmp"))
	default:
		return s.backupSQLiteDatabase(ctx, filepath.Join(backupSubDir, sqliteArchiveDBEntry+".tmp"))
	}
}

func (s *InstanceBackupService) backupSQLiteDatabase(ctx context.Context, destPath string) (databaseBackupArtifact, error) {
	// Validate destination path to prevent injection (T-15-04)
	cleanDest := filepath.Clean(destPath)
	if strings.ContainsAny(cleanDest, "';") {
		return databaseBackupArtifact{}, fmt.Errorf("invalid destination path: contains forbidden characters")
	}

	// Checkpoint WAL to ensure all data is in the main database file
	if _, err := s.db.ExecContext(ctx, "PRAGMA wal_checkpoint(TRUNCATE)"); err != nil {
		log.Printf("Warning: WAL checkpoint failed (non-fatal): %v", err)
	}

	// VACUUM INTO creates a clean, compacted copy of the database
	// Try parameterized first; fall back to formatted string if needed
	_, err := s.db.ExecContext(ctx, "VACUUM INTO ?", cleanDest)
	if err != nil {
		// Some SQLite versions don't support parameterized VACUUM INTO
		_, err = s.db.ExecContext(ctx, fmt.Sprintf("VACUUM INTO '%s'", cleanDest))
		if err != nil {
			return databaseBackupArtifact{}, fmt.Errorf("VACUUM INTO failed: %w", err)
		}
	}

	// Verify integrity of the backup copy
	backupDB, err := sql.Open("sqlite3", cleanDest+"?mode=ro")
	if err != nil {
		return databaseBackupArtifact{}, fmt.Errorf("opening backup DB for integrity check: %w", err)
	}
	defer backupDB.Close()

	var integrityResult string
	if err := backupDB.QueryRowContext(ctx, "PRAGMA integrity_check").Scan(&integrityResult); err != nil {
		return databaseBackupArtifact{}, fmt.Errorf("integrity check failed: %w", err)
	}
	if integrityResult != "ok" {
		return databaseBackupArtifact{}, fmt.Errorf("backup database integrity check failed: %s", integrityResult)
	}

	migrationVersion, err := readSQLiteMigrationVersion(cleanDest)
	if err != nil {
		return databaseBackupArtifact{}, fmt.Errorf("reading migration version: %w", err)
	}

	return databaseBackupArtifact{
		tempPath:         cleanDest,
		archiveEntryName: sqliteArchiveDBEntry,
		migrationVersion: migrationVersion,
	}, nil
}

func (s *InstanceBackupService) backupPostgresDatabase(ctx context.Context, destPath string) (databaseBackupArtifact, error) {
	if strings.TrimSpace(s.dbDSN) == "" {
		return databaseBackupArtifact{}, fmt.Errorf("postgres backup requires db_dsn")
	}
	if err := ensureExternalCommand("pg_dump"); err != nil {
		return databaseBackupArtifact{}, err
	}
	conn, err := postgresCLIConnInfo(s.dbDSN)
	if err != nil {
		return databaseBackupArtifact{}, fmt.Errorf("build postgres conninfo: %w", err)
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
		return databaseBackupArtifact{}, fmt.Errorf("pg_dump failed: %w", err)
	}

	migrationVersion, err := s.readCurrentMigrationVersion(ctx)
	if err != nil {
		return databaseBackupArtifact{}, fmt.Errorf("reading migration version: %w", err)
	}

	return databaseBackupArtifact{
		tempPath:         destPath,
		archiveEntryName: postgresArchiveDBEntry,
		migrationVersion: migrationVersion,
	}, nil
}

// computeEncryptionKeyHash returns the SHA-256 hash of the first 8 bytes of the encryption key.
// This allows verifying the correct key is used during restore without exposing the full key.
func computeEncryptionKeyHash(key []byte) string {
	if len(key) < 8 {
		// Key too short; hash what we have
		h := sha256.Sum256(key)
		return hex.EncodeToString(h[:])
	}
	h := sha256.Sum256(key[:8])
	return hex.EncodeToString(h[:])
}

// readSQLiteMigrationVersion reads the current schema migration version from a SQLite database file.
func readSQLiteMigrationVersion(dbPath string) (int, error) {
	db, err := sql.Open("sqlite3", dbPath+"?mode=ro")
	if err != nil {
		return 0, fmt.Errorf("opening DB for migration version: %w", err)
	}
	defer db.Close()

	var version int
	if err := db.QueryRow("SELECT version FROM schema_migrations").Scan(&version); err != nil {
		return 0, fmt.Errorf("querying migration version: %w", err)
	}
	return version, nil
}

func (s *InstanceBackupService) readCurrentMigrationVersion(ctx context.Context) (int, error) {
	if s.dialect == sqlite.DialectSQLite {
		return readSQLiteMigrationVersion(s.dbPath)
	}
	if s.db == nil {
		return 0, fmt.Errorf("database connection unavailable")
	}

	var version int
	if err := s.db.QueryRowContext(ctx, "SELECT version FROM schema_migrations").Scan(&version); err != nil {
		return 0, fmt.Errorf("querying migration version: %w", err)
	}
	return version, nil
}

func manifestDatabaseDialect(manifest backupManifest) (sqlite.Dialect, error) {
	if strings.TrimSpace(manifest.DBDriver) == "" {
		return sqlite.DialectSQLite, nil
	}
	return sqlite.NormalizeDialect(manifest.DBDriver)
}

func manifestDatabaseEntryName(dialect sqlite.Dialect, manifest backupManifest) (string, error) {
	if entry := strings.TrimSpace(manifest.DBEntryName); entry != "" {
		switch entry {
		case sqliteArchiveDBEntry, postgresArchiveDBEntry:
			return entry, nil
		default:
			return "", fmt.Errorf("unsupported database entry %q in manifest", entry)
		}
	}
	if dialect == sqlite.DialectPostgres {
		return postgresArchiveDBEntry, nil
	}
	return sqliteArchiveDBEntry, nil
}

func (s *InstanceBackupService) validatePostgresDump(ctx context.Context, dumpPath string) error {
	if err := ensureExternalCommand("pg_restore"); err != nil {
		return err
	}
	if _, err := runExternalCommand(ctx, "pg_restore", "--list", dumpPath); err != nil {
		return fmt.Errorf("validating postgres dump: %w", err)
	}
	return nil
}

// addFileToTar adds a file from disk to the tar archive. Returns the file size.
func addFileToTar(tw *tar.Writer, name string, sourcePath string) (int64, error) {
	f, err := os.Open(sourcePath)
	if err != nil {
		return 0, fmt.Errorf("opening %s: %w", sourcePath, err)
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return 0, fmt.Errorf("statting %s: %w", sourcePath, err)
	}

	header := &tar.Header{
		Name:    name,
		Size:    info.Size(),
		Mode:    0644,
		ModTime: info.ModTime(),
	}
	if err := tw.WriteHeader(header); err != nil {
		return 0, fmt.Errorf("writing header for %s: %w", name, err)
	}

	written, err := io.Copy(tw, f)
	if err != nil {
		return 0, fmt.Errorf("writing data for %s: %w", name, err)
	}
	return written, nil
}

// addBytesToTar adds raw bytes as a tar entry.
func addBytesToTar(tw *tar.Writer, name string, data []byte, modTime time.Time) error {
	header := &tar.Header{
		Name:    name,
		Size:    int64(len(data)),
		Mode:    0644,
		ModTime: modTime,
	}
	if err := tw.WriteHeader(header); err != nil {
		return fmt.Errorf("writing header for %s: %w", name, err)
	}
	if _, err := tw.Write(data); err != nil {
		return fmt.Errorf("writing data for %s: %w", name, err)
	}
	return nil
}

// computeFileHash computes the SHA-256 hash of a file using streaming I/O.
func computeFileHash(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("opening file for hash: %w", err)
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", fmt.Errorf("hashing file: %w", err)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// RestoreReport contains the results of archive validation and staging.
type RestoreReport struct {
	Valid            bool   `json:"valid"`
	AppVersion       string `json:"app_version"`
	GitCommit        string `json:"git_commit"`
	MigrationVersion int    `json:"migration_version"`
	CreatedAt        string `json:"created_at"`
	DBSizeBytes      int64  `json:"db_size_bytes"`
	BackupFileCount  int    `json:"backup_file_count"`
	TotalSizeBytes   int64  `json:"total_size_bytes"`
	NeedsMigration   bool   `json:"needs_migration"`
	CurrentMigration int    `json:"current_migration_version"`
	Message          string `json:"message"`
}

// ValidateAndStageRestore validates a backup archive and optionally stages it for restore.
// When dryRun is true, only validation is performed. When false, validated files are staged
// and a marker file is written for the restart-based restore flow.
func (s *InstanceBackupService) ValidateAndStageRestore(archivePath string, dryRun bool) (*RestoreReport, error) {
	ctx := context.Background()
	limits := normalizeRestoreArchiveLimits(s.restoreLimits)

	if err := validateRestoreArchiveFile(archivePath, limits); err != nil {
		return nil, err
	}

	// Step 1: Create temp extraction dir
	tempDir, err := os.MkdirTemp("", "theia-restore-*")
	if err != nil {
		return nil, fmt.Errorf("creating temp dir: %w", err)
	}
	defer os.RemoveAll(tempDir)

	// Step 2: Extract archive
	if err := extractArchive(archivePath, tempDir, limits); err != nil {
		return nil, err
	}

	// Step 3: Parse manifest
	manifestPath := filepath.Join(tempDir, "manifest.json")
	manifestData, err := os.ReadFile(manifestPath)
	if err != nil {
		return nil, fmt.Errorf("archive missing manifest.json")
	}

	var manifest backupManifest
	if err := json.Unmarshal(manifestData, &manifest); err != nil {
		return nil, fmt.Errorf("parsing manifest.json: %w", err)
	}

	archiveDialect, err := manifestDatabaseDialect(manifest)
	if err != nil {
		return nil, fmt.Errorf("parsing manifest database driver: %w", err)
	}
	if archiveDialect != s.dialect {
		return nil, fmt.Errorf("archive database driver %q does not match current runtime database driver %q", archiveDialect, s.dialect)
	}

	dbEntryName, err := manifestDatabaseEntryName(archiveDialect, manifest)
	if err != nil {
		return nil, err
	}
	extractedDBPath := filepath.Join(tempDir, filepath.FromSlash(dbEntryName))

	// Step 4: Verify encryption key hash
	currentKeyHash := computeEncryptionKeyHash(s.encryptionKey)
	if manifest.EncryptionKeyHash != currentKeyHash {
		return nil, fmt.Errorf("encryption key mismatch: backup was created with a different THEIA_ENCRYPTION_KEY")
	}

	// Step 5: Verify DB checksum
	actualDBHash, err := computeFileHash(extractedDBPath)
	if err != nil {
		return nil, fmt.Errorf("computing extracted DB hash: %w", err)
	}
	if manifest.DBSHA256 != actualDBHash {
		return nil, fmt.Errorf("database checksum mismatch: archive may be corrupted")
	}

	// Step 6: Validate the extracted database payload.
	switch archiveDialect {
	case sqlite.DialectPostgres:
		if err := s.validatePostgresDump(ctx, extractedDBPath); err != nil {
			return nil, err
		}
	default:
		checkDB, err := sql.Open("sqlite3", extractedDBPath+"?mode=ro")
		if err != nil {
			return nil, fmt.Errorf("opening extracted DB for integrity check: %w", err)
		}
		var integrityResult string
		if err := checkDB.QueryRow("PRAGMA integrity_check").Scan(&integrityResult); err != nil {
			checkDB.Close()
			return nil, fmt.Errorf("running integrity check: %w", err)
		}
		checkDB.Close()
		if integrityResult != "ok" {
			return nil, fmt.Errorf("database integrity check failed: %s", integrityResult)
		}
	}

	// Step 7: Read current migration version from live DB
	currentVersion, err := s.readCurrentMigrationVersion(ctx)
	if err != nil {
		return nil, fmt.Errorf("reading current migration version: %w", err)
	}

	// Step 8: Check migration version compatibility
	if manifest.MigrationVersion > currentVersion {
		return nil, fmt.Errorf("archive has newer migration version (%d) than current (%d); upgrade Theia first",
			manifest.MigrationVersion, currentVersion)
	}

	// Step 9: Determine if migration is needed
	needsMigration := manifest.MigrationVersion < currentVersion

	// Step 10: Run cross-version migration if needed and not dry run
	if archiveDialect == sqlite.DialectSQLite && needsMigration && !dryRun {
		migDB, err := sql.Open("sqlite3", extractedDBPath)
		if err != nil {
			return nil, fmt.Errorf("opening extracted DB for migration: %w", err)
		}
		if err := sqlite.RunMigrations(migDB, s.encryptionKey); err != nil {
			migDB.Close()
			return nil, fmt.Errorf("running migrations on extracted DB: %w", err)
		}
		migDB.Close()
	}

	// Step 11: Get DB file size for report
	dbInfo, err := os.Stat(extractedDBPath)
	if err != nil {
		return nil, fmt.Errorf("statting extracted DB: %w", err)
	}

	// Step 12: Build report
	report := &RestoreReport{
		Valid:            true,
		AppVersion:       manifest.AppVersion,
		GitCommit:        manifest.GitCommit,
		MigrationVersion: manifest.MigrationVersion,
		CreatedAt:        manifest.CreatedAt,
		DBSizeBytes:      dbInfo.Size(),
		BackupFileCount:  manifest.BackupFileCount,
		TotalSizeBytes:   manifest.TotalSizeBytes,
		NeedsMigration:   needsMigration,
		CurrentMigration: currentVersion,
	}

	// Step 13: Dry run — return report without staging
	if dryRun {
		report.Message = "Validation passed. Archive is ready to restore."
		return report, nil
	}

	// Step 14: Stage files for restore
	stagingDir := filepath.Join(filepath.Dir(s.dbPath), ".restore-staging")
	if err := os.RemoveAll(stagingDir); err != nil {
		return nil, fmt.Errorf("removing existing staging dir: %w", err)
	}
	if err := os.MkdirAll(stagingDir, 0700); err != nil {
		return nil, fmt.Errorf("creating staging dir: %w", err)
	}

	stagedDBName := filepath.Base(dbEntryName)
	stagedDBPath := filepath.Join(stagingDir, stagedDBName)

	// Copy the database payload to staging.
	if err := copyFile(extractedDBPath, stagedDBPath); err != nil {
		return nil, fmt.Errorf("staging database: %w", err)
	}

	// Copy backups/ directory if it exists
	srcBackups := filepath.Join(tempDir, "backups")
	if info, err := os.Stat(srcBackups); err == nil && info.IsDir() {
		if err := copyDir(srcBackups, filepath.Join(stagingDir, "backups")); err != nil {
			return nil, fmt.Errorf("staging backup files: %w", err)
		}
	}

	// Copy known_hosts if it exists
	srcKnownHosts := filepath.Join(tempDir, "known_hosts")
	if _, err := os.Stat(srcKnownHosts); err == nil {
		if err := copyFile(srcKnownHosts, filepath.Join(stagingDir, "known_hosts")); err != nil {
			return nil, fmt.Errorf("staging known_hosts: %w", err)
		}
	}

	// Write marker file
	markerPath := filepath.Join(filepath.Dir(s.dbPath), ".theia-restore-pending")
	marker := newRestoreMarker(
		stagedDBPath,
		filepath.Join(stagingDir, "backups"),
		filepath.Join(stagingDir, "known_hosts"),
		s.dbPath,
		s.deviceBackupDir,
		s.knownHostsPath,
		time.Now().UTC().Format(time.RFC3339),
	)
	marker.DBDriver = string(archiveDialect)
	markerJSON, err := json.MarshalIndent(marker, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshaling marker JSON: %w", err)
	}
	if err := os.WriteFile(markerPath, markerJSON, 0600); err != nil {
		return nil, fmt.Errorf("writing restore marker: %w", err)
	}
	if err := os.Chmod(markerPath, 0600); err != nil {
		return nil, fmt.Errorf("restricting restore marker permissions: %w", err)
	}

	report.Message = "Restore staged successfully. Server will restart to apply."
	return report, nil
}

func validateRestoreArchiveFile(archivePath string, limits RestoreArchiveLimits) error {
	info, err := os.Stat(archivePath)
	if err != nil {
		return fmt.Errorf("statting archive: %w", err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("restore archive must be a regular file")
	}
	if info.Size() > limits.MaxCompressedBytes {
		return fmt.Errorf("compressed archive exceeds restore limit: %d bytes > %d bytes", info.Size(), limits.MaxCompressedBytes)
	}
	return nil
}

// extractArchive extracts a .tar.gz archive to the given directory with security validation and quotas.
func extractArchive(archivePath, destDir string, limits RestoreArchiveLimits) error {
	f, err := os.Open(archivePath)
	if err != nil {
		return fmt.Errorf("opening archive: %w", err)
	}
	defer f.Close()

	gr, err := gzip.NewReader(f)
	if err != nil {
		return fmt.Errorf("creating gzip reader: %w", err)
	}
	defer gr.Close()

	tr := tar.NewReader(gr)
	var totalBytes int64
	var fileEntries int
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("reading tar entry: %w", err)
		}

		// Security: reject symlinks and hard links (T-17-01)
		if header.Typeflag == tar.TypeSymlink || header.Typeflag == tar.TypeLink {
			return fmt.Errorf("archive contains disallowed link entry: %s", header.Name)
		}

		// Security: only allow regular files and directories
		if header.Typeflag != tar.TypeReg && header.Typeflag != tar.TypeDir {
			continue // skip unknown types
		}

		// Security: validate path has no traversal (T-17-01)
		cleanName := filepath.Clean(header.Name)
		if strings.Contains(cleanName, "..") {
			return fmt.Errorf("archive contains path traversal: %s", header.Name)
		}
		if filepath.IsAbs(cleanName) {
			return fmt.Errorf("archive contains absolute path: %s", header.Name)
		}

		// Security: only allow known prefixes (T-17-01)
		if !isAllowedArchiveEntry(cleanName) {
			continue // skip unknown entries
		}

		targetPath := filepath.Join(destDir, cleanName)

		if header.Typeflag == tar.TypeDir {
			if err := os.MkdirAll(targetPath, 0700); err != nil {
				return fmt.Errorf("creating directory %s: %w", cleanName, err)
			}
			continue
		}

		if header.Size < 0 {
			return fmt.Errorf("archive entry %s has invalid negative size", cleanName)
		}
		if header.Size > limits.MaxEntryBytes {
			return fmt.Errorf("archive entry %s exceeds per-entry restore limit: %d bytes > %d bytes", cleanName, header.Size, limits.MaxEntryBytes)
		}
		if header.Size > limits.MaxTotalBytes-totalBytes {
			return fmt.Errorf("expanded archive exceeds restore limit: %d bytes > %d bytes", totalBytes+header.Size, limits.MaxTotalBytes)
		}
		fileEntries++
		if fileEntries > limits.MaxFileEntries {
			return fmt.Errorf("archive file count exceeds restore limit: %d files > %d files", fileEntries, limits.MaxFileEntries)
		}

		// Ensure parent directory exists
		if err := os.MkdirAll(filepath.Dir(targetPath), 0700); err != nil {
			return fmt.Errorf("creating parent directory for %s: %w", cleanName, err)
		}

		outFile, err := os.OpenFile(targetPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
		if err != nil {
			return fmt.Errorf("creating file %s: %w", cleanName, err)
		}
		if _, err := io.Copy(outFile, tr); err != nil {
			outFile.Close()
			return fmt.Errorf("writing file %s: %w", cleanName, err)
		}
		outFile.Close()
		totalBytes += header.Size
	}

	return nil
}

// isAllowedArchiveEntry checks if a tar entry name matches known allowed prefixes.
func isAllowedArchiveEntry(name string) bool {
	allowed := []string{"manifest.json", sqliteArchiveDBEntry, postgresArchiveDBEntry, "backups/", "known_hosts"}
	for _, prefix := range allowed {
		if name == prefix || strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}

// copyFile copies a single file from src to dst with private file permissions.
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("opening %s: %w", src, err)
	}
	defer in.Close()

	if err := os.MkdirAll(filepath.Dir(dst), 0700); err != nil {
		return fmt.Errorf("creating parent directory for %s: %w", dst, err)
	}

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return fmt.Errorf("creating %s: %w", dst, err)
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return fmt.Errorf("copying %s to %s: %w", src, dst, err)
	}

	return os.Chmod(dst, 0600)
}

// copyDir recursively copies a directory from srcDir to dstDir.
func copyDir(srcDir, dstDir string) error {
	return filepath.Walk(srcDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		rel, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dstDir, rel)

		if info.IsDir() {
			return os.MkdirAll(target, 0700)
		}

		return copyFile(path, target)
	})
}

// List returns all instance backups.
// FailStaleRunning reconciles any "running" backups on startup.
// If the archive file exists on disk, the backup completed but the DB was
// snapshot'd mid-process (self-referential backup) — mark it as success.
// Otherwise the goroutine is gone and the backup truly failed.
func (s *InstanceBackupService) FailStaleRunning() {
	backups, err := s.repo.List()
	if err != nil {
		log.Printf("Warning: failed to check for stale running backups: %v", err)
		return
	}
	for i := range backups {
		if backups[i].Status == domain.InstanceBackupStatusRunning {
			// The VACUUM snapshot is taken before FilePath is set, so for
			// self-referential backups (restored from own archive) FilePath is "".
			// Reconstruct the expected path: {backupDir}/{id}/{fileName}
			archivePath := backups[i].FilePath
			if archivePath == "" && backups[i].FileName != "" {
				archivePath = filepath.Join(s.backupDir, backups[i].ID.String(), backups[i].FileName)
			}

			if archivePath != "" {
				if info, statErr := os.Stat(archivePath); statErr == nil && info.Size() > 0 {
					backups[i].FilePath = archivePath
					backups[i].SizeBytes = info.Size()
					backups[i].Status = domain.InstanceBackupStatusSuccess
					backups[i].AppVersion = version.Version
					if err := s.repo.Update(&backups[i]); err != nil {
						log.Printf("Warning: failed to reconcile backup %s: %v", backups[i].ID, err)
					} else {
						log.Printf("Reconciled stale running backup %s as success (archive exists on disk)", backups[i].ID)
					}
					continue
				}
			}

			backups[i].Status = domain.InstanceBackupStatusFailed
			backups[i].ErrorMessage = "interrupted by server restart"
			if err := s.repo.Update(&backups[i]); err != nil {
				log.Printf("Warning: failed to mark stale backup %s as failed: %v", backups[i].ID, err)
			} else {
				log.Printf("Marked stale running backup %s as failed", backups[i].ID)
			}
		}
	}
}

func (s *InstanceBackupService) List(ctx context.Context) ([]domain.InstanceBackup, error) {
	return s.repo.List()
}

// GetByID returns an instance backup by ID.
func (s *InstanceBackupService) GetByID(ctx context.Context, id uuid.UUID) (*domain.InstanceBackup, error) {
	return s.repo.GetByID(id)
}

// Delete removes an instance backup's archive files from disk and its repo record.
func (s *InstanceBackupService) Delete(ctx context.Context, id uuid.UUID) error {
	backup, err := s.repo.GetByID(id)
	if err != nil {
		return fmt.Errorf("getting backup for delete: %w", err)
	}

	// Remove the UUID subdirectory containing the archive and sidecar
	if backup != nil && backup.FilePath != "" {
		if err := os.RemoveAll(filepath.Dir(backup.FilePath)); err != nil {
			log.Printf("Warning: failed to remove backup files at %s: %v", filepath.Dir(backup.FilePath), err)
		}
	}

	return s.repo.Delete(id)
}
