package main

import (
	"context"
	"flag"
	"log"
	"os"
	"strings"
	"time"

	"github.com/lollinoo/theia/internal/config"
	"github.com/lollinoo/theia/internal/repository/sqlite"

	_ "github.com/jackc/pgx/v5/stdlib"
	_ "github.com/mattn/go-sqlite3"
)

func main() {
	configPath := flag.String("config", "config.yaml", "Path to config file")
	driverFlag := flag.String("driver", "", "Database driver override (postgres or sqlite)")
	dsnFlag := flag.String("dsn", "", "Database DSN override")
	timeoutFlag := flag.Duration("timeout", 15*time.Second, "Validation timeout")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	driver := strings.TrimSpace(*driverFlag)
	if driver == "" {
		driver = strings.TrimSpace(cfg.DBDriver)
	}
	dsn := strings.TrimSpace(*dsnFlag)
	if dsn == "" {
		dsn = strings.TrimSpace(cfg.DBDSN)
	}
	if dsn == "" {
		dsn = strings.TrimSpace(os.Getenv("THEIA_DB_DSN"))
	}

	db, dialect, err := sqlite.OpenPrimaryDB(driver, cfg.DBPath, dsn)
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	if dialect != sqlite.DialectPostgres {
		log.Fatalf("theia-db-check only validates PostgreSQL production plans; got %s", dialect)
	}

	sqlite.ConfigureDB(db)

	ctx, cancel := context.WithTimeout(context.Background(), *timeoutFlag)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		log.Fatalf("Failed to ping database: %v", err)
	}

	if err := sqlite.RunMigrations(db); err != nil {
		log.Fatalf("Failed to run migrations before validation: %v", err)
	}

	if err := sqlite.ValidatePostgresPlanChecks(ctx, db, log.Printf); err != nil {
		log.Fatalf("PostgreSQL validation failed: %v", err)
	}

	log.Printf("PostgreSQL validation passed")
}
