package cache

import (
	"database/sql"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/domain"
	sqliterepo "github.com/lollinoo/theia/internal/repository/sqlite"
	_ "github.com/mattn/go-sqlite3"
)

var errNotFound = errors.New("not found")
var testEncryptionKey = []byte("0123456789abcdef0123456789abcdef")

type legacyMockDeviceRepo struct {
	getAllCount int32
	devices     []domain.Device
}

func (r *legacyMockDeviceRepo) Create(_ *domain.Device) error                 { return nil }
func (r *legacyMockDeviceRepo) GetByID(_ uuid.UUID) (*domain.Device, error)   { return nil, errNotFound }
func (r *legacyMockDeviceRepo) GetByIP(_ string) (*domain.Device, error)      { return nil, nil }
func (r *legacyMockDeviceRepo) GetBySysName(_ string) (*domain.Device, error) { return nil, nil }
func (r *legacyMockDeviceRepo) Update(_ *domain.Device) error                 { return nil }
func (r *legacyMockDeviceRepo) Delete(_ uuid.UUID) error                      { return nil }

func (r *legacyMockDeviceRepo) GetAll() ([]domain.Device, error) {
	atomic.AddInt32(&r.getAllCount, 1)
	result := make([]domain.Device, len(r.devices))
	copy(result, r.devices)
	return result, nil
}

type legacyMockLinkRepo struct {
	getAllCount int32
	links       []domain.Link
}

func (r *legacyMockLinkRepo) Create(_ *domain.Link) error               { return nil }
func (r *legacyMockLinkRepo) GetByID(_ uuid.UUID) (*domain.Link, error) { return nil, errNotFound }
func (r *legacyMockLinkRepo) GetByDeviceID(_ uuid.UUID) ([]domain.Link, error) {
	return nil, nil
}
func (r *legacyMockLinkRepo) Update(_ *domain.Link) error         { return nil }
func (r *legacyMockLinkRepo) Delete(_ uuid.UUID) error            { return nil }
func (r *legacyMockLinkRepo) Upsert(_ *domain.Link) (bool, error) { return false, nil }

func (r *legacyMockLinkRepo) GetAll() ([]domain.Link, error) {
	atomic.AddInt32(&r.getAllCount, 1)
	result := make([]domain.Link, len(r.links))
	copy(result, r.links)
	return result, nil
}

type mockDeviceRepo struct {
	mu           sync.RWMutex
	getAllCount  int32
	getByIDCount int32
	devices      map[uuid.UUID]domain.Device
	changeCh     chan domain.DeviceChangeEvent
	repair       atomic.Bool
}

func newMockDeviceRepo(devices ...domain.Device) *mockDeviceRepo {
	repo := &mockDeviceRepo{
		devices:  make(map[uuid.UUID]domain.Device, len(devices)),
		changeCh: make(chan domain.DeviceChangeEvent, 64),
	}
	for _, device := range devices {
		repo.devices[device.ID] = device
	}
	return repo
}

func (r *mockDeviceRepo) Create(_ *domain.Device) error                 { return nil }
func (r *mockDeviceRepo) GetByIP(_ string) (*domain.Device, error)      { return nil, nil }
func (r *mockDeviceRepo) GetBySysName(_ string) (*domain.Device, error) { return nil, nil }
func (r *mockDeviceRepo) Update(_ *domain.Device) error                 { return nil }
func (r *mockDeviceRepo) Delete(_ uuid.UUID) error                      { return nil }

func (r *mockDeviceRepo) GetByID(id uuid.UUID) (*domain.Device, error) {
	atomic.AddInt32(&r.getByIDCount, 1)
	r.mu.RLock()
	defer r.mu.RUnlock()

	device, ok := r.devices[id]
	if !ok {
		return nil, errNotFound
	}
	copy := device
	return &copy, nil
}

