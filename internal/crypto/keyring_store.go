package crypto

// This file defines persistent auto-generated encryption keyring storage, one-time onboarding state, and flexible restore key parsing.

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/lollinoo/theia/internal/secretinput"
)

const DefaultKeyringFileName = "encryption-keyring.json"

// PersistedKeyringState holds on-disk encryption keyring material when not managed externally.
type PersistedKeyringState struct {
	ActiveKeyID  string            `json:"active_key_id"`
	Keys         map[string]string `json:"keys"`
	Acknowledged bool              `json:"acknowledged"`
	CreatedAt    string            `json:"created_at"`
}

// EncryptionKeyStatus describes the current encryption key onboarding status for administrators.
// PendingSecret and EnvSnippet are only populated while Acknowledged is false.
type EncryptionKeyStatus struct {
	ActiveKeyID   string   `json:"active_key_id"`
	KeyIDs        []string `json:"key_ids"`
	Acknowledged  bool     `json:"acknowledged"`
	ManagedByEnv  bool     `json:"managed_by_env"`
	PendingSecret string   `json:"pending_secret,omitempty"`
	EnvSnippet    string   `json:"env_snippet,omitempty"`
}

// KeyringFilePath returns the canonical path for the persisted encryption keyring inside dataDir.
func KeyringFilePath(dataDir string) string {
	return filepath.Join(strings.TrimSpace(dataDir), DefaultKeyringFileName)
}

// HasEncryptionKeyEnv reports whether any THEIA_ENCRYPTION_KEY* environment or _FILE input is set.
func HasEncryptionKeyEnv() (bool, error) {
	for _, name := range []string{"THEIA_ENCRYPTION_KEY_ID", "THEIA_ENCRYPTION_KEYS", "THEIA_ENCRYPTION_KEY"} {
		val, err := secretinput.Read(name)
		if err != nil {
			return false, err
		}
		if strings.TrimSpace(val) != "" {
			return true, nil
		}
	}
	return false, nil
}

// LoadOrGenerateKeyring loads encryption keys from the environment when present, or loads/generates
// a persistent keyring in dataDir when no environment keys are configured.
func LoadOrGenerateKeyring(dataDir string) (*Keyring, error) {
	hasEnv, err := HasEncryptionKeyEnv()
	if err != nil {
		return nil, err
	}
	if hasEnv {
		kr, err := LoadKeyringFromEnv()
		if err != nil {
			return nil, err
		}
		if strings.TrimSpace(dataDir) != "" {
			if state, err := readPersistedKeyringState(KeyringFilePath(dataDir)); err == nil && state != nil && len(state.Keys) > 0 {
				return kr.WithAdditionalKeys(state.Keys)
			}
		}
		return kr, nil
	}

	if strings.TrimSpace(dataDir) == "" {
		return LoadKeyringFromEnv()
	}

	path := KeyringFilePath(dataDir)
	state, err := readPersistedKeyringState(path)
	if err != nil {
		return nil, err
	}
	if state != nil {
		return NewKeyring(state.ActiveKeyID, state.Keys)
	}

	now := time.Now().UTC()
	keyID, err := generateKeyID(now)
	if err != nil {
		return nil, err
	}
	secret, err := generateSecret()
	if err != nil {
		return nil, err
	}
	state = &PersistedKeyringState{
		ActiveKeyID:  keyID,
		Keys:         map[string]string{keyID: secret},
		Acknowledged: false,
		CreatedAt:    now.Format(time.RFC3339),
	}
	if err := writePersistedKeyringState(path, state); err != nil {
		return nil, err
	}
	return NewKeyring(state.ActiveKeyID, state.Keys)
}

