package sqlite

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/base64"
	"errors"
	"strings"
	"sync"
	"testing"
)

func TestCopyPrimaryData_CopiesAndUpsertsRows(t *testing.T) {
	source := openTestDB(t)
	target := openTestDB(t)

	if err := RunMigrations(source, testKey); err != nil {
		t.Fatalf("RunMigrations(source) failed: %v", err)
	}
	if err := RunMigrations(target, testKey); err != nil {
		t.Fatalf("RunMigrations(target) failed: %v", err)
	}
	seedCopyTestSource(t, source)

	if err := CopyPrimaryData(source, target, CopyOptions{BatchSize: 2}); err != nil {
		t.Fatalf("CopyPrimaryData() failed: %v", err)
	}

	assertCopyTargetState(t, target, "core-router", "https://prom.example", true, "core", intPtr(45))

	if _, err := source.Exec(`UPDATE devices SET hostname = 'core-router-renamed' WHERE id = 'dev-1'`); err != nil {
		t.Fatalf("updating source device: %v", err)
	}
	if _, err := source.Exec(`UPDATE settings SET value = 'https://prom-2.example' WHERE key = 'prometheus_url'`); err != nil {
		t.Fatalf("updating source settings: %v", err)
	}
	if _, err := source.Exec(`UPDATE device_credential_profiles SET is_winbox = 0 WHERE device_id = 'dev-1' AND profile_id = 'cred-1'`); err != nil {
		t.Fatalf("updating source device_credential_profiles: %v", err)
	}

	if err := CopyPrimaryData(source, target, CopyOptions{BatchSize: 1}); err != nil {
		t.Fatalf("second CopyPrimaryData() failed: %v", err)
	}

	assertCopyTargetState(t, target, "core-router-renamed", "https://prom-2.example", false, "core", intPtr(45))
}

func TestCopyPrimaryData_TruncateTargetRemovesStaleRows(t *testing.T) {
	source := openTestDB(t)
	target := openTestDB(t)

	if err := RunMigrations(source, testKey); err != nil {
		t.Fatalf("RunMigrations(source) failed: %v", err)
	}
	if err := RunMigrations(target, testKey); err != nil {
		t.Fatalf("RunMigrations(target) failed: %v", err)
	}
	seedCopyTestSource(t, source)

	if _, err := target.Exec(
		`INSERT INTO devices (
			id, hostname, ip, snmp_credentials_json, device_type, status, sys_name, sys_descr,
			sys_object_id, hardware_model, vendor, managed, tags_json, created_at, updated_at,
			metrics_source, prometheus_label_name, prometheus_label_value, sys_name_lookup
		) VALUES (
			'stale-dev', 'stale-router', '198.51.100.10', '{}', 'router', 'down', '', '', '',
			'', 'default', 1, '{}', '2026-04-10 00:00:00', '2026-04-10 00:00:00',
			'prometheus', 'instance', '', ''
		)`,
	); err != nil {
		t.Fatalf("seeding stale target row: %v", err)
	}

	if err := CopyPrimaryData(source, target, CopyOptions{TruncateTarget: true, BatchSize: 2}); err != nil {
		t.Fatalf("CopyPrimaryData(truncate) failed: %v", err)
	}

	var staleCount int
	if err := target.QueryRow(`SELECT COUNT(*) FROM devices WHERE id = 'stale-dev'`).Scan(&staleCount); err != nil {
		t.Fatalf("querying stale target row: %v", err)
	}
	if staleCount != 0 {
		t.Fatalf("stale target row still present after truncate copy")
	}

	assertCopyTargetState(t, target, "core-router", "https://prom.example", true, "core", intPtr(45))
}

