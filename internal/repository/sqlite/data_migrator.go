package sqlite

import (
	"database/sql"
	"encoding/base64"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

const defaultCopyBatchSize = 250

type CopyOptions struct {
	TruncateTarget bool
	BatchSize      int
	Logf           func(format string, args ...any)
}

type columnKind int

const (
	columnKindText columnKind = iota
	columnKindInt64
	columnKindFloat64
	columnKindBool
	columnKindTime
)

type columnSpec struct {
	name string
	kind columnKind
}

type tableCopySpec struct {
	name       string
	columns    []columnSpec
	keyColumns []string
}

var primaryDataCopySpecs = []tableCopySpec{
	{
		name: "devices",
		columns: []columnSpec{
			{name: "id", kind: columnKindText},
			{name: "hostname", kind: columnKindText},
			{name: "ip", kind: columnKindText},
			{name: "snmp_credentials_json", kind: columnKindText},
			{name: "device_type", kind: columnKindText},
			{name: "status", kind: columnKindText},
			{name: "sys_name", kind: columnKindText},
			{name: "sys_descr", kind: columnKindText},
			{name: "sys_object_id", kind: columnKindText},
			{name: "hardware_model", kind: columnKindText},
			{name: "os_version", kind: columnKindText},
			{name: "vendor", kind: columnKindText},
			{name: "managed", kind: columnKindInt64},
			{name: "tags_json", kind: columnKindText},
			{name: "created_at", kind: columnKindTime},
			{name: "updated_at", kind: columnKindTime},
			{name: "metrics_source", kind: columnKindText},
			{name: "prometheus_label_name", kind: columnKindText},
			{name: "prometheus_label_value", kind: columnKindText},
			{name: "sys_name_lookup", kind: columnKindText},
			{name: "poll_class", kind: columnKindText},
			{name: "poll_interval_override", kind: columnKindInt64},
			{name: "polling_enabled", kind: columnKindInt64},
			{name: "notes", kind: columnKindText},
		},
		keyColumns: []string{"id"},
	},
	{
		name: "snmp_profiles",
		columns: []columnSpec{
			{name: "id", kind: columnKindText},
			{name: "name", kind: columnKindText},
			{name: "description", kind: columnKindText},
			{name: "credentials_json", kind: columnKindText},
			{name: "created_at", kind: columnKindTime},
			{name: "updated_at", kind: columnKindTime},
		},
		keyColumns: []string{"id"},
	},
	{
		name: "vendor_configs",
		columns: []columnSpec{
			{name: "name", kind: columnKindText},
			{name: "display_name", kind: columnKindText},
			{name: "config_json", kind: columnKindText},
			{name: "created_at", kind: columnKindTime},
			{name: "updated_at", kind: columnKindTime},
		},
		keyColumns: []string{"name"},
	},
	{
		name: "areas",
		columns: []columnSpec{
			{name: "id", kind: columnKindText},
			{name: "name", kind: columnKindText},
			{name: "description", kind: columnKindText},
			{name: "color", kind: columnKindText},
			{name: "created_at", kind: columnKindTime},
			{name: "updated_at", kind: columnKindTime},
		},
		keyColumns: []string{"id"},
	},
	{
		name: "interfaces",
		columns: []columnSpec{
			{name: "id", kind: columnKindText},
			{name: "device_id", kind: columnKindText},
			{name: "if_index", kind: columnKindInt64},
			{name: "if_name", kind: columnKindText},
			{name: "if_descr", kind: columnKindText},
			{name: "speed", kind: columnKindInt64},
			{name: "admin_status", kind: columnKindText},
			{name: "oper_status", kind: columnKindText},
			{name: "created_at", kind: columnKindTime},
			{name: "updated_at", kind: columnKindTime},
		},
		keyColumns: []string{"id"},
	},
	{
		name: "device_positions",
		columns: []columnSpec{
			{name: "device_id", kind: columnKindText},
			{name: "x", kind: columnKindFloat64},
			{name: "y", kind: columnKindFloat64},
			{name: "pinned", kind: columnKindInt64},
			{name: "updated_at", kind: columnKindTime},
		},
		keyColumns: []string{"device_id"},
	},
	{
		name: "canvas_maps",
		columns: []columnSpec{
			{name: "id", kind: columnKindText},
			{name: "name", kind: columnKindText},
			{name: "description", kind: columnKindText},
			{name: "source_area_id", kind: columnKindText},
			{name: "filter_json", kind: columnKindText},
			{name: "is_default", kind: columnKindBool},
			{name: "created_at", kind: columnKindTime},
			{name: "updated_at", kind: columnKindTime},
		},
		keyColumns: []string{"id"},
	},
	{
		name: "canvas_map_positions",
		columns: []columnSpec{
			{name: "map_id", kind: columnKindText},
			{name: "device_id", kind: columnKindText},
			{name: "x", kind: columnKindFloat64},
			{name: "y", kind: columnKindFloat64},
			{name: "pinned", kind: columnKindBool},
			{name: "updated_at", kind: columnKindTime},
		},
		keyColumns: []string{"map_id", "device_id"},
	},
	{
		name: "canvas_map_devices",
		columns: []columnSpec{
			{name: "map_id", kind: columnKindText},
			{name: "device_id", kind: columnKindText},
			{name: "role", kind: columnKindText},
			{name: "added_at", kind: columnKindTime},
		},
		keyColumns: []string{"map_id", "device_id"},
	},
	{
		name: "canvas_map_areas",
		columns: []columnSpec{
			{name: "map_id", kind: columnKindText},
			{name: "area_id", kind: columnKindText},
			{name: "name", kind: columnKindText},
			{name: "description", kind: columnKindText},
			{name: "color", kind: columnKindText},
			{name: "added_at", kind: columnKindTime},
		},
		keyColumns: []string{"map_id", "area_id"},
	},
	{
		name: "device_areas",
		columns: []columnSpec{
			{name: "device_id", kind: columnKindText},
			{name: "area_id", kind: columnKindText},
		},
		keyColumns: []string{"device_id", "area_id"},
	},
	{
		name: "credential_profiles",
		columns: []columnSpec{
			{name: "id", kind: columnKindText},
			{name: "name", kind: columnKindText},
			{name: "description", kind: columnKindText},
			{name: "username", kind: columnKindText},
			{name: "port", kind: columnKindInt64},
			{name: "auth_method", kind: columnKindText},
			{name: "encrypted_secret", kind: columnKindText},
			{name: "created_at", kind: columnKindTime},
			{name: "updated_at", kind: columnKindTime},
			{name: "role", kind: columnKindText},
		},
		keyColumns: []string{"id"},
	},
	{
		name: "device_credential_profiles",
		columns: []columnSpec{
			{name: "device_id", kind: columnKindText},
			{name: "profile_id", kind: columnKindText},
			{name: "created_at", kind: columnKindTime},
			{name: "is_winbox", kind: columnKindBool},
		},
		keyColumns: []string{"device_id", "profile_id"},
	},
	{
		name: "links",
		columns: []columnSpec{
			{name: "id", kind: columnKindText},
			{name: "source_device_id", kind: columnKindText},
			{name: "source_if_name", kind: columnKindText},
			{name: "target_device_id", kind: columnKindText},
			{name: "target_if_name", kind: columnKindText},
			{name: "discovery_protocol", kind: columnKindText},
			{name: "created_at", kind: columnKindTime},
			{name: "updated_at", kind: columnKindTime},
		},
		keyColumns: []string{"id"},
	},
	{
		name: "canvas_map_links",
		columns: []columnSpec{
			{name: "map_id", kind: columnKindText},
			{name: "link_id", kind: columnKindText},
			{name: "added_at", kind: columnKindTime},
		},
		keyColumns: []string{"map_id", "link_id"},
	},
	{
		name: "settings",
		columns: []columnSpec{
			{name: "key", kind: columnKindText},
			{name: "value", kind: columnKindText},
			{name: "updated_at", kind: columnKindTime},
		},
		keyColumns: []string{"key"},
	},
	{
		name: "backup_jobs",
		columns: []columnSpec{
			{name: "id", kind: columnKindText},
			{name: "device_id", kind: columnKindText},
			{name: "status", kind: columnKindText},
			{name: "error_message", kind: columnKindText},
			{name: "created_at", kind: columnKindTime},
		},
		keyColumns: []string{"id"},
	},
	{
		name: "backup_files",
		columns: []columnSpec{
			{name: "id", kind: columnKindText},
			{name: "job_id", kind: columnKindText},
			{name: "file_type", kind: columnKindText},
			{name: "file_name", kind: columnKindText},
			{name: "file_path", kind: columnKindText},
			{name: "file_hash", kind: columnKindText},
			{name: "size_bytes", kind: columnKindInt64},
			{name: "created_at", kind: columnKindTime},
		},
		keyColumns: []string{"id"},
	},
	{
		name: "instance_backups",
		columns: []columnSpec{
			{name: "id", kind: columnKindText},
			{name: "file_name", kind: columnKindText},
			{name: "file_path", kind: columnKindText},
			{name: "size_bytes", kind: columnKindInt64},
			{name: "sha256", kind: columnKindText},
			{name: "app_version", kind: columnKindText},
			{name: "migration_version", kind: columnKindInt64},
			{name: "status", kind: columnKindText},
			{name: "error_message", kind: columnKindText},
			{name: "trigger_type", kind: columnKindText},
			{name: "created_at", kind: columnKindTime},
		},
		keyColumns: []string{"id"},
	},
}

func MigrateSQLiteToPostgres(sourcePath, targetDSN string, opts CopyOptions) error {
	sourcePath = strings.TrimSpace(sourcePath)
	if sourcePath == "" {
		return fmt.Errorf("source sqlite path is required")
	}
	if _, err := os.Stat(sourcePath); err != nil {
		return fmt.Errorf("checking source sqlite database: %w", err)
	}
	if strings.TrimSpace(targetDSN) == "" {
		return fmt.Errorf("target postgres dsn is required")
	}

	sourceDB, _, err := OpenPrimaryDB(string(DialectSQLite), sourcePath, "")
	if err != nil {
		return fmt.Errorf("opening source sqlite database: %w", err)
	}
	defer sourceDB.Close()
	ConfigureDB(sourceDB)

	targetDB, dialect, err := OpenPrimaryDB(string(DialectPostgres), "", targetDSN)
	if err != nil {
		return fmt.Errorf("opening target postgres database: %w", err)
	}
	defer targetDB.Close()
	ConfigureDB(targetDB)

	if dialect != DialectPostgres {
		return fmt.Errorf("target database must be postgres, got %s", dialect)
	}
	if err := targetDB.Ping(); err != nil {
		return fmt.Errorf("pinging target postgres database: %w", err)
	}
	if err := RunMigrations(targetDB); err != nil {
		return fmt.Errorf("running target postgres migrations: %w", err)
	}
	if err := CopyPrimaryData(sourceDB, targetDB, opts); err != nil {
		return err
	}
	if err := seedTargetDefaultCanvasMapAfterCopy(targetDB); err != nil {
		return err
	}

	if _, err := wrapDB(targetDB).Exec("ANALYZE"); err != nil {
		return fmt.Errorf("analyzing target database: %w", err)
	}
	return nil
}

func CopyPrimaryData(source, target *sql.DB, opts CopyOptions) error {
	if source == nil {
		return fmt.Errorf("source database is nil")
	}
	if target == nil {
		return fmt.Errorf("target database is nil")
	}

	sourceDialect := detectDialectFromDB(source)
	batchSize := opts.BatchSize
	if batchSize <= 0 {
		batchSize = defaultCopyBatchSize
	}

	sourceTx, err := source.Begin()
	if err != nil {
		return fmt.Errorf("beginning source read transaction: %w", err)
	}
	defer sourceTx.Rollback() //nolint:errcheck

	targetTx, err := wrapDB(target).Begin()
	if err != nil {
		return fmt.Errorf("beginning target write transaction: %w", err)
	}
	defer targetTx.Rollback() //nolint:errcheck

	if targetTx.dialect == DialectPostgres {
		if _, err := targetTx.Exec("SET LOCAL synchronous_commit = OFF"); err != nil {
			return fmt.Errorf("configuring postgres import transaction: %w", err)
		}
	}

	if opts.TruncateTarget {
		if err := clearTargetTables(targetTx, primaryDataCopySpecs); err != nil {
			return fmt.Errorf("clearing target data: %w", err)
		}
	} else {
		if err := clearGeneratedTargetDefaultCanvasMapForCopy(sourceTx, targetTx, sourceDialect); err != nil {
			return fmt.Errorf("preparing canvas map copy: %w", err)
		}
	}

	for _, spec := range primaryDataCopySpecs {
		rowCount, err := copyTableData(sourceTx, targetTx, spec, batchSize)
		if err != nil {
			return fmt.Errorf("copying %s: %w", spec.name, err)
		}
		if opts.Logf != nil {
			opts.Logf("copied %d rows into %s", rowCount, spec.name)
		}
	}

	if err := targetTx.Commit(); err != nil {
		return fmt.Errorf("committing target transaction: %w", err)
	}
	return nil
}

func seedTargetDefaultCanvasMapAfterCopy(target *sql.DB) error {
	if err := migrateDefaultCanvasMap(target); err != nil {
		return fmt.Errorf("seeding target default canvas map after copy: %w", err)
	}
	return nil
}

func clearGeneratedTargetDefaultCanvasMapForCopy(sourceTx *sql.Tx, targetTx *Tx, sourceDialect Dialect) error {
	sourceDefaultIDs, err := sourceDefaultCanvasMapIDs(sourceTx, sourceDialect)
	if err != nil {
		return err
	}
	if len(sourceDefaultIDs) == 0 {
		return nil
	}

	targetDefault, ok, err := targetDefaultCanvasMapForCopy(targetTx)
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}
	if !sourceHasDifferentDefaultCanvasMapID(sourceDefaultIDs, targetDefault.id) {
		return nil
	}
	if !targetDefault.isFreshGeneratedMigrationDefault() {
		return fmt.Errorf("target default canvas map conflicts with copied source default: %s", targetDefault.id)
	}

	return deleteFreshGeneratedTargetDefaultCanvasMap(targetTx, targetDefault)
}

