package crypto

import (
	"bytes"
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
