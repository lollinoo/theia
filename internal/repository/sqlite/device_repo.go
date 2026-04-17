package sqlite

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/domain"
	"github.com/lollinoo/theia/internal/observability"
)

// DeviceRepo implements domain.DeviceRepository using SQLite.
type DeviceRepo struct {
	db            *DB
	encryptionKey []byte
	onChange      chan<- struct{}
	changeEvents  chan domain.DeviceChangeEvent
	repairPending atomic.Bool
}

// NewDeviceRepo creates a new SQLite-backed device repository.
// The onChange channel, if non-nil, receives a non-blocking signal after
// every successful Create, Update, or Delete operation.
func NewDeviceRepo(db *sql.DB, encryptionKey []byte, onChange chan<- struct{}) *DeviceRepo {
	return &DeviceRepo{
		db:            wrapDB(db),
		encryptionKey: encryptionKey,
		onChange:      onChange,
		changeEvents:  make(chan domain.DeviceChangeEvent, 256),
	}
}

func (r *DeviceRepo) DeviceChanges() <-chan domain.DeviceChangeEvent {
	return r.changeEvents
}

func (r *DeviceRepo) DrainDeviceRepair() bool {
	return r.repairPending.Swap(false)
}

// notify sends a non-blocking signal on the onChange channel to indicate
// that the underlying data has been modified.
func (r *DeviceRepo) notify() {
	if r.onChange == nil {
		return
	}
	select {
	case r.onChange <- struct{}{}:
		observability.Default().IncCacheInvalidation("device_repo")
	default:
	}
}

func (r *DeviceRepo) publishChange(kind domain.ChangeKind, deviceID uuid.UUID) {
	if r.changeEvents == nil {
		return
	}

	event := domain.DeviceChangeEvent{
		Kind:     kind,
		DeviceID: deviceID,
	}
	select {
	case r.changeEvents <- event:
	default:
		r.repairPending.Store(true)
	}
}

// Create inserts a new device and its interfaces into the database.
func (r *DeviceRepo) Create(device *domain.Device) error {
	return withSQLiteBusyRetry(func() error {
		return r.createOnce(device)
	})
}