func TestDefaultCanvasMapAfterCopy_CopiesCopiedLegacyPositions(t *testing.T) {
	source := openTestDB(t)
	target := openTestDB(t)

	if err := RunMigrations(source, testKey); err != nil {
		t.Fatalf("RunMigrations(source) failed: %v", err)
	}
	if err := RunMigrations(target, testKey); err != nil {
		t.Fatalf("RunMigrations(target) failed: %v", err)
	}

	const sourceDefaultMapID = "00000000-0000-0000-0000-000000000901"
	const targetGeneratedMapID = "00000000-0000-0000-0000-000000000902"
	setDefaultCanvasMapID(t, source, sourceDefaultMapID)
	setDefaultCanvasMapID(t, target, targetGeneratedMapID)

	const deviceID = "00000000-0000-0000-0000-000000000301"
	if _, err := source.Exec(
		`INSERT INTO devices (
			id, hostname, ip, snmp_credentials_json, device_type, status, sys_name, sys_descr,
			sys_object_id, hardware_model, vendor, managed, tags_json, created_at, updated_at,
			metrics_source, prometheus_label_name, prometheus_label_value, sys_name_lookup,
			poll_class, poll_interval_override, polling_enabled, notes
		) VALUES (
			?, 'router-301', '10.0.3.1', '{}', 'router', 'up', 'router-301', '',
			'', 'CCR2004', 'mikrotik', 1, '{}', '2026-04-10 00:00:00',
			'2026-04-10 00:00:00', 'prometheus', 'instance', 'router-301:9100', 'router-301',
			'core', 60, 1, 'Canvas copy regression device'
		)`,
		deviceID,
	); err != nil {
		t.Fatalf("inserting source device: %v", err)
	}
	if _, err := source.Exec(
		`INSERT INTO device_positions (device_id, x, y, pinned, updated_at)
		 VALUES (?, 71.5, 82.25, 1, '2026-04-10 03:04:05')`,
		deviceID,
	); err != nil {
		t.Fatalf("inserting source legacy position: %v", err)
	}

	if err := CopyPrimaryData(source, target, CopyOptions{BatchSize: 2}); err != nil {
		t.Fatalf("CopyPrimaryData() failed: %v", err)
	}

	var preSeedCount int
	if err := target.QueryRow(`SELECT COUNT(*) FROM canvas_map_positions`).Scan(&preSeedCount); err != nil {
		t.Fatalf("counting pre-seed canvas_map_positions: %v", err)
	}
	if preSeedCount != 0 {
		t.Fatalf("pre-seed canvas_map_positions count = %d, want 0", preSeedCount)
	}

	if err := seedTargetDefaultCanvasMapAfterCopy(target); err != nil {
		t.Fatalf("seedTargetDefaultCanvasMapAfterCopy() failed: %v", err)
	}

	var mapID string
	if err := target.QueryRow(`SELECT id FROM canvas_maps WHERE is_default = 1`).Scan(&mapID); err != nil {
		t.Fatalf("querying target default map: %v", err)
	}
	if mapID != sourceDefaultMapID {
		t.Fatalf("target default map id = %s, want copied source default %s", mapID, sourceDefaultMapID)
	}

	var x, y float64
	var pinned int
	if err := target.QueryRow(
		`SELECT x, y, pinned FROM canvas_map_positions WHERE map_id = ? AND device_id = ?`,
		mapID,
		deviceID,
	).Scan(&x, &y, &pinned); err != nil {
		t.Fatalf("querying seeded canvas map position: %v", err)
	}
	if x != 71.5 || y != 82.25 || pinned != 1 {
		t.Fatalf("seeded position = (%v, %v, pinned=%d), want (71.5, 82.25, pinned=1)", x, y, pinned)
	}
}