func (r *mockDeviceRepo) GetAll() ([]domain.Device, error) {
	atomic.AddInt32(&r.getAllCount, 1)
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]domain.Device, 0, len(r.devices))
	for _, device := range r.devices {
		result = append(result, device)
	}
	return result, nil
}

func (r *mockDeviceRepo) DeviceChanges() <-chan domain.DeviceChangeEvent {
	return r.changeCh
}

func (r *mockDeviceRepo) DrainDeviceRepair() bool {
	return r.repair.Swap(false)
}

func (r *mockDeviceRepo) upsertDevice(device domain.Device, kind domain.ChangeKind) {
	r.mu.Lock()
	r.devices[device.ID] = device
	r.mu.Unlock()
	r.changeCh <- domain.DeviceChangeEvent{Kind: kind, DeviceID: device.ID}
}

func (r *mockDeviceRepo) deleteDevice(id uuid.UUID) {
	r.mu.Lock()
	delete(r.devices, id)
	r.mu.Unlock()
	r.changeCh <- domain.DeviceChangeEvent{Kind: domain.ChangeKindDeleted, DeviceID: id}
}

func (r *mockDeviceRepo) insertWithoutEvent(device domain.Device) {
	r.mu.Lock()
	r.devices[device.ID] = device
	r.mu.Unlock()
}

func (r *mockDeviceRepo) markRepair() {
	r.repair.Store(true)
}

type mockLinkRepo struct {
	mu           sync.RWMutex
	getAllCount  int32
	getByIDCount int32
	links        map[uuid.UUID]domain.Link
	changeCh     chan domain.LinkChangeEvent
	repair       atomic.Bool
}

func newMockLinkRepo(links ...domain.Link) *mockLinkRepo {
	repo := &mockLinkRepo{
		links:    make(map[uuid.UUID]domain.Link, len(links)),
		changeCh: make(chan domain.LinkChangeEvent, 64),
	}
	for _, link := range links {
		repo.links[link.ID] = link
	}
	return repo
}

func (r *mockLinkRepo) Create(_ *domain.Link) error { return nil }
func (r *mockLinkRepo) GetByDeviceID(_ uuid.UUID) ([]domain.Link, error) {
	return nil, nil
}
func (r *mockLinkRepo) Update(_ *domain.Link) error         { return nil }
func (r *mockLinkRepo) Delete(_ uuid.UUID) error            { return nil }
func (r *mockLinkRepo) Upsert(_ *domain.Link) (bool, error) { return false, nil }

func (r *mockLinkRepo) GetByID(id uuid.UUID) (*domain.Link, error) {
	atomic.AddInt32(&r.getByIDCount, 1)
	r.mu.RLock()
	defer r.mu.RUnlock()

	link, ok := r.links[id]
	if !ok {
		return nil, errNotFound
	}
	copy := link
	return &copy, nil
}

func (r *mockLinkRepo) GetAll() ([]domain.Link, error) {
	atomic.AddInt32(&r.getAllCount, 1)
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]domain.Link, 0, len(r.links))
	for _, link := range r.links {
		result = append(result, link)
	}
	return result, nil
}

func (r *mockLinkRepo) LinkChanges() <-chan domain.LinkChangeEvent {
	return r.changeCh
}

func (r *mockLinkRepo) DrainLinkRepair() bool {
	return r.repair.Swap(false)
}

func (r *mockLinkRepo) upsertLink(link domain.Link, kind domain.ChangeKind) {
	r.mu.Lock()
	r.links[link.ID] = link
	r.mu.Unlock()
	r.changeCh <- domain.LinkChangeEvent{Kind: kind, LinkID: link.ID}
}

func (r *mockLinkRepo) deleteLink(id uuid.UUID) {
	r.mu.Lock()
	delete(r.links, id)
	r.mu.Unlock()
	r.changeCh <- domain.LinkChangeEvent{Kind: domain.ChangeKindDeleted, LinkID: id}
}

