package crypto

// This file defines keyring cryptographic storage and key-handling behavior.

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"golang.org/x/crypto/argon2"
)

const (
	EnvelopePrefix = "theia:v1:"
	LegacyKeyID    = "legacy"

	envelopeVersion = 1
	envelopeAlg     = "AES-256-GCM"
	kdfArgon2id     = "argon2id"
)

var defaultKDFParams = kdfParams{
	Name:        kdfArgon2id,
	Time:        1,
	MemoryKiB:   64 * 1024,
	Parallelism: 4,
	KeyLength:   32,
}

// Keyring represents keyring data used by the cryptographic storage.
type Keyring struct {
	activeID string
	keys     map[string]string
}

type envelope struct {
	Version    int       `json:"version"`
	KeyID      string    `json:"key_id"`
	Algorithm  string    `json:"algorithm"`
	KDF        kdfParams `json:"kdf"`
	Salt       string    `json:"salt"`
	Nonce      string    `json:"nonce"`
	Ciphertext string    `json:"ciphertext"`
}

type kdfParams struct {
	Name        string `json:"name"`
	Time        uint32 `json:"time"`
	MemoryKiB   uint32 `json:"memory_kib"`
	Parallelism uint8  `json:"parallelism"`
	KeyLength   uint32 `json:"key_length"`
}

type envelopeAAD struct {
	Version   int       `json:"version"`
	KeyID     string    `json:"key_id"`
	Algorithm string    `json:"algorithm"`
	KDF       kdfParams `json:"kdf"`
	Salt      string    `json:"salt"`
	Nonce     string    `json:"nonce"`
}

// NewKeyring constructs keyring state for the cryptographic storage.
func NewKeyring(activeID string, secrets map[string]string) (*Keyring, error) {
	activeID = strings.TrimSpace(activeID)
	if activeID == "" {
		return nil, fmt.Errorf("active encryption key id is required")
	}
	if len(secrets) == 0 {
		return nil, fmt.Errorf("at least one encryption key is required")
	}

	keys := make(map[string]string, len(secrets))
	for id, secret := range secrets {
		id = strings.TrimSpace(id)
		secret = strings.TrimSpace(secret)
		if id == "" {
			return nil, fmt.Errorf("encryption key id cannot be empty")
		}
		if secret == "" {
			return nil, fmt.Errorf("encryption key %q secret cannot be empty", id)
		}
		keys[id] = secret
	}
	if _, ok := keys[activeID]; !ok {
		return nil, fmt.Errorf("active encryption key id %q is not configured", activeID)
	}

	return &Keyring{activeID: activeID, keys: keys}, nil
}

// NewKeyringFromLegacyKey constructs keyring from legacy key state for the cryptographic storage.
func NewKeyringFromLegacyKey(key []byte) (*Keyring, error) {
	return NewKeyring(LegacyKeyID, map[string]string{
		LegacyKeyID: base64.StdEncoding.EncodeToString(key),
	})
}

// ParseKeyring parses keyring input for the cryptographic storage.
func ParseKeyring(activeID, keyList string) (*Keyring, error) {
	activeID = strings.TrimSpace(activeID)
	keyList = strings.TrimSpace(keyList)
	if activeID == "" {
		return nil, fmt.Errorf("THEIA_ENCRYPTION_KEY_ID is required when THEIA_ENCRYPTION_KEYS is set")
	}
	if keyList == "" {
		return nil, fmt.Errorf("THEIA_ENCRYPTION_KEYS is required when THEIA_ENCRYPTION_KEY_ID is set")
	}

	secrets := make(map[string]string)
	for _, rawPair := range strings.Split(keyList, ",") {
		rawPair = strings.TrimSpace(rawPair)
		id, secret, ok := strings.Cut(rawPair, "=")
		if !ok {
			return nil, fmt.Errorf("malformed encryption key entry %q: expected key_id=secret", rawPair)
		}
		id = strings.TrimSpace(id)
		secret = strings.TrimSpace(secret)
		if id == "" {
			return nil, fmt.Errorf("encryption key id cannot be empty")
		}
		if secret == "" {
			return nil, fmt.Errorf("encryption key %q secret cannot be empty", id)
		}
		if _, exists := secrets[id]; exists {
			return nil, fmt.Errorf("duplicate encryption key id %q", id)
		}
		secrets[id] = secret
	}
	return NewKeyring(activeID, secrets)
}

