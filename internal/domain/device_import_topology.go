package domain

// This file defines the persistent, map-scoped state used by one-time topology imports.

import (
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
)

var (
	// ErrDeviceImportTopologyRunNotFound hides run ownership and map membership.
	ErrDeviceImportTopologyRunNotFound = errors.New("device import topology run not found")
	// ErrDeviceImportTopologyRunConflict reports a stale or invalid run transition.
	ErrDeviceImportTopologyRunConflict = errors.New("device import topology run conflict")
	// ErrDeviceImportTopologyLayoutStale reports that the map changed after layout was computed.
	ErrDeviceImportTopologyLayoutStale = errors.New("device import topology layout is stale")
)

// DeviceImportTopologyLayoutScope controls how a completed import may reposition a saved map.
type DeviceImportTopologyLayoutScope string

const (
	DeviceImportTopologyLayoutScopePreserve   DeviceImportTopologyLayoutScope = "preserve"
	DeviceImportTopologyLayoutScopeReorganize DeviceImportTopologyLayoutScope = "reorganize"
)

// NormalizeDeviceImportTopologyLayoutScope returns the safe default for unknown layout scopes.
func NormalizeDeviceImportTopologyLayoutScope(scope DeviceImportTopologyLayoutScope) DeviceImportTopologyLayoutScope {
	switch DeviceImportTopologyLayoutScope(strings.TrimSpace(string(scope))) {
	case DeviceImportTopologyLayoutScopeReorganize:
		return DeviceImportTopologyLayoutScopeReorganize
	default:
		return DeviceImportTopologyLayoutScopePreserve
	}
}

// DeviceImportTopologyRunState identifies the durable phase of a topology bootstrap run.
type DeviceImportTopologyRunState string

const (
	DeviceImportTopologyRunStateImporting      DeviceImportTopologyRunState = "importing"
	DeviceImportTopologyRunStateDiscovering    DeviceImportTopologyRunState = "discovering"
	DeviceImportTopologyRunStateReconciling    DeviceImportTopologyRunState = "reconciling"
	DeviceImportTopologyRunStateFollowup       DeviceImportTopologyRunState = "followup"
	DeviceImportTopologyRunStateReadyForLayout DeviceImportTopologyRunState = "ready_for_layout"
	DeviceImportTopologyRunStateFailed         DeviceImportTopologyRunState = "failed"
	DeviceImportTopologyRunStateCompleted      DeviceImportTopologyRunState = "completed"
	DeviceImportTopologyRunStateSuperseded     DeviceImportTopologyRunState = "superseded"
)

// Active reports whether the run still owns discovery or layout work.
func (state DeviceImportTopologyRunState) Active() bool {
	switch state {
	case DeviceImportTopologyRunStateImporting,
		DeviceImportTopologyRunStateDiscovering,
		DeviceImportTopologyRunStateReconciling,
		DeviceImportTopologyRunStateFollowup,
		DeviceImportTopologyRunStateReadyForLayout,
		DeviceImportTopologyRunStateFailed:
		return true
	default:
		return false
	}
}

// DeviceImportTopologyItemState identifies one imported device's bootstrap outcome.
type DeviceImportTopologyItemState string

const (
	DeviceImportTopologyItemStateQueued    DeviceImportTopologyItemState = "queued"
	DeviceImportTopologyItemStateRunning   DeviceImportTopologyItemState = "running"
	DeviceImportTopologyItemStateSucceeded DeviceImportTopologyItemState = "succeeded"
	DeviceImportTopologyItemStateWarning   DeviceImportTopologyItemState = "warning"
	DeviceImportTopologyItemStateFailed    DeviceImportTopologyItemState = "failed"
)

// Terminal reports whether an item no longer has scheduled discovery work.
func (state DeviceImportTopologyItemState) Terminal() bool {
	switch state {
	case DeviceImportTopologyItemStateSucceeded,
		DeviceImportTopologyItemStateWarning,
		DeviceImportTopologyItemStateFailed:
		return true
	default:
		return false
	}
}

// DeviceImportTopologyResultCode is a stable, credential-free diagnostic category.
type DeviceImportTopologyResultCode string

