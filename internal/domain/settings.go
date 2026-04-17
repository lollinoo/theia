package domain

// Setting keys for runtime configuration stored in the primary database.
const (
	SettingPrometheusURL                = "prometheus_url"
	SettingGrafanaURL                   = "grafana_url"
	SettingPollingInterval              = "polling_interval_seconds"
	SettingSNMPWorkerPoolSize           = "snmp_worker_pool_size"
	SettingSNMPWorkerPoolPerformance    = "snmp_worker_pool_performance_size"
	SettingSNMPWorkerPoolOperational    = "snmp_worker_pool_operational_size"
	SettingSNMPWorkerPoolStatic         = "snmp_worker_pool_static_size"
	SettingSNMPTimeout                  = "snmp_timeout_seconds"
	SettingSNMPRetries                  = "snmp_retries"
	SettingTimezone                     = "timezone"
	SettingInstanceBackupIntervalHours  = "instance_backup_interval_hours"
	SettingInstanceBackupRetentionCount = "instance_backup_retention_count"
	SettingDeviceBackupIntervalHours    = "device_backup_interval_hours"
	SettingDeviceBackupRetentionCount   = "device_backup_retention_count"
	// SettingBridgeSecret holds the hex-encoded 32-byte key copied from the bridge's
	// config.json (bridge_secret field).  The backend uses it only to encrypt a
	// per-request credential token — it is never stored in plaintext in the DB beyond
	// whatever the SettingsRepository already encrypts.
	SettingBridgeSecret = "bridge_secret"
	// SettingBridgePort holds the TCP port the WinBox bridge listens on.
	// Defaults to "1337" to match the bridge's default ListenPort.
	SettingBridgePort = "bridge_port"
)

// DefaultSettings returns the default runtime settings.
func DefaultSettings() map[string]string {
	return map[string]string{
		SettingPrometheusURL:                "",
		SettingGrafanaURL:                   "",
		SettingPollingInterval:              "60",
		SettingSNMPWorkerPoolSize:           "5",
		SettingSNMPWorkerPoolPerformance:    "3",
		SettingSNMPWorkerPoolOperational:    "1",
		SettingSNMPWorkerPoolStatic:         "1",
		SettingSNMPTimeout:                  "10",
		SettingSNMPRetries:                  "2",
		SettingTimezone:                     "UTC",
		SettingInstanceBackupIntervalHours:  "0",
		SettingInstanceBackupRetentionCount: "5",
		SettingDeviceBackupIntervalHours:    "0",
		SettingDeviceBackupRetentionCount:   "5",
		SettingBridgePort:                   "1337",
	}
}

// SettingsRepository defines persistence operations for runtime settings.
type SettingsRepository interface {
	Get(key string) (string, error)
	Set(key, value string) error
	GetAll() (map[string]string, error)
}
