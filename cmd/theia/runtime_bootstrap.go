package main

// This file defines runtime bootstrap behavior for Theia server startup.

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"sync"
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
	"github.com/lollinoo/theia/internal/repository/postgres"
	"github.com/lollinoo/theia/internal/scheduler"
	"github.com/lollinoo/theia/internal/service"
	"github.com/lollinoo/theia/internal/ssh"
	"github.com/lollinoo/theia/internal/state"
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

var openPrimaryRuntimeDB = postgres.OpenPrimaryDB

var knownSecretPlaceholders = map[string]struct{}{
	"change-me":        {},
	"changeme":         {},
	"example":          {},
	"example-secret":   {},
	"password":         {},
	"secret":           {},
	"theia":            {},
	"devkey1234567890": {},
}

func validateDatabasePolicy(cfg *runtimeConfig) error {
	if strings.TrimSpace(cfg.DBDSN) == "" {
		return fmt.Errorf(
			"postgres is the required database and needs db_dsn; " +
				"set THEIA_DB_DSN using postgres://<postgres-user>:<postgres-password>@<postgres-host>:5432/<postgres-db>?sslmode=disable or start the standard dev stack with make dev",
		)
	}

	return nil
}

func validateDeploymentSecretPolicy(cfg *runtimeConfig) error {
	deploymentEnv := strings.ToLower(strings.TrimSpace(cfg.DeploymentEnv))
	if deploymentEnv != "production" && deploymentEnv != "staging" {
		return nil
	}

	if err := validateEncryptionKeySecretPolicy(deploymentEnv); err != nil {
		return err
	}

	if isPostgresDSNPasswordPlaceholder(cfg.DBDSN) {
		return fmt.Errorf("%s deployment rejects example THEIA_DB_DSN password values", deploymentEnv)
	}
	if postgresPassword := os.Getenv("POSTGRES_PASSWORD"); isKnownSecretPlaceholder(postgresPassword) {
		return fmt.Errorf("%s deployment rejects example POSTGRES_PASSWORD values", deploymentEnv)
	}
	sessionSecret := strings.TrimSpace(cfg.SessionSecret)
	if sessionSecret == "" {
		return fmt.Errorf("THEIA_SESSION_SECRET is required for %s deployment", deploymentEnv)
	}
	if isKnownSecretPlaceholder(sessionSecret) || len(sessionSecret) < 32 {
		return fmt.Errorf("%s deployment rejects weak THEIA_SESSION_SECRET values", deploymentEnv)
	}
	metricsToken := strings.TrimSpace(cfg.MetricsToken)
	if metricsToken == "" {
		return fmt.Errorf("THEIA_METRICS_TOKEN is required for %s deployment", deploymentEnv)
	}
	if isKnownSecretPlaceholder(metricsToken) || len(metricsToken) < 32 {
		return fmt.Errorf("%s deployment rejects weak THEIA_METRICS_TOKEN values", deploymentEnv)
	}

	return nil
}

func validateEncryptionKeySecretPolicy(deploymentEnv string) error {
	activeKeyID := strings.TrimSpace(os.Getenv("THEIA_ENCRYPTION_KEY_ID"))
	keyList := strings.TrimSpace(os.Getenv("THEIA_ENCRYPTION_KEYS"))
	if activeKeyID != "" || keyList != "" {
		if activeKeyID == "" {
			return fmt.Errorf("THEIA_ENCRYPTION_KEY_ID is required for %s deployment when THEIA_ENCRYPTION_KEYS is set", deploymentEnv)
		}
		if keyList == "" {
			return fmt.Errorf("THEIA_ENCRYPTION_KEYS is required for %s deployment when THEIA_ENCRYPTION_KEY_ID is set", deploymentEnv)
		}
		if _, err := crypto.ParseKeyring(activeKeyID, keyList); err != nil {
			return fmt.Errorf("%s deployment rejects malformed THEIA_ENCRYPTION_KEYS: %w", deploymentEnv, err)
		}
		if encryptionKeyListHasPlaceholderSecret(keyList) {
			return fmt.Errorf("%s deployment rejects example THEIA_ENCRYPTION_KEYS values", deploymentEnv)
		}
		return nil
	}

	encryptionKey := strings.TrimSpace(os.Getenv("THEIA_ENCRYPTION_KEY"))
	if encryptionKey == "" {
		return fmt.Errorf("THEIA_ENCRYPTION_KEY is required for %s deployment", deploymentEnv)
	}
	if isKnownSecretPlaceholder(encryptionKey) {
		return fmt.Errorf("%s deployment rejects example THEIA_ENCRYPTION_KEY values", deploymentEnv)
	}
	return nil
}

