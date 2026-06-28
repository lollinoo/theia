package service

// This file defines explicit SSH host-key reset behavior for device backups.

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/domain"
)

// ResetSSHHostKey forgets the remembered SSH host key for a device backup target.
func (s *BackupService) ResetSSHHostKey(ctx context.Context, deviceID uuid.UUID) (SSHHostKeyResetResult, error) {
	var result SSHHostKeyResetResult
	if err := contextError(ctx); err != nil {
		return result, err
	}
	if s.hostKeyStore == nil {
		return result, ErrSSHHostKeyStoreUnavailable
	}

	device, err := s.deviceRepo.GetByID(deviceID)
	if err != nil {
		return result, fmt.Errorf("getting device: %w", err)
	}
	profile, err := s.credentialProfileRepo.GetBackupProfileForDevice(device.ID)
	if err != nil {
		return result, fmt.Errorf("no credential profile assigned to device %s", deviceID)
	}

	target := domain.BackupAddress(*device)
	removed, err := s.hostKeyStore.RemoveHost(target, profile.Port)
	if err != nil {
		return result, fmt.Errorf("resetting SSH host key for %s:%d: %w", target, profile.Port, err)
	}
	return SSHHostKeyResetResult{
		Target:  target,
		Port:    profile.Port,
		Removed: removed,
	}, nil
}
