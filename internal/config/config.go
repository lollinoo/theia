package config

import (
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

const maxInstanceBackupDurationSeconds = int64(math.MaxInt64 / int64(time.Second))

// Config holds bootstrap configuration loaded from YAML / environment.
// Runtime settings (Prometheus URL, polling interval, etc.) are stored
// in the primary database settings table and managed via the API.
type Config struct {
	ListenAddr                  string                      `yaml:"listen_addr"`
	DBDSN                       string                      `yaml:"db_dsn"`
	DataDir                     string                      `yaml:"data_dir"`
	LogLevel                    string                      `yaml:"log_level"`
	BridgeBinariesDir           string                      `yaml:"bridge_binaries_dir"`
	DeploymentEnv               string                      `yaml:"deployment_env"`
	SessionSecret               string                      `yaml:"session_secret"`
	SessionTTLMinutes           int                         `yaml:"session_ttl_minutes"`
	PasswordResetTTLMinutes     int                         `yaml:"password_reset_ttl_minutes"`
	MetricsToken                string                      `yaml:"metrics_token"`
	AllowedOrigins              []string                    `yaml:"allowed_origins"`
	RestoreArchiveLimits        RestoreArchiveLimits        `yaml:"restore_archive_limits"`
	InstanceBackupArchiveLimits InstanceBackupArchiveLimits `yaml:"instance_backup_archive_limits"`
	BulkBackupLimits            BulkBackupLimits            `yaml:"bulk_backup_limits"`
	BulkDownloadLimits          BulkDownloadLimits          `yaml:"bulk_download_limits"`
}

// RestoreArchiveLimits holds defensive quotas for uploaded restore archives.
type RestoreArchiveLimits struct {
	MaxCompressedBytes int64 `yaml:"max_compressed_bytes"`
	MaxTotalBytes      int64 `yaml:"max_total_bytes"`
	MaxEntryBytes      int64 `yaml:"max_entry_bytes"`
	MaxFileEntries     int   `yaml:"max_file_entries"`
}

// InstanceBackupArchiveLimits holds defensive quotas for instance backup archive creation.
type InstanceBackupArchiveLimits struct {
	MaxTotalBytes      int64 `yaml:"max_total_bytes"`
	MaxEntryBytes      int64 `yaml:"max_entry_bytes"`
	MaxFileEntries     int   `yaml:"max_file_entries"`
	MaxDurationSeconds int   `yaml:"max_duration_seconds"`
}

// BulkBackupLimits holds defensive quotas for one bulk device-backup request.
type BulkBackupLimits struct {
	MaxDevices    int `yaml:"max_devices"`
	MaxQueuedJobs int `yaml:"max_queued_jobs"`
}

// BulkDownloadLimits holds defensive quotas for one bulk backup download request.
type BulkDownloadLimits struct {
	MaxDevices            int   `yaml:"max_devices"`
	MaxFiles              int   `yaml:"max_files"`
	MaxBytes              int64 `yaml:"max_bytes"`
	MaxConcurrentPerActor int   `yaml:"max_concurrent_per_actor"`
	MaxConcurrentGlobal   int   `yaml:"max_concurrent_global"`
}

// defaults returns a Config with sensible default values.
func defaults() *Config {
	return &Config{
		ListenAddr: ":8080",
		DataDir:    "./data",
		LogLevel:   "info",
		RestoreArchiveLimits: RestoreArchiveLimits{
			MaxCompressedBytes: 256 << 20,
			MaxTotalBytes:      1 << 30,
			MaxEntryBytes:      512 << 20,
			MaxFileEntries:     25000,
		},
		InstanceBackupArchiveLimits: InstanceBackupArchiveLimits{
			MaxTotalBytes:      2 << 30,
			MaxEntryBytes:      1 << 30,
			MaxFileEntries:     50000,
			MaxDurationSeconds: 30 * 60,
		},
		BulkBackupLimits: BulkBackupLimits{
			MaxDevices:    100,
			MaxQueuedJobs: 100,
		},
		BulkDownloadLimits: BulkDownloadLimits{
			MaxDevices:            100,
			MaxFiles:              500,
			MaxBytes:              512 << 20,
			MaxConcurrentPerActor: 1,
			MaxConcurrentGlobal:   4,
		},
	}
}

// Load reads configuration from a YAML file and then applies environment
// variable overrides. Environment variables take precedence over the file.
//
// Supported env vars:
//   - THEIA_LISTEN_ADDR
//   - THEIA_DB_DSN
//   - THEIA_DATA_DIR
//   - THEIA_LOG_LEVEL
//   - THEIA_BRIDGE_BINARIES_DIR
//   - THEIA_DEPLOYMENT_ENV
//   - THEIA_SESSION_SECRET
//   - THEIA_SESSION_TTL_MINUTES
//   - THEIA_PASSWORD_RESET_TTL_MINUTES
//   - THEIA_METRICS_TOKEN
//   - THEIA_ALLOWED_ORIGINS
//   - THEIA_RESTORE_MAX_COMPRESSED_BYTES
//   - THEIA_RESTORE_MAX_TOTAL_BYTES
//   - THEIA_RESTORE_MAX_ENTRY_BYTES
//   - THEIA_RESTORE_MAX_FILE_ENTRIES
//   - THEIA_INSTANCE_BACKUP_MAX_TOTAL_BYTES
//   - THEIA_INSTANCE_BACKUP_MAX_ENTRY_BYTES
//   - THEIA_INSTANCE_BACKUP_MAX_FILE_ENTRIES
//   - THEIA_INSTANCE_BACKUP_MAX_DURATION_SECONDS
//   - THEIA_BULK_BACKUP_MAX_DEVICES
//   - THEIA_BULK_BACKUP_MAX_QUEUED_JOBS
//   - THEIA_BULK_DOWNLOAD_MAX_DEVICES
//   - THEIA_BULK_DOWNLOAD_MAX_FILES
//   - THEIA_BULK_DOWNLOAD_MAX_BYTES
//   - THEIA_BULK_DOWNLOAD_MAX_CONCURRENT_PER_ACTOR
//   - THEIA_BULK_DOWNLOAD_MAX_CONCURRENT_GLOBAL
func Load(path string) (*Config, error) {
	cfg := defaults()

	data, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			return nil, fmt.Errorf("reading config file: %w", err)
		}
		// Config file not found — proceed with defaults + env overrides
	} else {
		if err := yaml.Unmarshal(data, cfg); err != nil {
			return nil, fmt.Errorf("parsing config file: %w", err)
		}
	}

	// Environment variable overrides
	if v := os.Getenv("THEIA_LISTEN_ADDR"); v != "" {
		cfg.ListenAddr = v
	}
	if v := os.Getenv("THEIA_DB_DSN"); v != "" {
		cfg.DBDSN = v
	}
	if v := os.Getenv("THEIA_DATA_DIR"); v != "" {
		cfg.DataDir = v
	}
	if v := os.Getenv("THEIA_LOG_LEVEL"); v != "" {
		cfg.LogLevel = v
	}
	if v := os.Getenv("THEIA_BRIDGE_BINARIES_DIR"); v != "" {
		cfg.BridgeBinariesDir = v
	}
	if v := os.Getenv("THEIA_DEPLOYMENT_ENV"); v != "" {
		cfg.DeploymentEnv = v
	}
	if v := os.Getenv("THEIA_SESSION_SECRET"); v != "" {
		cfg.SessionSecret = v
	}
	if v := os.Getenv("THEIA_SESSION_TTL_MINUTES"); v != "" {
		minutes, err := parseEnvMinutes("THEIA_SESSION_TTL_MINUTES", v)
		if err != nil {
			return nil, err
		}
		cfg.SessionTTLMinutes = minutes
	}
	if v := os.Getenv("THEIA_PASSWORD_RESET_TTL_MINUTES"); v != "" {
		minutes, err := parseEnvMinutes("THEIA_PASSWORD_RESET_TTL_MINUTES", v)
		if err != nil {
			return nil, err
		}
		cfg.PasswordResetTTLMinutes = minutes
	}
	if v := os.Getenv("THEIA_METRICS_TOKEN"); v != "" {
		cfg.MetricsToken = v
	}
	if v := os.Getenv("THEIA_ALLOWED_ORIGINS"); v != "" {
		cfg.AllowedOrigins = splitAllowedOrigins(v)
	}
	if err := applyArchiveLimitEnv(cfg); err != nil {
		return nil, err
	}
	if err := applyBulkLimitEnv(cfg); err != nil {
		return nil, err
	}
	if err := validateArchiveLimits(cfg); err != nil {
		return nil, err
	}
	if err := validateBulkLimits(cfg); err != nil {
		return nil, err
	}

	return cfg, nil
}

