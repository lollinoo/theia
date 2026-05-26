package service

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/pkg/sftp"
	gossh "golang.org/x/crypto/ssh"

	"github.com/lollinoo/theia/internal/crypto"
	"github.com/lollinoo/theia/internal/domain"
	"github.com/lollinoo/theia/internal/ssh"
	"github.com/lollinoo/theia/internal/vendor"
	"golang.org/x/sys/unix"
)

var sanitizeRe = regexp.MustCompile(`[^a-zA-Z0-9_.-]`)

const defaultBulkBackupWorkerCount = 4
const defaultBulkBackupRunBatchSize = 10

var ErrBulkBackupRunAlreadyActive = errors.New("bulk backup run already active")

// BulkOperationLimits holds defensive quotas for bulk backup and download requests.
type BulkOperationLimits struct {
	BulkBackupMaxDevices    int
	BulkBackupMaxQueuedJobs int
	BulkDownloadMaxDevices  int
	BulkDownloadMaxFiles    int
	BulkDownloadMaxBytes    int64
}

// DefaultBulkOperationLimits bounds bulk workflows while preserving ordinary fleet use.
var DefaultBulkOperationLimits = BulkOperationLimits{
	BulkBackupMaxDevices:    100,
	BulkBackupMaxQueuedJobs: 100,
	BulkDownloadMaxDevices:  100,
	BulkDownloadMaxFiles:    500,
	BulkDownloadMaxBytes:    512 << 20,
}

// BulkLimitError reports a request that exceeds a configured bulk quota.
type BulkLimitError struct {
	Operation string
	Limit     string
	Max       int64
	Actual    int64
}

func (e *BulkLimitError) Error() string {
	return fmt.Sprintf("%s exceeds %s limit: requested %d, maximum %d", e.Operation, e.Limit, e.Actual, e.Max)
}

// IsBulkLimitError reports whether err is a bulk quota rejection.
func IsBulkLimitError(err error) bool {
	var target *BulkLimitError
	return errors.As(err, &target)
}

// BulkPathError reports an unsafe backup file path or zip entry name.
type BulkPathError struct {
	Path   string
	Reason string
}

func (e *BulkPathError) Error() string {
	if e.Path == "" {
		return e.Reason
	}
	return fmt.Sprintf("%s: %s", e.Reason, e.Path)
}

// IsBulkPathError reports whether err is an unsafe bulk download path rejection.
func IsBulkPathError(err error) bool {
	var target *BulkPathError
	return errors.As(err, &target)
}

// BackupService orchestrates credential profile management and config backups.
type BackupService struct {
	jobRepo               domain.BackupJobRepository
	fileRepo              domain.BackupFileRepository
	credentialProfileRepo domain.CredentialProfileRepository
	deviceRepo            domain.DeviceRepository
	settingsRepo          domain.SettingsRepository
	vendorRegistry        *vendor.Registry
	sshDialer             ssh.Dialer
	encryptionKey         []byte
	backupDir             string
	hostKeyCallback       gossh.HostKeyCallback
	deviceLocks           sync.Map // per-device mutex: map[uuid.UUID]*sync.Mutex
	bulkLimits            BulkOperationLimits
	bulkRunRepo           domain.BulkBackupRunRepository
}

type BackupServiceOption func(*BackupService)

func WithBulkBackupRunRepo(repo domain.BulkBackupRunRepository) BackupServiceOption {
	return func(s *BackupService) {
		s.bulkRunRepo = repo
	}
}

// NewBackupService creates a new BackupService.
func NewBackupService(
	jobRepo domain.BackupJobRepository,
	fileRepo domain.BackupFileRepository,
	credentialProfileRepo domain.CredentialProfileRepository,
	deviceRepo domain.DeviceRepository,
	settingsRepo domain.SettingsRepository,
	vendorRegistry *vendor.Registry,
	sshDialer ssh.Dialer,
	encryptionKey []byte,
	backupDir string,
	hostKeyCallback gossh.HostKeyCallback,
	opts ...BackupServiceOption,
) *BackupService {
	svc := &BackupService{
		jobRepo:               jobRepo,
		fileRepo:              fileRepo,
		credentialProfileRepo: credentialProfileRepo,
		deviceRepo:            deviceRepo,
		settingsRepo:          settingsRepo,
		vendorRegistry:        vendorRegistry,
		sshDialer:             sshDialer,
		encryptionKey:         encryptionKey,
		backupDir:             backupDir,
		hostKeyCallback:       hostKeyCallback,
		bulkLimits:            DefaultBulkOperationLimits,
	}
	for _, opt := range opts {
		opt(svc)
	}
	return svc
}

// SetBulkOperationLimits overrides bulk request quotas.
func (s *BackupService) SetBulkOperationLimits(limits BulkOperationLimits) {
	s.bulkLimits = normalizeBulkOperationLimits(limits)
}

// BulkOperationLimits returns the effective bulk request quotas.
func (s *BackupService) BulkOperationLimits() BulkOperationLimits {
	return normalizeBulkOperationLimits(s.bulkLimits)
}

func normalizeBulkOperationLimits(limits BulkOperationLimits) BulkOperationLimits {
	defaults := DefaultBulkOperationLimits
	if limits.BulkBackupMaxDevices <= 0 {
		limits.BulkBackupMaxDevices = defaults.BulkBackupMaxDevices
	}
	if limits.BulkBackupMaxQueuedJobs <= 0 {
		limits.BulkBackupMaxQueuedJobs = defaults.BulkBackupMaxQueuedJobs
	}
	if limits.BulkDownloadMaxDevices <= 0 {
		limits.BulkDownloadMaxDevices = defaults.BulkDownloadMaxDevices
	}
	if limits.BulkDownloadMaxFiles <= 0 {
		limits.BulkDownloadMaxFiles = defaults.BulkDownloadMaxFiles
	}
	if limits.BulkDownloadMaxBytes <= 0 {
		limits.BulkDownloadMaxBytes = defaults.BulkDownloadMaxBytes
	}
	return limits
}

// getDeviceLock returns or creates a per-device mutex.
func (s *BackupService) getDeviceLock(deviceID uuid.UUID) *sync.Mutex {
	val, _ := s.deviceLocks.LoadOrStore(deviceID, &sync.Mutex{})
	return val.(*sync.Mutex)
}

// nowInConfiguredTZ returns the current time in the timezone configured in settings.
// Falls back to UTC if the setting is missing or invalid.
func (s *BackupService) nowInConfiguredTZ() time.Time {
	tzName, err := s.settingsRepo.Get(domain.SettingTimezone)
	if err != nil || tzName == "" {
		return time.Now().UTC()
	}
	loc, err := time.LoadLocation(tzName)
	if err != nil {
		return time.Now().UTC()
	}
	return time.Now().In(loc)
}

// BulkBackupResult describes the outcome of a bulk backup request per device.
type BulkBackupResult struct {
	DeviceID   uuid.UUID  `json:"device_id"`
	DeviceName string     `json:"device_name"`
	Status     string     `json:"status"` // "queued", "skipped"
	Reason     string     `json:"reason,omitempty"`
	JobID      *uuid.UUID `json:"job_id,omitempty"`
}

type queuedDeviceBackup struct {
	device    domain.Device
	profile   *domain.CredentialProfile
	backupCfg vendor.BackupConfig
	jobID     uuid.UUID
}

type preparedDeviceBackup struct {
	device      domain.Device
	profile     *domain.CredentialProfile
	backupCfg   vendor.BackupConfig
	resultIndex int
}