func encryptionKeyListHasPlaceholderSecret(keyList string) bool {
	for _, rawPair := range strings.Split(keyList, ",") {
		_, secret, ok := strings.Cut(rawPair, "=")
		if !ok {
			continue
		}
		if isKnownSecretPlaceholder(secret) {
			return true
		}
	}
	return false
}

func isKnownSecretPlaceholder(value string) bool {
	_, ok := knownSecretPlaceholders[strings.ToLower(strings.TrimSpace(value))]
	return ok
}

func isPostgresDSNPasswordPlaceholder(dsn string) bool {
	for _, password := range postgresDSNPasswords(dsn) {
		if isKnownSecretPlaceholder(password) {
			return true
		}
	}

	return false
}

func postgresDSNPasswords(dsn string) []string {
	dsn = strings.TrimSpace(dsn)
	if dsn == "" {
		return nil
	}

	var passwords []string
	parsed, err := url.Parse(dsn)
	if err == nil && parsed.Scheme != "" {
		if parsed.User != nil {
			if password, ok := parsed.User.Password(); ok {
				passwords = append(passwords, password)
			}
		}
		for _, password := range parsed.Query()["password"] {
			passwords = append(passwords, password)
		}
		return passwords
	}

	return append(passwords, postgresKeywordDSNValues(dsn, "password")...)
}

func postgresKeywordDSNValues(dsn, wantKey string) []string {
	var values []string
	for i := 0; i < len(dsn); {
		for i < len(dsn) && isPostgresDSNSpace(dsn[i]) {
			i++
		}
		if i >= len(dsn) {
			return values
		}

		keyStart := i
		for i < len(dsn) && dsn[i] != '=' && !isPostgresDSNSpace(dsn[i]) {
			i++
		}
		key := dsn[keyStart:i]
		for i < len(dsn) && isPostgresDSNSpace(dsn[i]) {
			i++
		}
		if key == "" || i >= len(dsn) || dsn[i] != '=' {
			return values
		}
		i++
		for i < len(dsn) && isPostgresDSNSpace(dsn[i]) {
			i++
		}

		var value strings.Builder
		if i < len(dsn) && dsn[i] == '\'' {
			i++
			for i < len(dsn) {
				if dsn[i] == '\\' && i+1 < len(dsn) {
					i++
					value.WriteByte(dsn[i])
					i++
					continue
				}
				if dsn[i] == '\'' {
					i++
					break
				}
				value.WriteByte(dsn[i])
				i++
			}
		} else {
			for i < len(dsn) && !isPostgresDSNSpace(dsn[i]) {
				if dsn[i] == '\\' && i+1 < len(dsn) {
					i++
					value.WriteByte(dsn[i])
					i++
					continue
				}
				value.WriteByte(dsn[i])
				i++
			}
		}

		if strings.EqualFold(key, wantKey) {
			values = append(values, value.String())
		}
	}

	return values
}

func isPostgresDSNSpace(value byte) bool {
	switch value {
	case ' ', '\t', '\n', '\r':
		return true
	default:
		return false
	}
}

func wrapPostgresConnectError(err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf(
		"connect to database: %w; set THEIA_DB_DSN using postgres://<postgres-user>:<postgres-password>@<postgres-host>:5432/<postgres-db>?sslmode=disable or start the standard dev stack with make dev",
		err,
	)
}

func wrapPostgresOpenError(err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf(
		"open database: %w; set THEIA_DB_DSN using postgres://<postgres-user>:<postgres-password>@<postgres-host>:5432/<postgres-db>?sslmode=disable or start the standard dev stack with make dev",
		err,
	)
}

