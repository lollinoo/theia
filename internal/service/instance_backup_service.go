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
