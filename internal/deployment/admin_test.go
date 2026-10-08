package deployment

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/domain"
	"github.com/lollinoo/theia/internal/instance"
	"github.com/lollinoo/theia/internal/service"
	"gopkg.in/yaml.v3"
)

func testConfig() Config {
	return Config{Version: 1, Project: "theia-test", Release: "v1.8.0", BackendImage: "test-backend:v1.8.0", FrontendImage: "test-frontend:v1.8.0", Hostname: "localhost", TLSMode: "internal", HTTPPort: 4080, HTTPSPort: 4443, BindAddress: "127.0.0.1", BundledPostgres: true, PostgresMajor: 18}
}

func TestInstallOrdersMaintenanceBeforeHTTPAndNeverRegenerates(t *testing.T) {
	var commands []string
	var output bytes.Buffer
	a := &Admin{Dir: t.TempDir(), Output: &output, ReadyCheck: func(context.Context, Config) error { return nil }, Runner: func(_ context.Context, _ string, args []string, _ io.Writer) error {
		commands = append(commands, strings.Join(args, " "))
		return nil
	}}
	if err := a.Install(context.Background(), InstallOptions{Config: testConfig()}); err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(commands, "\n")
	if strings.Index(joined, "maintenance migrate") > strings.Index(joined, "120 backend frontend") || !strings.Contains(joined, "maintenance migrate") {
		t.Fatal("HTTP preceded maintenance")
	}
	s, err := (instance.Store{Path: StatePath(a.Dir)}).Load()
	if err != nil {
		t.Fatal(err)
	}
	compose, err := os.ReadFile(filepath.Join(a.Dir, "compose.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{s.DatabasePassword, s.SessionSecret, s.MetricsToken, s.CredentialKeys[s.ActiveKeyID].Secret} {
		if strings.Contains(string(compose), secret) {
			t.Fatal("generated Compose exposes a private secret")
		}
	}
	before, _ := os.ReadFile(StatePath(a.Dir))
	if err := a.Install(context.Background(), InstallOptions{Config: testConfig()}); err == nil {
		t.Fatal("existing instance was reinstalled")
	}
	after, _ := os.ReadFile(StatePath(a.Dir))
	if !bytes.Equal(before, after) {
		t.Fatal("failed reinstall replaced secrets")
	}
	if !strings.Contains(output.String(), "/activate#token=") {
		t.Fatal("activation link missing")
	}
}

func TestInstallFailureLeavesSecretsAndDoesNotStartHTTP(t *testing.T) {
	var commands []string
	a := &Admin{Dir: t.TempDir(), Output: io.Discard, ReadyCheck: func(context.Context, Config) error { return nil }, Runner: func(_ context.Context, _ string, args []string, _ io.Writer) error {
		command := strings.Join(args, " ")
		commands = append(commands, command)
		if strings.Contains(command, "maintenance migrate") {
			return fmt.Errorf("injected migration failure")
		}
		return nil
	}}
	if err := a.Install(context.Background(), InstallOptions{Config: testConfig()}); err == nil {
		t.Fatal("failure ignored")
	}
	if _, err := (instance.Store{Path: StatePath(a.Dir)}).Load(); err != nil {
		t.Fatal("secrets lost on failure", err)
	}
	if strings.Contains(strings.Join(commands, "\n"), "120 backend frontend") {
		t.Fatal("HTTP started after failed migration")
	}
}

func TestComposeHasOnlyFrontendPortsAndPersistentSecretFiles(t *testing.T) {
	c := testConfig()
	doc, err := RenderCompose(t.TempDir(), c)
	if err != nil {
		t.Fatal(err)
	}
	var parsed map[string]any
	if err := yaml.Unmarshal(doc, &parsed); err != nil {
		t.Fatal(err)
	}
	services := parsed["services"].(map[string]any)
	for _, name := range []string{"postgres", "backend"} {
		if _, exists := services[name].(map[string]any)["ports"]; exists {
			t.Fatalf("%s was published", name)
		}
	}
	if _, exists := parsed["secrets"]; !exists {
		t.Fatal("password file was not configured")
	}
	c.BundledPostgres = false
	doc, err = RenderCompose(t.TempDir(), c)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(doc), "postgres:") || strings.Contains(string(doc), "postgres_password") {
		t.Fatal("external deployment contains bundled database")
	}
}

