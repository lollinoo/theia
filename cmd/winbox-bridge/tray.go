package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"runtime"

	"fyne.io/systray"
)

// setupTray builds the system tray menu and wires click events to the ServerManager.
// Called from systray.Run(onReady, onExit) — runs inside the onReady callback.
// activeLogFile is the path to the active debug log file, or "" if not logging to file.
// When non-empty an "Open Log File" menu item is added so users can inspect logs
// without a visible console (particularly useful on Windows where the console is
// detached in tray mode).
func setupTray(mgr *ServerManager, initialCfg Config, activeLogFile string) {
	systray.SetIcon(iconBytes)
	systray.SetTooltip("WinBox Bridge")

	mStatus := systray.AddMenuItem("Status: Stopped", "Current server status")
	mStatus.Disable() // status is display-only, not clickable
	systray.AddSeparator()
	mStart := systray.AddMenuItem("Start Server", "Start the WinBox bridge HTTP server")
	mStop := systray.AddMenuItem("Stop Server", "Stop the WinBox bridge HTTP server")
	mStop.Disable() // initially stopped, so Stop is disabled
	systray.AddSeparator()
	mConfig := systray.AddMenuItem("Open Config File", "Open config.json in default editor")

	// "Open Log File" is only shown when --log-level debug is active and the log
	// file was opened successfully (activeLogFile != "").
	var mLog *systray.MenuItem
	if activeLogFile != "" {
		mLog = systray.AddMenuItem("Open Log File", fmt.Sprintf("Open debug log: %s", activeLogFile))
	}

	systray.AddSeparator()
	mQuit := systray.AddMenuItem("Quit", "Stop server and exit WinBox Bridge")

	// Track current config — reloaded on each Start
	cfg := initialCfg

	// Helper to update menu state based on ServerManager
	updateState := func() {
		if mgr.Running() {
			mStatus.SetTitle(fmt.Sprintf("Status: Running on :%d", mgr.Port()))
			mStart.Disable()
			mStop.Enable()
			systray.SetTooltip(fmt.Sprintf("WinBox Bridge — Running on :%d", mgr.Port()))
		} else {
			mStatus.SetTitle("Status: Stopped")
			mStart.Enable()
			mStop.Disable()
			systray.SetTooltip("WinBox Bridge — Stopped")
		}
	}

	// Set initial state (may have been auto-started before tray setup)
	updateState()

	// logClickedCh is the channel to receive "Open Log File" clicks.
	// When mLog is nil (no active log file) this is a nil channel — a nil channel
	// is never ready in a select, so the case is effectively disabled.
	var logClickedCh <-chan struct{}
	if mLog != nil {
		logClickedCh = mLog.ClickedCh
	}

	go func() {
		for {
			select {
			case <-mStart.ClickedCh:
				// Reload config on each start — user may have edited the file
				reloaded, err := loadConfig()
				if err != nil {
					log.Printf("winbox-bridge: config reload error: %v (using previous config)", err)
				} else {
					cfg = reloaded
				}
				if err := mgr.Start(cfg); err != nil {
					log.Printf("winbox-bridge: start error: %v", err)
				}
				updateState()
			case <-mStop.ClickedCh:
				if err := mgr.Stop(); err != nil {
					log.Printf("winbox-bridge: stop error: %v", err)
				}
				updateState()
			case <-mConfig.ClickedCh:
				path, err := configFilePath()
				if err != nil {
					log.Printf("winbox-bridge: config path error: %v", err)
					continue
				}
				// Ensure config file exists before opening
				if err := ensureConfigFileExists(cfg, path); err != nil {
					log.Printf("winbox-bridge: ensure config error: %v", err)
				}
				if err := openFileInEditor(path); err != nil {
					log.Printf("winbox-bridge: open config error: %v", err)
				}
			case <-logClickedCh:
				if err := openFileInEditor(activeLogFile); err != nil {
					log.Printf("winbox-bridge: open log file error: %v", err)
				}
			case <-mQuit.ClickedCh:
				mgr.Stop() //nolint:errcheck — best-effort shutdown on quit
				systray.Quit()
				return
			}
		}
	}()
}

// ensureConfigFileExists writes the current config to path if the file does not exist.
// This ensures the user has a file to edit even on first run.
func ensureConfigFileExists(cfg Config, path string) error {
	if _, err := os.Stat(path); err == nil {
		return nil // file exists
	}
	return saveConfig(cfg)
}

// openFileInEditor opens a file in the OS default application.
func openFileInEditor(path string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("notepad.exe", path) //nolint:gosec
	case "darwin":
		cmd = exec.Command("open", path) //nolint:gosec
	default:
		cmd = exec.Command("xdg-open", path) //nolint:gosec
	}
	return cmd.Start()
}
