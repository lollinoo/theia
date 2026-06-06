package cache

// This file defines cache cache behavior and expiry assumptions.

import (
	"sort"
	"strings"
	"sync"

	"github.com/google/uuid"

	"github.com/lollinoo/theia/internal/domain"
	"github.com/lollinoo/theia/internal/observability"
)

type deviceChangeSource interface {
	DeviceChanges() <-chan domain.DeviceChangeEvent
	DrainDeviceRepair() bool
}

type deviceChangeSubscriber interface {
	SubscribeDeviceChanges(buffer int) <-chan domain.DeviceChangeEvent
	DrainDeviceRepair() bool
}

type linkChangeSource interface {
	LinkChanges() <-chan domain.LinkChangeEvent
	DrainLinkRepair() bool
}

type linkChangeSubscriber interface {
	SubscribeLinkChanges(buffer int) <-chan domain.LinkChangeEvent
	DrainLinkRepair() bool
}

type linkEndpointKey struct {
	sourceDeviceID uuid.UUID
	sourceIfName   string
	targetDeviceID uuid.UUID
	targetIfName   string
}

// DeviceLinkCache keeps devices and links resident in memory. Startup and
// repair still use a full DB reload; steady-state writes are applied from
// incremental repo change events.
type DeviceLinkCache struct {
	deviceRepo domain.DeviceRepository
	linkRepo   domain.LinkRepository

	mu sync.Mutex

	devicesByID      map[uuid.UUID]domain.Device
	devicesBySysName map[string]uuid.UUID
	linksByID        map[uuid.UUID]domain.Link
	linksByEndpoint  map[linkEndpointKey]uuid.UUID

	devices []domain.Device
	links   []domain.Link

	devicesDirty    bool
	linksDirty      bool
	loaded          bool
	needsFullReload bool

	invalidateCh   <-chan struct{}
	deviceChanges  <-chan domain.DeviceChangeEvent
	linkChanges    <-chan domain.LinkChangeEvent
	deviceRepair   func() bool
	linkRepair     func() bool
	useIncremental bool
}

// NewDeviceLinkCache creates a new cache that prefers incremental repo events
// when supported and falls back to legacy invalidation otherwise.
func NewDeviceLinkCache(deviceRepo domain.DeviceRepository, linkRepo domain.LinkRepository, invalidateCh <-chan struct{}) *DeviceLinkCache {
	cache := &DeviceLinkCache{
		deviceRepo:       deviceRepo,
		linkRepo:         linkRepo,
		invalidateCh:     invalidateCh,
		devicesByID:      make(map[uuid.UUID]domain.Device),
		devicesBySysName: make(map[string]uuid.UUID),
		linksByID:        make(map[uuid.UUID]domain.Link),
		linksByEndpoint:  make(map[linkEndpointKey]uuid.UUID),
		devicesDirty:     true,
		linksDirty:       true,
		needsFullReload:  true,
	}

	if source, ok := deviceRepo.(deviceChangeSubscriber); ok {
		cache.deviceChanges = source.SubscribeDeviceChanges(256)
		cache.deviceRepair = source.DrainDeviceRepair
		cache.useIncremental = true
	} else if source, ok := deviceRepo.(deviceChangeSource); ok {
		cache.deviceChanges = source.DeviceChanges()
		cache.deviceRepair = source.DrainDeviceRepair
		cache.useIncremental = true
	}
	if source, ok := linkRepo.(linkChangeSubscriber); ok {
		cache.linkChanges = source.SubscribeLinkChanges(256)
		cache.linkRepair = source.DrainLinkRepair
		cache.useIncremental = true
	} else if source, ok := linkRepo.(linkChangeSource); ok {
		cache.linkChanges = source.LinkChanges()
		cache.linkRepair = source.DrainLinkRepair
		cache.useIncremental = true
	}

	return cache
}

// GetDevices returns cached devices after applying any pending incremental
// updates. Full reload is used only on first load or when repair is required.
func (c *DeviceLinkCache) GetDevices() ([]domain.Device, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if err := c.prepareLocked(); err != nil {
		return nil, err
	}
	if c.devicesDirty {
		c.devices = buildSortedDevices(c.devicesByID)
		c.devicesDirty = false
	}

	return c.devices, nil
}

// GetLinks returns cached links after applying any pending incremental
// updates. Full reload is used only on first load or when repair is required.
func (c *DeviceLinkCache) GetLinks() ([]domain.Link, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if err := c.prepareLocked(); err != nil {
		return nil, err
	}
	if c.linksDirty {
		c.links = buildSortedLinks(c.linksByID)
		c.linksDirty = false
	}

	return c.links, nil
}

