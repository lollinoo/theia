package worker

// This file defines device backup scheduler worker behavior, background lifecycle, and runtime state updates.

import (
	"bytes"
	"context"
	"errors"
	"log"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/domain"
	"github.com/lollinoo/theia/internal/service"
)

type deviceBackupService interface {
	DeleteBackupJob(ctx context.Context, id uuid.UUID) error
	GetLatestBulkBackupRun(ctx context.Context) (*domain.BulkBackupRun, error)
	StartBulkBackupRun(ctx context.Context, requestedDeviceIDs []uuid.UUID, createdBy string) (*domain.BulkBackupRun, error)
}

// DeviceBackupScheduler runs scheduled device config backups and per-device retention cleanup.
// It follows the same Start/Stop lifecycle pattern as BackupScheduler and PipelineOrchestrator.
type DeviceBackupScheduler struct {
	backupService deviceBackupService
	jobRepo       domain.BackupJobRepository
	settingsRepo  domain.SettingsRepository

	running            atomic.Bool
	cancel             context.CancelFunc
	done               chan struct{}
	retentionCursor    uuid.UUID
	retentionJobCursor uuid.UUID
}

// NewDeviceBackupScheduler creates a new DeviceBackupScheduler.
func NewDeviceBackupScheduler(
	backupService *service.BackupService,
	jobRepo domain.BackupJobRepository,
	settingsRepo domain.SettingsRepository,
) *DeviceBackupScheduler {
	return &DeviceBackupScheduler{
		backupService: backupService,
		jobRepo:       jobRepo,
		settingsRepo:  settingsRepo,
		done:          make(chan struct{}),
	}
}

// Start begins the background scheduler loop. It reads the backup interval
// from settings each cycle, so changes take effect without restart.
func (s *DeviceBackupScheduler) Start(ctx context.Context) {
	schedCtx, cancel := context.WithCancel(ctx)
	s.cancel = cancel
	s.running.Store(true)

	go func() {
		defer close(s.done)
		defer s.running.Store(false)
		backupTicker := time.NewTicker(time.Hour)
		cleanupTicker := time.NewTicker(time.Minute)
		defer backupTicker.Stop()
		defer cleanupTicker.Stop()
		s.cleanupDeletedFiles(schedCtx)

		for {
			select {
			case <-schedCtx.Done():
				log.Println("DeviceBackupScheduler shutting down")
				return
			case <-backupTicker.C:
				s.tick(schedCtx)
			case <-cleanupTicker.C:
				s.cleanupDeletedFiles(schedCtx)
			}
		}
	}()

	log.Println("DeviceBackupScheduler started")
}

// Stop gracefully stops the scheduler and waits for it to finish.
func (s *DeviceBackupScheduler) Stop() {
	if s.cancel != nil {
		s.cancel()
		<-s.done
	}
	log.Println("DeviceBackupScheduler stopped")
}

// Status returns "running" or "stopped".
func (s *DeviceBackupScheduler) Status() string {
	if s.running.Load() {
		return "running"
	}
	return "stopped"
}

// tick is called each cycle to check if a device backup is due and run retention.
func (s *DeviceBackupScheduler) tick(ctx context.Context) {
	interval := GetDeviceBackupInterval(s.settingsRepo)

	if interval > 0 {
		s.checkAndRunBulkBackup(ctx, interval)
	}

	// Always run per-device retention sweep regardless of interval setting
	s.runRetention(ctx)
}

// checkAndRunBulkBackup triggers a bulk backup if enough time has elapsed
// since the last bulk backup run. Uses the most recent backup job
// created_at across ALL devices as the schedule reference.
func (s *DeviceBackupScheduler) checkAndRunBulkBackup(ctx context.Context, interval time.Duration) {
	if s.checkAndRunBulkBackupFromLatestRun(ctx, interval) {
		return
	}

	// Find the most recent backup job globally to determine last bulk run time.
	deviceIDs, err := s.jobRepo.ListAllDeviceIDs()
	if err != nil {
		log.Printf("DeviceBackupScheduler: failed to list device IDs: %v", err)
		return
	}

	if len(deviceIDs) == 0 {
		// No backup jobs exist at all -- trigger first scheduled backup immediately
		s.runScheduledBulkBackup(ctx)
		return
	}

	// Find the newest successful job across all devices
	var newest *domain.BackupJob
	for _, did := range deviceIDs {
		job, err := s.jobRepo.GetLatestByDeviceID(did)
		if err != nil || job == nil {
			continue
		}
		if newest == nil || job.CreatedAt.After(newest.CreatedAt) {
			newest = job
		}
	}

	if newest == nil {
		// No successful jobs -- run immediately
		s.runScheduledBulkBackup(ctx)
		return
	}

	if time.Since(newest.CreatedAt) >= interval {
		s.runScheduledBulkBackup(ctx)
	}
}

