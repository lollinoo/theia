package worker

// This file defines pipeline runtime state worker behavior, background lifecycle, and runtime state updates.

import (
	"reflect"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/lollinoo/theia/internal/collector"
	"github.com/lollinoo/theia/internal/domain"
	"github.com/lollinoo/theia/internal/polling"
	"github.com/lollinoo/theia/internal/ws"
)

type pipelineRuntimeState struct {
	mu                   sync.RWMutex
	lastSnapshot         *ws.SnapshotPayload
	overviewVersion      uint64
	overviewStreamID     string
	overviewJournal      *overviewJournal
	topologyVersion      uint64
	alertVersion         uint64
	promStatus           ws.PrometheusStatusPayload
	hostnames            map[uuid.UUID]string
	hostnameObservedAt   map[uuid.UUID]time.Time
	alerts               map[uuid.UUID][]domain.AlertState
	previousAlertRuntime map[uuid.UUID]ws.DeviceRuntimeDTO
	lastPollingHealth    polling.HealthSnapshot
	lastPollingHealthAt  time.Time
	prevCounters         map[uuid.UUID]map[string]collector.CounterBaseline
	counterWalkCooldowns map[uuid.UUID]map[string]counterWalkCooldownState
	prevHashes           *sectionHashes
	now                  func() time.Time
}

type counterWalkCooldownState struct {
	ConsecutiveTimeouts int
	BackoffLevel        int
	CooldownUntil       time.Time
	LastAttemptAt       time.Time
	LastSuccessAt       time.Time
	LastResult          string
}

func newPipelineRuntimeState(initialPromStatus ws.PrometheusStatusPayload) *pipelineRuntimeState {
	return &pipelineRuntimeState{
		lastSnapshot:         ws.EmptySnapshot(),
		overviewStreamID:     uuid.NewString(),
		overviewJournal:      newOverviewJournal(overviewJournalCapacity, overviewJournalMaxBytes),
		promStatus:           initialPromStatus,
		hostnames:            make(map[uuid.UUID]string),
		hostnameObservedAt:   make(map[uuid.UUID]time.Time),
		alerts:               make(map[uuid.UUID][]domain.AlertState),
		previousAlertRuntime: make(map[uuid.UUID]ws.DeviceRuntimeDTO),
		prevCounters:         make(map[uuid.UUID]map[string]collector.CounterBaseline),
		counterWalkCooldowns: make(map[uuid.UUID]map[string]counterWalkCooldownState),
		now:                  time.Now,
	}
}

func (s *pipelineRuntimeState) getOverviewSnapshot() (*ws.SnapshotPayload, uint64) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return ws.CloneSnapshot(s.lastSnapshot), s.overviewVersion
}

func (s *pipelineRuntimeState) isPromAvailable() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.promStatus.Enabled && s.promStatus.Available
}

func (s *pipelineRuntimeState) getPrometheusStatus() ws.PrometheusStatusPayload {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.promStatus
}

func (s *pipelineRuntimeState) getAlerts() ws.AlertMessagePayload {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return ws.AlertMessagePayload{
		Version: s.alertVersion,
		Alerts:  ws.AlertsToDTOs(flattenAlerts(cloneAlertGroups(s.alerts))),
	}
}

func (s *pipelineRuntimeState) setAlerts(next map[uuid.UUID][]domain.AlertState) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	previous := ws.AlertsToDTOs(flattenAlerts(cloneAlertGroups(s.alerts)))
	current := ws.AlertsToDTOs(flattenAlerts(cloneAlertGroups(next)))
	changed := !reflect.DeepEqual(previous, current)
	s.alerts = next
	if changed {
		s.alertVersion++
	}
	return changed
}

func (s *pipelineRuntimeState) setPrometheusStatus(status ws.PrometheusStatusPayload) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	changed := s.promStatus != status
	s.promStatus = status
	return changed
}

