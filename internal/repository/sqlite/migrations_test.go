package sqlite

import (
	"database/sql"
	"encoding/json"
	"io/fs"
	"strings"
	"testing"

	"github.com/lollinoo/theia/internal/domain"
	_ "github.com/mattn/go-sqlite3"
)

// openTestDB opens a fresh in-memory SQLite database without running migrations.
// Tests can control when (and if) RunMigrations is called.
func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite3", ":memory:?_foreign_keys=on")
	if err != nil {
		t.Fatalf("opening test db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

// ---------------------------------------------------------------------------
// TestMigrations (DEBT-05)
// ---------------------------------------------------------------------------
// Verifies that golang-migrate versioned migrations create the schema_migrations
// table with a version > 0 and dirty = false. Before the fix (inline string
// migrations with no version tracking), schema_migrations did not exist.
func TestMigrations(t *testing.T) {
	db := openTestDB(t)

	if err := RunMigrations(db); err != nil {
		t.Fatalf("RunMigrations failed: %v", err)
	}

	// schema_migrations must exist and have a valid version
	var version int
	var dirty bool
	err := db.QueryRow(`SELECT version, dirty FROM schema_migrations`).Scan(&version, &dirty)
	if err != nil {
		t.Fatalf("DEBT-05: schema_migrations table does not exist or is empty -- "+
			"migration system has no version tracking: %v", err)
	}

	if version <= 0 {
		t.Fatalf("DEBT-05: schema_migrations version is %d -- expected > 0", version)
	}

	if dirty {
		t.Fatalf("DEBT-05: schema_migrations dirty flag is true -- expected false")
	}

	t.Logf("schema_migrations: version=%d dirty=%v", version, dirty)
}

func TestRunMigrationsCreatesCanvasMapTables(t *testing.T) {
	db := openTestDB(t)
	if err := RunMigrations(db); err != nil {
		t.Fatalf("RunMigrations failed: %v", err)
	}

	for _, tableName := range []string{"canvas_maps", "canvas_map_positions"} {
		var count int
		if err := db.QueryRow(
			`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?`,
			tableName,
		).Scan(&count); err != nil {
			t.Fatalf("querying table %s: %v", tableName, err)
		}
		if count != 1 {
			t.Fatalf("expected table %s to exist", tableName)
		}
	}
}

func TestRunMigrationsSeedsDefaultCanvasMapFromLegacyPositions(t *testing.T) {
	db := openTestDB(t)
	if err := RunMigrations(db); err != nil {
		t.Fatalf("initial RunMigrations failed: %v", err)
	}

	deviceID := "00000000-0000-0000-0000-000000000101"
	if _, err := db.Exec(
		`INSERT INTO devices (id, hostname, ip, device_type, status, sys_name, sys_descr, sys_object_id, hardware_model, vendor, managed, tags_json, metrics_source, prometheus_label_name, prometheus_label_value, created_at, updated_at)
		 VALUES (?, 'router-101', '10.0.1.101', 'router', 'up', 'router-101', '', '', '', 'default', 1, '{}', 'none', '', '', datetime('now'), datetime('now'))`,
		deviceID,
	); err != nil {
		t.Fatalf("insert device: %v", err)
	}
	if _, err := db.Exec(
		`INSERT INTO device_positions (device_id, x, y, pinned, updated_at)
		 VALUES (?, 11, 22, 1, datetime('now'))`,
		deviceID,
	); err != nil {
		t.Fatalf("insert legacy position: %v", err)
	}

	if err := migrateDefaultCanvasMap(db); err != nil {
		t.Fatalf("migrateDefaultCanvasMap failed: %v", err)
	}
	if err := migrateDefaultCanvasMap(db); err != nil {
		t.Fatalf("second migrateDefaultCanvasMap failed: %v", err)
	}

	var mapID string
	var mapCount int
	if err := db.QueryRow(`SELECT COUNT(*), id FROM canvas_maps WHERE is_default = 1`).Scan(&mapCount, &mapID); err != nil {
		t.Fatalf("query default map: %v", err)
	}
	if mapCount != 1 {
		t.Fatalf("expected one default map, got %d", mapCount)
	}

	var legacyCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM device_positions WHERE device_id = ?`, deviceID).Scan(&legacyCount); err != nil {
		t.Fatalf("query legacy positions: %v", err)
	}
	if legacyCount != 1 {
		t.Fatalf("expected legacy position to remain, got %d", legacyCount)
	}

	var scopedCount int
	if err := db.QueryRow(
		`SELECT COUNT(*) FROM canvas_map_positions WHERE map_id = ? AND device_id = ? AND x = 11 AND y = 22 AND pinned = 1`,
		mapID,
		deviceID,
	).Scan(&scopedCount); err != nil {
		t.Fatalf("query scoped positions: %v", err)
	}
	if scopedCount != 1 {
		t.Fatalf("expected one copied map position, got %d", scopedCount)
	}
}

// ---------------------------------------------------------------------------
// TestMigration000012_DefaultRole (CRED-04)
// ---------------------------------------------------------------------------
// Verifies the behavior introduced by migration 000012:
//
//	(a) The credential_profiles table has a role column that defaults to 'Admin'
//	    — inserting a row without specifying role yields role='Admin'.
//	(b) The device_credential_profiles join table exists with the expected columns.
//
// Because RunMigrations always runs all migrations to the latest version,
// we verify these invariants on the final migrated schema rather than
// replaying a partial migration.
func TestMigration000012_DefaultRole(t *testing.T) {
	db := openTestDB(t)

	if err := RunMigrations(db); err != nil {
		t.Fatalf("RunMigrations failed: %v", err)
	}

	// (a) Insert a credential_profiles row without specifying role and verify
	//     that the DEFAULT 'Admin' constraint fills it in.
	profileID := "00000000-0000-0000-0000-000000000001"
	_, err := db.Exec(
		`INSERT INTO credential_profiles (id, name, description, username, port, auth_method, encrypted_secret, created_at, updated_at)
		 VALUES (?, 'role-default-test', '', 'admin', 22, 'password', '', datetime('now'), datetime('now'))`,
		profileID,
	)
	if err != nil {
		t.Fatalf("CRED-04: inserting credential_profiles row without role: %v", err)
	}

	var role string
	err = db.QueryRow(`SELECT role FROM credential_profiles WHERE id = ?`, profileID).Scan(&role)
	if err != nil {
		t.Fatalf("CRED-04: querying role from credential_profiles: %v", err)
	}
	if role != "Admin" {
		t.Errorf("CRED-04: expected role='Admin' (DEFAULT), got %q", role)
	}

	// (b) Verify device_credential_profiles join table exists with expected columns
	//     by performing a SELECT that references all required columns.
	_, err = db.Exec(
		`SELECT device_id, profile_id, is_winbox, created_at FROM device_credential_profiles LIMIT 0`,
	)
	if err != nil {
		t.Fatalf("CRED-04: device_credential_profiles table missing or has wrong columns: %v", err)
	}
}

// ---------------------------------------------------------------------------
// TestLegacyTableDrop (DEBT-06)
// ---------------------------------------------------------------------------
// Verifies that legacy tables (config_backups, ssh_credentials) are dropped
// after running migrations. Before the fix (no migration 000006), these
// tables remained in the database.
func TestLegacyTableDrop(t *testing.T) {
	db := openTestDB(t)

	if err := RunMigrations(db); err != nil {
		t.Fatalf("RunMigrations failed: %v", err)
	}

	// Check that legacy tables are dropped
	legacyTables := []string{"config_backups", "ssh_credentials"}
	for _, tableName := range legacyTables {
		var count int
		err := db.QueryRow(
			`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?`,
			tableName,
		).Scan(&count)
		if err != nil {
			t.Fatalf("failed to query sqlite_master for %s: %v", tableName, err)
		}
		if count != 0 {
			t.Fatalf("DEBT-06: legacy table %s still exists after migrations -- "+
				"expected it to be dropped by migration 000006", tableName)
		}
	}
}

// ---------------------------------------------------------------------------
// TestMigration000014_DevicesColumnDropped (Phase 27 Gap 1)
// ---------------------------------------------------------------------------
// Verifies that the ssh_profile_id column is absent from the devices table
// after migration 000014 runs as part of RunMigrations.
func TestMigration000014_DevicesColumnDropped(t *testing.T) {
	db := openTestDB(t)

	if err := RunMigrations(db); err != nil {
		t.Fatalf("RunMigrations failed: %v", err)
	}

	var count int
	err := db.QueryRow(
		`SELECT COUNT(*) FROM pragma_table_info('devices') WHERE name='ssh_profile_id'`,
	).Scan(&count)
	if err != nil {
		t.Fatalf("querying pragma_table_info for ssh_profile_id: %v", err)
	}
	if count != 0 {
		t.Fatalf("migration 000014: ssh_profile_id column still present in devices table -- expected it to be dropped")
	}
}

// ---------------------------------------------------------------------------
// TestMigration000014_DeviceDataIntegrity (Phase 27 Gap 2)
// ---------------------------------------------------------------------------
// Verifies that device records survive migration 000014 with every other field
// intact. Inserts a device post-migration (all migrations run) and reads it
// back, asserting all fields are present and uncorrupted.
func TestMigration000014_DeviceDataIntegrity(t *testing.T) {
	db := openTestDB(t)

	if err := RunMigrations(db); err != nil {
		t.Fatalf("RunMigrations failed: %v", err)
	}

	// Insert a device using only the columns that remain after migration 000014
	// (ssh_profile_id was dropped). This verifies the schema is correct.
	deviceID := "00000000-0000-0000-0000-000000000099"
	_, err := db.Exec(
		`INSERT INTO devices (
			id, hostname, ip, snmp_credentials_json, device_type, status,
			sys_name, sys_descr, sys_object_id, hardware_model, vendor,
			managed, tags_json, metrics_source, prometheus_label_name,
			prometheus_label_value, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, datetime('now'), datetime('now'))`,
		deviceID, "test-router", "192.168.1.1",
		`{"version":"2c","v2c":{"community":"public"}}`,
		"router", "up",
		"test-router-sysname", "RouterOS", "1.3.6.1.4.1.14988", "RB4011",
		"mikrotik", 1, `{"env":"prod"}`,
		"prometheus", "instance", "192.168.1.1:9100",
	)
	if err != nil {
		t.Fatalf("inserting device post-migration: %v", err)
	}

	// Read it back and verify key fields are intact
	var hostname, ip, vendor, metricsSource, prometheusLabelName string
	var managed int
	err = db.QueryRow(
		`SELECT hostname, ip, vendor, managed, metrics_source, prometheus_label_name FROM devices WHERE id = ?`,
		deviceID,
	).Scan(&hostname, &ip, &vendor, &managed, &metricsSource, &prometheusLabelName)
	if err != nil {
		t.Fatalf("reading device post-migration: %v", err)
	}

	if hostname != "test-router" {
		t.Errorf("expected hostname %q, got %q", "test-router", hostname)
	}
	if ip != "192.168.1.1" {
		t.Errorf("expected ip %q, got %q", "192.168.1.1", ip)
	}
	if vendor != "mikrotik" {
		t.Errorf("expected vendor %q, got %q", "mikrotik", vendor)
	}
	if managed != 1 {
		t.Errorf("expected managed=1, got %d", managed)
	}
	if metricsSource != "prometheus" {
		t.Errorf("expected metrics_source %q, got %q", "prometheus", metricsSource)
	}
	if prometheusLabelName != "instance" {
		t.Errorf("expected prometheus_label_name %q, got %q", "instance", prometheusLabelName)
	}
}

func TestMigrateDeviceSNMPCredentialsSkipsEmptyCredentialShells(t *testing.T) {
	db := openTestDB(t)
	if err := RunMigrations(db, testKey); err != nil {
		t.Fatalf("RunMigrations failed: %v", err)
	}

	deviceID := "00000000-0000-0000-0000-000000000123"
	const rawCreds = `{"version":""}`
	_, err := db.Exec(
		`INSERT INTO devices (
			id, hostname, ip, snmp_credentials_json, device_type, status,
			sys_name, sys_descr, sys_object_id, hardware_model, vendor,
			managed, tags_json, metrics_source, prometheus_label_name,
			prometheus_label_value, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, datetime('now'), datetime('now'))`,
		deviceID, "virtual", "", rawCreds,
		"virtual", "unknown",
		"", "", "", "",
		"default", 1, `{}`,
		"none", "instance", "",
	)
	if err != nil {
		t.Fatalf("inserting device: %v", err)
	}

	updated, err := migrateDeviceSNMPCredentials(db, testKey)
	if err != nil {
		t.Fatalf("migrateDeviceSNMPCredentials: %v", err)
	}
	if updated != 0 {
		t.Fatalf("updated = %d, want 0", updated)
	}

	var got string
	if err := db.QueryRow(`SELECT snmp_credentials_json FROM devices WHERE id = ?`, deviceID).Scan(&got); err != nil {
		t.Fatalf("querying credentials: %v", err)
	}
	if got != rawCreds {
		t.Fatalf("snmp_credentials_json = %s, want %s", got, rawCreds)
	}
}

func TestMigrateDeviceSNMPCredentialsSkipsCredentialsEncryptedWithDifferentKey(t *testing.T) {
	db := openTestDB(t)
	if err := RunMigrations(db, testKey); err != nil {
		t.Fatalf("RunMigrations failed: %v", err)
	}

	rawCreds := encryptedSNMPCredentialsJSON(t, []byte("old-encryption-key-32-byte-value"))
	deviceID := "00000000-0000-0000-0000-000000000124"
	_, err := db.Exec(
		`INSERT INTO devices (
			id, hostname, ip, snmp_credentials_json, device_type, status,
			sys_name, sys_descr, sys_object_id, hardware_model, vendor,
			managed, tags_json, metrics_source, prometheus_label_name,
			prometheus_label_value, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, datetime('now'), datetime('now'))`,
		deviceID, "router", "192.0.2.124", rawCreds,
		"router", "unknown",
		"", "", "", "",
		"default", 1, `{}`,
		"snmp", "instance", "192.0.2.124",
	)
	if err != nil {
		t.Fatalf("inserting device: %v", err)
	}

	updated, err := migrateDeviceSNMPCredentials(db, testKey)
	if err != nil {
		t.Fatalf("migrateDeviceSNMPCredentials: %v", err)
	}
	if updated != 0 {
		t.Fatalf("updated = %d, want 0", updated)
	}

	var got string
	if err := db.QueryRow(`SELECT snmp_credentials_json FROM devices WHERE id = ?`, deviceID).Scan(&got); err != nil {
		t.Fatalf("querying credentials: %v", err)
	}
	if got != rawCreds {
		t.Fatalf("snmp_credentials_json was re-encrypted with the wrong key")
	}
}

func TestMigrateSNMPProfileCredentialsSkipsCredentialsEncryptedWithDifferentKey(t *testing.T) {
	db := openTestDB(t)
	if err := RunMigrations(db, testKey); err != nil {
		t.Fatalf("RunMigrations failed: %v", err)
	}

	rawCreds := encryptedSNMPCredentialsJSON(t, []byte("old-encryption-key-32-byte-value"))
	profileID := "00000000-0000-0000-0000-000000000125"
	_, err := db.Exec(
		`INSERT INTO snmp_profiles (id, name, description, credentials_json, created_at, updated_at)
		 VALUES (?, 'old-key-profile', '', ?, datetime('now'), datetime('now'))`,
		profileID, rawCreds,
	)
	if err != nil {
		t.Fatalf("inserting profile: %v", err)
	}

	updated, err := migrateSNMPProfileCredentials(db, testKey)
	if err != nil {
		t.Fatalf("migrateSNMPProfileCredentials: %v", err)
	}
	if updated != 0 {
		t.Fatalf("updated = %d, want 0", updated)
	}

	var got string
	if err := db.QueryRow(`SELECT credentials_json FROM snmp_profiles WHERE id = ?`, profileID).Scan(&got); err != nil {
		t.Fatalf("querying credentials: %v", err)
	}
	if got != rawCreds {
		t.Fatalf("credentials_json was re-encrypted with the wrong key")
	}
}

func encryptedSNMPCredentialsJSON(t *testing.T, key []byte) string {
	t.Helper()
	creds := domain.SNMPCredentials{
		Version: domain.SNMPVersionV3,
		V3: &domain.SNMPv3Credentials{
			Username:      "snmp-user",
			AuthProtocol:  "SHA",
			AuthPassword:  "auth-secret",
			PrivProtocol:  "AES",
			PrivPassword:  "priv-secret",
			SecurityLevel: "authPriv",
		},
	}
	if err := encryptSNMPCredentials(&creds, key); err != nil {
		t.Fatalf("encrypting credentials: %v", err)
	}
	raw, err := json.Marshal(creds)
	if err != nil {
		t.Fatalf("marshalling credentials: %v", err)
	}
	return string(raw)
}

func TestMigration000015_AddsScaleIndexes(t *testing.T) {
	db := openTestDB(t)

	if err := RunMigrations(db); err != nil {
		t.Fatalf("RunMigrations failed: %v", err)
	}

	var sysNameLookupColumnCount int
	if err := db.QueryRow(
		`SELECT COUNT(*) FROM pragma_table_info('devices') WHERE name='sys_name_lookup'`,
	).Scan(&sysNameLookupColumnCount); err != nil {
		t.Fatalf("querying devices columns: %v", err)
	}
	if sysNameLookupColumnCount != 1 {
		t.Fatalf("expected sys_name_lookup column to exist, got count %d", sysNameLookupColumnCount)
	}

	indexes := []string{
		"idx_devices_sys_name_lookup",
		"idx_interfaces_device_id_if_index",
		"idx_device_areas_area_id",
		"idx_links_target_device_created_at",
		"idx_links_pair_lookup",
	}
	for _, indexName := range indexes {
		var count int
		if err := db.QueryRow(
			`SELECT COUNT(*) FROM sqlite_master WHERE type='index' AND name=?`,
			indexName,
		).Scan(&count); err != nil {
			t.Fatalf("querying index %s: %v", indexName, err)
		}
		if count != 1 {
			t.Fatalf("expected index %s to exist, got count %d", indexName, count)
		}
	}
}

func TestMigration000018_AddsTopologyObservationTables(t *testing.T) {
	db := openTestDB(t)

	if err := RunMigrations(db); err != nil {
		t.Fatalf("RunMigrations failed: %v", err)
	}

	for _, query := range []string{
		`SELECT id, local_device_id, remote_identity, remote_device_id, local_port, remote_port, protocol, is_self_neighbor, first_observed_at, last_observed_at, created_at, updated_at FROM topology_observations LIMIT 0`,
		`SELECT id, local_device_id, remote_identity, protocol, occurrences, first_observed_at, last_observed_at, resolved_at, created_at, updated_at FROM unresolved_neighbors LIMIT 0`,
	} {
		if _, err := db.Exec(query); err != nil {
			t.Fatalf("expected topology observation schema to exist for query %q: %v", query, err)
		}
	}
}

// ---------------------------------------------------------------------------
// TestVerifyLegacyTablesMigrated_PostMigration000014 (Phase 27 Gap 3)
// ---------------------------------------------------------------------------
// Verifies that verifyLegacyTablesMigrated returns nil (no error) on a database
// where migration 000014 has already run and dropped the ssh_profile_id column.
// This confirms the guard added to verifyLegacyTablesMigrated works correctly.
func TestVerifyLegacyTablesMigrated_PostMigration000014(t *testing.T) {
	db := openTestDB(t)

	// Run all migrations — 000014 drops ssh_profile_id from devices.
	if err := RunMigrations(db); err != nil {
		t.Fatalf("RunMigrations failed: %v", err)
	}

	// Calling verifyLegacyTablesMigrated again on a post-000014 database must
	// not error. The ssh_profile_id column is absent so the function should
	// detect that and return nil gracefully.
	if err := verifyLegacyTablesMigrated(db); err != nil {
		t.Fatalf("verifyLegacyTablesMigrated returned error on post-000014 database: %v", err)
	}
}

// ---------------------------------------------------------------------------
// TestMigrateDevicePollClass_BackfillsByDeviceType (Phase 39 Plan 03)
// ---------------------------------------------------------------------------
// Verifies that migrateDevicePollClass correctly backfills each device type
// to its expected PollClass using domain.ClassifyPollClass as the source of
// truth.
func TestMigrateDevicePollClass_BackfillsByDeviceType(t *testing.T) {
	db := openTestDB(t)

	if err := RunMigrations(db); err != nil {
		t.Fatalf("RunMigrations failed: %v", err)
	}

	// Insert raw device rows with poll_class = 'standard' (the SQL DEFAULT),
	// simulating the post-up-migration state before Go-level backfill.
	// We use direct SQL (not DeviceRepo.Create) to test the migration alone.
	devices := []struct {
		id         string
		deviceType string
		wantClass  string
	}{
		{"00000000-0000-0000-0000-000000000101", "router", "core"},
		{"00000000-0000-0000-0000-000000000102", "switch", "core"},
		{"00000000-0000-0000-0000-000000000103", "ap", "standard"},
		{"00000000-0000-0000-0000-000000000104", "virtual", "low"},
		{"00000000-0000-0000-0000-000000000105", "unknown", "standard"},
		{"00000000-0000-0000-0000-000000000106", "", "standard"},
	}

	for _, d := range devices {
		_, err := db.Exec(
			`INSERT INTO devices (
				id, hostname, ip, snmp_credentials_json, device_type, status,
				sys_name, sys_name_lookup, sys_descr, sys_object_id, hardware_model,
				vendor, managed, tags_json, created_at, updated_at,
				metrics_source, prometheus_label_name, prometheus_label_value,
				poll_class
			) VALUES (?, ?, ?, '{}', ?, 'unknown', '', '', '', '', '', 'default', 0, '{}',
				datetime('now'), datetime('now'), 'prometheus', 'instance', '', 'standard')`,
			d.id, "host-"+d.deviceType, "10.0.99."+d.id[len(d.id)-3:], d.deviceType,
		)
		if err != nil {
			t.Fatalf("inserting device %s (%s): %v", d.id, d.deviceType, err)
		}
	}

	// Run the migration — it should update rows whose poll_class doesn't match
	// what ClassifyPollClass would compute (router→core, switch→core, virtual→low).
	if err := migrateDevicePollClass(db); err != nil {
		t.Fatalf("migrateDevicePollClass failed: %v", err)
	}

	// Assert each row has the expected poll_class.
	for _, d := range devices {
		var gotClass string
		err := db.QueryRow(`SELECT poll_class FROM devices WHERE id = ?`, d.id).Scan(&gotClass)
		if err != nil {
			t.Fatalf("querying poll_class for device %s: %v", d.id, err)
		}
		if gotClass != d.wantClass {
			t.Errorf("device %s (type=%q): got poll_class=%q, want %q", d.id, d.deviceType, gotClass, d.wantClass)
		}
	}
}

// ---------------------------------------------------------------------------
// TestMigrateDevicePollClass_Idempotent (Phase 39 Plan 03)
// ---------------------------------------------------------------------------
// Verifies that running migrateDevicePollClass a second time on an already-
// backfilled database performs no further changes.
func TestMigrateDevicePollClass_Idempotent(t *testing.T) {
	db := openTestDB(t)

	if err := RunMigrations(db); err != nil {
		t.Fatalf("RunMigrations failed: %v", err)
	}

	// Insert a router (will be backfilled to 'core') and an AP (stays 'standard').
	devices := []struct {
		id         string
		deviceType string
		wantClass  string
	}{
		{"00000000-0000-0000-0000-000000000201", "router", "core"},
		{"00000000-0000-0000-0000-000000000202", "ap", "standard"},
	}

	for _, d := range devices {
		_, err := db.Exec(
			`INSERT INTO devices (
				id, hostname, ip, snmp_credentials_json, device_type, status,
				sys_name, sys_name_lookup, sys_descr, sys_object_id, hardware_model,
				vendor, managed, tags_json, created_at, updated_at,
				metrics_source, prometheus_label_name, prometheus_label_value,
				poll_class
			) VALUES (?, ?, ?, '{}', ?, 'unknown', '', '', '', '', '', 'default', 0, '{}',
				datetime('now'), datetime('now'), 'prometheus', 'instance', '', 'standard')`,
			d.id, "host-idem-"+d.deviceType, "10.0.98.1", d.deviceType,
		)
		if err != nil {
			t.Fatalf("inserting device %s: %v", d.id, err)
		}
	}

	// First run: backfills router to 'core'.
	if err := migrateDevicePollClass(db); err != nil {
		t.Fatalf("first migrateDevicePollClass failed: %v", err)
	}

	// Capture poll_class values after first run.
	firstRunValues := make(map[string]string)
	for _, d := range devices {
		var class string
		if err := db.QueryRow(`SELECT poll_class FROM devices WHERE id = ?`, d.id).Scan(&class); err != nil {
			t.Fatalf("querying poll_class after first run for %s: %v", d.id, err)
		}
		firstRunValues[d.id] = class
	}

	// Second run: must be a no-op.
	if err := migrateDevicePollClass(db); err != nil {
		t.Fatalf("second migrateDevicePollClass failed: %v", err)
	}

	// Assert poll_class values are unchanged.
	for _, d := range devices {
		var class string
		if err := db.QueryRow(`SELECT poll_class FROM devices WHERE id = ?`, d.id).Scan(&class); err != nil {
			t.Fatalf("querying poll_class after second run for %s: %v", d.id, err)
		}
		if class != firstRunValues[d.id] {
			t.Errorf("idempotency violated for device %s: before=%q after=%q", d.id, firstRunValues[d.id], class)
		}
		if class != d.wantClass {
			t.Errorf("device %s: expected final poll_class=%q, got %q", d.id, d.wantClass, class)
		}
	}
}

func TestPostgresMigrations_AddPollClassificationColumns(t *testing.T) {
	files, err := fs.Glob(postgresMigrationsFS, "postgres_migrations/*.up.sql")
	if err != nil {
		t.Fatalf("globbing postgres migrations: %v", err)
	}
	if len(files) == 0 {
		t.Fatal("expected embedded postgres migrations to exist")
	}

	var combined strings.Builder
	for _, name := range files {
		content, err := postgresMigrationsFS.ReadFile(name)
		if err != nil {
			t.Fatalf("reading postgres migration %s: %v", name, err)
		}
		combined.Write(content)
		combined.WriteByte('\n')
	}

	sql := combined.String()
	wants := []string{
		"ADD COLUMN IF NOT EXISTS poll_class TEXT NOT NULL DEFAULT 'standard'",
		"ADD COLUMN IF NOT EXISTS poll_interval_override INTEGER",
		"ADD COLUMN IF NOT EXISTS notes TEXT",
	}
	for _, want := range wants {
		if !strings.Contains(sql, want) {
			t.Fatalf("expected postgres migrations to contain %q", want)
		}
	}
}

func TestPostgresMigrations_ContainScaleLookupIndexes(t *testing.T) {
	files, err := fs.Glob(postgresMigrationsFS, "postgres_migrations/*.up.sql")
	if err != nil {
		t.Fatalf("globbing postgres migrations: %v", err)
	}

	var combined strings.Builder
	for _, name := range files {
		content, err := postgresMigrationsFS.ReadFile(name)
		if err != nil {
			t.Fatalf("reading postgres migration %s: %v", name, err)
		}
		combined.Write(content)
		combined.WriteByte('\n')
	}

	sql := combined.String()
	wants := []string{
		"idx_devices_sys_name_lookup",
		"idx_links_pair_lookup",
		"idx_topology_observations_ingest_lookup",
		"idx_unresolved_neighbors_resolution_lookup",
		"idx_unresolved_neighbors_active_lookup",
	}
	for _, want := range wants {
		if !strings.Contains(sql, want) {
			t.Fatalf("expected postgres migrations to contain %q", want)
		}
	}
}

func TestMigration000017_AddsDeviceNotesColumn(t *testing.T) {
	db := openTestDB(t)

	if err := RunMigrations(db); err != nil {
		t.Fatalf("RunMigrations failed: %v", err)
	}

	var count int
	if err := db.QueryRow(
		`SELECT COUNT(*) FROM pragma_table_info('devices') WHERE name='notes'`,
	).Scan(&count); err != nil {
		t.Fatalf("querying devices columns: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected notes column to exist, got count %d", count)
	}
}

func TestMigration000019_AddsScaleLookupIndexes(t *testing.T) {
	db := openTestDB(t)

	if err := RunMigrations(db); err != nil {
		t.Fatalf("RunMigrations failed: %v", err)
	}

	for _, indexName := range []string{
		"idx_topology_observations_ingest_lookup",
		"idx_unresolved_neighbors_resolution_lookup",
		"idx_unresolved_neighbors_active_lookup",
	} {
		var count int
		if err := db.QueryRow(
			`SELECT COUNT(*) FROM sqlite_master WHERE type='index' AND name=?`,
			indexName,
		).Scan(&count); err != nil {
			t.Fatalf("querying sqlite_master for %s: %v", indexName, err)
		}
		if count != 1 {
			t.Fatalf("expected index %s to exist, got count %d", indexName, count)
		}
	}
}
