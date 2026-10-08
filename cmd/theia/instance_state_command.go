package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/lollinoo/theia/internal/config"
	"github.com/lollinoo/theia/internal/crypto"
	"github.com/lollinoo/theia/internal/instance"
	"github.com/lollinoo/theia/internal/secretinput"
)

// runInstanceStateCommand never prints private state. Import reads the original
// configuration and secrets; it deliberately refuses an already managed instance.
func runInstanceStateCommand(args []string, output io.Writer) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: theia instance import|status -state /persistent/secrets.json [-config config.yaml]")
	}
	command := args[0]
	flags := flag.NewFlagSet("theia instance "+command, flag.ContinueOnError)
	flags.SetOutput(output)
	path := flags.String("state", os.Getenv("THEIA_INSTANCE_STATE"), "Persistent private instance state")
	configPath := flags.String("config", "config.yaml", "Original configuration file")
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	if flags.NArg() != 0 || *path == "" {
		return fmt.Errorf("a persistent -state path is required")
	}
	store := instance.Store{Path: *path}
	switch command {
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
		return fmt.Errorf("unknown instance command %q; use import or status", command)
	}
}
