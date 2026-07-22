package postgres

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"hash/fnv"
	"io"
	"net"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/lollinoo/theia/internal/domain"
)

// DeviceImportStore persists each imported device and its saved-map membership in one transaction.
type DeviceImportStore struct {
	devices *DeviceRepo
}

var _ domain.DeviceImportStore = (*DeviceImportStore)(nil)

// NewDeviceImportStore creates an import store backed by the concrete device repository.
func NewDeviceImportStore(devices *DeviceRepo) *DeviceImportStore {
	return &DeviceImportStore{devices: devices}
}

// ExistingCanonicalAddresses returns the requested normalized addresses that already have owners.
func (s *DeviceImportStore) ExistingCanonicalAddresses(
	ctx context.Context,
	addresses []string,
) (map[string]struct{}, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if ctx.Err() != nil || s == nil || s.devices == nil || s.devices.db == nil {
		return nil, domain.ErrDeviceImportStoreUnavailable
	}
	existing, err := s.existingCanonicalAddresses(ctx, addresses)
	if err != nil {
		return nil, classifyDeviceImportStoreError(err)
	}
	return existing, nil
}

func (s *DeviceImportStore) existingCanonicalAddresses(
	ctx context.Context,
	addresses []string,
) (map[string]struct{}, error) {
	existing := make(map[string]struct{})
	canonical := canonicalDeviceImportAddresses(addresses)
	if len(canonical) == 0 {
		return existing, nil
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(canonical)), ",")
	args := make([]any, len(canonical))
	for i := range canonical {
		args[i] = canonical[i]
	}
	rows, err := s.devices.db.QueryContext(
		ctx,
		`SELECT DISTINCT normalized_address
		 FROM device_addresses
		 WHERE normalized_address IN (`+placeholders+`)`,
		args...,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var address string
		if err := rows.Scan(&address); err != nil {
			return nil, err
		}
		existing[address] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return existing, nil
}

// CreateDeviceInMap atomically appends one device and its map-local placement.
func (s *DeviceImportStore) CreateDeviceInMap(
	ctx context.Context,
	device *domain.Device,
	placement domain.DeviceImportPlacement,
) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if ctx.Err() != nil || s == nil || s.devices == nil || device == nil {
		return domain.ErrDeviceImportStoreUnavailable
	}
	persistenceDevice := cloneDeviceForImportPersistence(device)

	err := withWriteRetry(func() error {
		if ctx.Err() != nil {
			return domain.ErrDeviceImportStoreUnavailable
		}
		existing, err := s.existingCanonicalAddresses(ctx, domain.DeviceAddressValues(*persistenceDevice))
		if err != nil {
			return err
		}
		if len(existing) > 0 {
			return domain.ErrDeviceImportAddressConflict
		}

		return s.devices.createOnceWithAppend(
			persistenceDevice,
			func(tx *Tx, _ time.Time) error {
				return lockAndCheckDeviceImportAddresses(
					ctx,
					tx,
					domain.DeviceAddressValues(*persistenceDevice),
				)
			},
			func(tx *Tx, now time.Time) error {
				return appendImportedDevicePlacement(ctx, tx, persistenceDevice.ID, placement, now)
			},
			false,
		)
	})
	return classifyDeviceImportStoreError(err)
}

func lockAndCheckDeviceImportAddresses(ctx context.Context, tx *Tx, addresses []string) error {
	canonical := canonicalDeviceImportAddresses(addresses)
	lockKeys := make([]int64, 0, len(canonical))
	seenLockKeys := make(map[int64]struct{}, len(canonical))
	for _, address := range canonical {
		key := deviceImportAddressLockKey(address)
		if _, exists := seenLockKeys[key]; exists {
			continue
		}
		seenLockKeys[key] = struct{}{}
		lockKeys = append(lockKeys, key)
	}
	// A global numeric order prevents overlapping multi-address imports from deadlocking.
	sort.Slice(lockKeys, func(i, j int) bool { return lockKeys[i] < lockKeys[j] })
	for _, lockKey := range lockKeys {
		if _, err := tx.ExecContext(
			ctx,
			`SELECT pg_advisory_xact_lock(?)`,
			lockKey,
		); err != nil {
			return err
		}
	}
	if len(canonical) == 0 {
		return nil
	}

	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(canonical)), ",")
	args := make([]any, len(canonical))
	for i := range canonical {
		args[i] = canonical[i]
	}
	var exists bool
	if err := tx.QueryRowContext(
		ctx,
		`SELECT EXISTS (
			SELECT 1
			FROM device_addresses
			WHERE normalized_address IN (`+placeholders+`)
		)`,
		args...,
	).Scan(&exists); err != nil {
		return err
	}
	if exists {
		return domain.ErrDeviceImportAddressConflict
	}
	return nil
}

