package postgres

import (
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/domain"
)

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

// SetPrimary marks one saved canvas map as the primary map and clears the previous primary flag.
func (r *CanvasMapRepo) SetPrimary(id uuid.UUID) (domain.CanvasMap, error) {
	if id == uuid.Nil {
		return domain.CanvasMap{}, fmt.Errorf("canvas map id is required")
	}

	tx, err := r.db.Begin()
	if err != nil {
		return domain.CanvasMap{}, fmt.Errorf("starting canvas map primary transaction: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	if err := ensureCanvasMapExists(tx, id); err != nil {
		return domain.CanvasMap{}, err
	}

	now := time.Now().UTC()
	if _, err := tx.Exec(
		`UPDATE canvas_maps
		 SET is_default = ?, updated_at = ?
		 WHERE is_default = ?`,
		false,
		now,
		true,
	); err != nil {
		return domain.CanvasMap{}, fmt.Errorf("clearing previous primary canvas map: %w", err)
	}

	result, err := tx.Exec(
		`UPDATE canvas_maps
		 SET is_default = ?, updated_at = ?
		 WHERE id = ?`,
		true,
		now,
		id.String(),
	)
	if err != nil {
		return domain.CanvasMap{}, fmt.Errorf("setting primary canvas map: %w", err)
	}
	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return domain.CanvasMap{}, fmt.Errorf("canvas map not found: %s", id)
	}

	if err := tx.Commit(); err != nil {
		return domain.CanvasMap{}, fmt.Errorf("committing canvas map primary transaction: %w", err)
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
		`INSERT INTO canvas_maps (id, name, description, source_area_id, filter_json, is_default, membership_materialized, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		copyID.String(),
		name,
		source.Description,
		nullableUUIDString(source.SourceAreaID),
		source.FilterJSON,
		false,
		source.MembershipMaterialized,
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
		`INSERT INTO canvas_map_devices (map_id, device_id, role, visual_color, added_at)
		 SELECT ?, device_id, role, visual_color, added_at
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
	areaCount, err := countCanvasMapAreas(tx, copyID)
	if err != nil {
		return domain.CanvasMap{}, err
	}
	if areaCount == 0 {
		if err := backfillCanvasMapAreasFromMemberDevices(tx, copyID); err != nil {
			return domain.CanvasMap{}, err
		}
	}
	if _, err := tx.Exec(
		`INSERT INTO canvas_map_device_areas (map_id, device_id, area_id, assigned_at)
		 SELECT ?, device_id, area_id, assigned_at
		 FROM canvas_map_device_areas
		 WHERE map_id = ?`,
		copyID.String(),
		id.String(),
	); err != nil {
		return domain.CanvasMap{}, fmt.Errorf("copying canvas map device area membership: %w", err)
	}
	deviceAreaCount, err := countCanvasMapDeviceAreas(tx, copyID)
	if err != nil {
		return domain.CanvasMap{}, err
	}
	if deviceAreaCount == 0 {
		if err := backfillCanvasMapDeviceAreasFromMemberDevices(tx, copyID); err != nil {
			return domain.CanvasMap{}, err
		}
	}

	if err := tx.Commit(); err != nil {
		return domain.CanvasMap{}, fmt.Errorf("committing canvas map duplicate: %w", err)
	}

	return r.GetByID(copyID)
}