// TriggerBulkBackup validates all devices and queues backups for eligible ones.
func (s *BackupService) TriggerBulkBackup(ctx context.Context, requestedDeviceIDs ...uuid.UUID) ([]BulkBackupResult, error) {
	devices, err := s.bulkBackupDevices(ctx, requestedDeviceIDs)
	if err != nil {
		return nil, err
	}

	var results []BulkBackupResult
	preparedBackups := make([]preparedDeviceBackup, 0)
	for i := range devices {
		if err := contextError(ctx); err != nil {
			return nil, err
		}
		d := &devices[i]
		name := d.Tags["display_name"]
		if name == "" {
			name = d.SysName
		}
		if name == "" {
			name = d.IP
		}

		profile, err := s.credentialProfileRepo.GetBackupProfileForDevice(d.ID)
		if err != nil {
			results = append(results, BulkBackupResult{
				DeviceID: d.ID, DeviceName: name,
				Status: "skipped", Reason: "no credential profile assigned",
			})
			continue
		}

		backupCfg := s.vendorRegistry.ResolveBackupConfig(d.Vendor)
		if !backupCfg.Supported {
			results = append(results, BulkBackupResult{
				DeviceID: d.ID, DeviceName: name,
				Status: "skipped", Reason: "backup not supported for vendor",
			})
			continue
		}

		// Fast reachability check before creating the job
		if err := ssh.CheckReachable(d.IP, profile.Port, 5*time.Second); err != nil {
			results = append(results, BulkBackupResult{
				DeviceID: d.ID, DeviceName: name,
				Status: "skipped", Reason: "device unreachable",
			})
			continue
		}

		results = append(results, BulkBackupResult{
			DeviceID: d.ID, DeviceName: name,
			Status: "queued",
		})
		preparedBackups = append(preparedBackups, preparedDeviceBackup{
			device:      *d,
			profile:     profile,
			backupCfg:   backupCfg,
			resultIndex: len(results) - 1,
		})
	}

	limits := s.BulkOperationLimits()
	if len(preparedBackups) > limits.BulkBackupMaxQueuedJobs {
		return nil, &BulkLimitError{
			Operation: "bulk backup",
			Limit:     "queued jobs",
			Max:       int64(limits.BulkBackupMaxQueuedJobs),
			Actual:    int64(len(preparedBackups)),
		}
	}

	queuedBackups := make([]queuedDeviceBackup, 0, len(preparedBackups))
	for _, prepared := range preparedBackups {
		if err := contextError(ctx); err != nil {
			return nil, err
		}
		job := &domain.BackupJob{
			ID:       uuid.New(),
			DeviceID: prepared.device.ID,
			Status:   domain.BackupStatusPending,
		}
		if err := s.jobRepo.Create(job); err != nil {
			results[prepared.resultIndex].Status = "skipped"
			results[prepared.resultIndex].Reason = fmt.Sprintf("failed to create job: %v", err)
			continue
		}

		queuedBackups = append(queuedBackups, queuedDeviceBackup{
			device:    prepared.device,
			profile:   prepared.profile,
			backupCfg: prepared.backupCfg,
			jobID:     job.ID,
		})

		jobID := job.ID
		results[prepared.resultIndex].JobID = &jobID
	}

	s.startBulkBackupWorkers(queuedBackups)

	return results, nil
}

func (s *BackupService) startBulkBackupWorkers(queuedBackups []queuedDeviceBackup) {
	if len(queuedBackups) == 0 {
		return
	}

	workerCount := defaultBulkBackupWorkerCount
	if len(queuedBackups) < workerCount {
		workerCount = len(queuedBackups)
	}

	go func() {
		jobs := make(chan queuedDeviceBackup)
		var wg sync.WaitGroup
		wg.Add(workerCount)
		for i := 0; i < workerCount; i++ {
			go func() {
				defer wg.Done()
				for job := range jobs {
					device := job.device
					s.runFullBackup(&device, job.profile, job.backupCfg, job.jobID)
				}
			}()
		}
		for _, job := range queuedBackups {
			jobs <- job
		}
		close(jobs)
		wg.Wait()
	}()
}

func (s *BackupService) StartBulkBackupRun(ctx context.Context, requestedDeviceIDs []uuid.UUID, createdBy string) (*domain.BulkBackupRun, error) {
	if s.bulkRunRepo == nil {
		return nil, errors.New("bulk backup run repository is not configured")
	}
	if active, err := s.bulkRunRepo.GetActiveRun(); err != nil {
		return nil, fmt.Errorf("checking active bulk backup run: %w", err)
	} else if active != nil {
		return active, ErrBulkBackupRunAlreadyActive
	}
	devices, err := s.bulkBackupRunDevices(ctx, requestedDeviceIDs)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	run := &domain.BulkBackupRun{
		ID:        uuid.New(),
		Status:    domain.BulkBackupRunStatusRunning,
		BatchSize: defaultBulkBackupRunBatchSize,
		CreatedBy: createdBy,
		CreatedAt: now,
		StartedAt: &now,
	}
	items := make([]domain.BulkBackupRunItem, 0, len(devices))
	for i := range devices {
		device := devices[i]
		status := domain.BulkBackupRunItemStatusChecking
		reason := ""
		completedAt := (*time.Time)(nil)
		if device.Status == domain.DeviceStatusDown {
			status = domain.BulkBackupRunItemStatusSkipped
			reason = "device offline"
			doneAt := now
			completedAt = &doneAt
		}
		items = append(items, domain.BulkBackupRunItem{
			ID:          uuid.New(),
			RunID:       run.ID,
			DeviceID:    device.ID,
			DeviceName:  bulkBackupDeviceName(device),
			Status:      status,
			Reason:      reason,
			CreatedAt:   now,
			UpdatedAt:   now,
			CompletedAt: completedAt,
		})
	}
	if err := s.bulkRunRepo.CreateRun(run, items); err != nil {
		return nil, fmt.Errorf("creating bulk backup run: %w", err)
	}
	if _, err := s.bulkRunRepo.RecalculateRunCounters(run.ID); err != nil {
		return nil, fmt.Errorf("calculating bulk backup run counters: %w", err)
	}
	go s.processBulkBackupRun(run.ID)
	return s.bulkRunRepo.GetRun(run.ID)
}

func (s *BackupService) GetBulkBackupRun(ctx context.Context, id uuid.UUID) (*domain.BulkBackupRun, error) {
	if err := contextError(ctx); err != nil {
		return nil, err
	}
	if s.bulkRunRepo == nil {
		return nil, errors.New("bulk backup run repository is not configured")
	}
	return s.bulkRunRepo.GetRun(id)
}

func (s *BackupService) GetLatestBulkBackupRun(ctx context.Context) (*domain.BulkBackupRun, error) {
	if err := contextError(ctx); err != nil {
		return nil, err
	}
	if s.bulkRunRepo == nil {
		return nil, errors.New("bulk backup run repository is not configured")
	}
	return s.bulkRunRepo.GetLatestRun()
}

func (s *BackupService) CancelBulkBackupRun(ctx context.Context, id uuid.UUID) (*domain.BulkBackupRun, error) {
	if err := contextError(ctx); err != nil {
		return nil, err
	}
	if s.bulkRunRepo == nil {
		return nil, errors.New("bulk backup run repository is not configured")
	}
	run, err := s.bulkRunRepo.GetRun(id)
	if err != nil || run == nil {
		return run, err
	}
	wasPaused := run.Status == domain.BulkBackupRunStatusPaused
	if run.Status == domain.BulkBackupRunStatusRunning ||
		run.Status == domain.BulkBackupRunStatusPausing ||
		run.Status == domain.BulkBackupRunStatusPaused {
		run.Status = domain.BulkBackupRunStatusCancelling
	}
	run.CancelRequested = true
	if err := s.bulkRunRepo.UpdateRun(run); err != nil {
		return nil, err
	}
	if wasPaused {
		go s.processBulkBackupRun(id)
	}
	return s.bulkRunRepo.GetRun(id)
}

func (s *BackupService) PauseBulkBackupRun(ctx context.Context, id uuid.UUID) (*domain.BulkBackupRun, error) {
	if err := contextError(ctx); err != nil {
		return nil, err
	}
	if s.bulkRunRepo == nil {
		return nil, errors.New("bulk backup run repository is not configured")
	}
	run, err := s.bulkRunRepo.GetRun(id)
	if err != nil || run == nil {
		return run, err
	}
	if run.Status == domain.BulkBackupRunStatusRunning {
		run.Status = domain.BulkBackupRunStatusPausing
	}
	if err := s.bulkRunRepo.UpdateRun(run); err != nil {
		return nil, err
	}
	return s.bulkRunRepo.GetRun(id)
}

