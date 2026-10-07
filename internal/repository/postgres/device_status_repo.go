package postgres

import (
	"fmt"
	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/domain"
	"time"
)

// UpdateStatus updates runtime state from current persisted fields in one statement.
func (r *DeviceRepo) UpdateStatus(id uuid.UUID, status domain.DeviceStatus) error {
	result, err := r.db.Exec(`UPDATE devices SET status = ?, updated_at = ?,
  poll_class = CASE WHEN metrics_source = 'prometheus' AND ? = 'up' AND poll_interval_override IS NULL
		THEN CASE WHEN device_type IN ('router', 'switch') THEN 'core' WHEN device_type = 'virtual' THEN 'low' ELSE 'standard' END
    ELSE poll_class END WHERE id = ?`, string(status), time.Now().UTC(), string(status), id.String())
	if err != nil {
		return fmt.Errorf("updating device status: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("checking device status update: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("device not found: %s", id)
	}
	r.notify()
	r.publishChange(domain.ChangeKindUpdated, id)
	return nil
}
