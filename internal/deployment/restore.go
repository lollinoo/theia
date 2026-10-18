package deployment

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/lollinoo/theia/internal/instance"
)

// Restore pre-verifies a replacement-host archive before stopping any writers.
// Recovery identities are mounted from private temporary storage and removed on
// return. Persistent state retains only the administrator's public recipient.
func (a *Admin) Restore(ctx context.Context, archive, recoveryFile string) error {
	unlock, err := a.lock()
	if err != nil {
		return err
	}
	defer unlock()
	c, err := LoadConfig(a.Dir)
	if err != nil {
		return err
	}
	archive, err = filepath.Abs(archive)
	if err != nil {
		return err
	}
	info, err := os.Lstat(archive)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || strings.Contains(archive, ":") || strings.Contains(archive, "$") {
		return fmt.Errorf("archive must be a regular file with a mountable path")
	}
	data, err := readInputFile(recoveryFile, 64<<10)
	if err != nil {
		return err
	}
	temporary, err := os.MkdirTemp("", "theia-recovery-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(temporary)
	private := filepath.Join(temporary, "recovery.txt")
	if err := instance.WritePrivateFile(private, data); err != nil {
		return err
	}
	mounts := []string{"run", "--rm", "--no-deps", "-T", "-v", archive + ":/run/theia/archive.age:ro", "-v", private + ":/run/theia/recovery.txt:ro", "backend"}
	jobArgs := []string{"-archive", "/run/theia/archive.age", "-recovery-file", "/run/theia/recovery.txt"}
	verify := append(append([]string{}, mounts...), "maintenance", "verify")
	if err := a.compose(ctx, append(verify, jobArgs...)...); err != nil {
		return fmt.Errorf("backup verification failed before deployment changes: %w", err)
	}
	if c.BundledPostgres {
		if err := a.compose(ctx, "up", "-d", "--wait", "--wait-timeout", "120", "postgres"); err != nil {
			return err
		}
	}
	if err := a.pause(ctx, true); err != nil {
		return err
	}
	restore := append(append([]string{}, mounts...), "maintenance", "restore")
	if err := a.compose(ctx, append(restore, jobArgs...)...); err != nil {
		return err
	}
	if err := a.saveMetadata(*c); err != nil {
		return err
	}
	return a.startHTTP(ctx, *c)
}
