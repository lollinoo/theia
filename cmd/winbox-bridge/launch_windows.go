//go:build windows

package main

// This file defines launch windows behavior for the Winbox bridge command.

import (
	"fmt"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// user32 procedures used for foreground-window management.
// We load them lazily via NewLazyDLL so a missing symbol (very unlikely on any
// supported Windows version) causes an error only if the code path is reached.
var (
	user32                       = syscall.NewLazyDLL("user32.dll")
	procEnumWindows              = user32.NewProc("EnumWindows")
	procGetWindowThreadProcessId = user32.NewProc("GetWindowThreadProcessId")
	procGetForegroundWindow      = user32.NewProc("GetForegroundWindow")
	procAttachThreadInput        = user32.NewProc("AttachThreadInput")
	procSetForegroundWindow      = user32.NewProc("SetForegroundWindow")
	procBringWindowToTop         = user32.NewProc("BringWindowToTop")
	procShowWindow               = user32.NewProc("ShowWindow")
	procIsWindowVisible          = user32.NewProc("IsWindowVisible")
	procGetWindow                = user32.NewProc("GetWindow")

	kernel32             = syscall.NewLazyDLL("kernel32.dll")
	procGetCurrentThread = kernel32.NewProc("GetCurrentThreadId")
)

// Windows constants used by the foreground-placement code.
const (
	swRestore = 9 // ShowWindow: restore and activate
	gwOwner   = 4 // GetWindow: return the owner window handle
)

// defaultStartProcess launches WinBox on Windows using CreateProcess with
// SW_SHOWNORMAL, then brings the new window to the foreground by temporarily
// attaching to the foreground thread's input queue.
//
// Background: Windows prevents background/tray processes from stealing focus
// (foreground lock introduced in Win98). STARTF_USESHOWWINDOW+SW_SHOWNORMAL
// alone is insufficient — WinBox still opens behind the active window.
// The fix is to call AttachThreadInput so our thread shares the foreground
// thread's input state, then call SetForegroundWindow.
func defaultStartProcess(name string, args []string) error {
	all := make([]string, 0, len(args)+1)
	all = append(all, winCmdQuote(name))
	for _, a := range args {
		all = append(all, winCmdQuote(a))
	}
	cmdLine, err := windows.UTF16PtrFromString(strings.Join(all, " "))
	if err != nil {
		return fmt.Errorf("encode command line: %w", err)
	}

	si := windows.StartupInfo{
		Cb:         uint32(unsafe.Sizeof(windows.StartupInfo{})),
		Flags:      windows.STARTF_USESHOWWINDOW,
		ShowWindow: windows.SW_SHOWNORMAL,
	}
	var pi windows.ProcessInformation

	if err := windows.CreateProcess(nil, cmdLine, nil, nil, false,
		windows.CREATE_UNICODE_ENVIRONMENT, nil, nil, &si, &pi,
	); err != nil {
		return fmt.Errorf("launch WinBox: %w", err)
	}

	pid := pi.ProcessId
	windows.CloseHandle(pi.Process) //nolint:errcheck
	windows.CloseHandle(pi.Thread)  //nolint:errcheck

	// Bring WinBox to the foreground once its window appears.
	// Run in a goroutine so the HTTP handler returns immediately.
	go bringToForeground(pid)
	return nil
}

// bringToForeground polls for WinBox's main window (up to 10 s) then forces it
// to the foreground using the AttachThreadInput technique.
func bringToForeground(pid uint32) {
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		time.Sleep(150 * time.Millisecond)
		if hwnd := findMainWindowByPID(pid); hwnd != 0 {
			forceForeground(hwnd)
			return
		}
	}
}

// findMainWindowByPID enumerates all top-level windows and returns the first
// visible, unowned window that belongs to the given process ID.
// Returns 0 if no suitable window is found yet.
func findMainWindowByPID(pid uint32) uintptr {
	var found uintptr
	// syscall.NewCallback wraps the Go closure as a native WNDENUMPROC.
	cb := syscall.NewCallback(func(hwnd, _ uintptr) uintptr {
		// Check process ownership
		var wPID uint32
		procGetWindowThreadProcessId.Call(hwnd, uintptr(unsafe.Pointer(&wPID)))
		if wPID != pid {
			return 1 // continue enumeration
		}
		// Skip invisible windows
		vis, _, _ := procIsWindowVisible.Call(hwnd)
		if vis == 0 {
			return 1
		}
		// Skip owned windows (dialogs, tooltips) — we want the main frame
		owner, _, _ := procGetWindow.Call(hwnd, gwOwner)
		if owner != 0 {
			return 1
		}
		found = hwnd
		return 0 // stop enumeration
	})
	procEnumWindows.Call(cb, 0)
	return found
}

// forceForeground brings hwnd to the foreground by temporarily attaching our
// goroutine's thread to the foreground thread's input queue.
//
// Windows blocks SetForegroundWindow calls from threads that do not own
// keyboard focus. AttachThreadInput makes the OS treat our thread as if it were
// the foreground thread, allowing SetForegroundWindow to succeed.
func forceForeground(hwnd uintptr) {
	// Identify the foreground window's thread
	fgHwnd, _, _ := procGetForegroundWindow.Call()
	var fgPID uint32
	fgTID, _, _ := procGetWindowThreadProcessId.Call(fgHwnd, uintptr(unsafe.Pointer(&fgPID)))

	myTID, _, _ := procGetCurrentThread.Call()

	if fgTID != 0 && fgTID != myTID {
		procAttachThreadInput.Call(myTID, fgTID, 1)       // attach
		defer procAttachThreadInput.Call(myTID, fgTID, 0) // detach on return
	}

	procSetForegroundWindow.Call(hwnd)
	procBringWindowToTop.Call(hwnd)
	procShowWindow.Call(hwnd, swRestore)
}

// winCmdQuote quotes a single argument for a Windows CreateProcess command line.
// Per CommandLineToArgvW rules: wrap in double quotes if the argument contains
// spaces, tabs, or double quotes; escape interior double quotes with backslash.
func winCmdQuote(s string) string {
	if !strings.ContainsAny(s, " \t\"") {
		return s
	}
	var b strings.Builder
	b.WriteByte('"')
	for i := 0; i < len(s); i++ {
		if s[i] == '"' {
			b.WriteString(`\"`)
		} else {
			b.WriteByte(s[i])
		}
	}
	b.WriteByte('"')
	return b.String()
}
