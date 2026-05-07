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

	filterJSON := current.FilterJSON
	if input.Filter != nil {
		filterJSON, err = domain.CanonicalCanvasMapFilterJSON(*input.Filter)
		if err != nil {
			return domain.CanvasMap{}, err
		}
	}

	result, err := r.db.Exec(
		`UPDATE canvas_maps
		 SET name = ?, description = ?, filter_json = ?, updated_at = ?
		 WHERE id = ?`,
		name,
		description,
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
	canvasMap, err := r.GetByID(id)
	if err != nil {
		return err
	}
	if canvasMap.IsDefault {
		return fmt.Errorf("cannot delete default canvas map")
	}

	result, err := r.db.Exec(`DELETE FROM canvas_maps WHERE id = ?`, id.String())
	if err != nil {
		return fmt.Errorf("deleting canvas map: %w", err)
	}
	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
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

	if err := tx.Commit(); err != nil {
		return domain.CanvasMap{}, fmt.Errorf("committing canvas map duplicate: %w", err)
	}

	return r.GetByID(copyID)
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
			COUNT(cmp.device_id) AS position_count
		FROM canvas_maps cm
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

func nullableUUIDString(id *uuid.UUID) any {
	if id == nil {
		return nil
	}
	return id.String()
}
