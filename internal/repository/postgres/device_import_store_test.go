package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/domain"
)

func TestDeviceImportStoreExistingCanonicalAddressesBulk(t *testing.T) {
	db := newTestDB(t)
	devices := NewDeviceRepo(db, testKeyring, nil)
	store := NewDeviceImportStore(devices)

	first := newDeviceImportTestDevice("10.70.0.1")
	first.Addresses = append(first.Addresses, domain.DeviceAddress{
		Address:  "  Mgmt.Example.NET.  ",
		Label:    "Management",
		Role:     domain.DeviceAddressRoleManagement,
		Priority: 10,
	})
	if err := devices.Create(first); err != nil {
		t.Fatalf("create first existing device: %v", err)
	}
	second := newDeviceImportTestDevice("10.70.0.2")
	if err := devices.Create(second); err != nil {
		t.Fatalf("create second existing device: %v", err)
	}

	got, err := store.ExistingCanonicalAddresses(context.Background(), []string{
		" 10.70.0.1 ",
		"mgmt.example.net.",
		"10.70.0.2",
		"missing.example.net",
		"MGMT.EXAMPLE.NET.",
	})
	if err != nil {
		t.Fatalf("ExistingCanonicalAddresses: %v", err)
	}
	want := map[string]struct{}{
		"10.70.0.1":         {},
		"mgmt.example.net.": {},
		"10.70.0.2":         {},
	}
	if len(got) != len(want) {
		t.Fatalf("existing address count = %d, want %d: %#v", len(got), len(want), got)
	}
	for address := range want {
		if _, ok := got[address]; !ok {
			t.Fatalf("existing addresses missing %q: %#v", address, got)
		}
	}
}