func deleteFreshGeneratedTargetDefaultCanvasMap(targetTx *Tx, targetDefault targetDefaultCanvasMapCopyState) error {
	result, err := targetTx.Exec(
		`DELETE FROM canvas_maps
		 WHERE id = ?
		   AND is_default = ?
		   AND name = 'Default'
		   AND description = 'Global canvas layout'
		   AND source_area_id IS NULL
		   AND filter_json = '{}'
		   AND NOT EXISTS (SELECT 1 FROM canvas_map_positions)
		   AND NOT EXISTS (SELECT 1 FROM canvas_map_devices)
		   AND NOT EXISTS (SELECT 1 FROM canvas_map_links)
		   AND NOT EXISTS (SELECT 1 FROM canvas_map_areas)
		   AND (SELECT COUNT(*) FROM canvas_maps) = 1`,
		targetDefault.id,
		true,
	)
	if err != nil {
		return fmt.Errorf("deleting generated target default canvas map %s: %w", targetDefault.id, err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("checking generated target default canvas map delete %s: %w", targetDefault.id, err)
	}
	if rowsAffected == 0 {
		return fmt.Errorf("target default canvas map conflicts with copied source default: %s", targetDefault.id)
	}
	return nil
}

func sourceHasDifferentDefaultCanvasMapID(sourceDefaultIDs map[string]struct{}, targetDefaultID string) bool {
	for sourceDefaultID := range sourceDefaultIDs {
		if sourceDefaultID != targetDefaultID {
			return true
		}
	}
	return false
}

func sourceDefaultCanvasMapIDs(sourceTx *sql.Tx, sourceDialect Dialect) (map[string]struct{}, error) {
	query := `SELECT id FROM canvas_maps WHERE is_default = 1`
	if sourceDialect == DialectPostgres {
		query = `SELECT id FROM canvas_maps WHERE is_default = TRUE`
	}

	rows, err := sourceTx.Query(query)
	if err != nil {
		return nil, fmt.Errorf("querying source default canvas maps: %w", err)
	}
	defer rows.Close()

	ids := make(map[string]struct{})
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scanning source default canvas map id: %w", err)
		}
		ids[id] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating source default canvas map ids: %w", err)
	}
	return ids, nil
}

