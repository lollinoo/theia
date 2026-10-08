package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/lollinoo/theia/internal/config"
	"github.com/lollinoo/theia/internal/crypto"
	"github.com/lollinoo/theia/internal/instance"
	"github.com/lollinoo/theia/internal/repository/postgres"
	"github.com/lollinoo/theia/internal/secretinput"
)

// runInstanceStateCommand never prints private state. Import reads the original
// configuration and secrets; it deliberately refuses an already managed instance.
func runInstanceStateCommand(args []string, output io.Writer) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: theia instance init|import|activation|recovery|s3|status -state /persistent/secrets.json")
	}
	command := args[0]
	flags := flag.NewFlagSet("theia instance "+command, flag.ContinueOnError)
	flags.SetOutput(output)
	path := flags.String("state", os.Getenv("THEIA_INSTANCE_STATE"), "Persistent private instance state")
	configPath := flags.String("config", "config.yaml", "Original configuration file")
	recoveryFile := flags.String("recovery-file", "", "New operator-held recovery file outside instance storage")
	s3Endpoint := flags.String("endpoint", "", "S3 HTTP(S) endpoint")
	s3Bucket := flags.String("bucket", "", "Existing S3 bucket")
	s3Region := flags.String("region", "", "S3 region")
	s3Prefix := flags.String("prefix", "theia", "S3 object prefix")
	site := flags.String("site", "https://localhost", "Public site origin for the activation link")
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	if flags.NArg() != 0 || *path == "" {
		return fmt.Errorf("a persistent -state path is required")
	}
	store := instance.Store{Path: *path}
	switch command {
	case "init", "activation":
		origin, err := url.Parse(*site)
		if err != nil || (origin.Scheme != "https" && origin.Scheme != "http") || origin.Host == "" || origin.User != nil || origin.RawQuery != "" || origin.Fragment != "" || (origin.Path != "" && origin.Path != "/") {
			return fmt.Errorf("-site must be an HTTP(S) origin")
		}
		var token string
		if command == "init" {
			state, err := instance.Generate(time.Now())
			if err != nil {
				return err
			}
			state.DBDSN, err = secretinput.Read("THEIA_DB_DSN")
			if err != nil {
				return err
			}
			if state.DBDSN == "" {
				return fmt.Errorf("THEIA_DB_DSN[_FILE] is required for initial setup")
			}
			token, err = state.BeginActivation(time.Now())
			if err != nil {
				return err
			}
			if err := store.Create(state); err != nil {
				return err
			}
		} else {
			state, err := store.Load()
			if err != nil {
				return err
			}
			db, err := postgres.OpenPrimaryDB(state.DBDSN)
			if err != nil {
				return fmt.Errorf("activation database connection failed")
			}
			defer db.Close()
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			var count int
			if err := db.QueryRowContext(ctx, "SELECT count(*) FROM users").Scan(&count); err != nil {
				return fmt.Errorf("activation user check failed")
			}
			if count != 0 || state.RecoveryRecipient != "" {
				return fmt.Errorf("existing users or recovery configuration prohibit first-administrator activation")
			}
			if err := store.Update(func(s *instance.State) error {
				var err error
				token, err = s.BeginActivation(time.Now())
				return err
			}); err != nil {
				return err
			}
		}
		_, err = fmt.Fprintf(output, "Open this link within one hour to choose your administrator credentials and save your recovery file:\n%s/activate#token=%s\n", strings.TrimSuffix(origin.String(), "/"), token)
		return err
	case "s3":
		accessKey, err := secretinput.Read("THEIA_S3_ACCESS_KEY")
		if err != nil {
			return err
		}
		secretKey, err := secretinput.Read("THEIA_S3_SECRET_KEY")
		if err != nil {
			return err
		}
		destination := instance.S3Config{Endpoint: *s3Endpoint, Bucket: *s3Bucket, Region: *s3Region, Prefix: *s3Prefix, AccessKey: accessKey, SecretKey: secretKey}
		if _, err := instance.NewS3Destination(destination); err != nil {
			return err
		}
		if err := store.Update(func(state *instance.State) error { state.BackupDestination = &destination; return nil }); err != nil {
			return err
		}
		_, err = fmt.Fprintln(output, "S3 destination saved in private state. Restart the application to use this destination.")
		return err
	case "recovery":
		state, err := store.Load()
		if err != nil {
			return err
		}
		if state.RecoveryRecipient != "" {
			return fmt.Errorf("a recovery recipient is already configured")
		}
		if *recoveryFile == "" {
			return fmt.Errorf("-recovery-file outside instance storage is required")
		}
		dir, err := filepath.Abs(filepath.Dir(*path))
		if err != nil {
			return err
		}
		file, err := filepath.Abs(*recoveryFile)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(dir, file)
		if err != nil || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))) {
			return fmt.Errorf("recovery file must be outside instance storage")
		}
		recipient, err := instance.ExportRecoveryFile(file)
		if err != nil {
			return err
		}
		if err := store.Update(func(s *instance.State) error {
			if s.RecoveryRecipient != "" {
				return fmt.Errorf("recovery recipient was configured concurrently")
			}
			s.RecoveryRecipient = recipient
			return nil
		}); err != nil {
			return err
		}
		_, err = fmt.Fprintln(output, "Recovery file exported and verified. Move it to independent storage outside this host before enabling managed backups.")
		return err
	case "import":
		cfg, err := config.Load(*configPath)
		if err != nil {
			return err
		}
		if cfg.InstanceStatePath != "" {
			return fmt.Errorf("import requires the original unmanaged configuration")
		}
		keys, err := crypto.LoadKeyringFromEnv()
		if err != nil {
			return err
		}
		password, err := secretinput.Read("POSTGRES_PASSWORD")
		if err != nil {
			return err
		}
		state, err := instance.Import(keys, cfg.SessionSecret, cfg.MetricsToken, cfg.DBDSN, password, time.Now())
		if err != nil {
			return err
		}
		if err := store.Create(state); err != nil {
			return err
		}
		_, err = fmt.Fprintln(output, "Original keys and operational secrets imported. Stop the server before switching to THEIA_INSTANCE_STATE and removing legacy secret variables.")
		return err
	case "status":
		state, err := store.Load()
		if err != nil {
			return err
		}
		keys, err := state.Keyring()
		if err != nil {
			return err
		}
		return json.NewEncoder(output).Encode(struct {
			InstanceID         string    `json:"instance_id"`
			ActiveKeyID        string    `json:"active_key_id"`
			KeyIDs             []string  `json:"retained_key_ids"`
			RotationDueAt      time.Time `json:"rotation_due_at"`
			RecoveryConfigured bool      `json:"recovery_configured"`
		}{state.InstanceID, state.ActiveKeyID, keys.KeyIDs(), state.CredentialKeys[state.ActiveKeyID].CreatedAt.Add(instance.CredentialRotationInterval), state.RecoveryRecipient != ""})
	default:
		return fmt.Errorf("unknown instance command %q; use init, import, activation, recovery, s3 or status", command)
	}
}