func TestDeviceImportStoreCreatesDeviceAndMapMembershipAtomically(t *testing.T) {
	db := newTestDB(t)
	mapID := uuid.New()
	areaID := uuid.New()
	insertDeviceImportTestMap(t, db, mapID)
	insertDeviceImportTestArea(t, db, mapID, areaID)

	devices := NewDeviceRepo(db, testKeyring, nil)
	store := NewDeviceImportStore(devices)
	device := newDeviceImportTestDevice("router-atomic.example.net")
	device.AreaIDs = nil

	if err := store.CreateDeviceInMap(context.Background(), device, domain.DeviceImportPlacement{
		MapID:  mapID,
		AreaID: &areaID,
	}); err != nil {
		t.Fatalf("CreateDeviceInMap: %v", err)
	}

	var storedCredentials string
	if err := db.QueryRow(
		`SELECT snmp_credentials_json FROM devices WHERE id = $1`,
		device.ID.String(),
	).Scan(&storedCredentials); err != nil {
		t.Fatalf("read stored device: %v", err)
	}
	if strings.Contains(storedCredentials, "import-secret-community") {
		t.Fatalf("stored credentials contain plaintext: %s", storedCredentials)
	}
	gotDevice, err := devices.GetByID(device.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if gotDevice.SNMPCredentials.V2c == nil || gotDevice.SNMPCredentials.V2c.Community != "import-secret-community" {
		t.Fatalf("decrypted credentials = %#v", gotDevice.SNMPCredentials)
	}

	var address, normalizedAddress, role string
	var isPrimary bool
	if err := db.QueryRow(
		`SELECT address, normalized_address, role, is_primary
		 FROM device_addresses WHERE device_id = $1`,
		device.ID.String(),
	).Scan(&address, &normalizedAddress, &role, &isPrimary); err != nil {
		t.Fatalf("read primary address: %v", err)
	}
	if address != "router-atomic.example.net" || normalizedAddress != "router-atomic.example.net" || role != "primary" || !isPrimary {
		t.Fatalf("stored primary address = (%q, %q, %q, %v)", address, normalizedAddress, role, isPrimary)
	}

	var membershipRole string
	if err := db.QueryRow(
		`SELECT role FROM canvas_map_devices WHERE map_id = $1 AND device_id = $2`,
		mapID.String(),
		device.ID.String(),
	).Scan(&membershipRole); err != nil {
		t.Fatalf("read base membership: %v", err)
	}
	if membershipRole != string(domain.CanvasMapDeviceRoleBase) {
		t.Fatalf("membership role = %q, want %q", membershipRole, domain.CanvasMapDeviceRoleBase)
	}
	if got := importTestCount(t, db,
		`SELECT COUNT(*) FROM canvas_map_device_areas WHERE map_id = $1 AND device_id = $2 AND area_id = $3`,
		mapID.String(), device.ID.String(), areaID.String()); got != 1 {
		t.Fatalf("map-local device area count = %d, want 1", got)
	}
	if got := importTestCount(t, db,
		`SELECT COUNT(*) FROM device_areas WHERE device_id = $1`, device.ID.String()); got != 0 {
		t.Fatalf("global device area count = %d, want 0", got)
	}
	if got := importTestCount(t, db,
		`SELECT COUNT(*) FROM device_positions WHERE device_id = $1`, device.ID.String()); got != 0 {
		t.Fatalf("legacy position count = %d, want 0", got)
	}
	if got := importTestCount(t, db,
		`SELECT COUNT(*) FROM canvas_map_positions WHERE map_id = $1 AND device_id = $2`,
		mapID.String(), device.ID.String()); got != 0 {
		t.Fatalf("canvas map position count = %d, want 0", got)
	}

	var materialized bool
	var updatedAt time.Time
	if err := db.QueryRow(
		`SELECT membership_materialized, updated_at FROM canvas_maps WHERE id = $1`,
		mapID.String(),
	).Scan(&materialized, &updatedAt); err != nil {
		t.Fatalf("read updated map: %v", err)
	}
	if !materialized {
		t.Fatal("map membership_materialized = false, want true")
	}
	if !updatedAt.After(deviceImportOldTimestamp) {
		t.Fatalf("map updated_at = %s, want after %s", updatedAt, deviceImportOldTimestamp)
	}
}

func TestDeviceImportStoreMissingMapRollsBack(t *testing.T) {
	db := newTestDB(t)
	store := NewDeviceImportStore(NewDeviceRepo(db, testKeyring, nil))
	device := newDeviceImportTestDevice("missing-map.example.net")

	err := store.CreateDeviceInMap(context.Background(), device, domain.DeviceImportPlacement{MapID: uuid.New()})
	if err != domain.ErrDeviceImportDestinationChanged {
		t.Fatalf("CreateDeviceInMap error = %v, want exact destination-changed sentinel", err)
	}
	assertDeviceImportTargetAbsent(t, db, device.ID)
}

func TestDeviceImportStoreAreaFromAnotherMapRollsBack(t *testing.T) {
	db := newTestDB(t)
	selectedMapID := uuid.New()
	otherMapID := uuid.New()
	areaID := uuid.New()
	insertDeviceImportTestMap(t, db, selectedMapID)
	insertDeviceImportTestMap(t, db, otherMapID)
	insertDeviceImportTestArea(t, db, otherMapID, areaID)
	store := NewDeviceImportStore(NewDeviceRepo(db, testKeyring, nil))
	device := newDeviceImportTestDevice("wrong-area.example.net")

	err := store.CreateDeviceInMap(context.Background(), device, domain.DeviceImportPlacement{
		MapID:  selectedMapID,
		AreaID: &areaID,
	})
	if err != domain.ErrDeviceImportDestinationChanged {
		t.Fatalf("CreateDeviceInMap error = %v, want exact destination-changed sentinel", err)
	}
	assertDeviceImportTargetAbsent(t, db, device.ID)
}

func TestDeviceImportStoreExistingAddressReturnsConflict(t *testing.T) {
	db := newTestDB(t)
	mapID := uuid.New()
	insertDeviceImportTestMap(t, db, mapID)
	devices := NewDeviceRepo(db, testKeyring, nil)
	store := NewDeviceImportStore(devices)
	existing := newDeviceImportTestDevice("duplicate.example.net")
	if err := devices.Create(existing); err != nil {
		t.Fatalf("create existing device: %v", err)
	}
	device := newDeviceImportTestDevice("  DUPLICATE.EXAMPLE.NET  ")

	err := store.CreateDeviceInMap(context.Background(), device, domain.DeviceImportPlacement{MapID: mapID})
	if err != domain.ErrDeviceImportAddressConflict {
		t.Fatalf("CreateDeviceInMap error = %v, want exact address-conflict sentinel", err)
	}
	assertDeviceImportTargetAbsent(t, db, device.ID)
	if got := importTestCount(t, db, `SELECT COUNT(*) FROM devices`); got != 1 {
		t.Fatalf("device count = %d, want only the existing device", got)
	}
}

func TestDeviceImportStoreConcurrentSameAddressAllowsOneCreate(t *testing.T) {
	db := newTestDB(t)
	mapID := uuid.New()
	insertDeviceImportTestMap(t, db, mapID)
	const advisoryLockKey int64 = 340031
	installDeviceImportInsertPause(t, db, advisoryLockKey)
	release := holdDeviceImportAdvisoryLock(t, db, advisoryLockKey)

	store := NewDeviceImportStore(NewDeviceRepo(db, testKeyring, nil))
	first := newDeviceImportTestDevice("race.example.net")
	second := newDeviceImportTestDevice(" RACE.EXAMPLE.NET ")
	start := make(chan struct{})
	results := make(chan error, 2)
	for _, device := range []*domain.Device{first, second} {
		device := device
		go func() {
			<-start
			results <- store.CreateDeviceInMap(context.Background(), device, domain.DeviceImportPlacement{MapID: mapID})
		}()
	}
	close(start)
	waitForDeviceImportAdvisoryWaiters(t, db, advisoryLockKey, 2)
	release()

	firstErr := waitForDeviceImportResult(t, results)
	secondErr := waitForDeviceImportResult(t, results)
	errorsSeen := []error{firstErr, secondErr}
	successes := 0
	conflicts := 0
	for _, err := range errorsSeen {
		switch {
		case err == nil:
			successes++
		case err == domain.ErrDeviceImportAddressConflict:
			conflicts++
		default:
			t.Fatalf("concurrent CreateDeviceInMap error = %v", err)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("concurrent results = %#v, want one success and one conflict", errorsSeen)
	}
	if got := importTestCount(t, db, `SELECT COUNT(*) FROM devices`); got != 1 {
		t.Fatalf("device count = %d, want 1", got)
	}
	if got := importTestCount(t, db, `SELECT COUNT(*) FROM device_addresses`); got != 1 {
		t.Fatalf("address count = %d, want 1", got)
	}
	if got := importTestCount(t, db, `SELECT COUNT(*) FROM canvas_map_devices WHERE map_id = $1`, mapID.String()); got != 1 {
		t.Fatalf("map membership count = %d, want 1", got)
	}
}

func TestDeviceImportStoreDestinationRemovedDuringTransaction(t *testing.T) {
	t.Run("map", func(t *testing.T) {
		db := newTestDB(t)
		mapID := uuid.New()
		insertDeviceImportTestMap(t, db, mapID)
		device := newDeviceImportTestDevice("removed-map.example.net")
		const advisoryLockKey int64 = 340032
		installDeviceImportInsertPause(t, db, advisoryLockKey)
		release := holdDeviceImportAdvisoryLock(t, db, advisoryLockKey)

		result := make(chan error, 1)
		store := NewDeviceImportStore(NewDeviceRepo(db, testKeyring, nil))
		go func() {
			result <- store.CreateDeviceInMap(context.Background(), device, domain.DeviceImportPlacement{MapID: mapID})
		}()
		waitForDeviceImportAdvisoryWaiters(t, db, advisoryLockKey, 1)
		if _, err := db.Exec(`DELETE FROM canvas_maps WHERE id = $1`, mapID.String()); err != nil {
			t.Fatalf("remove map during import: %v", err)
		}
		release()

		if err := waitForDeviceImportResult(t, result); err != domain.ErrDeviceImportDestinationChanged {
			t.Fatalf("CreateDeviceInMap error = %v, want exact destination-changed sentinel", err)
		}
		assertDeviceImportTargetAbsent(t, db, device.ID)
	})

	t.Run("map local area", func(t *testing.T) {
		db := newTestDB(t)
		mapID := uuid.New()
		areaID := uuid.New()
		insertDeviceImportTestMap(t, db, mapID)
		insertDeviceImportTestArea(t, db, mapID, areaID)
		device := newDeviceImportTestDevice("removed-area.example.net")
		const advisoryLockKey int64 = 340033
		installDeviceImportInsertPause(t, db, advisoryLockKey)
		release := holdDeviceImportAdvisoryLock(t, db, advisoryLockKey)

		result := make(chan error, 1)
		store := NewDeviceImportStore(NewDeviceRepo(db, testKeyring, nil))
		go func() {
			result <- store.CreateDeviceInMap(context.Background(), device, domain.DeviceImportPlacement{
				MapID:  mapID,
				AreaID: &areaID,
			})
		}()
		waitForDeviceImportAdvisoryWaiters(t, db, advisoryLockKey, 1)
		if _, err := db.Exec(
			`DELETE FROM canvas_map_areas WHERE map_id = $1 AND area_id = $2`,
			mapID.String(),
			areaID.String(),
		); err != nil {
			t.Fatalf("remove map-local area during import: %v", err)
		}
		release()

		if err := waitForDeviceImportResult(t, result); err != domain.ErrDeviceImportDestinationChanged {
			t.Fatalf("CreateDeviceInMap error = %v, want exact destination-changed sentinel", err)
		}
		assertDeviceImportTargetAbsent(t, db, device.ID)
	})
}

func TestDeviceImportStoreUnavailableErrorsAreTyped(t *testing.T) {
	t.Run("cancelled context", func(t *testing.T) {
		db := newTestDB(t)
		mapID := uuid.New()
		insertDeviceImportTestMap(t, db, mapID)
		device := newDeviceImportTestDevice("cancelled.example.net")
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		err := NewDeviceImportStore(NewDeviceRepo(db, testKeyring, nil)).CreateDeviceInMap(
			ctx,
			device,
			domain.DeviceImportPlacement{MapID: mapID},
		)
		if err != domain.ErrDeviceImportStoreUnavailable {
			t.Fatalf("CreateDeviceInMap error = %v, want exact store-unavailable sentinel", err)
		}
		assertDeviceImportTargetAbsent(t, db, device.ID)
	})

	t.Run("connection loss", func(t *testing.T) {
		db := newTestDB(t)
		mapID := uuid.New()
		insertDeviceImportTestMap(t, db, mapID)
		device := newDeviceImportTestDevice("connection-loss.example.net")
		const advisoryLockKey int64 = 340034
		installDeviceImportInsertPause(t, db, advisoryLockKey)
		release := holdDeviceImportAdvisoryLock(t, db, advisoryLockKey)

		result := make(chan error, 1)
		store := NewDeviceImportStore(NewDeviceRepo(db, testKeyring, nil))
		go func() {
			result <- store.CreateDeviceInMap(context.Background(), device, domain.DeviceImportPlacement{MapID: mapID})
		}()
		waiterPID := waitForDeviceImportAdvisoryWaiterPID(t, db, advisoryLockKey)
		var terminated bool
		if err := db.QueryRow(`SELECT pg_terminate_backend($1)`, waiterPID).Scan(&terminated); err != nil {
			t.Fatalf("terminate import backend: %v", err)
		}
		if !terminated {
			t.Fatalf("pg_terminate_backend(%d) = false", waiterPID)
		}
		release()

		if err := waitForDeviceImportResult(t, result); err != domain.ErrDeviceImportStoreUnavailable {
			t.Fatalf("CreateDeviceInMap error = %v, want exact store-unavailable sentinel", err)
		}
		assertDeviceImportTargetAbsent(t, db, device.ID)
	})

	t.Run("SQLSTATE class 08", func(t *testing.T) {
		db := newTestDB(t)
		mapID := uuid.New()
		insertDeviceImportTestMap(t, db, mapID)
		installDeviceImportFailureTrigger(t, db, "devices", "08006", "test import connection failure")
		device := newDeviceImportTestDevice("sqlstate-08.example.net")

		err := NewDeviceImportStore(NewDeviceRepo(db, testKeyring, nil)).CreateDeviceInMap(
			context.Background(),
			device,
			domain.DeviceImportPlacement{MapID: mapID},
		)
		if err != domain.ErrDeviceImportStoreUnavailable {
			t.Fatalf("CreateDeviceInMap error = %v, want exact store-unavailable sentinel", err)
		}
		assertDeviceImportTargetAbsent(t, db, device.ID)
	})
}

func TestDeviceImportStoreForcedPostDeviceFailureRollsBack(t *testing.T) {
	db := newTestDB(t)
	mapID := uuid.New()
	insertDeviceImportTestMap(t, db, mapID)
	installDeviceImportFailureTrigger(t, db, "canvas_map_devices", "P0001", "forced map membership failure")
	device := newDeviceImportTestDevice("forced-rollback.example.net")

	err := NewDeviceImportStore(NewDeviceRepo(db, testKeyring, nil)).CreateDeviceInMap(
		context.Background(),
		device,
		domain.DeviceImportPlacement{MapID: mapID},
	)
	if err != domain.ErrDeviceImportStoreUnavailable {
		t.Fatalf("CreateDeviceInMap error = %v, want exact store-unavailable sentinel", err)
	}
	assertDeviceImportTargetAbsent(t, db, device.ID)
	if got := importTestCount(t, db, `SELECT COUNT(*) FROM canvas_maps WHERE id = $1`, mapID.String()); got != 1 {
		t.Fatalf("destination map count = %d, want 1", got)
	}
}

func TestDeviceImportStorePublishesOnlyAfterBatchCompletion(t *testing.T) {
	db := newTestDB(t)
	mapID := uuid.New()
	insertDeviceImportTestMap(t, db, mapID)
	onChange := make(chan struct{}, 4)
	devices := NewDeviceRepo(db, testKeyring, onChange)
	changes := devices.SubscribeDeviceChanges(4)
	store := NewDeviceImportStore(devices)
	first := newDeviceImportTestDevice("publish-first.example.net")
	second := newDeviceImportTestDevice("publish-second.example.net")

	for _, device := range []*domain.Device{first, second} {
		if err := store.CreateDeviceInMap(context.Background(), device, domain.DeviceImportPlacement{MapID: mapID}); err != nil {
			t.Fatalf("CreateDeviceInMap(%s): %v", device.IP, err)
		}
	}
	assertNoDeviceImportSignal(t, onChange, "cache invalidation before publication")
	assertNoDeviceImportEvent(t, changes, "created event before publication")

	store.PublishCreatedDevices([]uuid.UUID{first.ID, second.ID})
	select {
	case <-onChange:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for batch cache invalidation")
	}
	assertNoDeviceImportSignal(t, onChange, "second cache invalidation")
	for _, wantID := range []uuid.UUID{first.ID, second.ID} {
		select {
		case event := <-changes:
			if event.Kind != domain.ChangeKindCreated || event.DeviceID != wantID {
				t.Fatalf("device event = %#v, want created event for %s", event, wantID)
			}
		case <-time.After(time.Second):
			t.Fatalf("timed out waiting for created event for %s", wantID)
		}
	}
	assertNoDeviceImportEvent(t, changes, "extra created event")
}

var deviceImportOldTimestamp = time.Date(2000, time.January, 1, 0, 0, 0, 0, time.UTC)

func newDeviceImportTestDevice(address string) *domain.Device {
	id := uuid.New()
	return &domain.Device{
		ID:         id,
		Hostname:   "import-" + id.String()[len(id.String())-8:],
		IP:         address,
		DeviceType: domain.DeviceTypeRouter,
		Status:     domain.DeviceStatusProbing,
		Managed:    true,
		Tags:       map[string]string{"source": "prometheus-file-sd"},
		SNMPCredentials: domain.SNMPCredentials{
			Version: domain.SNMPVersionV2c,
			V2c:     &domain.SNMPv2cCredentials{Community: "import-secret-community"},
		},
	}
}

func insertDeviceImportTestMap(t *testing.T, db *sql.DB, mapID uuid.UUID) {
	t.Helper()
	if _, err := db.Exec(
		`INSERT INTO canvas_maps (
			id, name, description, filter_json, is_default, membership_materialized, created_at, updated_at
		 ) VALUES ($1, $2, '', '{}'::jsonb, FALSE, FALSE, $3, $3)`,
		mapID.String(),
		"Import "+mapID.String()[len(mapID.String())-8:],
		deviceImportOldTimestamp,
	); err != nil {
		t.Fatalf("insert import test map: %v", err)
	}
}

func insertDeviceImportTestArea(t *testing.T, db *sql.DB, mapID, areaID uuid.UUID) {
	t.Helper()
	if _, err := db.Exec(
		`INSERT INTO canvas_map_areas (map_id, area_id, name, description, color, added_at)
		 VALUES ($1, $2, $3, '', '#123456', CURRENT_TIMESTAMP)`,
		mapID.String(),
		areaID.String(),
		"Area "+areaID.String()[len(areaID.String())-8:],
	); err != nil {
		t.Fatalf("insert import test area: %v", err)
	}
}

func importTestCount(t *testing.T, db *sql.DB, query string, args ...any) int {
	t.Helper()
	var count int
	if err := db.QueryRow(query, args...).Scan(&count); err != nil {
		t.Fatalf("count import test rows: %v", err)
	}
	return count
}

func assertDeviceImportTargetAbsent(t *testing.T, db *sql.DB, deviceID uuid.UUID) {
	t.Helper()
	checks := []struct {
		name  string
		query string
	}{
		{name: "device", query: `SELECT COUNT(*) FROM devices WHERE id = $1`},
		{name: "address", query: `SELECT COUNT(*) FROM device_addresses WHERE device_id = $1`},
		{name: "global area", query: `SELECT COUNT(*) FROM device_areas WHERE device_id = $1`},
		{name: "map membership", query: `SELECT COUNT(*) FROM canvas_map_devices WHERE device_id = $1`},
		{name: "map-local area", query: `SELECT COUNT(*) FROM canvas_map_device_areas WHERE device_id = $1`},
		{name: "legacy position", query: `SELECT COUNT(*) FROM device_positions WHERE device_id = $1`},
		{name: "map position", query: `SELECT COUNT(*) FROM canvas_map_positions WHERE device_id = $1`},
	}
	for _, check := range checks {
		if got := importTestCount(t, db, check.query, deviceID.String()); got != 0 {
			t.Fatalf("%s count = %d, want 0", check.name, got)
		}
	}
}

func installDeviceImportInsertPause(t *testing.T, db *sql.DB, advisoryLockKey int64) {
	t.Helper()
	if _, err := db.Exec(`DROP TRIGGER IF EXISTS test_pause_device_import_insert ON devices`); err != nil {
		t.Fatalf("drop device import pause trigger: %v", err)
	}
	functionSQL := fmt.Sprintf(`
		CREATE OR REPLACE FUNCTION test_pause_device_import_insert()
		RETURNS trigger AS $$
		BEGIN
			PERFORM pg_advisory_xact_lock(%d);
			RETURN NEW;
		END;
		$$ LANGUAGE plpgsql`, advisoryLockKey)
	if _, err := db.Exec(functionSQL); err != nil {
		t.Fatalf("install device import pause function: %v", err)
	}
	if _, err := db.Exec(`
		CREATE TRIGGER test_pause_device_import_insert
		BEFORE INSERT ON devices
		FOR EACH ROW EXECUTE FUNCTION test_pause_device_import_insert()`); err != nil {
		t.Fatalf("install device import pause trigger: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.Exec(`DROP TRIGGER IF EXISTS test_pause_device_import_insert ON devices`)
		_, _ = db.Exec(`DROP FUNCTION IF EXISTS test_pause_device_import_insert()`)
	})
}

func holdDeviceImportAdvisoryLock(t *testing.T, db *sql.DB, advisoryLockKey int64) func() {
	t.Helper()
	conn, err := db.Conn(context.Background())
	if err != nil {
		t.Fatalf("reserve advisory lock connection: %v", err)
	}
	if _, err := conn.ExecContext(context.Background(), `SELECT pg_advisory_lock($1)`, advisoryLockKey); err != nil {
		conn.Close()
		t.Fatalf("hold device import advisory lock: %v", err)
	}
	var once sync.Once
	release := func() {
		once.Do(func() {
			_, _ = conn.ExecContext(context.Background(), `SELECT pg_advisory_unlock($1)`, advisoryLockKey)
			_ = conn.Close()
		})
	}
	t.Cleanup(release)
	return release
}

func waitForDeviceImportAdvisoryWaiters(
	t *testing.T,
	db *sql.DB,
	advisoryLockKey int64,
	want int,
) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		var count int
		if err := db.QueryRow(
			`SELECT COUNT(*)
			 FROM pg_locks
			 WHERE locktype = 'advisory'
			   AND granted = FALSE
			   AND classid = 0
			   AND objid = $1::oid`,
			advisoryLockKey,
		).Scan(&count); err != nil {
			t.Fatalf("query import advisory waiters: %v", err)
		}
		if count >= want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %d device import advisory waiter(s)", want)
}

func waitForDeviceImportAdvisoryWaiterPID(t *testing.T, db *sql.DB, advisoryLockKey int64) int {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		var pid int
		err := db.QueryRow(
			`SELECT pid
			 FROM pg_locks
			 WHERE locktype = 'advisory'
			   AND granted = FALSE
			   AND classid = 0
			   AND objid = $1::oid
			 ORDER BY pid
			 LIMIT 1`,
			advisoryLockKey,
		).Scan(&pid)
		if err == nil {
			return pid
		}
		if !errors.Is(err, sql.ErrNoRows) {
			t.Fatalf("query import advisory waiter pid: %v", err)
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("timed out waiting for device import advisory waiter pid")
	return 0
}

func installDeviceImportFailureTrigger(
	t *testing.T,
	db *sql.DB,
	tableName string,
	sqlState string,
	message string,
) {
	t.Helper()
	if tableName != "devices" && tableName != "canvas_map_devices" {
		t.Fatalf("unsupported import failure trigger table %q", tableName)
	}
	triggerName := "test_fail_device_import_" + tableName
	functionName := triggerName
	if _, err := db.Exec(fmt.Sprintf(`DROP TRIGGER IF EXISTS %s ON %s`, triggerName, tableName)); err != nil {
		t.Fatalf("drop import failure trigger: %v", err)
	}
	functionSQL := fmt.Sprintf(`
		CREATE OR REPLACE FUNCTION %s()
		RETURNS trigger AS $$
		BEGIN
			RAISE EXCEPTION USING ERRCODE = '%s', MESSAGE = '%s';
		END;
		$$ LANGUAGE plpgsql`, functionName, sqlState, strings.ReplaceAll(message, "'", "''"))
	if _, err := db.Exec(functionSQL); err != nil {
		t.Fatalf("install import failure function: %v", err)
	}
	triggerSQL := fmt.Sprintf(`
		CREATE TRIGGER %s
		BEFORE INSERT ON %s
		FOR EACH ROW EXECUTE FUNCTION %s()`, triggerName, tableName, functionName)
	if _, err := db.Exec(triggerSQL); err != nil {
		t.Fatalf("install import failure trigger: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.Exec(fmt.Sprintf(`DROP TRIGGER IF EXISTS %s ON %s`, triggerName, tableName))
		_, _ = db.Exec(fmt.Sprintf(`DROP FUNCTION IF EXISTS %s()`, functionName))
	})
}

func waitForDeviceImportResult(t *testing.T, results <-chan error) error {
	t.Helper()
	select {
	case err := <-results:
		return err
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for device import result")
		return nil
	}
}

func assertNoDeviceImportSignal(t *testing.T, signals <-chan struct{}, description string) {
	t.Helper()
	select {
	case <-signals:
		t.Fatalf("unexpected %s", description)
	default:
	}
}

func assertNoDeviceImportEvent(t *testing.T, events <-chan domain.DeviceChangeEvent, description string) {
	t.Helper()
	select {
	case event := <-events:
		t.Fatalf("unexpected %s: %#v", description, event)
	default:
	}
}
