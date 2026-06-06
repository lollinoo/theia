package postgres

// This file defines canvas map position repo persistence behavior, ordering guarantees, and not-found conventions.

import (
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/domain"
)

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

// scanCanvasMapPosition converts one saved-map position row into a domain position.
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
