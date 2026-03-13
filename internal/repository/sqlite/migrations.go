package sqlite

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/azmin/mikrotik-theia/internal/domain"
)

// RunMigrations creates all required tables if they do not exist
// and seeds default settings values.
func RunMigrations(db *sql.DB) error {
	migrations := []string{
		// Devices table
		`CREATE TABLE IF NOT EXISTS devices (
			id TEXT PRIMARY KEY,
			hostname TEXT NOT NULL DEFAULT '',
			ip TEXT NOT NULL,
			snmp_credentials_json TEXT NOT NULL DEFAULT '{}',
			device_type TEXT NOT NULL DEFAULT 'unknown',
			status TEXT NOT NULL DEFAULT 'unknown',
			sys_name TEXT NOT NULL DEFAULT '',
			sys_descr TEXT NOT NULL DEFAULT '',
			sys_object_id TEXT NOT NULL DEFAULT '',
			hardware_model TEXT NOT NULL DEFAULT '',
			managed INTEGER NOT NULL DEFAULT 1,
			tags_json TEXT NOT NULL DEFAULT '{}',
			created_at DATETIME NOT NULL,
			updated_at DATETIME NOT NULL
		)`,

		// Unique index on IP for dedup during neighbor discovery
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_devices_ip ON devices(ip)`,

		// Interfaces table
		`CREATE TABLE IF NOT EXISTS interfaces (
			id TEXT PRIMARY KEY,
			device_id TEXT NOT NULL,
			if_index INTEGER NOT NULL,
			if_name TEXT NOT NULL DEFAULT '',
			if_descr TEXT NOT NULL DEFAULT '',
			speed INTEGER NOT NULL DEFAULT 0,
			admin_status TEXT NOT NULL DEFAULT 'unknown',
			oper_status TEXT NOT NULL DEFAULT 'unknown',
			created_at DATETIME NOT NULL,
			updated_at DATETIME NOT NULL,
			FOREIGN KEY (device_id) REFERENCES devices(id) ON DELETE CASCADE
		)`,

		`CREATE INDEX IF NOT EXISTS idx_interfaces_device_id ON interfaces(device_id)`,

		// Links table
		`CREATE TABLE IF NOT EXISTS links (
			id TEXT PRIMARY KEY,
			source_device_id TEXT NOT NULL,
			source_if_name TEXT NOT NULL,
			target_device_id TEXT NOT NULL,
			target_if_name TEXT NOT NULL,
			discovery_protocol TEXT NOT NULL DEFAULT 'manual',
			created_at DATETIME NOT NULL,
			updated_at DATETIME NOT NULL,
			UNIQUE(source_device_id, source_if_name, target_device_id, target_if_name),
			FOREIGN KEY (source_device_id) REFERENCES devices(id) ON DELETE CASCADE,
			FOREIGN KEY (target_device_id) REFERENCES devices(id) ON DELETE CASCADE
		)`,

		// Settings table (key-value store for runtime config)
		`CREATE TABLE IF NOT EXISTS settings (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL DEFAULT '',
			updated_at DATETIME NOT NULL
		)`,

		// Device position table used by the interactive canvas layout
		`CREATE TABLE IF NOT EXISTS device_positions (
			device_id TEXT PRIMARY KEY,
			x REAL NOT NULL DEFAULT 0,
			y REAL NOT NULL DEFAULT 0,
			pinned INTEGER NOT NULL DEFAULT 0,
			updated_at DATETIME NOT NULL,
			FOREIGN KEY (device_id) REFERENCES devices(id) ON DELETE CASCADE
		)`,

		// SNMP credential profiles for reuse across multiple devices
		`CREATE TABLE IF NOT EXISTS snmp_profiles (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			description TEXT NOT NULL DEFAULT '',
			credentials_json TEXT NOT NULL DEFAULT '{}',
			created_at DATETIME NOT NULL,
			updated_at DATETIME NOT NULL
		)`,

		`CREATE UNIQUE INDEX IF NOT EXISTS idx_snmp_profiles_name ON snmp_profiles(name)`,

		// Per-device metric source configuration
		`ALTER TABLE devices ADD COLUMN metrics_source TEXT NOT NULL DEFAULT 'prometheus'`,
		`ALTER TABLE devices ADD COLUMN prometheus_label_name TEXT NOT NULL DEFAULT 'instance'`,
		`ALTER TABLE devices ADD COLUMN prometheus_label_value TEXT NOT NULL DEFAULT ''`,
	}

	for i, m := range migrations {
		if _, err := db.Exec(m); err != nil {
			// ALTER TABLE ADD COLUMN is idempotent in this setup; ignore duplicate column errors.
			if strings.Contains(m, "ADD COLUMN") && strings.Contains(err.Error(), "duplicate column name") {
				continue
			}
			return fmt.Errorf("migration %d failed: %w", i, err)
		}
	}

	// Seed default settings (INSERT OR IGNORE so existing values are not overwritten)
	now := time.Now().UTC()
	for key, value := range domain.DefaultSettings() {
		_, err := db.Exec(
			`INSERT OR IGNORE INTO settings (key, value, updated_at) VALUES (?, ?, ?)`,
			key, value, now,
		)
		if err != nil {
			return fmt.Errorf("seeding default setting %q: %w", key, err)
		}
	}

	return nil
}
