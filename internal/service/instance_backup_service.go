package service

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

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
	encryptionKey   []byte // for key hash in manifest
	restoreLimits   RestoreArchiveLimits
	backupLimits    BackupArchiveLimits
	createMu        sync.Mutex
	operationMu     sync.Mutex
	operations      map[uuid.UUID]*instanceBackupOperation
}

// backupManifest describes the contents and metadata of an instance backup archive.
type backupManifest struct {
	Version           int    `json:"version"`
	AppVersion        string `json:"app_version"`
	GitCommit         string `json:"git_commit"`
	DBEntryName       string `json:"db_entry_name,omitempty"`
	MigrationVersion  int    `json:"migration_version"`
	CreatedAt         string `json:"created_at"`
	DBSHA256          string `json:"db_sha256"`
	BackupFileCount   int    `json:"backup_file_count"`
	TotalSizeBytes    int64  `json:"total_size_bytes"`
	EncryptionKeyHash string `json:"encryption_key_hash"`
}

const (
	postgresArchiveDBEntry = "database.dump"
)

// RestoreArchiveLimits defines defensive quotas for uploaded restore archives.
type RestoreArchiveLimits struct {
	MaxCompressedBytes int64
	MaxTotalBytes      int64
	MaxEntryBytes      int64
	MaxFileEntries     int
}

// RestoreLimitError marks restore validation failures caused by defensive quota limits.
type RestoreLimitError struct {
	message string
}

func (e *RestoreLimitError) Error() string {
	return e.message
}

