package domain

import (
	"time"

	"github.com/google/uuid"
)

// DiscoveryProtocol indicates how a link was discovered.
type DiscoveryProtocol string

const (
	DiscoveryProtocolLLDP   DiscoveryProtocol = "lldp"
	DiscoveryProtocolCDP    DiscoveryProtocol = "cdp"
	DiscoveryProtocolManual DiscoveryProtocol = "manual"
)

// Link represents a discovered or manually defined connection between two device interfaces.
type Link struct {
	ID                uuid.UUID         `json:"id"`
	SourceDeviceID    uuid.UUID         `json:"source_device_id"`
	SourceIfName      string            `json:"source_if_name"`
	TargetDeviceID    uuid.UUID         `json:"target_device_id"`
	TargetIfName      string            `json:"target_if_name"`
	DiscoveryProtocol DiscoveryProtocol `json:"discovery_protocol"`
	CreatedAt         time.Time         `json:"created_at"`
	UpdatedAt         time.Time         `json:"updated_at"`
}

type LinkUpsertKind string

const (
	LinkUpsertKindCreated    LinkUpsertKind = "created"
	LinkUpsertKindEnriched   LinkUpsertKind = "enriched"
	LinkUpsertKindReoriented LinkUpsertKind = "reoriented"
	LinkUpsertKindUpdated    LinkUpsertKind = "updated"
	LinkUpsertKindNoop       LinkUpsertKind = "noop"
)

// LinkUpsertResult reports whether an upsert inserted a new row and whether it
// changed topology-visible link state, including materialized map membership.
type LinkUpsertResult struct {
	Created bool
	Changed bool
	Kind    LinkUpsertKind
}

// LinkRepository defines persistence operations for links.
type LinkRepository interface {
	Create(link *Link) error
	// CreateManualIdempotent inserts a user-created manual link or returns an existing equivalent link without mutating it.
	// When browserLocalStorageMigration is true, equivalence is any unordered device pair because legacy browser state has no interface identities.
	CreateManualIdempotent(link *Link, browserLocalStorageMigration bool) (*Link, bool, error)
	GetByID(id uuid.UUID) (*Link, error)
	GetByDeviceID(deviceID uuid.UUID) ([]Link, error)
	GetAll() ([]Link, error)
	Update(link *Link) error
	Delete(id uuid.UUID) error
	// Upsert inserts a new link or updates an existing one matching
	// the same source+target interface pair. Returns true when a new
	// link was inserted, false when an existing link was updated.
	Upsert(link *Link) (bool, error)
}
