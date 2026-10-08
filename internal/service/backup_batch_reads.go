package service

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/domain"
)

func (s *BackupService) backupFilesByJobIDs(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID][]domain.BackupFile, error) {
	ids = dedupeUUIDs(ids)
	if repo, ok := s.fileRepo.(interface {
		GetByJobIDsContext(context.Context, []uuid.UUID) (map[uuid.UUID][]domain.BackupFile, error)
	}); ok {
		return repo.GetByJobIDsContext(ctx, ids)
	}
	files := make(map[uuid.UUID][]domain.BackupFile, len(ids))
	for _, id := range ids {
		if err := contextError(ctx); err != nil {
			return nil, err
		}
		get := s.fileRepo.GetByJobID
		if repo, ok := s.fileRepo.(interface {
			GetByJobIDContext(context.Context, uuid.UUID) ([]domain.BackupFile, error)
		}); ok {
			get = func(id uuid.UUID) ([]domain.BackupFile, error) { return repo.GetByJobIDContext(ctx, id) }
		}
		var err error
		files[id], err = get(id)
		if err != nil {
			return nil, fmt.Errorf("loading files for backup job %s: %w", id, err)
		}
	}
	return files, nil
}

func (s *BackupService) backupFileTotals(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]domain.BackupFileTotals, error) {
	ids = dedupeUUIDs(ids)
	if repo, ok := s.fileRepo.(interface {
		GetTotalsByJobIDsContext(context.Context, []uuid.UUID) (map[uuid.UUID]domain.BackupFileTotals, error)
	}); ok {
		return repo.GetTotalsByJobIDsContext(ctx, ids)
	}
	files, err := s.backupFilesByJobIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	totals := make(map[uuid.UUID]domain.BackupFileTotals, len(ids))
	for id, files := range files {
		total := domain.BackupFileTotals{FileCount: len(files)}
		for _, file := range files {
			if file.SizeBytes > 0 {
				total.ByteCount += int64(file.SizeBytes)
			}
		}
		totals[id] = total
	}
	return totals, nil
}

type bulkDownloadMetadata struct {
	devices map[uuid.UUID]domain.Device
	jobs    map[uuid.UUID]domain.BackupJob
	files   map[uuid.UUID][]domain.BackupFile
}

func (s *BackupService) bulkDownloadMetadata(ctx context.Context, ids []uuid.UUID) (*bulkDownloadMetadata, error) {
	deviceRepo, devicesBatched := s.deviceRepo.(interface {
		GetBackupDownloadDevicesContext(context.Context, []uuid.UUID) ([]domain.Device, error)
	})
	jobRepo, jobsBatched := s.jobRepo.(interface {
		GetLatestByDeviceIDsContext(context.Context, []uuid.UUID) (map[uuid.UUID]domain.BackupJob, error)
	})
	_, filesBatched := s.fileRepo.(interface {
		GetByJobIDsContext(context.Context, []uuid.UUID) (map[uuid.UUID][]domain.BackupFile, error)
	})
	if !devicesBatched || !jobsBatched || !filesBatched {
		return nil, nil
	}
	devices, err := deviceRepo.GetBackupDownloadDevicesContext(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("loading devices for bulk download: %w", err)
	}
	metadata := &bulkDownloadMetadata{devices: make(map[uuid.UUID]domain.Device, len(devices))}
	foundIDs := make([]uuid.UUID, 0, len(devices))
	for _, device := range devices {
		metadata.devices[device.ID] = device
		foundIDs = append(foundIDs, device.ID)
	}
	metadata.jobs, err = jobRepo.GetLatestByDeviceIDsContext(ctx, foundIDs)
	if err != nil {
		return nil, fmt.Errorf("loading latest backups for bulk download: %w", err)
	}
	jobIDs := make([]uuid.UUID, 0, len(metadata.jobs))
	for _, job := range metadata.jobs {
		jobIDs = append(jobIDs, job.ID)
	}
	metadata.files, err = s.backupFilesByJobIDs(ctx, jobIDs)
	if err != nil {
		return nil, fmt.Errorf("loading backup files for bulk download: %w", err)
	}
	return metadata, nil
}