// GetDeviceByID retrieves device by id data from the package.
func (c *DeviceLinkCache) GetDeviceByID(id uuid.UUID) (domain.Device, bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if err := c.prepareLocked(); err != nil {
		return domain.Device{}, false, err
	}

	device, ok := c.devicesByID[id]
	return device, ok, nil
}

// GetDeviceBySysName retrieves device by sys name data from the package.
func (c *DeviceLinkCache) GetDeviceBySysName(sysName string) (domain.Device, bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if err := c.prepareLocked(); err != nil {
		return domain.Device{}, false, err
	}

	deviceID, ok := c.devicesBySysName[normalizeSysNameLookup(sysName)]
	if !ok {
		return domain.Device{}, false, nil
	}
	device, ok := c.devicesByID[deviceID]
	return device, ok, nil
}

// GetLinkByID retrieves link by id data from the package.
func (c *DeviceLinkCache) GetLinkByID(id uuid.UUID) (domain.Link, bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if err := c.prepareLocked(); err != nil {
		return domain.Link{}, false, err
	}

	link, ok := c.linksByID[id]
	return link, ok, nil
}

// GetLinkByEndpointPair retrieves link by endpoint pair data from the package.
func (c *DeviceLinkCache) GetLinkByEndpointPair(sourceDeviceID uuid.UUID, sourceIfName string, targetDeviceID uuid.UUID, targetIfName string) (domain.Link, bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if err := c.prepareLocked(); err != nil {
		return domain.Link{}, false, err
	}

	linkID, ok := c.linksByEndpoint[makeLinkEndpointKey(domain.Link{
		SourceDeviceID: sourceDeviceID,
		SourceIfName:   sourceIfName,
		TargetDeviceID: targetDeviceID,
		TargetIfName:   targetIfName,
	})]
	if !ok {
		return domain.Link{}, false, nil
	}
	link, ok := c.linksByID[linkID]
	return link, ok, nil
}

func (c *DeviceLinkCache) prepareLocked() error {
	if c.useIncremental {
		if c.deviceRepair != nil && c.deviceRepair() {
			c.needsFullReload = true
		}
		if c.linkRepair != nil && c.linkRepair() {
			c.needsFullReload = true
		}
	} else {
		c.drainInvalidations()
	}

	if !c.loaded || c.needsFullReload {
		if err := c.reloadLocked(); err != nil {
			return err
		}
		if c.useIncremental {
			return c.applyPendingIncrementalChangesLocked()
		}
		return nil
	}

	if !c.useIncremental {
		return nil
	}

	if err := c.applyPendingIncrementalChangesLocked(); err != nil {
		c.needsFullReload = true
		return c.reloadLocked()
	}

	return nil
}

func (c *DeviceLinkCache) drainInvalidations() {
	for {
		select {
		case <-c.invalidateCh:
			c.needsFullReload = true
		default:
			return
		}
	}
}

func (c *DeviceLinkCache) applyPendingIncrementalChangesLocked() error {
	for {
		applied := false

		select {
		case event := <-c.deviceChanges:
			if err := c.applyDeviceChangeLocked(event); err != nil {
				return err
			}
			applied = true
		default:
		}

		select {
		case event := <-c.linkChanges:
			if err := c.applyLinkChangeLocked(event); err != nil {
				return err
			}
			applied = true
		default:
		}

		if !applied {
			return nil
		}
	}
}

func (c *DeviceLinkCache) applyDeviceChangeLocked(event domain.DeviceChangeEvent) error {
	switch event.Kind {
	case domain.ChangeKindDeleted:
		c.deleteDeviceLocked(event.DeviceID)
		return nil
	case domain.ChangeKindCreated, domain.ChangeKindUpdated:
		device, err := c.deviceRepo.GetByID(event.DeviceID)
		if err != nil {
			return err
		}
		c.upsertDeviceLocked(*device)
		return nil
	default:
		return nil
	}
}

func (c *DeviceLinkCache) applyLinkChangeLocked(event domain.LinkChangeEvent) error {
	switch event.Kind {
	case domain.ChangeKindDeleted:
		c.deleteLinkLocked(event.LinkID)
		return nil
	case domain.ChangeKindCreated, domain.ChangeKindUpdated:
		link, err := c.linkRepo.GetByID(event.LinkID)
		if err != nil {
			return err
		}
		c.upsertLinkLocked(*link)
		return nil
	default:
		return nil
	}
}

