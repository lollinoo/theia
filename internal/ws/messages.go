package ws

import (
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/domain"
)

const (
	// MessageTypeSnapshot pushes the full state to clients.
	MessageTypeSnapshot = "snapshot"
	// MessageTypeSnapshotDelta pushes only changed entries since the last broadcast.
	MessageTypeSnapshotDelta = "snapshot_delta"
	// MessageTypeMetrics carries device metrics-only payloads.
	MessageTypeMetrics = "metrics"
	// MessageTypeLinkMetrics carries link metrics-only payloads.
	MessageTypeLinkMetrics = "link_metrics"
	// MessageTypeAlert carries alert-only payloads.
	MessageTypeAlert = "alert"
	// MessageTypePrometheusStatus notifies clients of Prometheus availability changes.
	MessageTypePrometheusStatus = "prometheus_status"
	// MessageTypeTopologyChanged notifies clients that the topology has changed (new links discovered).
	MessageTypeTopologyChanged = "topology_changed"
	// MessageTypeSubscribeDetail registers a device-specific detail subscription for one client.
	MessageTypeSubscribeDetail = "subscribe_detail"
	// MessageTypeUnsubscribeDetail clears the active device-specific detail subscription for one client.
	MessageTypeUnsubscribeDetail = "unsubscribe_detail"
)

// PrometheusStatusPayload is sent when Prometheus availability changes.
type PrometheusStatusPayload struct {
	Available bool   `json:"available"`
	Error     string `json:"error,omitempty"`
}

// Message is the WebSocket envelope used for all server pushes.
type Message struct {
	Type    string `json:"type"`
	Payload any    `json:"payload"`
}

type clientControlMessage struct {
	Type     string
	DeviceID uuid.UUID
}

type clientControlEnvelope struct {
	Type    string               `json:"type"`
	Payload clientControlPayload `json:"payload"`
}

type clientControlPayload struct {
	DeviceID string `json:"device_id"`
}

// SnapshotPayload contains the complete live state sent to clients.
type SnapshotPayload struct {
	DeviceMetrics   map[string]DeviceMetricsDTO `json:"device_metrics"`
	LinkMetrics     map[string][]LinkMetricsDTO `json:"link_metrics"`
	Alerts          []AlertDTO                  `json:"alerts"`
	DeviceStatuses  map[string]string           `json:"device_statuses"`
	DeviceHostnames map[string]string           `json:"device_hostnames"` // device ID → auto-discovered hostname
	DeviceModels    map[string]string           `json:"device_models"`    // device ID → hardware model
}

// DeviceMetricsDTO is the frontend JSON shape for device metrics.
type DeviceMetricsDTO struct {
	DeviceID                    string   `json:"device_id"`
	CPUPercent                  *float64 `json:"cpu_percent"`
	MemPercent                  *float64 `json:"mem_percent"`
	TempCelsius                 *float64 `json:"temp_celsius"`
	UptimeSecs                  *float64 `json:"uptime_secs"`
	CollectedAt                 string   `json:"collected_at"`
	Health                      string   `json:"health,omitempty"`
	Reachability                string   `json:"reachability,omitempty"`
	Stale                       *bool    `json:"stale,omitempty"`
	LastPolledAt                string   `json:"last_polled_at,omitempty"`
	ExpectedPollIntervalSeconds *int64   `json:"expected_poll_interval_seconds,omitempty"`
}

// LinkMetricsDTO is the frontend JSON shape for interface/link metrics.
type LinkMetricsDTO struct {
	DeviceID    string   `json:"device_id"`
	IfName      string   `json:"if_name"`
	TxBps       *float64 `json:"tx_bps"`
	RxBps       *float64 `json:"rx_bps"`
	Utilization *float64 `json:"utilization"`
	CollectedAt string   `json:"collected_at"`
}

// AlertDTO is the frontend JSON shape for Prometheus alerts.
type AlertDTO struct {
	DeviceID  string `json:"device_id"`
	Severity  string `json:"severity"`
	AlertName string `json:"alert_name"`
	State     string `json:"state"`
	Summary   string `json:"summary"`
}

// EmptySnapshot returns a fully initialized empty snapshot payload.
func EmptySnapshot() *SnapshotPayload {
	return &SnapshotPayload{
		DeviceMetrics:   map[string]DeviceMetricsDTO{},
		LinkMetrics:     map[string][]LinkMetricsDTO{},
		Alerts:          []AlertDTO{},
		DeviceStatuses:  map[string]string{},
		DeviceHostnames: map[string]string{},
		DeviceModels:    map[string]string{},
	}
}

// CloneSnapshot makes a deep copy so callers can safely share snapshots.
func CloneSnapshot(snapshot *SnapshotPayload) *SnapshotPayload {
	if snapshot == nil {
		return EmptySnapshot()
	}

	cloned := &SnapshotPayload{
		DeviceMetrics:   make(map[string]DeviceMetricsDTO, len(snapshot.DeviceMetrics)),
		LinkMetrics:     make(map[string][]LinkMetricsDTO, len(snapshot.LinkMetrics)),
		Alerts:          append([]AlertDTO(nil), snapshot.Alerts...),
		DeviceStatuses:  make(map[string]string, len(snapshot.DeviceStatuses)),
		DeviceHostnames: make(map[string]string, len(snapshot.DeviceHostnames)),
		DeviceModels:    make(map[string]string, len(snapshot.DeviceModels)),
	}

	for key, value := range snapshot.DeviceStatuses {
		cloned.DeviceStatuses[key] = value
	}

	for key, value := range snapshot.DeviceHostnames {
		cloned.DeviceHostnames[key] = value
	}

	for key, value := range snapshot.DeviceModels {
		cloned.DeviceModels[key] = value
	}

	for key, value := range snapshot.DeviceMetrics {
		cloned.DeviceMetrics[key] = value
	}

	for key, values := range snapshot.LinkMetrics {
		cloned.LinkMetrics[key] = append([]LinkMetricsDTO(nil), values...)
	}

	return cloned
}

