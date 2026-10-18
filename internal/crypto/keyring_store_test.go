package crypto

import (
	"testing"
)

func TestLoadOrGenerateKeyring_PersistsAndAcknowledges(t *testing.T) {
	t.Setenv("THEIA_ENCRYPTION_KEY_ID", "")
	t.Setenv("THEIA_ENCRYPTION_KEYS", "")
	t.Setenv("THEIA_ENCRYPTION_KEY", "")

	dataDir := t.TempDir()

	kr1, err := LoadOrGenerateKeyring(dataDir)
	if err != nil {
		t.Fatalf("LoadOrGenerateKeyring first call: %v", err)
	}
	if kr1.ActiveKeyID() == "" {
		t.Fatal("expected non-empty active key id")
	}

	status, err := ReadEncryptionKeyStatus(dataDir, kr1)
	if err != nil {
		t.Fatalf("ReadEncryptionKeyStatus: %v", err)
	}
	if status.Acknowledged {
		t.Fatal("expected unacknowledged status on initial generation")
	}
	if status.PendingSecret == "" {
		t.Fatal("expected pending_secret before acknowledgement")
	}

	// Second call reuses the exact same persisted key
	kr2, err := LoadOrGenerateKeyring(dataDir)
	if err != nil {
		t.Fatalf("LoadOrGenerateKeyring second call: %v", err)
	}
	if kr2.ActiveKeyID() != kr1.ActiveKeyID() {
		t.Fatalf("expected active key id %s, got %s", kr1.ActiveKeyID(), kr2.ActiveKeyID())
	}

	if err := AcknowledgeEncryptionKey(dataDir); err != nil {
		t.Fatalf("AcknowledgeEncryptionKey: %v", err)
	}

	statusAfter, err := ReadEncryptionKeyStatus(dataDir, kr2)
	if err != nil {
		t.Fatalf("ReadEncryptionKeyStatus after ack: %v", err)
	}
	if !statusAfter.Acknowledged {
		t.Fatal("expected acknowledged status after AcknowledgeEncryptionKey")
	}
	if statusAfter.PendingSecret != "" {
		t.Fatal("expected pending_secret to be cleared after acknowledgement")
	}
}

func TestParseFlexibleEncryptionKeys_SupportsEnvSnippetAndKeyringPairs(t *testing.T) {
	snippet := "# Theia recovery file\nTHEIA_ENCRYPTION_KEY_ID=k-20261018-abcd1234\nTHEIA_ENCRYPTION_KEYS=k-20261018-abcd1234=my-strong-secret-value-123456\n"
	parsed, err := ParseFlexibleEncryptionKeys(snippet, "")
	if err != nil {
		t.Fatalf("ParseFlexibleEncryptionKeys(snippet): %v", err)
	}
	if got := parsed["k-20261018-abcd1234"]; got != "my-strong-secret-value-123456" {
		t.Fatalf("got %q, want %q", got, "my-strong-secret-value-123456")
	}

	rawOnly, err := ParseFlexibleEncryptionKeys("raw-secret-without-id", "k-backup-1")
	if err != nil {
		t.Fatalf("ParseFlexibleEncryptionKeys(raw): %v", err)
	}
	if got := rawOnly["k-backup-1"]; got != "raw-secret-without-id" {
		t.Fatalf("got %q, want %q", got, "raw-secret-without-id")
	}
}

func TestSaveAdditionalEncryptionKeys_MergesIntoPersistedKeyring(t *testing.T) {
	t.Setenv("THEIA_ENCRYPTION_KEY_ID", "")
	t.Setenv("THEIA_ENCRYPTION_KEYS", "")
	t.Setenv("THEIA_ENCRYPTION_KEY", "")

	dataDir := t.TempDir()
	kr, err := LoadOrGenerateKeyring(dataDir)
	if err != nil {
		t.Fatalf("LoadOrGenerateKeyring: %v", err)
	}

	merged, err := SaveAdditionalEncryptionKeys(dataDir, kr, map[string]string{
		"k-restored-1": "restored-secret-material-123456",
	})
	if err != nil {
		t.Fatalf("SaveAdditionalEncryptionKeys: %v", err)
	}
	if !merged.HasKey("k-restored-1") {
		t.Fatal("expected merged keyring to contain k-restored-1")
	}

	reloaded, err := LoadOrGenerateKeyring(dataDir)
	if err != nil {
		t.Fatalf("LoadOrGenerateKeyring after save: %v", err)
	}
	if !reloaded.HasKey("k-restored-1") {
		t.Fatal("expected reloaded keyring to contain k-restored-1")
	}
}