func parseEnvMinutes(key, value string) (int, error) {
	minutes, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil {
		return 0, fmt.Errorf("parsing %s: %w", key, err)
	}
	if minutes < 0 {
		return 0, fmt.Errorf("parsing %s: value must be non-negative", key)
	}
	return minutes, nil
}

func applyArchiveLimitEnv(cfg *Config) error {
	if v := os.Getenv("THEIA_RESTORE_MAX_COMPRESSED_BYTES"); v != "" {
		parsed, err := parsePositiveEnvInt64("THEIA_RESTORE_MAX_COMPRESSED_BYTES", v)
		if err != nil {
			return err
		}
		cfg.RestoreArchiveLimits.MaxCompressedBytes = parsed
	}
	if v := os.Getenv("THEIA_RESTORE_MAX_TOTAL_BYTES"); v != "" {
		parsed, err := parsePositiveEnvInt64("THEIA_RESTORE_MAX_TOTAL_BYTES", v)
		if err != nil {
			return err
		}
		cfg.RestoreArchiveLimits.MaxTotalBytes = parsed
	}
	if v := os.Getenv("THEIA_RESTORE_MAX_ENTRY_BYTES"); v != "" {
		parsed, err := parsePositiveEnvInt64("THEIA_RESTORE_MAX_ENTRY_BYTES", v)
		if err != nil {
			return err
		}
		cfg.RestoreArchiveLimits.MaxEntryBytes = parsed
	}
	if v := os.Getenv("THEIA_RESTORE_MAX_FILE_ENTRIES"); v != "" {
		parsed, err := parsePositiveEnvInt("THEIA_RESTORE_MAX_FILE_ENTRIES", v)
		if err != nil {
			return err
		}
		cfg.RestoreArchiveLimits.MaxFileEntries = parsed
	}
	if v := os.Getenv("THEIA_INSTANCE_BACKUP_MAX_TOTAL_BYTES"); v != "" {
		parsed, err := parsePositiveEnvInt64("THEIA_INSTANCE_BACKUP_MAX_TOTAL_BYTES", v)
		if err != nil {
			return err
		}
		cfg.InstanceBackupArchiveLimits.MaxTotalBytes = parsed
	}
	if v := os.Getenv("THEIA_INSTANCE_BACKUP_MAX_ENTRY_BYTES"); v != "" {
		parsed, err := parsePositiveEnvInt64("THEIA_INSTANCE_BACKUP_MAX_ENTRY_BYTES", v)
		if err != nil {
			return err
		}
		cfg.InstanceBackupArchiveLimits.MaxEntryBytes = parsed
	}
	if v := os.Getenv("THEIA_INSTANCE_BACKUP_MAX_FILE_ENTRIES"); v != "" {
		parsed, err := parsePositiveEnvInt("THEIA_INSTANCE_BACKUP_MAX_FILE_ENTRIES", v)
		if err != nil {
			return err
		}
		cfg.InstanceBackupArchiveLimits.MaxFileEntries = parsed
	}
	if v := os.Getenv("THEIA_INSTANCE_BACKUP_MAX_DURATION_SECONDS"); v != "" {
		parsed, err := parsePositiveEnvDurationSeconds("THEIA_INSTANCE_BACKUP_MAX_DURATION_SECONDS", v)
		if err != nil {
			return err
		}
		cfg.InstanceBackupArchiveLimits.MaxDurationSeconds = parsed
	}
	return nil
}

