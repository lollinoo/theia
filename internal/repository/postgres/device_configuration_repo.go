package postgres

import (
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/domain"
	"strings"
	"time"
)

// GetDeviceForUpdate omits secret decryption and interface loading. Configuration
// validation and scheduler reconciliation need only the row, addresses and areas.
func (r *DeviceRepo) GetDeviceForUpdate(id uuid.UUID) (*domain.Device, error) {
	device, err := r.scanDevice(r.db.QueryRow(`SELECT id,hostname,ip,'{}',device_type,status,
  sys_name,sys_descr,sys_object_id,hardware_model,os_version,vendor,managed,tags_json,
  created_at,updated_at,metrics_source,prometheus_label_name,prometheus_label_value,
  poll_class,poll_interval_override,polling_enabled,notes,probe_ports,
  topology_discovery_mode,topology_bootstrap_state,last_topology_discovery_at,last_topology_discovery_result
  FROM devices WHERE id=?`, id.String()))
	if err != nil {
		return nil, err
	}
	device.Addresses, err = r.loadAddresses(id)
	if err != nil {
		return nil, err
	}
	device.AreaIDs, err = r.loadAreaIDs(id)
	if err != nil {
		return nil, err
	}
	return device, nil
}

// UpdateConfiguration writes selected fields and replaces only relationships
// requested by the caller. Discovery fields, interfaces and unchanged secrets survive.
func (r *DeviceRepo) UpdateConfiguration(device *domain.Device, fields domain.DeviceConfigurationFields) error {
	now := time.Now().UTC()
	columns := []string{"updated_at=?"}
	args := []interface{}{now}
	add := func(enabled bool, column string, value interface{}) {
		if enabled {
			columns = append(columns, column+"=?")
			args = append(args, value)
		}
	}
	add(fields.Hostname, "hostname", device.Hostname)
	add(fields.IP, "ip", device.IP)
	add(fields.ProbePorts, "probe_ports", domain.FormatProbePortsCSV(device.ProbePorts))
	add(fields.Notes, "notes", nullableStringValue(device.Notes))
	add(fields.Vendor, "vendor", device.Vendor)
	add(fields.MetricsSource, "metrics_source", string(device.MetricsSource))
	add(fields.PrometheusLabelName, "prometheus_label_name", device.PrometheusLabelName)
	add(fields.PrometheusLabelValue, "prometheus_label_value", device.PrometheusLabelValue)
	add(fields.TopologyDiscoveryMode, "topology_discovery_mode", string(device.TopologyDiscoveryMode))
	add(fields.TopologyBootstrapState, "topology_bootstrap_state", string(device.TopologyBootstrapState))
	add(fields.PollingEnabled, "polling_enabled", boolToDBInt(domain.DevicePollingEnabled(*device)))
	add(fields.PollIntervalOverride, "poll_interval_override", device.PollIntervalOverride)
	add(fields.Status, "status", string(device.Status))
	if fields.Tags {
		tags := device.Tags
		if tags == nil {
			tags = map[string]string{}
		}
		data, err := json.Marshal(tags)
		if err != nil {
			return err
		}
		add(true, "tags_json", string(data))
	}
	if fields.SNMPCredentials {
		credentials := deepCopySNMPCredentials(device.SNMPCredentials)
		if err := encryptSNMPCredentials(&credentials, r.keyring); err != nil {
			return err
		}
		data, err := json.Marshal(credentials)
		if err != nil {
			return err
		}
		add(true, "snmp_credentials_json", string(data))
	}
	args = append(args, device.ID.String())
	exec := r.db.Exec
	var tx *Tx
	if fields.Addresses || fields.AreaIDs {
		var err error
		tx, err = r.db.Begin()
		if err != nil {
			return err
		}
		defer tx.Rollback()
		exec = tx.Exec
	}
	result, err := exec("UPDATE devices SET "+strings.Join(columns, ",")+" WHERE id=?", args...)
	if err != nil {
		return fmt.Errorf("updating device configuration: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("device not found: %s", device.ID)
	}
	if fields.Addresses {
		if err := replaceDeviceAddressesTx(tx, device.ID, device.Addresses, now); err != nil {
			return err
		}
	}
	if fields.AreaIDs {
		if _, err := tx.Exec("DELETE FROM device_areas WHERE device_id=?", device.ID.String()); err != nil {
			return err
		}
		for _, area := range device.AreaIDs {
			if _, err := tx.Exec("INSERT INTO device_areas(device_id,area_id) VALUES(?,?)", device.ID.String(), area.String()); err != nil {
				return err
			}
		}
	}
	if tx != nil {
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	device.UpdatedAt = now
	r.notify()
	r.publishChange(domain.ChangeKindUpdated, device.ID)
	return nil
}
