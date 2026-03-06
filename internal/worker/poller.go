package worker

import (
	"context"
	"log"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/azmin/mikrotik-theia/internal/domain"
	"github.com/azmin/mikrotik-theia/internal/service"
)

// Poller runs a background goroutine that re-probes all managed devices
// at a configurable interval read from the settings repository.
type Poller struct {
	deviceService *service.DeviceService
	settingsRepo  domain.SettingsRepository

	running atomic.Bool
	cancel  context.CancelFunc
	done    chan struct{}
}

// NewPoller creates a new Poller.
func NewPoller(deviceService *service.DeviceService, settingsRepo domain.SettingsRepository) *Poller {
	return &Poller{
		deviceService: deviceService,
		settingsRepo:  settingsRepo,
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
			interval := p.getPollingInterval()

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
	devices, err := p.deviceService.GetAllDevices(ctx)
	if err != nil {
		log.Printf("Poller: failed to get devices: %v", err)
		return
	}

	poolSize := p.getWorkerPoolSize()
	sem := make(chan struct{}, poolSize)
	var wg sync.WaitGroup

	for i := range devices {
		if !devices[i].Managed {
			continue
		}

		deviceID := devices[i].ID

		wg.Add(1)
		sem <- struct{}{} // Acquire semaphore slot

		go func() {
			defer wg.Done()
			defer func() { <-sem }() // Release slot

			if err := p.deviceService.ProbeDevice(ctx, deviceID); err != nil {
				log.Printf("Poller: failed to probe device %s: %v", deviceID, err)
			}
		}()
	}

	wg.Wait()
	// Wait for all async probes to complete
	p.deviceService.WaitForProbes()
}

// getPollingInterval reads the polling interval from settings.
func (p *Poller) getPollingInterval() time.Duration {
	val, err := p.settingsRepo.Get(domain.SettingPollingInterval)
	if err != nil {
		return 60 * time.Second // default
	}
	seconds, err := strconv.Atoi(val)
	if err != nil || seconds <= 0 {
		return 60 * time.Second
	}
	return time.Duration(seconds) * time.Second
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