func (s *BackupService) ResumeBulkBackupRun(ctx context.Context, id uuid.UUID) (*domain.BulkBackupRun, error) {
	if err := contextError(ctx); err != nil {
		return nil, err
	}
	if s.bulkRunRepo == nil {
		return nil, errors.New("bulk backup run repository is not configured")
	}
	run, err := s.bulkRunRepo.GetRun(id)
	if err != nil || run == nil {
		return run, err
	}
	wasPaused := run.Status == domain.BulkBackupRunStatusPaused
	if run.Status == domain.BulkBackupRunStatusPaused || run.Status == domain.BulkBackupRunStatusPausing {
		run.Status = domain.BulkBackupRunStatusRunning
	}
	if err := s.bulkRunRepo.UpdateRun(run); err != nil {
		return nil, err
	}
	if wasPaused {
		go s.processBulkBackupRun(id)
	}
	return s.bulkRunRepo.GetRun(id)
}

func (s *BackupService) ResumeBulkBackupRuns(ctx context.Context) {
	if s.bulkRunRepo == nil {
		return
	}
	runs, err := s.bulkRunRepo.ListResumableRuns()
	if err != nil {
		log.Printf("Warning: failed to list resumable bulk backup runs: %v", err)
		return
	}
	for _, run := range runs {
		if err := contextError(ctx); err != nil {
			return
		}
		for _, item := range run.Items {
			if !bulkRunItemTerminal(item.Status) {
				if item.BackupJobID != nil {
					s.markInterruptedBackupJobFailed(*item.BackupJobID)
				}
				item.Status = domain.BulkBackupRunItemStatusChecking
				item.Reason = ""
				item.BackupJobID = nil
				item.CompletedAt = nil
				item.UpdatedAt = time.Now().UTC()
				if err := s.bulkRunRepo.UpdateRunItem(&item); err != nil {
					log.Printf("Warning: failed to reset bulk backup run item %s: %v", item.ID, err)
				}
			}
		}
		if run.Status == domain.BulkBackupRunStatusPausing {
			run.Status = domain.BulkBackupRunStatusPaused
			run.CancelRequested = false
			if err := s.bulkRunRepo.UpdateRun(&run); err != nil {
				log.Printf("Warning: failed to pause bulk backup run %s after restart: %v", run.ID, err)
			}
			continue
		}
		go s.processBulkBackupRun(run.ID)
	}
}

func (s *BackupService) processBulkBackupRun(runID uuid.UUID) {
	if s.bulkRunRepo == nil {
		return
	}
	for {
		run, err := s.bulkRunRepo.GetRun(runID)
		if err != nil {
			log.Printf("Warning: failed to load bulk backup run %s: %v", runID, err)
			return
		}
		if run == nil || bulkRunTerminal(run.Status) {
			return
		}
		if run.Status == domain.BulkBackupRunStatusPaused {
			return
		}
		if run.Status == domain.BulkBackupRunStatusPausing {
			latest, err := s.bulkRunRepo.GetRun(runID)
			if err != nil {
				log.Printf("Warning: failed to reload pausing bulk backup run %s: %v", runID, err)
				return
			}
			if latest == nil || latest.Status != domain.BulkBackupRunStatusPausing {
				continue
			}
			latest.Status = domain.BulkBackupRunStatusPaused
			if err := s.bulkRunRepo.UpdateRun(latest); err != nil {
				log.Printf("Warning: failed to pause bulk backup run %s: %v", run.ID, err)
			}
			return
		}
		if run.CancelRequested {
			s.cancelPendingBulkRunItems(run.ID, run.Items)
			refreshed, err := s.bulkRunRepo.GetRun(runID)
			if err != nil {
				log.Printf("Warning: failed to reload cancelling bulk backup run %s: %v", runID, err)
				return
			}
			if refreshed == nil {
				return
			}
			active := runningBulkRunItems(refreshed.Items)
			if len(active) == 0 {
				s.finishBulkBackupRun(run.ID)
				return
			}
			s.waitForBulkRunBatch(run.ID, active)
			continue
		}
		items := nextBulkRunBatch(run.Items, run.BatchSize)
		if len(items) == 0 {
			s.finishBulkBackupRun(run.ID)
			return
		}
		queued := s.prepareBulkRunBatch(items)
		if len(queued) > 0 {
			s.startBulkBackupWorkers(queued)
			s.waitForBulkRunBatch(run.ID, items)
		}
	}
}

func (s *BackupService) prepareBulkRunBatch(items []domain.BulkBackupRunItem) []queuedDeviceBackup {
	queued := make([]queuedDeviceBackup, 0, len(items))
	now := time.Now().UTC()
	for _, item := range items {
		device, err := s.deviceRepo.GetByID(item.DeviceID)
		if err != nil {
			s.completeBulkRunItem(item, domain.BulkBackupRunItemStatusFailed, fmt.Sprintf("getting device: %v", err), nil)
			continue
		}
		if device.Status == domain.DeviceStatusDown {
			s.completeBulkRunItem(item, domain.BulkBackupRunItemStatusSkipped, "device offline", nil)
			continue
		}
		item.DeviceName = bulkBackupDeviceName(*device)
		profile, err := s.credentialProfileRepo.GetBackupProfileForDevice(device.ID)
		if err != nil {
			s.completeBulkRunItem(item, domain.BulkBackupRunItemStatusSkipped, "no credential profile assigned", nil)
			continue
		}
		backupCfg := s.vendorRegistry.ResolveBackupConfig(device.Vendor)
		if !backupCfg.Supported {
			s.completeBulkRunItem(item, domain.BulkBackupRunItemStatusSkipped, "backup not supported for vendor", nil)
			continue
		}
		if err := ssh.CheckReachable(device.IP, profile.Port, 5*time.Second); err != nil {
			s.completeBulkRunItem(item, domain.BulkBackupRunItemStatusSkipped, "device unreachable", nil)
			continue
		}
		job := &domain.BackupJob{ID: uuid.New(), DeviceID: device.ID, Status: domain.BackupStatusPending}
		if err := s.jobRepo.Create(job); err != nil {
			s.completeBulkRunItem(item, domain.BulkBackupRunItemStatusFailed, fmt.Sprintf("failed to create job: %v", err), nil)
			continue
		}
		item.Status = domain.BulkBackupRunItemStatusQueued
		item.Reason = ""
		item.BackupJobID = &job.ID
		item.UpdatedAt = now
		item.CompletedAt = nil
		if err := s.bulkRunRepo.UpdateRunItem(&item); err != nil {
			log.Printf("Warning: failed to update bulk backup run item %s: %v", item.ID, err)
			continue
		}
		s.recalculateBulkRunCounters(item.RunID)
		queued = append(queued, queuedDeviceBackup{
			device:    *device,
			profile:   profile,
			backupCfg: backupCfg,
			jobID:     job.ID,
		})
	}
	return queued
}

func (s *BackupService) waitForBulkRunBatch(runID uuid.UUID, batch []domain.BulkBackupRunItem) {
	batchIDs := make(map[uuid.UUID]struct{}, len(batch))
	for _, item := range batch {
		batchIDs[item.ID] = struct{}{}
	}
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		<-ticker.C
		run, err := s.bulkRunRepo.GetRun(runID)
		if err != nil || run == nil {
			return
		}
		complete := true
		for _, item := range run.Items {
			if _, ok := batchIDs[item.ID]; !ok || bulkRunItemTerminal(item.Status) {
				continue
			}
			complete = false
			if item.BackupJobID == nil {
				continue
			}
			job, err := s.jobRepo.GetByID(*item.BackupJobID)
			if err != nil || job == nil {
				continue
			}
			switch job.Status {
			case domain.BackupStatusRunning:
				item.Status = domain.BulkBackupRunItemStatusRunning
				item.UpdatedAt = time.Now().UTC()
				if err := s.bulkRunRepo.UpdateRunItem(&item); err != nil {
					log.Printf("Warning: failed to update running bulk run item %s: %v", item.ID, err)
				}
			case domain.BackupStatusSuccess:
				s.completeBulkRunItem(item, domain.BulkBackupRunItemStatusSuccess, "", nil)
			case domain.BackupStatusFailed:
				s.completeBulkRunItem(item, domain.BulkBackupRunItemStatusFailed, job.ErrorMessage, nil)
			}
		}
		if complete {
			return
		}
	}
}

