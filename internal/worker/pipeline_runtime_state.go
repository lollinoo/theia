package worker

import (
	"reflect"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/lollinoo/theia/internal/collector"
	"github.com/lollinoo/theia/internal/domain"
	"github.com/lollinoo/theia/internal/ws"
)

type pipelineRuntimeState struct {
	mu                 sync.RWMutex
	lastSnapshot       *ws.SnapshotPayload
	overviewVersion    uint64
	alertVersion       uint64
	promStatus         ws.PrometheusStatusPayload
	hostnames          map[uuid.UUID]string
	hostnameObservedAt map[uuid.UUID]time.Time
	alerts             map[uuid.UUID][]domain.AlertState
	prevCounters       map[uuid.UUID]map[string]collector.CounterBaseline
	prevHashes         *sectionHashes
	now                func() time.Time
}

func newPipelineRuntimeState(initialPromStatus ws.PrometheusStatusPayload) *pipelineRuntimeState {
	return &pipelineRuntimeState{
		lastSnapshot:       ws.EmptySnapshot(),
		promStatus:         initialPromStatus,
		hostnames:          make(map[uuid.UUID]string),
		hostnameObservedAt: make(map[uuid.UUID]time.Time),
		alerts:             make(map[uuid.UUID][]domain.AlertState),
		prevCounters:       make(map[uuid.UUID]map[string]collector.CounterBaseline),
		now:                time.Now,
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
