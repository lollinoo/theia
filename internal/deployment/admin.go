package deployment

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/instance"
	"github.com/lollinoo/theia/internal/service"
)

// Runner executes Docker arguments directly, without shell interpolation.
type Runner func(context.Context, string, []string, io.Writer) error

// Admin adapts host operations to the same maintenance commands used by Jobs.
type Admin struct {
	Dir           string
	Output        io.Writer
	Runner        Runner
	ReadyCheck    func(context.Context, Config) error
	OfflineImages bool
}

func DockerRunner(ctx context.Context, dir string, args []string, output io.Writer) error {
	cmd := exec.CommandContext(ctx, "docker", args...)
	cmd.Dir, cmd.Stdout, cmd.Stderr = dir, output, output
	return cmd.Run()
}

func (a *Admin) run(ctx context.Context, args ...string) error {
	runner := a.Runner
	if runner == nil {
		runner = DockerRunner
	}
	out := a.Output
	if out == nil {
		out = io.Discard
	}
	return runner(ctx, a.Dir, args, out)
}

func (a *Admin) compose(ctx context.Context, args ...string) error {
	abs, err := filepath.Abs(a.Dir)
	if err != nil {
		return err
	}
	a.Dir = abs
	return a.run(ctx, append([]string{"compose", "--project-directory", a.Dir, "-f", filepath.Join(a.Dir, "compose.yaml")}, args...)...)
}

func (a *Admin) job(ctx context.Context, args ...string) error {
	return a.compose(ctx, append([]string{"run", "--rm", "--no-deps", "-T", "backend"}, args...)...)
}

func (a *Admin) lock() (func(), error) {
	abs, err := filepath.Abs(a.Dir)
	if err != nil {
		return nil, err
	}
	a.Dir = abs
	if err := instance.PrivateDirectory(filepath.Join(a.Dir, "control")); err != nil {
		return nil, err
	}
	return instance.AcquireLease(filepath.Join(a.Dir, "control", "deployment"))
}

// InstallOptions supplies one-time infrastructure coordinates. Secrets are read
// from files and are never copied into the generated Compose document.
type InstallOptions struct {
	Config          Config
	DatabaseDSNFile string
	TLSCertFile     string
	TLSKeyFile      string
	PrepareRestore  bool
}

