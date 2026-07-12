package settingscache

import (
	"fmt"
	"sync"
	"time"

	"github.com/lollinoo/theia/internal/domain"
)

const defaultTTL = 5 * time.Second

// Cache wraps a settings repository with a short-lived in-memory snapshot.
type Cache struct {
	repo domain.SettingsRepository
	ttl  time.Duration
	now  func() time.Time

	mu       sync.RWMutex
	values   map[string]string
	loadedAt time.Time
}

// New creates a settings cache. Non-positive TTL values use the default TTL.
func New(repo domain.SettingsRepository, ttl time.Duration) *Cache {
	if ttl <= 0 {
		ttl = defaultTTL
	}

	return &Cache{
		repo: repo,
		ttl:  ttl,
		now:  time.Now,
	}
}

// Get retrieves a setting from the cached snapshot, refreshing when needed.
func (c *Cache) Get(key string) (string, error) {
	if err := c.ensureFresh(); err != nil {
		return "", err
	}

	c.mu.RLock()
	defer c.mu.RUnlock()

	value, ok := c.values[key]
	if !ok {
		return "", fmt.Errorf("%w: %s", domain.ErrSettingNotFound, key)
	}
	return value, nil
}

// Set writes a setting through to the repository and updates the cached value.
func (c *Cache) Set(key, value string) error {
	if err := c.repo.Set(key, value); err != nil {
		return err
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.values == nil {
		return nil
	}
	c.values[key] = value
	return nil
}

// GetAll returns a clone of the cached settings snapshot, refreshing when needed.
func (c *Cache) GetAll() (map[string]string, error) {
	if err := c.ensureFresh(); err != nil {
		return nil, err
	}

	c.mu.RLock()
	defer c.mu.RUnlock()

	return clone(c.values), nil
}

func (c *Cache) ensureFresh() error {
	now := c.clockNow()

	c.mu.RLock()
	fresh := c.values != nil && now.Sub(c.loadedAt) < c.ttl
	c.mu.RUnlock()
	if fresh {
		return nil
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	now = c.clockNow()
	if c.values != nil && now.Sub(c.loadedAt) < c.ttl {
		return nil
	}

	values, err := c.repo.GetAll()
	if err != nil {
		return err
	}
	c.values = clone(values)
	c.loadedAt = now
	return nil
}

func (c *Cache) clockNow() time.Time {
	if c.now != nil {
		return c.now()
	}
	return time.Now()
}

func clone(values map[string]string) map[string]string {
	if values == nil {
		return nil
	}

	cloned := make(map[string]string, len(values))
	for key, value := range values {
		cloned[key] = value
	}
	return cloned
}
