package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/lollinoo/theia/internal/api"
	"github.com/lollinoo/theia/internal/cache"
	"github.com/lollinoo/theia/internal/collector"
	"github.com/lollinoo/theia/internal/config"
	"github.com/lollinoo/theia/internal/crypto"
	"github.com/lollinoo/theia/internal/domain"
	"github.com/lollinoo/theia/internal/metrics"
	"github.com/lollinoo/theia/internal/observability"
	"github.com/lollinoo/theia/internal/repository/sqlite"
	"github.com/lollinoo/theia/internal/scheduler"
	"github.com/lollinoo/theia/internal/service"
	"github.com/lollinoo/theia/internal/snmp"
	"github.com/lollinoo/theia/internal/ssh"
	"github.com/lollinoo/theia/internal/state"
	"github.com/lollinoo/theia/internal/vendor"
	"github.com/lollinoo/theia/internal/version"
	"github.com/lollinoo/theia/internal/worker"
	"github.com/lollinoo/theia/internal/ws"

	_ "github.com/jackc/pgx/v5/stdlib"
	_ "github.com/mattn/go-sqlite3"
)

type pendingRestoreCoordinator interface {
	ApplyPendingRestore() (bool, error)
}

var newRestoreCoordinator = func(dbPath, deviceBackupDir, knownHostsPath string) pendingRestoreCoordinator {
	return service.NewRestoreCoordinator(dbPath, deviceBackupDir, knownHostsPath)
}

var newCollectorSNMPClient = func(target string, creds domain.SNMPCredentials, timeout time.Duration, retries int) (collector.SNMPClient, error) {
	return snmp.NewClient(target, creds, timeout, retries)
}

