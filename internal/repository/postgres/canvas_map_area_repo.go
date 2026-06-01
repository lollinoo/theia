package postgres

import (
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/domain"
)

// ListAreas returns the map-local area catalog with map-local device counts.
func (r *CanvasMapRepo) ListAreas(id uuid.UUID) ([]domain.AreaWithCount, error) {
	if id == uuid.Nil {
		return nil, fmt.Errorf("canvas map id is required")
	}
	if err := ensureCanvasMapExists(r.db, id); err != nil {
		return nil, err
	}

	rows, err := r.db.Query(
		`SELECT cma.area_id, cma.name, cma.description, cma.color, cma.added_at,
		        COUNT(DISTINCT CASE WHEN cmd.role = ? THEN cmda.device_id END) AS device_count
		 FROM canvas_map_areas cma
		 LEFT JOIN canvas_map_device_areas cmda
		   ON cmda.map_id = cma.map_id AND cmda.area_id = cma.area_id
		 LEFT JOIN canvas_map_devices cmd
		   ON cmd.map_id = cmda.map_id AND cmd.device_id = cmda.device_id
		 WHERE cma.map_id = ?
		 GROUP BY cma.area_id, cma.name, cma.description, cma.color, cma.added_at
		 ORDER BY cma.name ASC, cma.area_id ASC`,
		string(domain.CanvasMapDeviceRoleBase),
		id.String(),
	)
	if err != nil {
		return nil, fmt.Errorf("querying canvas map areas: %w", err)
	}
	defer rows.Close()

	areas := make([]domain.AreaWithCount, 0)
	for rows.Next() {
		area, err := scanCanvasMapAreaWithCount(rows)
		if err != nil {
			return nil, err
		}
		areas = append(areas, area)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating canvas map areas: %w", err)
	}
	return areas, nil
}

// CreateArea adds one area only to the selected saved map.
func (r *CanvasMapRepo) CreateArea(id uuid.UUID, area domain.CanvasMapAreaMembership) (domain.AreaWithCount, error) {
	if id == uuid.Nil {
		return domain.AreaWithCount{}, fmt.Errorf("canvas map id is required")
	}
	if area.AreaID == uuid.Nil {
		area.AreaID = uuid.New()
	}

	tx, err := r.db.Begin()
	if err != nil {
		return domain.AreaWithCount{}, fmt.Errorf("starting canvas map area create transaction: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	if err := ensureCanvasMapExists(tx, id); err != nil {
		return domain.AreaWithCount{}, err
	}
	if err := ensureCanvasMapAreaNameAvailable(tx, id, area.AreaID, area.Name); err != nil {
		return domain.AreaWithCount{}, err
	}

	now := time.Now().UTC()
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
		return domain.AreaWithCount{}, fmt.Errorf("creating canvas map area %s: %w", area.AreaID, err)
	}
	if _, err := tx.Exec(
		`UPDATE canvas_maps SET updated_at = ? WHERE id = ?`,
		now,
		id.String(),
	); err != nil {
		return domain.AreaWithCount{}, fmt.Errorf("touching canvas map after area create %s: %w", id, err)
	}

	created, err := getCanvasMapArea(tx, id, area.AreaID)
	if err != nil {
		return domain.AreaWithCount{}, err
	}
	if err := tx.Commit(); err != nil {
		return domain.AreaWithCount{}, fmt.Errorf("committing canvas map area create: %w", err)
	}
	return created, nil
}

// UpdateArea edits one area only within the selected saved map.
func (r *CanvasMapRepo) UpdateArea(
	id uuid.UUID,
	areaID uuid.UUID,
	area domain.CanvasMapAreaMembership,
) (domain.AreaWithCount, error) {
	if id == uuid.Nil {
		return domain.AreaWithCount{}, fmt.Errorf("canvas map id is required")
	}
	if areaID == uuid.Nil {
		return domain.AreaWithCount{}, fmt.Errorf("canvas map area id is required")
	}

	tx, err := r.db.Begin()
	if err != nil {
		return domain.AreaWithCount{}, fmt.Errorf("starting canvas map area update transaction: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	if err := ensureCanvasMapExists(tx, id); err != nil {
		return domain.AreaWithCount{}, err
	}
	if err := ensureCanvasMapAreaNameAvailable(tx, id, areaID, area.Name); err != nil {
		return domain.AreaWithCount{}, err
	}

	now := time.Now().UTC()
	result, err := tx.Exec(
		`UPDATE canvas_map_areas
		 SET name = ?, description = ?, color = ?
		 WHERE map_id = ? AND area_id = ?`,
		area.Name,
		area.Description,
		area.Color,
		id.String(),
		areaID.String(),
	)
	if err != nil {
		return domain.AreaWithCount{}, fmt.Errorf("updating canvas map area %s: %w", areaID, err)
	}
	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return domain.AreaWithCount{}, fmt.Errorf("canvas map area not found: %s", areaID)
	}
	if _, err := tx.Exec(
		`UPDATE canvas_maps SET updated_at = ? WHERE id = ?`,
		now,
		id.String(),
	); err != nil {
		return domain.AreaWithCount{}, fmt.Errorf("touching canvas map after area update %s: %w", id, err)
	}

	updated, err := getCanvasMapArea(tx, id, areaID)
	if err != nil {
		return domain.AreaWithCount{}, err
	}
	if err := tx.Commit(); err != nil {
		return domain.AreaWithCount{}, fmt.Errorf("committing canvas map area update: %w", err)
	}
	return updated, nil
}

// DeleteArea removes one map-local area and its map-local device assignments.
func (r *CanvasMapRepo) DeleteArea(id uuid.UUID, areaID uuid.UUID) error {
	if id == uuid.Nil {
		return fmt.Errorf("canvas map id is required")
	}
	if areaID == uuid.Nil {
		return fmt.Errorf("canvas map area id is required")
	}

	tx, err := r.db.Begin()
	if err != nil {
		return fmt.Errorf("starting canvas map area delete transaction: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	if err := ensureCanvasMapExists(tx, id); err != nil {
		return err
	}
	result, err := tx.Exec(
		`DELETE FROM canvas_map_areas
		 WHERE map_id = ? AND area_id = ?`,
		id.String(),
		areaID.String(),
	)
	if err != nil {
		return fmt.Errorf("deleting canvas map area %s: %w", areaID, err)
	}
	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return fmt.Errorf("canvas map area not found: %s", areaID)
	}
	if _, err := tx.Exec(
		`UPDATE canvas_maps SET updated_at = ? WHERE id = ?`,
		time.Now().UTC(),
		id.String(),
	); err != nil {
		return fmt.Errorf("touching canvas map after area delete %s: %w", id, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing canvas map area delete: %w", err)
	}
	return nil
}
