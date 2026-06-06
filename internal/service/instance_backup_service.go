package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/lollinoo/theia/internal/crypto"
	"github.com/lollinoo/theia/internal/domain"
	"github.com/lollinoo/theia/internal/version"
)

// InstanceBackupService orchestrates full Theia instance backups (database + config files).
type InstanceBackupService struct {
	db              *sql.DB
	repo            domain.InstanceBackupRepository
	settingsRepo    domain.SettingsRepository
	backupDir       string // THEIA_INSTANCE_BACKUP_DIR
	deviceBackupDir string // THEIA_BACKUP_DIR (device config files)
	knownHostsPath  string // SSH known hosts file path
	stateDir        string // local restore marker/staging directory
	dbDSN           string // live DB DSN for postgres backups/restores
	encryptionKey   []byte // legacy key hash in old manifests
	keyring         *crypto.Keyring
	restoreLimits   RestoreArchiveLimits
	backupLimits    BackupArchiveLimits
	createMu        sync.Mutex
	operations      *instanceBackupOperationTracker
}

// backupManifest describes the contents and metadata of an instance backup archive.
type backupManifest struct {
	Version           int                       `json:"version"`
	AppVersion        string                    `json:"app_version"`
	GitCommit         string                    `json:"git_commit"`
	DBEntryName       string                    `json:"db_entry_name,omitempty"`
	MigrationVersion  int                       `json:"migration_version"`
	CreatedAt         string                    `json:"created_at"`
	DBSHA256          string                    `json:"db_sha256"`
	BackupFileCount   int                       `json:"backup_file_count"`
	TotalSizeBytes    int64                     `json:"total_size_bytes"`
	EncryptionKeyHash string                    `json:"encryption_key_hash"`
	Encryption        *backupManifestEncryption `json:"encryption,omitempty"`
}

type backupManifestEncryption struct {
	Version         int      `json:"version"`
	ActiveKeyID     string   `json:"active_key_id"`
	RequiredKeyIDs  []string `json:"required_key_ids"`
	LegacyKeyHashes []string `json:"legacy_key_hashes,omitempty"`
}

const (
	legacySQLiteArchiveDBEntry = "theia.db"
	postgresArchiveDBEntry     = "database.dump"
)

type databaseBackupArtifact struct {
	tempPath         string
	archiveEntryName string
	migrationVersion int
}

type archiveSourceFile struct {
	archiveName string
	diskPath    string
	sizeBytes   int64
}

// Instance backup lifecycle errors used by API handlers.
var (
	ErrInstanceBackupAlreadyRunning = errors.New("instance backup already in progress")
	ErrInstanceBackupNotFound       = errors.New("instance backup not found")
	ErrInstanceBackupNotRunning     = errors.New("instance backup is not running")
)

// NewInstanceBackupService creates a new InstanceBackupService.
func NewInstanceBackupService(
	db *sql.DB,
	repo domain.InstanceBackupRepository,
	settingsRepo domain.SettingsRepository,
	backupDir string,
	deviceBackupDir string,
	knownHostsPath string,
	stateDir string,
	dbDSN string,
	encryptionKey any,
) *InstanceBackupService {
	keyring, legacyKey := normalizeInstanceBackupEncryptionKey(encryptionKey)
	return &InstanceBackupService{
		db:              db,
		repo:            repo,
		settingsRepo:    settingsRepo,
		backupDir:       backupDir,
		deviceBackupDir: deviceBackupDir,
		knownHostsPath:  knownHostsPath,
		stateDir:        stateDir,
		dbDSN:           strings.TrimSpace(dbDSN),
		encryptionKey:   legacyKey,
		keyring:         keyring,
		restoreLimits:   DefaultRestoreArchiveLimits,
		backupLimits:    DefaultBackupArchiveLimits,
		operations:      newInstanceBackupOperationTracker(),
	}
}

func normalizeInstanceBackupEncryptionKey(key any) (*crypto.Keyring, []byte) {
	switch k := key.(type) {
	case *crypto.Keyring:
		return k, nil
	case []byte:
		return nil, k
	case nil:
		return nil, nil
	default:
		return nil, nil
	}
}

func (s *InstanceBackupService) RestoreOperationStatus() (*RestoreOperationStatus, bool, error) {
	if s == nil {
		return nil, false, nil
	}
	return readRestoreOperationStatus(s.stateDir)
}