func deviceImportAddressLockKey(address string) int64 {
	hasher := fnv.New64a()
	_, _ = hasher.Write([]byte(address))
	return int64(hasher.Sum64())
}

func cloneDeviceForImportPersistence(device *domain.Device) *domain.Device {
	cloned := *device
	cloned.AreaIDs = nil
	cloned.ProbePorts = append([]int(nil), device.ProbePorts...)
	cloned.Interfaces = append([]domain.Interface(nil), device.Interfaces...)
	cloned.Addresses = append([]domain.DeviceAddress(nil), device.Addresses...)
	for i := range cloned.Addresses {
		cloned.Addresses[i].ProbePorts = append([]int(nil), device.Addresses[i].ProbePorts...)
	}
	if device.Tags != nil {
		cloned.Tags = make(map[string]string, len(device.Tags))
		for key, value := range device.Tags {
			cloned.Tags[key] = value
		}
	}
	cloned.SNMPCredentials = deepCopySNMPCredentials(device.SNMPCredentials)
	if device.PollingEnabled != nil {
		value := *device.PollingEnabled
		cloned.PollingEnabled = &value
	}
	if device.PollIntervalOverride != nil {
		value := *device.PollIntervalOverride
		cloned.PollIntervalOverride = &value
	}
	if device.Notes != nil {
		value := *device.Notes
		cloned.Notes = &value
	}
	if device.LastTopologyDiscoveryAt != nil {
		value := *device.LastTopologyDiscoveryAt
		cloned.LastTopologyDiscoveryAt = &value
	}
	return &cloned
}

// PublishCreatedDevices emits one batch invalidation followed by one created event per committed ID.
func (s *DeviceImportStore) PublishCreatedDevices(deviceIDs []uuid.UUID) {
	if s == nil || s.devices == nil || len(deviceIDs) == 0 {
		return
	}
	s.devices.notify()
	for _, deviceID := range deviceIDs {
		s.devices.publishChange(domain.ChangeKindCreated, deviceID)
	}
}

func appendImportedDevicePlacement(
	ctx context.Context,
	tx *Tx,
	deviceID uuid.UUID,
	placement domain.DeviceImportPlacement,
	now time.Time,
) error {
	var exists int
	if err := tx.QueryRowContext(
		ctx,
		`SELECT 1 FROM canvas_maps WHERE id = ? FOR KEY SHARE`,
		placement.MapID.String(),
	).Scan(&exists); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.ErrDeviceImportDestinationChanged
		}
		return err
	}
	if placement.AreaID != nil {
		if err := tx.QueryRowContext(
			ctx,
			`SELECT 1
			 FROM canvas_map_areas
			 WHERE map_id = ? AND area_id = ?
			 FOR KEY SHARE`,
			placement.MapID.String(),
			placement.AreaID.String(),
		).Scan(&exists); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return domain.ErrDeviceImportDestinationChanged
			}
			return err
		}
	}

	if _, err := tx.ExecContext(
		ctx,
		`INSERT INTO canvas_map_devices (map_id, device_id, role, added_at)
		 VALUES (?, ?, ?, ?)`,
		placement.MapID.String(),
		deviceID.String(),
		string(domain.CanvasMapDeviceRoleBase),
		now,
	); err != nil {
		return err
	}
	if placement.AreaID != nil {
		if _, err := tx.ExecContext(
			ctx,
			`INSERT INTO canvas_map_device_areas (map_id, device_id, area_id, assigned_at)
			 VALUES (?, ?, ?, ?)`,
			placement.MapID.String(),
			deviceID.String(),
			placement.AreaID.String(),
			now,
		); err != nil {
			return err
		}
	}
	result, err := tx.ExecContext(
		ctx,
		`UPDATE canvas_maps
		 SET membership_materialized = ?, updated_at = ?
		 WHERE id = ?`,
		true,
		now,
		placement.MapID.String(),
	)
	if err != nil {
		return err
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected != 1 {
		return domain.ErrDeviceImportDestinationChanged
	}
	return nil
}

