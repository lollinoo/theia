package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/lollinoo/theia/internal/api"
	"github.com/lollinoo/theia/internal/cache"
	"github.com/lollinoo/theia/internal/collector"
	"github.com/lollinoo/theia/internal/config"
	"github.com/lollinoo/theia/internal/crypto"
	"github.com/lollinoo/theia/internal/domain"
	"github.com/lollinoo/theia/internal/logging"
	"github.com/lollinoo/theia/internal/metrics"
	"github.com/lollinoo/theia/internal/observability"
	"github.com/lollinoo/theia/internal/repository/sqlite"
	"github.com/lollinoo/theia/internal/scheduler"
	"github.com/lollinoo/theia/internal/service"
	"github.com/lollinoo/theia/internal/ssh"
	"github.com/lollinoo/theia/internal/state"
	"github.com/lollinoo/theia/internal/version"
	"github.com/lollinoo/theia/internal/worker"
	"github.com/lollinoo/theia/internal/ws"
)

type runtimeConfig = config.Config

type runtimeStopper interface {
	Stop()
}

type runtimeChildren []runtimeStopper

type runtimeServer interface {
	ListenAndServe() error
	Shutdown(ctx context.Context) error
}

type runtimeBootstrap struct{}

var loadRuntimeConfig = func(path string) (*runtimeConfig, error) {
	return config.Load(path)
}

var openPrimaryRuntimeDB = sqlite.OpenPrimaryDB

func validateDatabasePolicy(cfg *runtimeConfig, dialect sqlite.Dialect) error {
	switch dialect {
	case sqlite.DialectSQLite:
		if os.Getenv("THEIA_ALLOW_SQLITE_SMALL_INSTALL") == "true" {
			return nil
		}
		return fmt.Errorf(
			"sqlite is only supported for demo, lab, or small-install deployments; " +
				"set THEIA_ALLOW_SQLITE_SMALL_INSTALL=true only if this instance stays within the documented small-install limits (up to 50 devices, one Theia process, one active admin)",
		)
	case sqlite.DialectPostgres:
		if strings.TrimSpace(cfg.DBDSN) == "" {
			return fmt.Errorf(
				"postgres is the default database driver and requires db_dsn; " +
					"set THEIA_DB_DSN (for example postgres://theia:theia@127.0.0.1:5432/theia?sslmode=disable), start the standard dev stack with make dev, or migrate an existing SQLite dataset with make migrate-postgres",
			)
		}
	}

	return nil
}

func wrapPostgresConnectError(err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf(
		"connect to database: %w; set THEIA_DB_DSN (for example postgres://theia:theia@127.0.0.1:5432/theia?sslmode=disable), start the standard dev stack with make dev, or migrate an existing SQLite dataset with make migrate-postgres",
		err,
	)
}

func wrapPostgresOpenError(err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf(
		"open database: %w; set THEIA_DB_DSN (for example postgres://theia:theia@127.0.0.1:5432/theia?sslmode=disable), start the standard dev stack with make dev, or migrate an existing SQLite dataset with make migrate-postgres",
		err,
	)
}

func runtimeDBDriver(driver string) string {
	if strings.TrimSpace(driver) == "" {
		return string(sqlite.DialectPostgres)
	}
	return driver
}

