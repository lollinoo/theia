//go:build windows

package main

// This file defines console windows behavior for the Winbox bridge command.

import "syscall"

// freeConsole detaches the process from its console window on Windows so that
// double-clicking winbox-bridge.exe does not flash a black terminal.
// This is called only in tray mode (not --no-tray) so terminal use still works:
// running `winbox-bridge.exe --no-tray` from cmd.exe / PowerShell keeps its console.
func freeConsole() {
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	kernel32.NewProc("FreeConsole").Call() //nolint:errcheck — best-effort; failure is silent
}
