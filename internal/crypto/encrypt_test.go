package crypto

import (
	"bytes"
	"encoding/base64"
	"strings"
	"testing"
)

func TestEncryptDecryptRoundTrip(t *testing.T) {
	key := DeriveKey("test-passphrase")
	plaintext := []byte("super secret SSH password")

	ciphertext, err := Encrypt(plaintext, key)
	if err != nil {
		t.Fatalf("Encrypt failed: %v", err)
	}

	if bytes.Equal(plaintext, ciphertext) {
		t.Fatal("ciphertext should differ from plaintext")
	}

	decrypted, err := Decrypt(ciphertext, key)
	if err != nil {
		t.Fatalf("Decrypt failed: %v", err)
	}

	if !bytes.Equal(plaintext, decrypted) {
		t.Fatalf("decrypted text does not match: got %q, want %q", decrypted, plaintext)
	}
}

func TestDecryptWrongKey(t *testing.T) {
	key1 := DeriveKey("key-one")
	key2 := DeriveKey("key-two")

	ciphertext, err := Encrypt([]byte("secret"), key1)
	if err != nil {
		t.Fatalf("Encrypt failed: %v", err)
	}

	_, err = Decrypt(ciphertext, key2)
	if err == nil {
		t.Fatal("Decrypt with wrong key should fail")
	}
}

func TestDecryptTooShort(t *testing.T) {
	key := DeriveKey("key")
	_, err := Decrypt([]byte("short"), key)
	if err == nil {
		t.Fatal("Decrypt with short ciphertext should fail")
	}
}

func TestDeriveKey(t *testing.T) {
	key := DeriveKey("hello")
	if len(key) != 32 {
		t.Fatalf("expected 32-byte key, got %d", len(key))
	}

	// Same passphrase should produce same key
	key2 := DeriveKey("hello")
	if !bytes.Equal(key, key2) {
		t.Fatal("same passphrase should produce same key")
	}

	// Different passphrase should produce different key
	key3 := DeriveKey("world")
	if bytes.Equal(key, key3) {
		t.Fatal("different passphrases should produce different keys")
	}
}

func TestLoadEncryptionKeyRequired(t *testing.T) {
	t.Setenv("THEIA_ENCRYPTION_KEY", "")
	_, err := LoadEncryptionKey()
	if err == nil {
		t.Fatal("expected error when THEIA_ENCRYPTION_KEY is unset")
	}
	if !strings.Contains(err.Error(), "THEIA_ENCRYPTION_KEY") {
		t.Fatalf("error should mention THEIA_ENCRYPTION_KEY, got: %v", err)
	}
}

func TestLoadEncryptionKeySet(t *testing.T) {
	t.Setenv("THEIA_ENCRYPTION_KEY", "my-test-key")
	key, err := LoadEncryptionKey()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(key) != 32 {
		t.Fatalf("expected 32-byte key, got %d", len(key))
	}
	expected := DeriveKey("my-test-key")
	if !bytes.Equal(key, expected) {
		t.Fatal("key should equal DeriveKey(\"my-test-key\")")
	}
}

