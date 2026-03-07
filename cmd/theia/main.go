package main

import (
	"context"
	"database/sql"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"github.com/azmin/mikrotik-theia/internal/api"
	"github.com/azmin/mikrotik-theia/internal/config"
	"github.com/azmin/mikrotik-theia/internal/domain"
	"github.com/azmin/mikrotik-theia/internal/metrics"
	"github.com/azmin/mikrotik-theia/internal/repository/sqlite"
	"github.com/azmin/mikrotik-theia/internal/service"
	"github.com/azmin/mikrotik-theia/internal/snmp"
	"github.com/azmin/mikrotik-theia/internal/worker"
	"github.com/azmin/mikrotik-theia/internal/ws"

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

	// Run migrations
	if err := sqlite.RunMigrations(db); err != nil {
		log.Fatalf("Failed to run database migrations: %v", err)
	}
	log.Println("Database migrations completed")

	// Create repositories
	deviceRepo := sqlite.NewDeviceRepo(db)
	linkRepo := sqlite.NewLinkRepo(db)
	positionRepo := sqlite.NewPositionRepo(db)
	settingsRepo := sqlite.NewSettingsRepo(db)

	// Create SNMP discovery function (real gosnmp clients)
	discoverFunc := newSNMPDiscoverFunc(settingsRepo)

	// Create service layer
	deviceService := service.NewDeviceService(deviceRepo, linkRepo, settingsRepo, discoverFunc)

	// Create and start background poller
	poller := worker.NewPoller(deviceService, settingsRepo)
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

	collector := worker.NewMetricsCollector(promClient, hub, deviceRepo, linkRepo, settingsRepo)
	collector.Start(ctx)

	wsHandler := ws.NewHandler(hub, collector.GetSnapshot)

	// Create HTTP router with all /api/v1/ routes
	router := api.NewRouter(db, deviceService, linkRepo, positionRepo, settingsRepo, poller, wsHandler)

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

	log.Printf("MikroTik Theia starting on %s", cfg.ListenAddr)
	if err := server.ListenAndServe(); err != http.ErrServerClosed {
		log.Fatalf("Server error: %v", err)
	}
	log.Println("Server stopped")
}

func ensurePrometheusURL(settingsRepo *sqlite.SettingsRepo) (string, error) {
	const defaultPrometheusURL = "http://prometheus:9090"

	prometheusURL, err := settingsRepo.Get(domain.SettingPrometheusURL)
	if err == nil && prometheusURL != "" {
		return prometheusURL, nil
	}

	if err := settingsRepo.Set(domain.SettingPrometheusURL, defaultPrometheusURL); err != nil {
		return "", err
	}

	return defaultPrometheusURL, nil
}

// newSNMPDiscoverFunc creates a DiscoverFunc that uses real gosnmp clients.
// It reads SNMP timeout and retries from the settings repository.
func newSNMPDiscoverFunc(settingsRepo domain.SettingsRepository) service.DiscoverFunc {
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

		return snmp.DiscoverDevice(client)
	}
}
