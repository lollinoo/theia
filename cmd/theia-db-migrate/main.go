package main

import (
	"flag"
	"log"
	"os"
	"strings"

	"github.com/lollinoo/theia/internal/config"
	"github.com/lollinoo/theia/internal/repository/sqlite"

	_ "github.com/jackc/pgx/v5/stdlib"
	_ "github.com/mattn/go-sqlite3"
)

func main() {
	configPath := flag.String("config", "config.yaml", "Path to config file used as fallback for the source sqlite path")
	sourcePath := flag.String("source-sqlite", "", "Path to the source SQLite database file")
	targetDSN := flag.String("target-dsn", "", "PostgreSQL DSN for the target database")
	truncateTarget := flag.Bool("truncate-target", false, "Delete existing target rows before importing")
	batchSize := flag.Int("batch-size", 250, "Number of rows per insert batch")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	source := strings.TrimSpace(*sourcePath)
	if source == "" {
		source = strings.TrimSpace(cfg.DBPath)
	}

	target := strings.TrimSpace(*targetDSN)
	if target == "" {
		target = strings.TrimSpace(cfg.DBDSN)
	}
	if target == "" {
		target = strings.TrimSpace(os.Getenv("THEIA_DB_DSN"))
	}

	log.Printf("Starting SQLite -> PostgreSQL migration: source=%s truncate_target=%v batch_size=%d", source, *truncateTarget, *batchSize)

	err = sqlite.MigrateSQLiteToPostgres(source, target, sqlite.CopyOptions{
		TruncateTarget: *truncateTarget,
		BatchSize:      *batchSize,
		Logf:           log.Printf,
	})
	if err != nil {
		log.Fatalf("Migration failed: %v", err)
	}

	log.Printf("SQLite -> PostgreSQL migration completed successfully")
}
