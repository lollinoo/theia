package worker

import (
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/lollinoo/theia/internal/domain"
	"github.com/lollinoo/theia/internal/state"
	"github.com/lollinoo/theia/internal/ws"
)

const (
	normalizedReasonOK                  = "ok"
	normalizedReasonAwaitingPoll        = "awaiting_poll"
	normalizedReasonStale               = "stale"
	normalizedReasonDeviceUnreachable   = "device_unreachable"
	normalizedReasonUpstreamUnavailable = "upstream_unavailable"
	normalizedReasonNoData              = "no_data"
	normalizedReasonUnmonitored         = "unmonitored"
	normalizedReasonUnsupported         = "unsupported"
)

func normalizeDeviceRuntimeDTO(device domain.Device, deviceState state.DeviceState, alerts []domain.AlertState, promStatus ws.PrometheusStatusPayload) ws.DeviceRuntimeDTO {
	deviceID := device.ID.String()
	if domain.IsVirtualNoIPDevice(device) {
		return ws.DeviceRuntimeDTO{
			DeviceID:       deviceID,
			OperationalStatus: "unmonitored",
			Reachability:   "unmonitored",
			Health:         string(state.HealthStatusUnknown),
			Freshness:      "unmonitored",
			PrimaryReason:  normalizedReasonUnmonitored,
			MetricsStatus:  "unmonitored",
			MetricsReason:  normalizedReasonUnmonitored,
			AlertStatus:    string(domain.AlertStatusNormal),
		}
	}

	health := string(deviceState.Health)
	if health == "" {
		health = string(state.HealthStatusUnknown)
	}

	freshness := normalizeDeviceFreshness(deviceState)
	reachability := normalizeDeviceReachability(deviceState)
	primaryReason := normalizeDevicePrimaryReason(device, deviceState, promStatus, freshness)
	operationalStatus := normalizeOperationalStatus(device, deviceState, freshness, primaryReason)
	metricsStatus, metricsReason := normalizeDeviceMetricsStatus(device, deviceState, promStatus, freshness)
	alertStatus, firingAlertCount := summarizeAlerts(alerts)

	return ws.DeviceRuntimeDTO{
		DeviceID:                    deviceID,
		OperationalStatus:           operationalStatus,
		Reachability:                reachability,
		Health:                      health,
		Freshness:                   freshness,
		PrimaryReason:               primaryReason,
		MetricsStatus:               metricsStatus,
		MetricsReason:               metricsReason,
		AlertStatus:                 string(alertStatus),
		FiringAlertCount:            firingAlertCount,
		LastCollectedAt:             timestampPtr(deviceState.Metrics.CollectedAt),
		LastPolledAt:                timestampPtr(deviceState.LastPolledAt),
		ExpectedPollIntervalSeconds: durationSecondsPtr(deviceState.ExpectedInterval),
		CPUPercent:                  deviceState.Metrics.CPUPercent,
		MemPercent:                  deviceState.Metrics.MemPercent,
		TempCelsius:                 deviceState.Metrics.TempCelsius,
		UptimeSecs:                  deviceState.Metrics.UptimeSecs,
		CollectedAt:                 stringValue(timestampPtr(deviceState.Metrics.CollectedAt)),
		Stale:                       staleCompatibilityPtr(freshness),
	}
}

func normalizeLinkRuntimeDTO(link domain.Link, metric *domain.LinkMetrics, sourceRuntime ws.DeviceRuntimeDTO, targetRuntime ws.DeviceRuntimeDTO) ws.LinkRuntimeDTO {
	metricsStatus, metricsReason := normalizeLinkMetricsStatus(metric, sourceRuntime, targetRuntime)

	var lastCollectedAt *string
	var txBps, rxBps, utilization *float64
	deviceID := link.SourceDeviceID.String()
	ifName := link.SourceIfName
	if metric != nil {
		lastCollectedAt = timestampPtr(metric.CollectedAt)
		txBps = metric.TxBps
		rxBps = metric.RxBps
		utilization = metric.Utilization
		if metric.DeviceID != uuid.Nil {
			deviceID = metric.DeviceID.String()
		}
		if metric.IfName != "" {
			ifName = metric.IfName
		}
	}

	return ws.LinkRuntimeDTO{
		LinkID:          link.ID.String(),
		SourceDeviceID:  link.SourceDeviceID.String(),
		TargetDeviceID:  link.TargetDeviceID.String(),
		SourceIfName:    link.SourceIfName,
		TargetIfName:    link.TargetIfName,
		MetricsStatus:   metricsStatus,
		MetricsReason:   metricsReason,
		LastCollectedAt: lastCollectedAt,
		TxBps:           txBps,
		RxBps:           rxBps,
		Utilization:     utilization,
		DeviceID:        deviceID,
		IfName:          ifName,
		CollectedAt:     stringValue(lastCollectedAt),
	}
}

func normalizeDeviceFreshness(deviceState state.DeviceState) string {
	if deviceState.LastPolledAt.IsZero() && deviceState.Metrics.CollectedAt.IsZero() {
		return "awaiting_poll"
	}
	if deviceState.Stale {
		return "stale"
	}
	return "fresh"
}

func normalizeDeviceReachability(deviceState state.DeviceState) string {
	switch deviceState.Reachability {
	case state.ReachabilityUp:
		return "up"
	case state.ReachabilitySoftDown:
		return "soft_down"
	case state.ReachabilityHardDown:
		return "hard_down"
	default:
		return "unknown"
	}
}