func (r *mockLinkRepo) markRepair() {
	r.repair.Store(true)
}

func TestCacheLegacyInvalidationReloadsWholeSet(t *testing.T) {
	deviceRepo := &legacyMockDeviceRepo{
		devices: []domain.Device{
			{ID: uuid.New(), Hostname: "dev1", IP: "10.0.0.1"},
		},
	}
	linkRepo := &legacyMockLinkRepo{}
	invalidateCh := make(chan struct{}, 1)
	cache := NewDeviceLinkCache(deviceRepo, linkRepo, invalidateCh)

	if _, err := cache.GetDevices(); err != nil {
		t.Fatalf("GetDevices initial: %v", err)
	}
	if count := atomic.LoadInt32(&deviceRepo.getAllCount); count != 1 {
		t.Fatalf("GetAll count = %d, want 1", count)
	}

	invalidateCh <- struct{}{}

	if _, err := cache.GetDevices(); err != nil {
		t.Fatalf("GetDevices after invalidation: %v", err)
	}
	if count := atomic.LoadInt32(&deviceRepo.getAllCount); count != 2 {
		t.Fatalf("GetAll count = %d, want 2 after invalidation", count)
	}
}

func TestCacheIncrementalDeviceChangeAvoidsFullReload(t *testing.T) {
	initial := domain.Device{
		ID:       uuid.New(),
		Hostname: "alpha",
		IP:       "10.0.0.1",
		SysName:  "alpha.example.com.",
	}
	deviceRepo := newMockDeviceRepo(initial)
	linkRepo := newMockLinkRepo()
	cache := NewDeviceLinkCache(deviceRepo, linkRepo, make(chan struct{}, 1))

	devices, err := cache.GetDevices()
	if err != nil {
		t.Fatalf("GetDevices initial: %v", err)
	}
	if len(devices) != 1 {
		t.Fatalf("len(devices) = %d, want 1", len(devices))
	}

	added := domain.Device{
		ID:       uuid.New(),
		Hostname: "beta",
		IP:       "10.0.0.2",
		SysName:  "beta.example.com.",
	}
	deviceRepo.upsertDevice(added, domain.ChangeKindCreated)

	devices, err = cache.GetDevices()
	if err != nil {
		t.Fatalf("GetDevices after incremental change: %v", err)
	}
	if len(devices) != 2 {
		t.Fatalf("len(devices) = %d, want 2", len(devices))
	}
	if count := atomic.LoadInt32(&deviceRepo.getAllCount); count != 1 {
		t.Fatalf("GetAll count = %d, want 1 with incremental update", count)
	}

	resolved, ok, err := cache.GetDeviceBySysName("BETA")
	if err != nil {
		t.Fatalf("GetDeviceBySysName: %v", err)
	}
	if !ok || resolved.ID != added.ID {
		t.Fatalf("GetDeviceBySysName returned %#v, %v; want device %s", resolved, ok, added.ID)
	}
}