func parseClientControlMessage(raw []byte) (clientControlMessage, error) {
	var envelope clientControlEnvelope
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return clientControlMessage{}, fmt.Errorf("unmarshal client control message: %w", err)
	}

	switch envelope.Type {
	case MessageTypeSubscribeDetail, MessageTypeUnsubscribeDetail:
	default:
		return clientControlMessage{}, fmt.Errorf("unsupported client control type %q", envelope.Type)
	}

	if envelope.Payload.DeviceID == "" {
		if envelope.Type == MessageTypeUnsubscribeDetail {
			return clientControlMessage{Type: envelope.Type, DeviceID: uuid.Nil}, nil
		}
		return clientControlMessage{}, fmt.Errorf("missing payload.device_id for %s", envelope.Type)
	}

	deviceID, err := uuid.Parse(envelope.Payload.DeviceID)
	if err != nil {
		return clientControlMessage{}, fmt.Errorf("parse payload.device_id for %s: %w", envelope.Type, err)
	}

	if envelope.Type == MessageTypeSubscribeDetail && deviceID == uuid.Nil {
		return clientControlMessage{}, fmt.Errorf("payload.device_id for %s must be non-nil", envelope.Type)
	}

	return clientControlMessage{
		Type:     envelope.Type,
		DeviceID: deviceID,
	}, nil
}

// DeviceMetricsToDTOs converts domain metrics keyed by device ID into DTOs.
func DeviceMetricsToDTOs(metrics map[string]domain.DeviceMetrics) map[string]DeviceMetricsDTO {
	dtos := make(map[string]DeviceMetricsDTO, len(metrics))
	for key, metric := range metrics {
		deviceID := key
		if deviceID == "" && metric.DeviceID != uuid.Nil {
			deviceID = metric.DeviceID.String()
		}
		if deviceID == "" {
			continue
		}

		dtos[deviceID] = DeviceMetricsDTO{
			DeviceID:    deviceID,
			CPUPercent:  metric.CPUPercent,
			MemPercent:  metric.MemPercent,
			TempCelsius: metric.TempCelsius,
			UptimeSecs:  metric.UptimeSecs,
			CollectedAt: formatTimestamp(metric.CollectedAt),
		}
	}

	return dtos
}

// LinkMetricsToDTOs converts domain link metrics keyed by device ID into DTOs.
func LinkMetricsToDTOs(metrics map[string][]domain.LinkMetrics) map[string][]LinkMetricsDTO {
	dtos := make(map[string][]LinkMetricsDTO, len(metrics))
	for key, values := range metrics {
		deviceID := key
		list := make([]LinkMetricsDTO, 0, len(values))

		sort.Slice(values, func(i, j int) bool {
			if values[i].IfName != values[j].IfName {
				return values[i].IfName < values[j].IfName
			}
			return values[i].LinkID < values[j].LinkID
		})

		for _, metric := range values {
			if deviceID == "" && metric.DeviceID != uuid.Nil {
				deviceID = metric.DeviceID.String()
			}

			list = append(list, LinkMetricsDTO{
				DeviceID:    deviceID,
				IfName:      metric.IfName,
				TxBps:       metric.TxBps,
				RxBps:       metric.RxBps,
				Utilization: metric.Utilization,
				CollectedAt: formatTimestamp(metric.CollectedAt),
			})
		}

		if deviceID == "" {
			continue
		}

		dtos[deviceID] = list
	}

	return dtos
}

// AlertsToDTOs converts domain alerts into frontend DTOs.
func AlertsToDTOs(alerts []domain.AlertState) []AlertDTO {
	dtos := make([]AlertDTO, 0, len(alerts))

	sort.Slice(alerts, func(i, j int) bool {
		if alerts[i].DeviceID != alerts[j].DeviceID {
			return alerts[i].DeviceID.String() < alerts[j].DeviceID.String()
		}
		if alerts[i].Severity != alerts[j].Severity {
			return alerts[i].Severity < alerts[j].Severity
		}
		if alerts[i].AlertName != alerts[j].AlertName {
			return alerts[i].AlertName < alerts[j].AlertName
		}
		return alerts[i].Summary < alerts[j].Summary
	})

	for _, alert := range alerts {
		if alert.DeviceID == uuid.Nil {
			continue
		}

		dtos = append(dtos, AlertDTO{
			DeviceID:  alert.DeviceID.String(),
			Severity:  alert.Severity,
			AlertName: alert.AlertName,
			State:     alert.State,
			Summary:   alert.Summary,
		})
	}

	return dtos
}

func formatTimestamp(ts time.Time) string {
	if ts.IsZero() {
		return ""
	}
	return ts.UTC().Format(time.RFC3339)
}