type targetDefaultCanvasMapCopyState struct {
	id                 string
	name               string
	description        string
	sourceAreaID       sql.NullString
	filterJSON         string
	positionCount      int
	mapCount           int
	totalPositionCount int
	totalDeviceCount   int
	totalLinkCount     int
	totalAreaCount     int
}

func targetDefaultCanvasMapForCopy(targetTx *Tx) (targetDefaultCanvasMapCopyState, bool, error) {
	var targetDefault targetDefaultCanvasMapCopyState
	err := targetTx.QueryRow(
		`SELECT
			cm.id,
			cm.name,
			cm.description,
			cm.source_area_id,
			cm.filter_json,
			COUNT(cmp.device_id),
			(SELECT COUNT(*) FROM canvas_maps),
			(SELECT COUNT(*) FROM canvas_map_positions),
			(SELECT COUNT(*) FROM canvas_map_devices),
			(SELECT COUNT(*) FROM canvas_map_links),
			(SELECT COUNT(*) FROM canvas_map_areas)
		 FROM canvas_maps cm
		 LEFT JOIN canvas_map_positions cmp ON cmp.map_id = cm.id
		 WHERE cm.is_default = ?
		 GROUP BY cm.id, cm.name, cm.description, cm.source_area_id, cm.filter_json
		 LIMIT 1`,
		true,
	).Scan(
		&targetDefault.id,
		&targetDefault.name,
		&targetDefault.description,
		&targetDefault.sourceAreaID,
		&targetDefault.filterJSON,
		&targetDefault.positionCount,
		&targetDefault.mapCount,
		&targetDefault.totalPositionCount,
		&targetDefault.totalDeviceCount,
		&targetDefault.totalLinkCount,
		&targetDefault.totalAreaCount,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return targetDefaultCanvasMapCopyState{}, false, nil
		}
		return targetDefaultCanvasMapCopyState{}, false, fmt.Errorf("querying target default canvas map: %w", err)
	}
	return targetDefault, true, nil
}

