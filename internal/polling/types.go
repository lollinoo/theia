package polling

// This file defines types polling policy and freshness-budget behavior.

import "time"

// Lane represents lane data used by the package.
type Lane string

const (
	LaneEssential               Lane = "essential"
	LaneBackground              Lane = "background"
	LanePerformanceCounterWalks Lane = "performance_counter_walks"
	LaneBootstrap               Lane = "bootstrap"
	LaneQuarantine              Lane = "quarantine"
)

// TaskKind represents task kind data used by the package.
type TaskKind string

const (
	TaskKindEssential  TaskKind = "essential"
	TaskKindBackground TaskKind = "background"
	TaskKindBootstrap  TaskKind = "bootstrap"
)

// TriState represents tri state data used by the package.
type TriState string

const (
	TriStateTrue    TriState = "true"
	TriStateFalse   TriState = "false"
	TriStateUnknown TriState = "unknown"
)

// FieldState represents field state data used by the package.
type FieldState string

const (
	FieldStateOK      FieldState = "ok"
	FieldStateMissing FieldState = "missing"
	FieldStateError   FieldState = "error"
	FieldStateStale   FieldState = "stale"
)

// PollStatus represents poll status data used by the package.
type PollStatus string

const (
	PollStatusComplete PollStatus = "complete"
	PollStatusPartial  PollStatus = "partial"
	PollStatusFailed   PollStatus = "failed"
)

// PrimaryHealth represents primary health data used by the package.
type PrimaryHealth string

const (
	PrimaryHealthProbing      PrimaryHealth = "probing"
	PrimaryHealthUpFresh      PrimaryHealth = "up_fresh"
	PrimaryHealthUpStale      PrimaryHealth = "up_stale"
	PrimaryHealthSNMPDegraded PrimaryHealth = "snmp_degraded"
	PrimaryHealthUnreachable  PrimaryHealth = "unreachable"
	PrimaryHealthQuarantined  PrimaryHealth = "quarantined"
)

// RuntimeFlag represents runtime flag data used by the package.
type RuntimeFlag string

const (
	FlagDeadlineMissed     RuntimeFlag = "deadline_missed"
	FlagOverloaded         RuntimeFlag = "overloaded"
	FlagBackgroundPending  RuntimeFlag = "background_pending"
	FlagPartialTelemetry   RuntimeFlag = "partial_telemetry"
	FlagDegradedRisk       RuntimeFlag = "degraded_risk"
	FlagPersistenceLagging RuntimeFlag = "persistence_lagging"
)

// TimeoutProfile represents timeout profile data used by the package.
type TimeoutProfile struct {
	Timeout time.Duration
	Retries int
}

// NetworkProbeResult reports one TCP probe outcome used by network reachability.
type NetworkProbeResult struct {
	Port      int    `json:"port"`
	Reachable bool   `json:"reachable"`
	Error     string `json:"error,omitempty"`
}
