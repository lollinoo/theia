package worker

import (
	"context"
	"log"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/lollinoo/theia/internal/cache"
	"github.com/lollinoo/theia/internal/domain"
	"github.com/lollinoo/theia/internal/service"
)

// Poller runs a background goroutine that re-probes all managed devices
// at a configurable interval read from the settings repository.
type Poller struct {
	deviceService *service.DeviceService
	settingsRepo  domain.SettingsRepository
	cache         *cache.DeviceLinkCache

	running atomic.Bool
	cancel  context.CancelFunc
	done    chan struct{}
}

// NewPoller creates a new Poller.
func NewPoller(deviceService *service.DeviceService, settingsRepo domain.SettingsRepository, cache *cache.DeviceLinkCache) *Poller {
	return &Poller{
		deviceService: deviceService,
		settingsRepo:  settingsRepo,
		cache:         cache,
		done:          make(chan struct{}),
	}
}

// Start begins the background polling loop. It reads the polling interval
// from settings each cycle, so changes take effect without restart.
func (p *Poller) Start(ctx context.Context) {
	pollCtx, cancel := context.WithCancel(ctx)
	p.cancel = cancel
	p.running.Store(true)

	go func() {
		defer close(p.done)
		defer p.running.Store(false)

		for {
			interval := GetPollingInterval(p.settingsRepo)

			select {
			case <-pollCtx.Done():
				log.Println("Poller shutting down")
				return
			case <-time.After(interval):
				p.pollAllDevices(pollCtx)
			}
		}
	}()

	log.Printf("Poller started")
}

// Stop gracefully stops the poller and waits for it to finish.
func (p *Poller) Stop() {
	if p.cancel != nil {
		p.cancel()
		<-p.done
	}
	log.Println("Poller stopped")
}

// Status returns "running" or "stopped".
func (p *Poller) Status() string {
	if p.running.Load() {
		return "running"
	}
	return "stopped"
}

// pollAllDevices retrieves all managed devices and re-probes each using
// a configurable worker pool (semaphore pattern).
func (p *Poller) pollAllDevices(ctx context.Context) {
	devices, err := p.cache.GetDevices()
	if err != nil {
		log.Printf("Poller: failed to get devices: %v", err)
		return
	}

	poolSize := p.getWorkerPoolSize()
	sem := make(chan struct{}, poolSize)
	var wg sync.WaitGroup

	pingTimeout := p.getPingTimeout()

	for i := range devices {
		if !devices[i].Managed {
			continue
		}

		dev := devices[i]

		// Per VIRT-05: Virtual devices are never SNMP re-probed.
		// Instead, virtual devices with an IP get a lightweight TCP
		// reachability check so their status updates to up/down.
		if dev.DeviceType == domain.DeviceTypeVirtual {
			if dev.IP == "" {
				continue // no IP — nothing to ping
			}
			deviceID := dev.ID
			wg.Add(1)
			sem <- struct{}{}
			go func() {
				defer wg.Done()
				defer func() { <-sem }()
				if err := p.deviceService.PingVirtualDevice(ctx, deviceID, pingTimeout); err != nil {
					log.Printf("Poller: failed to ping virtual device %s: %v", deviceID, err)
				}
			}()
			continue
		}

		deviceID := dev.ID

		wg.Add(1)
		sem <- struct{}{} // Acquire semaphore slot

		go func() {
			defer wg.Done()
			defer func() { <-sem }() // Release slot

			if err := p.deviceService.ReprobeDevice(ctx, deviceID); err != nil {
				log.Printf("Poller: failed to reprobe device %s: %v", deviceID, err)
			}
		}()
	}

	wg.Wait()
	// Wait for all async probes to complete
	p.deviceService.WaitForProbes()
}

// getWorkerPoolSize reads the worker pool size from settings.
func (p *Poller) getWorkerPoolSize() int {
	val, err := p.settingsRepo.Get(domain.SettingSNMPWorkerPoolSize)
	if err != nil {
		return 5 // default
	}
	size, err := strconv.Atoi(val)
	if err != nil || size <= 0 {
		return 5
	}
	return size
}

// getPingTimeout returns the timeout for TCP reachability checks on virtual
// devices. It reuses the SNMP timeout setting (default 10s) since both are
// network-level reachability probes.
func (p *Poller) getPingTimeout() time.Duration {
	val, err := p.settingsRepo.Get(domain.SettingSNMPTimeout)
	if err != nil {
		return 5 * time.Second // default: 5s for TCP ping
	}
	secs, err := strconv.Atoi(val)
	if err != nil || secs <= 0 {
		return 5 * time.Second
	}
	return time.Duration(secs) * time.Second
}
