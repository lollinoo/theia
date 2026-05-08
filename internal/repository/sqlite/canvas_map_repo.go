package sqlite

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/domain"
)

// CanvasMapRepo implements domain.CanvasMapRepository using SQLite-compatible SQL.
type CanvasMapRepo struct {
	db *DB
}

// NewCanvasMapRepo creates a new SQLite-backed canvas map repository.
func NewCanvasMapRepo(db *sql.DB) *CanvasMapRepo {
	return &CanvasMapRepo{db: wrapDB(db)}
}

// CanvasMapPositionRepo implements domain.CanvasMapPositionRepository using SQLite-compatible SQL.
type CanvasMapPositionRepo struct {
	db *DB
}

// NewCanvasMapPositionRepo creates a new SQLite-backed canvas map position repository.
func NewCanvasMapPositionRepo(db *sql.DB) *CanvasMapPositionRepo {
	return &CanvasMapPositionRepo{db: wrapDB(db)}
}

// Create inserts a new saved canvas map.
func (r *CanvasMapRepo) Create(input domain.CanvasMapCreate) (domain.CanvasMap, error) {
	if err := domain.ValidateCanvasMapName(input.Name); err != nil {
		return domain.CanvasMap{}, err
	}
	if err := domain.ValidateCanvasMapDescription(input.Description); err != nil {
		return domain.CanvasMap{}, err
	}
	filterJSON, err := domain.CanonicalCanvasMapFilterJSON(input.Filter)
	if err != nil {
		return domain.CanvasMap{}, err
	}

	mapID := uuid.New()
	now := time.Now().UTC()
	if _, err := r.db.Exec(
		`INSERT INTO canvas_maps (id, name, description, source_area_id, filter_json, is_default, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		mapID.String(),
		input.Name,
		input.Description,
		nullableUUIDString(input.SourceAreaID),
		filterJSON,
		input.IsDefault,
		now,
		now,
	); err != nil {
		return domain.CanvasMap{}, fmt.Errorf("creating canvas map: %w", err)
	}

	return r.GetByID(mapID)
}

// GetByID returns a saved canvas map by ID.
func (r *CanvasMapRepo) GetByID(id uuid.UUID) (domain.CanvasMap, error) {
	if id == uuid.Nil {
		return domain.CanvasMap{}, fmt.Errorf("canvas map id is required")
	}

	canvasMap, err := scanCanvasMap(r.db.QueryRow(canvasMapSelectQuery(`
		WHERE cm.id = ?`), id.String()))
	if err != nil {
		return domain.CanvasMap{}, err
	}
	return canvasMap, nil
}

// GetDefault returns the default saved canvas map.
func (r *CanvasMapRepo) GetDefault() (domain.CanvasMap, error) {
	canvasMap, err := scanCanvasMap(r.db.QueryRow(canvasMapSelectQuery(`
		WHERE cm.is_default = ?`), true))
	if err != nil {
		return domain.CanvasMap{}, err
	}
	return canvasMap, nil
}

// List returns all saved canvas maps with persisted position counts.
func (r *CanvasMapRepo) List() ([]domain.CanvasMap, error) {
	rows, err := r.db.Query(canvasMapSelectQuery("") + `
		ORDER BY cm.is_default DESC, cm.name ASC, cm.id ASC`)
	if err != nil {
		return nil, fmt.Errorf("querying canvas maps: %w", err)
	}
	defer rows.Close()

	maps := make([]domain.CanvasMap, 0)
	for rows.Next() {
		canvasMap, err := scanCanvasMap(rows)
		if err != nil {
			return nil, err
		}
		maps = append(maps, canvasMap)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating canvas maps: %w", err)
	}
	return maps, nil
}

// Update applies mutable saved canvas map fields.
func (r *CanvasMapRepo) Update(id uuid.UUID, input domain.CanvasMapUpdate) (domain.CanvasMap, error) {
	current, err := r.GetByID(id)
	if err != nil {
		return domain.CanvasMap{}, err
	}

	name := current.Name
	if input.Name != nil {
		if err := domain.ValidateCanvasMapName(*input.Name); err != nil {
			return domain.CanvasMap{}, err
		}
		name = *input.Name
	}

	description := current.Description
	if input.Description != nil {
		if err := domain.ValidateCanvasMapDescription(*input.Description); err != nil {
			return domain.CanvasMap{}, err
		}
		description = *input.Description
	}

	sourceAreaID := current.SourceAreaID
	if input.SourceAreaIDSet {
		sourceAreaID = input.SourceAreaID
	}

	filterJSON := current.FilterJSON
	if input.Filter != nil {
		filterJSON, err = domain.CanonicalCanvasMapFilterJSON(*input.Filter)
		if err != nil {
			return domain.CanvasMap{}, err
		}
	}

	result, err := r.db.Exec(
		`UPDATE canvas_maps
		 SET name = ?, description = ?, source_area_id = ?, filter_json = ?, updated_at = ?
		 WHERE id = ?`,
		name,
		description,
		nullableUUIDString(sourceAreaID),
		filterJSON,
		time.Now().UTC(),
		id.String(),
	)
	if err != nil {
		return domain.CanvasMap{}, fmt.Errorf("updating canvas map: %w", err)
	}
	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return domain.CanvasMap{}, fmt.Errorf("canvas map not found: %s", id)
	}

	return r.GetByID(id)
}

// Delete removes a saved canvas map unless it is the default map.
func (r *CanvasMapRepo) Delete(id uuid.UUID) error {
	result, err := r.db.Exec(`DELETE FROM canvas_maps WHERE id = ? AND is_default = ?`, id.String(), false)
	if err != nil {
		return fmt.Errorf("deleting canvas map: %w", err)
	}
	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		canvasMap, err := r.GetByID(id)
		if err != nil {
			return err
		}
		if canvasMap.IsDefault {
			return fmt.Errorf("cannot delete default canvas map")
		}
		return fmt.Errorf("canvas map not found: %s", id)
	}
	return nil
}

// Duplicate copies a saved map and its positions into a new non-default map.
func (r *CanvasMapRepo) Duplicate(id uuid.UUID, name string) (domain.CanvasMap, error) {
	if err := domain.ValidateCanvasMapName(name); err != nil {
		return domain.CanvasMap{}, err
	}

	tx, err := r.db.Begin()
	if err != nil {
		return domain.CanvasMap{}, fmt.Errorf("starting canvas map duplicate transaction: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	source, err := scanCanvasMap(tx.QueryRow(canvasMapSelectQuery(`
		WHERE cm.id = ?`), id.String()))
	if err != nil {
		return domain.CanvasMap{}, err
	}

	copyID := uuid.New()
	now := time.Now().UTC()
	if _, err := tx.Exec(
		`INSERT INTO canvas_maps (id, name, description, source_area_id, filter_json, is_default, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		copyID.String(),
		name,
		source.Description,
		nullableUUIDString(source.SourceAreaID),
		source.FilterJSON,
		false,
		now,
		now,
	); err != nil {
		return domain.CanvasMap{}, fmt.Errorf("creating duplicate canvas map: %w", err)
	}

	if _, err := tx.Exec(
		`INSERT INTO canvas_map_positions (map_id, device_id, x, y, pinned, updated_at)
		 SELECT ?, device_id, x, y, pinned, updated_at
		 FROM canvas_map_positions
		 WHERE map_id = ?`,
		copyID.String(),
		id.String(),
	); err != nil {
		return domain.CanvasMap{}, fmt.Errorf("copying canvas map positions: %w", err)
	}

	if _, err := tx.Exec(
		`INSERT INTO canvas_map_devices (map_id, device_id, role, added_at)
		 SELECT ?, device_id, role, added_at
		 FROM canvas_map_devices
		 WHERE map_id = ?`,
		copyID.String(),
		id.String(),
	); err != nil {
		return domain.CanvasMap{}, fmt.Errorf("copying canvas map device membership: %w", err)
	}

	if _, err := tx.Exec(
		`INSERT INTO canvas_map_links (map_id, link_id, added_at)
		 SELECT ?, link_id, added_at
		 FROM canvas_map_links
		 WHERE map_id = ?`,
		copyID.String(),
		id.String(),
	); err != nil {
		return domain.CanvasMap{}, fmt.Errorf("copying canvas map link membership: %w", err)
	}

	if _, err := tx.Exec(
		`INSERT INTO canvas_map_areas (map_id, area_id, name, description, color, added_at)
		 SELECT ?, area_id, name, description, color, added_at
		 FROM canvas_map_areas
		 WHERE map_id = ?`,
		copyID.String(),
		id.String(),
	); err != nil {
		return domain.CanvasMap{}, fmt.Errorf("copying canvas map area membership: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return domain.CanvasMap{}, fmt.Errorf("committing canvas map duplicate: %w", err)
	}

	return r.GetByID(copyID)
}

// GetMembership retrieves the materialized device, link, and area membership for one saved map.
func (r *CanvasMapRepo) GetMembership(id uuid.UUID) (domain.CanvasMapMembership, error) {
	if id == uuid.Nil {
		return domain.CanvasMapMembership{}, fmt.Errorf("canvas map id is required")
	}
	if err := ensureCanvasMapExists(r.db, id); err != nil {
		return domain.CanvasMapMembership{}, err
	}

	membership := domain.CanvasMapMembership{
		Devices: []domain.CanvasMapDeviceMembership{},
		LinkIDs: []uuid.UUID{},
		Areas:   []domain.CanvasMapAreaMembership{},
	}

	deviceRows, err := r.db.Query(
		`SELECT device_id, role
		 FROM canvas_map_devices
		 WHERE map_id = ?
		 ORDER BY device_id`,
		id.String(),
	)
	if err != nil {
		return domain.CanvasMapMembership{}, fmt.Errorf("querying canvas map device membership: %w", err)
	}
	defer deviceRows.Close()

	for deviceRows.Next() {
		var deviceIDRaw, roleRaw string
		if err := deviceRows.Scan(&deviceIDRaw, &roleRaw); err != nil {
			return domain.CanvasMapMembership{}, fmt.Errorf("scanning canvas map device membership: %w", err)
		}
		deviceID, err := uuid.Parse(deviceIDRaw)
		if err != nil {
			return domain.CanvasMapMembership{}, fmt.Errorf("parsing canvas map device membership id %q: %w", deviceIDRaw, err)
		}
		role := domain.CanvasMapDeviceRole(roleRaw)
		if !role.IsValid() {
			return domain.CanvasMapMembership{}, fmt.Errorf("invalid canvas map device role %q", roleRaw)
		}
		membership.Devices = append(membership.Devices, domain.CanvasMapDeviceMembership{DeviceID: deviceID, Role: role})
	}
	if err := deviceRows.Err(); err != nil {
		return domain.CanvasMapMembership{}, fmt.Errorf("iterating canvas map device membership: %w", err)
	}

	linkRows, err := r.db.Query(
		`SELECT link_id
		 FROM canvas_map_links
		 WHERE map_id = ?
		 ORDER BY link_id`,
		id.String(),
	)
	if err != nil {
		return domain.CanvasMapMembership{}, fmt.Errorf("querying canvas map link membership: %w", err)
	}
	defer linkRows.Close()

	for linkRows.Next() {
		var linkIDRaw string
		if err := linkRows.Scan(&linkIDRaw); err != nil {
			return domain.CanvasMapMembership{}, fmt.Errorf("scanning canvas map link membership: %w", err)
		}
		linkID, err := uuid.Parse(linkIDRaw)
		if err != nil {
			return domain.CanvasMapMembership{}, fmt.Errorf("parsing canvas map link membership id %q: %w", linkIDRaw, err)
		}
		membership.LinkIDs = append(membership.LinkIDs, linkID)
	}
	if err := linkRows.Err(); err != nil {
		return domain.CanvasMapMembership{}, fmt.Errorf("iterating canvas map link membership: %w", err)
	}

	areaRows, err := r.db.Query(
		`SELECT area_id, name, description, color
		 FROM canvas_map_areas
		 WHERE map_id = ?
		 ORDER BY area_id`,
		id.String(),
	)
	if err != nil {
		return domain.CanvasMapMembership{}, fmt.Errorf("querying canvas map area membership: %w", err)
	}
	defer areaRows.Close()

	for areaRows.Next() {
		var area domain.CanvasMapAreaMembership
		var areaIDRaw string
		if err := areaRows.Scan(&areaIDRaw, &area.Name, &area.Description, &area.Color); err != nil {
			return domain.CanvasMapMembership{}, fmt.Errorf("scanning canvas map area membership: %w", err)
		}
		areaID, err := uuid.Parse(areaIDRaw)
		if err != nil {
			return domain.CanvasMapMembership{}, fmt.Errorf("parsing canvas map area membership id %q: %w", areaIDRaw, err)
		}
		area.AreaID = areaID
		membership.Areas = append(membership.Areas, area)
	}
	if err := areaRows.Err(); err != nil {
		return domain.CanvasMapMembership{}, fmt.Errorf("iterating canvas map area membership: %w", err)
	}

	return membership, nil
}

// ReplaceMembership atomically replaces a map's materialized device, link, and area membership.
func (r *CanvasMapRepo) ReplaceMembership(id uuid.UUID, membership domain.CanvasMapMembership) error {
	if id == uuid.Nil {
		return fmt.Errorf("canvas map id is required")
	}
	if err := validateCanvasMapMembership(membership); err != nil {
		return err
	}

	tx, err := r.db.Begin()
	if err != nil {
		return fmt.Errorf("starting canvas map membership transaction: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	if err := ensureCanvasMapExists(tx, id); err != nil {
		return err
	}

	for _, tableName := range []string{"canvas_map_devices", "canvas_map_links", "canvas_map_areas"} {
		if _, err := tx.Exec(
			"DELETE FROM "+tableName+" WHERE map_id = ?",
			id.String(),
		); err != nil {
			return fmt.Errorf("clearing %s: %w", tableName, err)
		}
	}

	now := time.Now().UTC()
	for _, device := range membership.Devices {
		if _, err := tx.Exec(
			`INSERT INTO canvas_map_devices (map_id, device_id, role, added_at)
			 VALUES (?, ?, ?, ?)`,
			id.String(),
			device.DeviceID.String(),
			string(device.Role),
			now,
		); err != nil {
			return fmt.Errorf("inserting canvas map device membership %s: %w", device.DeviceID, err)
		}
	}

	for _, linkID := range membership.LinkIDs {
		if _, err := tx.Exec(
			`INSERT INTO canvas_map_links (map_id, link_id, added_at)
			 VALUES (?, ?, ?)`,
			id.String(),
			linkID.String(),
			now,
		); err != nil {
			return fmt.Errorf("inserting canvas map link membership %s: %w", linkID, err)
		}
	}

	for _, area := range membership.Areas {
		if _, err := tx.Exec(
			`INSERT INTO canvas_map_areas (map_id, area_id, name, description, color, added_at)
			 VALUES (?, ?, ?, ?, ?, ?)`,
			id.String(),
			area.AreaID.String(),
			area.Name,
			area.Description,
			area.Color,
			now,
		); err != nil {
			return fmt.Errorf("inserting canvas map area membership %s: %w", area.AreaID, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing canvas map membership: %w", err)
	}
	return nil
}

// RemoveDevice removes one device from a map's materialized membership and drops its map-local position.
func (r *CanvasMapRepo) RemoveDevice(id uuid.UUID, deviceID uuid.UUID) error {
	if id == uuid.Nil {
		return fmt.Errorf("canvas map id is required")
	}
	if deviceID == uuid.Nil {
		return fmt.Errorf("device id is required")
	}

	tx, err := r.db.Begin()
	if err != nil {
		return fmt.Errorf("starting canvas map device removal transaction: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	if err := ensureCanvasMapExists(tx, id); err != nil {
		return err
	}
	if _, err := tx.Exec(
		`DELETE FROM canvas_map_devices WHERE map_id = ? AND device_id = ?`,
		id.String(),
		deviceID.String(),
	); err != nil {
		return fmt.Errorf("removing canvas map device membership %s: %w", deviceID, err)
	}
	if _, err := tx.Exec(
		`DELETE FROM canvas_map_positions WHERE map_id = ? AND device_id = ?`,
		id.String(),
		deviceID.String(),
	); err != nil {
		return fmt.Errorf("removing canvas map position for device %s: %w", deviceID, err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing canvas map device removal: %w", err)
	}
	return nil
}

// RemoveLink removes one link from a map's materialized membership.
func (r *CanvasMapRepo) RemoveLink(id uuid.UUID, linkID uuid.UUID) error {
	if id == uuid.Nil {
		return fmt.Errorf("canvas map id is required")
	}
	if linkID == uuid.Nil {
		return fmt.Errorf("link id is required")
	}
	if err := ensureCanvasMapExists(r.db, id); err != nil {
		return err
	}
	if _, err := r.db.Exec(
		`DELETE FROM canvas_map_links WHERE map_id = ? AND link_id = ?`,
		id.String(),
		linkID.String(),
	); err != nil {
		return fmt.Errorf("removing canvas map link membership %s: %w", linkID, err)
	}
	return nil
}

// GetAllForMap retrieves all persisted device positions for one saved map.
func (r *CanvasMapPositionRepo) GetAllForMap(mapID uuid.UUID) ([]domain.DevicePosition, error) {
	if mapID == uuid.Nil {
		return nil, fmt.Errorf("canvas map id is required")
	}

	rows, err := r.db.Query(
		`SELECT device_id, x, y, pinned, updated_at
		 FROM canvas_map_positions
		 WHERE map_id = ?
		 ORDER BY device_id`,
		mapID.String(),
	)
	if err != nil {
		return nil, fmt.Errorf("querying canvas map positions: %w", err)
	}
	defer rows.Close()

	positions := make([]domain.DevicePosition, 0)
	for rows.Next() {
		position, err := scanCanvasMapPosition(rows)
		if err != nil {
			return nil, err
		}
		positions = append(positions, position)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating canvas map positions: %w", err)
	}
	return positions, nil
}

// SaveAllForMap upserts a batch of device positions for one saved map.
func (r *CanvasMapPositionRepo) SaveAllForMap(mapID uuid.UUID, positions []domain.DevicePosition) error {
	if mapID == uuid.Nil {
		return fmt.Errorf("canvas map id is required")
	}
	if len(positions) == 0 {
		return nil
	}

	tx, err := r.db.Begin()
	if err != nil {
		return fmt.Errorf("starting canvas map position transaction: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	if err := rejectCanvasMapNonMemberPositions(tx, mapID, positions); err != nil {
		return err
	}

	stmt, err := tx.Prepare(
		`INSERT INTO canvas_map_positions (map_id, device_id, x, y, pinned, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?)
		 ON CONFLICT(map_id, device_id) DO UPDATE SET
			x = excluded.x,
			y = excluded.y,
			pinned = excluded.pinned,
			updated_at = excluded.updated_at`,
	)
	if err != nil {
		return fmt.Errorf("preparing canvas map position upsert: %w", err)
	}
	defer stmt.Close()

	now := time.Now().UTC()
	for i := range positions {
		position := positions[i]
		if position.DeviceID == uuid.Nil {
			return fmt.Errorf("canvas map position device_id is required")
		}
		if position.UpdatedAt.IsZero() {
			position.UpdatedAt = now
		}

		if _, err := stmt.Exec(
			mapID.String(),
			position.DeviceID.String(),
			position.X,
			position.Y,
			position.Pinned,
			position.UpdatedAt,
		); err != nil {
			return fmt.Errorf("upserting canvas map position for device %s: %w", position.DeviceID, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing canvas map positions: %w", err)
	}
	return nil
}

// DeleteByDeviceID removes saved map positions for a single device across maps.
func (r *CanvasMapPositionRepo) DeleteByDeviceID(deviceID uuid.UUID) error {
	if deviceID == uuid.Nil {
		return fmt.Errorf("device id is required")
	}
	if _, err := r.db.Exec(`DELETE FROM canvas_map_positions WHERE device_id = ?`, deviceID.String()); err != nil {
		return fmt.Errorf("deleting canvas map positions for device %s: %w", deviceID, err)
	}
	return nil
}

type rowScanner interface {
	Scan(dest ...any) error
}

func canvasMapSelectQuery(whereClause string) string {
	return `SELECT
			cm.id,
			cm.name,
			cm.description,
			cm.source_area_id,
			cm.filter_json,
			cm.is_default,
			cm.created_at,
			cm.updated_at,
			COUNT(DISTINCT cmd.device_id) AS device_count,
			COUNT(DISTINCT cml.link_id) AS link_count,
			COUNT(DISTINCT cmp.device_id) AS position_count
		FROM canvas_maps cm
		LEFT JOIN canvas_map_devices cmd ON cmd.map_id = cm.id
		LEFT JOIN canvas_map_links cml ON cml.map_id = cm.id
		LEFT JOIN canvas_map_positions cmp ON cmp.map_id = cm.id
		` + whereClause + `
		GROUP BY
			cm.id,
			cm.name,
			cm.description,
			cm.source_area_id,
			cm.filter_json,
			cm.is_default,
			cm.created_at,
			cm.updated_at`
}

func scanCanvasMap(scanner rowScanner) (domain.CanvasMap, error) {
	var canvasMap domain.CanvasMap
	var (
		idRaw           string
		sourceAreaIDRaw sql.NullString
		isDefaultRaw    any
	)

	if err := scanner.Scan(
		&idRaw,
		&canvasMap.Name,
		&canvasMap.Description,
		&sourceAreaIDRaw,
		&canvasMap.FilterJSON,
		&isDefaultRaw,
		&canvasMap.CreatedAt,
		&canvasMap.UpdatedAt,
		&canvasMap.DeviceCount,
		&canvasMap.LinkCount,
		&canvasMap.PositionCount,
	); err != nil {
		if err == sql.ErrNoRows {
			return domain.CanvasMap{}, fmt.Errorf("canvas map not found")
		}
		return domain.CanvasMap{}, fmt.Errorf("scanning canvas map: %w", err)
	}

	id, err := uuid.Parse(idRaw)
	if err != nil {
		return domain.CanvasMap{}, fmt.Errorf("parsing canvas map id %q: %w", idRaw, err)
	}
	canvasMap.ID = id

	if sourceAreaIDRaw.Valid {
		sourceAreaID, err := uuid.Parse(sourceAreaIDRaw.String)
		if err != nil {
			return domain.CanvasMap{}, fmt.Errorf("parsing canvas map source area id %q: %w", sourceAreaIDRaw.String, err)
		}
		canvasMap.SourceAreaID = &sourceAreaID
	}

	isDefault, err := normalizeBoolValue(isDefaultRaw)
	if err != nil {
		return domain.CanvasMap{}, fmt.Errorf("normalizing canvas map is_default: %w", err)
	}
	canvasMap.IsDefault = isDefault

	return canvasMap, nil
}

func scanCanvasMapPosition(scanner rowScanner) (domain.DevicePosition, error) {
	var position domain.DevicePosition
	var (
		deviceIDRaw string
		pinnedRaw   any
	)

	if err := scanner.Scan(
		&deviceIDRaw,
		&position.X,
		&position.Y,
		&pinnedRaw,
		&position.UpdatedAt,
	); err != nil {
		return domain.DevicePosition{}, fmt.Errorf("scanning canvas map position: %w", err)
	}

	deviceID, err := uuid.Parse(deviceIDRaw)
	if err != nil {
		return domain.DevicePosition{}, fmt.Errorf("parsing canvas map position device id %q: %w", deviceIDRaw, err)
	}
	position.DeviceID = deviceID

	pinned, err := normalizeBoolValue(pinnedRaw)
	if err != nil {
		return domain.DevicePosition{}, fmt.Errorf("normalizing canvas map position pinned: %w", err)
	}
	position.Pinned = pinned

	return position, nil
}

type canvasMapQueryRower interface {
	QueryRow(query string, args ...interface{}) *sql.Row
}

func ensureCanvasMapExists(queryer canvasMapQueryRower, id uuid.UUID) error {
	var count int
	if err := queryer.QueryRow(`SELECT COUNT(*) FROM canvas_maps WHERE id = ?`, id.String()).Scan(&count); err != nil {
		return fmt.Errorf("checking canvas map existence: %w", err)
	}
	if count == 0 {
		return fmt.Errorf("canvas map not found: %s", id)
	}
	return nil
}

func validateCanvasMapMembership(membership domain.CanvasMapMembership) error {
	deviceIDs := make(map[uuid.UUID]struct{}, len(membership.Devices))
	for _, device := range membership.Devices {
		if device.DeviceID == uuid.Nil {
			return fmt.Errorf("canvas map membership device_id is required")
		}
		if !device.Role.IsValid() {
			return fmt.Errorf("invalid canvas map device role %q", device.Role)
		}
		if _, exists := deviceIDs[device.DeviceID]; exists {
			return fmt.Errorf("duplicate canvas map device membership: %s", device.DeviceID)
		}
		deviceIDs[device.DeviceID] = struct{}{}
	}

	linkIDs := make(map[uuid.UUID]struct{}, len(membership.LinkIDs))
	for _, linkID := range membership.LinkIDs {
		if linkID == uuid.Nil {
			return fmt.Errorf("canvas map membership link_id is required")
		}
		if _, exists := linkIDs[linkID]; exists {
			return fmt.Errorf("duplicate canvas map link membership: %s", linkID)
		}
		linkIDs[linkID] = struct{}{}
	}

	areaIDs := make(map[uuid.UUID]struct{}, len(membership.Areas))
	for _, area := range membership.Areas {
		if area.AreaID == uuid.Nil {
			return fmt.Errorf("canvas map membership area_id is required")
		}
		if _, exists := areaIDs[area.AreaID]; exists {
			return fmt.Errorf("duplicate canvas map area membership: %s", area.AreaID)
		}
		areaIDs[area.AreaID] = struct{}{}
	}

	return nil
}

func rejectCanvasMapNonMemberPositions(tx *Tx, mapID uuid.UUID, positions []domain.DevicePosition) error {
	membershipExists, err := canvasMapMembershipExists(tx, mapID)
	if err != nil {
		return err
	}
	if !membershipExists {
		return nil
	}

	checked := make(map[uuid.UUID]struct{}, len(positions))
	for _, position := range positions {
		if position.DeviceID == uuid.Nil {
			continue
		}
		if _, exists := checked[position.DeviceID]; exists {
			continue
		}
		checked[position.DeviceID] = struct{}{}

		var count int
		if err := tx.QueryRow(
			`SELECT COUNT(*)
			 FROM canvas_map_devices
			 WHERE map_id = ? AND device_id = ?`,
			mapID.String(),
			position.DeviceID.String(),
		).Scan(&count); err != nil {
			return fmt.Errorf("checking canvas map position membership for device %s: %w", position.DeviceID, err)
		}
		if count == 0 {
			return fmt.Errorf("device %s is not a member of canvas map %s", position.DeviceID, mapID)
		}
	}

	return nil
}

func canvasMapMembershipExists(queryer canvasMapQueryRower, mapID uuid.UUID) (bool, error) {
	var exists int
	if err := queryer.QueryRow(
		`SELECT CASE WHEN
			EXISTS (SELECT 1 FROM canvas_map_devices WHERE map_id = ?)
			OR EXISTS (SELECT 1 FROM canvas_map_links WHERE map_id = ?)
			OR EXISTS (SELECT 1 FROM canvas_map_areas WHERE map_id = ?)
		 THEN 1 ELSE 0 END`,
		mapID.String(),
		mapID.String(),
		mapID.String(),
	).Scan(&exists); err != nil {
		return false, fmt.Errorf("checking canvas map membership existence: %w", err)
	}
	return exists != 0, nil
}

func nullableUUIDString(id *uuid.UUID) any {
	if id == nil {
		return nil
	}
	return id.String()
}
