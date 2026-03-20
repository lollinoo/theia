package sqlite

import (
	"encoding/base64"
	"fmt"

	"github.com/lollinoo/theia/internal/crypto"
	"github.com/lollinoo/theia/internal/domain"
)

// encryptSNMPCredentials encrypts sensitive fields in SNMP credentials before storage.
// Modifies the struct in place. Encrypts: v2c community, v3 auth_password, v3 priv_password.
func encryptSNMPCredentials(creds *domain.SNMPCredentials, key []byte) error {
	if creds.V2c != nil && creds.V2c.Community != "" {
		encrypted, err := crypto.Encrypt([]byte(creds.V2c.Community), key)
		if err != nil {
			return fmt.Errorf("encrypting v2c community: %w", err)
		}
		creds.V2c.Community = base64.StdEncoding.EncodeToString(encrypted)
	}
	if creds.V3 != nil {
		if creds.V3.AuthPassword != "" {
			encrypted, err := crypto.Encrypt([]byte(creds.V3.AuthPassword), key)
			if err != nil {
				return fmt.Errorf("encrypting v3 auth password: %w", err)
			}
			creds.V3.AuthPassword = base64.StdEncoding.EncodeToString(encrypted)
		}
		if creds.V3.PrivPassword != "" {
			encrypted, err := crypto.Encrypt([]byte(creds.V3.PrivPassword), key)
			if err != nil {
				return fmt.Errorf("encrypting v3 priv password: %w", err)
			}
			creds.V3.PrivPassword = base64.StdEncoding.EncodeToString(encrypted)
		}
	}
	return nil
}

// decryptSNMPCredentials decrypts sensitive fields in SNMP credentials after reading from storage.
// Modifies the struct in place. Gracefully handles plaintext values (pre-migration data)
// by detecting base64 decode or decrypt failure and leaving the value unchanged.
func decryptSNMPCredentials(creds *domain.SNMPCredentials, key []byte) {
	if creds.V2c != nil && creds.V2c.Community != "" {
		if decrypted, ok := tryDecryptField(creds.V2c.Community, key); ok {
			creds.V2c.Community = decrypted
		}
	}
	if creds.V3 != nil {
		if creds.V3.AuthPassword != "" {
			if decrypted, ok := tryDecryptField(creds.V3.AuthPassword, key); ok {
				creds.V3.AuthPassword = decrypted
			}
		}
		if creds.V3.PrivPassword != "" {
			if decrypted, ok := tryDecryptField(creds.V3.PrivPassword, key); ok {
				creds.V3.PrivPassword = decrypted
			}
		}
	}
}

// tryDecryptField attempts to base64-decode and AES-GCM decrypt a field value.
// Returns (decrypted, true) on success, or ("", false) if the value is not encrypted
// (e.g., plaintext from pre-migration data).
func tryDecryptField(value string, key []byte) (string, bool) {
	ciphertext, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		return "", false // Not base64 = plaintext
	}
	plaintext, err := crypto.Decrypt(ciphertext, key)
	if err != nil {
		return "", false // Decrypt failed = plaintext that happened to be valid base64
	}
	return string(plaintext), true
}