func (s *BackupService) completeBulkRunItem(item domain.BulkBackupRunItem, status domain.BulkBackupRunItemStatus, reason string, jobID *uuid.UUID) {
	now := time.Now().UTC()
	item.Status = status
	item.Reason = reason
	item.BackupJobID = jobID
	item.UpdatedAt = now
	item.CompletedAt = &now
	if err := s.bulkRunRepo.UpdateRunItem(&item); err != nil {
		log.Printf("Warning: failed to complete bulk backup run item %s: %v", item.ID, err)
		return
	}
	s.recalculateBulkRunCounters(item.RunID)
}

func (s *BackupService) cancelPendingBulkRunItems(runID uuid.UUID, items []domain.BulkBackupRunItem) {
	for _, item := range items {
		if bulkRunItemTerminal(item.Status) || item.Status == domain.BulkBackupRunItemStatusRunning {
			continue
		}
		s.completeBulkRunItem(item, domain.BulkBackupRunItemStatusCancelled, "bulk backup cancelled", nil)
	}
	s.recalculateBulkRunCounters(runID)
}

func runningBulkRunItems(items []domain.BulkBackupRunItem) []domain.BulkBackupRunItem {
	running := make([]domain.BulkBackupRunItem, 0)
	for _, item := range items {
		if item.Status == domain.BulkBackupRunItemStatusRunning {
			running = append(running, item)
		}
	}
	return running
}

func (s *BackupService) finishBulkBackupRun(runID uuid.UUID) {
	run, err := s.bulkRunRepo.RecalculateRunCounters(runID)
	if err != nil || run == nil {
		if err != nil {
			log.Printf("Warning: failed to recalculate bulk backup run %s: %v", runID, err)
		}
		return
	}
	now := time.Now().UTC()
	run.CompletedAt = &now
	if run.CancelRequested || run.CancelledCount > 0 {
		if run.SuccessCount > 0 || run.FailedCount > 0 || run.SkippedCount > 0 {
			run.Status = domain.BulkBackupRunStatusPartial
		} else {
			run.Status = domain.BulkBackupRunStatusCancelled
		}
	} else if run.TotalCount > 0 && run.SuccessCount == run.TotalCount {
		run.Status = domain.BulkBackupRunStatusSuccess
	} else if run.SuccessCount > 0 || run.SkippedCount > 0 {
		run.Status = domain.BulkBackupRunStatusPartial
	} else {
		run.Status = domain.BulkBackupRunStatusFailed
	}
	if err := s.bulkRunRepo.UpdateRun(run); err != nil {
		log.Printf("Warning: failed to finish bulk backup run %s: %v", runID, err)
	}
}

func (s *BackupService) recalculateBulkRunCounters(runID uuid.UUID) {
	if _, err := s.bulkRunRepo.RecalculateRunCounters(runID); err != nil {
		log.Printf("Warning: failed to recalculate bulk backup run %s: %v", runID, err)
	}
}

func (s *BackupService) markInterruptedBackupJobFailed(jobID uuid.UUID) {
	job, err := s.jobRepo.GetByID(jobID)
	if err != nil || job == nil {
		return
	}
	if job.Status != domain.BackupStatusPending && job.Status != domain.BackupStatusRunning {
		return
	}
	job.Status = domain.BackupStatusFailed
	job.ErrorMessage = "interrupted by server restart"
	if err := s.jobRepo.Update(job); err != nil {
		log.Printf("Warning: failed to mark interrupted backup job %s failed: %v", jobID, err)
	}
}

func nextBulkRunBatch(items []domain.BulkBackupRunItem, batchSize int) []domain.BulkBackupRunItem {
	if batchSize <= 0 {
		batchSize = defaultBulkBackupRunBatchSize
	}
	batch := make([]domain.BulkBackupRunItem, 0, batchSize)
	for _, item := range items {
		if bulkRunItemTerminal(item.Status) {
			continue
		}
		batch = append(batch, item)
		if len(batch) == batchSize {
			break
		}
	}
	return batch
}

func bulkRunTerminal(status domain.BulkBackupRunStatus) bool {
	switch status {
	case domain.BulkBackupRunStatusSuccess,
		domain.BulkBackupRunStatusPartial,
		domain.BulkBackupRunStatusFailed,
		domain.BulkBackupRunStatusCancelled:
		return true
	default:
		return false
	}
}

func bulkRunItemTerminal(status domain.BulkBackupRunItemStatus) bool {
	switch status {
	case domain.BulkBackupRunItemStatusSkipped,
		domain.BulkBackupRunItemStatusSuccess,
		domain.BulkBackupRunItemStatusFailed,
		domain.BulkBackupRunItemStatusCancelled:
		return true
	default:
		return false
	}
}

func bulkBackupDeviceName(device domain.Device) string {
	name := device.Tags["display_name"]
	if name == "" {
		name = device.SysName
	}
	if name == "" {
		name = device.IP
	}
	return name
}

func (s *BackupService) bulkBackupDevices(ctx context.Context, requestedDeviceIDs []uuid.UUID) ([]domain.Device, error) {
	limits := s.BulkOperationLimits()
	if len(requestedDeviceIDs) == 0 {
		if err := contextError(ctx); err != nil {
			return nil, err
		}
		devices, err := s.deviceRepo.GetAll()
		if err != nil {
			return nil, fmt.Errorf("fetching devices: %w", err)
		}
		if len(devices) > limits.BulkBackupMaxDevices {
			return nil, &BulkLimitError{
				Operation: "bulk backup",
				Limit:     "devices",
				Max:       int64(limits.BulkBackupMaxDevices),
				Actual:    int64(len(devices)),
			}
		}
		return devices, nil
	}

	uniqueIDs := dedupeUUIDs(requestedDeviceIDs)
	if len(uniqueIDs) > limits.BulkBackupMaxDevices {
		return nil, &BulkLimitError{
			Operation: "bulk backup",
			Limit:     "devices",
			Max:       int64(limits.BulkBackupMaxDevices),
			Actual:    int64(len(uniqueIDs)),
		}
	}

	devices := make([]domain.Device, 0, len(uniqueIDs))
	for _, id := range uniqueIDs {
		if err := contextError(ctx); err != nil {
			return nil, err
		}
		device, err := s.deviceRepo.GetByID(id)
		if err != nil || device == nil {
			continue
		}
		devices = append(devices, *device)
	}
	return devices, nil
}

func (s *BackupService) bulkBackupRunDevices(ctx context.Context, requestedDeviceIDs []uuid.UUID) ([]domain.Device, error) {
	if len(requestedDeviceIDs) == 0 {
		if err := contextError(ctx); err != nil {
			return nil, err
		}
		devices, err := s.deviceRepo.GetAll()
		if err != nil {
			return nil, fmt.Errorf("fetching devices: %w", err)
		}
		return devices, nil
	}

	uniqueIDs := dedupeUUIDs(requestedDeviceIDs)
	devices := make([]domain.Device, 0, len(uniqueIDs))
	for _, id := range uniqueIDs {
		if err := contextError(ctx); err != nil {
			return nil, err
		}
		device, err := s.deviceRepo.GetByID(id)
		if err != nil || device == nil {
			continue
		}
		devices = append(devices, *device)
	}
	return devices, nil
}

// BulkDownloadEntry pairs a backup file with a device-derived folder name.
type BulkDownloadEntry struct {
	File      domain.BackupFile
	DeviceDir string // sanitized device name for zip folder
	ZipPath   string // slash-separated, prevalidated zip entry path
	SizeBytes int64
}