// LoadKeyringFromEnv loads keyring from env data for the cryptographic storage.
func LoadKeyringFromEnv() (*Keyring, error) {
	activeID := os.Getenv("THEIA_ENCRYPTION_KEY_ID")
	keyList := os.Getenv("THEIA_ENCRYPTION_KEYS")
	if strings.TrimSpace(activeID) != "" || strings.TrimSpace(keyList) != "" {
		keyring, err := ParseKeyring(activeID, keyList)
		if err != nil {
			return nil, err
		}
		legacySecret := strings.TrimSpace(os.Getenv("THEIA_ENCRYPTION_KEY"))
		if legacySecret == "" || keyring.HasKey(LegacyKeyID) {
			return keyring, nil
		}
		secrets := make(map[string]string, len(keyring.keys)+1)
		for id, secret := range keyring.keys {
			secrets[id] = secret
		}
		secrets[LegacyKeyID] = legacySecret
		return NewKeyring(keyring.activeID, secrets)
	}

	legacySecret := strings.TrimSpace(os.Getenv("THEIA_ENCRYPTION_KEY"))
	if legacySecret == "" {
		return nil, fmt.Errorf(
			"THEIA_ENCRYPTION_KEY_ID and THEIA_ENCRYPTION_KEYS are required. " +
				"For legacy deployments, THEIA_ENCRYPTION_KEY may be set as a fallback.")
	}
	return NewKeyring(LegacyKeyID, map[string]string{LegacyKeyID: legacySecret})
}

func (k *Keyring) ActiveKeyID() string {
	if k == nil {
		return ""
	}
	return k.activeID
}

func (k *Keyring) HasKey(id string) bool {
	if k == nil {
		return false
	}
	_, ok := k.keys[id]
	return ok
}

func (k *Keyring) KeyIDs() []string {
	if k == nil {
		return nil
	}
	ids := make([]string, 0, len(k.keys))
	for id := range k.keys {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func (k *Keyring) LegacyKeyHashes() []string {
	if k == nil {
		return nil
	}
	hashes := make([]string, 0, len(k.keys))
	for _, secret := range k.keys {
		hashes = append(hashes, legacyKeyHash(DeriveKey(secret)))
	}
	sort.Strings(hashes)
	return hashes
}

func (k *Keyring) EncryptString(plaintext string) (string, error) {
	if k == nil {
		return "", fmt.Errorf("encryption keyring is nil")
	}
	secret, ok := k.keys[k.activeID]
	if !ok {
		return "", fmt.Errorf("active encryption key id %q is not configured", k.activeID)
	}

	salt := make([]byte, 16)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return "", fmt.Errorf("generating kdf salt: %w", err)
	}

	key := deriveEnvelopeKey(secret, salt, defaultKDFParams)
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("creating cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("creating GCM: %w", err)
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("generating nonce: %w", err)
	}

	env := envelope{
		Version:   envelopeVersion,
		KeyID:     k.activeID,
		Algorithm: envelopeAlg,
		KDF:       defaultKDFParams,
		Salt:      encodeURL(salt),
		Nonce:     encodeURL(nonce),
	}
	aad, err := envelopeAdditionalData(env)
	if err != nil {
		return "", err
	}
	env.Ciphertext = encodeURL(gcm.Seal(nil, nonce, []byte(plaintext), aad))
	return encodeEnvelope(env)
}

func (k *Keyring) DecryptString(value string) (string, error) {
	if !IsEnvelope(value) {
		return "", fmt.Errorf("ciphertext is not a %s envelope", EnvelopePrefix)
	}
	env, err := decodeEnvelope(value)
	if err != nil {
		return "", err
	}
	plaintext, err := k.decryptEnvelope(env)
	if err != nil {
		return "", err
	}
	return string(plaintext), nil
}

func (k *Keyring) DecryptLegacyString(value string) (plaintext string, keyID string, err error) {
	if k == nil {
		return "", "", fmt.Errorf("encryption keyring is nil")
	}
	raw, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		return "", "", fmt.Errorf("legacy ciphertext is not base64: %w", err)
	}
	for id, secret := range k.keys {
		plaintext, err := Decrypt(raw, DeriveKey(secret))
		if err == nil {
			return string(plaintext), id, nil
		}
	}
	return "", "", fmt.Errorf("legacy ciphertext cannot be decrypted with any configured key")
}

func (k *Keyring) RewrapString(value string) (string, error) {
	if IsEnvelope(value) {
		plaintext, err := k.DecryptString(value)
		if err != nil {
			return "", err
		}
		return k.EncryptString(plaintext)
	}
	plaintext, _, err := k.DecryptLegacyString(value)
	if err != nil {
		return "", err
	}
	return k.EncryptString(plaintext)
}

