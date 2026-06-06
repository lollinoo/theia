package postgres

// This file defines position repo persistence behavior, ordering guarantees, and not-found conventions.

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/domain"
)

// PositionRepo implements domain.PositionRepository using PostgreSQL.
type PositionRepo struct {
	db *DB
}

// NewPositionRepo creates a new PostgreSQL-backed position repository.
func NewPositionRepo(db *sql.DB) *PositionRepo {
	return &PositionRepo{db: wrapDB(db)}
}

// GetAll retrieves all persisted device positions.
func (r *PositionRepo) GetAll() ([]domain.DevicePosition, error) {
	rows, err := r.db.Query(
		`SELECT device_id, x, y, pinned, updated_at
		FROM device_positions
		ORDER BY device_id`,
	)
	if err != nil {
		return nil, fmt.Errorf("querying positions: %w", err)
	}
	defer rows.Close()

	positions := make([]domain.DevicePosition, 0)
	for rows.Next() {
		var position domain.DevicePosition
		var deviceID string
		var pinned int

		if err := rows.Scan(
			&deviceID,
			&position.X,
			&position.Y,
			&pinned,
			&position.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scanning position: %w", err)
		}

		position.DeviceID = uuid.MustParse(deviceID)
		position.Pinned = pinned == 1
		positions = append(positions, position)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating positions: %w", err)
	}

	return positions, nil
}

// SaveAll upserts a batch of device positions in a single transaction.
func (r *PositionRepo) SaveAll(positions []domain.DevicePosition) error {
	if len(positions) == 0 {
		return nil
	}

	tx, err := r.db.Begin()
	if err != nil {
		return fmt.Errorf("starting position transaction: %w", err)
	}

	stmt, err := tx.Prepare(
		`INSERT INTO device_positions (device_id, x, y, pinned, updated_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(device_id) DO UPDATE SET
			x = excluded.x,
			y = excluded.y,
			pinned = excluded.pinned,
			updated_at = excluded.updated_at`,
	)
	if err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("preparing position upsert: %w", err)
	}
	defer stmt.Close()

	now := time.Now().UTC()
	for i := range positions {
		position := &positions[i]
		if position.DeviceID == uuid.Nil {
			_ = tx.Rollback()
			return fmt.Errorf("position device_id is required")
		}
		if position.UpdatedAt.IsZero() {
			position.UpdatedAt = now
		}

		pinned := 0
		if position.Pinned {
			pinned = 1
		}

		if _, err := stmt.Exec(
			position.DeviceID.String(),
			position.X,
			position.Y,
			pinned,
			position.UpdatedAt,
		); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("upserting position for device %s: %w", position.DeviceID, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing positions: %w", err)
	}

	return nil
}

// DeleteByDeviceID removes a persisted position for a single device.
func (r *PositionRepo) DeleteByDeviceID(deviceID uuid.UUID) error {
	result, err := r.db.Exec(`DELETE FROM device_positions WHERE device_id = ?`, deviceID.String())
	if err != nil {
		return fmt.Errorf("deleting position: %w", err)
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return fmt.Errorf("position not found for device: %s", deviceID)
	}

	return nil
}
