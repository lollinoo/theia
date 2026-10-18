//go:build !windows

package service

import (
	"os"
	"os/exec"
	"syscall"
)

// PostgreSQL refuses root. A root application drops privileges only for the
// isolated server; non-root containers retain their configured UID/GID.
func isolatedPostgresUser(cmd *exec.Cmd, dir string) error {
	if os.Geteuid() != 0 {
		return nil
	}
	if err := os.Chown(dir, 65534, 65534); err != nil {
		return err
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{Credential: &syscall.Credential{Uid: 65534, Gid: 65534}}
	return nil
}
