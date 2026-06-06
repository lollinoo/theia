package postgres

// This file defines canvas map helpers persistence behavior, ordering guarantees, and not-found conventions.

import (
	"database/sql"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/domain"
)

type rowScanner interface {
	Scan(dest ...any) error
}

// canvasMapSelectQuery builds the shared saved-map select with aggregate counts.
func canvasMapSelectQuery(whereClause string) string {
	return `SELECT
			cm.id,
			cm.name,
			cm.description,
			cm.source_area_id,
			cm.filter_json,
			cm.is_default,
			cm.membership_materialized,
			cm.created_at,
			cm.updated_at,
			COALESCE(device_counts.device_count, 0) AS device_count,
			COALESCE(link_counts.link_count, 0) AS link_count,
			COALESCE(position_counts.position_count, 0) AS position_count
		FROM canvas_maps cm
		LEFT JOIN (
			SELECT map_id, COUNT(*) AS device_count
			FROM canvas_map_devices
			GROUP BY map_id
		) device_counts ON device_counts.map_id = cm.id
		LEFT JOIN (
			SELECT map_id, COUNT(*) AS link_count
			FROM canvas_map_links
			GROUP BY map_id
		) link_counts ON link_counts.map_id = cm.id
		LEFT JOIN (
			SELECT map_id, COUNT(*) AS position_count
			FROM canvas_map_positions
			GROUP BY map_id
		) position_counts ON position_counts.map_id = cm.id
		` + whereClause + `
		GROUP BY
			cm.id,
			cm.name,
			cm.description,
			cm.source_area_id,
			cm.filter_json,
			cm.is_default,
			cm.membership_materialized,
			cm.created_at,
			cm.updated_at,
			device_counts.device_count,
			link_counts.link_count,
			position_counts.position_count`
}