func (s *pipelineRuntimeState) recordPrometheusHostname(deviceID uuid.UUID, hostname string) {
	if hostname == "" {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.hostnames[deviceID] = hostname
	s.hostnameObservedAt[deviceID] = s.clockNow()
}

func (s *pipelineRuntimeState) clearPrometheusHostnames() {
	s.mu.Lock()
	defer s.mu.Unlock()
	clear(s.hostnames)
	clear(s.hostnameObservedAt)
}

func (s *pipelineRuntimeState) resetDeviceRuntime(deviceID uuid.UUID) {
	if s == nil || deviceID == uuid.Nil {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.prevCounters, deviceID)
	delete(s.counterWalkCooldowns, deviceID)
	delete(s.hostnames, deviceID)
	delete(s.hostnameObservedAt, deviceID)
}

func (s *pipelineRuntimeState) ShouldSkipCounterWalk(deviceID uuid.UUID, operation string, now time.Time) bool {
	if s == nil || deviceID == uuid.Nil || operation == "" {
		return false
	}
	if now.IsZero() {
		now = s.clockNow()
	}
	now = now.UTC()

	s.mu.Lock()
	defer s.mu.Unlock()
	operations := s.counterWalkCooldowns[deviceID]
	if len(operations) == 0 {
		return false
	}
	state := operations[operation]
	return !state.CooldownUntil.IsZero() && now.Before(state.CooldownUntil)
}

func (s *pipelineRuntimeState) RecordCounterWalkResult(deviceID uuid.UUID, operation string, result string, now time.Time, expectedInterval time.Duration) {
	if s == nil || deviceID == uuid.Nil || operation == "" {
		return
	}
	if result == "skipped_cooldown" {
		return
	}
	if now.IsZero() {
		now = s.clockNow()
	}
	now = now.UTC()

	s.mu.Lock()
	defer s.mu.Unlock()
	switch result {
	case "success":
		s.clearCounterWalkCooldownLocked(deviceID, operation)
	case "timeout":
		if s.counterWalkCooldowns == nil {
			s.counterWalkCooldowns = make(map[uuid.UUID]map[string]counterWalkCooldownState)
		}
		operations := s.counterWalkCooldowns[deviceID]
		if operations == nil {
			operations = make(map[string]counterWalkCooldownState)
			s.counterWalkCooldowns[deviceID] = operations
		}
		state := operations[operation]
		state.ConsecutiveTimeouts++
		state.LastAttemptAt = now
		state.LastResult = result
		if state.ConsecutiveTimeouts >= 2 {
			duration := counterWalkCooldownDuration(expectedInterval, state.BackoffLevel)
			state.CooldownUntil = now.Add(duration)
			state.BackoffLevel++
		}
		operations[operation] = state
	default:
		s.clearCounterWalkCooldownLocked(deviceID, operation)
	}
}

func (s *pipelineRuntimeState) clearCounterWalkCooldownLocked(deviceID uuid.UUID, operation string) {
	operations := s.counterWalkCooldowns[deviceID]
	if len(operations) == 0 {
		return
	}
	delete(operations, operation)
	if len(operations) == 0 {
		delete(s.counterWalkCooldowns, deviceID)
	}
}

func counterWalkCooldownDuration(expectedInterval time.Duration, backoffLevel int) time.Duration {
	const (
		minCooldown = time.Minute
		maxCooldown = 30 * time.Minute
	)
	if expectedInterval <= 0 {
		expectedInterval = minCooldown
	}
	duration := expectedInterval
	for i := 0; i < backoffLevel; i++ {
		if duration >= maxCooldown/2 {
			duration = maxCooldown
			break
		}
		duration *= 2
	}
	if duration < minCooldown {
		return minCooldown
	}
	if duration > maxCooldown {
		return maxCooldown
	}
	return duration
}

func (s *pipelineRuntimeState) prunePrometheusHostnames() {
	cutoff := s.clockNow().Add(-prometheusEnrichmentRetention)

	s.mu.Lock()
	defer s.mu.Unlock()
	for deviceID, observedAt := range s.hostnameObservedAt {
		if observedAt.After(cutoff) {
			continue
		}
		delete(s.hostnameObservedAt, deviceID)
		delete(s.hostnames, deviceID)
	}
}

func (s *pipelineRuntimeState) clockNow() time.Time {
	if s != nil && s.now != nil {
		return s.now().UTC()
	}
	return time.Now().UTC()
}

func (p *PipelineOrchestrator) ResetDeviceRuntime(deviceID uuid.UUID) {
	if p == nil || deviceID == uuid.Nil {
		return
	}
	p.clearStaticPersistenceDedupe(deviceID)
	if p.runtime != nil {
		p.runtime.resetDeviceRuntime(deviceID)
	}
	if p.stateStore != nil {
		p.stateStore.Remove(deviceID)
	}
}

func (p *PipelineOrchestrator) clearStaticPersistenceDedupe(deviceID uuid.UUID) {
	if p == nil || deviceID == uuid.Nil {
		return
	}
	p.staticPersistenceMu.Lock()
	defer p.staticPersistenceMu.Unlock()
	delete(p.staticPersistenceCache, deviceID)
}
