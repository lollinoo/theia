package security

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	"golang.org/x/crypto/argon2"
)

const (
	passwordMinLength = 12
	// MaxPasswordLength bounds password processing while still allowing long passphrases.
	MaxPasswordLength = 1024

	argon2idMemory      = 64 * 1024
	argon2idIterations  = 3
	argon2idParallelism = 4
	argon2idSaltLength  = 16
	argon2idKeyLength   = 32

	tokenBytes = 32
)

var (
	// ErrPasswordPolicyViolation indicates that a password does not meet local policy.
	ErrPasswordPolicyViolation = errors.New("password does not meet policy")
	errInvalidPasswordHash     = errors.New("invalid password hash")
)

var commonWeakPasswords = map[string]struct{}{
	"1234567890":    {},
	"123456789012":  {},
	"administrator": {},
	"admin":         {},
	"letmein":       {},
	"password":      {},
	"password123":   {},
	"qwerty123456":  {},
	"theia":         {},
}

// ValidatePasswordPolicy validates a user-managed password.
func ValidatePasswordPolicy(password string) error {
	if len(password) > MaxPasswordLength {
		return fmt.Errorf("%w: password is too long", ErrPasswordPolicyViolation)
	}
	trimmed := strings.TrimSpace(password)
	if utf8.RuneCountInString(trimmed) < passwordMinLength {
		return fmt.Errorf("%w: password must be at least %d characters", ErrPasswordPolicyViolation, passwordMinLength)
	}
	normalized := strings.ToLower(trimmed)
	if _, ok := commonWeakPasswords[normalized]; ok {
		return fmt.Errorf("%w: password is too common", ErrPasswordPolicyViolation)
	}
	if allRunesSame(normalized) {
		return fmt.Errorf("%w: password is too repetitive", ErrPasswordPolicyViolation)
	}
	return nil
}

// HashPassword hashes password using Argon2id with an encoded parameter string.
func HashPassword(password string) (string, error) {
	salt := make([]byte, argon2idSaltLength)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("generating password salt: %w", err)
	}

	hash := argon2.IDKey(
		[]byte(password),
		salt,
		argon2idIterations,
		argon2idMemory,
		argon2idParallelism,
		argon2idKeyLength,
	)
	return fmt.Sprintf(
		"$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version,
		argon2idMemory,
		argon2idIterations,
		argon2idParallelism,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(hash),
	), nil
}

// VerifyPassword verifies password against an encoded Argon2id hash.
func VerifyPassword(password, encodedHash string) (bool, error) {
	params, salt, expectedHash, err := parseArgon2idHash(encodedHash)
	if err != nil {
		return false, err
	}
	actualHash := argon2.IDKey(
		[]byte(password),
		salt,
		params.iterations,
		params.memory,
		params.parallelism,
		uint32(len(expectedHash)),
	)
	return subtle.ConstantTimeCompare(actualHash, expectedHash) == 1, nil
}

// GenerateToken returns a URL-safe high-entropy bearer token.
func GenerateToken() (string, error) {
	raw := make([]byte, tokenBytes)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("generating token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

// HashToken returns a keyed HMAC-SHA256 hash suitable for storing bearer tokens.
func HashToken(token string, secret []byte) string {
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write([]byte(token))
	return "hmac-sha256:" + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

type argon2idParams struct {
	memory      uint32
	iterations  uint32
	parallelism uint8
}

func parseArgon2idHash(encodedHash string) (argon2idParams, []byte, []byte, error) {
	parts := strings.Split(encodedHash, "$")
	if len(parts) != 6 || parts[0] != "" || parts[1] != "argon2id" {
		return argon2idParams{}, nil, nil, errInvalidPasswordHash
	}
	if parts[2] != "v="+strconv.Itoa(argon2.Version) {
		return argon2idParams{}, nil, nil, errInvalidPasswordHash
	}
	params, err := parseArgon2idParams(parts[3])
	if err != nil {
		return argon2idParams{}, nil, nil, err
	}
	if params.memory != argon2idMemory ||
		params.iterations != argon2idIterations ||
		params.parallelism != argon2idParallelism {
		return argon2idParams{}, nil, nil, errInvalidPasswordHash
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return argon2idParams{}, nil, nil, fmt.Errorf("%w: salt", errInvalidPasswordHash)
	}
	hash, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return argon2idParams{}, nil, nil, fmt.Errorf("%w: hash", errInvalidPasswordHash)
	}
	if len(salt) != argon2idSaltLength || len(hash) != argon2idKeyLength {
		return argon2idParams{}, nil, nil, errInvalidPasswordHash
	}
	return params, salt, hash, nil
}

func parseArgon2idParams(raw string) (argon2idParams, error) {
	values := make(map[string]uint64)
	for _, part := range strings.Split(raw, ",") {
		key, value, ok := strings.Cut(part, "=")
		if !ok {
			return argon2idParams{}, errInvalidPasswordHash
		}
		switch key {
		case "m", "t", "p":
		default:
			return argon2idParams{}, errInvalidPasswordHash
		}
		if _, exists := values[key]; exists {
			return argon2idParams{}, errInvalidPasswordHash
		}
		parsed, err := strconv.ParseUint(value, 10, 32)
		if err != nil {
			return argon2idParams{}, fmt.Errorf("%w: params", errInvalidPasswordHash)
		}
		if parsed == 0 {
			return argon2idParams{}, errInvalidPasswordHash
		}
		values[key] = parsed
	}
	if len(values) != 3 {
		return argon2idParams{}, errInvalidPasswordHash
	}
	memory, ok := values["m"]
	if !ok {
		return argon2idParams{}, errInvalidPasswordHash
	}
	iterations, ok := values["t"]
	if !ok {
		return argon2idParams{}, errInvalidPasswordHash
	}
	parallelism, ok := values["p"]
	if !ok || parallelism > 255 {
		return argon2idParams{}, errInvalidPasswordHash
	}
	return argon2idParams{
		memory:      uint32(memory),
		iterations:  uint32(iterations),
		parallelism: uint8(parallelism),
	}, nil
}

func allRunesSame(value string) bool {
	var first rune
	for i, r := range value {
		if i == 0 {
			first = r
			continue
		}
		if r != first {
			return false
		}
	}
	return true
}