func (r *DeviceRepo) createOnce(device *domain.Device) error {
	now := time.Now().UTC()
	device.CreatedAt = now
	device.UpdatedAt = now
	if device.ID == uuid.Nil {
		device.ID = uuid.New()
	}
	if device.Tags == nil {
		device.Tags = map[string]string{}
	}

	// Deep copy credentials for encryption (don't modify the original)
	credsCopy := deepCopySNMPCredentials(device.SNMPCredentials)
	if err := encryptSNMPCredentials(&credsCopy, r.encryptionKey); err != nil {
		return fmt.Errorf("encrypting snmp credentials: %w", err)
	}
	credsJSON, err := json.Marshal(credsCopy)
	if err != nil {
		return fmt.Errorf("marshaling snmp credentials: %w", err)
	}
	tagsJSON, err := json.Marshal(device.Tags)
	if err != nil {
		return fmt.Errorf("marshaling tags: %w", err)
	}
	managedValue := boolToDBInt(device.Managed)

	// Default empty PollClass to PollClassStandard before insert so the SQL
	// default never has to fire — keeps in-memory and DB state in sync.
	pollClass := device.PollClass
	if pollClass == "" {
		pollClass = domain.PollClassStandard
	}

	tx, err := r.db.Begin()
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}
	defer tx.Rollback()

	_, err = tx.Exec(
		`INSERT INTO devices (id, hostname, ip, snmp_credentials_json, device_type, status,
			sys_name, sys_name_lookup, sys_descr, sys_object_id, hardware_model, vendor, managed, tags_json,
			created_at, updated_at, metrics_source, prometheus_label_name, prometheus_label_value,
			poll_class, poll_interval_override, notes)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		device.ID.String(), device.Hostname, device.IP, string(credsJSON),
		string(device.DeviceType), string(device.Status),
		device.SysName, normalizeDeviceSysNameLookup(device.SysName), device.SysDescr,
		device.SysObjectID, device.HardwareModel,
		device.Vendor, managedValue, string(tagsJSON), device.CreatedAt, device.UpdatedAt,
		string(device.MetricsSource), device.PrometheusLabelName, device.PrometheusLabelValue,
		string(pollClass), device.PollIntervalOverride, nullableStringValue(device.Notes),
	)
	if err != nil {
		return fmt.Errorf("inserting device: %w", err)
	}

	// Write canonicalized PollClass back so callers see the normalized value.
	device.PollClass = pollClass

	// Insert area associations
	for _, areaID := range device.AreaIDs {
		_, err = tx.Exec(
			`INSERT INTO device_areas (device_id, area_id) VALUES (?, ?)`,
			device.ID.String(), areaID.String(),
		)
		if err != nil {
			return fmt.Errorf("inserting device area %s: %w", areaID, err)
		}
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

	if err := tx.Commit(); err != nil {
		return err
	}
	r.notify()
	r.publishChange(domain.ChangeKindCreated, device.ID)
	return nil
}

// GetByID retrieves a device by UUID, including its interfaces.
func (r *DeviceRepo) GetByID(id uuid.UUID) (*domain.Device, error) {
	device, err := r.scanDevice(
		r.db.QueryRow(
			`SELECT id, hostname, ip, snmp_credentials_json, device_type, status,
				sys_name, sys_descr, sys_object_id, hardware_model, vendor, managed, tags_json,
				created_at, updated_at, metrics_source, prometheus_label_name, prometheus_label_value,
				poll_class, poll_interval_override, notes
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

	areaIDs, err := r.loadAreaIDs(device.ID)
	if err != nil {
		return nil, err
	}
	device.AreaIDs = areaIDs

	return device, nil
}