// Install creates persistent state exactly once, then migrates before starting
// HTTP. A failed installation resumes with Up and a renewed activation link.
func (a *Admin) Install(ctx context.Context, options InstallOptions) error {
	unlock, err := a.lock()
	if err != nil {
		return err
	}
	defer unlock()
	if _, err := os.Lstat(StatePath(a.Dir)); !os.IsNotExist(err) {
		return fmt.Errorf("instance state already exists; use up or restore instead of reinstalling")
	}
	c := options.Config
	c.Version = ConfigVersion
	if c.Project == "" {
		c.Project = "theia-" + uuid.NewString()[:8]
	}
	if c.Hostname == "" {
		c.Hostname = "localhost"
	}
	c.TLSMode = selectTLS(c.Hostname, c.TLSMode)
	if c.BindAddress == "" {
		c.BindAddress = "0.0.0.0"
	}
	if c.HTTPPort == 0 {
		c.HTTPPort = 80
	}
	if c.HTTPSPort == 0 {
		c.HTTPSPort = 443
	}
	if c.PostgresMajor == 0 {
		c.PostgresMajor = 18
	}
	c.BundledPostgres = options.DatabaseDSNFile == ""
	if c.BackendImage == "" {
		c.BackendImage = "ghcr.io/lollinoo/theia-backend:" + c.Release
	}
	if c.FrontendImage == "" {
		c.FrontendImage = "ghcr.io/lollinoo/theia-frontend:" + c.Release
	}
	if err := c.Validate(); err != nil {
		return err
	}
	s, err := instance.Generate(time.Now())
	if err != nil {
		return err
	}
	if c.BundledPostgres {
		dsn := &url.URL{Scheme: "postgres", Host: "postgres:5432", Path: "/theia", User: url.UserPassword("theia", s.DatabasePassword), RawQuery: "sslmode=disable"}
		s.DBDSN = dsn.String()
	} else {
		data, err := readInputFile(options.DatabaseDSNFile, 64<<10)
		if err != nil {
			return err
		}
		s.DBDSN = strings.TrimRight(string(data), "\r\n")
		if s.DBDSN == "" {
			return fmt.Errorf("external database connection file is empty")
		}
	}
	var token string
	if !options.PrepareRestore {
		token, err = s.BeginActivation(time.Now())
		if err != nil {
			return err
		}
	}
	s.DeploymentMetadata, err = json.Marshal(c)
	if err != nil {
		return err
	}
	for _, dir := range []string{a.Dir, filepath.Join(a.Dir, "data"), filepath.Join(a.Dir, "data", "certificates"), filepath.Join(a.Dir, "control", "public")} {
		if err := instance.PrivateDirectory(dir); err != nil {
			return err
		}
	}
	if c.TLSMode == "external" {
		for _, file := range []struct{ source, name string }{{options.TLSCertFile, "cert.pem"}, {options.TLSKeyFile, "key.pem"}} {
			data, err := readInputFile(file.source, 1<<20)
			if err != nil {
				return err
			}
			if err := instance.WritePrivateFile(filepath.Join(a.Dir, "data", "certificates", "external", file.name), data); err != nil {
				return err
			}
		}
	}
	if err := SaveConfig(a.Dir, c); err != nil {
		return err
	}
	if err := (instance.Store{Path: StatePath(a.Dir)}).Create(s); err != nil {
		return err
	}
	if err := a.prepare(c, s); err != nil {
		return err
	}
	if !a.OfflineImages {
		if err := a.compose(ctx, "pull"); err != nil {
			return fmt.Errorf("image download failed; persistent instance secrets are preserved: %w", err)
		}
	}
	if options.PrepareRestore {
		if c.BundledPostgres {
			return a.compose(ctx, "up", "-d", "--wait", "--wait-timeout", "120", "postgres")
		}
		return nil
	}
	if err := a.up(ctx, c); err != nil {
		return err
	}
	fmt.Fprintf(a.Output, "Theia is ready at %s\nActivate within one hour:\n%s/activate#token=%s\n", c.Origin(), c.Origin(), token)
	if c.TLSMode == "internal" {
		fmt.Fprintf(a.Output, "Trust the local CA once on each client. Export it with: theia-admin ca -dir %s -output theia-ca.crt\n", a.Dir)
	}
	return nil
}

func readInputFile(path string, limit int64) ([]byte, error) {
	if path == "" {
		return nil, fmt.Errorf("input file is required")
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > limit {
		return nil, fmt.Errorf("input must be a regular file within its size limit")
	}
	data, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("input file exceeds size limit")
	}
	return data, nil
}

func (a *Admin) prepare(c Config, s *instance.State) error {
	if err := instance.PrivateDirectory(filepath.Join(a.Dir, "control", "public")); err != nil {
		return err
	}
	if c.BundledPostgres {
		if s.DatabasePassword == "" {
			return fmt.Errorf("bundled PostgreSQL password is missing")
		}
		if err := instance.WritePrivateFile(filepath.Join(a.Dir, "control", "postgres-password.txt"), []byte(s.DatabasePassword)); err != nil {
			return err
		}
	}
	return WriteCompose(a.Dir, c)
}

func (a *Admin) saveMetadata(c Config) error {
	data, err := json.Marshal(c)
	if err != nil {
		return err
	}
	return (instance.Store{Path: StatePath(a.Dir)}).Update(func(s *instance.State) error { s.DeploymentMetadata = data; return nil })
}

