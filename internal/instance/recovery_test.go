package instance

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"

	"filippo.io/age"
)

func TestExportedRecoveryFileOpensArchiveAndEphemeralIdentityIsIndependent(t *testing.T) {
	file := filepath.Join(t.TempDir(), "recovery.txt")
	recipient, err := ExportRecoveryFile(file)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ExportRecoveryFile(file); err == nil {
		t.Fatal("recovery file was overwritten")
	}
	path := filepath.Join(t.TempDir(), "backup.age")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	writer, temporary, err := ProtectWriter(f, recipient)
	if err != nil {
		t.Fatal(err)
	}
	plaintext := bytes.Repeat([]byte("protected-instance-data"), 10000)
	if _, err := writer.Write(plaintext); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	ciphertext, _ := os.ReadFile(path)
	identities, err := ReadRecoveryFile(file)
	if err != nil {
		t.Fatal(err)
	}
	for _, identity := range []age.Identity{identities[0], temporary} {
		reader, err := age.Decrypt(bytes.NewReader(ciphertext), identity)
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(reader)
		if err != nil || !bytes.Equal(data, plaintext) {
			t.Fatalf("archive recovery failed: %v", err)
		}
	}
	other, _ := age.GenerateX25519Identity()
	if err := VerifyRecoveryFile(file, other.Recipient().String()); err == nil {
		t.Fatal("unrelated exported identity accepted")
	}
	for _, damaged := range [][]byte{ciphertext[:len(ciphertext)-1], append(append([]byte(nil), ciphertext...), 1)} {
		reader, err := age.Decrypt(bytes.NewReader(damaged), temporary)
		if err == nil {
			_, err = io.ReadAll(reader)
		}
		if err == nil {
			t.Fatal("damaged archive passed authentication")
		}
	}
}