func wirePollRescheduler(deviceService *service.DeviceService, sched *scheduler.Scheduler) {
	deviceService.SetPollRescheduler(sched)
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

func main() {
	// Determine config file path
	configPath := flag.String("config", "", "Path to config file")
	flag.Parse()

	cfgPath := *configPath
	if cfgPath == "" {
		cfgPath = os.Getenv("THEIA_CONFIG")
	}
	if cfgPath == "" {
		cfgPath = "config.yaml"
	}

	// Load bootstrap configuration
	cfg, err := config.Load(cfgPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	log.Printf("Config loaded: driver=%s listen=%s log_level=%s", cfg.DBDriver, cfg.ListenAddr, cfg.LogLevel)

	paths := resolveRuntimePaths(cfg)

	dialect, err := sqlite.NormalizeDialect(cfg.DBDriver)
	if err != nil {
		log.Fatalf("Invalid database driver: %v", err)
	}

	if dialect == sqlite.DialectSQLite {
		dbDir := filepath.Dir(cfg.DBPath)
		if err := os.MkdirAll(dbDir, 0755); err != nil {
			log.Fatalf("Failed to create database directory %s: %v", dbDir, err)
		}

		// Check for pending restore BEFORE opening the database (D-07)
		if err := applyPendingSQLiteRestore(cfg.DBPath, paths.backupDir, paths.knownHostsPath); err != nil {
			log.Fatalf("Failed to apply pending SQLite restore: %v", err)
		}
	}

	if err := os.MkdirAll(paths.appDataDir, 0755); err != nil {
		log.Fatalf("Failed to create application data directory %s: %v", paths.appDataDir, err)
	}

	db, dialect, err := sqlite.OpenPrimaryDB(cfg.DBDriver, cfg.DBPath, cfg.DBDSN)
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}
	sqlite.ConfigureDB(db)
	switch dialect {
	case sqlite.DialectPostgres:
		log.Printf("Database dialect: %s (production reference path)", dialect)
	default:
		log.Printf("Database dialect: %s (development/small-install path; PostgreSQL is the production reference)", dialect)
	}
	defer db.Close()

	if err := db.Ping(); err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}

	// Load encryption key (required for SNMP credential migration and all crypto operations)
	encryptionKey, err := crypto.LoadEncryptionKey()
	if err != nil {
		log.Fatalf("Security configuration error: %v", err)
	}

	// Run migrations
	if err := sqlite.RunMigrations(db, encryptionKey); err != nil {
		log.Fatalf("Failed to run database migrations: %v", err)
	}
	log.Println("Database migrations completed")

	// Load vendor registry for seeding.
	// By default the built-in configs are loaded from the binary (go:embed).
	// Set THEIA_VENDORS_DIR to override with a custom directory on disk.
	yamlRegistry, envVendors, err := loadBootstrapVendorRegistry()
	if err != nil {
		if envVendors != "" {
			log.Fatalf("Failed to load vendor registry from %s: %v", envVendors, err)
		}
		log.Fatalf("Failed to load embedded vendor registry: %v", err)
	}
	if envVendors != "" {
		log.Printf("Vendor YAML loaded: %d vendors from %s", yamlRegistry.VendorCount(), envVendors)
	} else {
		log.Printf("Vendor registry loaded: %d vendors (embedded)", yamlRegistry.VendorCount())
	}

	// Create vendor config repo and seed DB from YAML
	vendorConfigRepo := sqlite.NewVendorConfigRepo(db)
	seedVendorConfigs(yamlRegistry, vendorConfigRepo)

	// Build live registry from DB
	vendorRegistry, err := loadRegistryFromDB(vendorConfigRepo)
	if err != nil || vendorRegistry == nil {
		if err != nil {
			log.Printf("Warning: failed to load registry from DB, falling back to YAML: %v", err)
		} else {
			log.Printf("Warning: DB vendor registry empty/invalid, falling back to YAML registry")
		}
		vendorRegistry = yamlRegistry
	}
	log.Printf("Vendor registry loaded: %d vendors", vendorRegistry.VendorCount())

	// Create shared invalidation channel for device/link cache
	cacheInvalidate := make(chan struct{}, 1)

	// Create repositories
	deviceRepo := sqlite.NewDeviceRepo(db, encryptionKey, cacheInvalidate)
	linkRepo := sqlite.NewLinkRepo(db, cacheInvalidate)
	topologyObservationRepo := sqlite.NewTopologyObservationRepo(db)
	deviceLinkCache := cache.NewDeviceLinkCache(deviceRepo, linkRepo, cacheInvalidate)
	deviceChangeNotify := deviceRepo.SubscribeDeviceChanges(256)
	linkChangeNotify := linkRepo.SubscribeLinkChanges(256)
	positionRepo := sqlite.NewPositionRepo(db)
	settingsRepo := sqlite.NewSettingsRepo(db)
	snmpProfileRepo := sqlite.NewSNMPProfileRepo(db, encryptionKey)
	credentialProfileRepo := sqlite.NewCredentialProfileRepo(db)
	areaRepo := sqlite.NewAreaRepo(db)
	backupJobRepo := sqlite.NewBackupJobRepo(db)
	backupFileRepo := sqlite.NewBackupFileRepo(db)

	// Create SNMP discovery function (real gosnmp clients)
	discoverFunc := newSNMPDiscoverFunc(settingsRepo, vendorRegistry)

	// Topology notify channel: DeviceService signals when probeDevice creates new LLDP/CDP links.
	// MetricsCollector drains this channel after each broadcast and sends topology_changed to clients.
	// Buffered(1) with non-blocking send so probeDevice goroutines never block (T-33-02).
	topologyNotify := make(chan struct{}, 1)

	// Create service layer
	deviceService := service.NewDeviceService(
		deviceRepo,
		linkRepo,
		settingsRepo,
		discoverFunc,
		topologyNotify,
		service.WithTopologyObservationStore(topologyObservationRepo),
	)

	// Create backup service
	sshDialer := &ssh.DefaultDialer{}
	if err := os.MkdirAll(paths.backupDir, 0755); err != nil {
		log.Fatalf("Failed to create backup directory %s: %v", paths.backupDir, err)
	}

	// Initialize SSH known hosts store for host key verification
	knownHostsStore, err := ssh.NewKnownHostsStore(paths.knownHostsPath)
	if err != nil {
		log.Fatalf("Failed to initialize SSH known hosts store: %v", err)
	}
	log.Printf("SSH known hosts store: %s", paths.knownHostsPath)

	backupService := service.NewBackupService(backupJobRepo, backupFileRepo, credentialProfileRepo, deviceRepo, settingsRepo, vendorRegistry, sshDialer, encryptionKey, paths.backupDir, knownHostsStore.HostKeyCallback())

	var instanceBackupService *service.InstanceBackupService
	var backupScheduler *worker.BackupScheduler
	if dialect == sqlite.DialectSQLite {
		instanceBackupRepo := sqlite.NewInstanceBackupRepo(db)
		if err := os.MkdirAll(paths.instanceBackupDir, 0755); err != nil {
			log.Fatalf("Failed to create instance backup directory %s: %v", paths.instanceBackupDir, err)
		}
		instanceBackupService = service.NewInstanceBackupService(
			db,
			instanceBackupRepo,
			settingsRepo,
			paths.instanceBackupDir,
			paths.backupDir,
			paths.knownHostsPath,
			cfg.DBPath,
			encryptionKey,
		)
		log.Printf("Instance backup directory: %s", paths.instanceBackupDir)
		instanceBackupService.FailStaleRunning()
		backupScheduler = worker.NewBackupScheduler(instanceBackupService, instanceBackupRepo, settingsRepo)
	} else {
		log.Printf("Instance backup and restore are disabled for database driver %s", dialect)
	}

	// Create device backup scheduler
	deviceBackupScheduler := worker.NewDeviceBackupScheduler(backupService, backupJobRepo, settingsRepo)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	prometheusURL := ""
	if value, err := settingsRepo.Get(domain.SettingPrometheusURL); err == nil {
		prometheusURL = strings.TrimSpace(value)
	} else {
		log.Printf("Warning: failed to read Prometheus URL: %v", err)
	}

	var promClient collector.PrometheusEnrichmentClient
	promHTTPClient := &http.Client{Timeout: 4 * time.Second}
	if prometheusURL != "" {
		promClient = metrics.NewPromClient(prometheusURL, promHTTPClient)
	} else {
		log.Println("Prometheus integration disabled: no prometheus_url configured")
	}
	hub := ws.NewHub()
	go hub.Run()

	stateStore := state.NewStore()
	sched := scheduler.NewScheduler(deviceLinkCache, settingsRepo)
	wirePollRescheduler(deviceService, sched)
	snmpClientFactory := newCollectorSNMPClientFunc(settingsRepo)
	performanceCollector := collector.NewPerformanceCollector(vendorRegistry, snmpClientFactory)
	operationalCollector := collector.NewOperationalCollector(vendorRegistry, snmpClientFactory)
	staticCollector := collector.NewStaticCollector(vendorRegistry, snmpClientFactory)
	promCollector := collector.NewPrometheusCollector(promClient)
	promCollector.SetClientFactory(func(baseURL string) collector.PrometheusEnrichmentClient {
		return metrics.NewPromClient(baseURL, promHTTPClient)
	})
	pipeline := worker.NewPipelineOrchestrator(
		sched,
		stateStore,
		deviceLinkCache,
		hub,
		performanceCollector,
		operationalCollector,
		staticCollector,
		promCollector,
		deviceService,
		settingsRepo,
		topologyNotify,
		deviceChangeNotify,
		linkChangeNotify,
	)
	if err := pipeline.Start(ctx); err != nil {
		log.Fatalf("Failed to start runtime pipeline: %v", err)
	}
	if backupScheduler != nil {
		backupScheduler.Start(ctx)
	}
	deviceBackupScheduler.Start(ctx)

	wsHandler := ws.NewHandler(hub, pipeline.GetOverviewSnapshot, pipeline.GetAlerts, pipeline.GetPrometheusStatus)

	// Create HTTP router with all /api/v1/ routes
	router := api.NewRouter(db, deviceService, linkRepo, positionRepo, settingsRepo, snmpProfileRepo, credentialProfileRepo, areaRepo, backupService, vendorRegistry, vendorConfigRepo, pipeline, instanceBackupService, cfg.BridgeBinariesDir, wsHandler)
	metricsHandler := observability.Handler()

	// Create HTTP server
	server := &http.Server{
		Addr: cfg.ListenAddr,
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/metrics" {
				metricsHandler.ServeHTTP(w, r)
				return
			}
			router.ServeHTTP(w, r)
		}),
	}

	// Graceful shutdown on SIGINT/SIGTERM
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-sigCh
		log.Printf("Received signal %s, shutting down...", sig)

		// Stop background workers
		pipeline.Stop()
		if backupScheduler != nil {
			backupScheduler.Stop()
		}
		deviceBackupScheduler.Stop()

		// Shutdown HTTP server with timeout
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer shutdownCancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			log.Printf("HTTP server shutdown error: %v", err)
		}
	}()

	log.Printf("Theia %s (commit=%s built=%s) starting on %s",
		version.Version, version.GitCommit, version.BuildDate, cfg.ListenAddr)
	if err := server.ListenAndServe(); err != http.ErrServerClosed {
		log.Fatalf("Server error: %v", err)
	}
	log.Println("Server stopped")
}

func newCollectorSNMPClientFunc(settingsRepo domain.SettingsRepository) collector.NewSNMPClientFunc {
	return func(target string, creds domain.SNMPCredentials, timeout time.Duration, retries int) (collector.SNMPClient, error) {
		if timeout <= 0 {
			timeout = 10 * time.Second
		}
		if retries < 0 {
			retries = 2
		}

		if settingsRepo != nil {
			if val, err := settingsRepo.Get(domain.SettingSNMPTimeout); err == nil {
				if secs, err := strconv.Atoi(val); err == nil && secs > 0 {
					timeout = time.Duration(secs) * time.Second
				}
			}
			if val, err := settingsRepo.Get(domain.SettingSNMPRetries); err == nil {
				if parsed, err := strconv.Atoi(val); err == nil && parsed >= 0 {
					retries = parsed
				}
			}
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
