package worker

import (
	"context"
	"errors"
	"log"
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
// It follows the same Start/Stop lifecycle pattern as BackupScheduler and Poller.
type DeviceBackupScheduler struct {
	backupService deviceBackupService
	jobRepo       domain.BackupJobRepository
	settingsRepo  domain.SettingsRepository

	running atomic.Bool
	cancel  context.CancelFunc
	done    chan struct{}
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

		for {
			select {
			case <-schedCtx.Done():
				log.Println("DeviceBackupScheduler shutting down")
				return
			case <-time.After(1 * time.Hour):
				s.tick(schedCtx)
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

// runRetention performs per-device retention (delete oldest successful beyond count)
// and cleans up failed records older than 7 days.
// Uses a 60s context timeout and processes devices in batches of 100 to bound
// resource consumption even at scale (T-19-02 remediation).
func (s *DeviceBackupScheduler) runRetention(ctx context.Context) {
	retCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	retentionCount := GetDeviceBackupRetentionCount(s.settingsRepo)

	deviceIDs, err := s.jobRepo.ListAllDeviceIDs()
	if err != nil {
		log.Printf("DeviceBackupScheduler: retention: failed to list device IDs: %v", err)
		return
	}

	const batchSize = 100
	totalDeleted := 0
	timedOut := false

	for i := 0; i < len(deviceIDs); i += batchSize {
		select {
		case <-retCtx.Done():
			log.Printf("DeviceBackupScheduler: retention sweep timed out after processing %d/%d devices, will resume next cycle", i, len(deviceIDs))
			timedOut = true
		default:
		}
		if timedOut {
			break
		}

		end := i + batchSize
		if end > len(deviceIDs) {
			end = len(deviceIDs)
		}

		for _, did := range deviceIDs[i:end] {
			successful, err := s.jobRepo.ListSuccessfulByDeviceOldest(did)
			if err != nil {
				log.Printf("DeviceBackupScheduler: retention: failed to list jobs for device %s: %v", did, err)
				continue
			}

			if len(successful) > retentionCount {
				toDelete := successful[:len(successful)-retentionCount]
				for _, job := range toDelete {
					if err := s.backupService.DeleteBackupJob(ctx, job.ID); err != nil {
						log.Printf("DeviceBackupScheduler: retention: failed to delete job %s: %v", job.ID, err)
						continue
					}
					totalDeleted++
				}
			}
		}
	}

	// Always clean up failed backup records regardless of timeout
	cutoff := time.Now().Add(-7 * 24 * time.Hour)
	failedCount, err := s.jobRepo.DeleteFailedOlderThan(cutoff)
	if err != nil {
		log.Printf("DeviceBackupScheduler: retention: failed to clean failed records: %v", err)
	}

	if totalDeleted > 0 || failedCount > 0 {
		log.Printf("Device backup retention: deleted %d old jobs, cleaned %d failed records", totalDeleted, failedCount)
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
	val, err := settingsRepo.Get(domain.SettingDeviceBackupRetentionCount)
	if err != nil {
		return 5
	}
	count, err := strconv.Atoi(strings.TrimSpace(val))
	if err != nil || count < 0 {
		return 5
	}
	return domain.CoerceConstrainedInt(domain.SettingDeviceBackupRetentionCount, val, 5)
}
