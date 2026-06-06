package service

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/domain"
	"github.com/lollinoo/theia/internal/observability"
	"github.com/lollinoo/theia/internal/ssh"
)

func (s *BackupService) StartBulkBackupRun(ctx context.Context, requestedDeviceIDs []uuid.UUID, createdBy string) (*domain.BulkBackupRun, error) {
	if s.bulkRunRepo == nil {
		return nil, errors.New("bulk backup run repository is not configured")
	}
	if active, err := s.bulkRunRepo.GetActiveRun(); err != nil {
		return nil, fmt.Errorf("checking active bulk backup run: %w", err)
	} else if active != nil {
		hydrated, hydrateErr := s.hydrateBulkBackupRunFileTotals(active)
		if hydrateErr != nil {
			return nil, fmt.Errorf("loading active bulk backup run totals: %w", hydrateErr)
		}
		return hydrated, ErrBulkBackupRunAlreadyActive
	}
	devices, err := s.bulkBackupRunDevices(ctx, requestedDeviceIDs)
	if err != nil {
		return nil, err
	}
	limits := s.BulkOperationLimits()
	if len(devices) > limits.BulkBackupMaxQueuedJobs {
		return nil, &BulkLimitError{
			Operation: "bulk backup run",
			Limit:     "queued jobs",
			Max:       int64(limits.BulkBackupMaxQueuedJobs),
			Actual:    int64(len(devices)),
		}
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
	return s.getBulkBackupRunWithFileTotals(run.ID)
}

func (s *BackupService) GetBulkBackupRun(ctx context.Context, id uuid.UUID) (*domain.BulkBackupRun, error) {
	if err := contextError(ctx); err != nil {
		return nil, err
	}
	if s.bulkRunRepo == nil {
		return nil, errors.New("bulk backup run repository is not configured")
	}
	return s.getBulkBackupRunWithFileTotals(id)
}

func (s *BackupService) GetLatestBulkBackupRun(ctx context.Context) (*domain.BulkBackupRun, error) {
	if err := contextError(ctx); err != nil {
		return nil, err
	}
	if s.bulkRunRepo == nil {
		return nil, errors.New("bulk backup run repository is not configured")
	}
	run, err := s.bulkRunRepo.GetLatestRun()
	if err != nil {
		return nil, err
	}
	return s.hydrateBulkBackupRunFileTotals(run)
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
	return s.getBulkBackupRunWithFileTotals(id)
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
	return s.getBulkBackupRunWithFileTotals(id)
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
	return s.getBulkBackupRunWithFileTotals(id)
}

func (s *BackupService) getBulkBackupRunWithFileTotals(id uuid.UUID) (*domain.BulkBackupRun, error) {
	run, err := s.bulkRunRepo.GetRun(id)
	if err != nil {
		return nil, err
	}
	return s.hydrateBulkBackupRunFileTotals(run)
}

func (s *BackupService) hydrateBulkBackupRunFileTotals(run *domain.BulkBackupRun) (*domain.BulkBackupRun, error) {
	if run == nil || s.fileRepo == nil {
		return run, nil
	}

	run.FileCount = 0
	run.ByteCount = 0
	for index := range run.Items {
		item := &run.Items[index]
		item.FileCount = 0
		item.ByteCount = 0
		if item.BackupJobID == nil {
			continue
		}
		files, err := s.fileRepo.GetByJobID(*item.BackupJobID)
		if err != nil {
			return nil, fmt.Errorf("loading bulk backup run item files: %w", err)
		}
		item.FileCount = len(files)
		for _, file := range files {
			if file.SizeBytes > 0 {
				item.ByteCount += int64(file.SizeBytes)
			}
		}
		run.FileCount += item.FileCount
		run.ByteCount += item.ByteCount
	}
	return run, nil
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
	activeItems := s.markBulkRunBatchActive(items)
	for _, item := range activeItems {
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
		item.Status = domain.BulkBackupRunItemStatusActive
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

func (s *BackupService) markBulkRunBatchActive(items []domain.BulkBackupRunItem) []domain.BulkBackupRunItem {
	now := time.Now().UTC()
	activeItems := make([]domain.BulkBackupRunItem, 0, len(items))
	for _, item := range items {
		item.Status = domain.BulkBackupRunItemStatusActive
		item.Reason = ""
		item.UpdatedAt = now
		item.CompletedAt = nil
		if err := s.bulkRunRepo.UpdateRunItem(&item); err != nil {
			log.Printf("Warning: failed to mark bulk run item %s active: %v", item.ID, err)
			continue
		}
		s.recalculateBulkRunCounters(item.RunID)
		activeItems = append(activeItems, item)
	}
	return activeItems
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
				s.completeBulkRunItem(item, domain.BulkBackupRunItemStatusSuccess, "", item.BackupJobID)
			case domain.BackupStatusFailed:
				s.completeBulkRunItem(item, domain.BulkBackupRunItemStatusFailed, job.ErrorMessage, item.BackupJobID)
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
		if bulkRunItemTerminal(item.Status) ||
			item.Status == domain.BulkBackupRunItemStatusActive ||
			item.Status == domain.BulkBackupRunItemStatusRunning {
			continue
		}
		s.completeBulkRunItem(item, domain.BulkBackupRunItemStatusCancelled, "bulk backup cancelled", nil)
	}
	s.recalculateBulkRunCounters(runID)
}

func runningBulkRunItems(items []domain.BulkBackupRunItem) []domain.BulkBackupRunItem {
	running := make([]domain.BulkBackupRunItem, 0)
	for _, item := range items {
		if item.Status == domain.BulkBackupRunItemStatusActive ||
			item.Status == domain.BulkBackupRunItemStatusRunning {
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
		return
	}
	durationStart := run.CreatedAt
	if run.StartedAt != nil {
		durationStart = *run.StartedAt
	}
	duration := time.Duration(0)
	if run.CompletedAt != nil && !durationStart.IsZero() {
		duration = run.CompletedAt.Sub(durationStart)
	}
	if hydratedRun, err := s.hydrateBulkBackupRunFileTotals(run); err != nil {
		log.Printf("Warning: failed to hydrate bulk backup run %s file totals for metrics: %v", runID, err)
	} else if hydratedRun != nil {
		run = hydratedRun
	}
	observability.Default().ObserveBulkOperationCompletion(
		"bulk_backup_run",
		"distributed",
		string(run.Status),
		duration,
		run.TotalCount,
		run.FileCount,
		run.ByteCount,
	)
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
