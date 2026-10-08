package instance

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"filippo.io/age"
)

// ExportRecoveryFile writes a new operator-held identity and rereads it to prove
// that the exported file corresponds to the public recipient. It never overwrites
// an existing recovery file and never stores the private identity in instance state.
func ExportRecoveryFile(path string) (string, error) {
	identity, err := age.GenerateX25519Identity()
	if err != nil {
		return "", err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return "", err
	}
	_, writeErr := fmt.Fprintf(f, "# Theia recovery identity. Keep outside the instance host.\n# recipient: %s\n%s\n", identity.Recipient(), identity)
	syncErr := f.Sync()
	closeErr := f.Close()
	if err := errors.Join(writeErr, syncErr, closeErr); err != nil {
		return "", err
	}
	if err := VerifyRecoveryFile(path, identity.Recipient().String()); err != nil {
		return "", err
	}
	return identity.Recipient().String(), nil
}

// ReadRecoveryFile reads operator identities transiently. Callers must not persist
// them on the instance, in a journal, or inside a backup archive.
func ReadRecoveryFile(path string) ([]age.Identity, error) {
	if err := privateRegularFile(path); err != nil {
		return nil, err
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	identities, err := age.ParseIdentities(io.LimitReader(f, 64<<10))
	if err != nil || len(identities) == 0 {
		return nil, fmt.Errorf("invalid or empty recovery file")
	}
	return identities, nil
}

// VerifyRecoveryFile checks the actual exported identity against a public recipient.
func VerifyRecoveryFile(path, recipient string) error {
	identities, err := ReadRecoveryFile(path)
	if err != nil {
		return err
	}
	for _, identity := range identities {
		if identity, ok := identity.(*age.X25519Identity); ok && identity.Recipient().String() == recipient {
			return nil
		}
	}
	return fmt.Errorf("recovery file does not contain the identity for this instance recipient")
}

type protectedWriter struct {
	io.WriteCloser
	destination io.WriteCloser
}

// ExportRecoveryHistory writes a new active identity together with every identity
// supplied by the operator. Old archives remain recoverable after replacement.
func ExportRecoveryHistory(original, output, previousRecipient string) (string, error) {
	identities, err := ReadRecoveryFile(original)
	if err != nil {
		return "", err
	}
	if err := VerifyRecoveryFile(original, previousRecipient); err != nil {
		return "", err
	}
	identity, err := age.GenerateX25519Identity()
	if err != nil {
		return "", err
	}
	f, err := os.OpenFile(output, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return "", err
	}
	_, writeErr := fmt.Fprintf(f, "# Theia recovery history. Keep outside the instance host.\n# active recipient: %s\n%s\n", identity.Recipient(), identity)
	for _, old := range identities {
		previous, ok := old.(*age.X25519Identity)
		if !ok {
			writeErr = errors.Join(writeErr, fmt.Errorf("unsupported recovery identity"))
			break
		}
		_, err := fmt.Fprintln(f, previous.String())
		writeErr = errors.Join(writeErr, err)
	}
	if err := errors.Join(writeErr, f.Sync(), f.Close()); err != nil {
		return "", err
	}
	if err := VerifyRecoveryFile(output, identity.Recipient().String()); err != nil {
		return "", err
	}
	if err := VerifyRecoveryFile(output, previousRecipient); err != nil {
		return "", err
	}
	return identity.Recipient().String(), nil
}

func (w *protectedWriter) Close() error {
	closeEncryptionErr := w.WriteCloser.Close()
	var syncErr error
	if f, ok := w.destination.(interface{ Sync() error }); ok {
		syncErr = f.Sync()
	}
	return errors.Join(closeEncryptionErr, syncErr, w.destination.Close())
}

// ProtectWriter encrypts directly to storage, with an offline recovery recipient
// and a fresh in-memory verification identity. Both can open the entire archive.
// The returned identity must be discarded only after full archive verification.
func ProtectWriter(destination io.WriteCloser, recipient string) (io.WriteCloser, *age.X25519Identity, error) {
	public, err := age.ParseX25519Recipient(strings.TrimSpace(recipient))
	if err != nil {
		return nil, nil, fmt.Errorf("invalid recovery recipient")
	}
	verification, err := age.GenerateX25519Identity()
	if err != nil {
		return nil, nil, err
	}
	encrypted, err := age.Encrypt(destination, public, verification.Recipient())
	if err != nil {
		return nil, nil, err
	}
	return &protectedWriter{encrypted, destination}, verification, nil
}
