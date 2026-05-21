package main

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestEnsurePrivateDirCreatesMissingDirWith0700(t *testing.T) {
	path := filepath.Join(t.TempDir(), "private")

	if err := ensurePrivateDir(path); err != nil {
		t.Fatalf("ensurePrivateDir() error = %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat() error = %v", err)
	}
	if !info.IsDir() {
		t.Fatal("path is not a directory")
	}
	assertRuntimePathMode(t, "dir", info.Mode().Perm(), privateDirMode)
}

func TestEnsurePrivateDirTightensExisting0755DirTo0700(t *testing.T) {
	path := filepath.Join(t.TempDir(), "private")
	if err := os.Mkdir(path, 0o755); err != nil {
		t.Fatalf("Mkdir() error = %v", err)
	}

	if err := ensurePrivateDir(path); err != nil {
		t.Fatalf("ensurePrivateDir() error = %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat() error = %v", err)
	}
	assertRuntimePathMode(t, "dir", info.Mode().Perm(), privateDirMode)
}

func TestEnsurePrivateDirLeavesExisting0700DirUnchanged(t *testing.T) {
	path := filepath.Join(t.TempDir(), "private")
	if err := os.Mkdir(path, privateDirMode); err != nil {
		t.Fatalf("Mkdir() error = %v", err)
	}

	before, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat() before ensurePrivateDir = %v", err)
	}

	if err := ensurePrivateDir(path); err != nil {
		t.Fatalf("ensurePrivateDir() error = %v", err)
	}

	after, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat() after ensurePrivateDir = %v", err)
	}
	if runtime.GOOS != "windows" {
		if got := after.Mode().Perm(); got != privateDirMode {
			t.Fatalf("dir mode = %o, want %o", got, privateDirMode)
		}
		if got := after.Mode().Perm(); got != before.Mode().Perm() {
			t.Fatalf("dir mode changed from %o to %o", before.Mode().Perm(), got)
		}
	}
}

func TestEnsurePrivateDirRejectsExistingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "private")
	if err := os.WriteFile(path, []byte("x"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	err := ensurePrivateDir(path)
	if err == nil {
		t.Fatal("ensurePrivateDir() error = nil, want error")
	}
	if got, want := err.Error(), "ensure private dir: path is not a directory"; got != want {
		t.Fatalf("ensurePrivateDir() error = %q, want %q", got, want)
	}
}

func TestEnsurePrivateDirRejectsSymlink(t *testing.T) {
	target := filepath.Join(t.TempDir(), "target")
	if err := os.Mkdir(target, 0o700); err != nil {
		t.Fatalf("Mkdir(target) error = %v", err)
	}

	path := filepath.Join(t.TempDir(), "private")
	createRuntimeTestSymlink(t, target, path)

	err := ensurePrivateDir(path)
	if err == nil {
		t.Fatal("ensurePrivateDir() error = nil, want error")
	}
	if got, want := err.Error(), "ensure private dir: path is a symlink"; got != want {
		t.Fatalf("ensurePrivateDir() error = %q, want %q", got, want)
	}
}

func TestEnsureFileModeTightensExisting0644FileTo0600(t *testing.T) {
	path := filepath.Join(t.TempDir(), "private")
	if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	if err := ensureFileMode(path, privateFileMode); err != nil {
		t.Fatalf("ensureFileMode() error = %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat() error = %v", err)
	}
	assertRuntimePathMode(t, "file", info.Mode().Perm(), privateFileMode)
}

func TestEnsureFileModeRejectsDirectory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "private")
	if err := os.Mkdir(path, 0o700); err != nil {
		t.Fatalf("Mkdir() error = %v", err)
	}

	err := ensureFileMode(path, privateFileMode)
	if err == nil {
		t.Fatal("ensureFileMode() error = nil, want error")
	}
	if got, want := err.Error(), "ensure file mode: path is a directory"; got != want {
		t.Fatalf("ensureFileMode() error = %q, want %q", got, want)
	}
}

func createRuntimeTestSymlink(t *testing.T, target, path string) {
	t.Helper()
	if err := os.Symlink(target, path); err != nil {
		if runtime.GOOS == "windows" {
			t.Skipf("Windows symlink privilege unavailable: %v", err)
		}
		t.Fatalf("Symlink() error = %v", err)
	}
}

func assertRuntimePathMode(t *testing.T, name string, got, want os.FileMode) {
	t.Helper()
	if runtime.GOOS == "windows" {
		return
	}
	if got != want {
		t.Fatalf("%s mode = %o, want %o", name, got, want)
	}
}