// Create produces a full instance backup archive with trigger set to "manual".
func (s *InstanceBackupService) Create(ctx context.Context) (*domain.InstanceBackup, error) {
	return s.CreateWithTrigger(ctx, domain.InstanceBackupTriggerManual)
}

// CreateWithTrigger produces a full instance backup archive containing the database,
// device config files, SSH known_hosts, and a manifest with integrity metadata.
// The trigger field records what initiated the backup (manual or scheduled).
func (s *InstanceBackupService) CreateWithTrigger(ctx context.Context, trigger domain.InstanceBackupTrigger) (*domain.InstanceBackup, error) {
	backup, backupSubDir, err := s.prepareInstanceBackup(trigger)
	if err != nil {
		return nil, err
	}
	return s.runPreparedInstanceBackup(ctx, backup, backupSubDir)
}

// StartCreateWithTrigger creates a running instance backup record and starts archive work in the background.
func (s *InstanceBackupService) StartCreateWithTrigger(ctx context.Context, trigger domain.InstanceBackupTrigger) (*domain.InstanceBackup, error) {
	backup, backupSubDir, err := s.prepareInstanceBackup(trigger)
	if err != nil {
		return nil, err
	}
	runCtx, cancel := s.backupRunContext(ctx)
	s.beginInstanceBackupOperation(backup.ID, cancel, domain.InstanceBackupProgress{
		Phase:   "queued",
		Message: "Backup queued",
	})
	go func() {
		defer s.endInstanceBackupOperation(backup.ID)
		defer cancel()
		if _, err := s.runPreparedInstanceBackupWithContext(runCtx, backup, backupSubDir, false); err != nil {
			log.Printf("Instance backup %s failed: %v", backup.ID, err)
		}
	}()
	return backup, nil
}

// prepareInstanceBackup creates the filesystem and repository state for a new run.
func (s *InstanceBackupService) prepareInstanceBackup(trigger domain.InstanceBackupTrigger) (*domain.InstanceBackup, string, error) {
	s.createMu.Lock()
	defer s.createMu.Unlock()

	if s.hasActiveInstanceBackupOperation() {
		return nil, "", ErrInstanceBackupAlreadyRunning
	}
	if running, err := s.hasRunningInstanceBackup(); err != nil {
		return nil, "", fmt.Errorf("checking running backups: %w", err)
	} else if running {
		return nil, "", ErrInstanceBackupAlreadyRunning
	}

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
		return nil, "", fmt.Errorf("creating backup directory: %w", err)
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
		return nil, "", fmt.Errorf("creating backup record: %w", err)
	}

	return backup, backupSubDir, nil
}

// runPreparedInstanceBackup runs a prepared backup synchronously with its own tracker entry.
func (s *InstanceBackupService) runPreparedInstanceBackup(ctx context.Context, backup *domain.InstanceBackup, backupSubDir string) (*domain.InstanceBackup, error) {
	runCtx, cancel := s.backupRunContext(ctx)
	s.beginInstanceBackupOperation(backup.ID, cancel, domain.InstanceBackupProgress{
		Phase:   "starting",
		Message: "Preparing backup",
	})
	defer s.endInstanceBackupOperation(backup.ID)
	defer cancel()
	return s.runPreparedInstanceBackupWithContext(runCtx, backup, backupSubDir, true)
}

