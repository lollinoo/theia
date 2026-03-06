package sqlite

import (
	"database/sql"
	"fmt"
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
	}

	for i, m := range migrations {
		if _, err := db.Exec(m); err != nil {
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