func TestCopyPrimaryData_DefaultConflictPreservesGeneratedLookingTargetDefault(t *testing.T) {
	source := openTestDB(t)
	target := openTestDB(t)

	if err := RunMigrations(source, testKey); err != nil {
		t.Fatalf("RunMigrations(source) failed: %v", err)
	}
	if err := RunMigrations(target, testKey); err != nil {
		t.Fatalf("RunMigrations(target) failed: %v", err)
	}

	const sourceDefaultMapID = "00000000-0000-0000-0000-000000000911"
	const targetDefaultMapID = "00000000-0000-0000-0000-000000000912"
	setDefaultCanvasMapID(t, source, sourceDefaultMapID)
	setDefaultCanvasMapID(t, target, targetDefaultMapID)

	if _, err := target.Exec(
		`INSERT INTO canvas_maps (id, name, description, filter_json, is_default, created_at, updated_at)
		 VALUES (
			'00000000-0000-0000-0000-000000000913',
			'Target Operations',
			'User-created target map',
			'{}',
			0,
			'2026-04-10 00:00:00',
			'2026-04-10 00:00:00'
		 )`,
	); err != nil {
		t.Fatalf("inserting target user-created map: %v", err)
	}

	err := CopyPrimaryData(source, target, CopyOptions{BatchSize: 2})
	if err == nil {
		t.Fatal("CopyPrimaryData() succeeded, want target default conflict")
	}
	if !strings.Contains(err.Error(), "target default canvas map conflicts with copied source default") {
		t.Fatalf("CopyPrimaryData() error = %v, want target default conflict", err)
	}

	var targetDefaultCount int
	if err := target.QueryRow(`SELECT COUNT(*) FROM canvas_maps WHERE id = ? AND is_default = 1`, targetDefaultMapID).Scan(&targetDefaultCount); err != nil {
		t.Fatalf("querying preserved target default: %v", err)
	}
	if targetDefaultCount != 1 {
		t.Fatalf("preserved target default count = %d, want 1", targetDefaultCount)
	}

	var sourceDefaultCount int
	if err := target.QueryRow(`SELECT COUNT(*) FROM canvas_maps WHERE id = ?`, sourceDefaultMapID).Scan(&sourceDefaultCount); err != nil {
		t.Fatalf("querying copied source default after conflict: %v", err)
	}
	if sourceDefaultCount != 0 {
		t.Fatalf("copied source default count after conflict = %d, want 0", sourceDefaultCount)
	}
}

func TestCopyPrimaryData_DeletesFreshGeneratedTargetDefaultBeforeCopyingSourceDefault(t *testing.T) {
	source := openTestDB(t)
	target := openTestDB(t)

	if err := RunMigrations(source, testKey); err != nil {
		t.Fatalf("RunMigrations(source) failed: %v", err)
	}
	if err := RunMigrations(target, testKey); err != nil {
		t.Fatalf("RunMigrations(target) failed: %v", err)
	}

	const sourceDefaultMapID = "00000000-0000-0000-0000-000000000921"
	const targetGeneratedMapID = "00000000-0000-0000-0000-000000000922"
	setDefaultCanvasMapID(t, source, sourceDefaultMapID)
	setDefaultCanvasMapID(t, target, targetGeneratedMapID)

	if err := CopyPrimaryData(source, target, CopyOptions{BatchSize: 2}); err != nil {
		t.Fatalf("CopyPrimaryData() failed: %v", err)
	}

	var copiedDefaultID string
	if err := target.QueryRow(`SELECT id FROM canvas_maps WHERE is_default = 1`).Scan(&copiedDefaultID); err != nil {
		t.Fatalf("querying copied target default: %v", err)
	}
	if copiedDefaultID != sourceDefaultMapID {
		t.Fatalf("target default map id = %s, want copied source default %s", copiedDefaultID, sourceDefaultMapID)
	}

	var generatedDefaultCount int
	if err := target.QueryRow(`SELECT COUNT(*) FROM canvas_maps WHERE id = ?`, targetGeneratedMapID).Scan(&generatedDefaultCount); err != nil {
		t.Fatalf("querying generated target default: %v", err)
	}
	if generatedDefaultCount != 0 {
		t.Fatalf("generated target default count = %d, want 0", generatedDefaultCount)
	}
}

func TestNormalizeCredentialProfileSecretForCopy_Base64EncodesInvalidUTF8(t *testing.T) {
	raw := string([]byte{0x9d, 0x01, 0x02})

	got := normalizeCredentialProfileSecretForCopy("credential_profiles", "encrypted_secret", raw)
	gotText, ok := got.(string)
	if !ok {
		t.Fatalf("normalizeCredentialProfileSecretForCopy returned %T, want string", got)
	}

	want := base64.StdEncoding.EncodeToString([]byte(raw))
	if gotText != want {
		t.Fatalf("normalizeCredentialProfileSecretForCopy() = %q, want %q", gotText, want)
	}
}

func TestNormalizeCredentialProfileSecretForCopy_LeavesValidUTF8Unchanged(t *testing.T) {
	const raw = "enc-secret"

	got := normalizeCredentialProfileSecretForCopy("credential_profiles", "encrypted_secret", raw)
	gotText, ok := got.(string)
	if !ok {
		t.Fatalf("normalizeCredentialProfileSecretForCopy returned %T, want string", got)
	}
	if gotText != raw {
		t.Fatalf("normalizeCredentialProfileSecretForCopy() = %q, want %q", gotText, raw)
	}
}

