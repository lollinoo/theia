package settingscache

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

type fakeSettingsRepo struct {
	mu          sync.Mutex
	values      map[string]string
	getAllCount int
	setCount    int
	updateCount int
}

type blockingSetSettingsRepo struct {
	*fakeSettingsRepo
	setStarted chan struct{}
	releaseSet chan struct{}
}

func (r *blockingSetSettingsRepo) Set(key, value string) error {
	close(r.setStarted)
	<-r.releaseSet
	return r.fakeSettingsRepo.Set(key, value)
}

func newFakeSettingsRepo(values map[string]string) *fakeSettingsRepo {
	return &fakeSettingsRepo{values: values}
}

func (r *fakeSettingsRepo) Get(key string) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	value, ok := r.values[key]
	if !ok {
		return "", fmt.Errorf("setting not found: %s", key)
	}
	return value, nil
}

func (r *fakeSettingsRepo) Set(key, value string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.values[key] = value
	r.setCount++
	return nil
}

func (r *fakeSettingsRepo) GetAll() (map[string]string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.getAllCount++
	values := make(map[string]string, len(r.values))
	for key, value := range r.values {
		values[key] = value
	}
	return values, nil
}

func (r *fakeSettingsRepo) Update(key string, update func(string) (string, error)) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	current, ok := r.values[key]
	if !ok {
		return "", fmt.Errorf("setting not found: %s", key)
	}
	next, err := update(current)
	if err != nil {
		return "", err
	}
	r.values[key] = next
	r.updateCount++
	return next, nil
}

func (r *fakeSettingsRepo) mutate(key, value string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.values[key] = value
}

func (r *fakeSettingsRepo) counts() (int, int) {
	r.mu.Lock()
	defer r.mu.Unlock()

	return r.getAllCount, r.setCount
}

func TestCacheGetUsesSnapshot(t *testing.T) {
	repo := newFakeSettingsRepo(map[string]string{"device.poll_rate_seconds": "30"})
	cache := New(repo, time.Minute)

	value, err := cache.Get("device.poll_rate_seconds")
	if err != nil {
		t.Fatalf("Get returned error: %v", err)
	}
	if value != "30" {
		t.Fatalf("Get returned %q, want %q", value, "30")
	}
	getAllCount, _ := repo.counts()
	if getAllCount != 1 {
		t.Fatalf("GetAll count = %d, want 1", getAllCount)
	}

	repo.mutate("device.poll_rate_seconds", "45")

	value, err = cache.Get("device.poll_rate_seconds")
	if err != nil {
		t.Fatalf("Get returned error: %v", err)
	}
	if value != "30" {
		t.Fatalf("Get returned %q, want cached %q", value, "30")
	}
	getAllCount, _ = repo.counts()
	if getAllCount != 1 {
		t.Fatalf("GetAll count = %d, want 1", getAllCount)
	}
}

func TestCacheSetWritesThroughAndRefreshesValue(t *testing.T) {
	repo := newFakeSettingsRepo(map[string]string{"device.poll_rate_seconds": "30"})
	cache := New(repo, time.Minute)

	if _, err := cache.Get("device.poll_rate_seconds"); err != nil {
		t.Fatalf("Get returned error: %v", err)
	}
	if err := cache.Set("device.poll_rate_seconds", "45"); err != nil {
		t.Fatalf("Set returned error: %v", err)
	}
	getAllCount, setCount := repo.counts()
	if getAllCount != 1 {
		t.Fatalf("GetAll count = %d, want 1", getAllCount)
	}
	if setCount != 1 {
		t.Fatalf("Set count = %d, want 1", setCount)
	}

	value, err := cache.Get("device.poll_rate_seconds")
	if err != nil {
		t.Fatalf("Get returned error: %v", err)
	}
	if value != "45" {
		t.Fatalf("Get returned %q, want %q", value, "45")
	}
	getAllCount, _ = repo.counts()
	if getAllCount != 1 {
		t.Fatalf("GetAll count = %d, want 1", getAllCount)
	}
}

func TestCacheSetCoordinatesWithAtomicUpdates(t *testing.T) {
	repo := &blockingSetSettingsRepo{
		fakeSettingsRepo: newFakeSettingsRepo(map[string]string{"setting": "initial"}),
		setStarted:       make(chan struct{}),
		releaseSet:       make(chan struct{}),
	}
	cache := New(repo, time.Minute)
	setDone := make(chan error, 1)
	go func() {
		setDone <- cache.Set("setting", "set")
	}()
	<-repo.setStarted

	mutationLockWasFree := cache.updateMu.TryLock()
	if mutationLockWasFree {
		cache.updateMu.Unlock()
	}
	close(repo.releaseSet)
	if err := <-setDone; err != nil {
		t.Fatalf("Set returned error: %v", err)
	}
	if mutationLockWasFree {
		t.Fatal("Set did not coordinate with atomic updates")
	}
}

