package postgres

// This file defines device repo persistence behavior, ordering guarantees, and not-found conventions.

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/crypto"
	"github.com/lollinoo/theia/internal/domain"
	"github.com/lollinoo/theia/internal/observability"
)

// DeviceRepo implements domain.DeviceRepository using PostgreSQL.
type DeviceRepo struct {
	db            *DB
	keyring       *crypto.Keyring
	onChange      chan<- struct{}
	subscribersMu sync.RWMutex
	subscribers   map[chan domain.DeviceChangeEvent]struct{}
	repairPending atomic.Bool
}

// NewDeviceRepo creates a new PostgreSQL-backed device repository.
// The onChange channel, if non-nil, receives a non-blocking signal after
// every successful Create, Update, or Delete operation.
func NewDeviceRepo(db *sql.DB, keyring *crypto.Keyring, onChange chan<- struct{}) *DeviceRepo {
	return &DeviceRepo{
		db:          wrapDB(db),
		keyring:     keyring,
		onChange:    onChange,
		subscribers: make(map[chan domain.DeviceChangeEvent]struct{}),
	}
}

func (r *DeviceRepo) DeviceChanges() <-chan domain.DeviceChangeEvent {
	return r.SubscribeDeviceChanges(256)
}

func (r *DeviceRepo) SubscribeDeviceChanges(buffer int) <-chan domain.DeviceChangeEvent {
	if buffer <= 0 {
		buffer = 256
	}

	ch := make(chan domain.DeviceChangeEvent, buffer)
	r.subscribersMu.Lock()
	r.subscribers[ch] = struct{}{}
	r.subscribersMu.Unlock()
	return ch
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
	event := domain.DeviceChangeEvent{
		Kind:     kind,
		DeviceID: deviceID,
	}

	r.subscribersMu.RLock()
	defer r.subscribersMu.RUnlock()
	for subscriber := range r.subscribers {
		select {
		case subscriber <- event:
		default:
			r.repairPending.Store(true)
		}
	}
}