func TestBuildSelectQueryQuotesStaticIdentifiers(t *testing.T) {
	spec := tableCopySpec{
		name: "device_positions",
		columns: []columnSpec{
			{name: "device_id", kind: columnKindText},
			{name: "x", kind: columnKindFloat64},
			{name: "updated_at", kind: columnKindTime},
		},
		keyColumns: []string{"device_id"},
	}

	got := buildSelectQuery(spec)
	want := `SELECT "device_id", "x", "updated_at" FROM "device_positions" ORDER BY "device_id"`
	if got != want {
		t.Fatalf("buildSelectQuery() = %q, want %q", got, want)
	}
}

func TestBuildSelectQueryQuotesCompositeKeyOrder(t *testing.T) {
	spec := tableCopySpec{
		name: "device_areas",
		columns: []columnSpec{
			{name: "device_id", kind: columnKindText},
			{name: "area_id", kind: columnKindText},
		},
		keyColumns: []string{"device_id", "area_id"},
	}

	got := buildSelectQuery(spec)
	want := `SELECT "device_id", "area_id" FROM "device_areas" ORDER BY "device_id", "area_id"`
	if got != want {
		t.Fatalf("buildSelectQuery() = %q, want %q", got, want)
	}
}

func TestBuildBatchInsertQueryQuotesStaticIdentifiers(t *testing.T) {
	spec := tableCopySpec{
		name: "device_positions",
		columns: []columnSpec{
			{name: "device_id", kind: columnKindText},
			{name: "x", kind: columnKindFloat64},
			{name: "updated_at", kind: columnKindTime},
		},
		keyColumns: []string{"device_id"},
	}

	got := buildBatchInsertQuery(spec, 2, DialectSQLite)
	want := `INSERT INTO "device_positions" ("device_id", "x", "updated_at") VALUES (?, ?, ?), (?, ?, ?) ON CONFLICT ("device_id") DO UPDATE SET "x" = EXCLUDED."x", "updated_at" = EXCLUDED."updated_at"`
	if got != want {
		t.Fatalf("buildBatchInsertQuery() = %q, want %q", got, want)
	}
}

func TestBuildBatchInsertQueryUsesPostgresPlaceholders(t *testing.T) {
	spec := tableCopySpec{
		name: "device_positions",
		columns: []columnSpec{
			{name: "device_id", kind: columnKindText},
			{name: "x", kind: columnKindFloat64},
			{name: "updated_at", kind: columnKindTime},
		},
		keyColumns: []string{"device_id"},
	}

	got := buildBatchInsertQuery(spec, 2, DialectPostgres)
	want := `INSERT INTO "device_positions" ("device_id", "x", "updated_at") VALUES ($1, $2, $3), ($4, $5, $6) ON CONFLICT ("device_id") DO UPDATE SET "x" = EXCLUDED."x", "updated_at" = EXCLUDED."updated_at"`
	if got != want {
		t.Fatalf("buildBatchInsertQuery() = %q, want %q", got, want)
	}
}

func TestBuildBatchInsertQueryUnknownDialectUsesSQLitePlaceholders(t *testing.T) {
	spec := tableCopySpec{
		name: "device_areas",
		columns: []columnSpec{
			{name: "device_id", kind: columnKindText},
			{name: "area_id", kind: columnKindText},
		},
		keyColumns: []string{"device_id", "area_id"},
	}

	got := buildBatchInsertQuery(spec, 2, Dialect(""))
	want := `INSERT INTO "device_areas" ("device_id", "area_id") VALUES (?, ?), (?, ?) ON CONFLICT ("device_id", "area_id") DO NOTHING`
	if got != want {
		t.Fatalf("buildBatchInsertQuery() = %q, want %q", got, want)
	}
}

func TestBuildBatchInsertQueryQuotesCompositeConflictTarget(t *testing.T) {
	spec := tableCopySpec{
		name: "device_areas",
		columns: []columnSpec{
			{name: "device_id", kind: columnKindText},
			{name: "area_id", kind: columnKindText},
		},
		keyColumns: []string{"device_id", "area_id"},
	}

	got := buildBatchInsertQuery(spec, 1, DialectSQLite)
	want := `INSERT INTO "device_areas" ("device_id", "area_id") VALUES (?, ?) ON CONFLICT ("device_id", "area_id") DO NOTHING`
	if got != want {
		t.Fatalf("buildBatchInsertQuery() = %q, want %q", got, want)
	}
}