func newRestoreLimitError(format string, args ...interface{}) error {
	return &RestoreLimitError{message: fmt.Sprintf(format, args...)}
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

type archiveSourceFile struct {
	archiveName string
	diskPath    string
	sizeBytes   int64
}

type instanceBackupOperation struct {
	cancel          context.CancelFunc
	progress        domain.InstanceBackupProgress
	cancelRequested bool
}

// BackupArchiveLimits defines defensive quotas for instance backup archive creation.
type BackupArchiveLimits struct {
	MaxTotalBytes  int64
	MaxEntryBytes  int64
	MaxFileEntries int
	MaxDuration    time.Duration
}

// DefaultBackupArchiveLimits caps instance backup archive creation work.
var DefaultBackupArchiveLimits = BackupArchiveLimits{
	MaxTotalBytes:  2 << 30, // 2 GiB expanded archive content
	MaxEntryBytes:  1 << 30, // 1 GiB per archived file
	MaxFileEntries: 50000,
	MaxDuration:    30 * time.Minute,
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
	encryptionKey []byte,
) *InstanceBackupService {
	return &InstanceBackupService{
		db:              db,
		repo:            repo,
		settingsRepo:    settingsRepo,
		backupDir:       backupDir,
		deviceBackupDir: deviceBackupDir,
		knownHostsPath:  knownHostsPath,
		stateDir:        stateDir,
		dbDSN:           strings.TrimSpace(dbDSN),
		encryptionKey:   encryptionKey,
		restoreLimits:   DefaultRestoreArchiveLimits,
		backupLimits:    DefaultBackupArchiveLimits,
		operations:      make(map[uuid.UUID]*instanceBackupOperation),
	}
}

// SetRestoreArchiveLimitsForTest overrides restore archive quotas in focused tests.
func (s *InstanceBackupService) SetRestoreArchiveLimitsForTest(limits RestoreArchiveLimits) {
	s.restoreLimits = normalizeRestoreArchiveLimits(limits)
}

// RestoreArchiveLimits returns the normalized current restore archive limits.
func (s *InstanceBackupService) RestoreArchiveLimits() RestoreArchiveLimits {
	return normalizeRestoreArchiveLimits(s.restoreLimits)
}

// SetBackupArchiveLimitsForTest overrides backup archive quotas in focused tests.
func (s *InstanceBackupService) SetBackupArchiveLimitsForTest(limits BackupArchiveLimits) {
	s.backupLimits = normalizeBackupArchiveLimits(limits)
}

// BackupArchiveLimits returns the normalized current backup archive limits.
func (s *InstanceBackupService) BackupArchiveLimits() BackupArchiveLimits {
	return normalizeBackupArchiveLimits(s.backupLimits)
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

func normalizeBackupArchiveLimits(limits BackupArchiveLimits) BackupArchiveLimits {
	defaults := DefaultBackupArchiveLimits
	if limits.MaxTotalBytes <= 0 {
		limits.MaxTotalBytes = defaults.MaxTotalBytes
	}
	if limits.MaxEntryBytes <= 0 {
		limits.MaxEntryBytes = defaults.MaxEntryBytes
	}
	if limits.MaxFileEntries <= 0 {
		limits.MaxFileEntries = defaults.MaxFileEntries
	}
	if limits.MaxDuration <= 0 {
		limits.MaxDuration = defaults.MaxDuration
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

	// Step 5: Build manifest
	manifest := backupManifest{
		Version:           1,
		AppVersion:        version.Version,
		GitCommit:         version.GitCommit,
		DBEntryName:       dbArtifact.archiveEntryName,
		MigrationVersion:  dbArtifact.migrationVersion,
		CreatedAt:         backup.CreatedAt.UTC().Format(time.RFC3339),
		DBSHA256:          dbHash,
		BackupFileCount:   backupFileCount,
		TotalSizeBytes:    0, // will be updated after archiving
		EncryptionKeyHash: computeEncryptionKeyHash(s.encryptionKey),
	}

	manifestJSON, err := json.MarshalIndent(&manifest, "", "  ")
	if err != nil {
		cleanupOnError(fmt.Sprintf("marshaling manifest: %v", err), err)
		return nil, fmt.Errorf("marshaling manifest: %w", err)
	}
	manifest.TotalSizeBytes = totalSourceBytes + int64(len(manifestJSON))
	manifestJSON, err = json.MarshalIndent(&manifest, "", "  ")
	if err != nil {
		cleanupOnError(fmt.Sprintf("marshaling manifest: %v", err), err)
		return nil, fmt.Errorf("marshaling manifest: %w", err)
	}
	totalArchiveBytes := totalSourceBytes + int64(len(manifestJSON))
	manifest.TotalSizeBytes = totalArchiveBytes
	if err := checkBackupArchiveTotals(totalArchiveBytes, archiveFileEntries+1, limits); err != nil {
		cleanupOnError(fmt.Sprintf("checking archive quota: %v", err), err)
		return nil, err
	}

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

func (s *InstanceBackupService) completeInstanceBackupSuccess(backup *domain.InstanceBackup, totalSize int64, ownOperation bool) error {
	s.operationMu.Lock()
	defer s.operationMu.Unlock()

	if op := s.operations[backup.ID]; op != nil && op.cancelRequested {
		return context.Canceled
	}
	if err := s.repo.Update(backup); err != nil {
		return err
	}
	if ownOperation {
		if op := s.operations[backup.ID]; op != nil {
			op.progress = domain.InstanceBackupProgress{
				Phase:   "complete",
				Message: "Backup complete",
				Current: totalSize,
				Total:   totalSize,
			}
		}
	}
	delete(s.operations, backup.ID)
	return nil
}

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

func backupAlreadyCancelled(repo domain.InstanceBackupRepository, id uuid.UUID) bool {
	backup, err := repo.GetByID(id)
	return err == nil && backup != nil && backup.Status == domain.InstanceBackupStatusCancelled
}

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

func (s *InstanceBackupService) hasActiveInstanceBackupOperation() bool {
	s.operationMu.Lock()
	defer s.operationMu.Unlock()
	return len(s.operations) > 0
}

func (s *InstanceBackupService) beginInstanceBackupOperation(id uuid.UUID, cancel context.CancelFunc, progress domain.InstanceBackupProgress) {
	s.operationMu.Lock()
	defer s.operationMu.Unlock()
	s.operations[id] = &instanceBackupOperation{
		cancel:   cancel,
		progress: progress,
	}
}

func (s *InstanceBackupService) endInstanceBackupOperation(id uuid.UUID) {
	s.operationMu.Lock()
	defer s.operationMu.Unlock()
	delete(s.operations, id)
}

func (s *InstanceBackupService) updateInstanceBackupProgress(id uuid.UUID, progress domain.InstanceBackupProgress) {
	s.operationMu.Lock()
	defer s.operationMu.Unlock()
	if op := s.operations[id]; op != nil {
		op.progress = progress
	}
}

func (s *InstanceBackupService) instanceBackupCancellationRequested(id uuid.UUID) bool {
	s.operationMu.Lock()
	defer s.operationMu.Unlock()
	op := s.operations[id]
	return op != nil && op.cancelRequested
}

// GetProgress returns best-effort in-memory progress for a running instance backup.
func (s *InstanceBackupService) GetProgress(id uuid.UUID) (domain.InstanceBackupProgress, bool) {
	s.operationMu.Lock()
	defer s.operationMu.Unlock()
	op := s.operations[id]
	if op == nil {
		return domain.InstanceBackupProgress{}, false
	}
	return op.progress, true
}

// Cancel requests cancellation of a running instance backup.
func (s *InstanceBackupService) Cancel(ctx context.Context, id uuid.UUID) (*domain.InstanceBackup, error) {
	if ctx != nil {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
	}

	s.operationMu.Lock()
	op := s.operations[id]
	if op != nil {
		backup, err := s.repo.GetByID(id)
		if err != nil {
			s.operationMu.Unlock()
			return nil, fmt.Errorf("getting backup for cancel: %w", err)
		}
		if backup == nil {
			s.operationMu.Unlock()
			return nil, ErrInstanceBackupNotFound
		}
		if backup.Status != domain.InstanceBackupStatusRunning {
			s.operationMu.Unlock()
			return nil, ErrInstanceBackupNotRunning
		}
		op.cancelRequested = true
		op.cancel()
		op.progress = domain.InstanceBackupProgress{
			Phase:   "cancelling",
			Message: "Cancellation requested",
		}
		s.operationMu.Unlock()
		backup.ErrorMessage = "cancellation requested"
		return backup, nil
	}
	s.operationMu.Unlock()

	backup, err := s.repo.GetByID(id)
	if err != nil {
		return nil, fmt.Errorf("getting backup for cancel: %w", err)
	}
	if backup == nil {
		return nil, ErrInstanceBackupNotFound
	}
	if backup.Status != domain.InstanceBackupStatusRunning {
		return nil, ErrInstanceBackupNotRunning
	}

	backup.Status = domain.InstanceBackupStatusCancelled
	backup.ErrorMessage = "cancelled by user"
	if err := s.repo.Update(backup); err != nil {
		return nil, fmt.Errorf("updating cancelled backup: %w", err)
	}
	return backup, nil
}

func checkBackupArchiveEntryQuota(name string, size int64, limits BackupArchiveLimits) error {
	if size > limits.MaxEntryBytes {
		return newRestoreLimitError("backup archive entry %s exceeds per-entry backup limit: %d bytes > %d bytes", name, size, limits.MaxEntryBytes)
	}
	return nil
}

func checkBackupArchiveTotals(totalBytes int64, fileEntries int, limits BackupArchiveLimits) error {
	if totalBytes > limits.MaxTotalBytes {
		return newRestoreLimitError("backup archive exceeds expanded backup limit: %d bytes > %d bytes", totalBytes, limits.MaxTotalBytes)
	}
	if fileEntries > limits.MaxFileEntries {
		return newRestoreLimitError("backup archive file count exceeds backup limit: %d entries > %d entries", fileEntries, limits.MaxFileEntries)
	}
	return nil
}

func isArchiveQuotaError(err error) bool {
	var limitErr *RestoreLimitError
	return errors.As(err, &limitErr)
}

func (s *InstanceBackupService) collectArchiveSourceFiles(ctx context.Context, limits BackupArchiveLimits, initialBytes int64) ([]archiveSourceFile, int, *archiveSourceFile, int64, int, error) {
	if err := checkBackupArchiveTotals(initialBytes, 1, limits); err != nil {
		return nil, 0, nil, 0, 0, err
	}

	deviceBackupFiles := make([]archiveSourceFile, 0)
	backupFileCount := 0
	totalBytes := initialBytes
	fileEntries := 1 // database entry

	if info, err := os.Stat(s.deviceBackupDir); err == nil && info.IsDir() {
		err := filepath.Walk(s.deviceBackupDir, func(path string, info os.FileInfo, err error) error {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return ctxErr
			}
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
			if !info.Mode().IsRegular() {
				return nil
			}

			rel, err := filepath.Rel(s.deviceBackupDir, path)
			if err != nil {
				return nil
			}

			// Validate archive entry name: no absolute paths, no traversal (T-15-05)
			archiveName := filepath.ToSlash(filepath.Join("backups", rel))
			if strings.HasPrefix(archiveName, "/") || archiveEntryHasTraversal(archiveName) {
				return nil
			}
			if err := checkBackupArchiveEntryQuota(archiveName, info.Size(), limits); err != nil {
				return err
			}
			totalBytes += info.Size()
			fileEntries++
			if err := checkBackupArchiveTotals(totalBytes, fileEntries, limits); err != nil {
				return err
			}

			deviceBackupFiles = append(deviceBackupFiles, archiveSourceFile{
				archiveName: archiveName,
				diskPath:    path,
				sizeBytes:   info.Size(),
			})
			backupFileCount++
			return nil
		})
		if err != nil {
			return nil, 0, nil, 0, 0, err
		}
	} else if err != nil && !os.IsNotExist(err) {
		return nil, 0, nil, 0, 0, fmt.Errorf("statting device backup dir: %w", err)
	}

	var knownHostsFile *archiveSourceFile
	if info, err := os.Stat(s.knownHostsPath); err == nil && !info.IsDir() && info.Mode().IsRegular() {
		if err := checkBackupArchiveEntryQuota("known_hosts", info.Size(), limits); err != nil {
			return nil, 0, nil, 0, 0, err
		}
		totalBytes += info.Size()
		fileEntries++
		if err := checkBackupArchiveTotals(totalBytes, fileEntries, limits); err != nil {
			return nil, 0, nil, 0, 0, err
		}
		knownHostsFile = &archiveSourceFile{
			archiveName: "known_hosts",
			diskPath:    s.knownHostsPath,
			sizeBytes:   info.Size(),
		}
	} else if err != nil && !os.IsNotExist(err) {
		return nil, 0, nil, 0, 0, fmt.Errorf("statting known_hosts: %w", err)
	}

	return deviceBackupFiles, backupFileCount, knownHostsFile, totalBytes, fileEntries, nil
}

// createArchive builds a .tar.gz archive containing manifest, database, device backups, and known_hosts.
// Returns the total size of all archived file data.
func (s *InstanceBackupService) createArchive(
	ctx context.Context,
	archivePath string,
	dbArtifact databaseBackupArtifact,
	deviceBackupFiles []archiveSourceFile,
	knownHostsFile *archiveSourceFile,
	manifestJSON []byte,
	manifest *backupManifest,
	backupID uuid.UUID,
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
	if err := addBytesToTar(tw, "manifest.json", manifestJSON, time.Now().UTC()); err != nil {
		return 0, fmt.Errorf("adding manifest to archive: %w", err)
	}
	totalSize += int64(len(manifestJSON))
	s.updateInstanceBackupProgress(backupID, domain.InstanceBackupProgress{
		Phase:   "archiving",
		Message: "Archived manifest",
		Current: totalSize,
		Total:   manifest.TotalSizeBytes,
	})

	// Add the PostgreSQL dump.
	dbSize, err := addFileToTarContext(ctx, tw, dbArtifact.archiveEntryName, dbArtifact.tempPath)
	if err != nil {
		return 0, fmt.Errorf("adding database to archive: %w", err)
	}
	totalSize += dbSize
	s.updateInstanceBackupProgress(backupID, domain.InstanceBackupProgress{
		Phase:   "archiving",
		Message: "Archived database snapshot",
		Current: totalSize,
		Total:   manifest.TotalSizeBytes,
	})

	// Add device backup files under backups/
	for _, bf := range deviceBackupFiles {
		size, err := addCollectedFileToTarContext(ctx, tw, bf, s.BackupArchiveLimits())
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || isArchiveQuotaError(err) {
				return 0, err
			}
			log.Printf("Warning: skipping device backup file %s: %v", bf.diskPath, err)
			continue
		}
		totalSize += size
		s.updateInstanceBackupProgress(backupID, domain.InstanceBackupProgress{
			Phase:   "archiving",
			Message: "Archived " + bf.archiveName,
			Current: totalSize,
			Total:   manifest.TotalSizeBytes,
		})
	}

	// Add known_hosts if it exists
	if knownHostsFile != nil {
		size, err := addCollectedFileToTarContext(ctx, tw, *knownHostsFile, s.BackupArchiveLimits())
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || isArchiveQuotaError(err) {
				return 0, err
			}
			log.Printf("Warning: failed to add known_hosts to archive: %v", err)
		} else {
			totalSize += size
			s.updateInstanceBackupProgress(backupID, domain.InstanceBackupProgress{
				Phase:   "archiving",
				Message: "Archived known_hosts",
				Current: totalSize,
				Total:   manifest.TotalSizeBytes,
			})
		}
	}

	return totalSize, nil
}

// backupDatabase creates a logical dump of the live PostgreSQL database.
func (s *InstanceBackupService) backupDatabase(ctx context.Context, backupSubDir string) (databaseBackupArtifact, error) {
	return s.backupPostgresDatabase(ctx, filepath.Join(backupSubDir, postgresArchiveDBEntry+".tmp"))
}

func (s *InstanceBackupService) backupPostgresDatabase(ctx context.Context, destPath string) (databaseBackupArtifact, error) {
	if strings.TrimSpace(s.dbDSN) == "" {
		return databaseBackupArtifact{}, fmt.Errorf("postgres backup requires db_dsn")
	}
	if err := ensureSupportedPostgresCLITools(ctx, "pg_dump"); err != nil {
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

func manifestDatabaseEntryName(manifest backupManifest) (string, error) {
	if entry := strings.TrimSpace(manifest.DBEntryName); entry != "" {
		if entry == postgresArchiveDBEntry {
			return entry, nil
		}
		return "", fmt.Errorf("unsupported database entry %q in manifest", entry)
	}
	return postgresArchiveDBEntry, nil
}

func (s *InstanceBackupService) validatePostgresDump(ctx context.Context, dumpPath string) error {
	if err := ensureSupportedPostgresCLITools(ctx, "pg_restore"); err != nil {
		return err
	}
	if _, err := runExternalCommand(ctx, "pg_restore", "--list", dumpPath); err != nil {
		return fmt.Errorf("validating postgres dump: %w", err)
	}
	return nil
}

// addFileToTar adds a file from disk to the tar archive. Returns the file size.
func addFileToTar(tw *tar.Writer, name string, sourcePath string) (int64, error) {
	return addFileToTarContext(context.Background(), tw, name, sourcePath)
}

func addFileToTarContext(ctx context.Context, tw *tar.Writer, name string, sourcePath string) (int64, error) {
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

	written, err := copyWithContext(ctx, tw, f)
	if err != nil {
		return 0, fmt.Errorf("writing data for %s: %w", name, err)
	}
	if written != info.Size() {
		return 0, fmt.Errorf("writing data for %s: wrote %d bytes, expected %d bytes", name, written, info.Size())
	}
	return written, nil
}

func addCollectedFileToTarContext(ctx context.Context, tw *tar.Writer, source archiveSourceFile, limits BackupArchiveLimits) (int64, error) {
	f, err := os.Open(source.diskPath)
	if err != nil {
		return 0, fmt.Errorf("opening %s: %w", source.diskPath, err)
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return 0, fmt.Errorf("statting %s: %w", source.diskPath, err)
	}
	if err := checkBackupArchiveEntryQuota(source.archiveName, info.Size(), limits); err != nil {
		return 0, err
	}
	if info.Size() > source.sizeBytes {
		return 0, newRestoreLimitError("backup archive entry %s grew after collection: %d bytes > %d bytes", source.archiveName, info.Size(), source.sizeBytes)
	}

	header := &tar.Header{
		Name:    source.archiveName,
		Size:    info.Size(),
		Mode:    0644,
		ModTime: info.ModTime(),
	}
	if err := tw.WriteHeader(header); err != nil {
		return 0, fmt.Errorf("writing header for %s: %w", source.archiveName, err)
	}

	written, err := copyWithContext(ctx, tw, f)
	if err != nil {
		return 0, fmt.Errorf("writing data for %s: %w", source.archiveName, err)
	}
	if written != info.Size() {
		return 0, fmt.Errorf("writing data for %s: wrote %d bytes, expected %d bytes", source.archiveName, written, info.Size())
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
	return computeFileHashContext(context.Background(), path)
}

func computeFileHashContext(ctx context.Context, path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("opening file for hash: %w", err)
	}
	defer f.Close()

	h := sha256.New()
	if _, err := copyWithContext(ctx, h, f); err != nil {
		return "", fmt.Errorf("hashing file: %w", err)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func copyWithContext(ctx context.Context, dst io.Writer, src io.Reader) (int64, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	buf := make([]byte, 32*1024)
	var written int64
	for {
		if err := ctx.Err(); err != nil {
			return written, err
		}
		nr, er := src.Read(buf)
		if nr > 0 {
			if err := ctx.Err(); err != nil {
				return written, err
			}
			nw, ew := dst.Write(buf[:nr])
			if nw > 0 {
				written += int64(nw)
			}
			if ew != nil {
				return written, ew
			}
			if nr != nw {
				return written, io.ErrShortWrite
			}
		}
		if er != nil {
			if er == io.EOF {
				return written, nil
			}
			return written, er
		}
	}
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
	manifestPath := filepath.Join(tempDir, "manifest.json")
	manifestData, err := os.ReadFile(manifestPath)
	if err != nil {
		return nil, fmt.Errorf("archive missing manifest.json")
	}

	var manifest backupManifest
	if err := json.Unmarshal(manifestData, &manifest); err != nil {
		return nil, fmt.Errorf("parsing manifest.json: %w", err)
	}

	dbEntryName, err := manifestDatabaseEntryName(manifest)
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
	if manifest.MigrationVersion > currentVersion {
		return nil, fmt.Errorf("archive has newer migration version (%d) than current (%d); upgrade Theia first",
			manifest.MigrationVersion, currentVersion)
	}

	// Step 9: Determine if migration is needed
	needsMigration := manifest.MigrationVersion < currentVersion

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
	markerPath := filepath.Join(s.stateDir, ".theia-restore-pending")
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

	// Copy the database payload to staging.
	if err := copyFileContext(ctx, extractedDBPath, stagedDBPath); err != nil {
		return nil, cleanupStagingOnError(fmt.Errorf("staging database: %w", err))
	}

	// Copy backups/ directory if it exists
	srcBackups := filepath.Join(tempDir, "backups")
	if info, err := os.Stat(srcBackups); err == nil && info.IsDir() {
		if err := copyDirContext(ctx, srcBackups, filepath.Join(stagingDir, "backups")); err != nil {
			return nil, cleanupStagingOnError(fmt.Errorf("staging backup files: %w", err))
		}
	}

	// Copy known_hosts if it exists
	srcKnownHosts := filepath.Join(tempDir, "known_hosts")
	if _, err := os.Stat(srcKnownHosts); err == nil {
		if err := copyFileContext(ctx, srcKnownHosts, filepath.Join(stagingDir, "known_hosts")); err != nil {
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
	markerJSON, err := json.MarshalIndent(marker, "", "  ")
	if err != nil {
		return nil, cleanupStagingOnError(fmt.Errorf("marshaling marker JSON: %w", err))
	}
	if err := os.WriteFile(markerPath, markerJSON, 0600); err != nil {
		return nil, cleanupStagingOnError(fmt.Errorf("writing restore marker: %w", err))
	}
	if err := os.Chmod(markerPath, 0600); err != nil {
		return nil, cleanupStagingOnError(fmt.Errorf("restricting restore marker permissions: %w", err))
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
		return newRestoreLimitError("compressed archive exceeds restore limit: %d bytes > %d bytes", info.Size(), limits.MaxCompressedBytes)
	}
	return nil
}

// extractArchive extracts a .tar.gz archive to the given directory with security validation and quotas.
func extractArchive(archivePath, destDir string, limits RestoreArchiveLimits) error {
	return extractArchiveContext(context.Background(), archivePath, destDir, limits)
}

func extractArchiveContext(ctx context.Context, archivePath, destDir string, limits RestoreArchiveLimits) error {
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
	var archiveEntries int
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
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

		regularFile := header.Typeflag == tar.TypeReg || header.Typeflag == tar.TypeRegA

		// Security: only allow regular files and directories
		if !regularFile && header.Typeflag != tar.TypeDir {
			return fmt.Errorf("unsupported restore archive entry type %c: %s", header.Typeflag, header.Name)
		}

		entryName := strings.TrimPrefix(header.Name, "./")

		// Security: validate path has no traversal (T-17-01)
		if archiveEntryHasTraversal(entryName) {
			return fmt.Errorf("archive contains path traversal: %s", header.Name)
		}
		cleanName := filepath.Clean(entryName)
		if filepath.IsAbs(cleanName) {
			return fmt.Errorf("archive contains absolute path: %s", header.Name)
		}

		targetPath := filepath.Join(destDir, cleanName)
		archiveEntries++
		if archiveEntries > limits.MaxFileEntries {
			return newRestoreLimitError(
				"archive file count exceeds restore limit (archive entry count exceeds): %d entries > %d entries",
				archiveEntries,
				limits.MaxFileEntries,
			)
		}

		if header.Typeflag == tar.TypeDir {
			if !isAllowedRestoreArchiveDirectory(cleanName) {
				return fmt.Errorf("disallowed restore archive entry: %s", cleanName)
			}
			if err := os.MkdirAll(targetPath, 0700); err != nil {
				return fmt.Errorf("creating directory %s: %w", cleanName, err)
			}
			continue
		}

		if header.Size < 0 {
			return fmt.Errorf("archive entry %s has invalid negative size", cleanName)
		}
		if header.Size > limits.MaxEntryBytes {
			return newRestoreLimitError("archive entry %s exceeds per-entry restore limit: %d bytes > %d bytes", cleanName, header.Size, limits.MaxEntryBytes)
		}
		if header.Size > limits.MaxTotalBytes-totalBytes {
			return newRestoreLimitError("expanded archive exceeds restore limit: %d bytes > %d bytes", totalBytes+header.Size, limits.MaxTotalBytes)
		}

		// Security: regular files outside the restore archive contract are rejected.
		if !isAllowedRestoreArchiveFile(cleanName) {
			return fmt.Errorf("disallowed restore archive entry: %s", cleanName)
		}

		// Ensure parent directory exists
		if err := os.MkdirAll(filepath.Dir(targetPath), 0700); err != nil {
			return fmt.Errorf("creating parent directory for %s: %w", cleanName, err)
		}

		outFile, err := os.OpenFile(targetPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
		if err != nil {
			return fmt.Errorf("creating file %s: %w", cleanName, err)
		}
		if _, err := copyWithContext(ctx, outFile, tr); err != nil {
			outFile.Close()
			return fmt.Errorf("writing file %s: %w", cleanName, err)
		}
		outFile.Close()
		totalBytes += header.Size
	}

	return nil
}

func archiveEntryHasTraversal(name string) bool {
	normalized := strings.ReplaceAll(name, "\\", "/")
	for _, part := range strings.Split(normalized, "/") {
		if part == ".." {
			return true
		}
	}
	return false
}

// isAllowedRestoreArchiveFile checks if a regular file entry matches the restore archive contract.
func isAllowedRestoreArchiveFile(name string) bool {
	normalized := strings.ReplaceAll(name, "\\", "/")
	switch normalized {
	case "manifest.json", postgresArchiveDBEntry, "known_hosts":
		return true
	default:
		return strings.HasPrefix(normalized, "backups/")
	}
}

// isAllowedRestoreArchiveDirectory checks if a directory entry matches the restore archive contract.
func isAllowedRestoreArchiveDirectory(name string) bool {
	normalized := strings.ReplaceAll(name, "\\", "/")
	return normalized == "backups" || strings.HasPrefix(normalized, "backups/")
}

// copyFile copies a single file from src to dst with private file permissions.
func copyFile(src, dst string) error {
	return copyFileContext(context.Background(), src, dst)
}

func copyFileContext(ctx context.Context, src, dst string) error {
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

	if _, err := copyWithContext(ctx, out, in); err != nil {
		return fmt.Errorf("copying %s to %s: %w", src, dst, err)
	}

	return os.Chmod(dst, 0600)
}

// copyDir recursively copies a directory from srcDir to dstDir.
func copyDir(srcDir, dstDir string) error {
	return copyDirContext(context.Background(), srcDir, dstDir)
}

func copyDirContext(ctx context.Context, srcDir, dstDir string) error {
	return filepath.Walk(srcDir, func(path string, info os.FileInfo, err error) error {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
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

		return copyFileContext(ctx, path, target)
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
