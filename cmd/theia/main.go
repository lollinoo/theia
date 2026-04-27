package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/collector"
	"github.com/lollinoo/theia/internal/domain"
	"github.com/lollinoo/theia/internal/scheduler"
	"github.com/lollinoo/theia/internal/service"
	"github.com/lollinoo/theia/internal/snmp"
	"github.com/lollinoo/theia/internal/vendor"
	"github.com/lollinoo/theia/internal/worker"

	_ "github.com/jackc/pgx/v5/stdlib"
	_ "github.com/mattn/go-sqlite3"
)

type pendingRestoreCoordinator interface {
	ApplyPendingRestore() (bool, error)
}

type deviceRuntimeResetter interface {
	ResetDeviceRuntime(uuid.UUID)
}

var newRestoreCoordinator = func(dbPath, deviceBackupDir, knownHostsPath string) pendingRestoreCoordinator {
	return service.NewRestoreCoordinator(dbPath, deviceBackupDir, knownHostsPath)
}

var newCollectorSNMPClient = func(target string, creds domain.SNMPCredentials, timeout time.Duration, retries int) (collector.SNMPClient, error) {
	return snmp.NewClient(target, creds, timeout, retries)
}

func wirePollRescheduler(deviceService *service.DeviceService, sched *scheduler.Scheduler) {
	deviceService.SetPollRescheduler(sched)
	deviceService.SetBootstrapScheduler(sched)
}

func wireRuntimeResetter(deviceService *service.DeviceService, resetter deviceRuntimeResetter) {
	deviceService.SetRuntimeResetter(resetter)
}

func applyPendingSQLiteRestore(dbPath, deviceBackupDir, knownHostsPath string) error {
	applied, err := newRestoreCoordinator(dbPath, deviceBackupDir, knownHostsPath).ApplyPendingRestore()
	if err != nil {
		return fmt.Errorf("apply pending restore: %w", err)
	}
	if applied {
		log.Println("Restore applied successfully, continuing with normal startup")
	}

	return nil
}

func applyPendingPostgresRestore(dbPath, dbDSN, deviceBackupDir, knownHostsPath string) error {
	applied, err := service.NewRestoreCoordinatorWithDSN(dbPath, dbDSN, deviceBackupDir, knownHostsPath).ApplyPendingRestore()
	if err != nil {
		return fmt.Errorf("apply pending restore: %w", err)
	}
	if applied {
		log.Println("PostgreSQL restore applied successfully, continuing with normal startup")
	}

	return nil
}

type bootstrapRunner interface {
	Run(configPath string) error
}

var newBootstrapRunner = func() bootstrapRunner {
	return &runtimeBootstrap{}
}

