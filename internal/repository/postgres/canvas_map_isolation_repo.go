package postgres

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/google/uuid"
)

// lockCanvasMap serializes map-local membership writes before child rows change.
func lockCanvasMap(tx *Tx, id uuid.UUID) error {
	var found string
	if err := tx.QueryRow(`SELECT id FROM canvas_maps WHERE id=? FOR UPDATE`, id.String()).Scan(&found); err != nil {
		if err == sql.ErrNoRows {
			return fmt.Errorf("canvas map not found: %s", id)
		}
		return err
	}
	return nil
}

// IsolateVirtualDevices clones shared virtual members and remaps their map-local
// relationships in one transaction. Repeated and concurrent calls are idempotent.
func (r *CanvasMapRepo) IsolateVirtualDevices(ctx context.Context, mapID uuid.UUID) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := lockCanvasMap(tx, mapID); err != nil {
		return err
	}
	rows, err := tx.QueryContext(ctx, `SELECT d.id FROM devices d JOIN canvas_map_devices m ON m.device_id=d.id
 WHERE m.map_id=? AND d.device_type='virtual' AND EXISTS (
 SELECT 1 FROM canvas_map_devices other WHERE other.device_id=d.id AND other.map_id<>m.map_id)
 ORDER BY d.id`, mapID.String())
	if err != nil {
		return err
	}
	clones := make(map[string]string)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		clones[id] = uuid.NewString()
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	if len(clones) == 0 {
		return tx.Commit()
	}
	for oldID, newID := range clones {
		if _, err := tx.ExecContext(ctx, `INSERT INTO devices (
 id,hostname,ip,snmp_credentials_json,device_type,status,sys_name,sys_descr,sys_object_id,hardware_model,vendor,
 managed,tags_json,created_at,updated_at,metrics_source,prometheus_label_name,prometheus_label_value,sys_name_lookup,
 poll_class,poll_interval_override,notes,topology_discovery_mode,topology_bootstrap_state,
 last_topology_discovery_at,last_topology_discovery_result,os_version,polling_enabled,probe_ports)
 SELECT ?,hostname,ip,snmp_credentials_json,device_type,status,sys_name,sys_descr,sys_object_id,hardware_model,vendor,
 managed,tags_json,NOW(),NOW(),'none',prometheus_label_name,prometheus_label_value,sys_name_lookup,
 poll_class,poll_interval_override,notes,topology_discovery_mode,topology_bootstrap_state,
 last_topology_discovery_at,last_topology_discovery_result,os_version,polling_enabled,probe_ports FROM devices WHERE id=?`, newID, oldID); err != nil {
			return err
		}
		for _, query := range []string{
			`INSERT INTO device_addresses(id,device_id,address,normalized_address,label,role,is_primary,priority,probe_ports,created_at,updated_at)
 SELECT gen_random_uuid()::text,?,address,normalized_address,label,role,is_primary,priority,probe_ports,NOW(),NOW() FROM device_addresses WHERE device_id=?`,
			`INSERT INTO canvas_map_devices(map_id,device_id,role,visual_color,added_at)
 SELECT map_id,?,role,visual_color,added_at FROM canvas_map_devices WHERE device_id=? AND map_id='` + mapID.String() + `'`,
			`INSERT INTO canvas_map_device_areas(map_id,device_id,area_id,assigned_at)
 SELECT map_id,?,area_id,assigned_at FROM canvas_map_device_areas WHERE device_id=? AND map_id='` + mapID.String() + `'`,
		} {
			if _, err := tx.ExecContext(ctx, query, newID, oldID); err != nil {
				return err
			}
		}
		if _, err := tx.ExecContext(ctx, `UPDATE canvas_map_positions SET device_id=? WHERE map_id=? AND device_id=?`, newID, mapID.String(), oldID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM canvas_map_devices WHERE map_id=? AND device_id=?`, mapID.String(), oldID); err != nil {
			return err
		}
	}
	rows, err = tx.QueryContext(ctx, `SELECT l.id,l.source_device_id,l.target_device_id FROM links l
 JOIN canvas_map_links m ON m.link_id=l.id WHERE m.map_id=?`, mapID.String())
	if err != nil {
		return err
	}
	type remap struct{ id, source, target string }
	var links []remap
	for rows.Next() {
		var link remap
		if err := rows.Scan(&link.id, &link.source, &link.target); err != nil {
			rows.Close()
			return err
		}
		links = append(links, link)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, link := range links {
		changed := false
		if id, ok := clones[link.source]; ok {
			link.source = id
			changed = true
		}
		if id, ok := clones[link.target]; ok {
			link.target = id
			changed = true
		}
		if !changed {
			continue
		}
		newID := uuid.NewString()
		if _, err := tx.ExecContext(ctx, `INSERT INTO links(id,source_device_id,source_if_name,target_device_id,target_if_name,discovery_protocol,created_at,updated_at)
 SELECT ?,?,source_if_name,?,target_if_name,discovery_protocol,NOW(),NOW() FROM links WHERE id=?`, newID, link.source, link.target, link.id); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE canvas_map_links SET link_id=? WHERE map_id=? AND link_id=?`, newID, mapID.String(), link.id); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE canvas_map_link_routes SET link_id=? WHERE map_id=? AND link_id=?`, newID, mapID.String(), link.id); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, `UPDATE canvas_maps SET updated_at=NOW() WHERE id=?`, mapID.String()); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	if r.onChange != nil {
		select {
		case r.onChange <- struct{}{}:
		default:
		}
	}
	return nil
}