func (state targetDefaultCanvasMapCopyState) isFreshGeneratedMigrationDefault() bool {
	return state.name == "Default" &&
		state.description == "Global canvas layout" &&
		!state.sourceAreaID.Valid &&
		state.filterJSON == "{}" &&
		state.positionCount == 0 &&
		state.mapCount == 1 &&
		state.totalPositionCount == 0 &&
		state.totalDeviceCount == 0 &&
		state.totalLinkCount == 0 &&
		state.totalAreaCount == 0
}

func clearTargetTables(targetTx *Tx, specs []tableCopySpec) error {
	if targetTx.dialect == DialectPostgres {
		tableNames := make([]string, 0, len(specs))
		for _, spec := range specs {
			tableNames = append(tableNames, quoteStaticIdentifier(spec.name))
		}
		_, err := targetTx.Exec("TRUNCATE TABLE " + strings.Join(tableNames, ", ") + " CASCADE")
		return err
	}

	for i := len(specs) - 1; i >= 0; i-- {
		if _, err := targetTx.Exec("DELETE FROM " + quoteStaticIdentifier(specs[i].name)); err != nil {
			return fmt.Errorf("deleting %s: %w", specs[i].name, err)
		}
	}
	return nil
}

func copyTableData(sourceTx *sql.Tx, targetTx *Tx, spec tableCopySpec, batchSize int) (int, error) {
	query := buildSelectQuery(spec)
	rows, err := sourceTx.Query(query)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	totalRows := 0
	batch := make([][]any, 0, batchSize)

	for rows.Next() {
		rawValues := make([]any, len(spec.columns))
		dest := make([]any, len(spec.columns))
		for i := range rawValues {
			dest[i] = &rawValues[i]
		}

		if err := rows.Scan(dest...); err != nil {
			return totalRows, fmt.Errorf("scanning source row: %w", err)
		}

		normalized := make([]any, len(spec.columns))
		for i, column := range spec.columns {
			normalized[i], err = normalizeCopyValue(column.kind, rawValues[i])
			if err != nil {
				return totalRows, fmt.Errorf("normalizing %s.%s: %w", spec.name, column.name, err)
			}
			normalized[i] = normalizeCredentialProfileSecretForCopy(spec.name, column.name, normalized[i])
		}

		batch = append(batch, normalized)
		totalRows++

		if len(batch) >= batchSize {
			if err := insertBatch(targetTx, spec, batch); err != nil {
				return totalRows, err
			}
			batch = batch[:0]
		}
	}
	if err := rows.Err(); err != nil {
		return totalRows, err
	}
	if len(batch) > 0 {
		if err := insertBatch(targetTx, spec, batch); err != nil {
			return totalRows, err
		}
	}

	return totalRows, nil
}

