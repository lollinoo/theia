package service

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/lollinoo/theia/internal/crypto"
)

// VerifyStoredCredentials proves decryption succeeds for every sensitive value.
// requireActive additionally proves that credential rewrapping completed; skipped
// migration rows cannot silently turn into a successful maintenance operation.
func VerifyStoredCredentials(ctx context.Context, db *sql.DB, keys *crypto.Keyring, requireActive bool) error {
	if keys == nil {
		return fmt.Errorf("credential verification requires instance keys")
	}
	s := &InstanceBackupService{db: db}
	values, err := s.collectSensitiveCredentialValues(ctx)
	if err != nil {
		return err
	}
	for _, value := range values {
		if value == "" {
			continue
		}
		if !crypto.IsEnvelope(value) {
			return fmt.Errorf("sensitive credential remains outside the managed encryption envelope")
		}
		if _, err := keys.DecryptString(value); err != nil {
			return fmt.Errorf("verify stored credential: %w", err)
		}
		if requireActive {
			id, err := crypto.EnvelopeKeyID(value)
			if err != nil {
				return err
			}
			if id != keys.ActiveKeyID() {
				return fmt.Errorf("credential rewrapping is incomplete")
			}
		}
	}
	return nil
}
