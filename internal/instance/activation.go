package instance

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"time"
)

// Activation stores only the digest of a short-lived first-administrator token.
type Activation struct {
	TokenHash string    `json:"token_hash"`
	ExpiresAt time.Time `json:"expires_at"`
}

func (a *Activation) Validate() error {
	digest, err := hex.DecodeString(a.TokenHash)
	if err != nil || len(digest) != sha256.Size || a.ExpiresAt.IsZero() {
		return fmt.Errorf("invalid activation state")
	}
	return nil
}

// BeginActivation issues a one-time token. Only the digest is persisted by callers.
// A renewal must first establish that no users have been registered.
func (s *State) BeginActivation(now time.Time) (string, error) {
	token, err := RandomSecret()
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256([]byte(token))
	s.Activation = &Activation{TokenHash: hex.EncodeToString(digest[:]), ExpiresAt: now.UTC().Add(time.Hour)}
	return token, nil
}

// Accepts compares the token digest without exposing the persisted verifier.
func (a *Activation) Accepts(token string, now time.Time) bool {
	if a == nil || token == "" || !now.Before(a.ExpiresAt) {
		return false
	}
	digest := sha256.Sum256([]byte(token))
	return subtle.ConstantTimeCompare([]byte(hex.EncodeToString(digest[:])), []byte(a.TokenHash)) == 1
}