func normalizeCredentialProfileSecretForCopy(tableName, columnName string, value any) any {
	if tableName != "credential_profiles" || columnName != "encrypted_secret" {
		return value
	}

	text, ok := value.(string)
	if !ok || text == "" || utf8.ValidString(text) {
		return value
	}

	return base64.StdEncoding.EncodeToString([]byte(text))
}

func quoteStaticIdentifier(identifier string) string {
	if identifier == "" {
		panic("static identifier must not be empty")
	}
	if identifier[0] < 'a' || identifier[0] > 'z' {
		panic(fmt.Sprintf("unsafe static identifier %q", identifier))
	}
	for i := 1; i < len(identifier); i++ {
		char := identifier[i]
		if (char >= 'a' && char <= 'z') || (char >= '0' && char <= '9') || char == '_' {
			continue
		}
		panic(fmt.Sprintf("unsafe static identifier %q", identifier))
	}
	return `"` + identifier + `"`
}

func quoteStaticIdentifiers(identifiers []string) []string {
	quoted := make([]string, len(identifiers))
	for i, identifier := range identifiers {
		quoted[i] = quoteStaticIdentifier(identifier)
	}
	return quoted
}

func quotedColumnNames(spec tableCopySpec) []string {
	columnNames := make([]string, len(spec.columns))
	for i, column := range spec.columns {
		columnNames[i] = quoteStaticIdentifier(column.name)
	}
	return columnNames
}

