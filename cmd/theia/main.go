package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
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

// restoreMarker is the JSON structure written by ValidateAndStageRestore.
// It contains paths to staged files and their target destinations.
type restoreMarker struct {
	StagedDB         string `json:"staged_db"`
	StagedBackups    string `json:"staged_backups"`
	StagedKnownHosts string `json:"staged_known_hosts"`
	DBPath           string `json:"db_path"`
	DeviceBackupDir  string `json:"device_backup_dir"`
	KnownHostsPath   string `json:"known_hosts_path"`
	Timestamp        string `json:"timestamp"`
}

var newCollectorSNMPClient = func(target string, creds domain.SNMPCredentials, timeout time.Duration, retries int) (collector.SNMPClient, error) {
	return snmp.NewClient(target, creds, timeout, retries)
}

func wirePollRescheduler(deviceService *service.DeviceService, sched *scheduler.Scheduler) {
	deviceService.SetPollRescheduler(sched)
}

// applyPendingRestore checks for a .theia-restore-pending marker file and
// applies the staged restore if found. Must be called BEFORE opening the database.
// Returns true if a restore was applied (caller should log and continue normally).
func applyPendingRestore(dbPath string) bool {
	markerPath := filepath.Join(filepath.Dir(dbPath), ".theia-restore-pending")

	data, err := os.ReadFile(markerPath)
	if err != nil {
		if os.IsNotExist(err) {
			return false // No pending restore — normal startup
		}
		log.Printf("Warning: failed to read restore marker %s: %v", markerPath, err)
		return false
	}

	var marker restoreMarker
	if err := json.Unmarshal(data, &marker); err != nil {
		log.Printf("Error: invalid restore marker JSON: %v", err)
		return false
	}

	log.Printf("Restore marker found (staged at %s), applying restore...", marker.Timestamp)

	// Step 1: Back up current DB as .pre-restore.bak (D-09)
	bakPath := dbPath + ".pre-restore.bak"
	if _, err := os.Stat(dbPath); err == nil {
		// Remove old bak if exists
		os.Remove(bakPath)
		if err := copyFileForRestore(dbPath, bakPath); err != nil {
			log.Fatalf("Restore failed: could not back up current DB to %s: %v", bakPath, err)
		}
		log.Printf("Restore: backed up current DB to %s", bakPath)
	}

	// Step 2: Replace DB with staged DB
	if marker.StagedDB != "" {
		if _, err := os.Stat(marker.StagedDB); err == nil {
			if err := os.Remove(dbPath); err != nil && !os.IsNotExist(err) {
				log.Fatalf("Restore failed: could not remove current DB: %v", err)
			}
			// Also remove WAL and SHM files from current DB
			os.Remove(dbPath + "-wal")
			os.Remove(dbPath + "-shm")
			if err := os.Rename(marker.StagedDB, dbPath); err != nil {
				log.Fatalf("Restore failed: could not move staged DB to %s: %v", dbPath, err)
			}
			log.Printf("Restore: replaced DB at %s", dbPath)
		}
	}

	// Step 3: Replace device backup directory with staged backups
	if marker.StagedBackups != "" && marker.DeviceBackupDir != "" {
		if info, err := os.Stat(marker.StagedBackups); err == nil && info.IsDir() {
			if err := replaceDirForRestore(marker.StagedBackups, marker.DeviceBackupDir); err != nil {
				log.Printf("Warning: failed to replace device backups at %s: %v", marker.DeviceBackupDir, err)
				return false
			}
			log.Printf("Restore: replaced device backups at %s", marker.DeviceBackupDir)
		}
	}

	// Step 4: Replace known_hosts with staged version
	if marker.StagedKnownHosts != "" && marker.KnownHostsPath != "" {
		if _, err := os.Stat(marker.StagedKnownHosts); err == nil {
			if err := replaceFileForRestore(marker.StagedKnownHosts, marker.KnownHostsPath); err != nil {
				log.Printf("Warning: failed to replace known_hosts at %s: %v", marker.KnownHostsPath, err)
				return false
			}
			log.Printf("Restore: replaced known_hosts at %s", marker.KnownHostsPath)
		}
	}

	// Step 5: Clean up marker and staging directory
	os.Remove(markerPath)
	stagingDir := filepath.Dir(marker.StagedDB)
	if stagingDir != "" && stagingDir != "." {
		os.RemoveAll(stagingDir)
	}
	log.Printf("Restore: cleanup complete, marker and staging removed")

	return true
}