// GetBulkDownloadFiles returns file entries from the latest successful backup of each given device.
func (s *BackupService) GetBulkDownloadFiles(ctx context.Context, deviceIDs []uuid.UUID) ([]BulkDownloadEntry, error) {
	limits := s.BulkOperationLimits()
	deviceIDs = dedupeUUIDs(deviceIDs)
	if len(deviceIDs) > limits.BulkDownloadMaxDevices {
		return nil, &BulkLimitError{
			Operation: "bulk download",
			Limit:     "devices",
			Max:       int64(limits.BulkDownloadMaxDevices),
			Actual:    int64(len(deviceIDs)),
		}
	}

	var entries []BulkDownloadEntry
	var totalBytes int64
	var backupRoot string
	for _, did := range deviceIDs {
		if err := contextError(ctx); err != nil {
			return nil, err
		}
		device, err := s.deviceRepo.GetByID(did)
		if err != nil || device == nil {
			continue
		}
		job, err := s.jobRepo.GetLatestByDeviceID(did)
		if err != nil || job == nil {
			continue
		}
		files, err := s.fileRepo.GetByJobID(job.ID)
		if err != nil {
			continue
		}
		dirName := device.Tags["display_name"]
		if dirName == "" {
			dirName = device.SysName
		}
		if dirName == "" {
			dirName = device.Hostname
		}
		if dirName == "" {
			dirName = device.IP
		}
		dirName = sanitizeHostname(dirName)

		for _, f := range files {
			if len(entries)+1 > limits.BulkDownloadMaxFiles {
				return nil, &BulkLimitError{
					Operation: "bulk download",
					Limit:     "files",
					Max:       int64(limits.BulkDownloadMaxFiles),
					Actual:    int64(len(entries) + 1),
				}
			}
			if backupRoot == "" {
				backupRoot, err = validatedBackupRoot(s.backupDir)
				if err != nil {
					return nil, err
				}
			}
			filePath, sizeBytes, err := validateBulkDownloadFile(backupRoot, f.FilePath)
			if err != nil {
				return nil, err
			}
			if totalBytes > limits.BulkDownloadMaxBytes-sizeBytes {
				return nil, &BulkLimitError{
					Operation: "bulk download",
					Limit:     "bytes",
					Max:       limits.BulkDownloadMaxBytes,
					Actual:    saturatedInt64Sum(totalBytes, sizeBytes),
				}
			}
			zipPath, err := safeBulkDownloadZipPath(dirName, f.FileName)
			if err != nil {
				return nil, err
			}
			totalBytes += sizeBytes
			f.FilePath = filePath
			entries = append(entries, BulkDownloadEntry{
				File:      f,
				DeviceDir: dirName,
				ZipPath:   zipPath,
				SizeBytes: sizeBytes,
			})
		}
	}
	return entries, nil
}

func validatedBackupRoot(backupDir string) (string, error) {
	backupDir = strings.TrimSpace(backupDir)
	if backupDir == "" {
		return "", &BulkPathError{Reason: "backup directory is not configured"}
	}
	absRoot, err := filepath.Abs(backupDir)
	if err != nil {
		return "", fmt.Errorf("resolving backup directory: %w", err)
	}
	root, err := filepath.EvalSymlinks(absRoot)
	if err != nil {
		return "", fmt.Errorf("resolving backup directory symlinks: %w", err)
	}
	return root, nil
}

func validateBulkDownloadFile(backupRoot, filePath string) (string, int64, error) {
	if strings.TrimSpace(filePath) == "" {
		return "", 0, &BulkPathError{Reason: "backup file path is empty"}
	}
	absPath, err := filepath.Abs(filePath)
	if err != nil {
		return "", 0, fmt.Errorf("resolving backup file path: %w", err)
	}
	resolvedPath, err := filepath.EvalSymlinks(absPath)
	if err != nil {
		return "", 0, fmt.Errorf("resolving backup file symlinks: %w", err)
	}
	if !pathIsUnderDir(backupRoot, resolvedPath) {
		return "", 0, &BulkPathError{Path: filePath, Reason: "backup file path is outside backup directory"}
	}
	info, err := os.Stat(resolvedPath)
	if err != nil {
		return "", 0, fmt.Errorf("stat backup file: %w", err)
	}
	if !info.Mode().IsRegular() {
		return "", 0, &BulkPathError{Path: filePath, Reason: "backup file path is not a regular file"}
	}
	return resolvedPath, info.Size(), nil
}

// OpenBulkDownloadEntry opens a previously selected bulk download entry and
// revalidates the opened file descriptor before any bytes are streamed.
func (s *BackupService) OpenBulkDownloadEntry(entry BulkDownloadEntry) (*os.File, error) {
	if entry.SizeBytes < 0 {
		return nil, &BulkPathError{Path: entry.File.FilePath, Reason: "backup file size is invalid"}
	}
	backupRoot, err := validatedBackupRoot(s.backupDir)
	if err != nil {
		return nil, err
	}
	f, sizeBytes, err := openValidatedBulkDownloadFile(backupRoot, entry.File.FilePath)
	if err != nil {
		return nil, err
	}
	if sizeBytes != entry.SizeBytes {
		f.Close()
		return nil, &BulkPathError{Path: entry.File.FilePath, Reason: "backup file changed after validation"}
	}
	return f, nil
}

func openValidatedBulkDownloadFile(backupRoot, filePath string) (*os.File, int64, error) {
	if strings.TrimSpace(filePath) == "" {
		return nil, 0, &BulkPathError{Reason: "backup file path is empty"}
	}
	absPath, err := filepath.Abs(filePath)
	if err != nil {
		return nil, 0, fmt.Errorf("resolving backup file path: %w", err)
	}

	fd, err := unix.Open(absPath, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0)
	if err != nil {
		if errors.Is(err, unix.ELOOP) {
			return nil, 0, &BulkPathError{Path: filePath, Reason: "backup file path is a symlink"}
		}
		return nil, 0, fmt.Errorf("opening backup file: %w", err)
	}
	f := os.NewFile(uintptr(fd), absPath)
	if f == nil {
		unix.Close(fd)
		return nil, 0, fmt.Errorf("opening backup file: invalid file descriptor")
	}

	info, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, 0, fmt.Errorf("stat backup file: %w", err)
	}
	if !info.Mode().IsRegular() {
		f.Close()
		return nil, 0, &BulkPathError{Path: filePath, Reason: "backup file path is not a regular file"}
	}

	resolvedPath, err := resolveOpenedBackupPath(absPath, info)
	if err != nil {
		f.Close()
		return nil, 0, err
	}
	if !pathIsUnderDir(backupRoot, resolvedPath) {
		f.Close()
		return nil, 0, &BulkPathError{Path: filePath, Reason: "backup file path is outside backup directory"}
	}
	return f, info.Size(), nil
}

func resolveOpenedBackupPath(absPath string, openedInfo os.FileInfo) (string, error) {
	resolved, err := filepath.EvalSymlinks(absPath)
	if err != nil {
		return "", fmt.Errorf("resolving opened backup file path: %w", err)
	}
	currentInfo, err := os.Stat(resolved)
	if err != nil {
		return "", fmt.Errorf("stat opened backup file path: %w", err)
	}
	if !os.SameFile(openedInfo, currentInfo) {
		return "", &BulkPathError{Path: absPath, Reason: "backup file changed after open"}
	}
	return filepath.Clean(resolved), nil
}

func pathIsUnderDir(root, candidate string) bool {
	rel, err := filepath.Rel(root, candidate)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}

func safeBulkDownloadZipPath(deviceDir, fileName string) (string, error) {
	deviceDir = sanitizeHostname(deviceDir)
	if deviceDir == "" {
		deviceDir = "device"
	}
	name := strings.ReplaceAll(fileName, "\\", "/")
	if strings.TrimSpace(name) == "" {
		return "", &BulkPathError{Path: fileName, Reason: "backup file name is empty"}
	}
	if path.IsAbs(name) || hasWindowsDrivePrefix(name) {
		return "", &BulkPathError{Path: fileName, Reason: "backup file name is absolute"}
	}
	for _, segment := range strings.Split(name, "/") {
		if segment == ".." {
			return "", &BulkPathError{Path: fileName, Reason: "backup file name contains traversal"}
		}
	}
	cleanName := path.Clean(name)
	if cleanName == "." || cleanName == ".." || strings.HasPrefix(cleanName, "../") || path.IsAbs(cleanName) || hasWindowsDrivePrefix(cleanName) {
		return "", &BulkPathError{Path: fileName, Reason: "backup file name is unsafe"}
	}
	zipPath := path.Join(deviceDir, cleanName)
	if zipPath == "." || zipPath == ".." || strings.HasPrefix(zipPath, "../") || path.IsAbs(zipPath) {
		return "", &BulkPathError{Path: fileName, Reason: "zip entry path is unsafe"}
	}
	return zipPath, nil
}

func hasWindowsDrivePrefix(name string) bool {
	if len(name) < 2 || name[1] != ':' {
		return false
	}
	first := name[0]
	return (first >= 'a' && first <= 'z') || (first >= 'A' && first <= 'Z')
}