func TestCacheIncrementalLinkChangeUpdatesIndexesWithoutFullReload(t *testing.T) {
	sourceID := uuid.New()
	targetID := uuid.New()
	deviceRepo := newMockDeviceRepo(
		domain.Device{ID: sourceID, Hostname: "alpha", IP: "10.0.0.1"},
		domain.Device{ID: targetID, Hostname: "beta", IP: "10.0.0.2"},
	)
	linkRepo := newMockLinkRepo()
	cache := NewDeviceLinkCache(deviceRepo, linkRepo, make(chan struct{}, 1))

	if _, err := cache.GetLinks(); err != nil {
		t.Fatalf("GetLinks initial: %v", err)
	}

	link := domain.Link{
		ID:                uuid.New(),
		SourceDeviceID:    sourceID,
		SourceIfName:      "ether1",
		TargetDeviceID:    targetID,
		TargetIfName:      "ether2",
		DiscoveryProtocol: domain.DiscoveryProtocolLLDP,
		CreatedAt:         time.Unix(10, 0).UTC(),
	}
	linkRepo.upsertLink(link, domain.ChangeKindCreated)

	links, err := cache.GetLinks()
	if err != nil {
		t.Fatalf("GetLinks after incremental change: %v", err)
	}
	if len(links) != 1 {
		t.Fatalf("len(links) = %d, want 1", len(links))
	}
	if count := atomic.LoadInt32(&linkRepo.getAllCount); count != 1 {
		t.Fatalf("GetAll count = %d, want 1 with incremental link update", count)
	}

	resolved, ok, err := cache.GetLinkByEndpointPair(sourceID, "ETHER1", targetID, "ether2")
	if err != nil {
		t.Fatalf("GetLinkByEndpointPair: %v", err)
	}
	if !ok || resolved.ID != link.ID {
		t.Fatalf("GetLinkByEndpointPair returned %#v, %v; want link %s", resolved, ok, link.ID)
	}
}

func TestCacheRepairFallsBackToFullReload(t *testing.T) {
	initial := domain.Device{
		ID:       uuid.New(),
		Hostname: "alpha",
		IP:       "10.0.0.1",
	}
	deviceRepo := newMockDeviceRepo(initial)
	linkRepo := newMockLinkRepo()
	cache := NewDeviceLinkCache(deviceRepo, linkRepo, make(chan struct{}, 1))

	if _, err := cache.GetDevices(); err != nil {
		t.Fatalf("GetDevices initial: %v", err)
	}

	added := domain.Device{
		ID:       uuid.New(),
		Hostname: "beta",
		IP:       "10.0.0.2",
	}
	deviceRepo.insertWithoutEvent(added)
	deviceRepo.markRepair()

	devices, err := cache.GetDevices()
	if err != nil {
		t.Fatalf("GetDevices after repair: %v", err)
	}
	if len(devices) != 2 {
		t.Fatalf("len(devices) = %d, want 2 after repair reload", len(devices))
	}
	if count := atomic.LoadInt32(&deviceRepo.getAllCount); count != 2 {
		t.Fatalf("GetAll count = %d, want 2 after repair reload", count)
	}
}

func TestCacheConcurrentReadsWhileApplyingIncrementalUpdates(t *testing.T) {
	sourceID := uuid.New()
	targetID := uuid.New()
	deviceRepo := newMockDeviceRepo(
		domain.Device{ID: sourceID, Hostname: "alpha", IP: "10.0.0.1"},
		domain.Device{ID: targetID, Hostname: "beta", IP: "10.0.0.2"},
	)
	linkRepo := newMockLinkRepo()
	cache := NewDeviceLinkCache(deviceRepo, linkRepo, make(chan struct{}, 1))

	if _, err := cache.GetDevices(); err != nil {
		t.Fatalf("GetDevices initial: %v", err)
	}

	stop := make(chan struct{})
	errCh := make(chan error, 8)
	var readers sync.WaitGroup

	for i := 0; i < 4; i++ {
		readers.Add(1)
		go func() {
			defer readers.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}

				if _, err := cache.GetDevices(); err != nil {
					errCh <- err
					return
				}
				if _, err := cache.GetLinks(); err != nil {
					errCh <- err
					return
				}
			}
		}()
	}

	for i := 0; i < 20; i++ {
		device := domain.Device{
			ID:       uuid.New(),
			Hostname: fmt.Sprintf("device-%02d", i),
			IP:       fmt.Sprintf("10.0.1.%d", i+1),
		}
		deviceRepo.upsertDevice(device, domain.ChangeKindCreated)
		linkRepo.upsertLink(domain.Link{
			ID:                uuid.New(),
			SourceDeviceID:    sourceID,
			SourceIfName:      fmt.Sprintf("ether-%02d", i),
			TargetDeviceID:    targetID,
			TargetIfName:      fmt.Sprintf("ether-%02d", i),
			DiscoveryProtocol: domain.DiscoveryProtocolLLDP,
			CreatedAt:         time.Unix(int64(i+1), 0).UTC(),
		}, domain.ChangeKindCreated)
	}

	close(stop)
	readers.Wait()
	close(errCh)
	for err := range errCh {
		if err != nil {
			t.Fatalf("concurrent read failed: %v", err)
		}
	}

	devices, err := cache.GetDevices()
	if err != nil {
		t.Fatalf("GetDevices final: %v", err)
	}
	if len(devices) != 22 {
		t.Fatalf("len(devices) = %d, want 22", len(devices))
	}

	links, err := cache.GetLinks()
	if err != nil {
		t.Fatalf("GetLinks final: %v", err)
	}
	if len(links) != 20 {
		t.Fatalf("len(links) = %d, want 20", len(links))
	}
}