func normalizeDevicePrimaryReason(device domain.Device, deviceState state.DeviceState, promStatus ws.PrometheusStatusPayload, freshness string) string {
	if isUnsupportedDevice(device) {
		return normalizedReasonUnsupported
	}
	if isPrometheusBlocked(device, deviceState, promStatus) {
		return normalizedReasonUpstreamUnavailable
	}
	if deviceState.Reachability == state.ReachabilitySoftDown || deviceState.Reachability == state.ReachabilityHardDown {
		return normalizedReasonDeviceUnreachable
	}
	if freshness == "awaiting_poll" {
		return normalizedReasonAwaitingPoll
	}
	if freshness == "stale" {
		return normalizedReasonStale
	}
	return normalizedReasonOK
}

func normalizeOperationalStatus(device domain.Device, deviceState state.DeviceState, freshness string, primaryReason string) string {
	if primaryReason == normalizedReasonUnsupported || primaryReason == normalizedReasonUpstreamUnavailable {
		return "unknown"
	}
	switch deviceState.Reachability {
	case state.ReachabilityUp:
		return "up"
	case state.ReachabilitySoftDown, state.ReachabilityHardDown:
		return "down"
	}
	if freshness == "awaiting_poll" {
		return "probing"
	}
	return "unknown"
}

func normalizeDeviceMetricsStatus(device domain.Device, deviceState state.DeviceState, promStatus ws.PrometheusStatusPayload, freshness string) (string, string) {
	if isUnsupportedDevice(device) {
		return "unavailable", normalizedReasonUnsupported
	}
	if isPrometheusBlocked(device, deviceState, promStatus) {
		return "unavailable", normalizedReasonUpstreamUnavailable
	}
	if deviceState.Reachability == state.ReachabilitySoftDown || deviceState.Reachability == state.ReachabilityHardDown {
		return "unavailable", normalizedReasonDeviceUnreachable
	}
	if freshness == "stale" {
		return "unavailable", normalizedReasonStale
	}
	metricCount := deviceMetricCount(deviceState.Metrics)
	if metricCount == 0 {
		if freshness == "awaiting_poll" {
			return "unavailable", normalizedReasonAwaitingPoll
		}
		return "unavailable", normalizedReasonNoData
	}
	if metricCount < 4 {
		return "partial", normalizedReasonOK
	}
	return "available", normalizedReasonOK
}

func normalizeLinkMetricsStatus(metric *domain.LinkMetrics, sourceRuntime ws.DeviceRuntimeDTO, targetRuntime ws.DeviceRuntimeDTO) (string, string) {
	if metric != nil {
		count := 0
		if metric.TxBps != nil {
			count++
		}
		if metric.RxBps != nil {
			count++
		}
		if metric.Utilization != nil {
			count++
		}
		if count == 3 {
			return "available", normalizedReasonOK
		}
		if count > 0 {
			return "partial", normalizedReasonOK
		}
	}

	for _, reason := range []string{sourceRuntime.PrimaryReason, targetRuntime.PrimaryReason, sourceRuntime.MetricsReason, targetRuntime.MetricsReason} {
		switch reason {
		case normalizedReasonUpstreamUnavailable:
			return "unavailable", normalizedReasonUpstreamUnavailable
		case normalizedReasonDeviceUnreachable:
			return "unavailable", normalizedReasonDeviceUnreachable
		case normalizedReasonStale:
			return "unavailable", normalizedReasonStale
		case normalizedReasonAwaitingPoll:
			return "unavailable", normalizedReasonAwaitingPoll
		case normalizedReasonUnmonitored:
			return "unavailable", normalizedReasonUnmonitored
		case normalizedReasonUnsupported:
			return "unavailable", normalizedReasonUnsupported
		}
	}

	return "unavailable", normalizedReasonNoData
}

func summarizeAlerts(alerts []domain.AlertState) (domain.AlertStatus, int) {
	firingCount := 0
	status := domain.AlertStatusNormal
	for _, alert := range alerts {
		if !strings.EqualFold(alert.State, "firing") {
			continue
		}
		firingCount++
		if strings.EqualFold(alert.Severity, "critical") {
			status = domain.AlertStatusDown
			continue
		}
		if status != domain.AlertStatusDown {
			status = domain.AlertStatusDegraded
		}
	}
	return status, firingCount
}

func isUnsupportedDevice(device domain.Device) bool {
	return device.MetricsSource == domain.MetricsSourceNone && !domain.IsVirtualNoIPDevice(device)
}

func isPrometheusBlocked(device domain.Device, deviceState state.DeviceState, promStatus ws.PrometheusStatusPayload) bool {
	if !deviceDependsOnPrometheus(device) || !promStatus.Enabled || promStatus.Available {
		return false
	}
	return deviceMetricCount(deviceState.Metrics) == 0
}

func deviceDependsOnPrometheus(device domain.Device) bool {
	src := device.MetricsSource
	if src == "" {
		src = domain.MetricsSourcePrometheus
	}
	return src == domain.MetricsSourcePrometheus || src == domain.MetricsSourcePrometheusSNMPFallback
}

func deviceMetricCount(metrics domain.DeviceMetrics) int {
	count := 0
	if metrics.CPUPercent != nil {
		count++
	}
	if metrics.MemPercent != nil {
		count++
	}
	if metrics.TempCelsius != nil {
		count++
	}
	if metrics.UptimeSecs != nil {
		count++
	}
	return count
}

func timestampPtr(ts time.Time) *string {
	if ts.IsZero() {
		return nil
	}
	formatted := ts.UTC().Format(time.RFC3339)
	return &formatted
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func staleCompatibilityPtr(freshness string) *bool {
	stale := freshness == "stale"
	return &stale
}