// copyFileForRestore copies a single file from src to dst. Used during restore to
// preserve the original DB as a .pre-restore.bak and as fallback for cross-device moves.
func copyFileForRestore(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Close()
}

func replaceDirForRestore(src, dst string) error {
	tmpPath := dst + ".restore-tmp"
	backupPath := dst + ".restore-old"

	if err := os.RemoveAll(tmpPath); err != nil {
		return err
	}
	if err := os.RemoveAll(backupPath); err != nil {
		return err
	}
	if err := copyDirForRestore(src, tmpPath); err != nil {
		return err
	}

	movedExisting := false
	if _, err := os.Stat(dst); err == nil {
		if err := os.Rename(dst, backupPath); err != nil {
			_ = os.RemoveAll(tmpPath)
			return err
		}
		movedExisting = true
	} else if !os.IsNotExist(err) {
		_ = os.RemoveAll(tmpPath)
		return err
	}

	if err := os.Rename(tmpPath, dst); err != nil {
		if movedExisting {
			if restoreErr := os.Rename(backupPath, dst); restoreErr != nil {
				return fmt.Errorf("activate staged restore dir: %w (restore previous dir: %v)", err, restoreErr)
			}
		}
		_ = os.RemoveAll(tmpPath)
		return err
	}

	if movedExisting {
		if err := os.RemoveAll(backupPath); err != nil {
			log.Printf("Warning: failed to remove restore backup dir %s: %v", backupPath, err)
		}
	}

	return nil
}

func replaceFileForRestore(src, dst string) error {
	tmpPath := dst + ".restore-tmp"
	backupPath := dst + ".restore-old"

	if err := os.Remove(tmpPath); err != nil && !os.IsNotExist(err) {
		return err
	}
	if err := os.Remove(backupPath); err != nil && !os.IsNotExist(err) {
		return err
	}
	if err := copyFileForRestore(src, tmpPath); err != nil {
		return err
	}

	movedExisting := false
	if _, err := os.Stat(dst); err == nil {
		if err := os.Rename(dst, backupPath); err != nil {
			_ = os.Remove(tmpPath)
			return err
		}
		movedExisting = true
	} else if !os.IsNotExist(err) {
		_ = os.Remove(tmpPath)
		return err
	}

	if err := os.Rename(tmpPath, dst); err != nil {
		if movedExisting {
			if restoreErr := os.Rename(backupPath, dst); restoreErr != nil {
				return fmt.Errorf("activate staged restore file: %w (restore previous file: %v)", err, restoreErr)
			}
		}
		_ = os.Remove(tmpPath)
		return err
	}

	if movedExisting {
		if err := os.Remove(backupPath); err != nil && !os.IsNotExist(err) {
			log.Printf("Warning: failed to remove restore backup file %s: %v", backupPath, err)
		}
	}

	return nil
}

