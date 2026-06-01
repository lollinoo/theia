package postgres

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/domain"
)

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
		`SELECT device_id, role, visual_color
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
		var visualColorRaw sql.NullString
		if err := deviceRows.Scan(&deviceIDRaw, &roleRaw, &visualColorRaw); err != nil {
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
		device := domain.CanvasMapDeviceMembership{DeviceID: deviceID, Role: role}
		if visualColorRaw.Valid {
			device.VisualColor = &visualColorRaw.String
		}
		membership.Devices = append(membership.Devices, device)
	}
	if err := deviceRows.Err(); err != nil {
		return domain.CanvasMapMembership{}, fmt.Errorf("iterating canvas map device membership: %w", err)
	}
	deviceIndex := make(map[uuid.UUID]int, len(membership.Devices))
	for i, device := range membership.Devices {
		deviceIndex[device.DeviceID] = i
	}
	deviceAreaRows, err := r.db.Query(
		`SELECT device_id, area_id
		 FROM canvas_map_device_areas
		 WHERE map_id = ?
		 ORDER BY device_id, area_id`,
		id.String(),
	)
	if err != nil {
		return domain.CanvasMapMembership{}, fmt.Errorf("querying canvas map device area membership: %w", err)
	}
	defer deviceAreaRows.Close()

	for deviceAreaRows.Next() {
		var deviceIDRaw, areaIDRaw string
		if err := deviceAreaRows.Scan(&deviceIDRaw, &areaIDRaw); err != nil {
			return domain.CanvasMapMembership{}, fmt.Errorf("scanning canvas map device area membership: %w", err)
		}
		deviceID, err := uuid.Parse(deviceIDRaw)
		if err != nil {
			return domain.CanvasMapMembership{}, fmt.Errorf("parsing canvas map device area device id %q: %w", deviceIDRaw, err)
		}
		areaID, err := uuid.Parse(areaIDRaw)
		if err != nil {
			return domain.CanvasMapMembership{}, fmt.Errorf("parsing canvas map device area id %q: %w", areaIDRaw, err)
		}
		if index, ok := deviceIndex[deviceID]; ok {
			membership.Devices[index].AreaIDs = append(membership.Devices[index].AreaIDs, areaID)
		}
	}
	if err := deviceAreaRows.Err(); err != nil {
		return domain.CanvasMapMembership{}, fmt.Errorf("iterating canvas map device area membership: %w", err)
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

// ListMemberDeviceIDs returns distinct device IDs that belong to at least one saved map.
func (r *CanvasMapRepo) ListMemberDeviceIDs() ([]uuid.UUID, error) {
	rows, err := r.db.Query(
		`SELECT DISTINCT device_id
		 FROM canvas_map_devices
		 ORDER BY device_id`,
	)
	if err != nil {
		return nil, fmt.Errorf("querying saved map member device ids: %w", err)
	}
	defer rows.Close()

	deviceIDs := []uuid.UUID{}
	for rows.Next() {
		var rawID string
		if err := rows.Scan(&rawID); err != nil {
			return nil, fmt.Errorf("scanning saved map member device id: %w", err)
		}
		deviceID, err := uuid.Parse(rawID)
		if err != nil {
			return nil, fmt.Errorf("parsing saved map member device id %q: %w", rawID, err)
		}
		deviceIDs = append(deviceIDs, deviceID)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating saved map member device ids: %w", err)
	}
	return deviceIDs, nil
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

	for _, tableName := range []string{"canvas_map_device_areas", "canvas_map_devices", "canvas_map_links", "canvas_map_areas"} {
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
			`INSERT INTO canvas_map_devices (map_id, device_id, role, visual_color, added_at)
			 VALUES (?, ?, ?, ?, ?)`,
			id.String(),
			device.DeviceID.String(),
			string(device.Role),
			nullableStringValue(device.VisualColor),
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
	if err := insertCanvasMapDeviceAreas(tx, id, membership.Devices, now); err != nil {
		return err
	}

	if err := pruneCanvasMapPositionsForMembership(tx, id, membership.Devices); err != nil {
		return err
	}

	if _, err := tx.Exec(
		`UPDATE canvas_maps
		 SET membership_materialized = ?, updated_at = ?
		 WHERE id = ?`,
		true,
		now,
		id.String(),
	); err != nil {
		return fmt.Errorf("marking canvas map membership materialized: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing canvas map membership: %w", err)
	}
	return nil
}

// AddDeviceMembership adds a device and related map-local links/areas without rebuilding the map.
func (r *CanvasMapRepo) AddDeviceMembership(
	id uuid.UUID,
	device domain.CanvasMapDeviceMembership,
	linkIDs []uuid.UUID,
	areas []domain.CanvasMapAreaMembership,
) error {
	if id == uuid.Nil {
		return fmt.Errorf("canvas map id is required")
	}
	if device.DeviceID == uuid.Nil {
		return fmt.Errorf("device id is required")
	}
	if !device.Role.IsValid() {
		return fmt.Errorf("invalid canvas map device role %q", device.Role)
	}

	membership := domain.CanvasMapMembership{
		Devices: []domain.CanvasMapDeviceMembership{device},
		LinkIDs: linkIDs,
		Areas:   areas,
	}
	if err := validateCanvasMapMembership(membership); err != nil {
		return err
	}

	tx, err := r.db.Begin()
	if err != nil {
		return fmt.Errorf("starting canvas map add-device transaction: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	if err := ensureCanvasMapExists(tx, id); err != nil {
		return err
	}

	now := time.Now().UTC()
	if _, err := tx.Exec(
		`INSERT INTO canvas_map_devices (map_id, device_id, role, visual_color, added_at)
		 VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT(map_id, device_id) DO UPDATE SET
			role = excluded.role,
			visual_color = COALESCE(excluded.visual_color, canvas_map_devices.visual_color)`,
		id.String(),
		device.DeviceID.String(),
		string(device.Role),
		nullableStringValue(device.VisualColor),
		now,
	); err != nil {
		return fmt.Errorf("adding canvas map device membership %s: %w", device.DeviceID, err)
	}

	for _, linkID := range linkIDs {
		if _, err := tx.Exec(
			`INSERT INTO canvas_map_links (map_id, link_id, added_at)
			 VALUES (?, ?, ?)
			 ON CONFLICT(map_id, link_id) DO NOTHING`,
			id.String(),
			linkID.String(),
			now,
		); err != nil {
			return fmt.Errorf("adding canvas map link membership %s: %w", linkID, err)
		}
	}

	for _, area := range areas {
		if _, err := tx.Exec(
			`INSERT INTO canvas_map_areas (map_id, area_id, name, description, color, added_at)
			 VALUES (?, ?, ?, ?, ?, ?)
			 ON CONFLICT(map_id, area_id) DO NOTHING`,
			id.String(),
			area.AreaID.String(),
			area.Name,
			area.Description,
			area.Color,
			now,
		); err != nil {
			return fmt.Errorf("adding canvas map area membership %s: %w", area.AreaID, err)
		}
	}
	if _, err := tx.Exec(
		`DELETE FROM canvas_map_device_areas
		 WHERE map_id = ? AND device_id = ?`,
		id.String(),
		device.DeviceID.String(),
	); err != nil {
		return fmt.Errorf("clearing canvas map device area membership %s: %w", device.DeviceID, err)
	}
	if err := insertCanvasMapDeviceAreas(tx, id, []domain.CanvasMapDeviceMembership{device}, now); err != nil {
		return err
	}

	if _, err := tx.Exec(
		`UPDATE canvas_maps
		 SET membership_materialized = ?, updated_at = ?
		 WHERE id = ?`,
		true,
		now,
		id.String(),
	); err != nil {
		return fmt.Errorf("touching canvas map after add-device %s: %w", id, err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing canvas map add-device: %w", err)
	}
	return nil
}

// UpdateDeviceAreaMemberships replaces map-local area assignments for selected member devices.
func (r *CanvasMapRepo) UpdateDeviceAreaMemberships(
	id uuid.UUID,
	deviceIDs []uuid.UUID,
	areaIDs []uuid.UUID,
) error {
	if id == uuid.Nil {
		return fmt.Errorf("canvas map id is required")
	}
	canonicalDeviceIDs, err := validateCanvasMapUUIDList(deviceIDs, "device_id")
	if err != nil {
		return err
	}
	if len(canonicalDeviceIDs) == 0 {
		return fmt.Errorf("at least one device_id is required")
	}
	canonicalAreaIDs, err := validateCanvasMapUUIDList(areaIDs, "area_id")
	if err != nil {
		return err
	}

	tx, err := r.db.Begin()
	if err != nil {
		return fmt.Errorf("starting canvas map device area update transaction: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	if err := ensureCanvasMapExists(tx, id); err != nil {
		return err
	}
	for _, deviceID := range canonicalDeviceIDs {
		var count int
		if err := tx.QueryRow(
			`SELECT COUNT(*)
			 FROM canvas_map_devices
			 WHERE map_id = ? AND device_id = ?`,
			id.String(),
			deviceID.String(),
		).Scan(&count); err != nil {
			return fmt.Errorf("checking canvas map device membership %s: %w", deviceID, err)
		}
		if count == 0 {
			return fmt.Errorf("canvas map device %s is not a member of map %s", deviceID, id)
		}
	}
	for _, areaID := range canonicalAreaIDs {
		var count int
		if err := tx.QueryRow(
			`SELECT COUNT(*)
			 FROM canvas_map_areas
			 WHERE map_id = ? AND area_id = ?`,
			id.String(),
			areaID.String(),
		).Scan(&count); err != nil {
			return fmt.Errorf("checking canvas map area membership %s: %w", areaID, err)
		}
		if count == 0 {
			return fmt.Errorf("canvas map area %s is not a member of map %s", areaID, id)
		}
	}

	for _, deviceID := range canonicalDeviceIDs {
		if _, err := tx.Exec(
			`DELETE FROM canvas_map_device_areas
			 WHERE map_id = ? AND device_id = ?`,
			id.String(),
			deviceID.String(),
		); err != nil {
			return fmt.Errorf("clearing canvas map device areas for %s: %w", deviceID, err)
		}
	}

	now := time.Now().UTC()
	for _, deviceID := range canonicalDeviceIDs {
		for _, areaID := range canonicalAreaIDs {
			if _, err := tx.Exec(
				`INSERT INTO canvas_map_device_areas (map_id, device_id, area_id, assigned_at)
				 VALUES (?, ?, ?, ?)`,
				id.String(),
				deviceID.String(),
				areaID.String(),
				now,
			); err != nil {
				return fmt.Errorf("assigning canvas map device %s to area %s: %w", deviceID, areaID, err)
			}
		}
	}
	if _, err := tx.Exec(
		`UPDATE canvas_maps SET updated_at = ? WHERE id = ?`,
		now,
		id.String(),
	); err != nil {
		return fmt.Errorf("touching canvas map after device area update %s: %w", id, err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing canvas map device area update: %w", err)
	}
	return nil
}

// UpdateDeviceVisualColor replaces one map-local device visual color override.
func (r *CanvasMapRepo) UpdateDeviceVisualColor(
	id uuid.UUID,
	deviceID uuid.UUID,
	visualColor *string,
) error {
	if id == uuid.Nil {
		return fmt.Errorf("canvas map id is required")
	}
	if deviceID == uuid.Nil {
		return fmt.Errorf("device id is required")
	}

	tx, err := r.db.Begin()
	if err != nil {
		return fmt.Errorf("starting canvas map device visual color update transaction: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	if err := ensureCanvasMapExists(tx, id); err != nil {
		return err
	}

	now := time.Now().UTC()
	result, err := tx.Exec(
		`UPDATE canvas_map_devices
		 SET visual_color = ?
		 WHERE map_id = ? AND device_id = ?`,
		nullableStringValue(visualColor),
		id.String(),
		deviceID.String(),
	)
	if err != nil {
		return fmt.Errorf("updating canvas map device visual color %s: %w", deviceID, err)
	}
	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return fmt.Errorf("canvas map device %s is not a member of map %s", deviceID, id)
	}
	if _, err := tx.Exec(
		`UPDATE canvas_maps SET updated_at = ? WHERE id = ?`,
		now,
		id.String(),
	); err != nil {
		return fmt.Errorf("touching canvas map after device visual color update %s: %w", id, err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing canvas map device visual color update: %w", err)
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
	if _, err := tx.Exec(
		`DELETE FROM canvas_map_links
		 WHERE map_id = ?
		   AND link_id IN (
			 SELECT id FROM links
			 WHERE source_device_id = ? OR target_device_id = ?
		   )`,
		id.String(),
		deviceID.String(),
		deviceID.String(),
	); err != nil {
		return fmt.Errorf("removing canvas map links for device %s: %w", deviceID, err)
	}
	if _, err := tx.Exec(
		`UPDATE canvas_maps SET updated_at = ? WHERE id = ?`,
		time.Now().UTC(),
		id.String(),
	); err != nil {
		return fmt.Errorf("touching canvas map after device removal %s: %w", id, err)
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
	if _, err := r.db.Exec(
		`UPDATE canvas_maps SET updated_at = ? WHERE id = ?`,
		time.Now().UTC(),
		id.String(),
	); err != nil {
		return fmt.Errorf("touching canvas map after link removal %s: %w", id, err)
	}
	return nil
}
