package cache

import (
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/domain"
)

// --- Mock Device Repository ---

type mockDeviceRepo struct {
	getAllCount int32
	devices    []domain.Device
}

func (r *mockDeviceRepo) Create(_ *domain.Device) error              { return nil }
func (r *mockDeviceRepo) GetByID(_ uuid.UUID) (*domain.Device, error) { return nil, nil }
func (r *mockDeviceRepo) GetByIP(_ string) (*domain.Device, error)    { return nil, nil }
func (r *mockDeviceRepo) GetBySysName(_ string) (*domain.Device, error) { return nil, nil }
func (r *mockDeviceRepo) Update(_ *domain.Device) error              { return nil }
func (r *mockDeviceRepo) Delete(_ uuid.UUID) error                   { return nil }

func (r *mockDeviceRepo) GetAll() ([]domain.Device, error) {
	atomic.AddInt32(&r.getAllCount, 1)
	result := make([]domain.Device, len(r.devices))
	copy(result, r.devices)
	return result, nil
}

// --- Mock Link Repository ---

type mockLinkRepo struct {
	getAllCount int32
	links      []domain.Link
}

func (r *mockLinkRepo) Create(_ *domain.Link) error                         { return nil }
func (r *mockLinkRepo) GetByID(_ uuid.UUID) (*domain.Link, error)           { return nil, nil }
func (r *mockLinkRepo) GetByDeviceID(_ uuid.UUID) ([]domain.Link, error)    { return nil, nil }
func (r *mockLinkRepo) Update(_ *domain.Link) error                         { return nil }
func (r *mockLinkRepo) Delete(_ uuid.UUID) error                            { return nil }
func (r *mockLinkRepo) Upsert(_ *domain.Link) (bool, error) { return false, nil }

func (r *mockLinkRepo) GetAll() ([]domain.Link, error) {
	atomic.AddInt32(&r.getAllCount, 1)
	result := make([]domain.Link, len(r.links))
	copy(result, r.links)
	return result, nil
}

// --- Tests ---

func TestCacheLazyLoad(t *testing.T) {
	deviceRepo := &mockDeviceRepo{
		devices: []domain.Device{
			{Hostname: "dev1", IP: "10.0.0.1"},
			{Hostname: "dev2", IP: "10.0.0.2"},
		},
	}
	linkRepo := &mockLinkRepo{}
	ch := make(chan struct{}, 1)

	c := NewDeviceLinkCache(deviceRepo, linkRepo, ch)

	devices, err := c.GetDevices()
	if err != nil {
		t.Fatalf("GetDevices: %v", err)
	}

	if len(devices) != 2 {
		t.Errorf("expected 2 devices, got %d", len(devices))
	}

	count := atomic.LoadInt32(&deviceRepo.getAllCount)
	if count != 1 {
		t.Errorf("expected repo.GetAll called 1 time, got %d", count)
	}
}

func TestCacheReturnsCachedData(t *testing.T) {
	deviceRepo := &mockDeviceRepo{
		devices: []domain.Device{
			{Hostname: "dev1", IP: "10.0.0.1"},
			{Hostname: "dev2", IP: "10.0.0.2"},
		},
	}
	linkRepo := &mockLinkRepo{}
	ch := make(chan struct{}, 1)

	c := NewDeviceLinkCache(deviceRepo, linkRepo, ch)

	// First call
	_, _ = c.GetDevices()
	// Second call
	devices, err := c.GetDevices()
	if err != nil {
		t.Fatalf("GetDevices: %v", err)
	}

	if len(devices) != 2 {
		t.Errorf("expected 2 devices, got %d", len(devices))
	}

	count := atomic.LoadInt32(&deviceRepo.getAllCount)
	if count != 1 {
		t.Errorf("expected repo.GetAll called only 1 time total, got %d", count)
	}
}

func TestCacheInvalidation(t *testing.T) {
	deviceRepo := &mockDeviceRepo{
		devices: []domain.Device{
			{Hostname: "dev1", IP: "10.0.0.1"},
		},
	}
	linkRepo := &mockLinkRepo{}
	ch := make(chan struct{}, 1)

	c := NewDeviceLinkCache(deviceRepo, linkRepo, ch)

	// First call -- loads from repo
	_, _ = c.GetDevices()
	if atomic.LoadInt32(&deviceRepo.getAllCount) != 1 {
		t.Fatal("expected 1 GetAll call after first read")
	}

	// Send invalidation signal
	ch <- struct{}{}

	// Next call should refetch
	_, _ = c.GetDevices()
	count := atomic.LoadInt32(&deviceRepo.getAllCount)
	if count != 2 {
		t.Errorf("expected repo.GetAll called 2 times after invalidation, got %d", count)
	}
}

func TestCacheLinks(t *testing.T) {
	deviceRepo := &mockDeviceRepo{}
	linkRepo := &mockLinkRepo{
		links: []domain.Link{
			{SourceIfName: "ether1", TargetIfName: "ether2"},
		},
	}
	ch := make(chan struct{}, 1)

	c := NewDeviceLinkCache(deviceRepo, linkRepo, ch)

	links, err := c.GetLinks()
	if err != nil {
		t.Fatalf("GetLinks: %v", err)
	}

	if len(links) != 1 {
		t.Errorf("expected 1 link, got %d", len(links))
	}

	count := atomic.LoadInt32(&linkRepo.getAllCount)
	if count != 1 {
		t.Errorf("expected linkRepo.GetAll called 1 time, got %d", count)
	}

	// Second call should use cache
	_, _ = c.GetLinks()
	count = atomic.LoadInt32(&linkRepo.getAllCount)
	if count != 1 {
		t.Errorf("expected linkRepo.GetAll still 1 after second call, got %d", count)
	}
}

func TestNonBlockingNotify(t *testing.T) {
	// Create channel with buffer 1
	ch := make(chan struct{}, 1)

	// Fill the buffer
	ch <- struct{}{}

	// Second send should not block (uses select/default pattern)
	// This tests the pattern used in repo notify() method
	select {
	case ch <- struct{}{}:
		// channel accepted, good
	default:
		// channel full, also good -- this is the expected non-blocking path
	}

	// If we reach here without blocking, the test passes
}