func IsEnvelope(value string) bool {
	return strings.HasPrefix(value, EnvelopePrefix)
}

func EnvelopeKeyID(value string) (string, error) {
	env, err := decodeEnvelope(value)
	if err != nil {
		return "", err
	}
	if env.KeyID == "" {
		return "", fmt.Errorf("encryption envelope key_id is required")
	}
	return env.KeyID, nil
}

func (k *Keyring) decryptEnvelope(env envelope) ([]byte, error) {
	if k == nil {
		return nil, fmt.Errorf("encryption keyring is nil")
	}
	if err := validateEnvelopeMetadata(env); err != nil {
		return nil, err
	}
	secret, ok := k.keys[env.KeyID]
	if !ok {
		return nil, fmt.Errorf("archive or ciphertext requires encryption key id %q, but it is not configured", env.KeyID)
	}
	salt, err := decodeURL(env.Salt, "salt")
	if err != nil {
		return nil, err
	}
	nonce, err := decodeURL(env.Nonce, "nonce")
	if err != nil {
		return nil, err
	}
	ciphertext, err := decodeURL(env.Ciphertext, "ciphertext")
	if err != nil {
		return nil, err
	}
	key := deriveEnvelopeKey(secret, salt, env.KDF)
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("creating cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("creating GCM: %w", err)
	}
	if len(nonce) != gcm.NonceSize() {
		return nil, fmt.Errorf("invalid nonce size %d", len(nonce))
	}
	aad, err := envelopeAdditionalData(env)
	if err != nil {
		return nil, err
	}
	plaintext, err := gcm.Open(nil, nonce, ciphertext, aad)
	if err != nil {
		return nil, fmt.Errorf("decrypting envelope with key id %q: %w", env.KeyID, err)
	}
	return plaintext, nil
}

func validateEnvelopeMetadata(env envelope) error {
	if env.Version != envelopeVersion {
		return fmt.Errorf("unsupported encryption envelope version %d", env.Version)
	}
	if env.KeyID == "" {
		return fmt.Errorf("encryption envelope key_id is required")
	}
	if env.Algorithm != envelopeAlg {
		return fmt.Errorf("unsupported encryption algorithm %q", env.Algorithm)
	}
	if env.KDF.Name != kdfArgon2id {
		return fmt.Errorf("unsupported encryption kdf %q", env.KDF.Name)
	}
	if env.KDF.Time == 0 || env.KDF.MemoryKiB == 0 || env.KDF.Parallelism == 0 || env.KDF.KeyLength != 32 {
		return fmt.Errorf("invalid encryption kdf parameters")
	}
	if env.Salt == "" || env.Nonce == "" || env.Ciphertext == "" {
		return fmt.Errorf("encryption envelope is missing salt, nonce, or ciphertext")
	}
	return nil
}

func deriveEnvelopeKey(secret string, salt []byte, params kdfParams) []byte {
	return argon2.IDKey([]byte(secret), salt, params.Time, params.MemoryKiB, params.Parallelism, params.KeyLength)
}

func envelopeAdditionalData(env envelope) ([]byte, error) {
	return json.Marshal(envelopeAAD{
		Version:   env.Version,
		KeyID:     env.KeyID,
		Algorithm: env.Algorithm,
		KDF:       env.KDF,
		Salt:      env.Salt,
		Nonce:     env.Nonce,
	})
}

func decodeEnvelope(value string) (envelope, error) {
	var env envelope
	if !IsEnvelope(value) {
		return env, fmt.Errorf("ciphertext is not a %s envelope", EnvelopePrefix)
	}
	raw, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(value, EnvelopePrefix))
	if err != nil {
		return env, fmt.Errorf("decoding encryption envelope: %w", err)
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		return env, fmt.Errorf("parsing encryption envelope: %w", err)
	}
	return env, nil
}

func encodeEnvelope(env envelope) (string, error) {
	raw, err := json.Marshal(env)
	if err != nil {
		return "", fmt.Errorf("encoding encryption envelope: %w", err)
	}
	return EnvelopePrefix + base64.RawURLEncoding.EncodeToString(raw), nil
}

func encodeURL(value []byte) string {
	return base64.RawURLEncoding.EncodeToString(value)
}

func decodeURL(value, field string) ([]byte, error) {
	out, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return nil, fmt.Errorf("decoding encryption envelope %s: %w", field, err)
	}
	return out, nil
}

func legacyKeyHash(key []byte) string {
	if len(key) < 8 {
		h := sha256.Sum256(key)
		return hex.EncodeToString(h[:])
	}
	h := sha256.Sum256(key[:8])
	return hex.EncodeToString(h[:])
}
