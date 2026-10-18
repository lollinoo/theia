package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/lollinoo/theia/internal/instance"
	"golang.org/x/sys/unix"
)

func TestRecoveryCommandCleansUpFileOnUpdateError(t *testing.T) {
	tempDir := t.TempDir()
	stateDir := filepath.Join(tempDir, "state")
	if err := os.MkdirAll(stateDir, 0700); err != nil {
		t.Fatal(err)
	}
	statePath := filepath.Join(stateDir, "secrets.json")
	store := instance.Store{Path: statePath}
	state, err := instance.Generate(time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Create(state); err != nil {
		t.Fatal(err)
	}

	recoveryFile := filepath.Join(tempDir, "recovery.txt")

	// Lock the state lockfile so store.Update fails when acquiring lock
	lockPath := statePath + ".lock"
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := unix.Flock(int(f.Fd()), unix.LOCK_EX); err != nil {
		t.Fatal(err)
	}
	defer unix.Flock(int(f.Fd()), unix.LOCK_UN)

	var buf bytes.Buffer
	err = runInstanceStateCommand([]string{"recovery", "-state", statePath, "-recovery-file", recoveryFile}, &buf)
	if err == nil {
		t.Fatal("expected error from runInstanceStateCommand")
	}

	if _, err := os.Stat(recoveryFile); !os.IsNotExist(err) {
		t.Fatalf("expected recovery file %s to be removed, stat err: %v", recoveryFile, err)
	}
}