func (b *runtimeBootstrap) Run(configPath string) error {
	cfg, err := loadRuntimeConfig(configPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	logging.Configure(cfg.LogLevel)
	logging.Infof("Config loaded: listen=%s log_level=%s", cfg.ListenAddr, cfg.LogLevel)

	paths := resolveRuntimePaths(cfg)

	if err := validateDeploymentSecretPolicy(cfg); err != nil {
		return err
	}
	if err := validateDatabasePolicy(cfg); err != nil {
		return err
	}
	if err := ensurePrivateDir(paths.backupDir); err != nil {
		return fmt.Errorf("prepare backup directory %s: %w", paths.backupDir, err)
	}
	if err := ensurePrivateDir(paths.appDataDir); err != nil {
		return fmt.Errorf("prepare application data directory %s: %w", paths.appDataDir, err)
	}
	if err := applyPendingPostgresRestore(paths.appDataDir, cfg.DBDSN, paths.backupDir, paths.knownHostsPath); err != nil {
		return fmt.Errorf("apply pending PostgreSQL restore: %w", err)
	}
	if _, err := os.Stat(paths.knownHostsPath); err == nil {
		if err := ensureFileMode(paths.knownHostsPath, privateFileMode); err != nil {
			return fmt.Errorf("prepare known_hosts file %s: %w", paths.knownHostsPath, err)
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("stat known_hosts file %s: %w", paths.knownHostsPath, err)
	}

	db, err := openPrimaryRuntimeDB(cfg.DBDSN)
	if err != nil {
		return wrapPostgresOpenError(err)
	}
	defer db.Close()

	postgres.ConfigureDB(db)
	log.Printf("Database dialect: %s", postgres.DialectPostgres)

	if err := db.Ping(); err != nil {
		return wrapPostgresConnectError(err)
	}

	encryptionKeyring, err := crypto.LoadKeyringFromEnv()
	if err != nil {
		return fmt.Errorf("security configuration error: %w", err)
	}

	if err := postgres.RunMigrations(db, encryptionKeyring); err != nil {
		return fmt.Errorf("run database migrations: %w", err)
	}
	log.Println("Database migrations completed")

	authRepo := postgres.NewAuthRepo(db)
	authService, err := service.NewAuthService(service.AuthServiceConfig{
		Users:            authRepo,
		Roles:            authRepo,
		Sessions:         authRepo,
		PasswordResets:   authRepo,
		AuditLogs:        authRepo,
		SessionSecret:    []byte(strings.TrimSpace(cfg.SessionSecret)),
		SessionTTL:       minutesToDuration(cfg.SessionTTLMinutes),
		PasswordResetTTL: minutesToDuration(cfg.PasswordResetTTLMinutes),
	})
	if err != nil {
		return fmt.Errorf("initialize auth service: %w", err)
	}
	if user, created, err := authService.EnsureBootstrapSuperAdmin(context.Background()); err != nil {
		return fmt.Errorf("ensure bootstrap super admin: %w", err)
	} else if created {
		log.Printf("Bootstrap super admin created username=%q must_change_password=true", user.Username)
	}

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

	vendorConfigRepo := postgres.NewVendorConfigRepo(db)
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

	deviceRepo := postgres.NewDeviceRepo(db, encryptionKeyring, cacheInvalidate)
	linkRepo := postgres.NewLinkRepo(db, cacheInvalidate)
	topologyObservationRepo := postgres.NewTopologyObservationRepo(db)
	deviceLinkCache := cache.NewDeviceLinkCache(deviceRepo, linkRepo, cacheInvalidate)
	deviceChangeNotify := deviceRepo.SubscribeDeviceChanges(256)
	linkChangeNotify := linkRepo.SubscribeLinkChanges(256)
	positionRepo := postgres.NewPositionRepo(db)
	canvasMapRepo := postgres.NewCanvasMapRepo(db)
	canvasMapPositionRepo := postgres.NewCanvasMapPositionRepo(db)
	settingsRepo := postgres.NewSettingsRepo(db)
	logging.Debugf("runtime effective config %s", runtimeDebugSettingsSummary(cfg, settingsRepo))
	snmpProfileRepo := postgres.NewSNMPProfileRepo(db, encryptionKeyring)
	credentialProfileRepo := postgres.NewCredentialProfileRepo(db)
	areaRepo := postgres.NewAreaRepo(db)
	backupJobRepo := postgres.NewBackupJobRepo(db)
	backupFileRepo := postgres.NewBackupFileRepo(db)
	bulkBackupRunRepo := postgres.NewBulkBackupRunRepo(db)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	discoverFunc := newSNMPDiscoverFunc(settingsRepo, vendorRegistry)
	topologyNotify := make(chan struct{}, 1)
	deviceService := service.NewDeviceService(
		deviceRepo,
		linkRepo,
		settingsRepo,
		discoverFunc,
		topologyNotify,
		service.WithLifecycleContext(ctx),
		service.WithTopologyObservationStore(topologyObservationRepo),
	)

	sshDialer := &ssh.DefaultDialer{}

	knownHostsStore, err := ssh.NewKnownHostsStore(paths.knownHostsPath)
	if err != nil {
		return fmt.Errorf("initialize SSH known hosts store: %w", err)
	}
	log.Printf("SSH known hosts store: %s", paths.knownHostsPath)

	backupService := service.NewBackupService(
		backupJobRepo, backupFileRepo, credentialProfileRepo, deviceRepo, settingsRepo,
		vendorRegistry, sshDialer, encryptionKeyring, paths.backupDir, knownHostsStore.HostKeyCallback(),
		service.WithBulkBackupRunRepo(bulkBackupRunRepo),
	)
	configureBackupServiceBulkOperationLimits(backupService, cfg)
	bridgeRepo := postgres.NewBridgeRepo(db)
	bridgeService, err := service.NewBridgeService(service.BridgeServiceConfig{
		BridgeRepo:            bridgeRepo,
		SettingsRepo:          settingsRepo,
		Users:                 authRepo,
		AuditLogs:             authRepo,
		BackupService:         backupService,
		CredentialProfileRepo: credentialProfileRepo,
		SessionSecret:         []byte(strings.TrimSpace(cfg.SessionSecret)),
	})
	if err != nil {
		return fmt.Errorf("initialize bridge service: %w", err)
	}

	var instanceBackupService *service.InstanceBackupService
	var backupScheduler *worker.BackupScheduler
	instanceBackupRepo := postgres.NewInstanceBackupRepo(db)
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
		paths.appDataDir,
		cfg.DBDSN,
		encryptionKeyring,
	)
	configureInstanceBackupArchiveLimits(instanceBackupService, cfg)
	log.Printf("Instance backup directory: %s", paths.instanceBackupDir)
	instanceBackupService.FailStaleRunning()
	backupScheduler = worker.NewBackupScheduler(instanceBackupService, instanceBackupRepo, settingsRepo)

	deviceBackupScheduler := worker.NewDeviceBackupScheduler(backupService, backupJobRepo, settingsRepo)
	backupService.ResumeBulkBackupRuns(ctx)

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
	pollingDeviceSource := scheduler.NewSavedMapDeviceSource(deviceLinkCache, canvasMapRepo)
	sched := scheduler.NewScheduler(pollingDeviceSource, settingsRepo)
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

	apiSecurity := api.SecurityConfig{
		AllowedOrigins: cfg.AllowedOrigins,
	}
	wsHandler := ws.NewHandler(
		hub,
		pipeline.GetOverviewSnapshot,
		pipeline.GetAlerts,
		pipeline.GetPrometheusStatus,
		ws.WithAllowedOrigins(cfg.AllowedOrigins),
	)
	children := runtimeChildren{deviceService, pipeline}
	if backupScheduler != nil {
		children = append(children, backupScheduler)
	}
	children = append(children, deviceBackupScheduler)

	var server *http.Server
	var restoreShutdownOnce sync.Once
	restoreRestarter := func() {
		restoreShutdownOnce.Do(func() {
			log.Printf("Restore staged successfully; shutting down so the configured supervisor can restart Theia")
			cancel()
			b.stopRuntime(children)

			shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer shutdownCancel()
			if server != nil {
				if err := server.Shutdown(shutdownCtx); err != nil {
					log.Printf("HTTP server shutdown error after restore staging: %v", err)
				}
			}
		})
	}

	router := api.NewRouter(db, deviceService, linkRepo, positionRepo, canvasMapRepo, canvasMapPositionRepo, settingsRepo, snmpProfileRepo, credentialProfileRepo, areaRepo, backupService, vendorRegistry, vendorConfigRepo, pipeline, instanceBackupService, restoreRestarter, cfg.BridgeBinariesDir, pipeline.GetOrBuildOverviewSnapshot, wsHandler, api.WithSecurity(apiSecurity), api.WithAuthService(authService), api.WithBridgeService(bridgeService), api.WithAuditLogRepository(authRepo), api.WithRuntimeEnvironment(cfg.DeploymentEnv))
	metricsHandler := observability.Handler()
	metricsToken := strings.TrimSpace(cfg.MetricsToken)
	server = &http.Server{
		Addr: cfg.ListenAddr,
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/metrics" {
				if !authenticateMetricsRequest(w, r, metricsToken) {
					return
				}
				metricsHandler.ServeHTTP(w, r)
				return
			}
			router.ServeHTTP(w, r)
		}),
	}

	b.handleShutdown(cancel, server, children)

	log.Printf("Theia starting on %s (environment=%s)", cfg.ListenAddr, cfg.DeploymentEnv)
	if err := b.serve(server); err != nil {
		return fmt.Errorf("server error: %w", err)
	}
	log.Println("Server stopped")
	return nil
}