// runPreparedInstanceBackupWithContext performs database dump, manifest, archive, hash, and persistence steps.
func (s *InstanceBackupService) runPreparedInstanceBackupWithContext(ctx context.Context, backup *domain.InstanceBackup, backupSubDir string, ownOperation bool) (*domain.InstanceBackup, error) {
	if err := ctx.Err(); err != nil {
		s.cleanupFailedInstanceBackup(backup, backupSubDir, "backup cancelled", err)
		return nil, err
	}

	limits := s.BackupArchiveLimits()

	cleanupOnError := func(errMsg string, err error) {
		s.cleanupFailedInstanceBackup(backup, backupSubDir, errMsg, err)
	}

	s.updateInstanceBackupProgress(backup.ID, domain.InstanceBackupProgress{
		Phase:   "database",
		Message: "Creating database snapshot",
	})

	// Step 1: Create a PostgreSQL database dump.
	dbArtifact, err := s.backupDatabase(ctx, backupSubDir)
	if err != nil {
		cleanupOnError(fmt.Sprintf("backing up database: %v", err), err)
		return nil, fmt.Errorf("backing up database: %w", err)
	}
	defer os.Remove(dbArtifact.tempPath)

	dbInfo, err := os.Stat(dbArtifact.tempPath)
	if err != nil {
		cleanupOnError(fmt.Sprintf("statting database backup: %v", err), err)
		return nil, fmt.Errorf("statting database backup: %w", err)
	}
	if err := checkBackupArchiveEntryQuota(dbArtifact.archiveEntryName, dbInfo.Size(), limits); err != nil {
		cleanupOnError(fmt.Sprintf("checking database archive quota: %v", err), err)
		return nil, err
	}

	s.updateInstanceBackupProgress(backup.ID, domain.InstanceBackupProgress{
		Phase:   "hashing",
		Message: "Hashing database snapshot",
		Current: dbInfo.Size(),
		Total:   dbInfo.Size(),
	})

	// Step 2: Compute SHA-256 of the database copy
	dbHash, err := computeFileHashContext(ctx, dbArtifact.tempPath)
	if err != nil {
		cleanupOnError(fmt.Sprintf("computing DB hash: %v", err), err)
		return nil, fmt.Errorf("computing DB hash: %w", err)
	}

	s.updateInstanceBackupProgress(backup.ID, domain.InstanceBackupProgress{
		Phase:   "collecting",
		Message: "Collecting device backup files",
	})

	deviceBackupFiles, backupFileCount, knownHostsFile, totalSourceBytes, archiveFileEntries, err := s.collectArchiveSourceFiles(ctx, limits, dbInfo.Size())
	if err != nil {
		cleanupOnError(fmt.Sprintf("collecting backup files: %v", err), err)
		return nil, err
	}
	requiredKeyIDs, err := s.collectRequiredCredentialKeyIDs(ctx)
	if err != nil {
		cleanupOnError(fmt.Sprintf("collecting credential encryption metadata: %v", err), err)
		return nil, err
	}

	// Step 5: Build manifest
	manifestPlan, err := buildInstanceBackupArchiveManifestPlan(instanceBackupArchiveManifestInput{
		appVersion:         version.Version,
		gitCommit:          version.GitCommit,
		dbArtifact:         dbArtifact,
		backupCreatedAt:    backup.CreatedAt,
		dbSHA256:           dbHash,
		backupFileCount:    backupFileCount,
		totalSourceBytes:   totalSourceBytes,
		archiveFileEntries: archiveFileEntries,
		encryptionKey:      s.encryptionKey,
		encryptionKeyring:  s.keyring,
		requiredKeyIDs:     requiredKeyIDs,
		limits:             limits,
	})
	if err != nil {
		errMsg := err.Error()
		if !strings.HasPrefix(errMsg, "marshaling manifest:") {
			errMsg = fmt.Sprintf("checking archive quota: %v", err)
		}
		cleanupOnError(errMsg, err)
		return nil, err
	}
	manifest := manifestPlan.manifest
	manifestJSON := manifestPlan.manifestJSON
	totalArchiveBytes := manifestPlan.totalArchiveBytes

	s.updateInstanceBackupProgress(backup.ID, domain.InstanceBackupProgress{
		Phase:   "archiving",
		Message: "Writing backup archive",
		Total:   totalArchiveBytes,
	})

	// Step 6: Create archive at temp path
	finalPath := filepath.Join(backupSubDir, backup.FileName)
	tempArchivePath := finalPath + ".tmp"

	totalSize, err := s.createArchive(ctx, tempArchivePath, dbArtifact, deviceBackupFiles, knownHostsFile, manifestJSON, &manifest, backup.ID)
	if err != nil {
		cleanupOnError(fmt.Sprintf("creating archive: %v", err), err)
		os.Remove(tempArchivePath)
		return nil, fmt.Errorf("creating archive: %w", err)
	}
	manifest.TotalSizeBytes = totalSize

	// Step 7: Rename temp archive to final path
	if err := os.Rename(tempArchivePath, finalPath); err != nil {
		cleanupOnError(fmt.Sprintf("renaming archive: %v", err), err)
		os.Remove(tempArchivePath)
		return nil, fmt.Errorf("renaming archive: %w", err)
	}
	if err := os.Chmod(finalPath, 0600); err != nil {
		cleanupOnError(fmt.Sprintf("restricting archive permissions: %v", err), err)
		return nil, fmt.Errorf("restricting archive permissions: %w", err)
	}

	s.updateInstanceBackupProgress(backup.ID, domain.InstanceBackupProgress{
		Phase:   "hashing",
		Message: "Hashing backup archive",
	})

	// Step 8: Compute archive SHA-256 and write sidecar
	archiveHash, err := computeFileHashContext(ctx, finalPath)
	if err != nil {
		cleanupOnError(fmt.Sprintf("computing archive hash: %v", err), err)
		return nil, fmt.Errorf("computing archive hash: %w", err)
	}

	sidecarContent := fmt.Sprintf("%s  %s\n", archiveHash, filepath.Base(finalPath))
	sidecarPath := finalPath + ".sha256"
	if err := os.WriteFile(sidecarPath, []byte(sidecarContent), 0600); err != nil {
		cleanupOnError(fmt.Sprintf("writing sidecar: %v", err), err)
		return nil, fmt.Errorf("writing sidecar: %w", err)
	}
	if err := ctx.Err(); err != nil {
		cleanupOnError("backup cancelled", err)
		return nil, err
	}
	if s.instanceBackupCancellationRequested(backup.ID) {
		err := context.Canceled
		cleanupOnError("backup cancelled", err)
		return nil, err
	}

	// Step 9: Get archive file size
	archiveInfo, err := os.Stat(finalPath)
	if err != nil {
		cleanupOnError(fmt.Sprintf("statting archive: %v", err), err)
		return nil, fmt.Errorf("statting archive: %w", err)
	}

	// Step 10: Update DB record to success
	backup.FilePath = finalPath
	backup.SizeBytes = archiveInfo.Size()
	backup.SHA256 = archiveHash
	backup.AppVersion = version.Version
	backup.MigrationVersion = dbArtifact.migrationVersion
	backup.Status = domain.InstanceBackupStatusSuccess
	backup.ErrorMessage = ""

	if err := s.completeInstanceBackupSuccess(backup, totalSize, ownOperation); err != nil {
		if errors.Is(err, context.Canceled) {
			cleanupOnError("backup cancelled", err)
			return nil, err
		}
		return nil, fmt.Errorf("updating backup record: %w", err)
	}

	return backup, nil
}

