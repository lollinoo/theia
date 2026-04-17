package domain

import "github.com/google/uuid"

type ChangeKind string

const (
	ChangeKindCreated ChangeKind = "created"
	ChangeKindUpdated ChangeKind = "updated"
	ChangeKindDeleted ChangeKind = "deleted"
)

type DeviceChangeEvent struct {
	Kind     ChangeKind
	DeviceID uuid.UUID
}

type LinkChangeEvent struct {
	Kind   ChangeKind
	LinkID uuid.UUID
}