func TestInsertBatchUsesTargetDialectForPostgresPlaceholders(t *testing.T) {
	db := openCaptureExecDB(t)
	defer db.Close()

	rawTx, err := db.Begin()
	if err != nil {
		t.Fatalf("beginning capture transaction: %v", err)
	}
	defer rawTx.Rollback() //nolint:errcheck

	targetTx := &Tx{raw: rawTx, dialect: DialectPostgres}
	spec := tableCopySpec{
		name: "device_areas",
		columns: []columnSpec{
			{name: "device_id", kind: columnKindText},
			{name: "area_id", kind: columnKindText},
		},
		keyColumns: []string{"device_id", "area_id"},
	}

	err = insertBatch(targetTx, spec, [][]any{
		{"dev-1", "area-1"},
		{"dev-2", "area-2"},
	})
	if err != nil {
		t.Fatalf("insertBatch() failed: %v", err)
	}

	got := singleCapturedExecQuery(t)
	want := `INSERT INTO "device_areas" ("device_id", "area_id") VALUES ($1, $2), ($3, $4) ON CONFLICT ("device_id", "area_id") DO NOTHING`
	if got != want {
		t.Fatalf("captured exec query = %q, want %q", got, want)
	}
	if strings.Contains(got, "?") {
		t.Fatalf("captured exec query contains SQLite placeholder: %q", got)
	}
	for _, placeholder := range []string{"$1", "$2", "$3", "$4"} {
		if !strings.Contains(got, placeholder) {
			t.Fatalf("captured exec query missing %s: %q", placeholder, got)
		}
	}
}

func TestPrimaryDataCopySpecsHaveStaticIdentifierGuardrails(t *testing.T) {
	seenTables := make(map[string]int, len(primaryDataCopySpecs))

	for specIndex, spec := range primaryDataCopySpecs {
		if spec.name == "" {
			t.Errorf("spec[%d] has empty table name", specIndex)
		} else {
			assertStaticIdentifierQuoted(t, "table name", spec.name)
		}

		if previousIndex, ok := seenTables[spec.name]; ok {
			t.Errorf("duplicate table name %q at spec[%d], first seen at spec[%d]", spec.name, specIndex, previousIndex)
		}
		seenTables[spec.name] = specIndex

		if len(spec.columns) == 0 {
			t.Errorf("spec[%d] table %q has no columns", specIndex, spec.name)
		}

		seenColumns := make(map[string]int, len(spec.columns))
		for columnIndex, column := range spec.columns {
			assertStaticIdentifierQuoted(t, "column name", column.name)
			if previousIndex, ok := seenColumns[column.name]; ok {
				t.Errorf(
					"table %q has duplicate column %q at columns[%d], first seen at columns[%d]",
					spec.name,
					column.name,
					columnIndex,
					previousIndex,
				)
			}
			seenColumns[column.name] = columnIndex
		}

		for keyIndex, keyColumn := range spec.keyColumns {
			assertStaticIdentifierQuoted(t, "key column", keyColumn)
			if _, ok := seenColumns[keyColumn]; !ok {
				t.Errorf("table %q keyColumns[%d] %q is not present in columns", spec.name, keyIndex, keyColumn)
			}
		}
	}
}

func assertStaticIdentifierQuoted(t *testing.T, label, identifier string) {
	t.Helper()

	defer func() {
		if recovered := recover(); recovered != nil {
			t.Errorf("%s %q did not pass quoteStaticIdentifier: %v", label, identifier, recovered)
		}
	}()
	_ = quoteStaticIdentifier(identifier)
}

const captureExecDriverName = "theia_data_migrator_capture_exec"

var (
	registerCaptureExecDriverOnce sync.Once

	captureExecMu      sync.Mutex
	captureExecQueries []string
)