// scanCanvasMap converts one saved-map row into the domain DTO.
func scanCanvasMap(scanner rowScanner) (domain.CanvasMap, error) {
	var canvasMap domain.CanvasMap
	var (
		idRaw           string
		sourceAreaIDRaw sql.NullString
		isDefaultRaw    any
		materializedRaw any
	)

	if err := scanner.Scan(
		&idRaw,
		&canvasMap.Name,
		&canvasMap.Description,
		&sourceAreaIDRaw,
		&canvasMap.FilterJSON,
		&isDefaultRaw,
		&materializedRaw,
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

	membershipMaterialized, err := normalizeBoolValue(materializedRaw)
	if err != nil {
		return domain.CanvasMap{}, fmt.Errorf("normalizing canvas map membership_materialized: %w", err)
	}
	canvasMap.MembershipMaterialized = membershipMaterialized

	return canvasMap, nil
}

// scanCanvasMapAreaWithCount converts one map-local area row into an area count DTO.
func scanCanvasMapAreaWithCount(scanner rowScanner) (domain.AreaWithCount, error) {
	var area domain.AreaWithCount
	var areaIDRaw string
	if err := scanner.Scan(
		&areaIDRaw,
		&area.Name,
		&area.Description,
		&area.Color,
		&area.CreatedAt,
		&area.DeviceCount,
	); err != nil {
		if err == sql.ErrNoRows {
			return domain.AreaWithCount{}, fmt.Errorf("canvas map area not found")
		}
		return domain.AreaWithCount{}, fmt.Errorf("scanning canvas map area: %w", err)
	}
	areaID, err := uuid.Parse(areaIDRaw)
	if err != nil {
		return domain.AreaWithCount{}, fmt.Errorf("parsing canvas map area id %q: %w", areaIDRaw, err)
	}
	area.ID = areaID
	area.UpdatedAt = area.CreatedAt
	return area, nil
}

type canvasMapQueryRower interface {
	QueryRow(query string, args ...interface{}) *sql.Row
}

// ensureCanvasMapExists fails with the stable not-found text used by API mapping.
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

// countCanvasMapAreas counts the map-local area catalog for one saved map.
func countCanvasMapAreas(queryer canvasMapQueryRower, id uuid.UUID) (int, error) {
	var count int
	if err := queryer.QueryRow(`SELECT COUNT(*) FROM canvas_map_areas WHERE map_id = ?`, id.String()).Scan(&count); err != nil {
		return 0, fmt.Errorf("counting canvas map areas for %s: %w", id, err)
	}
	return count, nil
}

// countCanvasMapDeviceAreas counts map-local device-to-area assignments.
func countCanvasMapDeviceAreas(queryer canvasMapQueryRower, id uuid.UUID) (int, error) {
	var count int
	if err := queryer.QueryRow(`SELECT COUNT(*) FROM canvas_map_device_areas WHERE map_id = ?`, id.String()).Scan(&count); err != nil {
		return 0, fmt.Errorf("counting canvas map device areas for %s: %w", id, err)
	}
	return count, nil
}

// getCanvasMapArea loads one map-local area with its base-device count.
func getCanvasMapArea(queryer canvasMapQueryRower, mapID uuid.UUID, areaID uuid.UUID) (domain.AreaWithCount, error) {
	return scanCanvasMapAreaWithCount(queryer.QueryRow(
		`SELECT cma.area_id, cma.name, cma.description, cma.color, cma.added_at,
		        COUNT(DISTINCT CASE WHEN cmd.role = ? THEN cmda.device_id END) AS device_count
		 FROM canvas_map_areas cma
		 LEFT JOIN canvas_map_device_areas cmda
		   ON cmda.map_id = cma.map_id AND cmda.area_id = cma.area_id
		 LEFT JOIN canvas_map_devices cmd
		   ON cmd.map_id = cmda.map_id AND cmd.device_id = cmda.device_id
		 WHERE cma.map_id = ? AND cma.area_id = ?
		 GROUP BY cma.area_id, cma.name, cma.description, cma.color, cma.added_at`,
		string(domain.CanvasMapDeviceRoleBase),
		mapID.String(),
		areaID.String(),
	))
}

// ensureCanvasMapAreaNameAvailable enforces per-map area-name uniqueness.
func ensureCanvasMapAreaNameAvailable(queryer canvasMapQueryRower, mapID uuid.UUID, areaID uuid.UUID, name string) error {
	var count int
	if err := queryer.QueryRow(
		`SELECT COUNT(*)
		 FROM canvas_map_areas
		 WHERE map_id = ? AND area_id <> ? AND name = ?`,
		mapID.String(),
		areaID.String(),
		name,
	).Scan(&count); err != nil {
		return fmt.Errorf("checking canvas map area name: %w", err)
	}
	if count > 0 {
		return fmt.Errorf("canvas map area name already exists: %s", name)
	}
	return nil
}

// backfillCanvasMapAreasFromMemberDevices snapshots global areas for materialized base members.
func backfillCanvasMapAreasFromMemberDevices(tx *Tx, mapID uuid.UUID) error {
	rows, err := tx.Query(
		`SELECT DISTINCT a.id, a.name, a.description, a.color
		 FROM canvas_map_devices cmd
		 JOIN device_areas da ON da.device_id = cmd.device_id
		 JOIN areas a ON a.id = da.area_id
		 WHERE cmd.map_id = ? AND cmd.role = ?
		 ORDER BY a.id`,
		mapID.String(),
		string(domain.CanvasMapDeviceRoleBase),
	)
	if err != nil {
		return fmt.Errorf("querying inferred canvas map areas for %s: %w", mapID, err)
	}
	defer rows.Close()

	areas := []domain.CanvasMapAreaMembership{}
	for rows.Next() {
		var area domain.CanvasMapAreaMembership
		var areaIDRaw string
		if err := rows.Scan(&areaIDRaw, &area.Name, &area.Description, &area.Color); err != nil {
			return fmt.Errorf("scanning inferred canvas map area for %s: %w", mapID, err)
		}
		areaID, err := uuid.Parse(areaIDRaw)
		if err != nil {
			return fmt.Errorf("parsing inferred canvas map area id %q: %w", areaIDRaw, err)
		}
		area.AreaID = areaID
		areas = append(areas, area)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterating inferred canvas map areas for %s: %w", mapID, err)
	}

	now := time.Now().UTC()
	for _, area := range areas {
		if _, err := tx.Exec(
			`INSERT INTO canvas_map_areas (map_id, area_id, name, description, color, added_at)
			 VALUES (?, ?, ?, ?, ?, ?)
			 ON CONFLICT(map_id, area_id) DO NOTHING`,
			mapID.String(),
			area.AreaID.String(),
			area.Name,
			area.Description,
			area.Color,
			now,
		); err != nil {
			return fmt.Errorf("backfilling inferred canvas map area %s for %s: %w", area.AreaID, mapID, err)
		}
	}
	return nil
}

// backfillCanvasMapDeviceAreasFromMemberDevices snapshots base-member global area assignments.
func backfillCanvasMapDeviceAreasFromMemberDevices(tx *Tx, mapID uuid.UUID) error {
	if _, err := tx.Exec(
		`INSERT INTO canvas_map_device_areas (map_id, device_id, area_id, assigned_at)
		 SELECT cmd.map_id, cmd.device_id, da.area_id, ?
		 FROM canvas_map_devices cmd
		 JOIN device_areas da ON da.device_id = cmd.device_id
		 JOIN canvas_map_areas cma ON cma.map_id = cmd.map_id AND cma.area_id = da.area_id
		 WHERE cmd.map_id = ? AND cmd.role = ?
		 ON CONFLICT(map_id, device_id, area_id) DO NOTHING`,
		time.Now().UTC(),
		mapID.String(),
		string(domain.CanvasMapDeviceRoleBase),
	); err != nil {
		return fmt.Errorf("backfilling canvas map device area memberships for %s: %w", mapID, err)
	}
	return nil
}

// insertCanvasMapDeviceAreas inserts normalized map-local device area memberships.
func insertCanvasMapDeviceAreas(
	tx *Tx,
	mapID uuid.UUID,
	devices []domain.CanvasMapDeviceMembership,
	assignedAt time.Time,
) error {
	for _, device := range devices {
		areaIDs, err := validateCanvasMapUUIDList(device.AreaIDs, "device area_id")
		if err != nil {
			return err
		}
		for _, areaID := range areaIDs {
			if _, err := tx.Exec(
				`INSERT INTO canvas_map_device_areas (map_id, device_id, area_id, assigned_at)
				 VALUES (?, ?, ?, ?)
				 ON CONFLICT(map_id, device_id, area_id) DO NOTHING`,
				mapID.String(),
				device.DeviceID.String(),
				areaID.String(),
				assignedAt,
			); err != nil {
				return fmt.Errorf("inserting canvas map device area membership %s/%s: %w", device.DeviceID, areaID, err)
			}
		}
	}
	return nil
}

// validateCanvasMapMembership rejects invalid or duplicate materialized membership rows.
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
		if _, err := validateCanvasMapUUIDList(device.AreaIDs, "device area_id"); err != nil {
			return err
		}
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
	for _, device := range membership.Devices {
		for _, areaID := range device.AreaIDs {
			if _, exists := areaIDs[areaID]; !exists {
				return fmt.Errorf("canvas map device area %s is not present in area membership", areaID)
			}
		}
	}

	return nil
}