func applyBulkLimitEnv(cfg *Config) error {
	if v := os.Getenv("THEIA_BULK_BACKUP_MAX_DEVICES"); v != "" {
		parsed, err := parsePositiveEnvInt("THEIA_BULK_BACKUP_MAX_DEVICES", v)
		if err != nil {
			return err
		}
		cfg.BulkBackupLimits.MaxDevices = parsed
	}
	if v := os.Getenv("THEIA_BULK_BACKUP_MAX_QUEUED_JOBS"); v != "" {
		parsed, err := parsePositiveEnvInt("THEIA_BULK_BACKUP_MAX_QUEUED_JOBS", v)
		if err != nil {
			return err
		}
		cfg.BulkBackupLimits.MaxQueuedJobs = parsed
	}
	if v := os.Getenv("THEIA_BULK_DOWNLOAD_MAX_DEVICES"); v != "" {
		parsed, err := parsePositiveEnvInt("THEIA_BULK_DOWNLOAD_MAX_DEVICES", v)
		if err != nil {
			return err
		}
		cfg.BulkDownloadLimits.MaxDevices = parsed
	}
	if v := os.Getenv("THEIA_BULK_DOWNLOAD_MAX_FILES"); v != "" {
		parsed, err := parsePositiveEnvInt("THEIA_BULK_DOWNLOAD_MAX_FILES", v)
		if err != nil {
			return err
		}
		cfg.BulkDownloadLimits.MaxFiles = parsed
	}
	if v := os.Getenv("THEIA_BULK_DOWNLOAD_MAX_BYTES"); v != "" {
		parsed, err := parsePositiveEnvInt64("THEIA_BULK_DOWNLOAD_MAX_BYTES", v)
		if err != nil {
			return err
		}
		cfg.BulkDownloadLimits.MaxBytes = parsed
	}
	if v := os.Getenv("THEIA_BULK_DOWNLOAD_MAX_CONCURRENT_PER_ACTOR"); v != "" {
		parsed, err := parsePositiveEnvInt("THEIA_BULK_DOWNLOAD_MAX_CONCURRENT_PER_ACTOR", v)
		if err != nil {
			return err
		}
		cfg.BulkDownloadLimits.MaxConcurrentPerActor = parsed
	}
	if v := os.Getenv("THEIA_BULK_DOWNLOAD_MAX_CONCURRENT_GLOBAL"); v != "" {
		parsed, err := parsePositiveEnvInt("THEIA_BULK_DOWNLOAD_MAX_CONCURRENT_GLOBAL", v)
		if err != nil {
			return err
		}
		cfg.BulkDownloadLimits.MaxConcurrentGlobal = parsed
	}
	return nil
}