// GetByIP retrieves a device by IP address, or returns nil if not found.
func (r *DeviceRepo) GetByIP(ip string) (*domain.Device, error) {
	device, err := r.scanDevice(
		r.db.QueryRow(
			`SELECT id, hostname, ip, snmp_credentials_json, device_type, status,
				sys_name, sys_descr, sys_object_id, hardware_model, vendor, managed, tags_json,
				created_at, updated_at, metrics_source, prometheus_label_name, prometheus_label_value,
				poll_class, poll_interval_override, notes
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

	areaIDs, err := r.loadAreaIDs(device.ID)
	if err != nil {
		return nil, err
	}
	device.AreaIDs = areaIDs

	return device, nil
}

// GetBySysName retrieves a device by SNMP sysName, or returns nil if not found.
// Matching is normalization-aware for lookup only: it trims whitespace,
// lowercases, strips a trailing dot, and removes any FQDN suffix.
func (r *DeviceRepo) GetBySysName(sysName string) (*domain.Device, error) {
	normalizedLookup := normalizeDeviceSysNameLookup(sysName)
	if normalizedLookup == "" {
		return nil, nil
	}

	device, err := r.scanDevice(
		r.db.QueryRow(
			`SELECT id, hostname, ip, snmp_credentials_json, device_type, status,
				sys_name, sys_descr, sys_object_id, hardware_model, vendor, managed, tags_json,
				created_at, updated_at, metrics_source, prometheus_label_name, prometheus_label_value,
				poll_class, poll_interval_override, notes
			FROM devices
			WHERE sys_name_lookup = ?
			ORDER BY updated_at DESC, created_at DESC
			LIMIT 1`,
			normalizedLookup,
		),
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("querying device by sys_name: %w", err)
	}

	ifaces, err := r.loadInterfaces(device.ID)
	if err != nil {
		return nil, err
	}
	device.Interfaces = ifaces

	areaIDs, err := r.loadAreaIDs(device.ID)
	if err != nil {
		return nil, err
	}
	device.AreaIDs = areaIDs

	return device, nil
}

func normalizeDeviceSysNameLookup(sysName string) string {
	normalized := strings.ToLower(strings.TrimSpace(sysName))
	normalized = strings.TrimSuffix(normalized, ".")
	if idx := strings.Index(normalized, "."); idx >= 0 {
		normalized = normalized[:idx]
	}
	return normalized
}

// GetAll retrieves all devices with their interfaces.
// Uses batched interface loading to avoid N+1 queries.
func (r *DeviceRepo) GetAll() ([]domain.Device, error) {
	rows, err := r.db.Query(
		`SELECT id, hostname, ip, snmp_credentials_json, device_type, status,
			sys_name, sys_descr, sys_object_id, hardware_model, vendor, managed, tags_json,
			created_at, updated_at, metrics_source, prometheus_label_name, prometheus_label_value,
			poll_class, poll_interval_override, notes
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
		devices = append(devices, *device)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	if len(devices) == 0 {
		return devices, nil
	}

	// Batch load all interfaces in one query
	allInterfaces, err := r.loadAllInterfaces()
	if err != nil {
		return nil, fmt.Errorf("loading all interfaces: %w", err)
	}

	// Group by device_id
	ifacesByDevice := make(map[uuid.UUID][]domain.Interface)
	for _, iface := range allInterfaces {
		ifacesByDevice[iface.DeviceID] = append(ifacesByDevice[iface.DeviceID], iface)
	}

	// Batch load all area IDs in one query
	allAreaIDs, err := r.loadAllAreaIDs()
	if err != nil {
		return nil, fmt.Errorf("loading all device areas: %w", err)
	}

	// Attach to devices
	for i := range devices {
		devices[i].Interfaces = ifacesByDevice[devices[i].ID]
		devices[i].AreaIDs = allAreaIDs[devices[i].ID]
	}

	return devices, nil
}

// Update modifies an existing device and replaces its interfaces.
func (r *DeviceRepo) Update(device *domain.Device) error {
	return withSQLiteBusyRetry(func() error {
		return r.updateOnce(device)
	})
}

func (r *DeviceRepo) updateOnce(device *domain.Device) error {
	device.UpdatedAt = time.Now().UTC()
	if device.Tags == nil {
		device.Tags = map[string]string{}
	}

	// Deep copy credentials for encryption (don't modify the original)
	credsCopy := deepCopySNMPCredentials(device.SNMPCredentials)
	if err := encryptSNMPCredentials(&credsCopy, r.encryptionKey); err != nil {
		return fmt.Errorf("encrypting snmp credentials: %w", err)
	}
	credsJSON, err := json.Marshal(credsCopy)
	if err != nil {
		return fmt.Errorf("marshaling snmp credentials: %w", err)
	}
	tagsJSON, err := json.Marshal(device.Tags)
	if err != nil {
		return fmt.Errorf("marshaling tags: %w", err)
	}
	managedValue := boolToDBInt(device.Managed)

	// Default empty PollClass to PollClassStandard before update.
	pollClass := device.PollClass
	if pollClass == "" {
		pollClass = domain.PollClassStandard
	}

	tx, err := r.db.Begin()
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}
	defer tx.Rollback()

	result, err := tx.Exec(
		`UPDATE devices SET hostname=?, ip=?, snmp_credentials_json=?, device_type=?,
			status=?, sys_name=?, sys_name_lookup=?, sys_descr=?, sys_object_id=?, hardware_model=?,
			vendor=?, managed=?, tags_json=?, updated_at=?,
			metrics_source=?, prometheus_label_name=?, prometheus_label_value=?,
			poll_class=?, poll_interval_override=?, notes=?
		WHERE id = ?`,
		device.Hostname, device.IP, string(credsJSON),
		string(device.DeviceType), string(device.Status),
		device.SysName, normalizeDeviceSysNameLookup(device.SysName), device.SysDescr,
		device.SysObjectID, device.HardwareModel,
		device.Vendor, managedValue, string(tagsJSON), device.UpdatedAt,
		string(device.MetricsSource), device.PrometheusLabelName, device.PrometheusLabelValue,
		string(pollClass), device.PollIntervalOverride, nullableStringValue(device.Notes),
		device.ID.String(),
	)
	if err != nil {
		return fmt.Errorf("updating device: %w", err)
	}

	// Write canonicalized PollClass back so callers see the normalized value.
	device.PollClass = pollClass
	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return fmt.Errorf("device not found: %s", device.ID)
	}

	// Replace area assignments
	if _, err := tx.Exec(`DELETE FROM device_areas WHERE device_id = ?`, device.ID.String()); err != nil {
		return fmt.Errorf("deleting existing device areas: %w", err)
	}
	for _, areaID := range device.AreaIDs {
		_, err = tx.Exec(
			`INSERT INTO device_areas (device_id, area_id) VALUES (?, ?)`,
			device.ID.String(), areaID.String(),
		)
		if err != nil {
			return fmt.Errorf("inserting device area %s: %w", areaID, err)
		}
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

	if err := tx.Commit(); err != nil {
		return err
	}
	r.notify()
	r.publishChange(domain.ChangeKindUpdated, device.ID)
	return nil
}

// Delete removes a device and its interfaces (via CASCADE) by UUID.
func (r *DeviceRepo) Delete(id uuid.UUID) error {
	return withSQLiteBusyRetry(func() error {
		return r.deleteOnce(id)
	})
}

func (r *DeviceRepo) deleteOnce(id uuid.UUID) error {
	result, err := r.db.Exec(`DELETE FROM devices WHERE id = ?`, id.String())
	if err != nil {
		return fmt.Errorf("deleting device: %w", err)
	}
	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return fmt.Errorf("device not found: %s", id)
	}
	r.notify()
	r.publishChange(domain.ChangeKindDeleted, id)
	return nil
}

func boolToDBInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func nullableStringValue(value *string) any {
	if value == nil {
		return nil
	}
	return *value
}

// scanDevice scans a single device row from a *sql.Row.
func (r *DeviceRepo) scanDevice(row *sql.Row) (*domain.Device, error) {
	var d domain.Device
	var idStr, credsJSON, tagsJSON, deviceType, status string
	var managed int
	var metricsSource, prometheusLabelName, prometheusLabelValue string
	var pollClass string
	var pollIntervalOverride sql.NullInt64
	var notes sql.NullString

	err := row.Scan(
		&idStr, &d.Hostname, &d.IP, &credsJSON, &deviceType, &status,
		&d.SysName, &d.SysDescr, &d.SysObjectID, &d.HardwareModel,
		&d.Vendor, &managed, &tagsJSON, &d.CreatedAt, &d.UpdatedAt,
		&metricsSource, &prometheusLabelName, &prometheusLabelValue,
		&pollClass, &pollIntervalOverride, &notes,
	)
	if err != nil {
		return nil, err
	}

	d.ID = uuid.MustParse(idStr)
	d.DeviceType = domain.DeviceType(deviceType)
	d.Status = domain.DeviceStatus(status)
	d.Managed = managed != 0
	d.MetricsSource = domain.MetricsSource(metricsSource)
	d.PrometheusLabelName = prometheusLabelName
	d.PrometheusLabelValue = prometheusLabelValue
	d.PollClass = domain.PollClass(pollClass)
	if pollIntervalOverride.Valid {
		v := int(pollIntervalOverride.Int64)
		d.PollIntervalOverride = &v
	}
	if notes.Valid {
		v := notes.String
		d.Notes = &v
	}

	if err := json.Unmarshal([]byte(credsJSON), &d.SNMPCredentials); err != nil {
		return nil, fmt.Errorf("unmarshaling snmp credentials: %w", err)
	}
	decryptSNMPCredentials(&d.SNMPCredentials, r.encryptionKey)
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
	var metricsSource, prometheusLabelName, prometheusLabelValue string
	var pollClass string
	var pollIntervalOverride sql.NullInt64
	var notes sql.NullString

	err := rows.Scan(
		&idStr, &d.Hostname, &d.IP, &credsJSON, &deviceType, &status,
		&d.SysName, &d.SysDescr, &d.SysObjectID, &d.HardwareModel,
		&d.Vendor, &managed, &tagsJSON, &d.CreatedAt, &d.UpdatedAt,
		&metricsSource, &prometheusLabelName, &prometheusLabelValue,
		&pollClass, &pollIntervalOverride, &notes,
	)
	if err != nil {
		return nil, err
	}

	d.ID = uuid.MustParse(idStr)
	d.DeviceType = domain.DeviceType(deviceType)
	d.Status = domain.DeviceStatus(status)
	d.Managed = managed != 0
	d.MetricsSource = domain.MetricsSource(metricsSource)
	d.PrometheusLabelName = prometheusLabelName
	d.PrometheusLabelValue = prometheusLabelValue
	d.PollClass = domain.PollClass(pollClass)
	if pollIntervalOverride.Valid {
		v := int(pollIntervalOverride.Int64)
		d.PollIntervalOverride = &v
	}
	if notes.Valid {
		v := notes.String
		d.Notes = &v
	}

	if err := json.Unmarshal([]byte(credsJSON), &d.SNMPCredentials); err != nil {
		return nil, fmt.Errorf("unmarshaling snmp credentials: %w", err)
	}
	decryptSNMPCredentials(&d.SNMPCredentials, r.encryptionKey)
	if err := json.Unmarshal([]byte(tagsJSON), &d.Tags); err != nil {
		return nil, fmt.Errorf("unmarshaling tags: %w", err)
	}

	return &d, nil
}

