// Package collector defines stateless per-volatility-class collection
// contracts that later pipeline phases can schedule and consume.
package collector

// This file defines results metrics collection behavior and normalized collector output.

import (
	"time"

	"github.com/google/uuid"

	"github.com/lollinoo/theia/internal/domain"
	"github.com/lollinoo/theia/internal/polling"
	"github.com/lollinoo/theia/internal/snmp"
	"github.com/lollinoo/theia/internal/state"
)

// StateUpdate is the common collector result contract shared by every
// volatility-specific collector result type.
type StateUpdate interface {
	GetDeviceID() uuid.UUID
	GetVolatilityClass() domain.VolatilityClass
	GetCollectedAt() time.Time
}

// SNMPClient is the shared SNMP client contract reused by collector
// implementations. It extends the discovery polling interface with connection
// lifecycle methods.
type SNMPClient interface {
	snmp.ClientInterface
	Connect() error
	Close() error
}

// NewSNMPClientFunc constructs an SNMP client for a single device poll.
type NewSNMPClientFunc func(target string, creds domain.SNMPCredentials, timeout time.Duration, retries int) (SNMPClient, error)

// GetDeviceID returns the device identifier for this essential result.
func (r EssentialResult) GetDeviceID() uuid.UUID {
	return r.DeviceID
}

// GetVolatilityClass returns the performance volatility class constant.
func (r EssentialResult) GetVolatilityClass() domain.VolatilityClass {
	return domain.VolatilityClassPerformance
}

// GetCollectedAt returns when the essential result was collected.
func (r EssentialResult) GetCollectedAt() time.Time {
	return r.CollectedAt
}

func (r EssentialResult) ToStoreUpdate(expectedInterval time.Duration, deadlineMissed bool) state.StateUpdate {
	return state.StateUpdate{
		DeviceID:         r.DeviceID,
		VolatilityClass:  domain.VolatilityClassPerformance,
		Metrics:          r.toMetrics(),
		PollSuccess:      r.SNMPReachable == polling.TriStateTrue,
		ExpectedInterval: expectedInterval,
		Timestamp:        r.CollectedAt,
		Essential: &state.EssentialUpdate{
			PollStatus:                 r.PollStatus,
			NetworkReachable:           r.NetworkReachable,
			NetworkReachabilityResults: cloneNetworkProbeResults(r.NetworkReachabilityResults),
			SNMPReachable:              r.SNMPReachable,
			Uptime:                     r.Uptime,
			CPU:                        r.CPU,
			Memory:                     r.Memory,
			DeadlineMissed:             deadlineMissed,
		},
	}
}

func cloneNetworkProbeResults(results []polling.NetworkProbeResult) []polling.NetworkProbeResult {
	if len(results) == 0 {
		return nil
	}

	cloned := make([]polling.NetworkProbeResult, len(results))
	copy(cloned, results)
	return cloned
}

func (r EssentialResult) toMetrics() *domain.DeviceMetrics {
	if r.SNMPReachable != polling.TriStateTrue {
		return nil
	}
	return &domain.DeviceMetrics{
		DeviceID:    r.DeviceID,
		CPUPercent:  cloneFloat64Ptr(r.CPUPercent),
		MemPercent:  cloneFloat64Ptr(r.MemPercent),
		UptimeSecs:  cloneFloat64Ptr(r.UptimeSecs),
		CollectedAt: r.CollectedAt,
	}
}

// InterfaceCounterSnapshot stores the raw octet counters and discovered link
// speed metadata for one interface at a single collection time.
type InterfaceCounterSnapshot struct {
	IfName    string
	InOctets  uint64
	OutOctets uint64
	SpeedBps  int64
}

// PrometheusEnrichment reserves non-authoritative enrichment fields for later
// plans without changing the SNMP-primary collector contract.
type PrometheusEnrichment struct {
	DeviceID       uuid.UUID
	Hostname       string
	ProbeReachable *bool
	Alerts         []domain.AlertState
	CollectedAt    time.Time
	Error          string
}

// PerformanceResult carries the high-volatility SNMP metrics and raw interface
// counters for a single device poll.
type PerformanceResult struct {
	DeviceID    uuid.UUID
	Metrics     domain.DeviceMetrics
	Counters    []InterfaceCounterSnapshot
	Enrichment  PrometheusEnrichment
	CollectedAt time.Time
	Err         error
}

// GetDeviceID returns the device identifier for this performance result.
func (r PerformanceResult) GetDeviceID() uuid.UUID {
	return r.DeviceID
}