func runMain(args []string) error {
	flags := flag.NewFlagSet("theia", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	configPath := flags.String("config", "", "Path to config file")
	if err := flags.Parse(args); err != nil {
		return err
	}

	cfgPath := *configPath
	if cfgPath == "" {
		cfgPath = os.Getenv("THEIA_CONFIG")
	}
	if cfgPath == "" {
		cfgPath = "config.yaml"
	}

	return newBootstrapRunner().Run(cfgPath)
}

func main() {
	if err := runMain(os.Args[1:]); err != nil {
		log.Fatal(err)
	}
}

func newCollectorSNMPClientFunc(settingsRepo domain.SettingsRepository) collector.NewSNMPClientFunc {
	return func(target string, creds domain.SNMPCredentials, timeout time.Duration, retries int) (collector.SNMPClient, error) {
		if settingsRepo != nil && timeout <= 0 {
			if val, err := settingsRepo.Get(domain.SettingSNMPTimeout); err == nil {
				if secs, err := strconv.Atoi(val); err == nil && secs > 0 {
					timeout = time.Duration(secs) * time.Second
				}
			}
		}
		if settingsRepo != nil && retries < 0 {
			if val, err := settingsRepo.Get(domain.SettingSNMPRetries); err == nil {
				if parsed, err := strconv.Atoi(val); err == nil && parsed >= 0 {
					retries = parsed
				}
			}
		}
		if timeout <= 0 {
			timeout = 10 * time.Second
		}
		if retries < 0 {
			retries = 2
		}

		return newCollectorSNMPClient(target, creds, timeout, retries)
	}
}

// newSNMPMetricsPollFunc creates an SNMPPollFunc that polls CPU/MEM/UPTIME/TEMP
// directly from a device. Used as a fallback when Prometheus has no data.
func newSNMPMetricsPollFunc(settingsRepo domain.SettingsRepository, vendorRegistry *vendor.Registry) worker.SNMPPollFunc {
	return func(target string, creds domain.SNMPCredentials, vendorName string) (domain.DeviceMetrics, error) {
		timeout := 5 * time.Second
		retries := 1

		if val, err := settingsRepo.Get(domain.SettingSNMPTimeout); err == nil {
			if secs, err := strconv.Atoi(val); err == nil && secs > 0 {
				timeout = time.Duration(secs) * time.Second
			}
		}

		client, err := snmp.NewClient(target, creds, timeout, retries)
		if err != nil {
			return domain.DeviceMetrics{}, err
		}
		if err := client.Connect(); err != nil {
			return domain.DeviceMetrics{}, err
		}
		defer client.Close()

		perfOIDs := vendorRegistry.ResolvePerformanceOIDs(vendorName)
		cpu, mem, uptime, temp := snmp.PollDeviceMetrics(client, perfOIDs)
		return domain.DeviceMetrics{
			CPUPercent:  cpu,
			MemPercent:  mem,
			UptimeSecs:  uptime,
			TempCelsius: temp,
		}, nil
	}
}

// newSNMPLinkPollFunc creates an SNMPLinkPollFunc that polls ifHCInOctets and
// ifHCOutOctets for interface throughput data on SNMP-sourced devices.
func newSNMPLinkPollFunc(settingsRepo domain.SettingsRepository) worker.SNMPLinkPollFunc {
	return func(target string, creds domain.SNMPCredentials) ([]worker.SNMPIfCounter, error) {
		timeout := 5 * time.Second
		retries := 1

		if val, err := settingsRepo.Get(domain.SettingSNMPTimeout); err == nil {
			if secs, err := strconv.Atoi(val); err == nil && secs > 0 {
				timeout = time.Duration(secs) * time.Second
			}
		}

		client, err := snmp.NewClient(target, creds, timeout, retries)
		if err != nil {
			return nil, err
		}
		if err := client.Connect(); err != nil {
			return nil, err
		}
		defer client.Close()

		raw := snmp.PollInterfaceCounters(client)
		result := make([]worker.SNMPIfCounter, len(raw))
		for i, c := range raw {
			result[i] = worker.SNMPIfCounter{
				IfName:    c.IfName,
				InOctets:  c.InOctets,
				OutOctets: c.OutOctets,
			}
		}
		return result, nil
	}
}

// newSNMPDiscoverFunc creates a DiscoverFunc that uses real gosnmp clients.
// It reads SNMP timeout and retries from the settings repository.
func newSNMPDiscoverFunc(settingsRepo domain.SettingsRepository, vendorRegistry *vendor.Registry) service.DiscoverFunc {
	return func(target string, creds domain.SNMPCredentials, topologyMode domain.TopologyDiscoveryMode) (*snmp.DiscoveryResult, error) {
		// Read timeout and retries from settings
		timeout := 5 * time.Second
		retries := 2

		if val, err := settingsRepo.Get(domain.SettingSNMPTimeout); err == nil {
			if secs, err := strconv.Atoi(val); err == nil && secs > 0 {
				timeout = time.Duration(secs) * time.Second
			}
		}
		if val, err := settingsRepo.Get(domain.SettingSNMPRetries); err == nil {
			if r, err := strconv.Atoi(val); err == nil && r >= 0 {
				retries = r
			}
		}

		client, err := snmp.NewClient(target, creds, timeout, retries)
		if err != nil {
			return nil, err
		}

		if err := client.Connect(); err != nil {
			return nil, err
		}
		defer client.Close()

		return snmp.DiscoverDeviceWithPolicy(client, vendorRegistry, snmp.NeighborDiscoveryPolicyFromMode(topologyMode))
	}
}
