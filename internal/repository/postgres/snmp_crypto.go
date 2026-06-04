package postgres

import (
	"encoding/base64"
	"fmt"

	"github.com/lollinoo/theia/internal/crypto"
	"github.com/lollinoo/theia/internal/domain"
)

// deepCopySNMPCredentials returns a fully independent copy of SNMPCredentials,
// including the V2c and V3 pointer targets. This ensures that in-place encryption
// does not mutate the caller's original struct.
func deepCopySNMPCredentials(src domain.SNMPCredentials) domain.SNMPCredentials {
	dst := src
	if src.V2c != nil {
		v2c := *src.V2c
		dst.V2c = &v2c
	}
	if src.V3 != nil {
		v3 := *src.V3
		dst.V3 = &v3
	}
	return dst
}

// encryptSNMPCredentials encrypts sensitive fields in SNMP credentials before storage.
// Modifies the struct in place. Encrypts: v2c community, v3 auth_password, v3 priv_password.
func encryptSNMPCredentials(creds *domain.SNMPCredentials, keySource any) error {
	if creds.V2c != nil && creds.V2c.Community != "" {
		encrypted, err := encryptSensitiveSNMPField(creds.V2c.Community, keySource)
		if err != nil {
			return fmt.Errorf("encrypting v2c community: %w", err)
		}
		creds.V2c.Community = encrypted
	}
	if creds.V3 != nil {
		if creds.V3.AuthPassword != "" {
			encrypted, err := encryptSensitiveSNMPField(creds.V3.AuthPassword, keySource)
			if err != nil {
				return fmt.Errorf("encrypting v3 auth password: %w", err)
			}
			creds.V3.AuthPassword = encrypted
		}
		if creds.V3.PrivPassword != "" {
			encrypted, err := encryptSensitiveSNMPField(creds.V3.PrivPassword, keySource)
			if err != nil {
				return fmt.Errorf("encrypting v3 priv password: %w", err)
			}
			creds.V3.PrivPassword = encrypted
		}
	}
	return nil
}

// decryptSNMPCredentials decrypts sensitive fields in SNMP credentials after reading from storage.
// Modifies the struct in place.
//
// When given a *crypto.Keyring, decrypt is strict: sensitive values must be
// versioned envelopes and failures are returned to the caller. The []byte path
// is legacy compatibility for callers that have not been wired to Keyring yet.
func decryptSNMPCredentials(creds *domain.SNMPCredentials, keySource any) error {
	if creds.V2c != nil && creds.V2c.Community != "" {
		decrypted, err := decryptSensitiveSNMPField(creds.V2c.Community, keySource)
		if err != nil {
			return fmt.Errorf("decrypting v2c community: %w", err)
		}
		creds.V2c.Community = decrypted
	}
	if creds.V3 != nil {
		if creds.V3.AuthPassword != "" {
			decrypted, err := decryptSensitiveSNMPField(creds.V3.AuthPassword, keySource)
			if err != nil {
				return fmt.Errorf("decrypting v3 auth password: %w", err)
			}
			creds.V3.AuthPassword = decrypted
		}
		if creds.V3.PrivPassword != "" {
			decrypted, err := decryptSensitiveSNMPField(creds.V3.PrivPassword, keySource)
			if err != nil {
				return fmt.Errorf("decrypting v3 priv password: %w", err)
			}
			creds.V3.PrivPassword = decrypted
		}
	}
	return nil
}

func encryptSensitiveSNMPField(value string, keySource any) (string, error) {
	switch key := keySource.(type) {
	case *crypto.Keyring:
		return key.EncryptString(value)
	case []byte:
		encrypted, err := crypto.Encrypt([]byte(value), key)
		if err != nil {
			return "", err
		}
		return base64.StdEncoding.EncodeToString(encrypted), nil
	case nil:
		return "", fmt.Errorf("encryption keyring is required")
	default:
		return "", fmt.Errorf("unsupported encryption key source %T", keySource)
	}
}

func decryptSensitiveSNMPField(value string, keySource any) (string, error) {
	switch key := keySource.(type) {
	case *crypto.Keyring:
		if !crypto.IsEnvelope(value) {
			return "", fmt.Errorf("sensitive SNMP field is not a versioned encryption envelope")
		}
		return key.DecryptString(value)
	case []byte:
		if decrypted, ok := tryDecryptField(value, key); ok {
			return decrypted, nil
		}
		return value, nil
	case nil:
		return "", fmt.Errorf("encryption keyring is required")
	default:
		return "", fmt.Errorf("unsupported encryption key source %T", keySource)
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
