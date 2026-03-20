package domain

import (
	"time"

	"github.com/google/uuid"
)

// SSHProfile represents a reusable set of SSH credentials shared across devices.
type SSHProfile struct {
	ID              uuid.UUID     `json:"id"`
	Name            string        `json:"name"`
	Description     string        `json:"description"`
	Username        string        `json:"username"`
	Port            int           `json:"port"`
	AuthMethod      SSHAuthMethod `json:"auth_method"`
	EncryptedSecret string        `json:"-"` // never serialized
	CreatedAt       time.Time     `json:"created_at"`
	UpdatedAt       time.Time     `json:"updated_at"`
}

// SSHProfileRepository defines persistence operations for SSH profiles.
type SSHProfileRepository interface {
	Create(profile *SSHProfile) error
	GetByID(id uuid.UUID) (*SSHProfile, error)
	GetAll() ([]SSHProfile, error)
	Update(profile *SSHProfile) error
	Delete(id uuid.UUID) error
}
