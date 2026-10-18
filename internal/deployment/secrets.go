package deployment

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/lollinoo/theia/internal/instance"
)

func outsideStorage(file string, roots ...string) error {
	absolute, err := filepath.Abs(file)
	if err != nil {
		return err
	}
	for _, root := range roots {
		if root == "" {
			continue
		}
		base, err := filepath.Abs(root)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(base, absolute)
		if err != nil || rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return fmt.Errorf("recovery output must be outside all instance storage")
		}
	}
	return nil
}

// RotateRecovery retains archive recovery history only in the operator's new
// file. Private identities never enter persistent instance state or its journal.
func (a *Admin) RotateRecovery(ctx context.Context, original, output string) error {
	unlock, err := a.lock()
	if err != nil {
		return err
	}
	defer unlock()
	c, err := LoadConfig(a.Dir)
	if err != nil {
		return err
	}
	if err := outsideStorage(output, a.Dir, c.DataBind, c.PostgresBind); err != nil {
		return err
	}
	data, err := readInputFile(original, 64<<10)
	if err != nil {
		return err
	}
	temporary, err := os.MkdirTemp("", "theia-recovery-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(temporary)
	private := filepath.Join(temporary, "original.txt")
	if err := instance.WritePrivateFile(private, data); err != nil {
		return err
	}
	state, err := (instance.Store{Path: StatePath(a.Dir)}).Load()
	if err != nil {
		return err
	}
	if state.Activation != nil {
		return fmt.Errorf("finish first-administrator activation before replacing its recovery recipient")
	}
	recipient, err := instance.ExportRecoveryHistory(private, output, state.RecoveryRecipient)
	if err != nil {
		return err
	}
	if err := a.pause(ctx, false); err != nil {
		return err
	}
	if err := (instance.Store{Path: StatePath(a.Dir)}).Update(func(s *instance.State) error {
		if s.RecoveryRecipient != state.RecoveryRecipient {
			return fmt.Errorf("recovery recipient changed concurrently")
		}
		s.RecoveryRecipient = recipient
		return nil
	}); err != nil {
		return err
	}
	fmt.Fprintln(a.Output, "New recovery file verified, including historical identities. Move it to independent storage.")
	return a.startHTTP(ctx, *c)
}

// ConfigureS3 persists a one-time destination configuration. Routine backups
// reuse it and verify the downloaded object before reporting success.
func (a *Admin) ConfigureS3(ctx context.Context, destination instance.S3Config, accessFile, secretFile string) error {
	unlock, err := a.lock()
	if err != nil {
		return err
	}
	defer unlock()
	c, err := LoadConfig(a.Dir)
	if err != nil {
		return err
	}
	access, err := readInputFile(accessFile, 64<<10)
	if err != nil {
		return err
	}
	secret, err := readInputFile(secretFile, 64<<10)
	if err != nil {
		return err
	}
	destination.AccessKey = strings.TrimRight(string(access), "\r\n")
	destination.SecretKey = strings.TrimRight(string(secret), "\r\n")
	if _, err := instance.NewS3Destination(destination); err != nil {
		return err
	}
	if err := a.pause(ctx, false); err != nil {
		return err
	}
	if err := (instance.Store{Path: StatePath(a.Dir)}).Update(func(s *instance.State) error { s.BackupDestination = &destination; return nil }); err != nil {
		return err
	}
	return a.startHTTP(ctx, *c)
}