func configureInstanceBackupArchiveLimits(instanceBackupService *service.InstanceBackupService, cfg *runtimeConfig) {
	if instanceBackupService == nil || cfg == nil {
		return
	}
	instanceBackupService.SetRestoreArchiveLimits(service.RestoreArchiveLimits{
		MaxCompressedBytes: cfg.RestoreArchiveLimits.MaxCompressedBytes,
		MaxTotalBytes:      cfg.RestoreArchiveLimits.MaxTotalBytes,
		MaxEntryBytes:      cfg.RestoreArchiveLimits.MaxEntryBytes,
		MaxFileEntries:     cfg.RestoreArchiveLimits.MaxFileEntries,
	})
	instanceBackupService.SetBackupArchiveLimits(service.BackupArchiveLimits{
		MaxTotalBytes:  cfg.InstanceBackupArchiveLimits.MaxTotalBytes,
		MaxEntryBytes:  cfg.InstanceBackupArchiveLimits.MaxEntryBytes,
		MaxFileEntries: cfg.InstanceBackupArchiveLimits.MaxFileEntries,
		MaxDuration:    time.Duration(cfg.InstanceBackupArchiveLimits.MaxDurationSeconds) * time.Second,
	})
}

func configureBackupServiceBulkOperationLimits(backupService *service.BackupService, cfg *runtimeConfig) {
	if backupService == nil || cfg == nil {
		return
	}
	backupService.SetBulkOperationLimits(service.BulkOperationLimits{
		BulkBackupMaxDevices:              cfg.BulkBackupLimits.MaxDevices,
		BulkBackupMaxQueuedJobs:           cfg.BulkBackupLimits.MaxQueuedJobs,
		BulkDownloadMaxDevices:            cfg.BulkDownloadLimits.MaxDevices,
		BulkDownloadMaxFiles:              cfg.BulkDownloadLimits.MaxFiles,
		BulkDownloadMaxBytes:              cfg.BulkDownloadLimits.MaxBytes,
		BulkDownloadMaxConcurrentPerActor: cfg.BulkDownloadLimits.MaxConcurrentPerActor,
		BulkDownloadMaxConcurrentGlobal:   cfg.BulkDownloadLimits.MaxConcurrentGlobal,
	})
}

