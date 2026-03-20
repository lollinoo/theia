package vendor

// VendorConfig represents a vendor definition loaded from YAML.
type VendorConfig struct {
	Vendor          VendorInfo       `yaml:"vendor" json:"vendor"`
	Detection       Detection        `yaml:"detection" json:"detection"`
	DeviceTypeRules []DeviceTypeRule `yaml:"device_type_rules" json:"device_type_rules"`
	ModelExtraction ModelExtraction  `yaml:"model_extraction" json:"model_extraction"`
	Metrics         MetricsConfig    `yaml:"metrics" json:"metrics"`
	SNMP            SNMPConfig       `yaml:"snmp" json:"snmp"`
	Backup          BackupConfig     `yaml:"backup" json:"backup"`
}

// VendorInfo holds vendor identity.
type VendorInfo struct {
	Name        string `yaml:"name" json:"name"`
	DisplayName string `yaml:"display_name" json:"display_name"`
}

// Detection defines how to match a device to this vendor.
type Detection struct {
	SysObjectIDPrefixes []string `yaml:"sys_object_id_prefixes" json:"sys_object_id_prefixes"`
	SysDescrPatterns    []string `yaml:"sys_descr_patterns" json:"sys_descr_patterns"`
}

// DeviceTypeRule maps a sysDescr match to a device type.
type DeviceTypeRule struct {
	Match *DeviceTypeMatch `yaml:"match,omitempty" json:"match,omitempty"`
	Type  string           `yaml:"type" json:"type"`
}

// DeviceTypeMatch defines the condition for a device type rule.
type DeviceTypeMatch struct {
	SysDescrContains string `yaml:"sys_descr_contains" json:"sys_descr_contains"`
}

// ModelExtraction defines how to extract a hardware model from sysDescr.
type ModelExtraction struct {
	SysDescrRegex string `yaml:"sys_descr_regex" json:"sys_descr_regex"`
	CaptureGroup  int    `yaml:"capture_group" json:"capture_group"`
}

// MetricsConfig holds both Prometheus and SNMP metric definitions.
type MetricsConfig struct {
	Prometheus PrometheusMetrics `yaml:"prometheus" json:"prometheus"`
}

// PrometheusMetrics holds PromQL query templates per metric type.
// The placeholder %label% is replaced at query time with
// labelName=~"labelValue" for the target device.
type PrometheusMetrics struct {
	CPU         string `yaml:"cpu" json:"cpu"`
	Memory      string `yaml:"memory" json:"memory"`
	Temperature string `yaml:"temperature" json:"temperature"`
	Uptime      string `yaml:"uptime" json:"uptime"`
}

// SNMPConfig holds vendor-specific SNMP OIDs and scale factors.
type SNMPConfig struct {
	TemperatureOID   string  `yaml:"temperature_oid" json:"temperature_oid"`
	TemperatureScale float64 `yaml:"temperature_scale" json:"temperature_scale"`
	CPUOID           string  `yaml:"cpu_oid" json:"cpu_oid"`
	MemoryUsedOID    string  `yaml:"memory_used_oid" json:"memory_used_oid"`
	MemoryTotalOID   string  `yaml:"memory_total_oid" json:"memory_total_oid"`
}

// BackupConfig describes how to back up a device's configuration.
type BackupConfig struct {
	Supported     bool          `yaml:"supported" json:"supported"`
	Methods       []string      `yaml:"methods" json:"methods"`
	DefaultMethod string        `yaml:"default_method" json:"default_method"`
	SSHCommands   SSHBackupCmds `yaml:"ssh_commands" json:"ssh_commands"`
}

// SSHBackupCmds holds the SSH commands to export device configuration.
type SSHBackupCmds struct {
	ExportRunning string           `yaml:"export_running" json:"export_running"`
	ExportCompact string           `yaml:"export_compact" json:"export_compact"`
	ExportVerbose string           `yaml:"export_verbose" json:"export_verbose"`
	ExportStartup string           `yaml:"export_startup" json:"export_startup"`
	BinaryBackup  *BinaryBackupCmd `yaml:"binary_backup,omitempty" json:"binary_backup,omitempty"`
}

// BinaryBackupCmd describes how to create and retrieve a binary backup file via SFTP.
type BinaryBackupCmd struct {
	SaveCommand    string `yaml:"save_command" json:"save_command"`
	RemoteFilePath string `yaml:"remote_file_path" json:"remote_file_path"`
	CleanupCommand string `yaml:"cleanup_command" json:"cleanup_command"`
}