func (s *DeviceBackupScheduler) checkAndRunBulkBackupFromLatestRun(ctx context.Context, interval time.Duration) bool {
	run, err := s.backupService.GetLatestBulkBackupRun(ctx)
	if err != nil {
		log.Printf("DeviceBackupScheduler: failed to get latest bulk backup run: %v", err)
		return false
	}
	if run == nil {
		return false
	}
	if run.Status == domain.BulkBackupRunStatusRunning ||
		run.Status == domain.BulkBackupRunStatusPausing ||
		run.Status == domain.BulkBackupRunStatusPaused ||
		run.Status == domain.BulkBackupRunStatusCancelling {
		return true
	}

	reference := run.CreatedAt
	if run.CompletedAt != nil {
		reference = *run.CompletedAt
	}
	if time.Since(reference) >= interval {
		s.runScheduledBulkBackup(ctx)
	}
	return true
}

// runScheduledBulkBackup starts a persistent bulk backup run on the backup service.
//
// Authorization model (T-19-03): This function runs as an internal scheduler goroutine
// with no external trigger surface. StartBulkBackupRun enforces per-device authorization
// implicitly: each device must have an SSH profile assigned, the vendor must support
// backup commands, and the device must be SSH-reachable. There is no user-facing
// privilege escalation risk since the scheduler operates on the same device set that
// an authenticated user could trigger manually via POST /api/v1/backups/bulk-runs.
func (s *DeviceBackupScheduler) runScheduledBulkBackup(ctx context.Context) {
	run, err := s.backupService.StartBulkBackupRun(ctx, nil, "scheduler")
	if err != nil {
		if errors.Is(err, service.ErrBulkBackupRunAlreadyActive) {
			if run != nil {
				log.Printf("DeviceBackupScheduler: bulk backup run already active: %s", run.ID)
			} else {
				log.Printf("DeviceBackupScheduler: bulk backup run already active")
			}
			return
		}
		log.Printf("DeviceBackupScheduler: failed to start persistent bulk backup run: %v", err)
		return
	}

	if run == nil {
		log.Printf("DeviceBackupScheduler: persistent bulk backup run did not return a run")
		return
	}
	log.Printf("Scheduled device backup run started: %s (%d devices)", run.ID, run.TotalCount)
}

// runRetention bounds database retention and disk cleanup to one 60s budget.
// The cursor survives sweeps in this scheduler instance and advances even when
// a device fails, so one slow device cannot starve all later devices.
func (s *DeviceBackupScheduler) runRetention(ctx context.Context) {
	retCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	defer s.cleanupDeletedFiles(retCtx)
	s.runRetentionSweep(retCtx)
}

