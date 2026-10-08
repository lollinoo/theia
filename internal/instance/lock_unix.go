//go:build !windows

package instance

import (
	"golang.org/x/sys/unix"
	"os"
	"path/filepath"
	"syscall"
)

func lockFile(f *os.File) error { return unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB) }
func unlockFile(f *os.File)     { _ = unix.Flock(int(f.Fd()), unix.LOCK_UN) }
func syncDirectory(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return f.Sync()
}

func preservePrivateDirectoryOwner(path, parent string) error {
	if os.Geteuid() != 0 {
		return nil
	}
	info, err := os.Stat(parent)
	if err != nil {
		return err
	}
	if stat, ok := info.Sys().(*syscall.Stat_t); ok {
		return os.Chown(path, int(stat.Uid), int(stat.Gid))
	}
	return nil
}

// Root containers preserve bind-mounted state ownership for the host administrator.
func preservePrivateFileOwner(f *os.File, target string) error {
	if os.Geteuid() != 0 {
		return nil
	}
	info, err := os.Lstat(target)
	if os.IsNotExist(err) {
		info, err = os.Stat(filepath.Dir(target))
	}
	if err != nil {
		return err
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return nil
	}
	return f.Chown(int(stat.Uid), int(stat.Gid))
}