// ReadEncryptionKeyStatus returns the current key status and one-time secret if not yet acknowledged.
func ReadEncryptionKeyStatus(dataDir string, fallback *Keyring) (*EncryptionKeyStatus, error) {
	hasEnv, err := HasEncryptionKeyEnv()
	if err != nil {
		return nil, err
	}
	if !hasEnv && strings.TrimSpace(dataDir) != "" {
		state, err := readPersistedKeyringState(KeyringFilePath(dataDir))
		if err != nil {
			return nil, err
		}
		if state != nil {
			kr, err := NewKeyring(state.ActiveKeyID, state.Keys)
			if err != nil {
				return nil, err
			}
			status := &EncryptionKeyStatus{
				ActiveKeyID:  kr.ActiveKeyID(),
				KeyIDs:       kr.KeyIDs(),
				Acknowledged: state.Acknowledged,
				ManagedByEnv: false,
			}
			if !state.Acknowledged {
				secret := state.Keys[state.ActiveKeyID]
				status.PendingSecret = fmt.Sprintf("%s=%s", state.ActiveKeyID, secret)
				status.EnvSnippet = fmt.Sprintf("THEIA_ENCRYPTION_KEY_ID=%s\nTHEIA_ENCRYPTION_KEYS=%s=%s", state.ActiveKeyID, state.ActiveKeyID, secret)
			}
			return status, nil
		}
	}

	if fallback == nil {
		return &EncryptionKeyStatus{
			Acknowledged: true,
			ManagedByEnv: hasEnv,
		}, nil
	}
	return &EncryptionKeyStatus{
		ActiveKeyID:  fallback.ActiveKeyID(),
		KeyIDs:       fallback.KeyIDs(),
		Acknowledged: true,
		ManagedByEnv: true,
	}, nil
}

// AcknowledgeEncryptionKey marks the auto-generated encryption key as saved by the administrator.
func AcknowledgeEncryptionKey(dataDir string) error {
	if strings.TrimSpace(dataDir) == "" {
		return nil
	}
	path := KeyringFilePath(dataDir)
	state, err := readPersistedKeyringState(path)
	if err != nil {
		return err
	}
	if state == nil {
		return nil
	}
	if state.Acknowledged {
		return nil
	}
	state.Acknowledged = true
	return writePersistedKeyringState(path, state)
}

// SaveAdditionalEncryptionKeys merges extra key_id=secret entries into the persisted keyring on disk
// and returns the updated Keyring.
func SaveAdditionalEncryptionKeys(dataDir string, current *Keyring, extra map[string]string) (*Keyring, error) {
	if len(extra) == 0 {
		if current != nil {
			return current, nil
		}
		return LoadOrGenerateKeyring(dataDir)
	}
	var updated *Keyring
	var err error
	if current != nil {
		updated, err = current.WithAdditionalKeys(extra)
		if err != nil {
			return nil, err
		}
	} else {
		for firstID := range extra {
			updated, err = NewKeyring(firstID, extra)
			break
		}
		if err != nil {
			return nil, err
		}
	}

	if strings.TrimSpace(dataDir) == "" {
		return updated, nil
	}

	path := KeyringFilePath(dataDir)
	state, err := readPersistedKeyringState(path)
	if err != nil {
		return nil, err
	}
	if state == nil {
		state = &PersistedKeyringState{
			ActiveKeyID:  updated.ActiveKeyID(),
			Keys:         updated.Secrets(),
			Acknowledged: true,
			CreatedAt:    time.Now().UTC().Format(time.RFC3339),
		}
	} else {
		if state.Keys == nil {
			state.Keys = make(map[string]string)
		}
		for id, secret := range extra {
			id = strings.TrimSpace(id)
			secret = strings.TrimSpace(secret)
			if id != "" && secret != "" {
				state.Keys[id] = secret
			}
		}
		if state.ActiveKeyID == "" {
			state.ActiveKeyID = updated.ActiveKeyID()
		}
	}
	if err := writePersistedKeyringState(path, state); err != nil {
		return nil, err
	}
	return updated, nil
}

