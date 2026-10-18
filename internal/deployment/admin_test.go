package deployment

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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

func TestKubernetesInstallExternalPostgresTrimsCRLFAndRejectsEmpty(t *testing.T) {
	dir := t.TempDir()
	k := &Kubernetes{Dir: dir, Output: io.Discard, Runner: func(_ context.Context, _ string, _ []string, _ io.Writer) error {
		return nil
	}}
	cfg := KubernetesConfig{
		Name:          "theia",
		Namespace:     "theia",
		Context:       "test-ctx",
		Release:       "v1.8.0",
		BackendImage:  "test-backend:v1.8.0",
		FrontendImage: "test-frontend:v1.8.0",
		Hostname:      "theia.local",
		TLSSecret:     "theia-tls",
	}

	emptyFile := filepath.Join(dir, "empty.dsn")
	if err := os.WriteFile(emptyFile, []byte("\r\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := k.Install(context.Background(), cfg, emptyFile, false); err == nil || !strings.Contains(err.Error(), "external database connection file is empty") {
		t.Fatalf("expected empty DSN error, got: %v", err)
	}

	validFile := filepath.Join(dir, "valid.dsn")
	dsn := "postgres://user:pass@host:5432/theia?sslmode=disable"
	if err := os.WriteFile(validFile, []byte(dsn+"\r\n"), 0600); err != nil {
		t.Fatal(err)
	}
	var capturedDSN string
	k.Runner = func(_ context.Context, program string, args []string, out io.Writer) error {
		cmd := strings.Join(args, " ")
		if strings.Contains(cmd, "apply -f") {
			for i, arg := range args {
				if arg == "-f" && i+1 < len(args) {
					content, err := os.ReadFile(args[i+1])
					if err == nil {
						var obj map[string]any
						if json.Unmarshal(content, &obj) == nil {
							if stringData, ok := obj["stringData"].(map[string]any); ok {
								if stateJSON, ok := stringData["state.json"].(string); ok {
									var st instance.State
									if json.Unmarshal([]byte(stateJSON), &st) == nil {
										capturedDSN = st.DBDSN
									}
								}
							}
						}
					}
				}
			}
		}
		if strings.Contains(cmd, "jsonpath={.items[0].metadata.name}") {
			_, _ = io.WriteString(out, "test-pod")
		}
		return nil
	}
	dir2 := t.TempDir()
	k.Dir = dir2
	if err := k.Install(context.Background(), cfg, validFile, false); err != nil {
		t.Fatal(err)
	}
	if capturedDSN != dsn {
		t.Fatalf("expected trimmed DSN %q, got %q", dsn, capturedDSN)
	}
}

func TestKubernetesUpgradeSuccessAndRollbackRemoveMaintenance(t *testing.T) {
	dir := t.TempDir()
	cfg := KubernetesConfig{
		Name:          "theia",
		Namespace:     "theia",
		Context:       "test-ctx",
		Release:       "v1.8.0",
		BackendImage:  "test-backend:v1.8.0",
		FrontendImage: "test-frontend:v1.8.0",
		Hostname:      "theia.local",
		TLSSecret:     "theia-tls",
	}
	cfgData, _ := json.Marshal(cfg)
	_ = instance.WritePrivateFile(filepath.Join(dir, "control", "kubernetes.json"), cfgData)

	var commands []string
	k := &Kubernetes{
		Dir:    dir,
		Output: io.Discard,
		Runner: func(_ context.Context, program string, args []string, out io.Writer) error {
			cmd := program + " " + strings.Join(args, " ")
			commands = append(commands, cmd)
			if strings.Contains(cmd, "jsonpath={.items[0].metadata.name}") {
				_, _ = io.WriteString(out, "test-pod")
			}
			if strings.Contains(cmd, "theia maintenance status") {
				_ = json.NewEncoder(out).Encode(&service.MaintenanceOperation{Phase: "completed", VerifiedReleaseTag: "v1.9.0"})
			}
			return nil
		},
	}

	// 1. Successful upgrade
	if err := k.Upgrade(context.Background(), "v1.9.0", "test-backend:v1.9.0", "test-frontend:v1.9.0"); err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(commands, "\n")
	rmMaintenance := "kubectl --context test-ctx --namespace theia exec deployment/theia-backend -- rm -f /persist/control/public/deployment-maintenance"
	if !strings.Contains(joined, rmMaintenance) {
		t.Fatalf("expected rm -f deployment-maintenance on success, got commands:\n%s", joined)
	}
	data, err := os.ReadFile(filepath.Join(dir, "control", "kubernetes-upgrade.json"))
	if err != nil {
		t.Fatal(err)
	}
	var journal kubernetesUpgrade
	if err := json.Unmarshal(data, &journal); err != nil {
		t.Fatal(err)
	}
	if journal.Phase != "completed" {
		t.Fatalf("expected completed journal phase, got %s", journal.Phase)
	}

	// 2. Rollback upgrade
	commands = nil
	k.Runner = func(_ context.Context, program string, args []string, out io.Writer) error {
		cmd := program + " " + strings.Join(args, " ")
		commands = append(commands, cmd)
		if strings.Contains(cmd, "jsonpath={.items[0].metadata.name}") {
			_, _ = io.WriteString(out, "test-pod")
		}
		if strings.Contains(cmd, "theia maintenance migrate") {
			return fmt.Errorf("migration failure")
		}
		if strings.Contains(cmd, "theia maintenance status") {
			_ = json.NewEncoder(out).Encode(&service.MaintenanceOperation{Phase: "rolled_back", VerifiedReleaseTag: "v1.9.0"})
		}
		return nil
	}
	if err := k.Upgrade(context.Background(), "v2.0.0", "test-backend:v2.0.0", "test-frontend:v2.0.0"); err == nil {
		t.Fatal("expected migration failure error")
	}
	joined = strings.Join(commands, "\n")
	if !strings.Contains(joined, rmMaintenance) {
		t.Fatalf("expected rm -f deployment-maintenance on rollback, got commands:\n%s", joined)
	}
	data, err = os.ReadFile(filepath.Join(dir, "control", "kubernetes-upgrade.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &journal); err != nil {
		t.Fatal(err)
	}
	if journal.Phase != "rolled_back" {
		t.Fatalf("expected rolled_back journal phase, got %s", journal.Phase)
	}

	// 3. New upgrade after rolled_back should succeed and not fail as incomplete
	commands = nil
	k.Runner = func(_ context.Context, program string, args []string, out io.Writer) error {
		cmd := program + " " + strings.Join(args, " ")
		commands = append(commands, cmd)
		if strings.Contains(cmd, "jsonpath={.items[0].metadata.name}") {
			_, _ = io.WriteString(out, "test-pod")
		}
		if strings.Contains(cmd, "theia maintenance status") {
			_ = json.NewEncoder(out).Encode(&service.MaintenanceOperation{Phase: "completed", VerifiedReleaseTag: "v2.0.0"})
		}
		return nil
	}
	if err := k.Upgrade(context.Background(), "v2.0.0", "test-backend:v2.0.0", "test-frontend:v2.0.0"); err != nil {
		t.Fatalf("unexpected error starting upgrade after rollback: %v", err)
	}
}

func TestKubernetesRestoreRemovesMaintenanceWhenNotPrepared(t *testing.T) {
	for _, prepared := range []bool{false, true} {
		t.Run(fmt.Sprintf("prepared=%v", prepared), func(t *testing.T) {
			dir := t.TempDir()
			cfg := KubernetesConfig{
				Name:           "theia",
				Namespace:      "theia",
				Context:        "test-ctx",
				Release:        "v1.8.0",
				BackendImage:   "test-backend:v1.8.0",
				FrontendImage:  "test-frontend:v1.8.0",
				Hostname:       "theia.local",
				TLSSecret:      "theia-tls",
				PrepareRestore: prepared,
			}
			cfgData, _ := json.Marshal(cfg)
			_ = instance.WritePrivateFile(filepath.Join(dir, "control", "kubernetes.json"), cfgData)

			recoveryFile := filepath.Join(dir, "recovery.txt")
			_ = os.WriteFile(recoveryFile, []byte("test-recovery-key"), 0600)
			archiveFile := filepath.Join(dir, "archive.age")
			_ = os.WriteFile(archiveFile, []byte("test-archive"), 0600)

			var commands []string
			k := &Kubernetes{
				Dir:    dir,
				Output: io.Discard,
				Runner: func(_ context.Context, program string, args []string, out io.Writer) error {
					cmd := program + " " + strings.Join(args, " ")
					commands = append(commands, cmd)
					if strings.Contains(cmd, "jsonpath={.items[0].metadata.name}") {
						_, _ = io.WriteString(out, "test-pod")
					}
					return nil
				},
			}

			if err := k.Restore(context.Background(), archiveFile, recoveryFile); err != nil {
				t.Fatal(err)
			}
			joined := strings.Join(commands, "\n")
			rmMaintenance := "kubectl --context test-ctx --namespace theia exec deployment/theia-backend -- rm -f /persist/control/public/deployment-maintenance"
			if !prepared {
				if !strings.Contains(joined, rmMaintenance) {
					t.Fatalf("expected rm -f deployment-maintenance on restore when !prepared, got commands:\n%s", joined)
				}
			} else {
				if strings.Contains(joined, rmMaintenance) {
					t.Fatalf("did not expect rm -f deployment-maintenance on restore when prepared, got commands:\n%s", joined)
				}
			}
		})
	}
}

func TestWaitHTTPSProxyMode(t *testing.T) {
	var requestedHost string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestedHost = r.Host
		if r.URL.Path == "/readyz" {
			w.WriteHeader(http.StatusOK)
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	u, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	host, portStr, _ := net.SplitHostPort(u.Host)
	var port int
	_, _ = fmt.Sscanf(portStr, "%d", &port)

	cfg := testConfig()
	cfg.TLSMode = "proxy"
	cfg.Hostname = "theia.custom-domain.internal"
	cfg.BindAddress = host
	cfg.HTTPPort = port

	a := &Admin{Dir: t.TempDir(), Output: io.Discard}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := a.waitHTTPS(ctx, cfg); err != nil {
		t.Fatalf("waitHTTPS failed: %v", err)
	}
	if requestedHost != cfg.Hostname {
		t.Fatalf("expected request Host %q, got %q", cfg.Hostname, requestedHost)
	}
}

func TestChartFrontendPodAffinityUsesTheiaInstance(t *testing.T) {
	dir := t.TempDir()
	if err := ExportChart(dir); err != nil {
		t.Fatal(err)
	}
	resourcesData, err := os.ReadFile(filepath.Join(dir, "templates", "resources.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	content := string(resourcesData)
	if strings.Contains(content, "podAffinity:\n          requiredDuringSchedulingIgnoredDuringExecution:\n            - labelSelector:\n                matchLabels: {app: {{ $name }}-backend}") {
		t.Fatal("resources.yaml frontend podAffinity still matches backend app label")
	}
	if !strings.Contains(content, "podAffinity:\n          requiredDuringSchedulingIgnoredDuringExecution:\n            - labelSelector:\n                matchLabels: {theia-instance: {{ $name }}}") {
		t.Fatal("resources.yaml frontend podAffinity does not match theia-instance label")
	}
}