func (b *runtimeBootstrap) Run(configPath string) error {
	cfg, err := loadRuntimeConfig(configPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	logging.Configure(cfg.LogLevel)
	logging.Infof("Config loaded: driver=%s listen=%s log_level=%s", cfg.DBDriver, cfg.ListenAddr, cfg.LogLevel)

	paths := resolveRuntimePaths(cfg)
	runtimeDriver := runtimeDBDriver(cfg.DBDriver)

	dialect, err := sqlite.NormalizeDialect(runtimeDriver)
	if err != nil {
		return fmt.Errorf("invalid database driver: %w", err)
	}
	if err := validateDatabasePolicy(cfg, dialect); err != nil {
		return err
	}
	if err := ensurePrivateDir(paths.backupDir); err != nil {
		return fmt.Errorf("prepare backup directory %s: %w", paths.backupDir, err)
	}
	dbDir := filepath.Dir(cfg.DBPath)
	if err := ensurePrivateDir(dbDir); err != nil {
		return fmt.Errorf("prepare database directory %s: %w", dbDir, err)
	}

	switch dialect {
	case sqlite.DialectSQLite:
		if err := applyPendingSQLiteRestore(cfg.DBPath, paths.backupDir, paths.knownHostsPath); err != nil {
			return fmt.Errorf("apply pending SQLite restore: %w", err)
		}
	case sqlite.DialectPostgres:
		if err := applyPendingPostgresRestore(cfg.DBPath, cfg.DBDSN, paths.backupDir, paths.knownHostsPath); err != nil {
			return fmt.Errorf("apply pending PostgreSQL restore: %w", err)
		}
	}

	if err := ensurePrivateDir(paths.appDataDir); err != nil {
		return fmt.Errorf("prepare application data directory %s: %w", paths.appDataDir, err)
	}
	if _, err := os.Stat(paths.knownHostsPath); err == nil {
		if err := ensureFileMode(paths.knownHostsPath, privateFileMode); err != nil {
			return fmt.Errorf("prepare known_hosts file %s: %w", paths.knownHostsPath, err)
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("stat known_hosts file %s: %w", paths.knownHostsPath, err)
	}

	db, openedDialect, err := openPrimaryRuntimeDB(runtimeDriver, cfg.DBPath, cfg.DBDSN)
	if err != nil {
		if dialect == sqlite.DialectPostgres {
			return wrapPostgresOpenError(err)
		}
		return fmt.Errorf("open database: %w", err)
	}
	dialect = openedDialect
	defer db.Close()

	sqlite.ConfigureDB(db)
	switch dialect {
	case sqlite.DialectPostgres:
		log.Printf("Database dialect: %s (production reference path)", dialect)
	default:
		log.Printf("Database dialect: %s (development/small-install path; PostgreSQL is the production reference)", dialect)
	}

	if err := db.Ping(); err != nil {
		if dialect == sqlite.DialectPostgres {
			return wrapPostgresConnectError(err)
		}
		return fmt.Errorf("connect to database: %w", err)
	}

	encryptionKey, err := crypto.LoadEncryptionKey()
	if err != nil {
		return fmt.Errorf("security configuration error: %w", err)
	}

	if err := sqlite.RunMigrations(db, encryptionKey); err != nil {
		return fmt.Errorf("run database migrations: %w", err)
	}
	log.Println("Database migrations completed")

	yamlRegistry, envVendors, err := loadBootstrapVendorRegistry()
	if err != nil {
		if envVendors != "" {
			return fmt.Errorf("load vendor registry from %s: %w", envVendors, err)
		}
		return fmt.Errorf("load embedded vendor registry: %w", err)
	}
	if envVendors != "" {
		log.Printf("Vendor YAML loaded: %d vendors from %s", yamlRegistry.VendorCount(), envVendors)
	} else {
		log.Printf("Vendor registry loaded: %d vendors (embedded)", yamlRegistry.VendorCount())
	}

	vendorConfigRepo := sqlite.NewVendorConfigRepo(db)
	seedVendorConfigs(yamlRegistry, vendorConfigRepo)

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

	cacheInvalidate := make(chan struct{}, 1)

	deviceRepo := sqlite.NewDeviceRepo(db, encryptionKey, cacheInvalidate)
	linkRepo := sqlite.NewLinkRepo(db, cacheInvalidate)
	topologyObservationRepo := sqlite.NewTopologyObservationRepo(db)
	deviceLinkCache := cache.NewDeviceLinkCache(deviceRepo, linkRepo, cacheInvalidate)
	deviceChangeNotify := deviceRepo.SubscribeDeviceChanges(256)
	linkChangeNotify := linkRepo.SubscribeLinkChanges(256)
	positionRepo := sqlite.NewPositionRepo(db)
	settingsRepo := sqlite.NewSettingsRepo(db)
	logging.Debugf("runtime effective config %s", runtimeDebugSettingsSummary(cfg, settingsRepo))
	snmpProfileRepo := sqlite.NewSNMPProfileRepo(db, encryptionKey)
	credentialProfileRepo := sqlite.NewCredentialProfileRepo(db)
	areaRepo := sqlite.NewAreaRepo(db)
	backupJobRepo := sqlite.NewBackupJobRepo(db)
	backupFileRepo := sqlite.NewBackupFileRepo(db)

	discoverFunc := newSNMPDiscoverFunc(settingsRepo, vendorRegistry)
	topologyNotify := make(chan struct{}, 1)
	deviceService := service.NewDeviceService(
		deviceRepo,
		linkRepo,
		settingsRepo,
		discoverFunc,
		topologyNotify,
		service.WithTopologyObservationStore(topologyObservationRepo),
	)

	sshDialer := &ssh.DefaultDialer{}

	knownHostsStore, err := ssh.NewKnownHostsStore(paths.knownHostsPath)
	if err != nil {
		return fmt.Errorf("initialize SSH known hosts store: %w", err)
	}
	log.Printf("SSH known hosts store: %s", paths.knownHostsPath)

	backupService := service.NewBackupService(backupJobRepo, backupFileRepo, credentialProfileRepo, deviceRepo, settingsRepo, vendorRegistry, sshDialer, encryptionKey, paths.backupDir, knownHostsStore.HostKeyCallback())

	var instanceBackupService *service.InstanceBackupService
	var backupScheduler *worker.BackupScheduler
	instanceBackupRepo := sqlite.NewInstanceBackupRepo(db)
	if err := ensurePrivateDir(paths.instanceBackupDir); err != nil {
		return fmt.Errorf("prepare instance backup directory %s: %w", paths.instanceBackupDir, err)
	}
	instanceBackupService = service.NewInstanceBackupService(
		db,
		instanceBackupRepo,
		settingsRepo,
		paths.instanceBackupDir,
		paths.backupDir,
		paths.knownHostsPath,
		cfg.DBPath,
		cfg.DBDSN,
		encryptionKey,
	)
	log.Printf("Instance backup directory: %s", paths.instanceBackupDir)
	instanceBackupService.FailStaleRunning()
	backupScheduler = worker.NewBackupScheduler(instanceBackupService, instanceBackupRepo, settingsRepo)

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
	essentialCollector := collector.NewEssentialCollector(vendorRegistry, snmpClientFactory)
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
		essentialCollector,
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
	wireRuntimeResetter(deviceService, pipeline)
	if err := pipeline.Start(ctx); err != nil {
		return fmt.Errorf("start runtime pipeline: %w", err)
	}
	if backupScheduler != nil {
		backupScheduler.Start(ctx)
	}
	deviceBackupScheduler.Start(ctx)

	wsHandler := ws.NewHandler(hub, pipeline.GetOverviewSnapshot, pipeline.GetAlerts, pipeline.GetPrometheusStatus)
	router := api.NewRouter(db, deviceService, linkRepo, positionRepo, settingsRepo, snmpProfileRepo, credentialProfileRepo, areaRepo, backupService, vendorRegistry, vendorConfigRepo, pipeline, instanceBackupService, cfg.BridgeBinariesDir, pipeline.GetOrBuildOverviewSnapshot, wsHandler)
	metricsHandler := observability.Handler()
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

	children := runtimeChildren{pipeline}
	if backupScheduler != nil {
		children = append(children, backupScheduler)
	}
	children = append(children, deviceBackupScheduler)
	b.handleShutdown(cancel, server, children)

	log.Printf("Theia %s (commit=%s built=%s) starting on %s", version.Version, version.GitCommit, version.BuildDate, cfg.ListenAddr)
	if err := b.serve(server); err != nil {
		return fmt.Errorf("server error: %w", err)
	}
	log.Println("Server stopped")
	return nil
}

func runtimeDebugSettingsSummary(cfg *runtimeConfig, settingsRepo domain.SettingsRepository) string {
	cfgLogLevel := ""
	cfgDBDriver := ""
	cfgListen := ""
	if cfg != nil {
		cfgLogLevel = cfg.LogLevel
		cfgDBDriver = cfg.DBDriver
		cfgListen = cfg.ListenAddr
	}
	prometheusSetting := runtimeDebugSetting(settingsRepo, domain.SettingPrometheusURL)

	parts := []string{
		"log_level=" + debugSettingValue(cfgLogLevel),
		"db_driver=" + debugSettingValue(cfgDBDriver),
		"listen=" + debugSettingValue(cfgListen),
		"polling_interval_seconds=" + runtimeDebugSetting(settingsRepo, domain.SettingPollingInterval),
		"pool_performance=" + runtimeDebugSetting(settingsRepo, domain.SettingSNMPWorkerPoolPerformance),
		"pool_operational=" + runtimeDebugSetting(settingsRepo, domain.SettingSNMPWorkerPoolOperational),
		"pool_static=" + runtimeDebugSetting(settingsRepo, domain.SettingSNMPWorkerPoolStatic),
		"polling_max_workers_per_device=" + runtimeDebugSetting(settingsRepo, domain.SettingPollingMaxWorkersPerDevice),
		"snmp_timeout_seconds=" + runtimeDebugSetting(settingsRepo, domain.SettingSNMPTimeout),
		"snmp_retries=" + runtimeDebugSetting(settingsRepo, domain.SettingSNMPRetries),
		"websocket_coalesce_ms=" + runtimeDebugSetting(settingsRepo, domain.SettingPollingWebSocketCoalesceMS),
		"persistence_batch_ms=" + runtimeDebugSetting(settingsRepo, domain.SettingPollingPersistenceBatchMS),
		"topology_discovery_default_mode=" + runtimeDebugSetting(settingsRepo, domain.SettingTopologyDiscoveryDefaultMode),
		fmt.Sprintf("prometheus_configured=%t", strings.TrimSpace(prometheusSetting) != "" && prometheusSetting != "-"),
	}
	return strings.Join(parts, " ")
}

func runtimeDebugSetting(settingsRepo domain.SettingsRepository, key string) string {
	if settingsRepo == nil {
		return "-"
	}
	value, err := settingsRepo.Get(key)
	if err != nil {
		return "-"
	}
	if key == domain.SettingPrometheusURL || key == domain.SettingGrafanaURL || key == domain.SettingBridgeSecret {
		if strings.TrimSpace(value) == "" {
			return ""
		}
		return "<set>"
	}
	return debugSettingValue(value)
}

func debugSettingValue(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "-"
	}
	return value
}

func (b *runtimeBootstrap) handleShutdown(cancel context.CancelFunc, server runtimeServer, children runtimeChildren) {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-sigCh
		log.Printf("Received signal %s, shutting down...", sig)

		cancel()
		b.stopRuntime(children)

		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer shutdownCancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			log.Printf("HTTP server shutdown error: %v", err)
		}
	}()
}

func (b *runtimeBootstrap) stopRuntime(children runtimeChildren) {
	for i := len(children) - 1; i >= 0; i-- {
		if children[i] != nil {
			children[i].Stop()
		}
	}
}

func (b *runtimeBootstrap) serve(server runtimeServer) error {
	err := server.ListenAndServe()
	if err == nil || errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}
