package domain

// This file defines change event domain contracts and lifecycle invariants.

import "github.com/google/uuid"

// ChangeKind represents change kind data used by the domain model.
type ChangeKind string

const (
	ChangeKindCreated ChangeKind = "created"
	ChangeKindUpdated ChangeKind = "updated"
	ChangeKindDeleted ChangeKind = "deleted"
)

// DeviceChangeEvent represents device change event data used by the domain model.
type DeviceChangeEvent struct {
	Kind     ChangeKind
	DeviceID uuid.UUID
}

// LinkChangeEvent represents link change event data used by the domain model.
type LinkChangeEvent struct {
	Kind   ChangeKind
	LinkID uuid.UUID
}