func dedupeUUIDs(ids []uuid.UUID) []uuid.UUID {
	if len(ids) <= 1 {
		return ids
	}
	seen := make(map[uuid.UUID]struct{}, len(ids))
	unique := make([]uuid.UUID, 0, len(ids))
	for _, id := range ids {
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		unique = append(unique, id)
	}
	return unique
}

func saturatedInt64Sum(a, b int64) int64 {
	if b > 0 && a > (1<<63-1)-b {
		return 1<<63 - 1
	}
	return a + b
}

func contextError(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		return nil
	}
}

// TriggerBackup creates a pending backup job and runs all backup types asynchronously.
func (s *BackupService) TriggerBackup(ctx context.Context, deviceID uuid.UUID) (*domain.BackupJob, error) {
	device, err := s.deviceRepo.GetByID(deviceID)
	if err != nil {
		return nil, fmt.Errorf("getting device: %w", err)
	}

	profile, err := s.credentialProfileRepo.GetBackupProfileForDevice(device.ID)
	if err != nil {
		return nil, fmt.Errorf("no credential profile assigned to device %s", deviceID)
	}

	backupCfg := s.vendorRegistry.ResolveBackupConfig(device.Vendor)
	if !backupCfg.Supported {
		return nil, fmt.Errorf("backup not supported for vendor %s", device.Vendor)
	}

	// Fast reachability check before creating the job
	if err := ssh.CheckReachable(device.IP, profile.Port, 5*time.Second); err != nil {
		return nil, fmt.Errorf("device unreachable: %w", err)
	}

	job := &domain.BackupJob{
		ID:       uuid.New(),
		DeviceID: deviceID,
		Status:   domain.BackupStatusPending,
	}
	if err := s.jobRepo.Create(job); err != nil {
		return nil, fmt.Errorf("creating backup job: %w", err)
	}

	go s.runFullBackup(device, profile, backupCfg, job.ID)

	return job, nil
}

func (s *BackupService) runFullBackup(device *domain.Device, profile *domain.CredentialProfile, backupCfg vendor.BackupConfig, jobID uuid.UUID) {
	lock := s.getDeviceLock(device.ID)
	lock.Lock()
	defer lock.Unlock()

	// Set job to running
	s.updateJobStatus(jobID, domain.BackupStatusRunning, "")

	secret, err := s.decryptSecret(profile.EncryptedSecret)
	if err != nil {
		s.failJob(jobID, fmt.Sprintf("decrypting credentials: %v", err))
		return
	}

	// Connect via SSH
	var client *ssh.Client
	timeout := 30 * time.Second

	if profile.AuthMethod == domain.SSHAuthPassword {
		client, err = ssh.NewClient(s.sshDialer, device.IP, profile.Port, profile.Username, secret, timeout, s.hostKeyCallback)
	} else {
		client, err = ssh.NewClientWithKey(s.sshDialer, device.IP, profile.Port, profile.Username, []byte(secret), timeout, s.hostKeyCallback)
	}
	if err != nil {
		s.failJob(jobID, fmt.Sprintf("SSH connection failed: %v", err))
		return
	}
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
	defer cancel()

	// Determine file prefix — try device fields first, then SSH identity
	hostname := sanitizeHostname(device.Tags["display_name"])
	if hostname == "" {
		hostname = sanitizeHostname(device.SysName)
	}
	if hostname == "" {
		// SysName empty (SNMP may have failed) — get identity via SSH
		if identity, identErr := client.RunCommand(ctx, "/system identity print"); identErr == nil {
			identity = strings.TrimSpace(identity)
			// MikroTik returns "name: <identity>", parse it
			if strings.HasPrefix(identity, "name:") {
				identity = strings.TrimSpace(strings.TrimPrefix(identity, "name:"))
			}
			hostname = sanitizeHostname(identity)
		}
	}
	if hostname == "" && device.Hostname != device.IP {
		hostname = sanitizeHostname(device.Hostname)
	}
	if hostname == "" {
		hostname = sanitizeHostname(device.IP)
	}
	log.Printf("Backup file prefix hostname: %q (device: %s)", hostname, device.IP)
	prefix := s.nowInConfiguredTZ().Format("20060102_150405") + "_" + hostname

	// Ensure device backup directory
	deviceDir := filepath.Join(s.backupDir, device.ID.String())
	if err := os.MkdirAll(deviceDir, 0700); err != nil {
		s.failJob(jobID, fmt.Sprintf("creating backup directory: %v", err))
		return
	}
	if err := os.Chmod(deviceDir, 0700); err != nil {
		s.failJob(jobID, fmt.Sprintf("restricting backup directory permissions: %v", err))
		return
	}

	var warnings []string

	// Step A: /export (running)
	if backupCfg.SSHCommands.ExportRunning != "" {
		if err := s.runTextExport(ctx, client, jobID, deviceDir, prefix+".rsc", "running", backupCfg.SSHCommands.ExportRunning); err != nil {
			warnings = append(warnings, fmt.Sprintf("running export: %v", err))
		}
	}

	// Step B: /export verbose
	if backupCfg.SSHCommands.ExportVerbose != "" {
		if err := s.runTextExport(ctx, client, jobID, deviceDir, prefix+"_verbose.rsc", "verbose", backupCfg.SSHCommands.ExportVerbose); err != nil {
			warnings = append(warnings, fmt.Sprintf("verbose export: %v", err))
		}
	}

	// Step C: /export compact
	if backupCfg.SSHCommands.ExportCompact != "" {
		if err := s.runTextExport(ctx, client, jobID, deviceDir, prefix+"_compact.rsc", "compact", backupCfg.SSHCommands.ExportCompact); err != nil {
			warnings = append(warnings, fmt.Sprintf("compact export: %v", err))
		}
	}

	// Step D: Binary backup
	if backupCfg.SSHCommands.BinaryBackup != nil {
		if err := s.runBinaryExport(ctx, client, jobID, deviceDir, prefix+".backup", backupCfg.SSHCommands.BinaryBackup); err != nil {
			warnings = append(warnings, fmt.Sprintf("binary backup: %v", err))
		}
	}

	// Check results
	files, _ := s.fileRepo.GetByJobID(jobID)
	if len(files) == 0 {
		s.failJob(jobID, "all backup types failed: "+strings.Join(warnings, "; "))
		return
	}

	errMsg := ""
	if len(warnings) > 0 {
		errMsg = "partial: " + strings.Join(warnings, "; ")
	}
	s.updateJobStatus(jobID, domain.BackupStatusSuccess, errMsg)
}

// waitForRemoteFile polls for a remote file's existence using SFTP Stat.
func (s *BackupService) waitForRemoteFile(sshClient *gossh.Client, remotePath string, timeout time.Duration) error {
	if sshClient == nil {
		return fmt.Errorf("creating SFTP client for stat: nil SSH client")
	}
	sftpClient, err := sftp.NewClient(sshClient)
	if err != nil {
		return fmt.Errorf("creating SFTP client for stat: %w", err)
	}
	defer sftpClient.Close()

	deadline := time.Now().Add(timeout)
	pollInterval := 500 * time.Millisecond

	for time.Now().Before(deadline) {
		_, err := sftpClient.Stat(remotePath)
		if err == nil {
			return nil // File exists
		}
		if !os.IsNotExist(err) {
			return fmt.Errorf("SFTP stat %q: %w", remotePath, err)
		}
		time.Sleep(pollInterval)
	}

	return fmt.Errorf("timed out waiting for remote file %q after %v", remotePath, timeout)
}