func minutesToDuration(minutes int) time.Duration {
	if minutes <= 0 {
		return 0
	}
	return time.Duration(minutes) * time.Minute
}

func authenticateMetricsRequest(w http.ResponseWriter, r *http.Request, expectedToken string) bool {
	expectedToken = strings.TrimSpace(expectedToken)
	if expectedToken == "" {
		return true
	}
	if constantTimeStringEqual(metricsBearerToken(r.Header.Get("Authorization")), expectedToken) {
		return true
	}
	w.Header().Set("WWW-Authenticate", `Bearer realm="theia-metrics"`)
	http.Error(w, "metrics authentication required", http.StatusUnauthorized)
	return false
}

func metricsBearerToken(header string) string {
	header = strings.TrimSpace(header)
	if header == "" {
		return ""
	}
	const prefix = "bearer "
	if !strings.HasPrefix(strings.ToLower(header), prefix) {
		return ""
	}
	return strings.TrimSpace(header[len(prefix):])
}

func constantTimeStringEqual(got, want string) bool {
	if got == "" || want == "" {
		return false
	}
	gotHash := sha256.Sum256([]byte(got))
	wantHash := sha256.Sum256([]byte(want))
	return subtle.ConstantTimeCompare(gotHash[:], wantHash[:]) == 1
}

func runtimeDebugSettingsSummary(cfg *runtimeConfig, settingsRepo domain.SettingsRepository) string {
	cfgLogLevel := ""
	cfgListen := ""
	if cfg != nil {
		cfgLogLevel = cfg.LogLevel
		cfgListen = cfg.ListenAddr
	}
	prometheusSetting := runtimeDebugSetting(settingsRepo, domain.SettingPrometheusURL)

	parts := []string{
		"log_level=" + debugSettingValue(cfgLogLevel),
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