func TestKeyringEnvelopeRoundTripIncludesVersionedMetadata(t *testing.T) {
	keyring := mustKeyring(t, "kid-2026", map[string]string{
		"kid-2026": "correct horse battery staple",
	})

	ciphertext, err := keyring.EncryptString("super secret SSH password")
	if err != nil {
		t.Fatalf("EncryptString failed: %v", err)
	}

	if !strings.HasPrefix(ciphertext, EnvelopePrefix) {
		t.Fatalf("ciphertext prefix = %q, want %q", ciphertext[:min(len(ciphertext), len(EnvelopePrefix))], EnvelopePrefix)
	}
	if strings.Contains(ciphertext, "super secret") {
		t.Fatal("ciphertext should not contain plaintext")
	}

	env := mustDecodeEnvelope(t, ciphertext)
	if env.Version != 1 {
		t.Fatalf("version = %d, want 1", env.Version)
	}
	if env.KeyID != "kid-2026" {
		t.Fatalf("key_id = %q, want kid-2026", env.KeyID)
	}
	if env.Algorithm != "AES-256-GCM" {
		t.Fatalf("algorithm = %q, want AES-256-GCM", env.Algorithm)
	}
	if env.KDF.Name != "argon2id" {
		t.Fatalf("kdf.name = %q, want argon2id", env.KDF.Name)
	}
	if env.KDF.Time == 0 || env.KDF.MemoryKiB == 0 || env.KDF.Parallelism == 0 || env.KDF.KeyLength != 32 {
		t.Fatalf("unexpected kdf params: %#v", env.KDF)
	}
	if env.Salt == "" || env.Nonce == "" || env.Ciphertext == "" {
		t.Fatalf("envelope missing binary fields: %#v", env)
	}

	plaintext, err := keyring.DecryptString(ciphertext)
	if err != nil {
		t.Fatalf("DecryptString failed: %v", err)
	}
	if plaintext != "super secret SSH password" {
		t.Fatalf("plaintext = %q, want original secret", plaintext)
	}
}

func TestKeyringDecryptFailsWhenKeyIDMissing(t *testing.T) {
	writer := mustKeyring(t, "kid-old", map[string]string{
		"kid-old": "old secret material",
	})
	reader := mustKeyring(t, "kid-new", map[string]string{
		"kid-new": "new secret material",
	})

	ciphertext, err := writer.EncryptString("secret")
	if err != nil {
		t.Fatalf("EncryptString failed: %v", err)
	}

	_, err = reader.DecryptString(ciphertext)
	if err == nil {
		t.Fatal("DecryptString should fail when key id is not configured")
	}
	if !strings.Contains(err.Error(), "kid-old") {
		t.Fatalf("error should identify missing key id, got: %v", err)
	}
}

func TestKeyringTamperedMetadataFailsAuthentication(t *testing.T) {
	keyring := mustKeyring(t, "kid-2026", map[string]string{
		"kid-2026": "correct horse battery staple",
	})
	ciphertext, err := keyring.EncryptString("secret")
	if err != nil {
		t.Fatalf("EncryptString failed: %v", err)
	}
	env := mustDecodeEnvelope(t, ciphertext)
	env.Algorithm = "AES-128-GCM"
	tampered := mustEncodeEnvelope(t, env)

	_, err = keyring.DecryptString(tampered)
	if err == nil {
		t.Fatal("DecryptString should fail when authenticated metadata is tampered")
	}
}

func TestKeyringLegacyCiphertextRequiresCompatibilityPathAndRewraps(t *testing.T) {
	keyring := mustKeyring(t, "kid-new", map[string]string{
		"kid-new": "new secret material",
		"legacy":  "legacy passphrase",
	})
	legacyRaw, err := Encrypt([]byte("legacy secret"), DeriveKey("legacy passphrase"))
	if err != nil {
		t.Fatalf("legacy Encrypt failed: %v", err)
	}
	legacyCiphertext := base64.StdEncoding.EncodeToString(legacyRaw)

	if _, err := keyring.DecryptString(legacyCiphertext); err == nil {
		t.Fatal("strict DecryptString should reject legacy raw ciphertext")
	}

	plaintext, legacyKeyID, err := keyring.DecryptLegacyString(legacyCiphertext)
	if err != nil {
		t.Fatalf("DecryptLegacyString failed: %v", err)
	}
	if plaintext != "legacy secret" || legacyKeyID != "legacy" {
		t.Fatalf("legacy decrypt = (%q, %q), want plaintext and legacy key id", plaintext, legacyKeyID)
	}

	rewrapped, err := keyring.RewrapString(legacyCiphertext)
	if err != nil {
		t.Fatalf("RewrapString failed: %v", err)
	}
	if !strings.HasPrefix(rewrapped, EnvelopePrefix) {
		t.Fatalf("rewrapped value should use envelope prefix, got %q", rewrapped)
	}
	if got := mustDecodeEnvelope(t, rewrapped).KeyID; got != "kid-new" {
		t.Fatalf("rewrapped key_id = %q, want active kid-new", got)
	}
}

