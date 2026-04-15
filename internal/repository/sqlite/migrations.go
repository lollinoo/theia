package sqlite

import (
	"database/sql"
	"embed"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/golang-migrate/migrate/v4"
	pgdriver "github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/database/sqlite3"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/domain"
)

//go:embed migrations/*.sql
var sqliteMigrationsFS embed.FS

//go:embed postgres_migrations/*.sql
var postgresMigrationsFS embed.FS

// lastPreMigrateVersion is the version of the last migration that re-expresses
// the existing inline schema. Existing databases are seeded at this version
// so golang-migrate does not re-apply them.
const lastPreMigrateVersion = 5

// RunMigrations runs all pending database migrations using golang-migrate.
// On existing databases (those with a devices table but no schema_migrations),
// it seeds the schema_migrations table at the correct version first.
// The encryptionKey is used for Go-level data migrations (e.g., encrypting
// existing plaintext SNMP credentials).
func RunMigrations(db *sql.DB, encryptionKey ...[]byte) error {
	var encKey []byte
	if len(encryptionKey) > 0 {
		encKey = encryptionKey[0]
	}
	dialect := detectDialectFromDB(db)
	switch dialect {
	case DialectSQLite:
		if err := runSQLiteMigrations(db); err != nil {
			return err
		}
	case DialectPostgres:
		if err := runPostgresMigrations(db); err != nil {
			return err
		}
	default:
		return fmt.Errorf("unsupported database dialect %q", dialect)
	}

	// Encrypt any plaintext SNMP credentials (Go-level data migration)
	if err := migrateEncryptSNMPCredentials(db, encKey); err != nil {
		return fmt.Errorf("encrypting SNMP credentials: %w", err)
	}

	// Backfill devices.poll_class from device_type using the runtime
	// classification helper (Phase 39 D-16). Idempotent.
	if err := migrateDevicePollClass(db); err != nil {
		return fmt.Errorf("backfilling device poll_class: %w", err)
	}

	// Seed default settings (INSERT OR IGNORE so existing values are not overwritten)
	if err := seedDefaultSettings(db); err != nil {
		return fmt.Errorf("seeding default settings: %w", err)
	}

	return nil
}

func runSQLiteMigrations(db *sql.DB) error {
	if err := ensureMigrationVersion(db, lastPreMigrateVersion); err != nil {
		return fmt.Errorf("ensuring migration version: %w", err)
	}

	if err := verifyLegacyTablesMigrated(db); err != nil {
		return fmt.Errorf("verifying legacy table migration: %w", err)
	}

	sourceDriver, err := iofs.New(sqliteMigrationsFS, "migrations")
	if err != nil {
		return fmt.Errorf("creating migration source: %w", err)
	}

	dbDriver, err := sqlite3.WithInstance(db, &sqlite3.Config{
		MigrationsTable: "schema_migrations",
	})
	if err != nil {
		return fmt.Errorf("creating migration db driver: %w", err)
	}

	m, err := migrate.NewWithInstance("iofs", sourceDriver, "sqlite3", dbDriver)
	if err != nil {
		return fmt.Errorf("creating migrator: %w", err)
	}

	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return fmt.Errorf("running migrations: %w", err)
	}

	version, dirty, _ := m.Version()
	log.Printf("Migrations complete: dialect=%s version=%d dirty=%v", DialectSQLite, version, dirty)
	return nil
}

func runPostgresMigrations(db *sql.DB) error {
	sourceDriver, err := iofs.New(postgresMigrationsFS, "postgres_migrations")
	if err != nil {
		return fmt.Errorf("creating postgres migration source: %w", err)
	}

	dbDriver, err := pgdriver.WithInstance(db, &pgdriver.Config{
		MigrationsTable: "schema_migrations",
	})
	if err != nil {
		return fmt.Errorf("creating postgres migration db driver: %w", err)
	}

	m, err := migrate.NewWithInstance("iofs", sourceDriver, "postgres", dbDriver)
	if err != nil {
		return fmt.Errorf("creating postgres migrator: %w", err)
	}

	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return fmt.Errorf("running postgres migrations: %w", err)
	}

	version, dirty, _ := m.Version()
	log.Printf("Migrations complete: dialect=%s version=%d dirty=%v", DialectPostgres, version, dirty)
	return nil
}

