package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/lollinoo/theia/internal/instance"
)

func TestManagedSecretsSurviveConfigurationReloadAndRejectOverrides(t *testing.T) {
	for _, name := range []string{"THEIA_ENCRYPTION_KEY", "THEIA_ENCRYPTION_KEYS", "THEIA_ENCRYPTION_KEY_ID", "THEIA_SESSION_SECRET", "THEIA_METRICS_TOKEN", "THEIA_DB_DSN"} {
		t.Setenv(name, "")
		t.Setenv(name+"_FILE", "")
	}
	path := filepath.Join(t.TempDir(), "secrets.json")
	state, err := instance.Generate(time.Now())
	if err != nil {
		t.Fatal(err)
	}
	state.DBDSN = "postgres://destination"
	if err := (instance.Store{Path: path}).Create(state); err != nil {
		t.Fatal(err)
	}
	t.Setenv("THEIA_INSTANCE_STATE", path)
	for i := 0; i < 2; i++ {
		cfg, err := Load("/nonexistent-config.yaml")
		if err != nil {
			t.Fatal(err)
		}
		keys, err := cfg.CredentialKeyring()
		if err != nil || keys.ActiveKeyID() != state.ActiveKeyID || cfg.SessionSecret != state.SessionSecret || cfg.DBDSN != state.DBDSN {
			t.Fatalf("managed state not loaded: %v", err)
		}
	}
	t.Setenv("THEIA_ENCRYPTION_KEY", "replacement-key")
	if _, err := Load("/nonexistent-config.yaml"); err == nil {
		t.Fatal("managed state accepted an environment key override")
	}
	t.Setenv("THEIA_ENCRYPTION_KEY", "")
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if _, err := Load("/nonexistent-config.yaml"); err == nil {
		t.Fatal("missing persistent secrets accepted")
	}
}

func TestSecretFilesAndDirectValuesCannotConflict(t *testing.T) {
	file := filepath.Join(t.TempDir(), "secret")
	if err := os.WriteFile(file, []byte("secret-file-value\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("THEIA_SESSION_SECRET", "")
	t.Setenv("THEIA_SESSION_SECRET_FILE", file)
	cfg, err := Load("/nonexistent-config.yaml")
	if err != nil || cfg.SessionSecret != "secret-file-value" {
		t.Fatalf("file secret: %v", err)
	}
	t.Setenv("THEIA_SESSION_SECRET", "direct-value")
	if _, err := Load("/nonexistent-config.yaml"); err == nil {
		t.Fatal("conflicting secrets accepted")
	}
}
