package cache

import (
	"sync"

	"github.com/lollinoo/theia/internal/domain"
	"github.com/lollinoo/theia/internal/observability"
)

// DeviceLinkCache provides a lazily-loaded, invalidation-driven cache for
// devices and links. The cache is populated on first read and refreshed only
// when a write-event signal arrives on the invalidation channel.
type DeviceLinkCache struct {
	deviceRepo   domain.DeviceRepository
	linkRepo     domain.LinkRepository
	mu           sync.Mutex
	devices      []domain.Device
	links        []domain.Link
	valid        bool
	invalidateCh <-chan struct{}
}

// NewDeviceLinkCache creates a new DeviceLinkCache that reads from the given
// repos and listens for invalidation signals on invalidateCh.
func NewDeviceLinkCache(deviceRepo domain.DeviceRepository, linkRepo domain.LinkRepository, invalidateCh <-chan struct{}) *DeviceLinkCache {
	return &DeviceLinkCache{
		deviceRepo:   deviceRepo,
		linkRepo:     linkRepo,
		valid:        false,
		invalidateCh: invalidateCh,
	}
}

// GetDevices returns cached devices, reloading from the database only if the
// cache has been invalidated or has never been loaded.
func (c *DeviceLinkCache) GetDevices() ([]domain.Device, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.drainInvalidations()

	if !c.valid {
		if err := c.reload(); err != nil {
			return nil, err
		}
	}

	return c.devices, nil
}

// GetLinks returns cached links, reloading from the database only if the
// cache has been invalidated or has never been loaded.
func (c *DeviceLinkCache) GetLinks() ([]domain.Link, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.drainInvalidations()

	if !c.valid {
		if err := c.reload(); err != nil {
			return nil, err
		}
	}

	return c.links, nil
}

// drainInvalidations reads all pending signals from the invalidation channel,
// marking the cache invalid for each. Must be called with c.mu held.
func (c *DeviceLinkCache) drainInvalidations() {
	for {
		select {
		case <-c.invalidateCh:
			c.valid = false
		default:
			return
		}
	}
}

// reload fetches all devices and links from the underlying repositories.
// Must be called with c.mu held.
func (c *DeviceLinkCache) reload() error {
	devices, err := c.deviceRepo.GetAll()
	if err != nil {
		return err
	}

	links, err := c.linkRepo.GetAll()
	if err != nil {
		return err
	}

	c.devices = devices
	c.links = links
	c.valid = true
	observability.Default().IncCacheReload()

	return nil
}