// ensureMigrationVersion handles the transition from the old inline migration
// system to golang-migrate. For existing databases (devices table exists but
// schema_migrations does not), it creates schema_migrations seeded at the
// given version so golang-migrate knows these migrations are already applied.
func ensureMigrationVersion(db *sql.DB, lastExistingVersion uint) error {
	// Check if schema_migrations already exists
	var count int
	err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master
		WHERE type='table' AND name='schema_migrations'`).Scan(&count)
	if err != nil {
		return fmt.Errorf("checking schema_migrations: %w", err)
	}
	if count > 0 {
		return nil // Already using golang-migrate
	}

	// Check if this is an existing database (has devices table)
	err = db.QueryRow(`SELECT COUNT(*) FROM sqlite_master
		WHERE type='table' AND name='devices'`).Scan(&count)
	if err != nil {
		return fmt.Errorf("checking devices table: %w", err)
	}
	if count == 0 {
		return nil // Fresh database, migrations will run from scratch
	}

	// Existing database without golang-migrate: seed schema_migrations
	log.Printf("Existing database detected, seeding schema_migrations at version %d", lastExistingVersion)
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
		version bigint not null primary key, dirty boolean not null)`)
	if err != nil {
		return fmt.Errorf("creating schema_migrations: %w", err)
	}
	_, err = db.Exec(`INSERT INTO schema_migrations (version, dirty) VALUES (?, false)`,
		lastExistingVersion)
	if err != nil {
		return fmt.Errorf("seeding migration version: %w", err)
	}

	return nil
}