func TestLoadKeyringFromEnvParsesNewVariablesAndLegacyFallback(t *testing.T) {
	t.Setenv("THEIA_ENCRYPTION_KEY_ID", "kid-b")
	t.Setenv("THEIA_ENCRYPTION_KEYS", "kid-a=old-secret,kid-b=new-secret")
	t.Setenv("THEIA_ENCRYPTION_KEY", "")

	keyring, err := LoadKeyringFromEnv()
	if err != nil {
		t.Fatalf("LoadKeyringFromEnv failed: %v", err)
	}
	if got := keyring.ActiveKeyID(); got != "kid-b" {
		t.Fatalf("ActiveKeyID = %q, want kid-b", got)
	}
	if !keyring.HasKey("kid-a") || !keyring.HasKey("kid-b") {
		t.Fatal("keyring should contain both configured keys")
	}

	t.Setenv("THEIA_ENCRYPTION_KEY_ID", "kid-c")
	t.Setenv("THEIA_ENCRYPTION_KEYS", "kid-c=new-secret")
	t.Setenv("THEIA_ENCRYPTION_KEY", "legacy-only-secret")

	keyring, err = LoadKeyringFromEnv()
	if err != nil {
		t.Fatalf("LoadKeyringFromEnv merged legacy fallback failed: %v", err)
	}
	if got := keyring.ActiveKeyID(); got != "kid-c" {
		t.Fatalf("merged fallback ActiveKeyID = %q, want kid-c", got)
	}
	if !keyring.HasKey("kid-c") || !keyring.HasKey(LegacyKeyID) {
		t.Fatal("keyring should include configured active key and legacy fallback key")
	}

	t.Setenv("THEIA_ENCRYPTION_KEY_ID", "")
	t.Setenv("THEIA_ENCRYPTION_KEYS", "")
	t.Setenv("THEIA_ENCRYPTION_KEY", "legacy-only-secret")

	keyring, err = LoadKeyringFromEnv()
	if err != nil {
		t.Fatalf("LoadKeyringFromEnv legacy fallback failed: %v", err)
	}
	if got := keyring.ActiveKeyID(); got != LegacyKeyID {
		t.Fatalf("legacy ActiveKeyID = %q, want %q", got, LegacyKeyID)
	}
	ciphertext, err := keyring.EncryptString("new write")
	if err != nil {
		t.Fatalf("EncryptString with legacy fallback failed: %v", err)
	}
	if !strings.HasPrefix(ciphertext, EnvelopePrefix) {
		t.Fatalf("legacy fallback new write should use envelope, got %q", ciphertext)
	}
}

func TestParseKeyringRejectsMalformedLists(t *testing.T) {
	tests := []struct {
		name     string
		activeID string
		keys     string
	}{
		{name: "missing active key", activeID: "kid-missing", keys: "kid-a=secret"},
		{name: "empty key id", activeID: "kid-a", keys: "=secret"},
		{name: "empty secret", activeID: "kid-a", keys: "kid-a="},
		{name: "duplicate key id", activeID: "kid-a", keys: "kid-a=secret,kid-a=other"},
		{name: "malformed pair", activeID: "kid-a", keys: "kid-a"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := ParseKeyring(tt.activeID, tt.keys); err == nil {
				t.Fatal("ParseKeyring should fail")
			}
		})
	}
}

func mustKeyring(t *testing.T, activeID string, secrets map[string]string) *Keyring {
	t.Helper()
	keyring, err := NewKeyring(activeID, secrets)
	if err != nil {
		t.Fatalf("NewKeyring failed: %v", err)
	}
	return keyring
}

func mustDecodeEnvelope(t *testing.T, value string) envelope {
	t.Helper()
	env, err := decodeEnvelope(value)
	if err != nil {
		t.Fatalf("decodeEnvelope failed: %v", err)
	}
	return env
}

func mustEncodeEnvelope(t *testing.T, env envelope) string {
	t.Helper()
	value, err := encodeEnvelope(env)
	if err != nil {
		t.Fatalf("encodeEnvelope failed: %v", err)
	}
	return value
}