func openCaptureExecDB(t *testing.T) *sql.DB {
	t.Helper()

	registerCaptureExecDriverOnce.Do(func() {
		sql.Register(captureExecDriverName, captureExecDriver{})
	})

	captureExecMu.Lock()
	captureExecQueries = nil
	captureExecMu.Unlock()

	db, err := sql.Open(captureExecDriverName, "")
	if err != nil {
		t.Fatalf("opening capture exec database: %v", err)
	}
	return db
}

func singleCapturedExecQuery(t *testing.T) string {
	t.Helper()

	captureExecMu.Lock()
	defer captureExecMu.Unlock()

	if len(captureExecQueries) != 1 {
		t.Fatalf("captured %d exec queries, want 1: %#v", len(captureExecQueries), captureExecQueries)
	}
	return captureExecQueries[0]
}

type captureExecDriver struct{}

func (captureExecDriver) Open(string) (driver.Conn, error) {
	return captureExecConn{}, nil
}

type captureExecConn struct{}

func (captureExecConn) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("capture exec driver does not support prepared statements")
}

func (captureExecConn) Close() error {
	return nil
}

func (captureExecConn) Begin() (driver.Tx, error) {
	return captureExecTx{}, nil
}

func (captureExecConn) ExecContext(_ context.Context, query string, _ []driver.NamedValue) (driver.Result, error) {
	captureExecMu.Lock()
	defer captureExecMu.Unlock()

	captureExecQueries = append(captureExecQueries, query)
	return captureExecResult{}, nil
}

type captureExecTx struct{}

func (captureExecTx) Commit() error {
	return nil
}

func (captureExecTx) Rollback() error {
	return nil
}

type captureExecResult struct{}

func (captureExecResult) LastInsertId() (int64, error) {
	return 0, nil
}

func (captureExecResult) RowsAffected() (int64, error) {
	return 0, nil
}

func TestQuoteStaticIdentifierRejectsInvalidIdentifiers(t *testing.T) {
	tests := []string{
		"",
		"device positions",
		"devices; DROP TABLE devices",
		"devices.name",
		"DeviceName",
		"1device",
	}

	for _, input := range tests {
		t.Run(input, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Fatalf("quoteStaticIdentifier(%q) did not panic", input)
				}
			}()
			_ = quoteStaticIdentifier(input)
		})
	}
}

func TestQuoteStaticIdentifierAcceptsKnownIdentifierShape(t *testing.T) {
	tests := map[string]string{
		"sys_name_lookup": `"sys_name_lookup"`,
		"sha256":          `"sha256"`,
	}

	for input, want := range tests {
		t.Run(input, func(t *testing.T) {
			got := quoteStaticIdentifier(input)
			if got != want {
				t.Fatalf("quoteStaticIdentifier() = %q, want %q", got, want)
			}
		})
	}
}

