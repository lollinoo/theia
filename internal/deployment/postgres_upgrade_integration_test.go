package deployment

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"filippo.io/age"
	"github.com/lollinoo/theia/internal/instance"
	"github.com/lollinoo/theia/internal/service"
)

func freeDeploymentPort(t *testing.T) int {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	return listener.Addr().(*net.TCPAddr).Port
}

func TestPostgresUpgradeIntegration(t *testing.T) {
	if os.Getenv("THEIA_DEPLOYMENT_INTEGRATION") != "1" {
		t.Skip("run make postgres-upgrade-test for isolated Docker integration")
	}
	for _, failAfterImport := range []bool{false, true} {
		t.Run(fmt.Sprint("fail_after_import=", failAfterImport), func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
			defer cancel()
			root := t.TempDir()
			dir := filepath.Join(root, "instance")
			a := &Admin{Dir: dir, Output: io.Discard, OfflineImages: true}
			defer func() {
				_ = a.compose(context.Background(), "down", "--remove-orphans", "--volumes")
				_ = exec.Command("docker", "run", "--rm", "-v", root+":/test", "--entrypoint", "sh", "theia-managed-backend:local", "-c", "rm -rf /test/*").Run()
			}()
			c := testConfig()
			c.Project = ""
			c.Release = "v0.0.0-test"
			c.PostgresMajor = 17
			c.BackendImage = "theia-managed-backend:local"
			c.FrontendImage = "theia-managed-frontend:local"
			c.HTTPPort = freeDeploymentPort(t)
			c.HTTPSPort = freeDeploymentPort(t)
			if err := a.Install(ctx, InstallOptions{Config: c}); err != nil {
				t.Fatal(err)
			}
			original, err := LoadConfig(dir)
			if err != nil {
				t.Fatal(err)
			}
			store := instance.Store{Path: StatePath(dir)}
			identity, _ := age.GenerateX25519Identity()
			if err := store.Update(func(s *instance.State) error {
				s.RecoveryRecipient = identity.Recipient().String()
				key := s.CredentialKeys[s.ActiveKeyID]
				key.CreatedAt = time.Now().Add(-91 * 24 * time.Hour)
				s.CredentialKeys[s.ActiveKeyID] = key
				return nil
			}); err != nil {
				t.Fatal(err)
			}
			state, err := store.Load()
			if err != nil {
				t.Fatal(err)
			}
			keys, _ := state.Keyring()
			encrypted, err := keys.EncryptString("original-upgrade-password")
			if err != nil {
				t.Fatal(err)
			}
			query := fmt.Sprintf("INSERT INTO credential_profiles(id,name,encrypted_secret,created_at,updated_at) VALUES('ba5eea48-de29-42db-8c6f-c85c068b8d02','upgrade-source','%s',now(),now())", encrypted)
			if err := a.compose(ctx, "exec", "-T", "postgres", "psql", "-U", "theia", "-d", "theia", "-v", "ON_ERROR_STOP=1", "-c", query); err != nil {
				t.Fatal(err)
			}
			if failAfterImport {
				a.Runner = func(ctx context.Context, dir string, args []string, out io.Writer) error {
					if err := DockerRunner(ctx, dir, args, out); err != nil {
						return err
					}
					if strings.Contains(strings.Join(args, " "), "maintenance postgres-finish") {
						return fmt.Errorf("injected interruption after verified import, before writes reopen")
					}
					return nil
				}
			}
			err = a.UpgradePostgres(ctx, 18)
			if failAfterImport && err == nil {
				t.Fatal("injected import interruption ignored")
			}
			if !failAfterImport && err != nil {
				t.Fatal(err)
			}
			current, err := LoadConfig(dir)
			if err != nil {
				t.Fatal(err)
			}
			persisted, err := store.Load()
			if err != nil {
				t.Fatal(err)
			}
			if failAfterImport {
				if current.PostgresMajor != 17 || persisted.ActiveKeyID != state.ActiveKeyID {
					t.Fatal("original volume or key state was not recovered")
				}
			} else {
				if current.PostgresMajor != 18 || current.PostgresBind == "" || current.PostgresBind == original.PostgresBind || persisted.ActiveKeyID == state.ActiveKeyID {
					t.Fatal("target was not separate or due key rotation was omitted")
				}
				if _, err := os.Lstat(filepath.Join(dir, "postgres-17")); err != nil {
					t.Fatal("original version 17 volume disappeared", err)
				}
			}
			var sqlOutput bytes.Buffer
			args := []string{"compose", "-f", filepath.Join(dir, "compose.yaml"), "exec", "-T", "postgres", "psql", "-U", "theia", "-d", "theia", "-At", "-c", "SELECT encrypted_secret FROM credential_profiles WHERE name='upgrade-source'"}
			if err := DockerRunner(ctx, dir, args, &sqlOutput); err != nil {
				t.Fatal(err)
			}
			currentKeys, _ := persisted.Keyring()
			plaintext, err := currentKeys.DecryptString(strings.TrimSpace(sqlOutput.String()))
			if err != nil || plaintext != "original-upgrade-password" {
				t.Fatal("upgraded credential cannot be decrypted", err)
			}
			op, err := (&service.Maintenance{StatePath: StatePath(dir)}).Status()
			if err != nil {
				t.Fatal(err)
			}
			if !op.WritesReopened || op.SourcePostgresMajor != 17 || op.TargetPostgresMajor != 18 {
				t.Fatal("cutover receipt missing or writes were not reopened")
			}
			j, err := a.loadUpgrade()
			if err != nil {
				t.Fatal(err)
			}
			wanted := "completed"
			if failAfterImport {
				wanted = "rolled_back"
			}
			if j.Phase != wanted {
				t.Fatalf("journal phase=%s", j.Phase)
			}
			t.Log("verified PostgreSQL", strconv.Itoa(current.PostgresMajor), "with original source retained")
		})
	}
}
