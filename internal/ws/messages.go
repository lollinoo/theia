package ws

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/domain"
	"github.com/lollinoo/theia/internal/polling"
)

const (
	// MessageTypeSnapshot pushes the full state to clients.
	MessageTypeSnapshot = "snapshot"
	// MessageTypeSnapshotDelta pushes only changed entries since the last broadcast.
	MessageTypeSnapshotDelta = "snapshot_delta"
	// MessageTypeRuntimeDelta pushes runtime-only changes using the modernized envelope.
	MessageTypeRuntimeDelta = "runtime_delta"
	// MessageTypeTopologyDelta pushes topology-only changes.
	MessageTypeTopologyDelta = "topology_delta"
	// MessageTypePollingHealthChanged notifies clients when polling health changes.
	MessageTypePollingHealthChanged = "polling_health_changed"
	// MessageTypeMetrics carries device metrics-only payloads.
	MessageTypeMetrics = "metrics"
	// MessageTypeLinkMetrics carries link metrics-only payloads.
	MessageTypeLinkMetrics = "link_metrics"
	// MessageTypeAlert carries alert-only payloads.
	MessageTypeAlert = "alert"
	// MessageTypePrometheusStatus notifies clients of Prometheus availability changes.
	MessageTypePrometheusStatus = "prometheus_status"
	// MessageTypeResyncRequired tells overview clients to expect a full snapshot resync.
	MessageTypeResyncRequired = "resync_required"
	// MessageTypeTopologyChanged notifies clients that the topology has changed (new links discovered).
	MessageTypeTopologyChanged = "topology_changed"
	// MessageTypeHello lets clients announce the canvas versions they already have.
	MessageTypeHello = "hello"
	// MessageTypeReady confirms the server skipped an already-current runtime snapshot.
	MessageTypeReady = "ready"
	// MessageTypeSubscribeDetail registers a device-specific detail subscription for one client.
	MessageTypeSubscribeDetail = "subscribe_detail"
	// MessageTypeUnsubscribeDetail clears the active device-specific detail subscription for one client.
	MessageTypeUnsubscribeDetail = "unsubscribe_detail"
)

const (
	CanvasTopologyEndpoint = "/api/v1/topology/canvas"

	ResyncScopeOverview                      = "overview"
	ResyncReasonClientResync                 = "client_resync_scheduled"
	ResyncReasonClientMissingRuntimeSnapshot = "client_missing_runtime_snapshot"
	ResyncReasonStateChangesDrop             = "state_changes_dropped"
	ResyncReasonHubBufferFull                = "hub_buffer_full"
)

// PrometheusStatusPayload is sent when Prometheus availability changes.
type PrometheusStatusPayload struct {
	Enabled   bool   `json:"enabled"`
	Available bool   `json:"available"`
	Error     string `json:"error,omitempty"`
}

// ResyncRequiredPayload tells clients why the overview stream is degraded.
type ResyncRequiredPayload struct {
	Scope  string `json:"scope"`
	Reason string `json:"reason"`
}

// SnapshotMessagePayload is the versioned full overview payload sent to clients.
type SnapshotMessagePayload struct {
	Version         uint64           `json:"version"`
	RuntimeIdentity string           `json:"runtime_identity,omitempty"`
	Snapshot        *SnapshotPayload `json:"snapshot"`
}

// SnapshotDeltaMessagePayload is the versioned sparse overview delta payload.
type SnapshotDeltaMessagePayload struct {
	BaseVersion uint64           `json:"base_version"`
	Version     uint64           `json:"version"`
	Delta       *SnapshotPayload `json:"delta"`
}

type RuntimeDeltaMessagePayload struct {
	BaseVersion     uint64           `json:"base_version"`
	Version         uint64           `json:"version"`
	RuntimeIdentity string           `json:"runtime_identity,omitempty"`
	Delta           *SnapshotPayload `json:"delta"`
}

type TopologyChangedPayload struct {
	TopologyVersion     uint64 `json:"topology_version"`
	Reason              string `json:"reason,omitempty"`
	RecommendedEndpoint string `json:"recommended_endpoint,omitempty"`
}

