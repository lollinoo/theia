package sqlite

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/azmin/mikrotik-theia/internal/domain"
	"github.com/google/uuid"
)

// DeviceRepo implements domain.DeviceRepository using SQLite.
type DeviceRepo struct {
	db *sql.DB
}

// NewDeviceRepo creates a new SQLite-backed device repository.
func NewDeviceRepo(db *sql.DB) *DeviceRepo {
	return &DeviceRepo{db: db}
}

// Create inserts a new device and its interfaces into the database.
func (r *DeviceRepo) Create(device *domain.Device) error {
	now := time.Now().UTC()
	device.CreatedAt = now
	device.UpdatedAt = now
	if device.ID == uuid.Nil {
		device.ID = uuid.New()
	}
	if device.Tags == nil {
		device.Tags = map[string]string{}
	}

	credsJSON, err := json.Marshal(device.SNMPCredentials)
	if err != nil {
		return fmt.Errorf("marshaling snmp credentials: %w", err)
	}
	tagsJSON, err := json.Marshal(device.Tags)
	if err != nil {
		return fmt.Errorf("marshaling tags: %w", err)
	}

	tx, err := r.db.Begin()
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}
	defer tx.Rollback()

	_, err = tx.Exec(
		`INSERT INTO devices (id, hostname, ip, snmp_credentials_json, device_type, status,
			sys_name, sys_descr, sys_object_id, hardware_model, managed, tags_json,
			created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		device.ID.String(), device.Hostname, device.IP, string(credsJSON),
		string(device.DeviceType), string(device.Status),
		device.SysName, device.SysDescr, device.SysObjectID, device.HardwareModel,
		device.Managed, string(tagsJSON), device.CreatedAt, device.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("inserting device: %w", err)
	}

	// Insert interfaces
	for i := range device.Interfaces {
		iface := &device.Interfaces[i]
		iface.DeviceID = device.ID
		iface.CreatedAt = now
		iface.UpdatedAt = now
		if iface.ID == uuid.Nil {
			iface.ID = uuid.New()
		}

		_, err = tx.Exec(
			`INSERT INTO interfaces (id, device_id, if_index, if_name, if_descr, speed,
				admin_status, oper_status, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			iface.ID.String(), iface.DeviceID.String(), iface.IfIndex,
			iface.IfName, iface.IfDescr, iface.Speed,
			iface.AdminStatus, iface.OperStatus, iface.CreatedAt, iface.UpdatedAt,
		)
		if err != nil {
			return fmt.Errorf("inserting interface %s: %w", iface.IfName, err)
		}
	}

	return tx.Commit()
}

// GetByID retrieves a device by UUID, including its interfaces.
func (r *DeviceRepo) GetByID(id uuid.UUID) (*domain.Device, error) {
	device, err := r.scanDevice(
		r.db.QueryRow(
			`SELECT id, hostname, ip, snmp_credentials_json, device_type, status,
				sys_name, sys_descr, sys_object_id, hardware_model, managed, tags_json,
				created_at, updated_at
			FROM devices WHERE id = ?`, id.String(),
		),
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("device not found: %s", id)
		}
		return nil, err
	}

	ifaces, err := r.loadInterfaces(device.ID)
	if err != nil {
		return nil, err
	}
	device.Interfaces = ifaces

	return device, nil
}