func downloadSFTPFileToDiskAndHash(ctx context.Context, sshClient *gossh.Client, remotePath, localPath string) (string, int, error) {
	if sshClient == nil {
		return "", 0, fmt.Errorf("creating SFTP client: nil SSH client")
	}
	sftpClient, err := sftp.NewClient(sshClient)
	if err != nil {
		return "", 0, fmt.Errorf("creating SFTP client: %w", err)
	}
	defer sftpClient.Close()

	type result struct {
		hash string
		size int
		err  error
	}

	done := make(chan result, 1)
	go func() {
		remoteFile, err := sftpClient.Open(remotePath)
		if err != nil {
			done <- result{err: fmt.Errorf("opening remote file %q: %w", remotePath, err)}
			return
		}
		defer remoteFile.Close()

		dir := filepath.Dir(localPath)
		tmpFile, err := os.CreateTemp(dir, ".theia-download-*")
		if err != nil {
			done <- result{err: fmt.Errorf("creating temp file: %w", err)}
			return
		}
		tmpPath := tmpFile.Name()

		hasher := sha256.New()
		written, err := io.Copy(io.MultiWriter(tmpFile, hasher), remoteFile)
		if err != nil {
			tmpFile.Close()
			os.Remove(tmpPath)
			done <- result{err: fmt.Errorf("downloading file: %w", err)}
			return
		}
		maxInt := int64(int(^uint(0) >> 1))
		if written > maxInt {
			tmpFile.Close()
			os.Remove(tmpPath)
			done <- result{err: fmt.Errorf("downloaded file too large: %d bytes", written)}
			return
		}
		if err := tmpFile.Close(); err != nil {
			os.Remove(tmpPath)
			done <- result{err: fmt.Errorf("closing temp file: %w", err)}
			return
		}

		if err := os.Rename(tmpPath, localPath); err != nil {
			os.Remove(tmpPath)
			done <- result{err: fmt.Errorf("renaming temp file: %w", err)}
			return
		}
		done <- result{
			hash: hex.EncodeToString(hasher.Sum(nil)),
			size: int(written),
		}
	}()

	select {
	case <-ctx.Done():
		return "", 0, ctx.Err()
	case r := <-done:
		return r.hash, r.size, r.err
	}
}

func (s *BackupService) runTextExport(ctx context.Context, client *ssh.Client, jobID uuid.UUID, dir, fileName, fileType, command string) error {
	filePath := filepath.Join(dir, fileName)
	tmpFile, err := os.CreateTemp(dir, ".theia-export-*")
	if err != nil {
		return fmt.Errorf("creating temp export file: %w", err)
	}
	tmpPath := tmpFile.Name()

	hasher := sha256.New()
	counter := &countingWriter{w: io.MultiWriter(tmpFile, hasher)}
	if err := client.RunCommandToWriter(ctx, command, counter); err != nil {
		tmpFile.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("command %q failed: %w", command, err)
	}
	maxInt := int64(int(^uint(0) >> 1))
	if counter.n > maxInt {
		tmpFile.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("export output too large: %d bytes", counter.n)
	}
	if err := tmpFile.Close(); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("closing temp export file: %w", err)
	}
	if err := os.Chmod(tmpPath, 0600); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("restricting temp file permissions: %w", err)
	}
	if err := os.Rename(tmpPath, filePath); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("renaming temp export file: %w", err)
	}
	if err := os.Chmod(filePath, 0600); err != nil {
		return fmt.Errorf("restricting file permissions: %w", err)
	}

	return s.fileRepo.Create(&domain.BackupFile{
		ID:        uuid.New(),
		JobID:     jobID,
		FileType:  fileType,
		FileName:  fileName,
		FilePath:  filePath,
		FileHash:  hex.EncodeToString(hasher.Sum(nil)),
		SizeBytes: int(counter.n),
	})
}

type countingWriter struct {
	w io.Writer
	n int64
}

func (w *countingWriter) Write(p []byte) (int, error) {
	n, err := w.w.Write(p)
	w.n += int64(n)
	return n, err
}

func (s *BackupService) runBinaryExport(ctx context.Context, client *ssh.Client, jobID uuid.UUID, dir, fileName string, bcfg *vendor.BinaryBackupCmd) error {
	// Step 1: Run save command
	log.Printf("Binary backup: running save command: %s", bcfg.SaveCommand)
	if _, err := client.RunCommand(ctx, bcfg.SaveCommand); err != nil {
		return fmt.Errorf("save command failed: %w", err)
	}

	// Step 2: Wait for file to appear on remote filesystem via SFTP stat polling
	if err := s.waitForRemoteFile(client.SSHClient(), bcfg.RemoteFilePath, 30*time.Second); err != nil {
		return fmt.Errorf("waiting for remote backup file: %w", err)
	}

	// Step 3: Download via SFTP to disk
	filePath := filepath.Join(dir, fileName)
	log.Printf("Binary backup: downloading file: %s -> %s", bcfg.RemoteFilePath, filePath)
	fileHash, sizeBytes, err := downloadSFTPFileToDiskAndHash(ctx, client.SSHClient(), bcfg.RemoteFilePath, filePath)
	if err != nil {
		return fmt.Errorf("SFTP download failed: %w", err)
	}
	if err := os.Chmod(filePath, 0600); err != nil {
		return fmt.Errorf("restricting downloaded file permissions: %w", err)
	}

	// Step 4: Cleanup remote file
	if bcfg.CleanupCommand != "" {
		log.Printf("Binary backup: cleaning up: %s", bcfg.CleanupCommand)
		if _, cleanErr := client.RunCommand(ctx, bcfg.CleanupCommand); cleanErr != nil {
			log.Printf("Warning: cleanup command failed: %v", cleanErr)
		}
	}

	return s.fileRepo.Create(&domain.BackupFile{
		ID:        uuid.New(),
		JobID:     jobID,
		FileType:  "binary",
		FileName:  fileName,
		FilePath:  filePath,
		FileHash:  fileHash,
		SizeBytes: sizeBytes,
	})
}

func (s *BackupService) updateJobStatus(jobID uuid.UUID, status domain.BackupStatus, errMsg string) {
	job, err := s.jobRepo.GetByID(jobID)
	if err != nil || job == nil {
		log.Printf("Failed to fetch job %s for update: %v", jobID, err)
		return
	}
	job.Status = status
	job.ErrorMessage = errMsg
	if err := s.jobRepo.Update(job); err != nil {
		log.Printf("Failed to update job %s: %v", jobID, err)
	}
}

func (s *BackupService) failJob(jobID uuid.UUID, errMsg string) {
	log.Printf("Backup job %s failed: %s", jobID, errMsg)
	s.updateJobStatus(jobID, domain.BackupStatusFailed, errMsg)
}

func sanitizeHostname(name string) string {
	s := sanitizeRe.ReplaceAllString(name, "_")
	if len(s) > 64 {
		s = s[:64]
	}
	return s
}

func (s *BackupService) decryptSecret(encrypted string) (string, error) {
	if encrypted == "" {
		return "", nil
	}

	var base64DecryptErr error
	if ciphertext, err := base64.StdEncoding.DecodeString(encrypted); err == nil {
		decrypted, err := crypto.Decrypt(ciphertext, s.encryptionKey)
		if err == nil {
			return string(decrypted), nil
		}
		base64DecryptErr = err
	}

	decrypted, err := crypto.Decrypt([]byte(encrypted), s.encryptionKey)
	if err == nil {
		return string(decrypted), nil
	}
	if base64DecryptErr != nil {
		return "", base64DecryptErr
	}
	return "", err
}

// EncryptSecret encrypts a plaintext secret for storage.
func (s *BackupService) EncryptSecret(plaintext string) (string, error) {
	if plaintext == "" {
		return "", nil
	}
	encrypted, err := crypto.Encrypt([]byte(plaintext), s.encryptionKey)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(encrypted), nil
}

// GetWinboxCredentials retrieves the decrypted WinBox password for a device.
// It fetches the device IP from the device repository and decrypts the
// credential profile's secret in the service layer (T-24-05 mitigation).
// Returns ip, decryptedPassword, and an error. username is returned separately.
func (s *BackupService) GetWinboxCredentials(deviceID uuid.UUID, encryptedSecret, username string) (ip, password string, err error) {
	device, err := s.deviceRepo.GetByID(deviceID)
	if err != nil {
		return "", "", fmt.Errorf("device not found: %w", err)
	}
	pwd, err := s.decryptSecret(encryptedSecret)
	if err != nil {
		return "", "", fmt.Errorf("decrypting credentials: %w", err)
	}
	if pwd == "" {
		return "", "", fmt.Errorf("WinBox profile has no password configured")
	}
	return device.IP, pwd, nil
}