// GetVolatilityClass returns the performance volatility class constant.
func (r PerformanceResult) GetVolatilityClass() domain.VolatilityClass {
	return domain.VolatilityClassPerformance
}

// GetCollectedAt returns when the performance result was collected.
func (r PerformanceResult) GetCollectedAt() time.Time {
	return r.CollectedAt
}

// ToStoreUpdate adapts the performance result to the current state-engine
// update shape without redesigning the store contract.
func (r PerformanceResult) ToStoreUpdate(expectedInterval time.Duration) state.StateUpdate {
	pollSuccess := r.Err == nil
	var metrics *domain.DeviceMetrics
	if pollSuccess {
		cloned := cloneMetrics(r.Metrics)
		cloned.DeviceID = r.DeviceID
		cloned.CollectedAt = r.CollectedAt
		metrics = &cloned
	}

	return state.StateUpdate{
		DeviceID:         r.DeviceID,
		VolatilityClass:  domain.VolatilityClassPerformance,
		Metrics:          metrics,
		LinkMetrics:      nil,
		PollSuccess:      pollSuccess,
		ExpectedInterval: expectedInterval,
		Timestamp:        r.CollectedAt,
	}
}

// OperationalResult carries medium-volatility reachability and uptime data for
// a single device poll.
type OperationalResult struct {
	DeviceID          uuid.UUID
	Reachable         bool
	UptimeSecs        *float64
	InterfaceStatuses map[string]string
	CollectedAt       time.Time
	Err               error
}

// GetDeviceID returns the device identifier for this operational result.
func (r OperationalResult) GetDeviceID() uuid.UUID {
	return r.DeviceID
}

// GetVolatilityClass returns the operational volatility class constant.
func (r OperationalResult) GetVolatilityClass() domain.VolatilityClass {
	return domain.VolatilityClassOperational
}

// GetCollectedAt returns when the operational result was collected.
func (r OperationalResult) GetCollectedAt() time.Time {
	return r.CollectedAt
}

// ToStoreUpdate adapts the operational result to the current state-engine
// update shape while preserving reachability semantics.
func (r OperationalResult) ToStoreUpdate(expectedInterval time.Duration) state.StateUpdate {
	var metrics *domain.DeviceMetrics
	if r.UptimeSecs != nil {
		cloned := cloneFloat64Ptr(r.UptimeSecs)
		metrics = &domain.DeviceMetrics{
			DeviceID:    r.DeviceID,
			UptimeSecs:  cloned,
			CollectedAt: r.CollectedAt,
		}
	}

	return state.StateUpdate{
		DeviceID:         r.DeviceID,
		VolatilityClass:  domain.VolatilityClassOperational,
		Metrics:          metrics,
		LinkMetrics:      nil,
		PollSuccess:      r.Reachable,
		ExpectedInterval: expectedInterval,
		Timestamp:        r.CollectedAt,
	}
}

// StaticResult carries low-volatility inventory and topology data for a single
// device poll.
type StaticResult struct {
	DeviceID                   uuid.UUID
	Metrics                    domain.DeviceMetrics
	SysName                    string
	SysDescr                   string
	SysObjectID                string
	HardwareModel              string
	OSVersion                  string
	Vendor                     string
	DeviceType                 domain.DeviceType
	Interfaces                 []domain.Interface
	Neighbors                  []snmp.NeighborInfo
	NeighborDiscoveryProtocols []domain.DiscoveryProtocol
	NeighborDiscoveryFailures  []snmp.NeighborDiscoveryFailure
	CollectedAt                time.Time
	Err                        error
}

// GetDeviceID returns the device identifier for this static result.
func (r StaticResult) GetDeviceID() uuid.UUID {
	return r.DeviceID
}

// GetVolatilityClass returns the static volatility class constant.
func (r StaticResult) GetVolatilityClass() domain.VolatilityClass {
	return domain.VolatilityClassStatic
}

// GetCollectedAt returns when the static result was collected.
func (r StaticResult) GetCollectedAt() time.Time {
	return r.CollectedAt
}

func cloneMetrics(metrics domain.DeviceMetrics) domain.DeviceMetrics {
	cloned := metrics
	cloned.CPUPercent = cloneFloat64Ptr(metrics.CPUPercent)
	cloned.MemPercent = cloneFloat64Ptr(metrics.MemPercent)
	cloned.TempCelsius = cloneFloat64Ptr(metrics.TempCelsius)
	cloned.UptimeSecs = cloneFloat64Ptr(metrics.UptimeSecs)
	return cloned
}

func cloneFloat64Ptr(value *float64) *float64 {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}
