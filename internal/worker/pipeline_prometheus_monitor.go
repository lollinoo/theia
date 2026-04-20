package worker

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/lollinoo/theia/internal/domain"
	"github.com/lollinoo/theia/internal/ws"
)

type pipelinePrometheusMonitor struct {
	pipeline *PipelineOrchestrator
}

func (m *pipelinePrometheusMonitor) run(ctx context.Context) {
	p := m.pipeline
	defer close(p.healthDone)

	m.refreshOnce(ctx)

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.refreshOnce(ctx)
		}
	}
}

func (m *pipelinePrometheusMonitor) refreshOnce(ctx context.Context) {
	p := m.pipeline
	if p.prometheus == nil || p.cache == nil {
		return
	}

	promURL := m.prometheusURL()
	if promURL == "" {
		p.prometheus.SetPrometheusURL("")
		m.setAlerts(make(map[uuid.UUID][]domain.AlertState))
		m.clearHostnames()
		m.publishStatus(ws.PrometheusStatusPayload{
			Enabled:   false,
			Available: false,
		})
		return
	}

	p.prometheus.SetPrometheusURL(promURL)

	devices, err := p.cache.GetDevices()
	if err != nil {
		m.setAlerts(make(map[uuid.UUID][]domain.AlertState))
		m.pruneHostnames()
		m.publishStatus(ws.PrometheusStatusPayload{
			Enabled:   true,
			Available: false,
			Error:     err.Error(),
		})
		return
	}

	alerts, err := p.prometheus.CollectAlerts(ctx, devices)
	if err != nil {
		m.setAlerts(make(map[uuid.UUID][]domain.AlertState))
		m.pruneHostnames()
		m.publishStatus(ws.PrometheusStatusPayload{
			Enabled:   true,
			Available: false,
			Error:     err.Error(),
		})
		return
	}

	m.setAlerts(alerts)
	m.publishStatus(ws.PrometheusStatusPayload{
		Enabled:   true,
		Available: true,
	})
}

func (m *pipelinePrometheusMonitor) setAlerts(next map[uuid.UUID][]domain.AlertState) {
	p := m.pipeline
	changed := p.runtime.setAlerts(next)
	if !changed {
		return
	}

	select {
	case p.alertNotify <- struct{}{}:
	default:
	}
}

func (m *pipelinePrometheusMonitor) publishStatus(status ws.PrometheusStatusPayload) {
	p := m.pipeline
	changed := p.runtime.setPrometheusStatus(status)
	if !changed || p.hub == nil {
		return
	}

	p.hub.Broadcast(ws.Message{
		Type:    ws.MessageTypePrometheusStatus,
		Payload: status,
	})
}

func (m *pipelinePrometheusMonitor) recordHostname(deviceID uuid.UUID, hostname string) {
	m.pipeline.runtime.recordPrometheusHostname(deviceID, hostname)
}

func (m *pipelinePrometheusMonitor) clearHostnames() {
	m.pipeline.runtime.clearPrometheusHostnames()
}

func (m *pipelinePrometheusMonitor) pruneHostnames() {
	m.pipeline.runtime.prunePrometheusHostnames()
}

func (m *pipelinePrometheusMonitor) prometheusURL() string {
	p := m.pipeline
	if p.settingsRepo == nil {
		return ""
	}

	value, err := p.settingsRepo.Get(domain.SettingPrometheusURL)
	if err != nil {
		return ""
	}

	return strings.TrimSpace(value)
}

func initialPrometheusStatus(settingsRepo domain.SettingsRepository) ws.PrometheusStatusPayload {
	if settingsRepo == nil {
		return ws.PrometheusStatusPayload{}
	}

	value, err := settingsRepo.Get(domain.SettingPrometheusURL)
	if err != nil {
		return ws.PrometheusStatusPayload{}
	}

	enabled := strings.TrimSpace(value) != ""
	return ws.PrometheusStatusPayload{
		Enabled:   enabled,
		Available: enabled,
	}
}
