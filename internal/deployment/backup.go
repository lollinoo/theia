package deployment

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/domain"
)

// Backup optionally exports the verified encrypted bytes to operator storage.
// Docker copies from the mounted volume even when host directory ownership differs.
func (a *Admin) Backup(ctx context.Context, output string) error {
	unlock, err := a.lock()
	if err != nil {
		return err
	}
	defer unlock()
	if output != "" {
		if _, err := os.Lstat(output); !os.IsNotExist(err) {
			return fmt.Errorf("backup output already exists or cannot be inspected")
		}
	}
	var log bytes.Buffer
	original := a.Output
	if original == nil {
		original = io.Discard
	}
	a.Output = io.MultiWriter(original, &log)
	err = a.offline(ctx, "backup")
	a.Output = original
	if err != nil || output == "" {
		return err
	}
	var backup domain.InstanceBackup
	for _, line := range strings.Split(log.String(), "\n") {
		var candidate domain.InstanceBackup
		if json.Unmarshal([]byte(line), &candidate) == nil && candidate.ID != uuid.Nil && candidate.Status == domain.InstanceBackupStatusSuccess {
			backup = candidate
		}
	}
	if backup.ID == uuid.Nil || filepath.Base(backup.FileName) != backup.FileName || !strings.HasSuffix(backup.FileName, ".tar.gz.age") {
		return fmt.Errorf("backup completed without valid export metadata")
	}
	temporary, err := os.CreateTemp(filepath.Dir(output), ".theia-export-*")
	if err != nil {
		return err
	}
	path := temporary.Name()
	temporary.Close()
	defer os.Remove(path)
	source := "backend:/data/instance-backups/" + backup.ID.String() + "/" + backup.FileName
	if err := a.compose(ctx, "cp", source, path); err != nil {
		return err
	}
	if err := os.Chmod(path, 0600); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	hash := sha256.New()
	size, readErr := io.Copy(hash, f)
	closeErr := errors.Join(f.Sync(), f.Close())
	if err := errors.Join(readErr, closeErr); err != nil {
		return err
	}
	if size != backup.SizeBytes || hex.EncodeToString(hash.Sum(nil)) != backup.SHA256 {
		return fmt.Errorf("exported backup does not match the verified archive")
	}
	// A hard link publishes verified bytes without overwriting an existing file.
	if err := os.Link(path, output); err != nil {
		return err
	}
	directory, err := os.Open(filepath.Dir(output))
	if err != nil {
		return err
	}
	defer directory.Close()
	if err := directory.Sync(); err != nil {
		return err
	}
	_, err = fmt.Fprintf(a.Output, "Verified encrypted backup exported to %s\n", output)
	return err
}