func TestCacheUpdateWritesThroughAndRefreshesValue(t *testing.T) {
	repo := newFakeSettingsRepo(map[string]string{"device.poll_rate_seconds": "30"})
	cache := New(repo, time.Minute)

	if _, err := cache.Get("device.poll_rate_seconds"); err != nil {
		t.Fatalf("Get returned error: %v", err)
	}
	updated, err := cache.Update("device.poll_rate_seconds", func(current string) (string, error) {
		if current != "30" {
			t.Fatalf("update received %q, want %q", current, "30")
		}
		return "45", nil
	})
	if err != nil {
		t.Fatalf("Update returned error: %v", err)
	}
	if updated != "45" {
		t.Fatalf("Update returned %q, want %q", updated, "45")
	}
	if repo.updateCount != 1 {
		t.Fatalf("repository Update count = %d, want 1", repo.updateCount)
	}

	value, err := cache.Get("device.poll_rate_seconds")
	if err != nil {
		t.Fatalf("Get returned error: %v", err)
	}
	if value != "45" {
		t.Fatalf("Get returned %q, want updated %q", value, "45")
	}
}

func TestCacheColdSetDoesNotMarkPartialSnapshotLoaded(t *testing.T) {
	repo := newFakeSettingsRepo(map[string]string{
		"device.poll_rate_seconds": "30",
		"bridge.port":              "1337",
	})
	cache := New(repo, time.Minute)

	if err := cache.Set("device.poll_rate_seconds", "45"); err != nil {
		t.Fatalf("Set returned error: %v", err)
	}
	getAllCount, setCount := repo.counts()
	if getAllCount != 0 {
		t.Fatalf("GetAll count = %d, want 0 before first read", getAllCount)
	}
	if setCount != 1 {
		t.Fatalf("Set count = %d, want 1", setCount)
	}

	value, err := cache.Get("bridge.port")
	if err != nil {
		t.Fatalf("Get returned error: %v", err)
	}
	if value != "1337" {
		t.Fatalf("Get returned %q, want %q", value, "1337")
	}
	getAllCount, _ = repo.counts()
	if getAllCount != 1 {
		t.Fatalf("GetAll count = %d, want 1", getAllCount)
	}
}

func TestCacheExpiredSetDoesNotMarkStaleSnapshotFresh(t *testing.T) {
	repo := newFakeSettingsRepo(map[string]string{
		"device.poll_rate_seconds": "30",
		"bridge.port":              "1337",
	})
	cache := New(repo, time.Minute)
	now := time.Date(2026, 6, 11, 12, 0, 0, 0, time.UTC)
	cache.now = func() time.Time {
		return now
	}

	if _, err := cache.Get("device.poll_rate_seconds"); err != nil {
		t.Fatalf("Get returned error: %v", err)
	}

	now = now.Add(2 * time.Minute)
	repo.mutate("bridge.port", "1443")
	if err := cache.Set("device.poll_rate_seconds", "45"); err != nil {
		t.Fatalf("Set returned error: %v", err)
	}

	value, err := cache.Get("bridge.port")
	if err != nil {
		t.Fatalf("Get returned error: %v", err)
	}
	if value != "1443" {
		t.Fatalf("Get returned %q, want reloaded %q", value, "1443")
	}
	getAllCount, _ := repo.counts()
	if getAllCount != 2 {
		t.Fatalf("GetAll count = %d, want 2", getAllCount)
	}
}

func TestCacheReloadsAfterTTL(t *testing.T) {
	repo := newFakeSettingsRepo(map[string]string{"device.poll_rate_seconds": "30"})
	cache := New(repo, time.Millisecond)

	value, err := cache.Get("device.poll_rate_seconds")
	if err != nil {
		t.Fatalf("Get returned error: %v", err)
	}
	if value != "30" {
		t.Fatalf("Get returned %q, want %q", value, "30")
	}

	repo.mutate("device.poll_rate_seconds", "45")
	time.Sleep(5 * time.Millisecond)

	value, err = cache.Get("device.poll_rate_seconds")
	if err != nil {
		t.Fatalf("Get returned error: %v", err)
	}
	if value != "45" {
		t.Fatalf("Get returned %q, want %q", value, "45")
	}
	getAllCount, _ := repo.counts()
	if getAllCount != 2 {
		t.Fatalf("GetAll count = %d, want 2", getAllCount)
	}
}