func TestCacheSQLiteRepos_StartupFullReloadAndMidRunIncrementalUpdates(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:?_foreign_keys=on")
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	if err := sqliterepo.RunMigrations(db); err != nil {
		t.Fatalf("RunMigrations: %v", err)
	}

	invalidateCh := make(chan struct{}, 1)
	deviceRepo := sqliterepo.NewDeviceRepo(db, testEncryptionKey, invalidateCh)
	linkRepo := sqliterepo.NewLinkRepo(db, invalidateCh)
	cache := NewDeviceLinkCache(deviceRepo, linkRepo, invalidateCh)

	first := &domain.Device{
		Hostname: "alpha",
		IP:       "10.0.0.1",
		SNMPCredentials: domain.SNMPCredentials{
			Version: domain.SNMPVersionV2c,
			V2c:     &domain.SNMPv2cCredentials{Community: "public"},
		},
		DeviceType: domain.DeviceTypeRouter,
		Status:     domain.DeviceStatusUp,
		Managed:    true,
		SysName:    "alpha.example.com.",
	}
	second := &domain.Device{
		Hostname: "beta",
		IP:       "10.0.0.2",
		SNMPCredentials: domain.SNMPCredentials{
			Version: domain.SNMPVersionV2c,
			V2c:     &domain.SNMPv2cCredentials{Community: "public"},
		},
		DeviceType: domain.DeviceTypeSwitch,
		Status:     domain.DeviceStatusUp,
		Managed:    true,
		SysName:    "beta.example.com.",
	}

	if err := deviceRepo.Create(first); err != nil {
		t.Fatalf("Create first device: %v", err)
	}
	if err := deviceRepo.Create(second); err != nil {
		t.Fatalf("Create second device: %v", err)
	}

	devices, err := cache.GetDevices()
	if err != nil {
		t.Fatalf("GetDevices initial: %v", err)
	}
	if len(devices) != 2 {
		t.Fatalf("len(devices) = %d, want 2", len(devices))
	}

	link := &domain.Link{
		SourceDeviceID:    first.ID,
		SourceIfName:      "ether1",
		TargetDeviceID:    second.ID,
		TargetIfName:      "ether2",
		DiscoveryProtocol: domain.DiscoveryProtocolLLDP,
	}
	if err := linkRepo.Create(link); err != nil {
		t.Fatalf("Create link: %v", err)
	}

	links, err := cache.GetLinks()
	if err != nil {
		t.Fatalf("GetLinks after repo create: %v", err)
	}
	if len(links) != 1 {
		t.Fatalf("len(links) = %d, want 1", len(links))
	}

	resolved, ok, err := cache.GetDeviceBySysName("BETA")
	if err != nil {
		t.Fatalf("GetDeviceBySysName: %v", err)
	}
	if !ok || resolved.ID != second.ID {
		t.Fatalf("GetDeviceBySysName returned %#v, %v; want %s", resolved, ok, second.ID)
	}
}
