package domain

// This file defines poll class domain contracts and lifecycle invariants.

import "time"

// PollClass represents the polling-cadence tier assigned to a device.
// PollClass governs only the performance volatility group (CPU, memory,
// temperature, interface counters). Operational and static groups are
// system-defined intervals shared across all devices (see
// OperationalClassInterval, StaticClassInterval) per D-08.
type PollClass string

const (
	// PollClassCore is the fastest tier. Used for routers and switches —
	// critical infrastructure where short polling latency matters.
	PollClassCore PollClass = "core"

	// PollClassStandard is the default tier. Used for access points and
	// any device with an unknown type.
	PollClassStandard PollClass = "standard"

	// PollClassLow is the slowest tier. Used for virtual / representative
	// nodes that change rarely.
	PollClassLow PollClass = "low"
)

// VolatilityClass represents how frequently an OID group changes value
// and therefore which polling cadence applies. Vendor YAML groups OIDs
// into these tiers; collectors poll one tier per cycle.
type VolatilityClass string

const (
	// VolatilityClassStatic covers inventory and topology data (sysDescr,
	// sysObjectID, ifTable, LLDP/CDP). Polled on the slowest schedule.
	VolatilityClassStatic VolatilityClass = "static"

	// VolatilityClassOperational covers reachability and link state
	// (sysUpTime, ifOperStatus). Polled on a medium schedule.
	VolatilityClassOperational VolatilityClass = "operational"

	// VolatilityClassPerformance covers counters and gauges (CPU, memory,
	// temperature, ifHCInOctets/ifHCOutOctets). Polled on the fastest
	// schedule and governed by PollClass.
	VolatilityClassPerformance VolatilityClass = "performance"
)

// Performance-class polling intervals — the cadence each PollClass uses
// for the performance volatility group. Hardcoded per D-06/D-07; making
// these configurable is deferred to a future milestone, matching the
// state-engine threshold constants.
const (
	PollClassCoreInterval     = 30 * time.Second
	PollClassStandardInterval = 60 * time.Second
	PollClassLowInterval      = 300 * time.Second
)

// System-wide intervals for the non-performance volatility groups.
// These are NOT scoped to PollClass — every device shares them per D-08.
const (
	// OperationalClassInterval is the cadence for sysUpTime / ifOperStatus
	// reachability checks across all devices.
	OperationalClassInterval = 60 * time.Second

	// StaticClassInterval is the cadence for inventory / topology walks
	// across all devices.
	StaticClassInterval = 300 * time.Second
)

// ClassifyPollClass returns the PollClass for a given DeviceType per the
// mapping in D-04. Unknown / empty values fall back to PollClassStandard.
// This helper is the single source of truth used by both the runtime
// (DeviceService auto-reclassification) and the data migration that
// backfills existing rows.
func ClassifyPollClass(deviceType DeviceType) PollClass {
	switch deviceType {
	case DeviceTypeRouter, DeviceTypeSwitch:
		return PollClassCore
	case DeviceTypeAP:
		return PollClassStandard
	case DeviceTypeVirtual:
		return PollClassLow
	case DeviceTypeUnknown, "":
		return PollClassStandard
	default:
		return PollClassStandard
	}
}

// Interval returns the performance-tier polling cadence for this
// PollClass. Unknown / empty PollClass values fall back to the standard
// interval so a corrupt DB row never crashes the scheduler.
func (c PollClass) Interval() time.Duration {
	switch c {
	case PollClassCore:
		return PollClassCoreInterval
	case PollClassStandard:
		return PollClassStandardInterval
	case PollClassLow:
		return PollClassLowInterval
	default:
		return PollClassStandardInterval
	}
}