type ReadyPayload struct {
	RuntimeVersion  uint64 `json:"runtime_version"`
	RuntimeIdentity string `json:"runtime_identity,omitempty"`
	AlertVersion    uint64 `json:"alert_version"`
}

type PollingHealthChangedPayload = polling.HealthSnapshot

// AlertMessagePayload is the versioned alert-only payload sent to clients.
type AlertMessagePayload struct {
	Version uint64     `json:"version"`
	Alerts  []AlertDTO `json:"alerts"`
}

// Message is the WebSocket envelope used for all server pushes.
type Message struct {
	Type    string `json:"type"`
	Payload any    `json:"payload"`
}

func NewSnapshotMessage(snapshot *SnapshotPayload, version uint64) Message {
	return Message{
		Type: MessageTypeSnapshot,
		Payload: SnapshotMessagePayload{
			Version:         version,
			RuntimeIdentity: RuntimeIdentityForSnapshot(snapshot),
			Snapshot:        CloneSnapshot(snapshot),
		},
	}
}

func NewSnapshotDeltaMessage(delta *SnapshotPayload, baseVersion, version uint64) Message {
	return Message{
		Type: MessageTypeSnapshotDelta,
		Payload: SnapshotDeltaMessagePayload{
			BaseVersion: baseVersion,
			Version:     version,
			Delta:       CloneSnapshot(delta),
		},
	}
}

func NewRuntimeDeltaMessage(delta *SnapshotPayload, baseVersion, version uint64, currentSnapshot ...*SnapshotPayload) Message {
	runtimeIdentity := ""
	if len(currentSnapshot) > 0 {
		runtimeIdentity = RuntimeIdentityForSnapshot(currentSnapshot[0])
	}
	return Message{
		Type: MessageTypeRuntimeDelta,
		Payload: RuntimeDeltaMessagePayload{
			BaseVersion:     baseVersion,
			Version:         version,
			RuntimeIdentity: runtimeIdentity,
			Delta:           CloneSnapshot(delta),
		},
	}
}

func NewTopologyChangedMessage(topologyVersion uint64, reason string) Message {
	return Message{
		Type: MessageTypeTopologyChanged,
		Payload: TopologyChangedPayload{
			TopologyVersion:     topologyVersion,
			Reason:              reason,
			RecommendedEndpoint: CanvasTopologyEndpoint,
		},
	}
}

func NewReadyMessage(runtimeVersion uint64, alertVersion uint64, runtimeIdentity string) Message {
	return Message{
		Type: MessageTypeReady,
		Payload: ReadyPayload{
			RuntimeVersion:  runtimeVersion,
			RuntimeIdentity: runtimeIdentity,
			AlertVersion:    alertVersion,
		},
	}
}

func RuntimeIdentityForSnapshot(snapshot *SnapshotPayload) string {
	raw, err := json.Marshal(CloneSnapshot(snapshot))
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(raw)
	return fmt.Sprintf("rt-sha256:%x", sum[:])
}

func NewPollingHealthChangedMessage(snapshot polling.HealthSnapshot) Message {
	return Message{
		Type:    MessageTypePollingHealthChanged,
		Payload: PollingHealthChangedPayload(snapshot),
	}
}

func NewAlertMessage(alerts []AlertDTO, version uint64) Message {
	return Message{
		Type: MessageTypeAlert,
		Payload: AlertMessagePayload{
			Version: version,
			Alerts:  append([]AlertDTO{}, alerts...),
		},
	}
}

type clientControlMessage struct {
	Type                string
	DeviceID            uuid.UUID
	CanvasSchemaVersion int
	TopologyVersion     string
	RuntimeIdentity     string
	RuntimeVersion      *uint64
	AlertVersion        *uint64
}

type clientControlEnvelope struct {
	Type    string               `json:"type"`
	Payload clientControlPayload `json:"payload"`
}

type clientControlPayload struct {
	DeviceID            string  `json:"device_id"`
	CanvasSchemaVersion int     `json:"canvas_schema_version"`
	TopologyVersion     string  `json:"topology_version"`
	RuntimeIdentity     string  `json:"runtime_identity"`
	RuntimeVersion      *uint64 `json:"runtime_version"`
	AlertVersion        *uint64 `json:"alert_version"`
}