func buildSelectQuery(spec tableCopySpec) string {
	columnNames := quotedColumnNames(spec)

	query := fmt.Sprintf("SELECT %s FROM %s", strings.Join(columnNames, ", "), quoteStaticIdentifier(spec.name))
	if len(spec.keyColumns) > 0 {
		query += " ORDER BY " + strings.Join(quoteStaticIdentifiers(spec.keyColumns), ", ")
	}
	return query
}

func insertBatch(targetTx *Tx, spec tableCopySpec, rows [][]any) error {
	if len(rows) == 0 {
		return nil
	}

	query := buildBatchInsertQuery(spec, len(rows), targetTx.dialect)
	args := make([]any, 0, len(rows)*len(spec.columns))
	for _, row := range rows {
		args = append(args, row...)
	}

	// Batch insert placeholders are already generated for targetTx.dialect.
	if _, err := targetTx.raw.Exec(query, args...); err != nil {
		return fmt.Errorf("executing batch insert for %s: %w", spec.name, err)
	}
	return nil
}

func buildBatchInsertQuery(spec tableCopySpec, rowCount int, dialect Dialect) string {
	columnNames := quotedColumnNames(spec)

	var builder strings.Builder
	builder.Grow((len(columnNames) + 4) * rowCount)
	builder.WriteString("INSERT INTO ")
	builder.WriteString(quoteStaticIdentifier(spec.name))
	builder.WriteString(" (")
	builder.WriteString(strings.Join(columnNames, ", "))
	builder.WriteString(") VALUES ")

	for rowIndex := 0; rowIndex < rowCount; rowIndex++ {
		if rowIndex > 0 {
			builder.WriteString(", ")
		}
		builder.WriteByte('(')
		for colIndex := range spec.columns {
			if colIndex > 0 {
				builder.WriteString(", ")
			}
			if dialect == DialectPostgres {
				placeholderIndex := rowIndex*len(spec.columns) + colIndex + 1
				builder.WriteByte('$')
				builder.WriteString(strconv.Itoa(placeholderIndex))
			} else {
				builder.WriteByte('?')
			}
		}
		builder.WriteByte(')')
	}

	if len(spec.keyColumns) == 0 {
		return builder.String()
	}

	builder.WriteString(" ON CONFLICT (")
	builder.WriteString(strings.Join(quoteStaticIdentifiers(spec.keyColumns), ", "))
	builder.WriteByte(')')

	updateColumns := make([]string, 0, len(spec.columns))
	keyLookup := make(map[string]struct{}, len(spec.keyColumns))
	for _, keyColumn := range spec.keyColumns {
		keyLookup[keyColumn] = struct{}{}
	}
	for _, column := range spec.columns {
		if _, isKey := keyLookup[column.name]; isKey {
			continue
		}
		updateColumns = append(updateColumns, column.name)
	}

	if len(updateColumns) == 0 {
		builder.WriteString(" DO NOTHING")
		return builder.String()
	}

	builder.WriteString(" DO UPDATE SET ")
	for i, columnName := range updateColumns {
		if i > 0 {
			builder.WriteString(", ")
		}
		builder.WriteString(quoteStaticIdentifier(columnName))
		builder.WriteString(" = EXCLUDED.")
		builder.WriteString(quoteStaticIdentifier(columnName))
	}

	return builder.String()
}