// verifyLegacyTablesMigrated checks that ssh_credentials data has been
// migrated to ssh_profiles before allowing the drop migration to run.
// If unmigrated data is found, it re-runs the data migration.
func verifyLegacyTablesMigrated(db *sql.DB) error {
	// Check if ssh_credentials table exists
	var tableExists int
	err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master
		WHERE type='table' AND name='ssh_credentials'`).Scan(&tableExists)
	if err != nil {
		return fmt.Errorf("checking ssh_credentials: %w", err)
	}
	if tableExists == 0 {
		return nil // Already dropped or never existed
	}

	// Check if ssh_profile_id column still exists on devices table.
	// Migration 000014 drops this column; on databases that have already run
	// 000014, the column is absent and the legacy verification is no longer needed.
	var colExists int
	err = db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('devices') WHERE name='ssh_profile_id'`).Scan(&colExists)
	if err != nil {
		return fmt.Errorf("checking ssh_profile_id column: %w", err)
	}
	if colExists == 0 {
		log.Printf("Legacy table verification: ssh_profile_id column already dropped, skipping")
		return nil
	}

	// Count credentials that have no corresponding ssh_profile link on their device
	var unmigrated int
	err = db.QueryRow(`
		SELECT COUNT(*) FROM ssh_credentials sc
		JOIN devices d ON d.id = sc.device_id
		WHERE d.ssh_profile_id IS NULL
	`).Scan(&unmigrated)
	if err != nil {
		return fmt.Errorf("checking unmigrated credentials: %w", err)
	}

	if unmigrated == 0 {
		log.Printf("Legacy table verification: all ssh_credentials migrated")
		return nil
	}

	log.Printf("Legacy table verification: %d unmigrated credentials found, re-migrating", unmigrated)

	// Re-run the SSH credentials migration within a transaction
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("beginning re-migration transaction: %w", err)
	}
	defer tx.Rollback()

	rows, err := tx.Query(`
		SELECT sc.device_id, sc.username, sc.port, sc.auth_method, sc.encrypted_secret
		FROM ssh_credentials sc
		JOIN devices d ON d.id = sc.device_id
		WHERE d.ssh_profile_id IS NULL
	`)
	if err != nil {
		return fmt.Errorf("querying unmigrated credentials: %w", err)
	}
	defer rows.Close()

	type credRow struct {
		deviceID, username, authMethod, encryptedSecret string
		port                                            int
	}
	var creds []credRow
	for rows.Next() {
		var c credRow
		if err := rows.Scan(&c.deviceID, &c.username, &c.port, &c.authMethod, &c.encryptedSecret); err != nil {
			return fmt.Errorf("scanning credential: %w", err)
		}
		creds = append(creds, c)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterating credentials: %w", err)
	}

	now := time.Now().UTC()
	for _, c := range creds {
		profileID := uuid.New().String()
		name := fmt.Sprintf("Re-migrated: %s:%d", c.username, c.port)

		_, err := tx.Exec(
			`INSERT INTO ssh_profiles (id, name, description, username, port, auth_method, encrypted_secret, created_at, updated_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			profileID, name, "Auto-re-migrated from per-device credentials",
			c.username, c.port, c.authMethod, c.encryptedSecret, now, now,
		)
		if err != nil {
			return fmt.Errorf("creating re-migrated profile: %w", err)
		}

		_, err = tx.Exec(`UPDATE devices SET ssh_profile_id = ? WHERE id = ?`, profileID, c.deviceID)
		if err != nil {
			return fmt.Errorf("linking device to re-migrated profile: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing re-migration: %w", err)
	}

	log.Printf("Legacy table verification: re-migrated %d credentials successfully", len(creds))
	return nil
}

// migrateEncryptSNMPCredentials encrypts any plaintext SNMP credentials found in the database.
// This is a Go-level data migration that runs after SQL migrations.
// It is idempotent: already-encrypted values are detected by tryDecryptField and skipped.
func migrateEncryptSNMPCredentials(db *sql.DB, encryptionKey []byte) error {
	if len(encryptionKey) == 0 {
		return nil // No key = no encryption possible
	}

	// Migrate devices
	deviceCount, err := migrateDeviceSNMPCredentials(db, encryptionKey)
	if err != nil {
		return fmt.Errorf("migrating device SNMP credentials: %w", err)
	}

	// Migrate SNMP profiles
	profileCount, err := migrateSNMPProfileCredentials(db, encryptionKey)
	if err != nil {
		return fmt.Errorf("migrating SNMP profile credentials: %w", err)
	}

	if deviceCount > 0 || profileCount > 0 {
		log.Printf("SNMP credential encryption migration: %d devices, %d profiles updated", deviceCount, profileCount)
	}
	return nil
}

func migrateDeviceSNMPCredentials(db *sql.DB, key []byte) (int, error) {
	queryDB := wrapDB(db)
	rows, err := db.Query("SELECT id, snmp_credentials_json FROM devices WHERE snmp_credentials_json != '' AND snmp_credentials_json != '{}'")
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	type row struct {
		id        string
		credsJSON string
	}
	var toUpdate []row
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.id, &r.credsJSON); err != nil {
			return 0, err
		}
		toUpdate = append(toUpdate, r)
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}

	updated := 0
	for _, r := range toUpdate {
		var creds domain.SNMPCredentials
		if err := json.Unmarshal([]byte(r.credsJSON), &creds); err != nil {
			log.Printf("Warning: skipping device %s SNMP migration (invalid JSON): %v", r.id, err)
			continue
		}

		// Check if already encrypted by attempting decrypt
		if isAlreadyEncrypted(&creds, key) {
			continue
		}

		// Encrypt plaintext credentials
		if err := encryptSNMPCredentials(&creds, key); err != nil {
			log.Printf("Warning: failed to encrypt SNMP creds for device %s: %v", r.id, err)
			continue
		}

		newJSON, err := json.Marshal(creds)
		if err != nil {
			continue
		}

		if _, err := queryDB.Exec("UPDATE devices SET snmp_credentials_json = ? WHERE id = ?", string(newJSON), r.id); err != nil {
			log.Printf("Warning: failed to update device %s SNMP creds: %v", r.id, err)
			continue
		}
		updated++
	}
	return updated, nil
}

func migrateSNMPProfileCredentials(db *sql.DB, key []byte) (int, error) {
	queryDB := wrapDB(db)
	rows, err := db.Query("SELECT id, credentials_json FROM snmp_profiles WHERE credentials_json != '' AND credentials_json != '{}'")
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	type row struct {
		id        string
		credsJSON string
	}
	var toUpdate []row
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.id, &r.credsJSON); err != nil {
			return 0, err
		}
		toUpdate = append(toUpdate, r)
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}

	updated := 0
	for _, r := range toUpdate {
		var creds domain.SNMPCredentials
		if err := json.Unmarshal([]byte(r.credsJSON), &creds); err != nil {
			log.Printf("Warning: skipping profile %s SNMP migration (invalid JSON): %v", r.id, err)
			continue
		}

		if isAlreadyEncrypted(&creds, key) {
			continue
		}

		if err := encryptSNMPCredentials(&creds, key); err != nil {
			log.Printf("Warning: failed to encrypt SNMP creds for profile %s: %v", r.id, err)
			continue
		}

		newJSON, err := json.Marshal(creds)
		if err != nil {
			continue
		}

		if _, err := queryDB.Exec("UPDATE snmp_profiles SET credentials_json = ? WHERE id = ?", string(newJSON), r.id); err != nil {
			log.Printf("Warning: failed to update profile %s SNMP creds: %v", r.id, err)
			continue
		}
		updated++
	}
	return updated, nil
}

// isAlreadyEncrypted checks if any sensitive field in the credentials is already encrypted.
// Returns true if at least one field decrypts successfully (indicating already encrypted).
func isAlreadyEncrypted(creds *domain.SNMPCredentials, key []byte) bool {
	if creds.V2c != nil && creds.V2c.Community != "" {
		if _, ok := tryDecryptField(creds.V2c.Community, key); ok {
			return true
		}
	}
	if creds.V3 != nil {
		if creds.V3.AuthPassword != "" {
			if _, ok := tryDecryptField(creds.V3.AuthPassword, key); ok {
				return true
			}
		}
		if creds.V3.PrivPassword != "" {
			if _, ok := tryDecryptField(creds.V3.PrivPassword, key); ok {
				return true
			}
		}
	}
	return false
}

// migrateDevicePollClass backfills the devices.poll_class column by
// applying domain.ClassifyPollClass(device_type) to every row whose
// stored poll_class does not already match the computed value. This is
// a Go-level data migration that runs after SQL migrations apply, so
// the new column already exists. The function is idempotent: re-running
// on a fully-backfilled DB performs zero writes.
//
// Per D-16, this reuses the runtime classification helper as the single
// source of truth — the SQL CASE statement an .up.sql file would need
// and the runtime helper would otherwise drift over time.
func migrateDevicePollClass(db *sql.DB) error {
	queryDB := wrapDB(db)

	// Check the column exists before attempting backfill. On Postgres,
	// migrations have a different number; on SQLite the up file added
	// the column. Skip silently if the column is absent so this function
	// can run unconditionally from RunMigrations on every dialect.
	if !devicePollClassColumnExists(db) {
		return nil
	}

	rows, err := db.Query(`SELECT id, device_type, poll_class FROM devices`)
	if err != nil {
		return fmt.Errorf("querying devices for poll_class backfill: %w", err)
	}
	defer rows.Close()

	type row struct {
		id           string
		deviceType   string
		currentClass string
	}
	var toCheck []row
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.id, &r.deviceType, &r.currentClass); err != nil {
			return fmt.Errorf("scanning device row for poll_class backfill: %w", err)
		}
		toCheck = append(toCheck, r)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterating devices for poll_class backfill: %w", err)
	}

	updated := 0
	for _, r := range toCheck {
		computed := domain.ClassifyPollClass(domain.DeviceType(r.deviceType))
		if string(computed) == r.currentClass {
			continue // already correct — idempotency guarantee
		}
		if _, err := queryDB.Exec(
			`UPDATE devices SET poll_class = ? WHERE id = ?`,
			string(computed), r.id,
		); err != nil {
			log.Printf("Warning: failed to backfill poll_class for device %s: %v", r.id, err)
			continue
		}
		updated++
	}

	if updated > 0 {
		log.Printf("Device poll_class backfill: %d rows updated", updated)
	}
	return nil
}

// devicePollClassColumnExists returns true when the devices table has
// the poll_class column. Used by migrateDevicePollClass to skip cleanly
// on Postgres or partially-migrated databases.
func devicePollClassColumnExists(db *sql.DB) bool {
	if detectDialectFromDB(db) == DialectSQLite {
		var count int
		err := db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('devices') WHERE name='poll_class'`).Scan(&count)
		return err == nil && count > 0
	}
	// Postgres path: information_schema check
	var count int
	err := db.QueryRow(`SELECT COUNT(*) FROM information_schema.columns WHERE table_name='devices' AND column_name='poll_class'`).Scan(&count)
	return err == nil && count > 0
}

// seedDefaultSettings inserts default settings without overwriting existing values.
func seedDefaultSettings(db *sql.DB) error {
	queryDB := wrapDB(db)
	now := time.Now().UTC()
	statement := `INSERT INTO settings (key, value, updated_at) VALUES (?, ?, ?)
		ON CONFLICT(key) DO NOTHING`
	if detectDialectFromDB(db) == DialectSQLite {
		statement = `INSERT OR IGNORE INTO settings (key, value, updated_at) VALUES (?, ?, ?)`
	}
	for key, value := range domain.DefaultSettings() {
		_, err := queryDB.Exec(statement, key, value, now)
		if err != nil {
			return fmt.Errorf("seeding default setting %q: %w", key, err)
		}
	}
	return nil
}
