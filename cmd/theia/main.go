package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"github.com/lollinoo/theia/internal/api"
	"github.com/lollinoo/theia/internal/cache"
	"github.com/lollinoo/theia/internal/config"
	"github.com/lollinoo/theia/internal/crypto"
	"github.com/lollinoo/theia/internal/domain"
	"github.com/lollinoo/theia/internal/metrics"
	"github.com/lollinoo/theia/internal/repository/sqlite"
	"github.com/lollinoo/theia/internal/service"
	"github.com/lollinoo/theia/internal/snmp"
	"github.com/lollinoo/theia/internal/ssh"
	"github.com/lollinoo/theia/internal/vendor"
	"github.com/lollinoo/theia/internal/worker"
	"github.com/lollinoo/theia/internal/ws"

	_ "github.com/mattn/go-sqlite3"
)

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

	log.Printf("Config loaded: listen=%s db=%s log_level=%s", cfg.ListenAddr, cfg.DBPath, cfg.LogLevel)

	// Ensure the database directory exists
	dbDir := filepath.Dir(cfg.DBPath)
	if err := os.MkdirAll(dbDir, 0755); err != nil {
		log.Fatalf("Failed to create database directory %s: %v", dbDir, err)
	}

	// Open SQLite database
	db, err := sql.Open("sqlite3", cfg.DBPath+"?_journal_mode=WAL&_busy_timeout=5000&_foreign_keys=on")
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

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

	// Load vendor registry from YAML (used for seeding)
	vendorsDir := filepath.Join(filepath.Dir(cfgPath), "vendors")
	if envVendors := os.Getenv("THEIA_VENDORS_DIR"); envVendors != "" {
		vendorsDir = envVendors
	}
	yamlRegistry, err := vendor.LoadRegistryFromYAML(vendorsDir)
	if err != nil {
		log.Fatalf("Failed to load vendor registry from %s: %v", vendorsDir, err)
	}
	log.Printf("Vendor YAML loaded: %d vendors from %s", yamlRegistry.VendorCount(), vendorsDir)

	// Create vendor config repo and seed DB from YAML
	vendorConfigRepo := sqlite.NewVendorConfigRepo(db)
	seedVendorConfigs(yamlRegistry, vendorConfigRepo)

	// Build live registry from DB
	vendorRegistry, err := loadRegistryFromDB(vendorConfigRepo)
	if err != nil {
		log.Printf("Warning: failed to load registry from DB, falling back to YAML: %v", err)
		vendorRegistry = yamlRegistry
	}
	log.Printf("Vendor registry loaded: %d vendors", vendorRegistry.VendorCount())

	// Create shared invalidation channel for device/link cache
	cacheInvalidate := make(chan struct{}, 1)

	// Create repositories
	deviceRepo := sqlite.NewDeviceRepo(db, encryptionKey, cacheInvalidate)
	linkRepo := sqlite.NewLinkRepo(db, cacheInvalidate)
	deviceLinkCache := cache.NewDeviceLinkCache(deviceRepo, linkRepo, cacheInvalidate)
	positionRepo := sqlite.NewPositionRepo(db)
	settingsRepo := sqlite.NewSettingsRepo(db)
	snmpProfileRepo := sqlite.NewSNMPProfileRepo(db, encryptionKey)
	sshProfileRepo := sqlite.NewSSHProfileRepo(db)
	backupJobRepo := sqlite.NewBackupJobRepo(db)
	backupFileRepo := sqlite.NewBackupFileRepo(db)

	// Create SNMP discovery function (real gosnmp clients)
	discoverFunc := newSNMPDiscoverFunc(settingsRepo, vendorRegistry)

	// Create service layer
	deviceService := service.NewDeviceService(deviceRepo, linkRepo, settingsRepo, discoverFunc)

	// Create backup service
	sshDialer := &ssh.DefaultDialer{}
	backupDir := os.Getenv("THEIA_BACKUP_DIR")
	if backupDir == "" {
		backupDir = "/app/data/backups"
	}
	if err := os.MkdirAll(backupDir, 0755); err != nil {
		log.Fatalf("Failed to create backup directory %s: %v", backupDir, err)
	}

	// Initialize SSH known hosts store for host key verification
	knownHostsPath := filepath.Join(filepath.Dir(cfg.DBPath), "known_hosts")
	knownHostsStore, err := ssh.NewKnownHostsStore(knownHostsPath)
	if err != nil {
		log.Fatalf("Failed to initialize SSH known hosts store: %v", err)
	}
	log.Printf("SSH known hosts store: %s", knownHostsPath)

	backupService := service.NewBackupService(backupJobRepo, backupFileRepo, sshProfileRepo, deviceRepo, settingsRepo, vendorRegistry, sshDialer, encryptionKey, backupDir, knownHostsStore.HostKeyCallback())

	// Create and start background poller
	poller := worker.NewPoller(deviceService, settingsRepo, deviceLinkCache)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	poller.Start(ctx)

	prometheusURL, err := ensurePrometheusURL(settingsRepo)
	if err != nil {
		log.Fatalf("Failed to initialize Prometheus URL: %v", err)
	}

	promClient := metrics.NewPromClient(prometheusURL, http.DefaultClient)
	hub := ws.NewHub()
	go hub.Run()

	snmpPollFunc := newSNMPMetricsPollFunc(settingsRepo, vendorRegistry)
	collector := worker.NewMetricsCollector(promClient, hub, deviceLinkCache, deviceRepo, settingsRepo, vendorRegistry, snmpPollFunc)
	collector.Start(ctx)

	wsHandler := ws.NewHandler(hub, collector.GetSnapshot, collector.IsPromAvailable)

	// Create HTTP router with all /api/v1/ routes
	router := api.NewRouter(db, deviceService, linkRepo, positionRepo, settingsRepo, snmpProfileRepo, sshProfileRepo, backupService, vendorRegistry, vendorConfigRepo, poller, wsHandler)

	// Create HTTP server
	server := &http.Server{
		Addr:    cfg.ListenAddr,
		Handler: router,
	}

	// Graceful shutdown on SIGINT/SIGTERM
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-sigCh
		log.Printf("Received signal %s, shutting down...", sig)

		// Stop the poller first
		poller.Stop()
		collector.Stop()

		// Shutdown HTTP server with timeout
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer shutdownCancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			log.Printf("HTTP server shutdown error: %v", err)
		}
	}()

	log.Printf("Theia starting on %s", cfg.ListenAddr)
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

	return vendor.LoadRegistryFromDB(dbRecords)
}

func ensurePrometheusURL(settingsRepo *sqlite.SettingsRepo) (string, error) {
	const defaultPrometheusURL = "http://localhost:9090"

	prometheusURL, err := settingsRepo.Get(domain.SettingPrometheusURL)
	if err == nil && prometheusURL != "" {
		return prometheusURL, nil
	}

	if err := settingsRepo.Set(domain.SettingPrometheusURL, defaultPrometheusURL); err != nil {
		return "", err
	}

	return defaultPrometheusURL, nil
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

		snmpCfg := vendorRegistry.ResolveSNMPConfig(vendorName)
		cpu, mem, uptime, temp := snmp.PollDeviceMetrics(client, snmpCfg)
		return domain.DeviceMetrics{
			CPUPercent:  cpu,
			MemPercent:  mem,
			UptimeSecs:  uptime,
			TempCelsius: temp,
		}, nil
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
