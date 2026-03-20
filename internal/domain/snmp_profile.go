package domain

import (
	"time"

	"github.com/google/uuid"
)

// SNMPProfile is a reusable set of SNMP credentials that can be applied to
// multiple devices, avoiding repeated manual entry of the same credentials.
type SNMPProfile struct {
	ID          uuid.UUID       `json:"id"`
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Credentials SNMPCredentials `json:"credentials"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
}

// SNMPProfileRepository defines persistence operations for SNMP credential profiles.
type SNMPProfileRepository interface {
	Create(profile *SNMPProfile) error
	GetByID(id uuid.UUID) (*SNMPProfile, error)
	GetAll() ([]SNMPProfile, error)
	Update(profile *SNMPProfile) error
	Delete(id uuid.UUID) error
}
