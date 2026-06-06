package domain

// This file defines credential profile domain contracts and lifecycle invariants.

import (
	"time"

	"github.com/google/uuid"
)

// CredentialProfile represents a reusable set of credentials shared across devices.
// Supports SSH-based auth methods (password, key) with an encrypted secret.
type CredentialProfile struct {
	ID              uuid.UUID     `json:"id"`
	Name            string        `json:"name"`
	Description     string        `json:"description"`
	Username        string        `json:"username"`
	Port            int           `json:"port"`
	AuthMethod      SSHAuthMethod `json:"auth_method"`
	EncryptedSecret string        `json:"-"` // never serialized — MUST preserve json:"-" (T-23-01)
	Role            string        `json:"role"`
	CreatedAt       time.Time     `json:"created_at"`
	UpdatedAt       time.Time     `json:"updated_at"`
}

// CredentialProfileRepository defines persistence operations for credential profiles.
type CredentialProfileRepository interface {
	Create(profile *CredentialProfile) error
	GetByID(id uuid.UUID) (*CredentialProfile, error)
	GetAll() ([]CredentialProfile, error)
	Update(profile *CredentialProfile) error
	Delete(id uuid.UUID) error
	// GetBackupProfileForDevice returns the first assigned (non-WinBox) credential
	// profile for the given device, or an error if none is assigned.
	GetBackupProfileForDevice(deviceID uuid.UUID) (*CredentialProfile, error)
}