// GetBackupJobs returns all backup jobs for a device.
func (s *BackupService) GetBackupJobs(ctx context.Context, deviceID uuid.UUID) ([]domain.BackupJob, error) {
	jobs, err := s.jobRepo.GetByDeviceID(deviceID)
	if err != nil {
		return nil, err
	}
	// Attach file counts
	for i := range jobs {
		files, _ := s.fileRepo.GetByJobID(jobs[i].ID)
		jobs[i].Files = files
	}
	return jobs, nil
}

// GetBackupJob returns a single backup job with its files.
func (s *BackupService) GetBackupJob(ctx context.Context, id uuid.UUID) (*domain.BackupJob, error) {
	job, err := s.jobRepo.GetByID(id)
	if err != nil {
		return nil, err
	}
	if job == nil {
		return nil, nil
	}
	files, _ := s.fileRepo.GetByJobID(job.ID)
	job.Files = files
	return job, nil
}

// GetLatestBackupJob returns the latest successful backup job with files.
func (s *BackupService) GetLatestBackupJob(ctx context.Context, deviceID uuid.UUID) (*domain.BackupJob, error) {
	job, err := s.jobRepo.GetLatestByDeviceID(deviceID)
	if err != nil {
		return nil, err
	}
	if job == nil {
		return nil, nil
	}
	files, _ := s.fileRepo.GetByJobID(job.ID)
	job.Files = files
	return job, nil
}

// DeleteBackupJob removes a backup job, its files from disk and DB.
func (s *BackupService) DeleteBackupJob(ctx context.Context, id uuid.UUID) error {
	// Get files to delete from disk
	files, _ := s.fileRepo.GetByJobID(id)
	var fileWarnings []string
	for _, f := range files {
		if f.FilePath != "" {
			if err := os.Remove(f.FilePath); err != nil && !os.IsNotExist(err) {
				fileWarnings = append(fileWarnings, fmt.Sprintf("removing %s: %v", f.FilePath, err))
			}
		}
	}
	// Delete file records
	if err := s.fileRepo.DeleteByJobID(id); err != nil {
		return fmt.Errorf("deleting file records: %w", err)
	}
	// Delete job
	if err := s.jobRepo.Delete(id); err != nil {
		return fmt.Errorf("deleting job: %w", err)
	}
	if len(fileWarnings) > 0 {
		log.Printf("Warning: some backup files could not be removed for job %s: %s", id, strings.Join(fileWarnings, "; "))
	}
	return nil
}

// GetBackupFile returns a single backup file by ID.
func (s *BackupService) GetBackupFile(ctx context.Context, id uuid.UUID) (*domain.BackupFile, error) {
	return s.fileRepo.GetByID(id)
}

// GetBackupFileContent opens the backup file for streaming.
// The caller MUST close the returned io.ReadCloser when done.
func (s *BackupService) GetBackupFileContent(ctx context.Context, id uuid.UUID) (io.ReadCloser, string, error) {
	file, err := s.fileRepo.GetByID(id)
	if err != nil {
		return nil, "", err
	}
	if file == nil {
		return nil, "", fmt.Errorf("backup file not found")
	}
	f, err := os.Open(file.FilePath)
	if err != nil {
		return nil, "", fmt.Errorf("opening backup file: %w", err)
	}
	return f, file.FileName, nil
}

// TestSSHConnection tests SSH connectivity to a device using its assigned SSH profile.
func (s *BackupService) TestSSHConnection(ctx context.Context, deviceID uuid.UUID) error {
	device, err := s.deviceRepo.GetByID(deviceID)
	if err != nil {
		return fmt.Errorf("getting device: %w", err)
	}

	profile, err := s.credentialProfileRepo.GetBackupProfileForDevice(device.ID)
	if err != nil {
		return fmt.Errorf("no credential profile assigned to device %s", deviceID)
	}

	secret, err := s.decryptSecret(profile.EncryptedSecret)
	if err != nil {
		return fmt.Errorf("decrypting credentials: %w", err)
	}

	timeout := 10 * time.Second
	var client *ssh.Client

	if profile.AuthMethod == domain.SSHAuthPassword {
		client, err = ssh.NewClient(s.sshDialer, device.IP, profile.Port, profile.Username, secret, timeout, s.hostKeyCallback)
	} else {
		client, err = ssh.NewClientWithKey(s.sshDialer, device.IP, profile.Port, profile.Username, []byte(secret), timeout, s.hostKeyCallback)
	}
	if err != nil {
		return fmt.Errorf("SSH connection failed: %w", err)
	}
	defer client.Close()

	return nil
}

// TestCredentialProfile tests SSH connectivity using a credential profile against a target IP.
func (s *BackupService) TestCredentialProfile(ctx context.Context, profileID uuid.UUID, targetIP string) error {
	profile, err := s.credentialProfileRepo.GetByID(profileID)
	if err != nil {
		return fmt.Errorf("getting credential profile: %w", err)
	}

	secret, err := s.decryptSecret(profile.EncryptedSecret)
	if err != nil {
		return fmt.Errorf("decrypting credentials: %w", err)
	}

	timeout := 10 * time.Second
	var client *ssh.Client

	if profile.AuthMethod == domain.SSHAuthPassword {
		client, err = ssh.NewClient(s.sshDialer, targetIP, profile.Port, profile.Username, secret, timeout, s.hostKeyCallback)
	} else {
		client, err = ssh.NewClientWithKey(s.sshDialer, targetIP, profile.Port, profile.Username, []byte(secret), timeout, s.hostKeyCallback)
	}
	if err != nil {
		return fmt.Errorf("SSH connection failed: %w", err)
	}
	defer client.Close()

	return nil
}

// CreateCredentialProfile creates a new credential profile.
// If role is empty, it defaults to "Admin".
func (s *BackupService) CreateCredentialProfile(ctx context.Context, name, description, username string, port int, authMethod domain.SSHAuthMethod, secret string, role string) (*domain.CredentialProfile, error) {
	encryptedSecret, err := s.EncryptSecret(secret)
	if err != nil {
		return nil, fmt.Errorf("encrypting secret: %w", err)
	}

	if role == "" {
		role = "Admin"
	}

	profile := &domain.CredentialProfile{
		Name:            name,
		Description:     description,
		Username:        username,
		Port:            port,
		AuthMethod:      authMethod,
		EncryptedSecret: encryptedSecret,
		Role:            role,
	}
	if err := s.credentialProfileRepo.Create(profile); err != nil {
		return nil, fmt.Errorf("creating credential profile: %w", err)
	}
	return profile, nil
}

// GetCredentialProfile returns a credential profile by ID.
func (s *BackupService) GetCredentialProfile(ctx context.Context, id uuid.UUID) (*domain.CredentialProfile, error) {
	return s.credentialProfileRepo.GetByID(id)
}

// GetAllCredentialProfiles returns all credential profiles.
func (s *BackupService) GetAllCredentialProfiles(ctx context.Context) ([]domain.CredentialProfile, error) {
	return s.credentialProfileRepo.GetAll()
}

// UpdateCredentialProfile updates an existing credential profile. If secret is empty, the existing secret is kept.
// If role is empty, it defaults to "Admin".
func (s *BackupService) UpdateCredentialProfile(ctx context.Context, id uuid.UUID, name, description, username string, port int, authMethod domain.SSHAuthMethod, secret string, role string) (*domain.CredentialProfile, error) {
	profile, err := s.credentialProfileRepo.GetByID(id)
	if err != nil {
		return nil, fmt.Errorf("getting credential profile: %w", err)
	}

	profile.Name = name
	profile.Description = description
	profile.Username = username
	profile.Port = port
	profile.AuthMethod = authMethod

	if role == "" {
		role = "Admin"
	}
	profile.Role = role

	if secret != "" {
		encryptedSecret, err := s.EncryptSecret(secret)
		if err != nil {
			return nil, fmt.Errorf("encrypting secret: %w", err)
		}
		profile.EncryptedSecret = encryptedSecret
	}

	if err := s.credentialProfileRepo.Update(profile); err != nil {
		return nil, fmt.Errorf("updating credential profile: %w", err)
	}
	return profile, nil
}

// DeleteCredentialProfile removes a credential profile. Returns an error if any device references it.
func (s *BackupService) DeleteCredentialProfile(ctx context.Context, id uuid.UUID) error {
	return s.credentialProfileRepo.Delete(id)
}
