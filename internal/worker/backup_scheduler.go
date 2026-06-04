package worker

import (
	"context"
	"log"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/lollinoo/theia/internal/domain"
	"github.com/lollinoo/theia/internal/service"
)

// BackupScheduler runs scheduled instance backups and retention cleanup.
// It follows the same Start/Stop lifecycle pattern as Poller and MetricsCollector.
type BackupScheduler struct {
	backupService *service.InstanceBackupService
	backupRepo    domain.InstanceBackupRepository
	settingsRepo  domain.SettingsRepository

	running atomic.Bool
	cancel  context.CancelFunc
	done    chan struct{}
}

// NewBackupScheduler creates a new BackupScheduler.
func NewBackupScheduler(
	backupService *service.InstanceBackupService,
	backupRepo domain.InstanceBackupRepository,
	settingsRepo domain.SettingsRepository,
) *BackupScheduler {
	return &BackupScheduler{
		backupService: backupService,
		backupRepo:    backupRepo,
		settingsRepo:  settingsRepo,
		done:          make(chan struct{}),
	}
}

// Start begins the background scheduler loop. It reads the backup interval
// from settings each cycle, so changes take effect without restart.
func (s *BackupScheduler) Start(ctx context.Context) {
	schedCtx, cancel := context.WithCancel(ctx)
	s.cancel = cancel
	s.running.Store(true)

	go func() {
		defer close(s.done)
		defer s.running.Store(false)

		for {
			select {
			case <-schedCtx.Done():
				log.Println("BackupScheduler shutting down")
				return
			case <-time.After(1 * time.Hour):
				s.tick(schedCtx)
			}
		}
	}()

	log.Println("BackupScheduler started")
}

// Stop gracefully stops the scheduler and waits for it to finish.
func (s *BackupScheduler) Stop() {
	if s.cancel != nil {
		s.cancel()
		<-s.done
	}
	log.Println("BackupScheduler stopped")
}

// Status returns "running" or "stopped".
func (s *BackupScheduler) Status() string {
	if s.running.Load() {
		return "running"
	}
	return "stopped"
}

// tick is called each cycle to check if a backup is due and run retention.
func (s *BackupScheduler) tick(ctx context.Context) {
	interval := GetBackupInterval(s.settingsRepo)

	if interval > 0 {
		s.checkAndCreateBackup(ctx, interval)
	}

	// Always run retention sweep regardless of interval setting
	s.runRetention(ctx)
}

// checkAndCreateBackup creates a scheduled backup if enough time has elapsed
// since the last successful backup.
func (s *BackupScheduler) checkAndCreateBackup(ctx context.Context, interval time.Duration) {
	backups, err := s.backupRepo.ListSuccessfulOldest()
	if err != nil {
		log.Printf("BackupScheduler: failed to list backups: %v", err)
		return
	}

	if len(backups) == 0 {
		// No successful backups exist — create one immediately
		s.createScheduledBackup(ctx)
		return
	}

	// Check if enough time has elapsed since the most recent successful backup.
	// ListSuccessfulOldest returns oldest first, so the last element is the newest.
	newest := backups[len(backups)-1]
	if time.Since(newest.CreatedAt) >= interval {
		s.createScheduledBackup(ctx)
	}
}

// createScheduledBackup creates a backup with trigger set to "scheduled".
func (s *BackupScheduler) createScheduledBackup(ctx context.Context) {
	backup, err := s.backupService.CreateWithTrigger(ctx, domain.InstanceBackupTriggerScheduled)
	if err != nil {
		log.Printf("BackupScheduler: failed to create scheduled backup: %v", err)
		return
	}
	log.Printf("Scheduled backup created: %s", backup.FileName)
}

// runRetention deletes old successful backups beyond the retention count
// and cleans up failed backup records older than 7 days.
// Uses a 60s context timeout for consistency with DeviceBackupScheduler (T-19-02 pattern).
func (s *BackupScheduler) runRetention(ctx context.Context) {
	retCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	retentionCount := GetRetentionCount(s.settingsRepo)

	cutoff := time.Now().Add(-7 * 24 * time.Hour)
	result, err := s.backupService.CleanupRetention(retCtx, retentionCount, cutoff)
	if err != nil {
		log.Printf("BackupScheduler: retention: failed to list successful backups: %v", err)
		return
	}
	if result.TimedOut {
		log.Printf("BackupScheduler: retention sweep timed out after deleting %d/%d backups, will resume next cycle", result.SuccessfulDeleted, result.SuccessfulDeleteCandidates)
	}
	for _, failure := range result.DeleteFailures {
		log.Printf("BackupScheduler: retention: failed to delete backup %s: %v", failure.ID, failure.Err)
	}
	if result.FailedCleanupError != nil {
		log.Printf("BackupScheduler: retention: failed to clean failed records: %v", result.FailedCleanupError)
	}
	if result.SuccessfulDeleted > 0 || result.FailedDeleted > 0 {
		log.Printf("Retention: deleted %d old backups, cleaned %d failed records", result.SuccessfulDeleted, result.FailedDeleted)
	}
}

// GetBackupInterval reads the backup interval from the settings repository.
// Returns 0 if the setting is "0", missing, or invalid (0 means disabled).
// Unlike GetPollingInterval, 0 is a valid value meaning backups are disabled.
func GetBackupInterval(settingsRepo domain.SettingsRepository) time.Duration {
	val, err := settingsRepo.Get(domain.SettingInstanceBackupIntervalHours)
	if err != nil {
		return 0
	}
	hours, err := strconv.Atoi(val)
	if err != nil || hours < 0 {
		return 0
	}
	return time.Duration(hours) * time.Hour
}

// GetRetentionCount reads the retention count from the settings repository.
// Returns the configured count with a minimum of 1. Defaults to 5 if missing or invalid.
func GetRetentionCount(settingsRepo domain.SettingsRepository) int {
	val, err := settingsRepo.Get(domain.SettingInstanceBackupRetentionCount)
	if err != nil {
		return 5
	}
	count, err := strconv.Atoi(val)
	if err != nil || count < 0 {
		return 5
	}
	if count < 1 {
		return 1
	}
	return count
}