func (c *DeviceLinkCache) reloadLocked() error {
	devices, err := c.deviceRepo.GetAll()
	if err != nil {
		return err
	}

	links, err := c.linkRepo.GetAll()
	if err != nil {
		return err
	}

	c.devicesByID = make(map[uuid.UUID]domain.Device, len(devices))
	c.devicesBySysName = make(map[string]uuid.UUID, len(devices))
	for _, device := range devices {
		c.devicesByID[device.ID] = device
		if normalized := normalizeSysNameLookup(device.SysName); normalized != "" {
			c.devicesBySysName[normalized] = device.ID
		}
	}

	c.linksByID = make(map[uuid.UUID]domain.Link, len(links))
	c.linksByEndpoint = make(map[linkEndpointKey]uuid.UUID, len(links))
	for _, link := range links {
		c.linksByID[link.ID] = link
		c.linksByEndpoint[makeLinkEndpointKey(link)] = link.ID
	}

	c.devices = devices
	c.links = links
	c.devicesDirty = false
	c.linksDirty = false
	c.loaded = true
	c.needsFullReload = false
	observability.Default().IncCacheReload()

	return nil
}

func (c *DeviceLinkCache) upsertDeviceLocked(device domain.Device) {
	if existing, ok := c.devicesByID[device.ID]; ok {
		if normalized := normalizeSysNameLookup(existing.SysName); normalized != "" {
			delete(c.devicesBySysName, normalized)
		}
	}

	c.devicesByID[device.ID] = device
	if normalized := normalizeSysNameLookup(device.SysName); normalized != "" {
		c.devicesBySysName[normalized] = device.ID
	}
	c.devicesDirty = true
}

func (c *DeviceLinkCache) deleteDeviceLocked(deviceID uuid.UUID) {
	existing, ok := c.devicesByID[deviceID]
	if !ok {
		return
	}

	delete(c.devicesByID, deviceID)
	if normalized := normalizeSysNameLookup(existing.SysName); normalized != "" {
		delete(c.devicesBySysName, normalized)
	}
	c.devicesDirty = true
}

func (c *DeviceLinkCache) upsertLinkLocked(link domain.Link) {
	if existing, ok := c.linksByID[link.ID]; ok {
		delete(c.linksByEndpoint, makeLinkEndpointKey(existing))
	}

	c.linksByID[link.ID] = link
	c.linksByEndpoint[makeLinkEndpointKey(link)] = link.ID
	c.linksDirty = true
}

func (c *DeviceLinkCache) deleteLinkLocked(linkID uuid.UUID) {
	existing, ok := c.linksByID[linkID]
	if !ok {
		return
	}

	delete(c.linksByID, linkID)
	delete(c.linksByEndpoint, makeLinkEndpointKey(existing))
	c.linksDirty = true
}

func buildSortedDevices(devicesByID map[uuid.UUID]domain.Device) []domain.Device {
	devices := make([]domain.Device, 0, len(devicesByID))
	for _, device := range devicesByID {
		devices = append(devices, device)
	}

	sort.Slice(devices, func(i, j int) bool {
		if devices[i].Hostname != devices[j].Hostname {
			return devices[i].Hostname < devices[j].Hostname
		}
		return devices[i].ID.String() < devices[j].ID.String()
	})

	return devices
}

func buildSortedLinks(linksByID map[uuid.UUID]domain.Link) []domain.Link {
	links := make([]domain.Link, 0, len(linksByID))
	for _, link := range linksByID {
		links = append(links, link)
	}

	sort.Slice(links, func(i, j int) bool {
		if !links[i].CreatedAt.Equal(links[j].CreatedAt) {
			return links[i].CreatedAt.Before(links[j].CreatedAt)
		}
		return links[i].ID.String() < links[j].ID.String()
	})

	return links
}

func makeLinkEndpointKey(link domain.Link) linkEndpointKey {
	return linkEndpointKey{
		sourceDeviceID: link.SourceDeviceID,
		sourceIfName:   normalizeInterfaceName(link.SourceIfName),
		targetDeviceID: link.TargetDeviceID,
		targetIfName:   normalizeInterfaceName(link.TargetIfName),
	}
}

func normalizeSysNameLookup(sysName string) string {
	normalized := strings.ToLower(strings.TrimSpace(sysName))
	normalized = strings.TrimSuffix(normalized, ".")
	if idx := strings.Index(normalized, "."); idx >= 0 {
		normalized = normalized[:idx]
	}
	return normalized
}

func normalizeInterfaceName(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}