func TestUpgradeRecoversOriginalReleaseAfterMigrationRollback(t *testing.T) {
	dir := t.TempDir()
	original := testConfig()
	if err := SaveConfig(dir, original); err != nil {
		t.Fatal(err)
	}
	if err := WriteCompose(dir, original); err != nil {
		t.Fatal(err)
	}
	a := &Admin{Dir: dir, Output: io.Discard, ReadyCheck: func(context.Context, Config) error { return nil }, Runner: func(_ context.Context, _ string, args []string, _ io.Writer) error {
		if strings.Contains(strings.Join(args, " "), "maintenance migrate") {
			data, _ := json.Marshal(&service.MaintenanceOperation{ID: "a55c7aaf-0f9d-4aeb-8df1-279c0b2b5466", Phase: "rolled_back", VerifiedReleaseTag: original.Release})
			if err := instance.WritePrivateFile(filepath.Join(StatePath(dir)+".operations", "current.json"), data); err != nil {
				return err
			}
			return fmt.Errorf("injected rollback")
		}
		return nil
	}}
	if err := a.Upgrade(context.Background(), "v1.9.0", "test-backend:v1.9.0", "test-frontend:v1.9.0"); err == nil {
		t.Fatal("failed upgrade was reported as success")
	}
	restored, err := LoadConfig(dir)
	if err != nil {
		t.Fatal(err)
	}
	if restored.Release != original.Release || restored.BackendImage != original.BackendImage {
		t.Fatal("original release was not restored")
	}
	j, err := a.loadUpgrade()
	if err != nil || j.Phase != "rolled_back" {
		t.Fatal("rollback journal was not completed", err)
	}
}

func TestConfigRejectsMutableTagsAndShellLikeInput(t *testing.T) {
	for _, value := range []string{"test:latest", "test:master", "test:tag\nmalicious", "test:$(id)"} {
		c := testConfig()
		c.BackendImage = value
		if c.Validate() == nil {
			t.Fatalf("invalid image accepted: %q", value)
		}
	}
}

func TestBackupExportVerifiesBytesAndPreservesExistingFiles(t *testing.T) {
	for _, corrupt := range []bool{false, true} {
		t.Run(fmt.Sprint("corrupt=", corrupt), func(t *testing.T) {
			dir := t.TempDir()
			if err := SaveConfig(dir, testConfig()); err != nil {
				t.Fatal(err)
			}
			body := []byte("verified-encrypted-archive")
			hash := sha256.Sum256(body)
			backup := domain.InstanceBackup{ID: uuid.New(), FileName: "instance.tar.gz.age", Status: domain.InstanceBackupStatusSuccess, SizeBytes: int64(len(body)), SHA256: hex.EncodeToString(hash[:])}
			a := &Admin{Dir: dir, Output: io.Discard, ReadyCheck: func(context.Context, Config) error { return nil }, Runner: func(_ context.Context, _ string, args []string, out io.Writer) error {
				if strings.Contains(strings.Join(args, " "), "maintenance backup") {
					return json.NewEncoder(out).Encode(backup)
				}
				for _, arg := range args {
					if arg == "cp" {
						data := body
						if corrupt {
							data = []byte("corrupt-copy")
						}
						return os.WriteFile(args[len(args)-1], data, 0600)
					}
				}
				return nil
			}}
			output := filepath.Join(t.TempDir(), "operator.age")
			err := a.Backup(context.Background(), output)
			if corrupt {
				if err == nil {
					t.Fatal("corrupt export was accepted")
				}
				if _, err := os.Lstat(output); !os.IsNotExist(err) {
					t.Fatal("unverified output was published")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(output)
			if err != nil || !bytes.Equal(data, body) {
				t.Fatal("exported bytes differ", err)
			}
			if err := a.Backup(context.Background(), output); err == nil {
				t.Fatal("existing export was overwritten")
			}
		})
	}
}
