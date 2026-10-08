// Package instance owns persistent instance secrets. Creating state is an explicit
// administration operation; loading missing state never generates replacement keys.
package instance

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"filippo.io/age"
	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/crypto"
)

const StateVersion = 1
const CredentialRotationInterval = 90 * 24 * time.Hour

// CredentialKey retains the original secret, including legacy passphrases.
type CredentialKey struct {
	Secret    string    `json:"secret"`
	CreatedAt time.Time `json:"created_at"`
}

// State is private instance material. Only its public recovery recipient may be
// stored here; recovery identities belong exclusively to the operator.
type State struct {
	Version            int                      `json:"version"`
	InstanceID         string                   `json:"instance_id"`
	CreatedAt          time.Time                `json:"created_at"`
	ActiveKeyID        string                   `json:"active_key_id"`
	CredentialKeys     map[string]CredentialKey `json:"credential_keys"`
	SessionSecret      string                   `json:"session_secret"`
	MetricsToken       string                   `json:"metrics_token"`
	DBDSN              string                   `json:"db_dsn,omitempty"`
	DatabasePassword   string                   `json:"database_password,omitempty"`
	SnapshotSecret     string                   `json:"snapshot_secret"`
	RecoveryRecipient  string                   `json:"recovery_recipient,omitempty"`
	BackupDestination  *S3Config                `json:"backup_destination,omitempty"`
	Activation         *Activation              `json:"activation,omitempty"`
	DeploymentMetadata json.RawMessage          `json:"deployment_metadata,omitempty"`
	ApplicationConfig  json.RawMessage          `json:"application_config,omitempty"`
}

// Generate creates fresh secrets in memory without writing them or replacing state.
func Generate(now time.Time) (*State, error) {
	values := make([]string, 5)
	for i := range values {
		value, err := RandomSecret()
		if err != nil {
			return nil, err
		}
		values[i] = value
	}
	id := uuid.NewString()
	now = now.UTC()
	return &State{Version: StateVersion, InstanceID: uuid.NewString(), CreatedAt: now,
		ActiveKeyID: id, CredentialKeys: map[string]CredentialKey{id: {Secret: values[0], CreatedAt: now}},
		SessionSecret: values[1], MetricsToken: values[2], DatabasePassword: values[3], SnapshotSecret: values[4]}, nil
}

// Import preserves the original keyring instead of importing derived AES bytes.
func Import(keyring *crypto.Keyring, session, metrics, dsn, databasePassword string, now time.Time) (*State, error) {
	if keyring == nil {
		return nil, fmt.Errorf("original credential keys are required for import")
	}
	s, err := Generate(now)
	if err != nil {
		return nil, err
	}
	s.ActiveKeyID = keyring.ActiveKeyID()
	s.CredentialKeys = make(map[string]CredentialKey)
	for id, secret := range keyring.Secrets() {
		s.CredentialKeys[id] = CredentialKey{Secret: secret, CreatedAt: now.UTC()}
	}
	if session != "" {
		s.SessionSecret = session
	}
	if metrics != "" {
		s.MetricsToken = metrics
	}
	s.DBDSN, s.DatabasePassword = dsn, databasePassword
	return s, s.Validate()
}

// Keyring constructs the same encryption context after restart or recovery.
func (s *State) Keyring() (*crypto.Keyring, error) {
	if s == nil {
		return nil, fmt.Errorf("instance state is missing")
	}
	keys := make(map[string]string, len(s.CredentialKeys))
	for id, key := range s.CredentialKeys {
		keys[id] = key.Secret
	}
	return crypto.NewKeyring(s.ActiveKeyID, keys)
}

// Validate rejects partial or unknown state rather than generating missing secrets.
func (s *State) Validate() error {
	if s == nil || s.Version != StateVersion {
		return fmt.Errorf("unsupported or missing instance state version")
	}
	if _, err := uuid.Parse(s.InstanceID); err != nil {
		return fmt.Errorf("invalid instance identifier")
	}
	if s.CreatedAt.IsZero() {
		return fmt.Errorf("instance creation time is missing")
	}
	if _, err := s.Keyring(); err != nil {
		return fmt.Errorf("invalid instance credential keyring: %w", err)
	}
	for _, key := range s.CredentialKeys {
		if key.CreatedAt.IsZero() {
			return fmt.Errorf("credential key creation time is missing")
		}
	}
	if strings.TrimSpace(s.SessionSecret) == "" || strings.TrimSpace(s.MetricsToken) == "" || len(s.SnapshotSecret) < 32 {
		return fmt.Errorf("instance operational secrets are missing")
	}
	if s.RecoveryRecipient != "" {
		if _, err := age.ParseX25519Recipient(s.RecoveryRecipient); err != nil {
			return fmt.Errorf("invalid instance recovery recipient")
		}
	}
	if s.Activation != nil {
		if err := s.Activation.Validate(); err != nil {
			return err
		}
	}
	if len(s.DeploymentMetadata) > 0 && (len(s.DeploymentMetadata) > 64<<10 || !json.Valid(s.DeploymentMetadata)) {
		return fmt.Errorf("invalid deployment metadata")
	}
	if len(s.ApplicationConfig) > 64<<10 || len(s.ApplicationConfig) > 0 && !json.Valid(s.ApplicationConfig) {
		return fmt.Errorf("invalid imported application configuration")
	}
	return nil
}

// RotateCredentials changes the active key only when due unless force is true.
// Historical keys are retained for old credentials and unconverted backups.
// Persist this state before rewrapping database values so a crash remains recoverable.
func (s *State) RotateCredentials(now time.Time, force bool) (bool, error) {
	if err := s.Validate(); err != nil {
		return false, err
	}
	if !force && now.Sub(s.CredentialKeys[s.ActiveKeyID].CreatedAt) < CredentialRotationInterval {
		return false, nil
	}
	secret, err := RandomSecret()
	if err != nil {
		return false, err
	}
	id := uuid.NewString()
	s.CredentialKeys[id] = CredentialKey{Secret: secret, CreatedAt: now.UTC()}
	s.ActiveKeyID = id
	return true, nil
}

// RandomSecret returns 256 bits from the system cryptographic random source.
func RandomSecret() (string, error) {
	var raw [32]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", fmt.Errorf("generate instance secret: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(raw[:]), nil
}
