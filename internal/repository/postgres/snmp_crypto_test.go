package postgres

import (
	"encoding/base64"
	"strings"
	"testing"

	theiacrypto "github.com/lollinoo/theia/internal/crypto"
	"github.com/lollinoo/theia/internal/domain"
)

func TestEncryptSNMPCredentialsUsesEnvelopeAndDecryptsStrictly(t *testing.T) {
	keyring := mustTestKeyring(t, "kid-active", map[string]string{
		"kid-active": "active secret material",
	})
	creds := domain.SNMPCredentials{
		Version: domain.SNMPVersionV3,
		V3: &domain.SNMPv3Credentials{
			Username:      "admin",
			AuthProtocol:  "SHA",
			AuthPassword:  "auth-secret",
			PrivProtocol:  "AES",
			PrivPassword:  "priv-secret",
			SecurityLevel: "authPriv",
		},
	}

	if err := encryptSNMPCredentials(&creds, keyring); err != nil {
		t.Fatalf("encryptSNMPCredentials failed: %v", err)
	}
	if creds.V3.AuthPassword == "auth-secret" || creds.V3.PrivPassword == "priv-secret" {
		t.Fatal("sensitive SNMP fields should not remain plaintext")
	}
	if !strings.HasPrefix(creds.V3.AuthPassword, theiacrypto.EnvelopePrefix) {
		t.Fatalf("auth password should use envelope prefix, got %q", creds.V3.AuthPassword)
	}
	if !strings.HasPrefix(creds.V3.PrivPassword, theiacrypto.EnvelopePrefix) {
		t.Fatalf("priv password should use envelope prefix, got %q", creds.V3.PrivPassword)
	}

	if err := decryptSNMPCredentials(&creds, keyring); err != nil {
		t.Fatalf("decryptSNMPCredentials failed: %v", err)
	}
	if creds.V3.AuthPassword != "auth-secret" || creds.V3.PrivPassword != "priv-secret" {
		t.Fatalf("decrypted secrets = (%q, %q), want original values", creds.V3.AuthPassword, creds.V3.PrivPassword)
	}
	if creds.V3.Username != "admin" {
		t.Fatalf("non-sensitive username changed to %q", creds.V3.Username)
	}
}

func TestDecryptSNMPCredentialsRejectsPlaintextSensitiveValue(t *testing.T) {
	keyring := mustTestKeyring(t, "kid-active", map[string]string{
		"kid-active": "active secret material",
	})
	creds := domain.SNMPCredentials{
		Version: domain.SNMPVersionV2c,
		V2c:     &domain.SNMPv2cCredentials{Community: "public"},
	}

	err := decryptSNMPCredentials(&creds, keyring)
	if err == nil {
		t.Fatal("decryptSNMPCredentials should reject plaintext sensitive values")
	}
	if !strings.Contains(err.Error(), "v2c community") {
		t.Fatalf("error should name the field, got: %v", err)
	}
}

func TestDecryptSNMPCredentialsRejectsLegacyCiphertextInStrictRead(t *testing.T) {
	keyring := mustTestKeyring(t, "kid-active", map[string]string{
		"kid-active": "active secret material",
		"legacy":     "legacy secret material",
	})
	legacyRaw, err := theiacrypto.Encrypt([]byte("legacy-community"), theiacrypto.DeriveKey("legacy secret material"))
	if err != nil {
		t.Fatalf("legacy Encrypt failed: %v", err)
	}
	creds := domain.SNMPCredentials{
		Version: domain.SNMPVersionV2c,
		V2c:     &domain.SNMPv2cCredentials{Community: base64.StdEncoding.EncodeToString(legacyRaw)},
	}

	err = decryptSNMPCredentials(&creds, keyring)
	if err == nil {
		t.Fatal("strict decrypt should reject legacy ciphertext outside migration")
	}
	if strings.Contains(err.Error(), "legacy-community") {
		t.Fatalf("error must not include secret material, got: %v", err)
	}
}

func mustTestKeyring(t *testing.T, activeID string, secrets map[string]string) *theiacrypto.Keyring {
	t.Helper()
	keyring, err := theiacrypto.NewKeyring(activeID, secrets)
	if err != nil {
		t.Fatalf("NewKeyring failed: %v", err)
	}
	return keyring
}
