package cache

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/domain"
)

type refreshDeviceRepo struct {
	domain.DeviceRepository
	mu                 sync.Mutex
	device             domain.Device
	changes            chan domain.DeviceChangeEvent
	entered, release   chan struct{}
	blockAll           bool
	allCalls, rowCalls int
	err                error
}

func (r *refreshDeviceRepo) GetAll() ([]domain.Device, error) {
	r.mu.Lock()
	r.allCalls++
	device, entered, release, block, err := r.device, r.entered, r.release, r.blockAll, r.err
	r.mu.Unlock()
	if block {
		close(entered)
		<-release
	}
	return []domain.Device{device}, err
}
func (r *refreshDeviceRepo) GetByID(uuid.UUID) (*domain.Device, error) {
	r.mu.Lock()
	r.rowCalls++
	device, entered, release, err := r.device, r.entered, r.release, r.err
	r.mu.Unlock()
	if entered != nil {
		close(entered)
		<-release
	}
	return &device, err
}
func (r *refreshDeviceRepo) DeviceChanges() <-chan domain.DeviceChangeEvent { return r.changes }
func (r *refreshDeviceRepo) DrainDeviceRepair() bool                        { return false }

type refreshLinkRepo struct{ domain.LinkRepository }

func (r *refreshLinkRepo) GetAll() ([]domain.Link, error) { return nil, nil }

func TestCacheSlowIncrementalReadDoesNotBlockSnapshotReaders(t *testing.T) {
	id := uuid.New()
	repo := &refreshDeviceRepo{device: domain.Device{ID: id, Hostname: "before"}, changes: make(chan domain.DeviceChangeEvent, 8)}
	cache := NewDeviceLinkCache(repo, &refreshLinkRepo{}, nil)
	if _, err := cache.GetDevices(); err != nil {
		t.Fatal(err)
	}
	repo.device.Hostname = "after"
	repo.entered = make(chan struct{})
	repo.release = make(chan struct{})
	for i := 0; i < 5; i++ {
		repo.changes <- domain.DeviceChangeEvent{Kind: domain.ChangeKindUpdated, DeviceID: id}
	}
	finished := make(chan error, 1)
	go func() { _, err := cache.GetDevices(); finished <- err }()
	<-repo.entered
	read := make(chan domain.Device, 1)
	go func() { device, _, _ := cache.GetDeviceByID(id); read <- device }()
	select {
	case device := <-read:
		if device.Hostname != "before" {
			t.Fatalf("partial snapshot published: %#v", device)
		}
	case <-time.After(time.Second):
		close(repo.release)
		t.Fatal("reader blocked by database I/O")
	}
	close(repo.release)
	if err := <-finished; err != nil {
		t.Fatal(err)
	}
	device, _, err := cache.GetDeviceByID(id)
	if err != nil || device.Hostname != "after" || repo.rowCalls != 1 {
		t.Fatalf("refresh device=%#v error=%v rowReads=%d", device, err, repo.rowCalls)
	}
}

func TestCacheColdLoadIsSharedAndDoesNotHoldMutexDuringIO(t *testing.T) {
	repo := &refreshDeviceRepo{device: domain.Device{ID: uuid.New()}, changes: make(chan domain.DeviceChangeEvent), blockAll: true, entered: make(chan struct{}), release: make(chan struct{})}
	cache := NewDeviceLinkCache(repo, &refreshLinkRepo{}, nil)
	finished := make(chan error, 3)
	for i := 0; i < 3; i++ {
		go func() { _, err := cache.GetDevices(); finished <- err }()
	}
	<-repo.entered
	unlocked := make(chan struct{})
	go func() { cache.mu.Lock(); cache.mu.Unlock(); close(unlocked) }()
	select {
	case <-unlocked:
	case <-time.After(time.Second):
		close(repo.release)
		t.Fatal("cold DB load holds cache mutex")
	}
	close(repo.release)
	for i := 0; i < 3; i++ {
		if err := <-finished; err != nil {
			t.Fatal(err)
		}
	}
	if repo.allCalls != 1 {
		t.Fatalf("cold database loads=%d, want 1", repo.allCalls)
	}
}

func TestCacheRefreshFailureKeepsSnapshotAndRetriesRepair(t *testing.T) {
	id := uuid.New()
	repo := &refreshDeviceRepo{device: domain.Device{ID: id, Hostname: "before"}, changes: make(chan domain.DeviceChangeEvent, 1)}
	cache := NewDeviceLinkCache(repo, &refreshLinkRepo{}, nil)
	if _, err := cache.GetDevices(); err != nil {
		t.Fatal(err)
	}
	repo.err = errors.New("database offline")
	repo.changes <- domain.DeviceChangeEvent{Kind: domain.ChangeKindUpdated, DeviceID: id}
	if _, err := cache.GetDevices(); !errors.Is(err, repo.err) {
		t.Fatalf("refresh failure=%v", err)
	}
	if cache.devicesByID[id].Hostname != "before" {
		t.Fatal("failed refresh replaced last good snapshot")
	}
	repo.err = nil
	repo.device.Hostname = "repaired"
	device, _, err := cache.GetDeviceByID(id)
	if err != nil || device.Hostname != "repaired" {
		t.Fatalf("repair=%#v %v", device, err)
	}
}

type panicRefreshRepo struct {
	*refreshDeviceRepo
	panicAll, panicRow bool
}

func (r *panicRefreshRepo) GetAll() ([]domain.Device, error) {
	if r.panicAll {
		panic("repository panic")
	}
	return r.refreshDeviceRepo.GetAll()
}

func (r *panicRefreshRepo) GetByID(id uuid.UUID) (*domain.Device, error) {
	if r.panicRow {
		panic("repository panic")
	}
	return r.refreshDeviceRepo.GetByID(id)
}

func TestCacheRefreshPanicPreservesMutexLifecycle(t *testing.T) {
	for _, stage := range []string{"cold load", "incremental update"} {
		t.Run(stage, func(t *testing.T) {
			id := uuid.New()
			repo := &panicRefreshRepo{refreshDeviceRepo: &refreshDeviceRepo{device: domain.Device{ID: id}, changes: make(chan domain.DeviceChangeEvent, 1)}}
			cache := NewDeviceLinkCache(repo, &refreshLinkRepo{}, nil)
			if stage == "cold load" {
				repo.panicAll = true
			} else {
				if _, err := cache.GetDevices(); err != nil {
					t.Fatal(err)
				}
				repo.panicRow = true
				repo.changes <- domain.DeviceChangeEvent{Kind: domain.ChangeKindUpdated, DeviceID: id}
			}
			func() {
				defer func() {
					if recovered := recover(); recovered != "repository panic" {
						t.Fatalf("panic=%v", recovered)
					}
				}()
				cache.GetDevices()
			}()
			if !cache.mu.TryLock() {
				t.Fatal("cache mutex left locked after panic")
			}
			cache.mu.Unlock()
			if cache.refreshing {
				t.Fatal("refresh marker left active after panic")
			}
			repo.panicAll, repo.panicRow = false, false
			if _, err := cache.GetDevices(); err != nil {
				t.Fatal(err)
			}
		})
	}
}