// validateCanvasMapUUIDList sorts and de-duplicates required UUID inputs.
func validateCanvasMapUUIDList(ids []uuid.UUID, label string) ([]uuid.UUID, error) {
	if len(ids) == 0 {
		return []uuid.UUID{}, nil
	}
	canonical := append([]uuid.UUID(nil), ids...)
	sort.Slice(canonical, func(i, j int) bool {
		return canonical[i].String() < canonical[j].String()
	})
	for i, id := range canonical {
		if id == uuid.Nil {
			return nil, fmt.Errorf("canvas map %s is required", label)
		}
		if i > 0 && canonical[i-1] == id {
			return nil, fmt.Errorf("duplicate canvas map %s: %s", label, id)
		}
	}
	return canonical, nil
}

// rejectCanvasMapNonMemberPositions rejects position saves for devices outside materialized membership.
func rejectCanvasMapNonMemberPositions(tx *Tx, mapID uuid.UUID, positions []domain.DevicePosition) error {
	membershipMaterialized, err := canvasMapMembershipMaterialized(tx, mapID)
	if err != nil {
		return err
	}
	if !membershipMaterialized {
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

// canvasMapMembershipMaterialized returns whether saved-map membership constraints are active.
func canvasMapMembershipMaterialized(queryer canvasMapQueryRower, mapID uuid.UUID) (bool, error) {
	var materialized any
	if err := queryer.QueryRow(
		`SELECT membership_materialized FROM canvas_maps WHERE id = ?`,
		mapID.String(),
	).Scan(&materialized); err != nil {
		return false, fmt.Errorf("checking canvas map materialization: %w", err)
	}
	return normalizeBoolValue(materialized)
}

// pruneCanvasMapPositionsForMembership removes positions for devices no longer in membership.
func pruneCanvasMapPositionsForMembership(
	tx *Tx,
	mapID uuid.UUID,
	devices []domain.CanvasMapDeviceMembership,
) error {
	if len(devices) == 0 {
		if _, err := tx.Exec(`DELETE FROM canvas_map_positions WHERE map_id = ?`, mapID.String()); err != nil {
			return fmt.Errorf("pruning all canvas map positions for %s: %w", mapID, err)
		}
		return nil
	}

	args := make([]interface{}, 0, len(devices)+1)
	args = append(args, mapID.String())
	placeholders := make([]string, 0, len(devices))
	for _, device := range devices {
		placeholders = append(placeholders, "?")
		args = append(args, device.DeviceID.String())
	}
	if _, err := tx.Exec(
		`DELETE FROM canvas_map_positions
		 WHERE map_id = ?
		   AND device_id NOT IN (`+strings.Join(placeholders, ", ")+`)`,
		args...,
	); err != nil {
		return fmt.Errorf("pruning non-member canvas map positions for %s: %w", mapID, err)
	}
	return nil
}

// nullableUUIDString converts optional UUIDs to SQL values without empty-string sentinels.
func nullableUUIDString(id *uuid.UUID) any {
	if id == nil {
		return nil
	}
	return id.String()
}