func normalizeCopyValue(kind columnKind, raw any) (any, error) {
	if raw == nil {
		return nil, nil
	}

	switch kind {
	case columnKindText:
		return normalizeTextValue(raw)
	case columnKindInt64:
		return normalizeIntValue(raw)
	case columnKindFloat64:
		return normalizeFloatValue(raw)
	case columnKindBool:
		return normalizeBoolValue(raw)
	case columnKindTime:
		return normalizeTimeValue(raw)
	default:
		return nil, fmt.Errorf("unsupported column kind %d", kind)
	}
}

func normalizeTextValue(raw any) (string, error) {
	switch value := raw.(type) {
	case string:
		return value, nil
	case []byte:
		return string(value), nil
	case time.Time:
		return value.UTC().Format(time.RFC3339Nano), nil
	default:
		return fmt.Sprintf("%v", value), nil
	}
}

func normalizeIntValue(raw any) (int64, error) {
	switch value := raw.(type) {
	case int:
		return int64(value), nil
	case int32:
		return int64(value), nil
	case int64:
		return value, nil
	case float64:
		return int64(value), nil
	case bool:
		if value {
			return 1, nil
		}
		return 0, nil
	case []byte:
		return strconv.ParseInt(string(value), 10, 64)
	case string:
		return strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	default:
		return 0, fmt.Errorf("unsupported integer value type %T", raw)
	}
}

func normalizeFloatValue(raw any) (float64, error) {
	switch value := raw.(type) {
	case int:
		return float64(value), nil
	case int32:
		return float64(value), nil
	case int64:
		return float64(value), nil
	case float32:
		return float64(value), nil
	case float64:
		return value, nil
	case []byte:
		return strconv.ParseFloat(string(value), 64)
	case string:
		return strconv.ParseFloat(strings.TrimSpace(value), 64)
	default:
		return 0, fmt.Errorf("unsupported float value type %T", raw)
	}
}

func normalizeBoolValue(raw any) (bool, error) {
	switch value := raw.(type) {
	case bool:
		return value, nil
	case int:
		return value != 0, nil
	case int32:
		return value != 0, nil
	case int64:
		return value != 0, nil
	case []byte:
		return parseBoolString(string(value))
	case string:
		return parseBoolString(value)
	default:
		return false, fmt.Errorf("unsupported bool value type %T", raw)
	}
}

func parseBoolString(value string) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "t", "yes", "y":
		return true, nil
	case "0", "false", "f", "no", "n", "":
		return false, nil
	default:
		return false, fmt.Errorf("unsupported bool literal %q", value)
	}
}

func normalizeTimeValue(raw any) (time.Time, error) {
	switch value := raw.(type) {
	case time.Time:
		return value.UTC(), nil
	case []byte:
		return parseSQLiteTimestamp(string(value))
	case string:
		return parseSQLiteTimestamp(value)
	default:
		return time.Time{}, fmt.Errorf("unsupported time value type %T", raw)
	}
}

func parseSQLiteTimestamp(value string) (time.Time, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return time.Time{}, fmt.Errorf("empty timestamp")
	}

	layouts := []struct {
		layout string
		useUTC bool
	}{
		{layout: time.RFC3339Nano},
		{layout: "2006-01-02 15:04:05.999999999-07:00"},
		{layout: "2006-01-02 15:04:05.999999999Z07:00"},
		{layout: "2006-01-02 15:04:05.999999999", useUTC: true},
		{layout: "2006-01-02 15:04:05", useUTC: true},
		{layout: "2006-01-02T15:04:05.999999999", useUTC: true},
		{layout: "2006-01-02T15:04:05", useUTC: true},
	}

	for _, candidate := range layouts {
		var (
			parsed time.Time
			err    error
		)
		if candidate.useUTC {
			parsed, err = time.ParseInLocation(candidate.layout, trimmed, time.UTC)
		} else {
			parsed, err = time.Parse(candidate.layout, trimmed)
		}
		if err == nil {
			return parsed.UTC(), nil
		}
	}

	return time.Time{}, fmt.Errorf("unsupported timestamp literal %q", value)
}
