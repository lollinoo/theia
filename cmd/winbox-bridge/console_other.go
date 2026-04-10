//go:build !windows

package main

// freeConsole is a no-op on non-Windows platforms.
// On Windows the real implementation calls kernel32.dll!FreeConsole to detach
// the console window when running in tray mode.
func freeConsole() {}