func (a *Admin) up(ctx context.Context, c Config) error {
	if c.BundledPostgres {
		if err := a.compose(ctx, "up", "-d", "--wait", "--wait-timeout", "120", "postgres"); err != nil {
			return err
		}
	}
	if err := a.pause(ctx, false); err != nil {
		return err
	}
	if err := a.job(ctx, "maintenance", "migrate"); err != nil {
		return err
	}
	if err := a.startHTTP(ctx, c); err != nil {
		return err
	}
	return nil
}

// Up resumes deployment without generating or replacing any instance secret.
func (a *Admin) Up(ctx context.Context) error {
	unlock, err := a.lock()
	if err != nil {
		return err
	}
	defer unlock()
	c, err := LoadConfig(a.Dir)
	if err != nil {
		return err
	}
	s, err := (instance.Store{Path: StatePath(a.Dir)}).Load()
	if err != nil {
		return err
	}
	if err := a.prepare(*c, s); err != nil {
		return err
	}
	return a.up(ctx, *c)
}

// Status works without the database or HTTP server, showing the durable operation.
func (a *Admin) Status(ctx context.Context) error {
	c, err := LoadConfig(a.Dir)
	if err != nil {
		return err
	}
	op, err := (&service.Maintenance{StatePath: StatePath(a.Dir)}).Status()
	if err != nil {
		return err
	}
	if err := json.NewEncoder(a.Output).Encode(struct {
		Site        string                        `json:"site"`
		Release     string                        `json:"release"`
		Maintenance *service.MaintenanceOperation `json:"operation"`
	}{c.Origin(), c.Release, op}); err != nil {
		return err
	}
	return a.compose(ctx, "ps")
}

// Offline adapts an administration action to a stopped HTTP writer. The frontend
// remains available for maintenance status unless its certificate state is restored.
func (a *Admin) Offline(ctx context.Context, action string, args ...string) error {
	unlock, err := a.lock()
	if err != nil {
		return err
	}
	defer unlock()
	return a.offline(ctx, action, args...)
}

func (a *Admin) offline(ctx context.Context, action string, args ...string) error {
	c, err := LoadConfig(a.Dir)
	if err != nil {
		return err
	}
	if err := a.pause(ctx, false); err != nil {
		return err
	}
	jobErr := a.job(ctx, append([]string{"maintenance", action}, args...)...)
	if jobErr != nil {
		op, statusErr := (&service.Maintenance{StatePath: StatePath(a.Dir)}).Status()
		if statusErr == nil && op != nil && (op.Phase == "completed" || op.Phase == "rolled_back") && op.VerifiedReleaseTag == c.Release {
			return errors.Join(jobErr, a.startHTTP(ctx, *c))
		}
		return jobErr
	}
	return a.startHTTP(ctx, *c)
}

// Activation issues a replacement link only while no users exist. The shared
// backend command applies the same check to disabled users and imported instances.
func (a *Admin) Activation(ctx context.Context) error {
	unlock, err := a.lock()
	if err != nil {
		return err
	}
	defer unlock()
	c, err := LoadConfig(a.Dir)
	if err != nil {
		return err
	}
	return a.job(ctx, "instance", "activation", "-site", c.Origin())
}

// ExportCA exports only the public LAN root certificate; operator clients import
// it once, and the persistent authority retains its identity through restarts.
func (a *Admin) ExportCA(ctx context.Context, path string) error {
	abs, err := filepath.Abs(a.Dir)
	if err != nil {
		return err
	}
	a.Dir = abs
	var buffer bytes.Buffer
	runner := a.Runner
	if runner == nil {
		runner = DockerRunner
	}
	args := []string{"compose", "-f", filepath.Join(a.Dir, "compose.yaml"), "exec", "-T", "frontend", "cat", "/data/caddy/pki/authorities/local/root.crt"}
	if err := runner(ctx, a.Dir, args, &buffer); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
	if err != nil {
		return err
	}
	_, writeErr := f.Write(buffer.Bytes())
	return errors.Join(writeErr, f.Close())
}
