//go:build !windows

package main

// This file defines launch other behavior for the Winbox bridge command.

import "os/exec"

// defaultStartProcess launches WinBox by starting a detached process.
// On Linux / macOS the window manager handles foreground placement; no special
// STARTUPINFO flags are needed.
func defaultStartProcess(name string, args []string) error {
	cmd := exec.Command(name, args...) //nolint:gosec — name comes from trusted discoverWinBox()
	cmd.Stdout = nil
	cmd.Stderr = nil
	cmd.Stdin = nil
	if err := cmd.Start(); err != nil {
		return err
	}
	// Release ownership so the child outlives the bridge (fire-and-forget)
	return cmd.Process.Release()
}