// Create inserts a new device and its interfaces into the database.
func (r *DeviceRepo) Create(device *domain.Device) error {
	return withWriteRetry(func() error {
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
	domain.NormalizeDevicePollingEnabled(device)
	domain.NormalizeDeviceAddresses(device)

	// Deep copy credentials for encryption (don't modify the original)
	credsCopy := deepCopySNMPCredentials(device.SNMPCredentials)
	if err := encryptSNMPCredentials(&credsCopy, r.keyring); err != nil {
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
	topologyMode := device.TopologyDiscoveryMode
	if topologyMode == "" {
		topologyMode = domain.TopologyDiscoveryModeInherit
	}
	bootstrapState := device.TopologyBootstrapState
	if bootstrapState == "" {
		bootstrapState = domain.TopologyBootstrapStateIdle
	}

	tx, err := r.db.Begin()
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}
	defer tx.Rollback()

	_, err = tx.Exec(
		`INSERT INTO devices (id, hostname, ip, snmp_credentials_json, device_type, status,
			sys_name, sys_name_lookup, sys_descr, sys_object_id, hardware_model, os_version, vendor, managed, tags_json,
			created_at, updated_at, metrics_source, prometheus_label_name, prometheus_label_value,
			poll_class, poll_interval_override, polling_enabled, notes,
			probe_ports, topology_discovery_mode, topology_bootstrap_state, last_topology_discovery_at, last_topology_discovery_result)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		device.ID.String(), device.Hostname, device.IP, string(credsJSON),
		string(device.DeviceType), string(device.Status),
		device.SysName, normalizeDeviceSysNameLookup(device.SysName), device.SysDescr,
		device.SysObjectID, device.HardwareModel, device.OSVersion,
		device.Vendor, managedValue, string(tagsJSON), device.CreatedAt, device.UpdatedAt,
		string(device.MetricsSource), device.PrometheusLabelName, device.PrometheusLabelValue,
		string(pollClass), device.PollIntervalOverride, boolToDBInt(domain.DevicePollingEnabled(*device)), nullableStringValue(device.Notes),
		domain.FormatProbePortsCSV(device.ProbePorts), string(topologyMode), string(bootstrapState), nullableTimeValue(device.LastTopologyDiscoveryAt), device.LastTopologyDiscoveryResult,
	)
	if err != nil {
		return fmt.Errorf("inserting device: %w", err)
	}

	// Write canonicalized PollClass back so callers see the normalized value.
	device.PollClass = pollClass
	device.TopologyDiscoveryMode = topologyMode
	device.TopologyBootstrapState = bootstrapState

	if err := replaceDeviceAddressesTx(tx, device.ID, device.Addresses, now); err != nil {
		return fmt.Errorf("inserting device addresses: %w", err)
	}

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
				sys_name, sys_descr, sys_object_id, hardware_model, os_version, vendor, managed, tags_json,
				created_at, updated_at, metrics_source, prometheus_label_name, prometheus_label_value,
				poll_class, poll_interval_override, polling_enabled, notes,
				probe_ports, topology_discovery_mode, topology_bootstrap_state, last_topology_discovery_at, last_topology_discovery_result
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
	addresses, err := r.loadAddresses(device.ID)
	if err != nil {
		return nil, err
	}
	device.Addresses = addresses

	return device, nil
}

// GetByIP retrieves a device by IP address, or returns nil if not found.
func (r *DeviceRepo) GetByIP(ip string) (*domain.Device, error) {
	device, err := r.scanDevice(
		r.db.QueryRow(
			`SELECT id, hostname, ip, snmp_credentials_json, device_type, status,
				sys_name, sys_descr, sys_object_id, hardware_model, os_version, vendor, managed, tags_json,
				created_at, updated_at, metrics_source, prometheus_label_name, prometheus_label_value,
				poll_class, poll_interval_override, polling_enabled, notes,
				probe_ports, topology_discovery_mode, topology_bootstrap_state, last_topology_discovery_at, last_topology_discovery_result
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
	addresses, err := r.loadAddresses(device.ID)
	if err != nil {
		return nil, err
	}
	device.Addresses = addresses

	return device, nil
}

// GetByAddress retrieves a device by any normalized device address.
func (r *DeviceRepo) GetByAddress(address string) (*domain.Device, error) {
	normalized := domain.NormalizeDeviceAddressValue(address)
	if normalized == "" {
		return nil, nil
	}

	var idStr string
	err := r.db.QueryRow(
		`SELECT d.id
		FROM devices d
		JOIN device_addresses da ON da.device_id = d.id
		WHERE da.normalized_address = ?
		ORDER BY da.is_primary DESC, da.priority ASC, d.updated_at DESC, d.created_at DESC
		LIMIT 1`,
		normalized,
	).Scan(&idStr)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("querying device by address: %w", err)
	}

	id, err := uuid.Parse(idStr)
	if err != nil {
		return nil, fmt.Errorf("parsing device id %q: %w", idStr, err)
	}
	return r.GetByID(id)
}

// FindPhysicalVirtualIPConflict returns one device with the same normalized
// address and opposite physical/virtual classification, without loading full
// device relationships or credentials.
func (r *DeviceRepo) FindPhysicalVirtualIPConflict(ip string, deviceType domain.DeviceType, excludeID uuid.UUID) (*domain.Device, error) {
	address := strings.TrimSpace(ip)
	if address == "" {
		return nil, nil
	}
	candidateVirtual := 0
	if deviceType == domain.DeviceTypeVirtual {
		candidateVirtual = 1
	}

	var idStr, storedIP, storedType string
	err := r.db.QueryRow(
		`SELECT id, ip, device_type
		FROM devices
		WHERE btrim(ip) <> ''
			AND lower(btrim(ip)) = lower(btrim(?))
			AND (CASE WHEN device_type = 'virtual' THEN 1 ELSE 0 END) <> ?
			AND id <> ?
		ORDER BY updated_at DESC, created_at DESC
		LIMIT 1`,
		address,
		candidateVirtual,
		excludeID.String(),
	).Scan(&idStr, &storedIP, &storedType)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("querying physical/virtual device IP conflict: %w", err)
	}

	id, err := uuid.Parse(idStr)
	if err != nil {
		return nil, fmt.Errorf("parsing device id %q: %w", idStr, err)
	}
	return &domain.Device{
		ID:         id,
		IP:         storedIP,
		DeviceType: domain.DeviceType(storedType),
	}, nil
}

// FindAddressConflict returns a non-virtual device that already owns the normalized address.
func (r *DeviceRepo) FindAddressConflict(address string, deviceType domain.DeviceType, excludeID uuid.UUID) (*domain.Device, error) {
	normalized := domain.NormalizeDeviceAddressValue(address)
	if normalized == "" || deviceType == domain.DeviceTypeVirtual {
		return nil, nil
	}

	var idStr, storedAddress, storedType string
	err := r.db.QueryRow(
		`SELECT d.id, da.address, d.device_type
		FROM device_addresses da
		JOIN devices d ON d.id = da.device_id
		WHERE da.normalized_address = ?
			AND d.device_type <> 'virtual'
			AND d.id <> ?
		ORDER BY da.is_primary DESC, da.priority ASC, d.updated_at DESC, d.created_at DESC
		LIMIT 1`,
		normalized,
		excludeID.String(),
	).Scan(&idStr, &storedAddress, &storedType)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("querying device address conflict: %w", err)
	}

	id, err := uuid.Parse(idStr)
	if err != nil {
		return nil, fmt.Errorf("parsing device id %q: %w", idStr, err)
	}
	return &domain.Device{
		ID:         id,
		IP:         storedAddress,
		DeviceType: domain.DeviceType(storedType),
	}, nil
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
				sys_name, sys_descr, sys_object_id, hardware_model, os_version, vendor, managed, tags_json,
				created_at, updated_at, metrics_source, prometheus_label_name, prometheus_label_value,
				poll_class, poll_interval_override, polling_enabled, notes,
				probe_ports, topology_discovery_mode, topology_bootstrap_state, last_topology_discovery_at, last_topology_discovery_result
			FROM devices
			WHERE sys_name_lookup = ? AND sys_name_lookup != ''
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
	addresses, err := r.loadAddresses(device.ID)
	if err != nil {
		return nil, err
	}
	device.Addresses = addresses

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
			sys_name, sys_descr, sys_object_id, hardware_model, os_version, vendor, managed, tags_json,
			created_at, updated_at, metrics_source, prometheus_label_name, prometheus_label_value,
			poll_class, poll_interval_override, polling_enabled, notes,
			probe_ports, topology_discovery_mode, topology_bootstrap_state, last_topology_discovery_at, last_topology_discovery_result
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
	loadedIDs := make([]uuid.UUID, 0, len(devices))
	for _, device := range devices {
		loadedIDs = append(loadedIDs, device.ID)
	}
	addressesByDevice, err := r.loadAddressesForDeviceIDs(loadedIDs)
	if err != nil {
		return nil, fmt.Errorf("loading all device addresses: %w", err)
	}

	// Attach to devices
	for i := range devices {
		devices[i].Interfaces = ifacesByDevice[devices[i].ID]
		devices[i].AreaIDs = allAreaIDs[devices[i].ID]
		devices[i].Addresses = addressesByDevice[devices[i].ID]
	}

	return devices, nil
}