// SnapshotPayload contains the complete live state sent to clients.
type SnapshotPayload struct {
	Devices        map[string]DeviceRuntimeDTO `json:"devices"`
	Links          map[string]LinkRuntimeDTO   `json:"links"`
	DeviceMetrics  map[string]DeviceRuntimeDTO `json:"-"`
	LinkMetrics    map[string][]LinkRuntimeDTO `json:"-"`
	DeviceStatuses map[string]string           `json:"-"`
}

type DeviceMetricsDTO = DeviceRuntimeDTO
type LinkMetricsDTO = LinkRuntimeDTO

type DeviceRuntimeDTO struct {
	DeviceID                    string            `json:"device_id"`
	OperationalStatus           string            `json:"operational_status"`
	PrimaryHealth               string            `json:"primary_health"`
	RuntimeFlags                []string          `json:"runtime_flags"`
	FieldStates                 map[string]string `json:"field_states"`
	NetworkReachable            string            `json:"network_reachable"`
	SNMPReachable               string            `json:"snmp_reachable"`
	Reachability                string            `json:"reachability"`
	Health                      string            `json:"health"`
	Freshness                   string            `json:"freshness"`
	PrimaryReason               string            `json:"primary_reason"`
	MetricsStatus               string            `json:"metrics_status"`
	MetricsReason               string            `json:"metrics_reason"`
	AlertStatus                 string            `json:"alert_status"`
	FiringAlertCount            int               `json:"firing_alert_count"`
	LastCollectedAt             *string           `json:"last_collected_at"`
	LastPolledAt                *string           `json:"last_polled_at"`
	ExpectedPollIntervalSeconds *float64          `json:"expected_poll_interval_seconds"`
	CPUPercent                  *float64          `json:"cpu_percent"`
	MemPercent                  *float64          `json:"mem_percent"`
	TempCelsius                 *float64          `json:"temp_celsius"`
	UptimeSecs                  *float64          `json:"uptime_secs"`
	CollectedAt                 string            `json:"-"`
	Stale                       *bool             `json:"-"`
}

type LinkRuntimeDTO struct {
	LinkID          string   `json:"link_id"`
	SourceDeviceID  string   `json:"source_device_id"`
	TargetDeviceID  string   `json:"target_device_id"`
	SourceIfName    string   `json:"source_if_name"`
	TargetIfName    string   `json:"target_if_name"`
	MetricsStatus   string   `json:"metrics_status"`
	MetricsReason   string   `json:"metrics_reason"`
	LastCollectedAt *string  `json:"last_collected_at"`
	TxBps           *float64 `json:"tx_bps"`
	RxBps           *float64 `json:"rx_bps"`
	Utilization     *float64 `json:"utilization"`
	DeviceID        string   `json:"-"`
	IfName          string   `json:"-"`
	CollectedAt     string   `json:"-"`
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
		Devices:        map[string]DeviceRuntimeDTO{},
		Links:          map[string]LinkRuntimeDTO{},
		DeviceMetrics:  map[string]DeviceRuntimeDTO{},
		LinkMetrics:    map[string][]LinkRuntimeDTO{},
		DeviceStatuses: map[string]string{},
	}
}

// CloneSnapshot makes a deep copy so callers can safely share snapshots.
func CloneSnapshot(snapshot *SnapshotPayload) *SnapshotPayload {
	if snapshot == nil {
		return EmptySnapshot()
	}

	cloned := &SnapshotPayload{
		Devices:        make(map[string]DeviceRuntimeDTO, len(snapshot.Devices)),
		Links:          make(map[string]LinkRuntimeDTO, len(snapshot.Links)),
		DeviceMetrics:  make(map[string]DeviceRuntimeDTO, len(snapshot.DeviceMetrics)),
		LinkMetrics:    make(map[string][]LinkRuntimeDTO, len(snapshot.LinkMetrics)),
		DeviceStatuses: make(map[string]string, len(snapshot.DeviceStatuses)),
	}

	for key, value := range snapshot.Devices {
		cloned.Devices[key] = cloneDeviceRuntimeDTO(value)
	}

	for key, value := range snapshot.Links {
		cloned.Links[key] = value
	}
	for key, value := range snapshot.DeviceMetrics {
		cloned.DeviceMetrics[key] = cloneDeviceRuntimeDTO(value)
	}
	for key, value := range snapshot.LinkMetrics {
		cloned.LinkMetrics[key] = append([]LinkRuntimeDTO(nil), value...)
	}
	for key, value := range snapshot.DeviceStatuses {
		cloned.DeviceStatuses[key] = value
	}

	return cloned
}

