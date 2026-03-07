package domain

import (
	"time"

	"github.com/google/uuid"
)

// DeviceMetrics contains the latest Prometheus-backed metrics for a device.
type DeviceMetrics struct {
	DeviceID    uuid.UUID `json:"device_id"`
	CPUPercent  *float64  `json:"cpu_percent"`
	MemPercent  *float64  `json:"mem_percent"`
	TempCelsius *float64  `json:"temp_celsius"`
	UptimeSecs  *float64  `json:"uptime_secs"`
	CollectedAt time.Time `json:"collected_at"`
}

// LinkMetrics contains interface throughput and utilization information.
type LinkMetrics struct {
	LinkID      string    `json:"link_id"`
	DeviceID    uuid.UUID `json:"device_id"`
	IfName      string    `json:"if_name"`
	TxBps       *float64  `json:"tx_bps"`
	RxBps       *float64  `json:"rx_bps"`
	Utilization *float64  `json:"utilization"`
	CollectedAt time.Time `json:"collected_at"`
}

// AlertState models a Prometheus alert affecting a device or link.
type AlertState struct {
	DeviceID  uuid.UUID `json:"device_id"`
	Instance  string    `json:"instance,omitempty"`
	Severity  string    `json:"severity"`
	AlertName string    `json:"alert_name"`
	State     string    `json:"state"`
	Summary   string    `json:"summary"`
}

// AlertStatus is the normalized alert severity shown in the UI.
type AlertStatus string

const (
	AlertStatusNormal   AlertStatus = "normal"
	AlertStatusDegraded AlertStatus = "degraded"
	AlertStatusDown     AlertStatus = "down"
)