func seedCopyTestSource(t *testing.T, db testExecer) {
	t.Helper()

	statements := []string{
		`UPDATE settings SET value = 'https://prom.example', updated_at = '2026-04-10 01:02:03' WHERE key = 'prometheus_url'`,
		`INSERT INTO devices (
			id, hostname, ip, snmp_credentials_json, device_type, status, sys_name, sys_descr,
			sys_object_id, hardware_model, vendor, managed, tags_json, created_at, updated_at,
			metrics_source, prometheus_label_name, prometheus_label_value, sys_name_lookup,
			poll_class, poll_interval_override, polling_enabled, notes
		) VALUES (
			'dev-1', 'core-router', '192.0.2.10', '{"version":"2c","v2c":{"community":"secret"}}',
			'router', 'up', 'core-router.example.com', 'RouterOS', '1.3.6.1.4.1.14988',
			'CCR2004', 'mikrotik', 1, '{"role":"core"}', '2026-04-10 00:00:00',
			'2026-04-10 00:00:00', 'prometheus', 'instance', 'core-router:9100', 'core-router',
			'core', 45, 0, 'Primary aggregation node'
		)`,
		`INSERT INTO interfaces (
			id, device_id, if_index, if_name, if_descr, speed, admin_status, oper_status, created_at, updated_at
		) VALUES (
			'if-1', 'dev-1', 1, 'ether1', 'uplink', 1000000000, 'up', 'up',
			'2026-04-10 00:00:00', '2026-04-10 00:00:00'
		)`,
		`INSERT INTO links (
			id, source_device_id, source_if_name, target_device_id, target_if_name, discovery_protocol, created_at, updated_at
		) VALUES (
			'link-1', 'dev-1', 'ether1', 'dev-1', 'loopback', 'manual',
			'2026-04-10 00:00:00', '2026-04-10 00:00:00'
		)`,
		`INSERT INTO device_positions (device_id, x, y, pinned, updated_at)
		 VALUES ('dev-1', 12.5, 24.5, 1, '2026-04-10 00:00:00')`,
		`INSERT INTO snmp_profiles (id, name, description, credentials_json, created_at, updated_at)
		 VALUES ('snmp-1', 'default-snmp', 'seed profile', '{"version":"2c","v2c":{"community":"public"}}',
		 '2026-04-10 00:00:00', '2026-04-10 00:00:00')`,
		`INSERT INTO vendor_configs (name, display_name, config_json, created_at, updated_at)
		 VALUES ('mikrotik', 'MikroTik', '{"backup":{"enabled":true}}',
		 '2026-04-10 00:00:00', '2026-04-10 00:00:00')`,
		`INSERT INTO areas (id, name, description, color, created_at, updated_at)
		 VALUES ('area-1', 'POP North', 'North aggregation', '#123456',
		 '2026-04-10 00:00:00', '2026-04-10 00:00:00')`,
		`INSERT INTO device_areas (device_id, area_id) VALUES ('dev-1', 'area-1')`,
		`INSERT INTO credential_profiles (
			id, name, description, username, port, auth_method, encrypted_secret, created_at, updated_at, role
		) VALUES (
			'cred-1', 'admin-ssh', 'seed credential', 'admin', 22, 'password', 'enc-secret',
			'2026-04-10 00:00:00', '2026-04-10 00:00:00', 'Admin'
		)`,
		`INSERT INTO device_credential_profiles (device_id, profile_id, created_at, is_winbox)
		 VALUES ('dev-1', 'cred-1', '2026-04-10 00:00:00', 1)`,
		`INSERT INTO backup_jobs (id, device_id, status, error_message, created_at)
		 VALUES ('job-1', 'dev-1', 'success', '', '2026-04-10 00:00:00')`,
		`INSERT INTO backup_files (
			id, job_id, file_type, file_name, file_path, file_hash, size_bytes, created_at
		) VALUES (
			'file-1', 'job-1', 'running', 'backup.rsc', '/tmp/backup.rsc', 'abc123', 64,
			'2026-04-10 00:00:00'
		)`,
		`INSERT INTO instance_backups (
			id, file_name, file_path, size_bytes, sha256, app_version, migration_version, status, error_message, trigger_type, created_at
		) VALUES (
			'inst-1', 'theia-backup.tgz', '/tmp/theia-backup.tgz', 128, 'deadbeef', '1.3.7', 15,
			'success', '', 'manual', '2026-04-10 00:00:00'
		)`,
	}

	for _, statement := range statements {
		if _, err := db.Exec(statement); err != nil {
			t.Fatalf("executing seed statement %q: %v", statement, err)
		}
	}
}