func parsePositiveEnvInt64(key, value string) (int64, error) {
	parsed, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parsing %s: %w", key, err)
	}
	if parsed <= 0 {
		return 0, fmt.Errorf("parsing %s: value must be positive", key)
	}
	return parsed, nil
}

func parsePositiveEnvInt(key, value string) (int, error) {
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil {
		return 0, fmt.Errorf("parsing %s: %w", key, err)
	}
	if parsed <= 0 {
		return 0, fmt.Errorf("parsing %s: value must be positive", key)
	}
	return parsed, nil
}

func parsePositiveEnvDurationSeconds(key, value string) (int, error) {
	parsed, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parsing %s: %w", key, err)
	}
	if parsed <= 0 {
		return 0, fmt.Errorf("parsing %s: value must be positive", key)
	}
	if parsed > maxInstanceBackupDurationSeconds || parsed > int64(math.MaxInt) {
		return 0, fmt.Errorf("parsing %s: value exceeds maximum supported duration", key)
	}
	return int(parsed), nil
}

func validateArchiveLimits(cfg *Config) error {
	if err := validatePositiveInt64("restore_archive_limits.max_compressed_bytes", cfg.RestoreArchiveLimits.MaxCompressedBytes); err != nil {
		return err
	}
	if err := validatePositiveInt64("restore_archive_limits.max_total_bytes", cfg.RestoreArchiveLimits.MaxTotalBytes); err != nil {
		return err
	}
	if err := validatePositiveInt64("restore_archive_limits.max_entry_bytes", cfg.RestoreArchiveLimits.MaxEntryBytes); err != nil {
		return err
	}
	if err := validatePositiveInt("restore_archive_limits.max_file_entries", cfg.RestoreArchiveLimits.MaxFileEntries); err != nil {
		return err
	}
	if err := validatePositiveInt64("instance_backup_archive_limits.max_total_bytes", cfg.InstanceBackupArchiveLimits.MaxTotalBytes); err != nil {
		return err
	}
	if err := validatePositiveInt64("instance_backup_archive_limits.max_entry_bytes", cfg.InstanceBackupArchiveLimits.MaxEntryBytes); err != nil {
		return err
	}
	if err := validatePositiveInt("instance_backup_archive_limits.max_file_entries", cfg.InstanceBackupArchiveLimits.MaxFileEntries); err != nil {
		return err
	}
	if err := validatePositiveInt("instance_backup_archive_limits.max_duration_seconds", cfg.InstanceBackupArchiveLimits.MaxDurationSeconds); err != nil {
		return err
	}
	if int64(cfg.InstanceBackupArchiveLimits.MaxDurationSeconds) > maxInstanceBackupDurationSeconds {
		return fmt.Errorf("instance_backup_archive_limits.max_duration_seconds exceeds maximum supported duration")
	}
	return nil
}