// copyDirForRestore recursively copies a directory from src to dst.
// Used as fallback when os.Rename fails (cross-device move).
func copyDirForRestore(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, 0755)
		}
		return copyFileForRestore(path, target)
	})
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

	appDataDir := cfg.DataDir
	if appDataDir == "" {
		appDataDir = "./data"
	}
	if cfg.DBPath != "" {
		appDataDir = filepath.Dir(cfg.DBPath)
	}
	if cfg.DataDir != "" {
		appDataDir = cfg.DataDir
	}

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
		if applyPendingRestore(cfg.DBPath) {
			log.Println("Restore applied successfully, continuing with normal startup")
		}
	}

	if err := os.MkdirAll(appDataDir, 0755); err != nil {
		log.Fatalf("Failed to create application data directory %s: %v", appDataDir, err)
	}

	db, dialect, err := sqlite.OpenPrimaryDB(cfg.DBDriver, cfg.DBPath, cfg.DBDSN)
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}
	sqlite.ConfigureDB(db)
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
	var yamlRegistry *vendor.Registry
	if envVendors := os.Getenv("THEIA_VENDORS_DIR"); envVendors != "" {
		yamlRegistry, err = vendor.LoadRegistryFromYAML(envVendors)
		if err != nil {
			log.Fatalf("Failed to load vendor registry from %s: %v", envVendors, err)
		}
		log.Printf("Vendor YAML loaded: %d vendors from %s", yamlRegistry.VendorCount(), envVendors)
	} else {
		yamlRegistry, err = vendor.LoadRegistryFromEmbedded()
		if err != nil {
			log.Fatalf("Failed to load embedded vendor registry: %v", err)
		}
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
	backupDir := os.Getenv("THEIA_BACKUP_DIR")
	if backupDir == "" {
		backupDir = filepath.Join(appDataDir, "backups")
	}
	if err := os.MkdirAll(backupDir, 0755); err != nil {
		log.Fatalf("Failed to create backup directory %s: %v", backupDir, err)
	}

	// Initialize SSH known hosts store for host key verification
	knownHostsPath := filepath.Join(appDataDir, "known_hosts")
	knownHostsStore, err := ssh.NewKnownHostsStore(knownHostsPath)
	if err != nil {
		log.Fatalf("Failed to initialize SSH known hosts store: %v", err)
	}
	log.Printf("SSH known hosts store: %s", knownHostsPath)

	backupService := service.NewBackupService(backupJobRepo, backupFileRepo, credentialProfileRepo, deviceRepo, settingsRepo, vendorRegistry, sshDialer, encryptionKey, backupDir, knownHostsStore.HostKeyCallback())

	var instanceBackupService *service.InstanceBackupService
	var backupScheduler *worker.BackupScheduler
	if dialect == sqlite.DialectSQLite {
		instanceBackupRepo := sqlite.NewInstanceBackupRepo(db)
		instanceBackupDir := os.Getenv("THEIA_INSTANCE_BACKUP_DIR")
		if instanceBackupDir == "" {
			instanceBackupDir = filepath.Join(appDataDir, "instance-backups")
		}
		if err := os.MkdirAll(instanceBackupDir, 0755); err != nil {
			log.Fatalf("Failed to create instance backup directory %s: %v", instanceBackupDir, err)
		}
		instanceBackupService = service.NewInstanceBackupService(
			db,
			instanceBackupRepo,
			settingsRepo,
			instanceBackupDir,
			backupDir,
			knownHostsPath,
			cfg.DBPath,
			encryptionKey,
		)
		log.Printf("Instance backup directory: %s", instanceBackupDir)
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
	if prometheusURL != "" {
		promClient = metrics.NewPromClient(prometheusURL, http.DefaultClient)
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
	pipeline.Start(ctx)
	if backupScheduler != nil {
		backupScheduler.Start(ctx)
	}
	deviceBackupScheduler.Start(ctx)

	wsHandler := ws.NewHandler(hub, pipeline.GetSnapshot, pipeline.GetPrometheusStatus)

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

// seedVendorConfigs seeds vendor configs into the DB from YAML if not already present.
func seedVendorConfigs(yamlRegistry *vendor.Registry, repo *sqlite.VendorConfigRepo) {
	configs, err := yamlRegistry.ExportAllConfigs()
	if err != nil {
		log.Printf("Warning: failed to export YAML configs for seeding: %v", err)
		return
	}

	for name, configJSON := range configs {
		existing, err := repo.GetByName(name)
		if err != nil {
			log.Printf("Warning: failed to check vendor %s in DB: %v", name, err)
			continue
		}
		if existing != nil {
			continue // already seeded
		}

		displayName := yamlRegistry.GetDisplayName(name)
		if name == "default" {
			displayName = "Generic / Default"
		}

		record := &domain.VendorConfigRecord{
			Name:        name,
			DisplayName: displayName,
			ConfigJSON:  string(configJSON),
		}
		if err := repo.Upsert(record); err != nil {
			log.Printf("Warning: failed to seed vendor %s: %v", name, err)
		} else {
			log.Printf("Seeded vendor config: %s", name)
		}
	}
}

// loadRegistryFromDB builds a vendor registry from DB records.
func loadRegistryFromDB(repo *sqlite.VendorConfigRepo) (*vendor.Registry, error) {
	records, err := repo.GetAll()
	if err != nil {
		return nil, err
	}
	if len(records) == 0 {
		return nil, nil
	}

	var dbRecords []vendor.DBVendorRecord
	for _, rec := range records {
		// Validate JSON is parseable
		var raw json.RawMessage
		if err := json.Unmarshal([]byte(rec.ConfigJSON), &raw); err != nil {
			log.Printf("Warning: invalid vendor config JSON for %s, skipping: %v", rec.Name, err)
			continue
		}
		dbRecords = append(dbRecords, vendor.DBVendorRecord{
			Name:       rec.Name,
			ConfigJSON: rec.ConfigJSON,
		})
	}

	if len(dbRecords) == 0 {
		log.Printf("Warning: all DB vendor records failed JSON validation, falling back to YAML registry")
		return nil, nil
	}

	return vendor.LoadRegistryFromDB(dbRecords)
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
	return func(target string, creds domain.SNMPCredentials) (*snmp.DiscoveryResult, error) {
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

		return snmp.DiscoverDevice(client, vendorRegistry)
	}
}