// completeInstanceBackupSuccess persists successful metadata while respecting cancellation races.
func (s *InstanceBackupService) completeInstanceBackupSuccess(backup *domain.InstanceBackup, totalSize int64, ownOperation bool) error {
	return s.operations.completeSuccess(backup.ID, totalSize, ownOperation, func() error {
		return s.repo.Update(backup)
	})
}

// cleanupFailedInstanceBackup marks a failed or cancelled run and removes its working directory.
func (s *InstanceBackupService) cleanupFailedInstanceBackup(backup *domain.InstanceBackup, backupSubDir string, errMsg string, err error) {
	status := domain.InstanceBackupStatusFailed
	if errors.Is(err, context.Canceled) || s.instanceBackupCancellationRequested(backup.ID) || backupAlreadyCancelled(s.repo, backup.ID) {
		status = domain.InstanceBackupStatusCancelled
		errMsg = "cancelled by user"
	}
	backup.Status = status
	backup.ErrorMessage = errMsg
	if updateErr := s.repo.Update(backup); updateErr != nil {
		log.Printf("Failed to update backup record to %s: %v", status, updateErr)
	}
	os.RemoveAll(backupSubDir)
}

// backupAlreadyCancelled checks persisted state for cancellation that arrived during cleanup.
func backupAlreadyCancelled(repo domain.InstanceBackupRepository, id uuid.UUID) bool {
	backup, err := repo.GetByID(id)
	return err == nil && backup != nil && backup.Status == domain.InstanceBackupStatusCancelled
}

// backupRunContext applies the configured backup duration limit to a run context.
func (s *InstanceBackupService) backupRunContext(ctx context.Context) (context.Context, context.CancelFunc) {
	if ctx == nil {
		ctx = context.Background()
	}
	limits := s.BackupArchiveLimits()
	if limits.MaxDuration > 0 {
		return context.WithTimeout(ctx, limits.MaxDuration)
	}
	return context.WithCancel(ctx)
}

// hasRunningInstanceBackup checks persisted records for an already-running backup.
func (s *InstanceBackupService) hasRunningInstanceBackup() (bool, error) {
	backups, err := s.repo.List()
	if err != nil {
		return false, err
	}
	for i := range backups {
		if backups[i].Status == domain.InstanceBackupStatusRunning {
			return true, nil
		}
	}
	return false, nil
}