func validateBulkLimits(cfg *Config) error {
	if err := validatePositiveInt("bulk_backup_limits.max_devices", cfg.BulkBackupLimits.MaxDevices); err != nil {
		return err
	}
	if err := validatePositiveInt("bulk_backup_limits.max_queued_jobs", cfg.BulkBackupLimits.MaxQueuedJobs); err != nil {
		return err
	}
	if err := validatePositiveInt("bulk_download_limits.max_devices", cfg.BulkDownloadLimits.MaxDevices); err != nil {
		return err
	}
	if err := validatePositiveInt("bulk_download_limits.max_files", cfg.BulkDownloadLimits.MaxFiles); err != nil {
		return err
	}
	if err := validatePositiveInt64("bulk_download_limits.max_bytes", cfg.BulkDownloadLimits.MaxBytes); err != nil {
		return err
	}
	if err := validatePositiveInt("bulk_download_limits.max_concurrent_per_actor", cfg.BulkDownloadLimits.MaxConcurrentPerActor); err != nil {
		return err
	}
	if err := validatePositiveInt("bulk_download_limits.max_concurrent_global", cfg.BulkDownloadLimits.MaxConcurrentGlobal); err != nil {
		return err
	}
	return nil
}

func validatePositiveInt64(name string, value int64) error {
	if value <= 0 {
		return fmt.Errorf("%s must be positive", name)
	}
	return nil
}

func validatePositiveInt(name string, value int) error {
	if value <= 0 {
		return fmt.Errorf("%s must be positive", name)
	}
	return nil
}

func splitAllowedOrigins(value string) []string {
	fields := strings.FieldsFunc(value, func(r rune) bool {
		return r == ',' || r == '\n' || r == '\r' || r == '\t' || r == ' '
	})
	origins := make([]string, 0, len(fields))
	for _, field := range fields {
		field = strings.TrimSpace(field)
		if field != "" {
			origins = append(origins, field)
		}
	}
	return origins
}