func (s *DeviceBackupScheduler) runRetentionSweep(ctx context.Context) {
	if ctx.Err() != nil {
		return
	}
	retentionCount := getDeviceBackupRetentionCountContext(ctx, s.settingsRepo)
	if ctx.Err() != nil {
		return
	}

	// Attempt failed-record cleanup before the device sweep so it is not
	// indefinitely deferred when successful-job retention consumes the budget.
	deleteFailed := s.jobRepo.DeleteFailedOlderThan
	if repo, ok := s.jobRepo.(interface {
		DeleteFailedOlderThanContext(context.Context, time.Time) (int, error)
	}); ok {
		deleteFailed = func(cutoff time.Time) (int, error) { return repo.DeleteFailedOlderThanContext(ctx, cutoff) }
	}
	failedCount, err := deleteFailed(time.Now().Add(-7 * 24 * time.Hour))
	if err != nil {
		log.Printf("DeviceBackupScheduler: retention: failed to clean failed records: %v", err)
	}
	if ctx.Err() != nil {
		return
	}

	if repo, ok := s.jobRepo.(backupRetentionBatchRepository); ok {
		s.runRetentionBatches(ctx, repo, retentionCount)
		return
	}

	listDevices := s.jobRepo.ListAllDeviceIDs
	if repo, ok := s.jobRepo.(interface {
		ListAllDeviceIDsContext(context.Context) ([]uuid.UUID, error)
	}); ok {
		listDevices = func() ([]uuid.UUID, error) { return repo.ListAllDeviceIDsContext(ctx) }
	}
	deviceIDs, err := listDevices()
	if err != nil {
		log.Printf("DeviceBackupScheduler: retention: failed to list device IDs: %v", err)
		return
	}
	if ctx.Err() != nil {
		return
	}
	sort.Slice(deviceIDs, func(i, j int) bool { return bytes.Compare(deviceIDs[i][:], deviceIDs[j][:]) < 0 })
	start := sort.Search(len(deviceIDs), func(i int) bool { return bytes.Compare(deviceIDs[i][:], s.retentionCursor[:]) > 0 })

	listJobs := s.jobRepo.ListSuccessfulByDeviceOldest
	if repo, ok := s.jobRepo.(interface {
		ListSuccessfulByDeviceOldestContext(context.Context, uuid.UUID) ([]domain.BackupJob, error)
	}); ok {
		listJobs = func(id uuid.UUID) ([]domain.BackupJob, error) {
			return repo.ListSuccessfulByDeviceOldestContext(ctx, id)
		}
	}
	totalDeleted := 0
	defer func() {
		if totalDeleted > 0 || failedCount > 0 {
			log.Printf("Device backup retention: deleted %d old jobs, cleaned %d failed records", totalDeleted, failedCount)
		}
	}()
	for i := 0; i < len(deviceIDs); i++ {
		if ctx.Err() != nil {
			return
		}
		did := deviceIDs[(start+i)%len(deviceIDs)]
		s.retentionCursor = did
		successful, err := listJobs(did)
		if err != nil {
			log.Printf("DeviceBackupScheduler: retention: failed to list jobs for device %s: %v", did, err)
			continue
		}
		if len(successful) <= retentionCount {
			continue
		}
		for _, job := range successful[:len(successful)-retentionCount] {
			if ctx.Err() != nil {
				return
			}
			if err := s.backupService.DeleteBackupJob(ctx, job.ID); err != nil {
				log.Printf("DeviceBackupScheduler: retention: failed to delete job %s: %v", job.ID, err)
				continue
			}
			totalDeleted++
		}
	}
	if ctx.Err() == nil {
		s.retentionCursor = uuid.Nil
	}
}

func (s *DeviceBackupScheduler) cleanupDeletedFiles(ctx context.Context) {
	cleaner, ok := s.backupService.(interface {
		CleanupDeletedBackupFiles(context.Context) (int, error)
	})
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	count, err := cleaner.CleanupDeletedBackupFiles(ctx)
	if err != nil {
		log.Printf("DeviceBackupScheduler: backup file cleanup: %v", err)
	}
	if count > 0 {
		log.Printf("DeviceBackupScheduler: cleaned %d deleted backup files", count)
	}
}

// GetDeviceBackupInterval reads the device backup interval from settings.
// Returns 0 if the setting is "0", missing, or invalid (0 means disabled).
func GetDeviceBackupInterval(settingsRepo domain.SettingsRepository) time.Duration {
	val, err := settingsRepo.Get(domain.SettingDeviceBackupIntervalHours)
	if err != nil {
		return 0
	}
	hours, err := strconv.Atoi(val)
	if err != nil || hours < 0 {
		return 0
	}
	return time.Duration(hours) * time.Hour
}

// GetDeviceBackupRetentionCount reads the device backup retention count from settings.
// Returns the configured count with a minimum of 1. Defaults to 5 if missing or invalid.
func GetDeviceBackupRetentionCount(settingsRepo domain.SettingsRepository) int {
	return getDeviceBackupRetentionCountContext(context.Background(), settingsRepo)
}

func getDeviceBackupRetentionCountContext(ctx context.Context, settingsRepo domain.SettingsRepository) int {
	get := settingsRepo.Get
	if repo, ok := settingsRepo.(interface {
		GetContext(context.Context, string) (string, error)
	}); ok {
		get = func(key string) (string, error) { return repo.GetContext(ctx, key) }
	}
	val, err := get(domain.SettingDeviceBackupRetentionCount)
	if err != nil {
		return 5
	}
	count, err := strconv.Atoi(strings.TrimSpace(val))
	if err != nil || count < 0 {
		return 5
	}
	return domain.CoerceConstrainedInt(domain.SettingDeviceBackupRetentionCount, val, 5)
}
