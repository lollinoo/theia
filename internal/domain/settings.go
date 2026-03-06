package domain

// Setting keys for runtime configuration stored in SQLite.
const (
	SettingPrometheusURL      = "prometheus_url"
	SettingGrafanaURL         = "grafana_url"
	SettingPollingInterval    = "polling_interval_seconds"
	SettingSNMPWorkerPoolSize = "snmp_worker_pool_size"
	SettingSNMPTimeout        = "snmp_timeout_seconds"
	SettingSNMPRetries        = "snmp_retries"
)

// DefaultSettings returns the default runtime settings.
func DefaultSettings() map[string]string {
	return map[string]string{
		SettingPrometheusURL:      "",
		SettingGrafanaURL:         "",
		SettingPollingInterval:    "60",
		SettingSNMPWorkerPoolSize: "5",
		SettingSNMPTimeout:        "5",
		SettingSNMPRetries:        "2",
	}
}

// SettingsRepository defines persistence operations for runtime settings.
type SettingsRepository interface {
	Get(key string) (string, error)
	Set(key, value string) error
	GetAll() (map[string]string, error)
}