// ParseFlexibleEncryptionKeys parses user-submitted backup encryption keys in any of these formats:
//   - "kid1=secret1,kid2=secret2" (comma or newline separated)
//   - "THEIA_ENCRYPTION_KEYS=kid1=secret1,kid2=secret2" (with optional THEIA_ENCRYPTION_KEY_ID line)
//   - "THEIA_ENCRYPTION_KEY=legacysecret"
//   - "<raw-secret>" when defaultKeyID is non-empty
func ParseFlexibleEncryptionKeys(input string, defaultKeyID string) (map[string]string, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return nil, fmt.Errorf("encryption key input cannot be empty")
	}
	defaultKeyID = strings.TrimSpace(defaultKeyID)
	secrets := make(map[string]string)

	lines := strings.FieldsFunc(input, func(r rune) bool {
		return r == '\n' || r == '\r'
	})
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if after, ok := strings.CutPrefix(line, "THEIA_ENCRYPTION_KEY_ID="); ok {
			if id := strings.TrimSpace(after); id != "" && defaultKeyID == "" {
				defaultKeyID = id
			}
			continue
		}
		if after, ok := strings.CutPrefix(line, "THEIA_ENCRYPTION_KEYS="); ok {
			line = strings.TrimSpace(after)
		}

		for _, part := range strings.Split(line, ",") {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			if id, secret, ok := strings.Cut(part, "="); ok {
				id = strings.TrimSpace(id)
				secret = strings.TrimSpace(secret)
				if id == "" || secret == "" {
					return nil, fmt.Errorf("malformed encryption key entry: expected key_id=secret")
				}
				secrets[id] = secret
			} else if defaultKeyID != "" {
				secrets[defaultKeyID] = part
			} else {
				return nil, fmt.Errorf("malformed encryption key entry: expected key_id=secret")
			}
		}
	}

	if len(secrets) == 0 {
		return nil, fmt.Errorf("no valid encryption keys found in input")
	}
	return secrets, nil
}

func readPersistedKeyringState(path string) (*PersistedKeyringState, error) {
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("stat keyring file: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return nil, fmt.Errorf("keyring file must be a regular file")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading keyring file: %w", err)
	}
	var state PersistedKeyringState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("parsing keyring file: %w", err)
	}
	if _, err := NewKeyring(state.ActiveKeyID, state.Keys); err != nil {
		return nil, fmt.Errorf("invalid persisted keyring: %w", err)
	}
	return &state, nil
}

func writePersistedKeyringState(path string, state *PersistedKeyringState) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("creating keyring directory: %w", err)
	}
	if info, err := os.Lstat(path); err == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return fmt.Errorf("keyring file destination must be a regular file")
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("stat keyring destination: %w", err)
	}

	payload, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling keyring state: %w", err)
	}
	tmp, err := os.CreateTemp(dir, ".encryption-keyring-*.tmp")
	if err != nil {
		return fmt.Errorf("creating temp keyring file: %w", err)
	}
	tmpName := tmp.Name()
	cleanup := func(cause error) error {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return cause
	}
	if err := tmp.Chmod(0o600); err != nil {
		return cleanup(fmt.Errorf("chmod temp keyring file: %w", err))
	}
	if _, err := tmp.Write(append(payload, '\n')); err != nil {
		return cleanup(fmt.Errorf("writing temp keyring file: %w", err))
	}
	if err := tmp.Sync(); err != nil {
		return cleanup(fmt.Errorf("syncing temp keyring file: %w", err))
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("closing temp keyring file: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("renaming keyring file: %w", err)
	}
	return nil
}

func generateKeyID(now time.Time) (string, error) {
	var raw [4]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", fmt.Errorf("generating encryption key id: %w", err)
	}
	return fmt.Sprintf("kid-%s-%s", now.UTC().Format("2006-01"), hex.EncodeToString(raw[:])), nil
}

func generateSecret() (string, error) {
	var raw [32]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", fmt.Errorf("generating encryption key secret: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(raw[:]), nil
}
