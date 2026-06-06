//go:build !windows

package main

// This file defines console other behavior for the Winbox bridge command.

// freeConsole is a no-op on non-Windows platforms.
// On Windows the real implementation calls kernel32.dll!FreeConsole to detach
// the console window when running in tray mode.
func freeConsole() {}
