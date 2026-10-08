package instance

import (
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lollinoo/theia/internal/crypto"
)

func TestImportedCredentialsRemainRecoverableAfterRotationAndRestart(t *testing.T) {
	now := time.Now().UTC()
	original := "original-legacy-passphrase-with-=,+-characters"
	raw, err := crypto.Encrypt([]byte("device-password"), crypto.DeriveKey(original))
	if err != nil {
		t.Fatal(err)
	}
	legacy := base64.StdEncoding.EncodeToString(raw)
	keyring, err := crypto.NewKeyring(crypto.LegacyKeyID, map[string]string{crypto.LegacyKeyID: original})
	if err != nil {
		t.Fatal(err)
	}
	state, err := Import(keyring, "original-session", "original-metrics", "original-dsn", "original-db-password", now)
	if err != nil {
		t.Fatal(err)
	}
	store := Store{Path: filepath.Join(t.TempDir(), "state", "secrets.json")}
	if err := store.Create(state); err != nil {
		t.Fatal(err)
	}
	if err := store.Update(func(s *State) error {
		changed, err := s.RotateCredentials(now.Add(CredentialRotationInterval), false)
		if !changed {
			t.Error("due credential key did not rotate")
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
	restarted, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	keys, err := restarted.Keyring()
	if err != nil {
		t.Fatal(err)
	}
	plaintext, _, err := keys.DecryptLegacyString(legacy)
	if err != nil || plaintext != "device-password" {
		t.Fatalf("legacy credential lost: %v", err)
	}
	rewrapped, err := keys.RewrapString(legacy)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := keys.DecryptString(rewrapped); err != nil || got != plaintext {
		t.Fatalf("rewrapped credential lost: %v", err)
	}
	if restarted.SessionSecret != state.SessionSecret || restarted.DatabasePassword != state.DatabasePassword || restarted.SnapshotSecret != state.SnapshotSecret {
		t.Fatal("credential rotation changed operational secrets")
	}
	if _, err := state.Keyring(); err != nil {
		t.Fatal(err)
	}
	if state.CredentialKeys[crypto.LegacyKeyID].Secret != original {
		t.Fatal("original passphrase was changed")
	}
}

func TestMissingOrCorruptStateCannotBeReplaced(t *testing.T) {
	store := Store{Path: filepath.Join(t.TempDir(), "secrets.json")}
	if _, err := store.Load(); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing state: %v", err)
	}
	if err := os.WriteFile(store.Path, []byte("interrupted-secret-state"), 0600); err != nil {
		t.Fatal(err)
	}
	newState, err := Generate(time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Create(newState); err == nil {
		t.Fatal("corrupt state was silently replaced")
	}
	if err := store.Update(func(s *State) error { return nil }); err == nil {
		t.Fatal("corrupt state was silently accepted")
	}
	data, err := os.ReadFile(store.Path)
	if err != nil || string(data) != "interrupted-secret-state" {
		t.Fatal("original state was modified")
	}
}

func TestFailedUpdatePreservesOriginalStateAndPrivatePermissions(t *testing.T) {
	store := Store{Path: filepath.Join(t.TempDir(), "state", "secrets.json")}
	state, err := Generate(time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Create(state); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(store.Path)
	if err := store.Update(func(s *State) error { s.SessionSecret = ""; return nil }); err == nil {
		t.Fatal("invalid update succeeded")
	}
	after, _ := os.ReadFile(store.Path)
	if string(before) != string(after) {
		t.Fatal("failed update changed state")
	}
	for path, mode := range map[string]os.FileMode{store.Path: 0600, filepath.Dir(store.Path): 0700} {
		info, err := os.Stat(path)
		if err != nil || info.Mode().Perm() != mode {
			t.Fatalf("private permissions: %v", err)
		}
	}
}

func TestStateRejectsSymlinksAndLeakedPermissions(t *testing.T) {
	dir := t.TempDir()
	if err := os.Symlink(dir, filepath.Join(dir, "linked")); err != nil {
		t.Fatal(err)
	}
	state, _ := Generate(time.Now())
	if err := (Store{Path: filepath.Join(dir, "linked", "secrets.json")}).Create(state); err == nil {
		t.Fatal("symlink storage accepted")
	}
	store := Store{Path: filepath.Join(dir, "secrets.json")}
	if err := store.Create(state); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(store.Path, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Load(); err == nil {
		t.Fatal("world-readable secrets accepted")
	}
}

func TestKeyParseErrorsDoNotExposeSecretValues(t *testing.T) {
	secret := "private-value-that-must-not-appear-in-logs"
	_, err := crypto.ParseKeyring("active", secret)
	if err == nil || strings.Contains(err.Error(), secret) {
		t.Fatal("parse error exposes secret")
	}
}
