package domain

// This file defines settings domain contracts and lifecycle invariants.

// Setting keys for runtime configuration stored in the primary database.
const (
	SettingPrometheusURL                 = "prometheus_url"
	SettingGrafanaURL                    = "grafana_url"
	SettingGrafanaDashboardConfig        = "grafana_dashboard_config"
	SettingGrafanaLegacyDeviceURLPrefix  = "grafana_dashboard_url:"
	SettingPollingInterval               = "polling_interval_seconds"
	SettingSNMPWorkerPoolSize            = "snmp_worker_pool_size"
	SettingSNMPWorkerPoolPerformance     = "snmp_worker_pool_performance_size"
	SettingSNMPWorkerPoolOperational     = "snmp_worker_pool_operational_size"
	SettingSNMPWorkerPoolStatic          = "snmp_worker_pool_static_size"
	SettingSNMPTimeout                   = "snmp_timeout_seconds"
	SettingSNMPRetries                   = "snmp_retries"
	SettingPollingEssentialWorkers       = "polling_essential_workers"
	SettingPollingMaxWorkersPerSite      = "polling_max_workers_per_site"
	SettingPollingMaxWorkersPerSubnet    = "polling_max_workers_per_subnet"
	SettingPollingMaxWorkersPerDevice    = "polling_max_workers_per_device"
	SettingPollingMaxInflightPerProfile  = "polling_max_inflight_per_snmp_profile"
	SettingPollingEssentialTimeoutMillis = "polling_essential_timeout_ms"
	SettingPollingEssentialRetries       = "polling_essential_retries"
	SettingPollingWebSocketCoalesceMS    = "polling_websocket_coalesce_ms"
	SettingPollingPersistenceBatchMS     = "polling_persistence_batch_ms"
	SettingPollingCapacitySafetyMargin   = "polling_capacity_safety_margin"
	SettingPollingForceOverCapacity      = "polling_force_over_capacity"
	SettingTimezone                      = "timezone"
	SettingTopologyDiscoveryDefaultMode  = "topology_discovery_default_mode"
	SettingInstanceBackupIntervalHours   = "instance_backup_interval_hours"
	SettingInstanceBackupRetentionCount  = "instance_backup_retention_count"
	SettingDeviceBackupIntervalHours     = "device_backup_interval_hours"
	SettingDeviceBackupRetentionCount    = "device_backup_retention_count"
	// SettingBridgeSecret is the deprecated legacy global bridge secret key.
	// Runtime bridge authentication uses per-user bridge_credentials instead.
	SettingBridgeSecret = "bridge_secret"
	// SettingBridgePort holds the TCP port the WinBox bridge listens on.
	// Defaults to "1337" to match the bridge's default ListenPort.
	SettingBridgePort = "bridge_port"
)

// DefaultSettings returns the default runtime settings.
func DefaultSettings() map[string]string {
	return map[string]string{
		SettingPrometheusURL:                 "",
		SettingGrafanaURL:                    "",
		SettingGrafanaDashboardConfig:        "{}",
		SettingPollingInterval:               "60",
		SettingSNMPWorkerPoolSize:            "5",
		SettingSNMPWorkerPoolPerformance:     "3",
		SettingSNMPWorkerPoolOperational:     "1",
		SettingSNMPWorkerPoolStatic:          "1",
		SettingSNMPTimeout:                   "10",
		SettingSNMPRetries:                   "2",
		SettingPollingEssentialWorkers:       "64",
		SettingPollingMaxWorkersPerSite:      "16",
		SettingPollingMaxWorkersPerSubnet:    "8",
		SettingPollingMaxWorkersPerDevice:    "1",
		SettingPollingMaxInflightPerProfile:  "16",
		SettingPollingEssentialTimeoutMillis: "1200",
		SettingPollingEssentialRetries:       "1",
		SettingPollingWebSocketCoalesceMS:    "500",
		SettingPollingPersistenceBatchMS:     "1000",
		SettingPollingCapacitySafetyMargin:   "1.5",
		SettingPollingForceOverCapacity:      "false",
		SettingTimezone:                      "UTC",
		SettingTopologyDiscoveryDefaultMode:  string(TopologyDiscoveryModeLLDPCDP),
		SettingInstanceBackupIntervalHours:   "0",
		SettingInstanceBackupRetentionCount:  "5",
		SettingDeviceBackupIntervalHours:     "0",
		SettingDeviceBackupRetentionCount:    "5",
		SettingBridgePort:                    "1337",
	}
}

// SettingsRepository defines persistence operations for runtime settings.
type SettingsRepository interface {
	Get(key string) (string, error)
	Set(key, value string) error
	GetAll() (map[string]string, error)
}
