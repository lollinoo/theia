package polling

import "time"

type Lane string

const (
	LaneEssential  Lane = "essential"
	LaneBackground Lane = "background"
	LaneBootstrap  Lane = "bootstrap"
	LaneQuarantine Lane = "quarantine"
)

type TaskKind string

const (
	TaskKindEssential  TaskKind = "essential"
	TaskKindBackground TaskKind = "background"
	TaskKindBootstrap  TaskKind = "bootstrap"
)

type TriState string

const (
	TriStateTrue    TriState = "true"
	TriStateFalse   TriState = "false"
	TriStateUnknown TriState = "unknown"
)

type FieldState string

const (
	FieldStateOK      FieldState = "ok"
	FieldStateMissing FieldState = "missing"
	FieldStateError   FieldState = "error"
	FieldStateStale   FieldState = "stale"
)

type PollStatus string

const (
	PollStatusComplete PollStatus = "complete"
	PollStatusPartial  PollStatus = "partial"
	PollStatusFailed   PollStatus = "failed"
)

type PrimaryHealth string

const (
	PrimaryHealthProbing      PrimaryHealth = "probing"
	PrimaryHealthUpFresh      PrimaryHealth = "up_fresh"
	PrimaryHealthUpStale      PrimaryHealth = "up_stale"
	PrimaryHealthSNMPDegraded PrimaryHealth = "snmp_degraded"
	PrimaryHealthUnreachable  PrimaryHealth = "unreachable"
	PrimaryHealthQuarantined  PrimaryHealth = "quarantined"
)

type RuntimeFlag string

const (
	FlagDeadlineMissed     RuntimeFlag = "deadline_missed"
	FlagOverloaded         RuntimeFlag = "overloaded"
	FlagBackgroundPending  RuntimeFlag = "background_pending"
	FlagPartialTelemetry   RuntimeFlag = "partial_telemetry"
	FlagDegradedRisk       RuntimeFlag = "degraded_risk"
	FlagPersistenceLagging RuntimeFlag = "persistence_lagging"
)

type TimeoutProfile struct {
	Timeout time.Duration
	Retries int
}