// GetOrphans retrieves devices that do not belong to any saved canvas map.
func (r *DeviceRepo) GetOrphans() ([]domain.Device, error) {
	rows, err := r.db.Query(
		`SELECT id, hostname, ip, snmp_credentials_json, device_type, status,
			sys_name, sys_descr, sys_object_id, hardware_model, os_version, vendor, managed, tags_json,
			created_at, updated_at, metrics_source, prometheus_label_name, prometheus_label_value,
			poll_class, poll_interval_override, polling_enabled, notes,
			probe_ports, topology_discovery_mode, topology_bootstrap_state, last_topology_discovery_at, last_topology_discovery_result
		FROM devices d
		WHERE NOT EXISTS (
			SELECT 1
			FROM canvas_map_devices cmd
			WHERE cmd.device_id = d.id
		)
		ORDER BY hostname`,
	)
	if err != nil {
		return nil, fmt.Errorf("querying orphan devices: %w", err)
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

	loadedIDs := make([]uuid.UUID, 0, len(devices))
	for _, device := range devices {
		loadedIDs = append(loadedIDs, device.ID)
	}

	interfacesByDevice, err := r.loadInterfacesForDeviceIDs(loadedIDs)
	if err != nil {
		return nil, fmt.Errorf("loading interfaces for orphan devices: %w", err)
	}
	areaIDsByDevice, err := r.loadAreaIDsForDeviceIDs(loadedIDs)
	if err != nil {
		return nil, fmt.Errorf("loading area IDs for orphan devices: %w", err)
	}
	addressesByDevice, err := r.loadAddressesForDeviceIDs(loadedIDs)
	if err != nil {
		return nil, fmt.Errorf("loading addresses for orphan devices: %w", err)
	}

	for i := range devices {
		devices[i].Interfaces = interfacesByDevice[devices[i].ID]
		devices[i].AreaIDs = areaIDsByDevice[devices[i].ID]
		devices[i].Addresses = addressesByDevice[devices[i].ID]
	}

	return devices, nil
}

// GetByIDs retrieves the requested devices with their interfaces and area IDs.
func (r *DeviceRepo) GetByIDs(ids []uuid.UUID) ([]domain.Device, error) {
	if len(ids) == 0 {
		return []domain.Device{}, nil
	}

	placeholders := make([]string, 0, len(ids))
	args := make([]interface{}, 0, len(ids))
	for _, id := range ids {
		placeholders = append(placeholders, "?")
		args = append(args, id.String())
	}

	rows, err := r.db.Query(
		`SELECT id, hostname, ip, snmp_credentials_json, device_type, status,
			sys_name, sys_descr, sys_object_id, hardware_model, os_version, vendor, managed, tags_json,
			created_at, updated_at, metrics_source, prometheus_label_name, prometheus_label_value,
			poll_class, poll_interval_override, polling_enabled, notes,
			probe_ports, topology_discovery_mode, topology_bootstrap_state, last_topology_discovery_at, last_topology_discovery_result
		FROM devices
		WHERE id IN (`+strings.Join(placeholders, ", ")+`)
		ORDER BY hostname`,
		args...,
	)
	if err != nil {
		return nil, fmt.Errorf("querying devices by ids: %w", err)
	}
	defer rows.Close()

	devices := make([]domain.Device, 0, len(ids))
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

	loadedIDs := make([]uuid.UUID, 0, len(devices))
	for _, device := range devices {
		loadedIDs = append(loadedIDs, device.ID)
	}

	interfacesByDevice, err := r.loadInterfacesForDeviceIDs(loadedIDs)
	if err != nil {
		return nil, fmt.Errorf("loading interfaces for devices: %w", err)
	}
	areaIDsByDevice, err := r.loadAreaIDsForDeviceIDs(loadedIDs)
	if err != nil {
		return nil, fmt.Errorf("loading area IDs for devices: %w", err)
	}
	addressesByDevice, err := r.loadAddressesForDeviceIDs(loadedIDs)
	if err != nil {
		return nil, fmt.Errorf("loading addresses for devices: %w", err)
	}

	for i := range devices {
		devices[i].Interfaces = interfacesByDevice[devices[i].ID]
		devices[i].AreaIDs = areaIDsByDevice[devices[i].ID]
		devices[i].Addresses = addressesByDevice[devices[i].ID]
	}

	return devices, nil
}

// GetByIDsForTopology retrieves the requested devices without loading sensitive credentials.
func (r *DeviceRepo) GetByIDsForTopology(ids []uuid.UUID) ([]domain.Device, error) {
	if len(ids) == 0 {
		return []domain.Device{}, nil
	}

	placeholders := make([]string, 0, len(ids))
	args := make([]interface{}, 0, len(ids))
	for _, id := range ids {
		placeholders = append(placeholders, "?")
		args = append(args, id.String())
	}

	rows, err := r.db.Query(
		`SELECT id, hostname, ip, device_type, status,
			sys_name, sys_descr, sys_object_id, hardware_model, os_version, vendor, managed, tags_json,
			created_at, updated_at, metrics_source, prometheus_label_name, prometheus_label_value,
			poll_class, poll_interval_override, polling_enabled, notes,
			topology_discovery_mode, topology_bootstrap_state, last_topology_discovery_at, last_topology_discovery_result
		FROM devices
		WHERE id IN (`+strings.Join(placeholders, ", ")+`)
		ORDER BY hostname`,
		args...,
	)
	if err != nil {
		return nil, fmt.Errorf("querying topology devices by ids: %w", err)
	}
	defer rows.Close()

	devices := make([]domain.Device, 0, len(ids))
	for rows.Next() {
		device, err := r.scanDeviceTopologyRow(rows)
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

	loadedIDs := make([]uuid.UUID, 0, len(devices))
	for _, device := range devices {
		loadedIDs = append(loadedIDs, device.ID)
	}

	interfacesByDevice, err := r.loadInterfacesForDeviceIDs(loadedIDs)
	if err != nil {
		return nil, fmt.Errorf("loading interfaces for topology devices: %w", err)
	}
	areaIDsByDevice, err := r.loadAreaIDsForDeviceIDs(loadedIDs)
	if err != nil {
		return nil, fmt.Errorf("loading area IDs for topology devices: %w", err)
	}
	addressesByDevice, err := r.loadAddressesForDeviceIDs(loadedIDs)
	if err != nil {
		return nil, fmt.Errorf("loading addresses for topology devices: %w", err)
	}

	for i := range devices {
		devices[i].Interfaces = interfacesByDevice[devices[i].ID]
		devices[i].AreaIDs = areaIDsByDevice[devices[i].ID]
		devices[i].Addresses = addressesByDevice[devices[i].ID]
	}

	return devices, nil
}

// Update modifies an existing device and replaces its interfaces.
func (r *DeviceRepo) Update(device *domain.Device) error {
	return withWriteRetry(func() error {
		return r.updateOnce(device)
	})
}

func (r *DeviceRepo) updateOnce(device *domain.Device) error {
	device.UpdatedAt = time.Now().UTC()
	if device.Tags == nil {
		device.Tags = map[string]string{}
	}
	domain.NormalizeDevicePollingEnabled(device)
	domain.NormalizeDeviceAddresses(device)

	// Deep copy credentials for encryption (don't modify the original)
	credsCopy := deepCopySNMPCredentials(device.SNMPCredentials)
	if err := encryptSNMPCredentials(&credsCopy, r.keyring); err != nil {
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
	topologyMode := device.TopologyDiscoveryMode
	if topologyMode == "" {
		topologyMode = domain.TopologyDiscoveryModeInherit
	}
	bootstrapState := device.TopologyBootstrapState
	if bootstrapState == "" {
		bootstrapState = domain.TopologyBootstrapStateIdle
	}

	tx, err := r.db.Begin()
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}
	defer tx.Rollback()

	result, err := tx.Exec(
		`UPDATE devices SET hostname=?, ip=?, snmp_credentials_json=?, device_type=?,
			status=?, sys_name=?, sys_name_lookup=?, sys_descr=?, sys_object_id=?, hardware_model=?, os_version=?,
			vendor=?, managed=?, tags_json=?, updated_at=?,
			metrics_source=?, prometheus_label_name=?, prometheus_label_value=?,
			poll_class=?, poll_interval_override=?, polling_enabled=?, notes=?,
			probe_ports=?, topology_discovery_mode=?, topology_bootstrap_state=?, last_topology_discovery_at=?, last_topology_discovery_result=?
		WHERE id = ?`,
		device.Hostname, device.IP, string(credsJSON),
		string(device.DeviceType), string(device.Status),
		device.SysName, normalizeDeviceSysNameLookup(device.SysName), device.SysDescr,
		device.SysObjectID, device.HardwareModel, device.OSVersion,
		device.Vendor, managedValue, string(tagsJSON), device.UpdatedAt,
		string(device.MetricsSource), device.PrometheusLabelName, device.PrometheusLabelValue,
		string(pollClass), device.PollIntervalOverride, boolToDBInt(domain.DevicePollingEnabled(*device)), nullableStringValue(device.Notes),
		domain.FormatProbePortsCSV(device.ProbePorts), string(topologyMode), string(bootstrapState), nullableTimeValue(device.LastTopologyDiscoveryAt), device.LastTopologyDiscoveryResult,
		device.ID.String(),
	)
	if err != nil {
		return fmt.Errorf("updating device: %w", err)
	}

	// Write canonicalized PollClass back so callers see the normalized value.
	device.PollClass = pollClass
	device.TopologyDiscoveryMode = topologyMode
	device.TopologyBootstrapState = bootstrapState
	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return fmt.Errorf("device not found: %s", device.ID)
	}

	if err := replaceDeviceAddressesTx(tx, device.ID, device.Addresses, device.UpdatedAt); err != nil {
		return fmt.Errorf("replacing device addresses: %w", err)
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
	return withWriteRetry(func() error {
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

func nullableTimeValue(value *time.Time) any {
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
	var pollingEnabled int
	var notes sql.NullString
	var probePortsCSV string
	var topologyMode, bootstrapState, lastTopologyResult string
	var lastTopologyAt sql.NullTime

	err := row.Scan(
		&idStr, &d.Hostname, &d.IP, &credsJSON, &deviceType, &status,
		&d.SysName, &d.SysDescr, &d.SysObjectID, &d.HardwareModel, &d.OSVersion,
		&d.Vendor, &managed, &tagsJSON, &d.CreatedAt, &d.UpdatedAt,
		&metricsSource, &prometheusLabelName, &prometheusLabelValue,
		&pollClass, &pollIntervalOverride, &pollingEnabled, &notes,
		&probePortsCSV, &topologyMode, &bootstrapState, &lastTopologyAt, &lastTopologyResult,
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
	enabled := pollingEnabled != 0
	d.PollingEnabled = &enabled
	d.TopologyDiscoveryMode = domain.TopologyDiscoveryMode(topologyMode)
	d.TopologyBootstrapState = domain.TopologyBootstrapState(bootstrapState)
	d.LastTopologyDiscoveryResult = lastTopologyResult
	if pollIntervalOverride.Valid {
		v := int(pollIntervalOverride.Int64)
		d.PollIntervalOverride = &v
	}
	if lastTopologyAt.Valid {
		v := lastTopologyAt.Time
		d.LastTopologyDiscoveryAt = &v
	}
	if notes.Valid {
		v := notes.String
		d.Notes = &v
	}
	probePorts, err := domain.ParseProbePortsCSV(probePortsCSV)
	if err != nil {
		return nil, fmt.Errorf("parsing device probe_ports for device %s: %w", idStr, err)
	}
	d.ProbePorts = probePorts

	if err := json.Unmarshal([]byte(credsJSON), &d.SNMPCredentials); err != nil {
		return nil, fmt.Errorf("unmarshaling snmp credentials: %w", err)
	}
	if err := decryptSNMPCredentials(&d.SNMPCredentials, r.keyring); err != nil {
		return nil, fmt.Errorf("decrypting snmp credentials for device %s: %w", d.ID, err)
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
	var metricsSource, prometheusLabelName, prometheusLabelValue string
	var pollClass string
	var pollIntervalOverride sql.NullInt64
	var pollingEnabled int
	var notes sql.NullString
	var probePortsCSV string
	var topologyMode, bootstrapState, lastTopologyResult string
	var lastTopologyAt sql.NullTime

	err := rows.Scan(
		&idStr, &d.Hostname, &d.IP, &credsJSON, &deviceType, &status,
		&d.SysName, &d.SysDescr, &d.SysObjectID, &d.HardwareModel, &d.OSVersion,
		&d.Vendor, &managed, &tagsJSON, &d.CreatedAt, &d.UpdatedAt,
		&metricsSource, &prometheusLabelName, &prometheusLabelValue,
		&pollClass, &pollIntervalOverride, &pollingEnabled, &notes,
		&probePortsCSV, &topologyMode, &bootstrapState, &lastTopologyAt, &lastTopologyResult,
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
	enabled := pollingEnabled != 0
	d.PollingEnabled = &enabled
	d.TopologyDiscoveryMode = domain.TopologyDiscoveryMode(topologyMode)
	d.TopologyBootstrapState = domain.TopologyBootstrapState(bootstrapState)
	d.LastTopologyDiscoveryResult = lastTopologyResult
	if pollIntervalOverride.Valid {
		v := int(pollIntervalOverride.Int64)
		d.PollIntervalOverride = &v
	}
	if lastTopologyAt.Valid {
		v := lastTopologyAt.Time
		d.LastTopologyDiscoveryAt = &v
	}
	if notes.Valid {
		v := notes.String
		d.Notes = &v
	}
	probePorts, err := domain.ParseProbePortsCSV(probePortsCSV)
	if err != nil {
		return nil, fmt.Errorf("parsing device probe_ports for device %s: %w", idStr, err)
	}
	d.ProbePorts = probePorts

	if err := json.Unmarshal([]byte(credsJSON), &d.SNMPCredentials); err != nil {
		return nil, fmt.Errorf("unmarshaling snmp credentials: %w", err)
	}
	if err := decryptSNMPCredentials(&d.SNMPCredentials, r.keyring); err != nil {
		return nil, fmt.Errorf("decrypting snmp credentials for device %s: %w", d.ID, err)
	}
	if err := json.Unmarshal([]byte(tagsJSON), &d.Tags); err != nil {
		return nil, fmt.Errorf("unmarshaling tags: %w", err)
	}

	return &d, nil
}

// scanDeviceTopologyRow scans a device row that intentionally excludes sensitive credentials.
func (r *DeviceRepo) scanDeviceTopologyRow(rows *sql.Rows) (*domain.Device, error) {
	var d domain.Device
	var idStr, tagsJSON, deviceType, status string
	var managed int
	var metricsSource, prometheusLabelName, prometheusLabelValue string
	var pollClass string
	var pollIntervalOverride sql.NullInt64
	var pollingEnabled int
	var notes sql.NullString
	var topologyMode, bootstrapState, lastTopologyResult string
	var lastTopologyAt sql.NullTime

	err := rows.Scan(
		&idStr, &d.Hostname, &d.IP, &deviceType, &status,
		&d.SysName, &d.SysDescr, &d.SysObjectID, &d.HardwareModel, &d.OSVersion,
		&d.Vendor, &managed, &tagsJSON, &d.CreatedAt, &d.UpdatedAt,
		&metricsSource, &prometheusLabelName, &prometheusLabelValue,
		&pollClass, &pollIntervalOverride, &pollingEnabled, &notes,
		&topologyMode, &bootstrapState, &lastTopologyAt, &lastTopologyResult,
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
	enabled := pollingEnabled != 0
	d.PollingEnabled = &enabled
	d.TopologyDiscoveryMode = domain.TopologyDiscoveryMode(topologyMode)
	d.TopologyBootstrapState = domain.TopologyBootstrapState(bootstrapState)
	d.LastTopologyDiscoveryResult = lastTopologyResult
	if pollIntervalOverride.Valid {
		v := int(pollIntervalOverride.Int64)
		d.PollIntervalOverride = &v
	}
	if lastTopologyAt.Valid {
		v := lastTopologyAt.Time
		d.LastTopologyDiscoveryAt = &v
	}
	if notes.Valid {
		v := notes.String
		d.Notes = &v
	}

	if err := json.Unmarshal([]byte(tagsJSON), &d.Tags); err != nil {
		return nil, fmt.Errorf("unmarshaling tags: %w", err)
	}
	d.SNMPCredentials = domain.SNMPCredentials{}

	return &d, nil
}

// GetDeviceAddresses retrieves normalized addresses for one device.
func (r *DeviceRepo) GetDeviceAddresses(deviceID uuid.UUID) ([]domain.DeviceAddress, error) {
	return r.loadAddresses(deviceID)
}

// ReplaceDeviceAddresses replaces all address rows for one device.
func (r *DeviceRepo) ReplaceDeviceAddresses(deviceID uuid.UUID, addresses []domain.DeviceAddress) error {
	return withWriteRetry(func() error {
		tx, err := r.db.Begin()
		if err != nil {
			return fmt.Errorf("beginning transaction: %w", err)
		}
		defer tx.Rollback()

		if err := replaceDeviceAddressesTx(tx, deviceID, addresses, time.Now().UTC()); err != nil {
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
		r.notify()
		r.publishChange(domain.ChangeKindUpdated, deviceID)
		return nil
	})
}

func replaceDeviceAddressesTx(tx *Tx, deviceID uuid.UUID, addresses []domain.DeviceAddress, now time.Time) error {
	if _, err := tx.Exec(`DELETE FROM device_addresses WHERE device_id = ?`, deviceID.String()); err != nil {
		return fmt.Errorf("deleting existing device addresses: %w", err)
	}

	for i := range addresses {
		address := addresses[i]
		address.Address = strings.TrimSpace(address.Address)
		if address.Address == "" {
			continue
		}
		address.Label = strings.TrimSpace(address.Label)
		address.Role = domain.NormalizeDeviceAddressRole(address.Role)
		if address.ID == uuid.Nil {
			address.ID = uuid.New()
		}
		address.DeviceID = deviceID
		if address.CreatedAt.IsZero() {
			address.CreatedAt = now
		}
		if address.UpdatedAt.IsZero() {
			address.UpdatedAt = now
		}
		normalized := domain.NormalizeDeviceAddressValue(address.Address)
		_, err := tx.Exec(
			`INSERT INTO device_addresses (
				id, device_id, address, normalized_address, label, role, is_primary, priority, probe_ports, created_at, updated_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			address.ID.String(),
			address.DeviceID.String(),
			address.Address,
			normalized,
			address.Label,
			string(address.Role),
			address.IsPrimary,
			address.Priority,
			domain.FormatProbePortsCSV(address.ProbePorts),
			address.CreatedAt,
			address.UpdatedAt,
		)
		if err != nil {
			return fmt.Errorf("inserting device address %q: %w", address.Address, err)
		}
		addresses[i] = address
	}
	return nil
}

func (r *DeviceRepo) loadAddresses(deviceID uuid.UUID) ([]domain.DeviceAddress, error) {
	rows, err := r.db.Query(
		`SELECT id, device_id, address, label, role, is_primary, priority, probe_ports, created_at, updated_at
		FROM device_addresses
		WHERE device_id = ?
		ORDER BY is_primary DESC, priority ASC, normalized_address ASC`,
		deviceID.String(),
	)
	if err != nil {
		return nil, fmt.Errorf("querying device addresses: %w", err)
	}
	defer rows.Close()

	var addresses []domain.DeviceAddress
	for rows.Next() {
		address, err := scanDeviceAddressRow(rows)
		if err != nil {
			return nil, err
		}
		addresses = append(addresses, address)
	}
	return addresses, rows.Err()
}

func (r *DeviceRepo) loadAddressesForDeviceIDs(deviceIDs []uuid.UUID) (map[uuid.UUID][]domain.DeviceAddress, error) {
	if len(deviceIDs) == 0 {
		return map[uuid.UUID][]domain.DeviceAddress{}, nil
	}

	placeholders := make([]string, 0, len(deviceIDs))
	args := make([]interface{}, 0, len(deviceIDs))
	for _, id := range deviceIDs {
		placeholders = append(placeholders, "?")
		args = append(args, id.String())
	}

	rows, err := r.db.Query(
		`SELECT id, device_id, address, label, role, is_primary, priority, probe_ports, created_at, updated_at
		FROM device_addresses
		WHERE device_id IN (`+strings.Join(placeholders, ", ")+`)
		ORDER BY device_id, is_primary DESC, priority ASC, normalized_address ASC`,
		args...,
	)
	if err != nil {
		return nil, fmt.Errorf("querying selected device addresses: %w", err)
	}
	defer rows.Close()

	result := make(map[uuid.UUID][]domain.DeviceAddress)
	for rows.Next() {
		address, err := scanDeviceAddressRow(rows)
		if err != nil {
			return nil, err
		}
		result[address.DeviceID] = append(result[address.DeviceID], address)
	}
	return result, rows.Err()
}

func scanDeviceAddressRow(rows *sql.Rows) (domain.DeviceAddress, error) {
	var address domain.DeviceAddress
	var idStr, deviceIDStr, role string
	var probePortsCSV string
	err := rows.Scan(
		&idStr,
		&deviceIDStr,
		&address.Address,
		&address.Label,
		&role,
		&address.IsPrimary,
		&address.Priority,
		&probePortsCSV,
		&address.CreatedAt,
		&address.UpdatedAt,
	)
	if err != nil {
		return domain.DeviceAddress{}, fmt.Errorf("scanning device address: %w", err)
	}
	id, err := uuid.Parse(idStr)
	if err != nil {
		return domain.DeviceAddress{}, fmt.Errorf("parsing device address id %q: %w", idStr, err)
	}
	deviceID, err := uuid.Parse(deviceIDStr)
	if err != nil {
		return domain.DeviceAddress{}, fmt.Errorf("parsing device address device_id %q: %w", deviceIDStr, err)
	}
	address.ID = id
	address.DeviceID = deviceID
	address.Role = domain.NormalizeDeviceAddressRole(domain.DeviceAddressRole(role))
	probePorts, err := domain.ParseProbePortsCSV(probePortsCSV)
	if err != nil {
		return domain.DeviceAddress{}, fmt.Errorf("parsing device address probe_ports for address %s: %w", idStr, err)
	}
	address.ProbePorts = probePorts
	return address, nil
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

func (r *DeviceRepo) loadAreaIDsForDeviceIDs(deviceIDs []uuid.UUID) (map[uuid.UUID][]uuid.UUID, error) {
	if len(deviceIDs) == 0 {
		return map[uuid.UUID][]uuid.UUID{}, nil
	}

	placeholders := make([]string, 0, len(deviceIDs))
	args := make([]interface{}, 0, len(deviceIDs))
	for _, id := range deviceIDs {
		placeholders = append(placeholders, "?")
		args = append(args, id.String())
	}

	rows, err := r.db.Query(
		`SELECT device_id, area_id
		 FROM device_areas
		 WHERE device_id IN (`+strings.Join(placeholders, ", ")+`)
		 ORDER BY device_id, area_id`,
		args...,
	)
	if err != nil {
		return nil, fmt.Errorf("querying selected device areas: %w", err)
	}
	defer rows.Close()

	result := make(map[uuid.UUID][]uuid.UUID)
	for rows.Next() {
		var deviceIDStr, areaIDStr string
		if err := rows.Scan(&deviceIDStr, &areaIDStr); err != nil {
			return nil, fmt.Errorf("scanning selected device area: %w", err)
		}
		deviceID := uuid.MustParse(deviceIDStr)
		areaID := uuid.MustParse(areaIDStr)
		result[deviceID] = append(result[deviceID], areaID)
	}
	return result, rows.Err()
}

func (r *DeviceRepo) loadInterfacesForDeviceIDs(deviceIDs []uuid.UUID) (map[uuid.UUID][]domain.Interface, error) {
	if len(deviceIDs) == 0 {
		return map[uuid.UUID][]domain.Interface{}, nil
	}

	placeholders := make([]string, 0, len(deviceIDs))
	args := make([]interface{}, 0, len(deviceIDs))
	for _, id := range deviceIDs {
		placeholders = append(placeholders, "?")
		args = append(args, id.String())
	}

	rows, err := r.db.Query(
		`SELECT id, device_id, if_index, if_name, if_descr, speed,
			admin_status, oper_status, created_at, updated_at
		FROM interfaces
		WHERE device_id IN (`+strings.Join(placeholders, ", ")+`)
		ORDER BY device_id, if_index`,
		args...,
	)
	if err != nil {
		return nil, fmt.Errorf("querying selected interfaces: %w", err)
	}
	defer rows.Close()

	result := make(map[uuid.UUID][]domain.Interface)
	for rows.Next() {
		var iface domain.Interface
		var idStr, deviceIDStr string
		if err := rows.Scan(
			&idStr, &deviceIDStr, &iface.IfIndex, &iface.IfName,
			&iface.IfDescr, &iface.Speed, &iface.AdminStatus,
			&iface.OperStatus, &iface.CreatedAt, &iface.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scanning selected interface: %w", err)
		}

		iface.ID = uuid.MustParse(idStr)
		iface.DeviceID = uuid.MustParse(deviceIDStr)
		result[iface.DeviceID] = append(result[iface.DeviceID], iface)
	}

	return result, rows.Err()
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
