package service

// This file defines bulk backup selection backup and restore service behavior, including filesystem safety and cleanup expectations.

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/domain"
)

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
				Operation: "bulk backup run",
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
			Operation: "bulk backup run",
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
