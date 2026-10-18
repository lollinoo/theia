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
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/lollinoo/theia/internal/instance"
	"github.com/lollinoo/theia/internal/service"
	"gopkg.in/yaml.v3"
)

type legacyContainer struct {
	Config struct {
		Env        []string
		WorkingDir string
	}
	Mounts          []struct{ Type, Source, Name, Destination string }
	NetworkSettings struct{ Networks map[string]json.RawMessage }
}

// ImportOptions identifies an existing Compose installation. Docker supplies its
// resolved environment and mounts; the importer never reparses dotenv quoting.
type ImportOptions struct {
	Config                                           Config
	ComposeFile, EnvFile, ConfigPath, RecoveryOutput string
}

func (a *Admin) capture(ctx context.Context, args ...string) ([]byte, error) {
	var buffer bytes.Buffer
	runner := a.Runner
	if runner == nil {
		runner = DockerRunner
	}
	if err := runner(ctx, a.Dir, args, &buffer); err != nil {
		return nil, fmt.Errorf("Docker inspection failed")
	}
	if buffer.Len() > 4<<20 {
		return nil, fmt.Errorf("Docker inspection exceeds its size limit")
	}
	return buffer.Bytes(), nil
}

// Import adopts the original persistent storage and secrets, protects the source
// before migration, and retains the original Compose file for a verified rollback.
func (a *Admin) Import(ctx context.Context, options ImportOptions) error {
	unlock, err := a.lock()
	if err != nil {
		return err
	}
	defer unlock()
	if _, err := os.Lstat(StatePath(a.Dir)); !os.IsNotExist(err) {
		return fmt.Errorf("managed state already exists; use up or resume")
	}
	source, err := filepath.Abs(options.ComposeFile)
	if err != nil {
		return err
	}
	original := []string{"compose", "--project-directory", filepath.Dir(source), "-f", source}
	if options.EnvFile != "" {
		file, err := filepath.Abs(options.EnvFile)
		if err != nil {
			return err
		}
		original = append(original, "--env-file", file)
	}
	inspect := func(name string) (string, *legacyContainer, error) {
		data, err := a.capture(ctx, append(append([]string{}, original...), "ps", "-aq", name)...)
		if err != nil {
			return "", nil, err
		}
		id := strings.TrimSpace(string(data))
		if id == "" {
			return "", nil, nil
		}
		if strings.ContainsAny(id, "\r\n") {
			return "", nil, fmt.Errorf("legacy import requires one %s container", name)
		}
		data, err = a.capture(ctx, "inspect", id)
		if err != nil {
			return "", nil, err
		}
		var containers []legacyContainer
		if json.Unmarshal(data, &containers) != nil || len(containers) != 1 {
			return "", nil, fmt.Errorf("invalid original container inspection")
		}
		return id, &containers[0], nil
	}
	_, backend, err := inspect("backend")
	if err != nil {
		return err
	}
	if backend == nil {
		return fmt.Errorf("create the original backend container before importing its resolved configuration")
	}
	pgID, pg, err := inspect("postgres")
	if err != nil {
		return err
	}
	environment := map[string]string{}
	for _, entry := range backend.Config.Env {
		name, value, ok := strings.Cut(entry, "=")
		if ok {
			// The environment is already resolved; escape literal dollars before
			// Compose evaluates the temporary importer document a second time.
			environment[name] = strings.ReplaceAll(value, "$", "$$")
		}
	}
	if environment["THEIA_INSTANCE_STATE"] != "" {
		return fmt.Errorf("source is already managed")
	}
	c := options.Config
	c.Version = ConfigVersion
	c.Project = "theia-" + uuid.NewString()[:8]
	c.BundledPostgres = pg != nil
	c.PostgresMajor = 18
	for network := range backend.NetworkSettings.Networks {
		c.ExistingNetworks = append(c.ExistingNetworks, network)
	}
	sort.Strings(c.ExistingNetworks)
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
	if c.BackendImage == "" {
		c.BackendImage = "ghcr.io/lollinoo/theia-backend:" + c.Release
	}
	if c.FrontendImage == "" {
		c.FrontendImage = "ghcr.io/lollinoo/theia-frontend:" + c.Release
	}
	if c.TLSMode == "external" {
		return fmt.Errorf("import uses internal, auto or proxy TLS; configure supplied certificates after the transition")
	}
	dataTarget := environment["THEIA_DATA_DIR"]
	if dataTarget == "" {
		dataTarget = "/data"
	}
	found := false
	mounts := []string{filepath.Join(a.Dir, "control") + ":/control"}
	volumes := map[string]any{}
	for _, mount := range backend.Mounts {
		mountSource := mount.Source
		if mount.Type == "volume" {
			mountSource = mount.Name
			volumes[mount.Name] = map[string]any{"external": true, "name": mount.Name}
		}
		if mount.Type != "bind" && mount.Type != "volume" {
			return fmt.Errorf("unsupported legacy mount type")
		}
		if strings.ContainsAny(mountSource+mount.Destination, ":$\n\r") {
			return fmt.Errorf("legacy mount path cannot be imported")
		}
		mounts = append(mounts, mountSource+":"+mount.Destination+":ro")
		if mount.Destination == dataTarget {
			found = true
			if mount.Type == "volume" {
				c.DataVolume = mount.Name
			} else {
				c.DataBind = mount.Source
			}
		}
	}
	if !found {
		return fmt.Errorf("original application data must have a dedicated persistent mount")
	}
	if err := outsideStorage(options.RecoveryOutput, a.Dir, c.DataBind); err != nil {
		return err
	}
	if pg != nil {
		user := "postgres"
		for _, entry := range pg.Config.Env {
			if value, ok := strings.CutPrefix(entry, "POSTGRES_USER="); ok {
				user = value
			}
		}
		version, err := a.capture(ctx, "exec", pgID, "psql", "-U", user, "-d", "postgres", "-Atqc", "SHOW server_version_num")
		if err != nil {
			return err
		}
		number, err := strconv.Atoi(strings.TrimSpace(string(version)))
		if err != nil {
			return fmt.Errorf("original PostgreSQL version could not be read")
		}
		c.PostgresMajor = number / 10000
		pgTarget := "/var/lib/postgresql"
		if c.PostgresMajor == 17 {
			pgTarget += "/data"
		}
		for _, mount := range pg.Mounts {
			if mount.Destination == pgTarget {
				if mount.Type == "volume" {
					c.PostgresVolume = mount.Name
				} else if mount.Type == "bind" {
					c.PostgresBind = mount.Source
				}
			}
		}
		if c.PostgresVolume == "" && c.PostgresBind == "" {
			return fmt.Errorf("original PostgreSQL storage layout is unsupported; no data was changed")
		}
	}
	if err := c.Validate(); err != nil {
		return err
	}
	if !a.OfflineImages {
		for _, image := range []string{c.BackendImage, c.FrontendImage} {
			if err := a.run(ctx, "pull", image); err != nil {
				return err
			}
		}
	}
	importer, err := yaml.Marshal(map[string]any{"name": c.Project + "-import", "services": map[string]any{"importer": map[string]any{"image": c.BackendImage, "entrypoint": []string{"theia"}, "working_dir": backend.Config.WorkingDir, "environment": environment, "volumes": mounts}}, "volumes": volumes})
	if err != nil {
		return err
	}
	file := filepath.Join(a.Dir, "control", "import.yaml")
	if err := instance.WritePrivateFile(file, importer); err != nil {
		return err
	}
	defer os.Remove(file)
	configPath := options.ConfigPath
	if configPath == "" {
		configPath = "config.yaml"
	}
	if err := a.run(ctx, "compose", "-f", file, "run", "--rm", "--no-deps", "-T", "importer", "instance", "import", "-state", "/control/state.json", "-config", configPath); err != nil {
		return err
	}
	store := instance.Store{Path: StatePath(a.Dir)}
	state, err := store.Load()
	if err != nil {
		return err
	}
	if c.BundledPostgres {
		connection, err := pgx.ParseConfig(state.DBDSN)
		if err != nil {
			return fmt.Errorf("invalid original PostgreSQL connection")
		}
		c.PostgresUser, c.PostgresDatabase = connection.User, connection.Database
		dsn := &url.URL{Scheme: "postgres", Host: "postgres:5432", Path: "/" + connection.Database, User: url.UserPassword(connection.User, connection.Password), RawQuery: "sslmode=disable"}
		state.DBDSN, state.DatabasePassword = dsn.String(), connection.Password
	}
	recipient, err := instance.ExportRecoveryFile(options.RecoveryOutput)
	if err != nil {
		return err
	}
	state.RecoveryRecipient = recipient
	state.DeploymentMetadata, err = json.Marshal(c)
	if err != nil {
		return err
	}
	if err := store.Update(func(current *instance.State) error { *current = *state; return nil }); err != nil {
		return err
	}
	if err := SaveConfig(a.Dir, c); err != nil {
		return err
	}
	if err := a.prepare(c, state); err != nil {
		return err
	}
	// Only after inspection and recovery-file read-back stop the original writers.
	stopArgs := []string{"stop", "backend", "frontend"}
	if pg != nil {
		stopArgs = append(stopArgs, "postgres")
	}
	if err := a.run(ctx, append(append([]string{}, original...), stopArgs...)...); err != nil {
		return err
	}
	if err := a.up(ctx, c); err != nil {
		op, statusErr := (&service.Maintenance{StatePath: StatePath(a.Dir)}).Status()
		if statusErr == nil && op != nil && op.Phase == "rolled_back" {
			stopErr := a.compose(ctx, "stop")
			if stopErr != nil {
				return errors.Join(err, stopErr)
			}
			restartErr := a.run(ctx, append(append([]string{}, original...), "up", "-d", "--wait", "--wait-timeout", "120")...)
			return errors.Join(err, restartErr)
		}
		return err
	}
	fmt.Fprintln(a.Output, "Original storage, configuration and credential keys imported. Keep the original deployment files and move the verified recovery file to independent storage.")
	return a.ActivationUnlocked(ctx, c)
}

// ActivationUnlocked prints a link only when the imported instance has no users.
func (a *Admin) ActivationUnlocked(ctx context.Context, c Config) error {
	var output bytes.Buffer
	original := a.Output
	a.Output = &output
	err := a.job(ctx, "instance", "activation", "-site", c.Origin())
	a.Output = original
	if err == nil {
		_, err = io.Copy(original, &output)
		return err
	}
	// Registered users retain their existing account; renewal refuses to overwrite it.
	if strings.Contains(output.String(), "existing users prohibit") {
		return nil
	}
	return err
}
