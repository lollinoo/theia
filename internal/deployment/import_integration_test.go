package deployment

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lollinoo/theia/internal/instance"
	"gopkg.in/yaml.v3"
)

func TestLegacyImportIntegration(t *testing.T) {
	if os.Getenv("THEIA_DEPLOYMENT_INTEGRATION") != "1" {
		t.Skip("run make postgres-upgrade-test")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	root := t.TempDir()
	originalFile := filepath.Join(root, "original.yaml")
	password := "original-Pg+password&42"
	connection := (&url.URL{Scheme: "postgres", Host: "postgres:5432", Path: "/theia", User: url.UserPassword("theia", password), RawQuery: "sslmode=disable"}).String()
	project := fmt.Sprintf("theia-import-test-%d", time.Now().UnixNano())
	environment := map[string]string{"THEIA_DATA_DIR": "/data", "THEIA_DB_DSN": connection, "THEIA_ENCRYPTION_KEY": "faithfully-preserved-$legacy-${passphrase}", "THEIA_SESSION_SECRET": "ZTvFsgV73AfVDp9MJnvkwdkPMvXZ9USdt2saSUdiucA", "THEIA_METRICS_TOKEN": "KwkSVLUCECXAcX7wArDpydMF6bDm2Zh8CB5ds4uUaDw", "THEIA_LISTEN_ADDR": ":8080", "THEIA_SESSION_TTL_MINUTES": "73"}
	resolvedEnvironment := map[string]string{}
	for name, value := range environment {
		resolvedEnvironment[name] = strings.ReplaceAll(value, "$", "$$")
	}
	document := map[string]any{"name": project, "services": map[string]any{
		"postgres": map[string]any{"image": "postgres:18-bookworm", "environment": map[string]string{"POSTGRES_USER": "theia", "POSTGRES_DB": "theia", "POSTGRES_PASSWORD": password}, "volumes": []string{filepath.Join(root, "postgres") + ":/var/lib/postgresql"}, "healthcheck": map[string]any{"test": []string{"CMD", "pg_isready", "-U", "theia"}, "interval": "2s", "retries": 30}},
		"backend":  map[string]any{"image": "theia-managed-backend:local", "entrypoint": []string{"theia"}, "environment": resolvedEnvironment, "volumes": []string{filepath.Join(root, "data") + ":/data"}, "depends_on": map[string]any{"postgres": map[string]string{"condition": "service_healthy"}}, "healthcheck": map[string]any{"test": []string{"CMD", "curl", "-fsS", "http://localhost:8080/readyz"}, "interval": "2s", "retries": 45}},
		"frontend": map[string]any{"image": "theia-managed-frontend:local", "depends_on": map[string]any{"backend": map[string]string{"condition": "service_healthy"}}},
	}}
	data, err := yaml.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	if err := instance.WritePrivateFile(originalFile, data); err != nil {
		t.Fatal(err)
	}
	source := func(args ...string) error {
		return DockerRunner(ctx, root, append([]string{"compose", "-f", originalFile}, args...), io.Discard)
	}
	var logs bytes.Buffer
	a := &Admin{Dir: filepath.Join(root, "managed"), Output: &logs, OfflineImages: true}
	defer func() {
		_ = a.compose(context.Background(), "down", "--remove-orphans")
		_ = source("down", "--remove-orphans")
		_ = exec.Command("docker", "run", "--rm", "-v", root+":/test", "--entrypoint", "sh", "theia-managed-backend:local", "-c", "rm -rf /test/*").Run()
	}()
	if err := source("up", "-d", "--wait", "--wait-timeout", "120"); err != nil {
		t.Fatal(err)
	}
	c := testConfig()
	c.Release = "v0.0.0-test"
	c.BackendImage = "theia-managed-backend:local"
	c.FrontendImage = "theia-managed-frontend:local"
	c.HTTPPort = freeDeploymentPort(t)
	c.HTTPSPort = freeDeploymentPort(t)
	recovery := filepath.Join(root, "recovery.txt")
	if err := a.Import(ctx, ImportOptions{Config: c, ComposeFile: originalFile, RecoveryOutput: recovery}); err != nil {
		for _, line := range strings.Split(logs.String(), "\n") {
			if strings.HasPrefix(line, "theia:") || strings.Contains(line, "Error response") || strings.Contains(line, "error:") {
				t.Log(line)
			}
		}
		t.Fatal(err)
	}
	store := instance.Store{Path: StatePath(a.Dir)}
	before, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if before.CredentialKeys["legacy"].Secret != environment["THEIA_ENCRYPTION_KEY"] || before.SessionSecret != environment["THEIA_SESSION_SECRET"] || before.MetricsToken != environment["THEIA_METRICS_TOKEN"] || before.DatabasePassword != password {
		t.Fatal("original secrets changed")
	}
	if err := instance.VerifyRecoveryFile(recovery, before.RecoveryRecipient); err != nil {
		t.Fatal(err)
	}
	if err := a.Offline(ctx, "rotate-operational"); err != nil {
		t.Fatal(err)
	}
	after, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if after.DatabasePassword == before.DatabasePassword || after.SessionSecret == before.SessionSecret || after.MetricsToken == before.MetricsToken || after.ActiveKeyID != before.ActiveKeyID {
		t.Fatal("operational rotation changed the wrong secrets")
	}
	if err := a.RotateRecovery(ctx, recovery, filepath.Join(root, "new-recovery.txt")); err != nil {
		t.Fatal(err)
	}
	latest, _ := store.Load()
	if latest.RecoveryRecipient == before.RecoveryRecipient {
		t.Fatal("recovery identity was not replaced")
	}
	if err := instance.VerifyRecoveryFile(filepath.Join(root, "new-recovery.txt"), before.RecoveryRecipient); err != nil {
		t.Fatal("historical identity lost")
	}
	// Verify a second restart authenticates with the new database password.
	a.Runner = func(ctx context.Context, dir string, args []string, out io.Writer) error {
		return DockerRunner(ctx, dir, args, out)
	}
	if err := a.Up(ctx); err != nil {
		t.Fatal("rotated connection failed to restart", err)
	}
	a.Runner = func(ctx context.Context, dir string, args []string, out io.Writer) error {
		if err := DockerRunner(ctx, dir, args, out); err != nil {
			return err
		}
		if strings.Contains(strings.Join(args, " "), "maintenance rotate-operational") {
			return fmt.Errorf("interruption after password commit, before HTTP")
		}
		return nil
	}
	if err := a.Offline(ctx, "rotate-operational"); err == nil {
		t.Fatal("interrupted password rotation was ignored")
	}
	recovered, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if recovered.DatabasePassword != after.DatabasePassword || recovered.SessionSecret != after.SessionSecret || recovered.MetricsToken != after.MetricsToken {
		t.Fatal("password and operational state were not restored together")
	}
	a.Runner = nil
	if err := a.Up(ctx); err != nil {
		t.Fatal("restored password does not authenticate", err)
	}
}
