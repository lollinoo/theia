package main

import (
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"

	"github.com/azmin/mikrotik-theia/internal/config"
	"github.com/azmin/mikrotik-theia/internal/repository/sqlite"

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

	// Health endpoint
	http.HandleFunc("/api/v1/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"status": "ok",
			"name":   "mikrotik-theia",
		})
	})

	log.Printf("MikroTik Theia starting on %s", cfg.ListenAddr)
	if err := http.ListenAndServe(cfg.ListenAddr, nil); err != nil {
		log.Fatalf("Server error: %v", err)
	}
	fmt.Println()
}
