package config

import (
	"bytes"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadIgnoresDeprecatedBulkBackupQuotas(t *testing.T) {
	for _, source := range []string{"yaml", "environment", "both"} {
		t.Run(source, func(t *testing.T) {
			t.Setenv("THEIA_BULK_BACKUP_MAX_DEVICES", "")
			t.Setenv("THEIA_BULK_BACKUP_MAX_QUEUED_JOBS", "")
			path := filepath.Join(t.TempDir(), "config.yaml")
			content := "bulk_download_limits:\n  max_devices: 7\n"
			if source != "environment" {
				content += "bulk_backup_limits:\n  max_devices: 0\n  max_queued_jobs: obsolete\n"
			}
			if source != "yaml" {
				t.Setenv("THEIA_BULK_BACKUP_MAX_DEVICES", "not-an-integer")
				t.Setenv("THEIA_BULK_BACKUP_MAX_QUEUED_JOBS", "-1")
			}
			if err := os.WriteFile(path, []byte(content), 0600); err != nil {
				t.Fatal(err)
			}
			var output bytes.Buffer
			previous := log.Writer()
			log.SetOutput(&output)
			t.Cleanup(func() { log.SetOutput(previous) })
			cfg, err := Load(path)
			if err != nil {
				t.Fatalf("deprecated quotas blocked startup: %v", err)
			}
			if cfg.BulkDownloadLimits.MaxDevices != 7 {
				t.Fatalf("active download limit = %d, want 7", cfg.BulkDownloadLimits.MaxDevices)
			}
			if strings.Count(output.String(), "deprecated and ignored") != 1 {
				t.Fatalf("expected one deprecation warning: %s", output.String())
			}
		})
	}
}