func setDefaultCanvasMapID(t *testing.T, db *sql.DB, id string) {
	t.Helper()

	result, err := db.Exec(`UPDATE canvas_maps SET id = ? WHERE is_default = 1`, id)
	if err != nil {
		t.Fatalf("setting default canvas map id: %v", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		t.Fatalf("checking default canvas map id update: %v", err)
	}
	if rowsAffected != 1 {
		t.Fatalf("updated %d default canvas maps, want 1", rowsAffected)
	}
}

func assertCopyTargetState(
	t *testing.T,
	db testQueryRower,
	wantHostname, wantPromURL string,
	wantWinbox bool,
	wantPollClass string,
	wantPollIntervalOverride *int,
) {
	t.Helper()

	var hostname, promURL string
	var winbox bool
	var pollClass string
	var pollIntervalOverride sql.NullInt64
	var pollingEnabled int64
	var notes sql.NullString
	var areaCount, linkCount, backupFileCount, instanceBackupCount, vendorConfigCount int

	if err := db.QueryRow(`SELECT hostname FROM devices WHERE id = 'dev-1'`).Scan(&hostname); err != nil {
		t.Fatalf("querying copied device: %v", err)
	}
	if hostname != wantHostname {
		t.Fatalf("hostname = %q, want %q", hostname, wantHostname)
	}

	if err := db.QueryRow(`SELECT value FROM settings WHERE key = 'prometheus_url'`).Scan(&promURL); err != nil {
		t.Fatalf("querying copied settings row: %v", err)
	}
	if promURL != wantPromURL {
		t.Fatalf("prometheus_url = %q, want %q", promURL, wantPromURL)
	}

	if err := db.QueryRow(`SELECT is_winbox FROM device_credential_profiles WHERE device_id = 'dev-1' AND profile_id = 'cred-1'`).Scan(&winbox); err != nil {
		t.Fatalf("querying copied device_credential_profiles row: %v", err)
	}
	if winbox != wantWinbox {
		t.Fatalf("is_winbox = %v, want %v", winbox, wantWinbox)
	}

	if err := db.QueryRow(`SELECT poll_class, poll_interval_override, polling_enabled FROM devices WHERE id = 'dev-1'`).Scan(&pollClass, &pollIntervalOverride, &pollingEnabled); err != nil {
		t.Fatalf("querying copied poll fields: %v", err)
	}
	if pollClass != wantPollClass {
		t.Fatalf("poll_class = %q, want %q", pollClass, wantPollClass)
	}
	if pollingEnabled != 0 {
		t.Fatalf("polling_enabled = %d, want 0", pollingEnabled)
	}
	switch {
	case wantPollIntervalOverride == nil && pollIntervalOverride.Valid:
		t.Fatalf("poll_interval_override = %d, want NULL", pollIntervalOverride.Int64)
	case wantPollIntervalOverride != nil && (!pollIntervalOverride.Valid || int(pollIntervalOverride.Int64) != *wantPollIntervalOverride):
		if !pollIntervalOverride.Valid {
			t.Fatalf("poll_interval_override = NULL, want %d", *wantPollIntervalOverride)
		}
		t.Fatalf("poll_interval_override = %d, want %d", pollIntervalOverride.Int64, *wantPollIntervalOverride)
	}

	if err := db.QueryRow(`SELECT notes FROM devices WHERE id = 'dev-1'`).Scan(&notes); err != nil {
		t.Fatalf("querying copied notes: %v", err)
	}
	if !notes.Valid || notes.String != "Primary aggregation node" {
		t.Fatalf("notes = %#v, want %q", notes, "Primary aggregation node")
	}

	if err := db.QueryRow(`SELECT COUNT(*) FROM device_areas WHERE device_id = 'dev-1'`).Scan(&areaCount); err != nil {
		t.Fatalf("counting copied device_areas rows: %v", err)
	}
	if areaCount != 1 {
		t.Fatalf("device_areas count = %d, want 1", areaCount)
	}

	if err := db.QueryRow(`SELECT COUNT(*) FROM links WHERE id = 'link-1'`).Scan(&linkCount); err != nil {
		t.Fatalf("counting copied links rows: %v", err)
	}
	if linkCount != 1 {
		t.Fatalf("links count = %d, want 1", linkCount)
	}

	if err := db.QueryRow(`SELECT COUNT(*) FROM backup_files WHERE id = 'file-1'`).Scan(&backupFileCount); err != nil {
		t.Fatalf("counting copied backup_files rows: %v", err)
	}
	if backupFileCount != 1 {
		t.Fatalf("backup_files count = %d, want 1", backupFileCount)
	}

	if err := db.QueryRow(`SELECT COUNT(*) FROM instance_backups WHERE id = 'inst-1'`).Scan(&instanceBackupCount); err != nil {
		t.Fatalf("counting copied instance_backups rows: %v", err)
	}
	if instanceBackupCount != 1 {
		t.Fatalf("instance_backups count = %d, want 1", instanceBackupCount)
	}

	if err := db.QueryRow(`SELECT COUNT(*) FROM vendor_configs WHERE name = 'mikrotik'`).Scan(&vendorConfigCount); err != nil {
		t.Fatalf("counting copied vendor_configs rows: %v", err)
	}
	if vendorConfigCount != 1 {
		t.Fatalf("vendor_configs count = %d, want 1", vendorConfigCount)
	}
}

type testExecer interface {
	Exec(query string, args ...any) (sql.Result, error)
}

type testQueryRower interface {
	QueryRow(query string, args ...any) *sql.Row
}