func cloneDeviceRuntimeDTO(value DeviceRuntimeDTO) DeviceRuntimeDTO {
	value.RuntimeFlags = append(make([]string, 0, len(value.RuntimeFlags)), value.RuntimeFlags...)
	if value.FieldStates != nil {
		cloned := make(map[string]string, len(value.FieldStates))
		for key, state := range value.FieldStates {
			cloned[key] = state
		}
		value.FieldStates = cloned
	}
	return value
}

func parseClientControlMessage(raw []byte) (clientControlMessage, error) {
	var envelope clientControlEnvelope
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return clientControlMessage{}, fmt.Errorf("unmarshal client control message: %w", err)
	}

	switch envelope.Type {
	case MessageTypeHello:
		return clientControlMessage{
			Type:                envelope.Type,
			CanvasSchemaVersion: envelope.Payload.CanvasSchemaVersion,
			TopologyVersion:     envelope.Payload.TopologyVersion,
			RuntimeIdentity:     envelope.Payload.RuntimeIdentity,
			RuntimeVersion:      envelope.Payload.RuntimeVersion,
			AlertVersion:        envelope.Payload.AlertVersion,
		}, nil
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

// DeviceMetricsToDTOs preserves the legacy internal helper shape for older worker code.
func DeviceMetricsToDTOs(metrics map[string]domain.DeviceMetrics) map[string]DeviceRuntimeDTO {
	dtos := make(map[string]DeviceRuntimeDTO, len(metrics))
	for key, metric := range metrics {
		deviceID := key
		if deviceID == "" && metric.DeviceID != uuid.Nil {
			deviceID = metric.DeviceID.String()
		}
		if deviceID == "" {
			continue
		}
		collectedAt := formatTimestamp(metric.CollectedAt)
		stale := false
		dto := DeviceRuntimeDTO{
			DeviceID:        deviceID,
			CPUPercent:      metric.CPUPercent,
			MemPercent:      metric.MemPercent,
			TempCelsius:     metric.TempCelsius,
			UptimeSecs:      metric.UptimeSecs,
			LastCollectedAt: nil,
			CollectedAt:     collectedAt,
			Stale:           &stale,
		}
		if collectedAt != "" {
			dto.LastCollectedAt = &collectedAt
		}
		dtos[deviceID] = dto
	}
	return dtos
}

// LinkMetricsToDTOs preserves the legacy internal helper shape for older worker code.
func LinkMetricsToDTOs(metrics map[string][]domain.LinkMetrics) map[string][]LinkRuntimeDTO {
	dtos := make(map[string][]LinkRuntimeDTO, len(metrics))
	for key, values := range metrics {
		deviceID := key
		list := make([]LinkRuntimeDTO, 0, len(values))
		for _, metric := range values {
			if deviceID == "" && metric.DeviceID != uuid.Nil {
				deviceID = metric.DeviceID.String()
			}
			collectedAt := formatTimestamp(metric.CollectedAt)
			dto := LinkRuntimeDTO{
				LinkID:         metric.LinkID,
				SourceDeviceID: deviceID,
				SourceIfName:   metric.IfName,
				MetricsStatus:  "available",
				MetricsReason:  "ok",
				TxBps:          metric.TxBps,
				RxBps:          metric.RxBps,
				Utilization:    metric.Utilization,
				DeviceID:       deviceID,
				IfName:         metric.IfName,
				CollectedAt:    collectedAt,
			}
			if collectedAt != "" {
				dto.LastCollectedAt = &collectedAt
			}
			list = append(list, dto)
		}
		if deviceID == "" {
			continue
		}
		dtos[deviceID] = list
	}
	return dtos
}

func formatTimestamp(ts time.Time) string {
	if ts.IsZero() {
		return ""
	}
	return ts.UTC().Format(time.RFC3339)
}