// loadAreaIDs retrieves area IDs for a single device.
func (r *DeviceRepo) loadAreaIDs(deviceID uuid.UUID) ([]uuid.UUID, error) {
	rows, err := r.db.Query(
		`SELECT area_id FROM device_areas WHERE device_id = ? ORDER BY area_id`,
		deviceID.String(),
	)
	if err != nil {
		return nil, fmt.Errorf("querying device areas: %w", err)
	}
	defer rows.Close()

	var ids []uuid.UUID
	for rows.Next() {
		var idStr string
		if err := rows.Scan(&idStr); err != nil {
			return nil, fmt.Errorf("scanning area_id: %w", err)
		}
		ids = append(ids, uuid.MustParse(idStr))
	}
	return ids, rows.Err()
}

// loadAllAreaIDs fetches all device↔area mappings in one query.
func (r *DeviceRepo) loadAllAreaIDs() (map[uuid.UUID][]uuid.UUID, error) {
	rows, err := r.db.Query(
		`SELECT device_id, area_id FROM device_areas ORDER BY device_id, area_id`,
	)
	if err != nil {
		return nil, fmt.Errorf("querying all device areas: %w", err)
	}
	defer rows.Close()

	result := make(map[uuid.UUID][]uuid.UUID)
	for rows.Next() {
		var deviceIDStr, areaIDStr string
		if err := rows.Scan(&deviceIDStr, &areaIDStr); err != nil {
			return nil, fmt.Errorf("scanning device area: %w", err)
		}
		deviceID := uuid.MustParse(deviceIDStr)
		areaID := uuid.MustParse(areaIDStr)
		result[deviceID] = append(result[deviceID], areaID)
	}
	return result, rows.Err()
}

// loadAllInterfaces fetches all interfaces in a single query (avoids N+1).
func (r *DeviceRepo) loadAllInterfaces() ([]domain.Interface, error) {
	rows, err := r.db.Query(
		`SELECT id, device_id, if_index, if_name, if_descr, speed,
			admin_status, oper_status, created_at, updated_at
		FROM interfaces ORDER BY device_id, if_index`,
	)
	if err != nil {
		return nil, fmt.Errorf("querying all interfaces: %w", err)
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
