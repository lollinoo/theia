package postgres

// This file defines migrations persistence behavior, ordering guarantees, and not-found conventions.

import (
	"database/sql"
	"embed"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/golang-migrate/migrate/v4"
	pgdriver "github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/crypto"
	"github.com/lollinoo/theia/internal/domain"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// RunMigrations runs all pending PostgreSQL migrations using golang-migrate.
// The optional key source is used for Go-level data migrations, such as encrypting
// existing plaintext SNMP credentials.
func RunMigrations(db *sql.DB, encryptionKey ...any) error {
	var encKey any
	if len(encryptionKey) > 0 {
		encKey = encryptionKey[0]
	}
	if err := runPostgresMigrations(db); err != nil {
		return err
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

	if err := migrateDefaultCanvasMap(db); err != nil {
		return fmt.Errorf("migrating default canvas map: %w", err)
	}
	if err := migrateCanvasMapMemberships(db); err != nil {
		return fmt.Errorf("materializing canvas map memberships: %w", err)
	}
	if err := migrateCanvasMapDeviceAreaMemberships(db); err != nil {
		return fmt.Errorf("materializing canvas map device area memberships: %w", err)
	}

	// Seed default settings without overwriting existing values.
	if err := seedDefaultSettings(db); err != nil {
		return fmt.Errorf("seeding default settings: %w", err)
	}
	if err := seedAuthSystemRolesAndPermissions(db); err != nil {
		return fmt.Errorf("seeding auth system roles and permissions: %w", err)
	}

	return nil
}

func runPostgresMigrations(db *sql.DB) error {
	sourceDriver, err := iofs.New(migrationsFS, "migrations")
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

// migrateEncryptSNMPCredentials encrypts any plaintext SNMP credentials found in the database.
// This is a Go-level data migration that runs after SQL migrations.
// It is idempotent: already-encrypted values are detected by tryDecryptField and skipped.
func migrateEncryptSNMPCredentials(db *sql.DB, encryptionKey any) error {
	if encryptionKey == nil {
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
	credentialProfileCount, err := migrateCredentialProfileSecrets(db, encryptionKey)
	if err != nil {
		return fmt.Errorf("migrating credential profile secrets: %w", err)
	}

	if deviceCount > 0 || profileCount > 0 || credentialProfileCount > 0 {
		log.Printf("Credential encryption migration: %d devices, %d SNMP profiles, %d credential profiles updated", deviceCount, profileCount, credentialProfileCount)
	}
	return nil
}

func migrateDeviceSNMPCredentials(db *sql.DB, key any) (int, error) {
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

		if !hasSensitiveSNMPCredentials(&creds) {
			continue
		}

		changed, err := normalizeSNMPCredentialsForMigration(&creds, key)
		if err != nil {
			return updated, fmt.Errorf("device %s has SNMP credentials that cannot be normalized: %w", r.id, err)
		}
		if !changed {
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

func migrateSNMPProfileCredentials(db *sql.DB, key any) (int, error) {
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

		if !hasSensitiveSNMPCredentials(&creds) {
			continue
		}

		changed, err := normalizeSNMPCredentialsForMigration(&creds, key)
		if err != nil {
			return updated, fmt.Errorf("SNMP profile %s has credentials that cannot be normalized: %w", r.id, err)
		}
		if !changed {
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

func migrateCredentialProfileSecrets(db *sql.DB, key any) (int, error) {
	queryDB := wrapDB(db)
	rows, err := db.Query("SELECT id, encrypted_secret FROM credential_profiles WHERE encrypted_secret != ''")
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	type row struct {
		id              string
		encryptedSecret string
	}
	var toUpdate []row
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.id, &r.encryptedSecret); err != nil {
			return 0, err
		}
		toUpdate = append(toUpdate, r)
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}

	updated := 0
	for _, r := range toUpdate {
		normalized, changed, err := normalizeSensitiveMigrationField(r.encryptedSecret, key)
		if err != nil {
			return updated, fmt.Errorf("credential profile %s secret cannot be normalized: %w", r.id, err)
		}
		if !changed {
			continue
		}
		if _, err := queryDB.Exec("UPDATE credential_profiles SET encrypted_secret = ? WHERE id = ?", normalized, r.id); err != nil {
			return updated, fmt.Errorf("updating credential profile %s secret: %w", r.id, err)
		}
		updated++
	}
	return updated, nil
}

func hasSensitiveSNMPCredentials(creds *domain.SNMPCredentials) bool {
	if creds == nil {
		return false
	}
	if creds.V2c != nil && creds.V2c.Community != "" {
		return true
	}
	if creds.V3 != nil {
		return creds.V3.AuthPassword != "" || creds.V3.PrivPassword != ""
	}
	return false
}

func normalizeSNMPCredentialsForMigration(creds *domain.SNMPCredentials, key any) (bool, error) {
	changed := false
	if creds.V2c != nil && creds.V2c.Community != "" {
		normalized, fieldChanged, err := normalizeSensitiveMigrationField(creds.V2c.Community, key)
		if err != nil {
			return false, fmt.Errorf("v2c community: %w", err)
		}
		creds.V2c.Community = normalized
		changed = changed || fieldChanged
	}
	if creds.V3 != nil {
		if creds.V3.AuthPassword != "" {
			normalized, fieldChanged, err := normalizeSensitiveMigrationField(creds.V3.AuthPassword, key)
			if err != nil {
				return false, fmt.Errorf("v3 auth password: %w", err)
			}
			creds.V3.AuthPassword = normalized
			changed = changed || fieldChanged
		}
		if creds.V3.PrivPassword != "" {
			normalized, fieldChanged, err := normalizeSensitiveMigrationField(creds.V3.PrivPassword, key)
			if err != nil {
				return false, fmt.Errorf("v3 priv password: %w", err)
			}
			creds.V3.PrivPassword = normalized
			changed = changed || fieldChanged
		}
	}
	return changed, nil
}

func normalizeSensitiveMigrationField(value string, key any) (string, bool, error) {
	if value == "" {
		return value, false, nil
	}
	keyring, ok := key.(*crypto.Keyring)
	if !ok || keyring == nil {
		if _, legacyOK := key.([]byte); legacyOK {
			if _, alreadyEncrypted := tryDecryptFieldWithKeySource(value, key); alreadyEncrypted {
				return value, false, nil
			}
			if looksLikeEncryptedSNMPField(value) {
				return "", false, fmt.Errorf("legacy ciphertext cannot be decrypted with the configured key")
			}
			encrypted, err := encryptSensitiveSNMPField(value, key)
			if err != nil {
				return "", false, err
			}
			return encrypted, true, nil
		}
		return "", false, fmt.Errorf("encryption keyring is required")
	}

	if crypto.IsEnvelope(value) {
		keyID, err := crypto.EnvelopeKeyID(value)
		if err != nil {
			return "", false, err
		}
		plaintext, err := keyring.DecryptString(value)
		if err != nil {
			return "", false, err
		}
		if keyID == keyring.ActiveKeyID() {
			return value, false, nil
		}
		rewrapped, err := keyring.EncryptString(plaintext)
		if err != nil {
			return "", false, err
		}
		return rewrapped, true, nil
	}

	if plaintext, _, err := keyring.DecryptLegacyString(value); err == nil {
		rewrapped, err := keyring.EncryptString(plaintext)
		if err != nil {
			return "", false, err
		}
		return rewrapped, true, nil
	}
	if looksLikeEncryptedSNMPField(value) {
		return "", false, fmt.Errorf("encrypted-looking value cannot be decrypted with any configured key")
	}
	encrypted, err := keyring.EncryptString(value)
	if err != nil {
		return "", false, err
	}
	return encrypted, true, nil
}

func hasUndecryptableEncryptedSNMPCredentials(creds *domain.SNMPCredentials, key any) bool {
	if creds == nil {
		return false
	}
	if creds.V2c != nil && isUndecryptableEncryptedSNMPField(creds.V2c.Community, key) {
		return true
	}
	if creds.V3 != nil {
		return isUndecryptableEncryptedSNMPField(creds.V3.AuthPassword, key) ||
			isUndecryptableEncryptedSNMPField(creds.V3.PrivPassword, key)
	}
	return false
}

func isUndecryptableEncryptedSNMPField(value string, key any) bool {
	if !looksLikeEncryptedSNMPField(value) {
		return false
	}
	_, ok := tryDecryptFieldWithKeySource(value, key)
	return !ok
}

func looksLikeEncryptedSNMPField(value string) bool {
	if value == "" {
		return false
	}
	if crypto.IsEnvelope(value) {
		return true
	}
	ciphertext, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		return false
	}
	return len(ciphertext) >= 12+16
}

// isAlreadyEncrypted checks if any sensitive field in the credentials is already encrypted.
// Returns true if at least one field decrypts successfully (indicating already encrypted).
func isAlreadyEncrypted(creds *domain.SNMPCredentials, key any) bool {
	if creds.V2c != nil && creds.V2c.Community != "" {
		if _, ok := tryDecryptFieldWithKeySource(creds.V2c.Community, key); ok {
			return true
		}
	}
	if creds.V3 != nil {
		if creds.V3.AuthPassword != "" {
			if _, ok := tryDecryptFieldWithKeySource(creds.V3.AuthPassword, key); ok {
				return true
			}
		}
		if creds.V3.PrivPassword != "" {
			if _, ok := tryDecryptFieldWithKeySource(creds.V3.PrivPassword, key); ok {
				return true
			}
		}
	}
	return false
}

func tryDecryptFieldWithKeySource(value string, key any) (string, bool) {
	switch k := key.(type) {
	case *crypto.Keyring:
		if !crypto.IsEnvelope(value) {
			if plaintext, _, err := k.DecryptLegacyString(value); err == nil {
				return plaintext, true
			}
			return "", false
		}
		plaintext, err := k.DecryptString(value)
		return plaintext, err == nil
	case []byte:
		return tryDecryptField(value, k)
	default:
		return "", false
	}
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

	// Check the column exists before attempting backfill. Skip silently if the
	// column is absent so this function can run unconditionally.
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
// on partially-migrated databases.
func devicePollClassColumnExists(db *sql.DB) bool {
	var count int
	err := db.QueryRow(`SELECT COUNT(*) FROM information_schema.columns WHERE table_name='devices' AND column_name='poll_class'`).Scan(&count)
	return err == nil && count > 0
}

func migrateDefaultCanvasMap(db *sql.DB) error {
	wrapped := wrapDB(db)
	hasCanvasMapsTable, err := tableExists(wrapped, "canvas_maps")
	if err != nil {
		return fmt.Errorf("checking canvas_maps table: %w", err)
	}
	if !hasCanvasMapsTable {
		return nil
	}
	hasMembershipMaterializedColumn, err := columnExists(wrapped, "canvas_maps", "membership_materialized")
	if err != nil {
		return fmt.Errorf("checking canvas_maps membership_materialized column: %w", err)
	}
	hasCanvasMapDevicesTable, err := tableExists(wrapped, "canvas_map_devices")
	if err != nil {
		return fmt.Errorf("checking canvas_map_devices table: %w", err)
	}
	hasCanvasMapLinksTable, err := tableExists(wrapped, "canvas_map_links")
	if err != nil {
		return fmt.Errorf("checking canvas_map_links table: %w", err)
	}
	hasCanvasMapAreasTable, err := tableExists(wrapped, "canvas_map_areas")
	if err != nil {
		return fmt.Errorf("checking canvas_map_areas table: %w", err)
	}
	hasAreasTable, err := tableExists(wrapped, "areas")
	if err != nil {
		return fmt.Errorf("checking areas table: %w", err)
	}

	tx, err := wrapped.Begin()
	if err != nil {
		return fmt.Errorf("starting default canvas map migration transaction: %w", err)
	}
	defer tx.Rollback()

	var mapID string
	defaultMaterialized := false
	if hasMembershipMaterializedColumn {
		var materializedRaw any
		err = tx.QueryRow(`SELECT id, membership_materialized FROM canvas_maps WHERE is_default = ? LIMIT 1`, true).Scan(&mapID, &materializedRaw)
		if err == nil {
			defaultMaterialized, err = normalizeBoolValue(materializedRaw)
			if err != nil {
				return fmt.Errorf("normalizing default canvas map materialization: %w", err)
			}
		}
	} else {
		err = tx.QueryRow(`SELECT id FROM canvas_maps WHERE is_default = ? LIMIT 1`, true).Scan(&mapID)
	}
	if err != nil {
		if err != sql.ErrNoRows {
			return fmt.Errorf("querying default canvas map: %w", err)
		}

		mapID = uuid.New().String()
		now := time.Now().UTC()
		if _, err := tx.Exec(
			`INSERT INTO canvas_maps (id, name, description, filter_json, is_default, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?)`,
			mapID,
			"Default",
			"Global canvas layout",
			"{}",
			true,
			now,
			now,
		); err != nil {
			return fmt.Errorf("creating default canvas map: %w", err)
		}
	}

	var copyPositionsStatement string
	copyPositionsArgs := []interface{}{mapID}
	if hasMembershipMaterializedColumn && defaultMaterialized {
		copyPositionsStatement = `INSERT INTO canvas_map_positions (map_id, device_id, x, y, pinned, updated_at)
			SELECT ?, dp.device_id, dp.x, dp.y, dp.pinned <> 0, dp.updated_at
			FROM device_positions dp
			WHERE EXISTS (
				SELECT 1 FROM canvas_map_devices cmd
				WHERE cmd.map_id = ? AND cmd.device_id = dp.device_id
			)
			ON CONFLICT(map_id, device_id) DO NOTHING`
		copyPositionsArgs = append(copyPositionsArgs, mapID)
	} else {
		copyPositionsStatement = `INSERT INTO canvas_map_positions (map_id, device_id, x, y, pinned, updated_at)
			SELECT ?, device_id, x, y, pinned <> 0, updated_at FROM device_positions
			ON CONFLICT(map_id, device_id) DO NOTHING`
	}

	if _, err := tx.Exec(copyPositionsStatement, copyPositionsArgs...); err != nil {
		return fmt.Errorf("copying legacy device positions into default canvas map: %w", err)
	}

	if hasMembershipMaterializedColumn && hasCanvasMapDevicesTable && hasCanvasMapLinksTable && !defaultMaterialized {
		copyDevicesStatement := `INSERT INTO canvas_map_devices (map_id, device_id, role, added_at)
			SELECT ?, id, 'base', CURRENT_TIMESTAMP FROM devices
			ON CONFLICT(map_id, device_id) DO NOTHING`
		copyLinksStatement := `INSERT INTO canvas_map_links (map_id, link_id, added_at)
			SELECT ?, id, CURRENT_TIMESTAMP FROM links
			ON CONFLICT(map_id, link_id) DO NOTHING`
		copyAreasStatement := `INSERT INTO canvas_map_areas (map_id, area_id, name, description, color, added_at)
			SELECT ?, id, name, description, color, CURRENT_TIMESTAMP FROM areas
			ON CONFLICT(map_id, area_id) DO NOTHING`

		if _, err := tx.Exec(copyDevicesStatement, mapID); err != nil {
			return fmt.Errorf("materializing default canvas map devices: %w", err)
		}
		if _, err := tx.Exec(copyLinksStatement, mapID); err != nil {
			return fmt.Errorf("materializing default canvas map links: %w", err)
		}
		if hasCanvasMapAreasTable && hasAreasTable {
			if _, err := tx.Exec(copyAreasStatement, mapID); err != nil {
				return fmt.Errorf("materializing default canvas map areas: %w", err)
			}
		}
		if _, err := tx.Exec(
			`UPDATE canvas_maps SET membership_materialized = ?, updated_at = ? WHERE id = ?`,
			true,
			time.Now().UTC(),
			mapID,
		); err != nil {
			return fmt.Errorf("marking default canvas map materialized: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing default canvas map migration: %w", err)
	}

	return nil
}

func migrateCanvasMapMemberships(db *sql.DB) error {
	wrapped := wrapDB(db)
	if ok, err := tableExists(wrapped, "canvas_maps"); err != nil {
		return fmt.Errorf("checking canvas_maps table: %w", err)
	} else if !ok {
		return nil
	}
	if ok, err := columnExists(wrapped, "canvas_maps", "membership_materialized"); err != nil {
		return fmt.Errorf("checking canvas_maps membership_materialized column: %w", err)
	} else if !ok {
		return nil
	}
	for _, tableName := range []string{"canvas_map_devices", "canvas_map_links", "canvas_map_areas", "devices", "links"} {
		ok, err := tableExists(wrapped, tableName)
		if err != nil {
			return fmt.Errorf("checking %s table: %w", tableName, err)
		}
		if !ok {
			return nil
		}
	}

	canvasMaps, err := unmaterializedCanvasMapsForMigration(wrapped)
	if err != nil {
		return err
	}
	if len(canvasMaps) == 0 {
		return nil
	}
	devices, err := canvasMapMigrationDevices(wrapped)
	if err != nil {
		return err
	}
	links, err := canvasMapMigrationLinks(wrapped)
	if err != nil {
		return err
	}
	areas, err := canvasMapMigrationAreas(wrapped)
	if err != nil {
		return err
	}

	tx, err := wrapped.Begin()
	if err != nil {
		return fmt.Errorf("starting canvas map membership migration transaction: %w", err)
	}
	defer tx.Rollback()

	for _, canvasMap := range canvasMaps {
		membership := materializeCanvasMapMigrationMembership(devices, links, areas, canvasMap.filter)
		for _, tableName := range []string{"canvas_map_device_areas", "canvas_map_devices", "canvas_map_links", "canvas_map_areas"} {
			if _, err := tx.Exec(
				"DELETE FROM "+tableName+" WHERE map_id = ?",
				canvasMap.id.String(),
			); err != nil {
				return fmt.Errorf("clearing %s for canvas map %s: %w", tableName, canvasMap.id, err)
			}
		}

		now := time.Now().UTC()
		for _, device := range membership.Devices {
			if _, err := tx.Exec(
				`INSERT INTO canvas_map_devices (map_id, device_id, role, added_at)
				 VALUES (?, ?, ?, ?)`,
				canvasMap.id.String(),
				device.DeviceID.String(),
				string(device.Role),
				now,
			); err != nil {
				return fmt.Errorf("inserting migrated canvas map device %s for map %s: %w", device.DeviceID, canvasMap.id, err)
			}
		}
		for _, linkID := range membership.LinkIDs {
			if _, err := tx.Exec(
				`INSERT INTO canvas_map_links (map_id, link_id, added_at)
				 VALUES (?, ?, ?)`,
				canvasMap.id.String(),
				linkID.String(),
				now,
			); err != nil {
				return fmt.Errorf("inserting migrated canvas map link %s for map %s: %w", linkID, canvasMap.id, err)
			}
		}
		for _, area := range membership.Areas {
			if _, err := tx.Exec(
				`INSERT INTO canvas_map_areas (map_id, area_id, name, description, color, added_at)
				 VALUES (?, ?, ?, ?, ?, ?)`,
				canvasMap.id.String(),
				area.AreaID.String(),
				area.Name,
				area.Description,
				area.Color,
				now,
			); err != nil {
				return fmt.Errorf("inserting migrated canvas map area %s for map %s: %w", area.AreaID, canvasMap.id, err)
			}
		}
		if err := insertCanvasMapDeviceAreas(tx, canvasMap.id, membership.Devices, now); err != nil {
			return err
		}
		if err := pruneCanvasMapPositionsForMembership(tx, canvasMap.id, membership.Devices); err != nil {
			return err
		}
		if _, err := tx.Exec(
			`UPDATE canvas_maps
			 SET membership_materialized = ?, updated_at = ?
			 WHERE id = ?`,
			true,
			now,
			canvasMap.id.String(),
		); err != nil {
			return fmt.Errorf("marking migrated canvas map %s materialized: %w", canvasMap.id, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing canvas map membership migration: %w", err)
	}
	return nil
}

func migrateCanvasMapDeviceAreaMemberships(db *sql.DB) error {
	wrapped := wrapDB(db)
	for _, tableName := range []string{"canvas_map_device_areas", "canvas_map_devices", "canvas_map_areas", "device_areas"} {
		ok, err := tableExists(wrapped, tableName)
		if err != nil {
			return fmt.Errorf("checking %s table: %w", tableName, err)
		}
		if !ok {
			return nil
		}
	}
	if _, err := wrapped.Exec(
		`INSERT INTO canvas_map_device_areas (map_id, device_id, area_id, assigned_at)
		 SELECT cmd.map_id, cmd.device_id, da.area_id, ?
		 FROM canvas_map_devices cmd
		 JOIN device_areas da ON da.device_id = cmd.device_id
		 JOIN canvas_map_areas cma ON cma.map_id = cmd.map_id AND cma.area_id = da.area_id
		 WHERE cmd.role = ?
		 ON CONFLICT(map_id, device_id, area_id) DO NOTHING`,
		time.Now().UTC(),
		string(domain.CanvasMapDeviceRoleBase),
	); err != nil {
		return fmt.Errorf("backfilling canvas map device area memberships: %w", err)
	}
	return nil
}

type canvasMapForMembershipMigration struct {
	id     uuid.UUID
	filter domain.CanvasMapFilter
}

func unmaterializedCanvasMapsForMigration(db *DB) ([]canvasMapForMembershipMigration, error) {
	rows, err := db.Query(
		`SELECT id, source_area_id, filter_json
		 FROM canvas_maps
		 WHERE membership_materialized = ?
		 ORDER BY id`,
		false,
	)
	if err != nil {
		return nil, fmt.Errorf("querying unmaterialized canvas maps: %w", err)
	}
	defer rows.Close()

	canvasMaps := []canvasMapForMembershipMigration{}
	for rows.Next() {
		var idRaw string
		var sourceAreaIDRaw sql.NullString
		var filterJSON string
		if err := rows.Scan(&idRaw, &sourceAreaIDRaw, &filterJSON); err != nil {
			return nil, fmt.Errorf("scanning unmaterialized canvas map: %w", err)
		}
		id, err := uuid.Parse(idRaw)
		if err != nil {
			return nil, fmt.Errorf("parsing canvas map id %q: %w", idRaw, err)
		}
		filter, err := domain.ParseCanvasMapFilter(filterJSON)
		if err != nil {
			filter, _ = domain.ParseCanvasMapFilter("")
		}
		if sourceAreaIDRaw.Valid && filter.AreaID == nil {
			sourceAreaID, err := uuid.Parse(sourceAreaIDRaw.String)
			if err != nil {
				return nil, fmt.Errorf("parsing canvas map source area id %q: %w", sourceAreaIDRaw.String, err)
			}
			filter.AreaID = &sourceAreaID
		}
		canvasMaps = append(canvasMaps, canvasMapForMembershipMigration{id: id, filter: filter})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating unmaterialized canvas maps: %w", err)
	}
	return canvasMaps, nil
}

func canvasMapMigrationDevices(db *DB) ([]domain.Device, error) {
	rows, err := db.Query(`SELECT id, tags_json FROM devices ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("querying canvas map migration devices: %w", err)
	}
	defer rows.Close()

	devices := []domain.Device{}
	for rows.Next() {
		var device domain.Device
		var idRaw string
		var tagsJSON string
		if err := rows.Scan(&idRaw, &tagsJSON); err != nil {
			return nil, fmt.Errorf("scanning canvas map migration device: %w", err)
		}
		id, err := uuid.Parse(idRaw)
		if err != nil {
			return nil, fmt.Errorf("parsing canvas map migration device id %q: %w", idRaw, err)
		}
		device.ID = id
		if err := json.Unmarshal([]byte(tagsJSON), &device.Tags); err != nil {
			return nil, fmt.Errorf("unmarshaling canvas map migration device tags for %s: %w", id, err)
		}
		if device.Tags == nil {
			device.Tags = map[string]string{}
		}
		devices = append(devices, device)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating canvas map migration devices: %w", err)
	}

	areaIDs, err := canvasMapMigrationDeviceAreaIDs(db)
	if err != nil {
		return nil, err
	}
	for i := range devices {
		devices[i].AreaIDs = areaIDs[devices[i].ID]
	}
	return devices, nil
}

func canvasMapMigrationDeviceAreaIDs(db *DB) (map[uuid.UUID][]uuid.UUID, error) {
	ok, err := tableExists(db, "device_areas")
	if err != nil {
		return nil, fmt.Errorf("checking device_areas table: %w", err)
	}
	if !ok {
		return map[uuid.UUID][]uuid.UUID{}, nil
	}

	rows, err := db.Query(`SELECT device_id, area_id FROM device_areas ORDER BY device_id, area_id`)
	if err != nil {
		return nil, fmt.Errorf("querying canvas map migration device areas: %w", err)
	}
	defer rows.Close()

	areaIDs := make(map[uuid.UUID][]uuid.UUID)
	for rows.Next() {
		var deviceIDRaw, areaIDRaw string
		if err := rows.Scan(&deviceIDRaw, &areaIDRaw); err != nil {
			return nil, fmt.Errorf("scanning canvas map migration device area: %w", err)
		}
		deviceID, err := uuid.Parse(deviceIDRaw)
		if err != nil {
			return nil, fmt.Errorf("parsing canvas map migration device area device id %q: %w", deviceIDRaw, err)
		}
		areaID, err := uuid.Parse(areaIDRaw)
		if err != nil {
			return nil, fmt.Errorf("parsing canvas map migration device area id %q: %w", areaIDRaw, err)
		}
		areaIDs[deviceID] = append(areaIDs[deviceID], areaID)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating canvas map migration device areas: %w", err)
	}
	return areaIDs, nil
}

func canvasMapMigrationLinks(db *DB) ([]domain.Link, error) {
	rows, err := db.Query(`SELECT id, source_device_id, target_device_id FROM links ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("querying canvas map migration links: %w", err)
	}
	defer rows.Close()

	links := []domain.Link{}
	for rows.Next() {
		var link domain.Link
		var idRaw, sourceIDRaw, targetIDRaw string
		if err := rows.Scan(&idRaw, &sourceIDRaw, &targetIDRaw); err != nil {
			return nil, fmt.Errorf("scanning canvas map migration link: %w", err)
		}
		id, err := uuid.Parse(idRaw)
		if err != nil {
			return nil, fmt.Errorf("parsing canvas map migration link id %q: %w", idRaw, err)
		}
		sourceID, err := uuid.Parse(sourceIDRaw)
		if err != nil {
			return nil, fmt.Errorf("parsing canvas map migration source device id %q: %w", sourceIDRaw, err)
		}
		targetID, err := uuid.Parse(targetIDRaw)
		if err != nil {
			return nil, fmt.Errorf("parsing canvas map migration target device id %q: %w", targetIDRaw, err)
		}
		link.ID = id
		link.SourceDeviceID = sourceID
		link.TargetDeviceID = targetID
		links = append(links, link)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating canvas map migration links: %w", err)
	}
	return links, nil
}

func canvasMapMigrationAreas(db *DB) ([]domain.AreaWithCount, error) {
	ok, err := tableExists(db, "areas")
	if err != nil {
		return nil, fmt.Errorf("checking areas table: %w", err)
	}
	if !ok {
		return []domain.AreaWithCount{}, nil
	}

	rows, err := db.Query(`SELECT id, name, description, color FROM areas ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("querying canvas map migration areas: %w", err)
	}
	defer rows.Close()

	areas := []domain.AreaWithCount{}
	for rows.Next() {
		var area domain.Area
		var idRaw string
		if err := rows.Scan(&idRaw, &area.Name, &area.Description, &area.Color); err != nil {
			return nil, fmt.Errorf("scanning canvas map migration area: %w", err)
		}
		id, err := uuid.Parse(idRaw)
		if err != nil {
			return nil, fmt.Errorf("parsing canvas map migration area id %q: %w", idRaw, err)
		}
		area.ID = id
		areas = append(areas, domain.AreaWithCount{Area: area})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating canvas map migration areas: %w", err)
	}
	return areas, nil
}

func materializeCanvasMapMigrationMembership(
	devices []domain.Device,
	links []domain.Link,
	areas []domain.AreaWithCount,
	filter domain.CanvasMapFilter,
) domain.CanvasMapMembership {
	projection := projectCanvasMapMigrationTopology(devices, links, filter)
	areaMemberships := canvasMapMigrationAreasForMembership(areas, projection.Devices, filter)
	includedAreaIDs := canvasMapMigrationAreaMembershipIDSet(areaMemberships)
	membership := domain.CanvasMapMembership{
		Devices: make([]domain.CanvasMapDeviceMembership, 0, len(projection.Devices)+len(projection.GhostDevices)),
		LinkIDs: make([]uuid.UUID, 0, len(projection.Links)),
		Areas:   make([]domain.CanvasMapAreaMembership, 0, len(areas)),
	}
	for _, device := range projection.Devices {
		membership.Devices = append(membership.Devices, domain.CanvasMapDeviceMembership{
			DeviceID: device.ID,
			Role:     domain.CanvasMapDeviceRoleBase,
			AreaIDs:  canvasMapMigrationFilterDeviceAreaIDs(device.AreaIDs, includedAreaIDs),
		})
	}
	for _, device := range projection.GhostDevices {
		membership.Devices = append(membership.Devices, domain.CanvasMapDeviceMembership{
			DeviceID: device.ID,
			Role:     domain.CanvasMapDeviceRoleGhost,
		})
	}
	for _, link := range projection.Links {
		membership.LinkIDs = append(membership.LinkIDs, link.ID)
	}
	for _, area := range areaMemberships {
		membership.Areas = append(membership.Areas, domain.CanvasMapAreaMembership{
			AreaID:      area.ID,
			Name:        area.Name,
			Description: area.Description,
			Color:       area.Color,
		})
	}
	return membership
}

func canvasMapMigrationAreaMembershipIDSet(areas []domain.AreaWithCount) map[uuid.UUID]struct{} {
	ids := make(map[uuid.UUID]struct{}, len(areas))
	for _, area := range areas {
		ids[area.ID] = struct{}{}
	}
	return ids
}

func canvasMapMigrationFilterDeviceAreaIDs(areaIDs []uuid.UUID, included map[uuid.UUID]struct{}) []uuid.UUID {
	if len(areaIDs) == 0 || len(included) == 0 {
		return []uuid.UUID{}
	}
	filtered := make([]uuid.UUID, 0, len(areaIDs))
	for _, areaID := range areaIDs {
		if _, ok := included[areaID]; ok {
			filtered = append(filtered, areaID)
		}
	}
	return filtered
}

type canvasMapMigrationProjection struct {
	Devices      []domain.Device
	Links        []domain.Link
	GhostDevices []domain.Device
}

func projectCanvasMapMigrationTopology(
	devices []domain.Device,
	links []domain.Link,
	filter domain.CanvasMapFilter,
) canvasMapMigrationProjection {
	knownDeviceIDs := make(map[uuid.UUID]struct{}, len(devices))
	baseDeviceIDs := make(map[uuid.UUID]struct{}, len(devices))
	selectedDeviceIDs := make(map[uuid.UUID]struct{}, len(filter.DeviceIDs))
	for _, deviceID := range filter.DeviceIDs {
		selectedDeviceIDs[deviceID] = struct{}{}
	}

	projection := canvasMapMigrationProjection{
		Devices: []domain.Device{},
		Links:   []domain.Link{},
	}
	for _, device := range devices {
		knownDeviceIDs[device.ID] = struct{}{}

		baseDevice := false
		switch {
		case len(selectedDeviceIDs) > 0:
			_, baseDevice = selectedDeviceIDs[device.ID]
		case filter.AreaID != nil:
			baseDevice = canvasMapMigrationDeviceHasArea(device, *filter.AreaID)
		default:
			baseDevice = true
		}
		if !baseDevice || !canvasMapMigrationDeviceMatchesTags(device, filter.Tags) {
			continue
		}

		projection.Devices = append(projection.Devices, device)
		baseDeviceIDs[device.ID] = struct{}{}
	}

	ghostDeviceIDs := make(map[uuid.UUID]struct{})
	for _, link := range links {
		_, sourceKnown := knownDeviceIDs[link.SourceDeviceID]
		_, targetKnown := knownDeviceIDs[link.TargetDeviceID]
		if !sourceKnown || !targetKnown {
			continue
		}

		_, sourceIsBase := baseDeviceIDs[link.SourceDeviceID]
		_, targetIsBase := baseDeviceIDs[link.TargetDeviceID]
		includeLink := sourceIsBase && targetIsBase
		if filter.IncludeCrossAreaLinks && (sourceIsBase || targetIsBase) {
			includeLink = true
		}
		if !includeLink {
			continue
		}

		projection.Links = append(projection.Links, link)
		if !filter.IncludeGhostDevices {
			continue
		}
		if sourceIsBase && !targetIsBase {
			ghostDeviceIDs[link.TargetDeviceID] = struct{}{}
		}
		if targetIsBase && !sourceIsBase {
			ghostDeviceIDs[link.SourceDeviceID] = struct{}{}
		}
	}

	if filter.IncludeGhostDevices && len(ghostDeviceIDs) > 0 {
		projection.GhostDevices = make([]domain.Device, 0, len(ghostDeviceIDs))
		for _, device := range devices {
			if _, isBase := baseDeviceIDs[device.ID]; isBase {
				continue
			}
			if _, isGhost := ghostDeviceIDs[device.ID]; isGhost {
				projection.GhostDevices = append(projection.GhostDevices, device)
			}
		}
	}

	return projection
}

func canvasMapMigrationAreasForMembership(
	areas []domain.AreaWithCount,
	baseDevices []domain.Device,
	filter domain.CanvasMapFilter,
) []domain.AreaWithCount {
	includedAreaIDs := make(map[uuid.UUID]struct{})
	if filter.AreaID != nil {
		includedAreaIDs[*filter.AreaID] = struct{}{}
	} else {
		for _, device := range baseDevices {
			for _, areaID := range device.AreaIDs {
				includedAreaIDs[areaID] = struct{}{}
			}
		}
	}

	filtered := make([]domain.AreaWithCount, 0, len(includedAreaIDs))
	for _, area := range areas {
		if _, ok := includedAreaIDs[area.ID]; ok {
			filtered = append(filtered, area)
		}
	}
	return filtered
}

func canvasMapMigrationDeviceHasArea(device domain.Device, areaID uuid.UUID) bool {
	for _, deviceAreaID := range device.AreaIDs {
		if deviceAreaID == areaID {
			return true
		}
	}
	return false
}

func canvasMapMigrationDeviceMatchesTags(device domain.Device, tags map[string]string) bool {
	for key, expected := range tags {
		actual, ok := device.Tags[key]
		if !ok || actual != expected {
			return false
		}
	}
	return true
}

func columnExists(db *DB, tableName string, columnName string) (bool, error) {
	var count int
	err := db.QueryRow(
		`SELECT COUNT(*)
		 FROM information_schema.columns
		 WHERE table_name = ? AND column_name = ?`,
		tableName,
		columnName,
	).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func tableExists(db *DB, tableName string) (bool, error) {
	var count int
	err := db.QueryRow(
		`SELECT COUNT(*) FROM pg_catalog.pg_class c
			WHERE c.relkind IN ('r', 'p')
				AND c.relname = ?
				AND pg_catalog.pg_table_is_visible(c.oid)`,
		tableName,
	).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// seedDefaultSettings inserts default settings without overwriting existing values.
func seedDefaultSettings(db *sql.DB) error {
	queryDB := wrapDB(db)
	now := time.Now().UTC()
	statement := `INSERT INTO settings (key, value, updated_at) VALUES (?, ?, ?)
		ON CONFLICT(key) DO NOTHING`
	for key, value := range domain.DefaultSettings() {
		_, err := queryDB.Exec(statement, key, value, now)
		if err != nil {
			return fmt.Errorf("seeding default setting %q: %w", key, err)
		}
	}
	return nil
}

func seedAuthSystemRolesAndPermissions(db *sql.DB) error {
	queryDB := wrapDB(db)
	now := time.Now().UTC()

	roleIDs := make(map[string]string, len(domain.SystemRoleNames()))
	roleExisted := make(map[string]bool, len(domain.SystemRoleNames()))
	for _, roleName := range domain.SystemRoleNames() {
		description := authSystemRoleDescription(roleName)
		var existed bool
		if err := queryDB.QueryRow(
			`SELECT EXISTS (SELECT 1 FROM roles WHERE name = ?)`,
			roleName,
		).Scan(&existed); err != nil {
			return fmt.Errorf("checking auth system role %q existence: %w", roleName, err)
		}
		roleExisted[roleName] = existed

		var roleID string
		if err := queryDB.QueryRow(
			`INSERT INTO roles (id, name, description, is_system_role, created_at, updated_at)
			 VALUES (?, ?, ?, ?, ?, ?)
			 ON CONFLICT(name) DO UPDATE SET
				description = EXCLUDED.description,
				is_system_role = TRUE,
				updated_at = EXCLUDED.updated_at
			 RETURNING id`,
			roleName,
			roleName,
			description,
			true,
			now,
			now,
		).Scan(&roleID); err != nil {
			return fmt.Errorf("seeding auth system role %q: %w", roleName, err)
		}
		roleIDs[roleName] = roleID
	}

	permissionIDs := make(map[string]string, len(domain.SystemPermissions()))
	for _, permission := range domain.SystemPermissions() {
		var permissionID string
		if err := queryDB.QueryRow(
			`INSERT INTO permissions (id, key, description, resource, action)
			 VALUES (?, ?, ?, ?, ?)
			 ON CONFLICT(key) DO UPDATE SET
				description = EXCLUDED.description,
				resource = EXCLUDED.resource,
				action = EXCLUDED.action
			 RETURNING id`,
			permission.Key,
			permission.Key,
			permission.Description,
			permission.Resource,
			permission.Action,
		).Scan(&permissionID); err != nil {
			return fmt.Errorf("seeding auth system permission %q: %w", permission.Key, err)
		}
		permissionIDs[permission.Key] = permissionID
	}

	for _, roleName := range domain.SystemRoleNames() {
		roleID := roleIDs[roleName]
		permissionKeys := domain.SystemRolePermissionKeys(roleName)
		if roleName != domain.RoleSuperAdmin && roleExisted[roleName] {
			continue
		}
		for _, permissionKey := range permissionKeys {
			permissionID, ok := permissionIDs[permissionKey]
			if !ok {
				return fmt.Errorf("seeding auth role %q: unknown permission %q", roleName, permissionKey)
			}
			if _, err := queryDB.Exec(
				`INSERT INTO role_permissions (role_id, permission_id)
				 VALUES (?, ?)
				 ON CONFLICT(role_id, permission_id) DO NOTHING`,
				roleID,
				permissionID,
			); err != nil {
				return fmt.Errorf("seeding auth role %q permission %q: %w", roleName, permissionKey, err)
			}
		}
	}

	return nil
}

func authSystemRoleDescription(roleName string) string {
	switch roleName {
	case domain.RoleSuperAdmin:
		return "Full system administration access"
	case domain.RoleAdmin:
		return "Administrative access without destructive or credential reveal permissions"
	case domain.RoleManager:
		return "Operational management access"
	case domain.RoleUser:
		return "Standard operator access"
	case domain.RoleViewer:
		return "Read-only access"
	default:
		return ""
	}
}
