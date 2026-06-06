package main

// This file defines main behavior for database connectivity checks.

import (
	"context"
	"flag"
	"log"
	"os"
	"strings"
	"time"

	"github.com/lollinoo/theia/internal/config"
	"github.com/lollinoo/theia/internal/repository/postgres"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func main() {
	configPath := flag.String("config", "config.yaml", "Path to config file")
	dsnFlag := flag.String("dsn", "", "Database DSN override")
	timeoutFlag := flag.Duration("timeout", 15*time.Second, "Validation timeout")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	dsn := strings.TrimSpace(*dsnFlag)
	if dsn == "" {
		dsn = strings.TrimSpace(cfg.DBDSN)
	}
	if dsn == "" {
		dsn = strings.TrimSpace(os.Getenv("THEIA_DB_DSN"))
	}

	db, err := postgres.OpenPrimaryDB(dsn)
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	postgres.ConfigureDB(db)

	ctx, cancel := context.WithTimeout(context.Background(), *timeoutFlag)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		log.Fatalf("Failed to ping database: %v", err)
	}

	if err := postgres.RunMigrations(db); err != nil {
		log.Fatalf("Failed to run migrations before validation: %v", err)
	}

	if err := postgres.ValidatePostgresPlanChecks(ctx, db, log.Printf); err != nil {
		log.Fatalf("PostgreSQL validation failed: %v", err)
	}

	log.Printf("PostgreSQL validation passed")
}