// GetByIP retrieves a device by IP address, or returns nil if not found.
func (r *DeviceRepo) GetByIP(ip string) (*domain.Device, error) {
	device, err := r.scanDevice(
		r.db.QueryRow(
			`SELECT id, hostname, ip, snmp_credentials_json, device_type, status,
				sys_name, sys_descr, sys_object_id, hardware_model, managed, tags_json,
				created_at, updated_at
			FROM devices WHERE ip = ?`, ip,
		),
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	ifaces, err := r.loadInterfaces(device.ID)
	if err != nil {
		return nil, err
	}
	device.Interfaces = ifaces

	return device, nil
}

// GetAll retrieves all devices with their interfaces.
func (r *DeviceRepo) GetAll() ([]domain.Device, error) {
	rows, err := r.db.Query(
		`SELECT id, hostname, ip, snmp_credentials_json, device_type, status,
			sys_name, sys_descr, sys_object_id, hardware_model, managed, tags_json,
			created_at, updated_at
		FROM devices ORDER BY hostname`,
	)
	if err != nil {
		return nil, fmt.Errorf("querying devices: %w", err)
	}
	defer rows.Close()

	var devices []domain.Device
	for rows.Next() {
		device, err := r.scanDeviceRow(rows)
		if err != nil {
			return nil, err
		}
		ifaces, err := r.loadInterfaces(device.ID)
		if err != nil {
			return nil, err
		}
		device.Interfaces = ifaces
		devices = append(devices, *device)
	}

	return devices, rows.Err()
}

// Update modifies an existing device and replaces its interfaces.
func (r *DeviceRepo) Update(device *domain.Device) error {
	device.UpdatedAt = time.Now().UTC()
	if device.Tags == nil {
		device.Tags = map[string]string{}
	}

	credsJSON, err := json.Marshal(device.SNMPCredentials)
	if err != nil {
		return fmt.Errorf("marshaling snmp credentials: %w", err)
	}
	tagsJSON, err := json.Marshal(device.Tags)
	if err != nil {
		return fmt.Errorf("marshaling tags: %w", err)
	}

	tx, err := r.db.Begin()
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}
	defer tx.Rollback()

	result, err := tx.Exec(
		`UPDATE devices SET hostname=?, ip=?, snmp_credentials_json=?, device_type=?,
			status=?, sys_name=?, sys_descr=?, sys_object_id=?, hardware_model=?,
			managed=?, tags_json=?, updated_at=?
		WHERE id = ?`,
		device.Hostname, device.IP, string(credsJSON),
		string(device.DeviceType), string(device.Status),
		device.SysName, device.SysDescr, device.SysObjectID, device.HardwareModel,
		device.Managed, string(tagsJSON), device.UpdatedAt, device.ID.String(),
	)
	if err != nil {
		return fmt.Errorf("updating device: %w", err)
	}
	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return fmt.Errorf("device not found: %s", device.ID)
	}

	// Replace interfaces: delete existing, insert new
	if _, err := tx.Exec(`DELETE FROM interfaces WHERE device_id = ?`, device.ID.String()); err != nil {
		return fmt.Errorf("deleting existing interfaces: %w", err)
	}

	now := time.Now().UTC()
	for i := range device.Interfaces {
		iface := &device.Interfaces[i]
		iface.DeviceID = device.ID
		iface.UpdatedAt = now
		if iface.ID == uuid.Nil {
			iface.ID = uuid.New()
		}
		if iface.CreatedAt.IsZero() {
			iface.CreatedAt = now
		}

		_, err = tx.Exec(
			`INSERT INTO interfaces (id, device_id, if_index, if_name, if_descr, speed,
				admin_status, oper_status, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			iface.ID.String(), iface.DeviceID.String(), iface.IfIndex,
			iface.IfName, iface.IfDescr, iface.Speed,
			iface.AdminStatus, iface.OperStatus, iface.CreatedAt, iface.UpdatedAt,
		)
		if err != nil {
			return fmt.Errorf("inserting interface %s: %w", iface.IfName, err)
		}
	}

	return tx.Commit()
}

// Delete removes a device and its interfaces (via CASCADE) by UUID.
func (r *DeviceRepo) Delete(id uuid.UUID) error {
	result, err := r.db.Exec(`DELETE FROM devices WHERE id = ?`, id.String())
	if err != nil {
		return fmt.Errorf("deleting device: %w", err)
	}
	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return fmt.Errorf("device not found: %s", id)
	}
	return nil
}

// scanDevice scans a single device row from a *sql.Row.
func (r *DeviceRepo) scanDevice(row *sql.Row) (*domain.Device, error) {
	var d domain.Device
	var idStr, credsJSON, tagsJSON, deviceType, status string
	var managed int

	err := row.Scan(
		&idStr, &d.Hostname, &d.IP, &credsJSON, &deviceType, &status,
		&d.SysName, &d.SysDescr, &d.SysObjectID, &d.HardwareModel,
		&managed, &tagsJSON, &d.CreatedAt, &d.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	d.ID = uuid.MustParse(idStr)
	d.DeviceType = domain.DeviceType(deviceType)
	d.Status = domain.DeviceStatus(status)
	d.Managed = managed != 0

	if err := json.Unmarshal([]byte(credsJSON), &d.SNMPCredentials); err != nil {
		return nil, fmt.Errorf("unmarshaling snmp credentials: %w", err)
	}
	if err := json.Unmarshal([]byte(tagsJSON), &d.Tags); err != nil {
		return nil, fmt.Errorf("unmarshaling tags: %w", err)
	}

	return &d, nil
}

// scanDeviceRow scans a single device row from *sql.Rows.
func (r *DeviceRepo) scanDeviceRow(rows *sql.Rows) (*domain.Device, error) {
	var d domain.Device
	var idStr, credsJSON, tagsJSON, deviceType, status string
	var managed int

	err := rows.Scan(
		&idStr, &d.Hostname, &d.IP, &credsJSON, &deviceType, &status,
		&d.SysName, &d.SysDescr, &d.SysObjectID, &d.HardwareModel,
		&managed, &tagsJSON, &d.CreatedAt, &d.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	d.ID = uuid.MustParse(idStr)
	d.DeviceType = domain.DeviceType(deviceType)
	d.Status = domain.DeviceStatus(status)
	d.Managed = managed != 0

	if err := json.Unmarshal([]byte(credsJSON), &d.SNMPCredentials); err != nil {
		return nil, fmt.Errorf("unmarshaling snmp credentials: %w", err)
	}
	if err := json.Unmarshal([]byte(tagsJSON), &d.Tags); err != nil {
		return nil, fmt.Errorf("unmarshaling tags: %w", err)
	}

	return &d, nil
}

// loadInterfaces retrieves all interfaces for a given device ID.
func (r *DeviceRepo) loadInterfaces(deviceID uuid.UUID) ([]domain.Interface, error) {
	rows, err := r.db.Query(
		`SELECT id, device_id, if_index, if_name, if_descr, speed,
			admin_status, oper_status, created_at, updated_at
		FROM interfaces WHERE device_id = ? ORDER BY if_index`,
		deviceID.String(),
	)
	if err != nil {
		return nil, fmt.Errorf("querying interfaces: %w", err)
	}
	defer rows.Close()

	var ifaces []domain.Interface
	for rows.Next() {
		var iface domain.Interface
		var idStr, deviceIDStr string

		err := rows.Scan(
			&idStr, &deviceIDStr, &iface.IfIndex, &iface.IfName,
			&iface.IfDescr, &iface.Speed, &iface.AdminStatus,
			&iface.OperStatus, &iface.CreatedAt, &iface.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("scanning interface: %w", err)
		}

		iface.ID = uuid.MustParse(idStr)
		iface.DeviceID = uuid.MustParse(deviceIDStr)
		ifaces = append(ifaces, iface)
	}

	return ifaces, rows.Err()
}