// backupDatabase creates a logical dump of the live PostgreSQL database.
func (s *InstanceBackupService) backupDatabase(ctx context.Context, backupSubDir string) (databaseBackupArtifact, error) {
	return s.backupPostgresDatabase(ctx, filepath.Join(backupSubDir, postgresArchiveDBEntry+".tmp"))
}

// backupPostgresDatabase dumps PostgreSQL and records the migration version for the manifest.
func (s *InstanceBackupService) backupPostgresDatabase(ctx context.Context, destPath string) (databaseBackupArtifact, error) {
	if err := runPostgresDump(ctx, s.dbDSN, destPath); err != nil {
		return databaseBackupArtifact{}, err
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

// readCurrentMigrationVersion reads the live schema migration version from PostgreSQL.
func (s *InstanceBackupService) readCurrentMigrationVersion(ctx context.Context) (int, error) {
	if s.db == nil {
		return 0, fmt.Errorf("database connection unavailable")
	}

	var version int
	if err := s.db.QueryRowContext(ctx, "SELECT version FROM schema_migrations").Scan(&version); err != nil {
		return 0, fmt.Errorf("querying migration version: %w", err)
	}
	return version, nil
}

// validatePostgresDump delegates dump validation to pg_restore inspection.
func (s *InstanceBackupService) validatePostgresDump(ctx context.Context, dumpPath string) error {
	return validatePostgresDumpArchive(ctx, dumpPath)
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
// When dryRun is true, only validation is performed. When false, this service
// owns validation, staging, and pending marker creation. RestoreCoordinator owns
// live activation, pre-restore DB backup, retry state, and pending marker/staging
// cleanup after activation; the API/restarter layer owns the process handoff
// before activation.
func (s *InstanceBackupService) ValidateAndStageRestore(archivePath string, dryRun bool) (*RestoreReport, error) {
	return s.ValidateAndStageRestoreContext(context.Background(), archivePath, dryRun)
}

// ValidateAndStageRestoreContext validates and stages a restore archive while observing caller cancellation.
func (s *InstanceBackupService) ValidateAndStageRestoreContext(ctx context.Context, archivePath string, dryRun bool) (*RestoreReport, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	limits := normalizeRestoreArchiveLimits(s.restoreLimits)

	if err := ctx.Err(); err != nil {
		return nil, err
	}
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
	if err := extractArchiveContext(ctx, archivePath, tempDir, limits); err != nil {
		return nil, err
	}

	// Step 3: Parse manifest
	manifest, err := readRestoreManifest(tempDir)
	if err != nil {
		return nil, err
	}
	dbEntryName, err := manifestDatabaseEntryName(manifest)
	if err != nil {
		return nil, err
	}
	extractedDBPath := filepath.Join(tempDir, filepath.FromSlash(dbEntryName))

	// Step 4: Verify configured encryption keys
	if err := validateRestoreManifestEncryptionKey(manifest, s.restoreEncryptionKeySource()); err != nil {
		return nil, err
	}

	// Step 5: Verify DB checksum
	actualDBHash, err := computeFileHashContext(ctx, extractedDBPath)
	if err != nil {
		return nil, fmt.Errorf("computing extracted DB hash: %w", err)
	}
	if manifest.DBSHA256 != actualDBHash {
		return nil, fmt.Errorf("database checksum mismatch: archive may be corrupted")
	}

	// Step 6: Validate the extracted database payload.
	if err := s.validatePostgresDump(ctx, extractedDBPath); err != nil {
		return nil, err
	}

	// Step 7: Read current migration version from live DB
	currentVersion, err := s.readCurrentMigrationVersion(ctx)
	if err != nil {
		return nil, fmt.Errorf("reading current migration version: %w", err)
	}

	// Step 8: Check migration version compatibility
	needsMigration, err := validateRestoreManifestMigrationCompatibility(manifest, currentVersion)
	if err != nil {
		return nil, err
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
	if err := ensureSupportedPostgresCLITools(ctx, "pg_dump", "pg_restore", "psql"); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	// Step 14: Stage files for restore
	stagingDir := filepath.Join(s.stateDir, ".restore-staging")
	markerPath := restoreMarkerFilePath(s.stateDir)
	if err := os.RemoveAll(stagingDir); err != nil {
		return nil, fmt.Errorf("removing existing staging dir: %w", err)
	}
	if err := os.MkdirAll(stagingDir, 0700); err != nil {
		return nil, fmt.Errorf("creating staging dir: %w", err)
	}
	cleanupStagingOnError := func(err error) error {
		if removeErr := os.RemoveAll(stagingDir); removeErr != nil {
			log.Printf("Warning: failed to remove restore staging dir after error: %v", removeErr)
		}
		if removeErr := os.Remove(markerPath); removeErr != nil && !os.IsNotExist(removeErr) {
			log.Printf("Warning: failed to remove restore marker after error: %v", removeErr)
		}
		return err
	}

	stagedDBName := filepath.Base(dbEntryName)
	stagedDBPath := filepath.Join(stagingDir, stagedDBName)

	// Move the database payload into staging when possible; copy only when the
	// extraction and staging directories live on different filesystems.
	if err := moveOrCopyFileForRestoreStagingContext(ctx, extractedDBPath, stagedDBPath); err != nil {
		return nil, cleanupStagingOnError(fmt.Errorf("staging database: %w", err))
	}

	// Stage backups/ directory if it exists.
	srcBackups := filepath.Join(tempDir, "backups")
	if info, err := os.Stat(srcBackups); err == nil && info.IsDir() {
		if err := moveOrCopyDirForRestoreStagingContext(ctx, srcBackups, filepath.Join(stagingDir, "backups")); err != nil {
			return nil, cleanupStagingOnError(fmt.Errorf("staging backup files: %w", err))
		}
	}

	// Stage known_hosts if it exists.
	srcKnownHosts := filepath.Join(tempDir, "known_hosts")
	if _, err := os.Stat(srcKnownHosts); err == nil {
		if err := moveOrCopyFileForRestoreStagingContext(ctx, srcKnownHosts, filepath.Join(stagingDir, "known_hosts")); err != nil {
			return nil, cleanupStagingOnError(fmt.Errorf("staging known_hosts: %w", err))
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, cleanupStagingOnError(err)
	}

	// Write marker file
	marker := newRestoreMarker(
		stagedDBPath,
		filepath.Join(stagingDir, "backups"),
		filepath.Join(stagingDir, "known_hosts"),
		s.stateDir,
		s.deviceBackupDir,
		s.knownHostsPath,
		time.Now().UTC().Format(time.RFC3339),
	)
	updateRestoreOperationFields(&marker, restorePhaseStagedRestartPending, "", "")
	if err := writeRestoreMarker(markerPath, marker); err != nil {
		return nil, cleanupStagingOnError(err)
	}
	if err := writeRestoreOperationStatus(s.stateDir, restoreOperationStatusFromMarker(marker)); err != nil {
		return nil, cleanupStagingOnError(err)
	}

	report.Message = "Restore staged successfully. Server will restart to apply."
	return report, nil
}

func (s *InstanceBackupService) restoreEncryptionKeySource() any {
	if s != nil && s.keyring != nil {
		return s.keyring
	}
	if s == nil {
		return nil
	}
	return s.encryptionKey
}

// FailStaleRunning reconciles any "running" backups on startup.
// If the archive file exists on disk, the backup completed but the DB was
// snapshot'd mid-process (self-referential backup), so it is marked as success.
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

// List returns all instance backups.
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
	if backup == nil {
		return ErrInstanceBackupNotFound
	}
	if backup.Status == domain.InstanceBackupStatusRunning {
		return ErrInstanceBackupNotRunning
	}

	// Remove the UUID subdirectory containing the archive and sidecar
	if backup != nil && backup.FilePath != "" {
		if err := validateInstanceBackupFilePath(s.backupDir, backup.FilePath); err != nil {
			return err
		}
		if err := os.RemoveAll(filepath.Dir(backup.FilePath)); err != nil {
			log.Printf("Warning: failed to remove backup files at %s: %v", filepath.Dir(backup.FilePath), err)
		}
	}

	return s.repo.Delete(id)
}

// validateInstanceBackupFilePath ensures deletion targets stay inside the backup root.
func validateInstanceBackupFilePath(rootDir string, filePath string) error {
	root := filepath.Clean(rootDir)
	dir := filepath.Clean(filepath.Dir(filePath))
	rel, err := filepath.Rel(root, dir)
	if err != nil {
		return fmt.Errorf("backup file path is outside instance backup directory: %w", err)
	}
	if rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return fmt.Errorf("backup file path is outside instance backup directory")
	}
	return nil
}
