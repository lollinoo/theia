package security

// This file defines bridge secret security policy helpers and trust-boundary handling.

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strings"
)

const (
	BridgeSecretPrefixValue = "theia_bridge_"
	bridgeSecretPublicBytes = 8
	bridgeSecretRandomBytes = 32
	bridgeSecretHashPrefix  = "sha256:"
)

// ErrInvalidBridgeSecret stores shared err invalid bridge secret state for the security boundary.
var ErrInvalidBridgeSecret = errors.New("invalid bridge secret")

// GenerateBridgeSecret returns a raw secret and a safe display prefix.
// The raw secret is shown once to the user. Only the prefix and hash should be stored.
func GenerateBridgeSecret() (rawSecret string, displayPrefix string, err error) {
	publicID := make([]byte, bridgeSecretPublicBytes)
	if _, err := io.ReadFull(rand.Reader, publicID); err != nil {
		return "", "", fmt.Errorf("generating bridge secret prefix: %w", err)
	}
	random := make([]byte, bridgeSecretRandomBytes)
	if _, err := io.ReadFull(rand.Reader, random); err != nil {
		return "", "", fmt.Errorf("generating bridge secret: %w", err)
	}
	displayPrefix = BridgeSecretPrefixValue + hex.EncodeToString(publicID)
	rawSecret = displayPrefix + "." + base64.RawURLEncoding.EncodeToString(random)
	return rawSecret, displayPrefix, nil
}

// BridgeSecretPrefix extracts the safe lookup/display prefix from a raw secret.
func BridgeSecretPrefix(rawSecret string) (string, error) {
	rawSecret = strings.TrimSpace(rawSecret)
	prefix, token, ok := strings.Cut(rawSecret, ".")
	if !ok || token == "" {
		return "", ErrInvalidBridgeSecret
	}
	if !strings.HasPrefix(prefix, BridgeSecretPrefixValue) {
		return "", ErrInvalidBridgeSecret
	}
	publicPart := strings.TrimPrefix(prefix, BridgeSecretPrefixValue)
	if len(publicPart) != bridgeSecretPublicBytes*2 {
		return "", ErrInvalidBridgeSecret
	}
	if _, err := hex.DecodeString(publicPart); err != nil {
		return "", ErrInvalidBridgeSecret
	}
	decoded, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil || len(decoded) < bridgeSecretRandomBytes {
		return "", ErrInvalidBridgeSecret
	}
	return prefix, nil
}

// HashBridgeSecret hashes a high-entropy Bridge Secret for storage.
func HashBridgeSecret(rawSecret string) (string, error) {
	if _, err := BridgeSecretPrefix(rawSecret); err != nil {
		return "", err
	}
	sum := sha256.Sum256([]byte(strings.TrimSpace(rawSecret)))
	return bridgeSecretHashPrefix + base64.RawURLEncoding.EncodeToString(sum[:]), nil
}

// VerifyBridgeSecret compares a raw Bridge Secret with a stored hash in constant time.
func VerifyBridgeSecret(rawSecret, storedHash string) bool {
	actualHash, err := HashBridgeSecret(rawSecret)
	if err != nil {
		return false
	}
	expectedEncoded := strings.TrimPrefix(strings.TrimSpace(storedHash), bridgeSecretHashPrefix)
	if expectedEncoded == storedHash {
		return false
	}
	actualEncoded := strings.TrimPrefix(actualHash, bridgeSecretHashPrefix)
	expected, err := base64.RawURLEncoding.DecodeString(expectedEncoded)
	if err != nil {
		return false
	}
	actual, err := base64.RawURLEncoding.DecodeString(actualEncoded)
	if err != nil {
		return false
	}
	return subtle.ConstantTimeCompare(actual, expected) == 1
}
