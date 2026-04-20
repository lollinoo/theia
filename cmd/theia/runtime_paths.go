package main

import (
	"os"
	"path/filepath"

	"github.com/lollinoo/theia/internal/config"
)

type runtimePaths struct {
	appDataDir        string
	backupDir         string
	knownHostsPath    string
	instanceBackupDir string
}

func resolveRuntimePaths(cfg *config.Config) runtimePaths {
	appDataDir := "./data"
	if cfg != nil {
		if cfg.DataDir != "" {
			appDataDir = cfg.DataDir
		}
		if cfg.DBPath != "" {
			appDataDir = filepath.Dir(cfg.DBPath)
		}
		if cfg.DataDir != "" {
			appDataDir = cfg.DataDir
		}
	}

	backupDir := os.Getenv("THEIA_BACKUP_DIR")
	if backupDir == "" {
		backupDir = filepath.Join(appDataDir, "backups")
	}

	instanceBackupDir := os.Getenv("THEIA_INSTANCE_BACKUP_DIR")
	if instanceBackupDir == "" {
		instanceBackupDir = filepath.Join(appDataDir, "instance-backups")
	}

	return runtimePaths{
		appDataDir:        appDataDir,
		backupDir:         backupDir,
		knownHostsPath:    filepath.Join(appDataDir, "known_hosts"),
		instanceBackupDir: instanceBackupDir,
	}
}