func canonicalDeviceImportAddresses(addresses []string) []string {
	seen := make(map[string]struct{}, len(addresses))
	canonical := make([]string, 0, len(addresses))
	for _, address := range addresses {
		normalized := domain.NormalizeDeviceAddressValue(address)
		if normalized == "" {
			continue
		}
		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}
		canonical = append(canonical, normalized)
	}
	sort.Strings(canonical)
	return canonical
}

func classifyDeviceImportStoreError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, domain.ErrDeviceImportAddressConflict) {
		return domain.ErrDeviceImportAddressConflict
	}
	if errors.Is(err, domain.ErrDeviceImportDestinationChanged) || isDeviceImportDestinationConstraint(err) {
		return domain.ErrDeviceImportDestinationChanged
	}
	if errors.Is(err, domain.ErrDeviceImportStoreUnavailable) {
		return domain.ErrDeviceImportStoreUnavailable
	}
	if isDeviceImportAddressConstraint(err) {
		return domain.ErrDeviceImportAddressConflict
	}
	if isDeviceImportStoreUnavailableError(err) {
		return domain.ErrDeviceImportStoreUnavailable
	}
	return fmt.Errorf("persisting imported device: %w", err)
}

func isDeviceImportStoreUnavailableError(err error) bool {
	if errors.Is(err, context.Canceled) ||
		errors.Is(err, context.DeadlineExceeded) ||
		errors.Is(err, driver.ErrBadConn) ||
		errors.Is(err, sql.ErrConnDone) ||
		errors.Is(err, pgconn.ErrConnClosed) ||
		errors.Is(err, io.EOF) ||
		errors.Is(err, io.ErrUnexpectedEOF) {
		return true
	}

	var connectErr *pgconn.ConnectError
	if errors.As(err, &connectErr) {
		return true
	}
	var networkErr *net.OpError
	if errors.As(err, &networkErr) {
		return true
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return strings.HasPrefix(pgErr.Code, "08") ||
			pgErr.Code == "57P01" ||
			pgErr.Code == "57P02" ||
			pgErr.Code == "57P03"
	}
	return false
}

func isDeviceImportAddressConstraint(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && (pgErr.Code == "23505" || pgErr.Code == "23P01") {
		constraint := strings.ToLower(pgErr.ConstraintName)
		return constraint == "idx_devices_ip" ||
			constraint == "devices_ip_physical_virtual_excl" ||
			strings.Contains(constraint, "device_addresses")
	}
	message := strings.ToLower(err.Error())
	if !strings.Contains(message, "unique constraint failed") &&
		!strings.Contains(message, "duplicate key") &&
		!strings.Contains(message, "exclusion constraint") {
		return false
	}
	return strings.Contains(message, "device_addresses") ||
		strings.Contains(message, "devices.ip") ||
		strings.Contains(message, "idx_devices_ip") ||
		strings.Contains(message, "devices_ip_physical_virtual_excl")
}

func isDeviceImportDestinationConstraint(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23503" {
		constraint := strings.ToLower(pgErr.ConstraintName)
		return strings.Contains(constraint, "canvas_map_devices_map_id") ||
			strings.Contains(constraint, "canvas_map_device_areas_map_id_area_id")
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "foreign key constraint failed") &&
		(strings.Contains(message, "canvas_map_devices") || strings.Contains(message, "canvas_map_device_areas"))
}
