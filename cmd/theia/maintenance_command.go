package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/lollinoo/theia/internal/config"
	"github.com/lollinoo/theia/internal/instance"
	"github.com/lollinoo/theia/internal/service"
)

func runMaintenanceCommand(args []string, output io.Writer) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: theia maintenance migrate|backup|restore|status|resume|verify [-config config.yaml]")
	}
	flags := flag.NewFlagSet("theia maintenance "+args[0], flag.ContinueOnError)
	flags.SetOutput(output)
	configPath := flags.String("config", "config.yaml", "Configuration file")
	statePath := flags.String("state", os.Getenv("THEIA_INSTANCE_STATE"), "Persistent instance state for status")
	archive := flags.String("archive", "", "Encrypted instance backup archive")
	recovery := flags.String("recovery-file", "", "Operator recovery file, read transiently")
	forceRotation := flags.Bool("rotate-credentials", false, "Rotate the active credential key during migration even when not due")
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected maintenance arguments")
	}
	if args[0] == "status" {
		if *statePath == "" {
			return fmt.Errorf("-state or THEIA_INSTANCE_STATE is required")
		}
		op, err := (&service.Maintenance{StatePath: *statePath}).Status()
		if err != nil {
			return err
		}
		return json.NewEncoder(output).Encode(op)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, 30*time.Minute)
	defer cancel()
	if args[0] == "verify" {
		identities, err := instance.ReadRecoveryFile(*recovery)
		if err != nil {
			return err
		}
		staging, err := os.MkdirTemp("", "theia-verify-*")
		if err != nil {
			return err
		}
		defer os.RemoveAll(staging)
		state, err := service.ExtractProtectedInstanceBackup(ctx, *archive, staging, service.RestoreArchiveLimits{MaxCompressedBytes: 2 << 30, MaxTotalBytes: 2 << 30, MaxEntryBytes: 1 << 30, MaxFileEntries: 50000}, identities...)
		if err != nil {
			return err
		}
		keys, err := state.Keyring()
		if err != nil {
			return err
		}
		if err := service.VerifyIsolatedPostgresDump(ctx, filepath.Join(staging, "database.dump"), keys); err != nil {
			return err
		}
		_, err = fmt.Fprintln(output, "Encrypted archive and isolated database restore verified.")
		return err
	}
	cfg, err := config.Load(*configPath)
	if err != nil {
		return err
	}
	if cfg.InstanceStatePath == "" {
		return fmt.Errorf("maintenance commands require managed instance state; import the original deployment first")
	}
	paths := resolveRuntimePaths(cfg)
	m := &service.Maintenance{StatePath: cfg.InstanceStatePath, DataDir: paths.appDataDir, BackupDir: paths.instanceBackupDir, DeviceBackupDir: paths.backupDir, KnownHostsPath: paths.knownHostsPath, DBDSN: cfg.DBDSN, ReleaseTag: os.Getenv("THEIA_RELEASE_TAG")}
	switch args[0] {
	case "migrate":
		return m.Migrate(ctx, *forceRotation)
	case "resume":
		return m.Resume(ctx)
	case "rollback":
		return m.RollbackBeforeReopen(ctx)
	case "backup":
		backup, err := m.Backup(ctx)
		if err != nil {
			return err
		}
		return json.NewEncoder(output).Encode(backup)
	case "restore":
		return m.Restore(ctx, *archive, *recovery)
	default:
		return fmt.Errorf("unknown maintenance command %q", args[0])
	}
}