const (
	DeviceImportTopologyResultNone                DeviceImportTopologyResultCode = ""
	DeviceImportTopologyResultDiscovered          DeviceImportTopologyResultCode = "discovered"
	DeviceImportTopologyResultNoNeighbors         DeviceImportTopologyResultCode = "no_neighbors"
	DeviceImportTopologyResultPartialProtocol     DeviceImportTopologyResultCode = "partial_protocol"
	DeviceImportTopologyResultUnresolvedNeighbors DeviceImportTopologyResultCode = "unresolved_neighbors"
	DeviceImportTopologyResultIncompletePorts     DeviceImportTopologyResultCode = "incomplete_ports"
	DeviceImportTopologyResultSNMPUnreachable     DeviceImportTopologyResultCode = "snmp_unreachable"
	DeviceImportTopologyResultAuthentication      DeviceImportTopologyResultCode = "authentication_failed"
	DeviceImportTopologyResultPersistence         DeviceImportTopologyResultCode = "persistence_failed"
	DeviceImportTopologyResultInternal            DeviceImportTopologyResultCode = "internal_error"
)

// DeviceImportTopologyRun is the durable map-level bootstrap operation.
type DeviceImportTopologyRun struct {
	ID                uuid.UUID                       `json:"id"`
	MapID             uuid.UUID                       `json:"map_id"`
	ActorUserID       uuid.UUID                       `json:"-"`
	FileDigest        string                          `json:"file_digest"`
	LayoutScope       DeviceImportTopologyLayoutScope `json:"layout_scope"`
	State             DeviceImportTopologyRunState    `json:"state"`
	AutoLayoutAllowed bool                            `json:"auto_layout_allowed"`
	Backgrounded      bool                            `json:"backgrounded"`
	LayoutInputToken  string                          `json:"layout_input_token,omitempty"`
	FailureCode       DeviceImportTopologyResultCode  `json:"failure_code,omitempty"`
	FailureMessage    string                          `json:"failure_message,omitempty"`
	FailureReference  string                          `json:"failure_reference,omitempty"`
	ReconcileAttempts int                             `json:"reconcile_attempts"`
	CreatedAt         time.Time                       `json:"created_at"`
	StartedAt         *time.Time                      `json:"started_at,omitempty"`
	CompletedAt       *time.Time                      `json:"completed_at,omitempty"`
	UpdatedAt         time.Time                       `json:"updated_at"`
}

// DeviceImportTopologyRunFailure is sanitized metadata for a durable run-level failure.
type DeviceImportTopologyRunFailure struct {
	Code      DeviceImportTopologyResultCode
	Message   string
	Reference string
}

// DeviceImportTopologyLayoutApply is one optimistic, atomic map-layout write.
type DeviceImportTopologyLayoutApply struct {
	InputToken        string           `json:"input_token"`
	Positions         []DevicePosition `json:"positions"`
	ResetLinkRouteIDs []uuid.UUID      `json:"reset_link_route_ids"`
}

// DeviceImportTopologyRunItem is the durable discovery outcome for one imported device.
type DeviceImportTopologyRunItem struct {
	RunID               uuid.UUID                      `json:"-"`
	DeviceID            uuid.UUID                      `json:"device_id"`
	State               DeviceImportTopologyItemState  `json:"state"`
	Attempt             int                            `json:"attempt"`
	ResultCode          DeviceImportTopologyResultCode `json:"result_code,omitempty"`
	Message             string                         `json:"message,omitempty"`
	Reference           string                         `json:"reference,omitempty"`
	NeighborCount       int                            `json:"neighbor_count"`
	LinksCreated        int                            `json:"links_created"`
	UnresolvedNeighbors int                            `json:"unresolved_neighbors"`
	StartedAt           *time.Time                     `json:"started_at,omitempty"`
	CompletedAt         *time.Time                     `json:"completed_at,omitempty"`
	UpdatedAt           time.Time                      `json:"updated_at"`
}

// DeviceImportTopologyItemCompletion is the sanitized terminal outcome emitted by discovery.
type DeviceImportTopologyItemCompletion struct {
	State               DeviceImportTopologyItemState
	ResultCode          DeviceImportTopologyResultCode
	Message             string
	Reference           string
	NeighborCount       int
	LinksCreated        int
	UnresolvedNeighbors int
	CompletedAt         time.Time
}

// DeviceImportTopologyItemReconciliation is the final map-scoped link outcome for one run item.
type DeviceImportTopologyItemReconciliation struct {
	DeviceID            uuid.UUID
	LinksCreated        int
	UnresolvedNeighbors int
}

// DeviceImportTopologyRunSnapshot is the authoritative run plus its ordered item results.
type DeviceImportTopologyRunSnapshot struct {
	Run   DeviceImportTopologyRun       `json:"run"`
	Items []DeviceImportTopologyRunItem `json:"items"`
}
