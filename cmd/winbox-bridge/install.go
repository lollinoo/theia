package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

type installStatus struct {
	InstalledPath            string `json:"installed_path"`
	InstalledConfigPath      string `json:"installed_config_path"`
	Installed                bool   `json:"installed"`
	InstalledExecutableValid bool   `json:"installed_executable_valid"`
	InstalledConfigExists    bool   `json:"installed_config_exists"`
	InstalledConfigValid     bool   `json:"installed_config_valid"`
	InstallHealthy           bool   `json:"install_healthy"`
	RunningFromInstalledPath bool   `json:"running_from_installed_path"`
}

type connectorInstaller interface {
	Status() (installStatus, error)
	EnsureInstalled() (installStatus, error)
}

type systemConnectorInstaller struct {
	currentExecutable func() (string, error)
	installedPath     func() (string, error)
	legacyConfigPath  func() (string, error)
	currentConfig     func() (Config, error)
}

func (i systemConnectorInstaller) Status() (installStatus, error) {
	current, err := i.executablePath()
	if err != nil {
		return installStatus{}, err
	}
	target, err := i.targetPath()
	if err != nil {
		return installStatus{}, err
	}
	configPath := installedConfigPathForExecutable(target)
	installed := fileExists(target)
	configExists := fileExists(configPath)
	executableValid := executableFileValid(target)
	configValid := configExists && configFileValid(configPath)
	return installStatus{
		InstalledPath:            target,
		InstalledConfigPath:      configPath,
		Installed:                installed,
		InstalledExecutableValid: executableValid,
		InstalledConfigExists:    configExists,
		InstalledConfigValid:     configValid,
		InstallHealthy:           installed && executableValid && configExists && configValid,
		RunningFromInstalledPath: sameFilePath(current, target),
	}, nil
}

func (i systemConnectorInstaller) EnsureInstalled() (installStatus, error) {
	status, err := i.Status()
	if err != nil {
		return installStatus{}, err
	}
	if status.RunningFromInstalledPath && status.Installed {
		if err := i.ensureInstalledConfig(status.InstalledPath); err != nil {
			return installStatus{}, err
		}
		return i.Status()
	}
	current, err := i.executablePath()
	if err != nil {
		return installStatus{}, err
	}
	if err := copyExecutableFile(current, status.InstalledPath); err != nil {
		return installStatus{}, fmt.Errorf("install connector: %w", err)
	}
	if err := i.ensureInstalledConfig(status.InstalledPath); err != nil {
		return installStatus{}, err
	}
	return i.Status()
}

func (i systemConnectorInstaller) executablePath() (string, error) {
	if i.currentExecutable != nil {
		return i.currentExecutable()
	}
	return os.Executable()
}

func (i systemConnectorInstaller) targetPath() (string, error) {
	if i.installedPath != nil {
		return i.installedPath()
	}
	return installedExecutablePath()
}

func (i systemConnectorInstaller) configPath() (string, error) {
	if i.legacyConfigPath != nil {
		return i.legacyConfigPath()
	}
	return legacyConfigFilePath()
}

func (i systemConnectorInstaller) loadCurrentConfig() (Config, error) {
	if i.currentConfig != nil {
		return i.currentConfig()
	}
	return loadConfig()
}

func (i systemConnectorInstaller) ensureInstalledConfig(installedExecutable string) error {
	legacyConfig, err := i.configPath()
	if err == nil {
		if err := moveConfigFileIfNeeded(legacyConfig, installedConfigPathForExecutable(installedExecutable)); err != nil {
			return fmt.Errorf("install config: %w", err)
		}
	}
	targetConfig := installedConfigPathForExecutable(installedExecutable)
	if configFileValid(targetConfig) {
		return nil
	}
	cfg, _ := i.loadCurrentConfig()
	if err := saveConfigTo(cfg, targetConfig); err != nil {
		return fmt.Errorf("install config: %w", err)
	}
	return nil
}

func migrateConfigToInstalledPath() error {
	installedExecutable, err := installedExecutablePath()
	if err != nil || !fileExists(installedExecutable) {
		return nil
	}
	currentExecutable, err := os.Executable()
	if err != nil || !sameFilePath(currentExecutable, installedExecutable) {
		return nil
	}
	legacyConfig, err := legacyConfigFilePath()
	if err != nil {
		return nil
	}
	return moveConfigFileIfNeeded(legacyConfig, installedConfigPathForExecutable(installedExecutable))
}

func installedExecutablePath() (string, error) {
	homeDir, _ := os.UserHomeDir()
	return installedExecutablePathFor(
		runtime.GOOS,
		homeDir,
		os.Getenv("LOCALAPPDATA"),
		os.Getenv("XDG_DATA_HOME"),
	)
}

func installedExecutablePathFor(goos, homeDir, localAppData, xdgDataHome string) (string, error) {
	switch goos {
	case "windows":
		base := strings.TrimSpace(localAppData)
		if base == "" && strings.TrimSpace(homeDir) != "" {
			base = filepath.Join(homeDir, "AppData", "Local")
		}
		if base == "" {
			return "", fmt.Errorf("LOCALAPPDATA is not available")
		}
		return filepath.Join(base, "Theia", "WinBoxBridge", "winbox-bridge.exe"), nil
	case "darwin":
		home := strings.TrimSpace(homeDir)
		if home == "" {
			return "", fmt.Errorf("home directory is not available")
		}
		return filepath.Join(
			home, "Library", "Application Support", "Theia", "WinBoxBridge", "winbox-bridge",
		), nil
	default:
		base := strings.TrimSpace(xdgDataHome)
		if base == "" {
			home := strings.TrimSpace(homeDir)
			if home == "" {
				return "", fmt.Errorf("home directory is not available")
			}
			base = filepath.Join(home, ".local", "share")
		}
		return filepath.Join(base, "theia", "winbox-bridge", "winbox-bridge"), nil
	}
}

func copyExecutableFile(source, target string) error {
	if sameFilePath(source, target) {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		return err
	}
	src, err := os.Open(source)
	if err != nil {
		return err
	}
	defer src.Close()

	tmp, err := os.CreateTemp(filepath.Dir(target), ".winbox-bridge-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer func() { _ = os.Remove(tmpPath) }()

	if _, err := io.Copy(tmp, src); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Chmod(0o700); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Remove(target); err != nil && !os.IsNotExist(err) {
		return err
	}
	return os.Rename(tmpPath, target)
}

func moveConfigFileIfNeeded(source, target string) error {
	if sameFilePath(source, target) || !fileExists(source) || fileExists(target) {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		return err
	}
	src, err := os.Open(source)
	if err != nil {
		return err
	}

	tmp, err := os.CreateTemp(filepath.Dir(target), ".config-*")
	if err != nil {
		_ = src.Close()
		return err
	}
	tmpPath := tmp.Name()
	defer func() { _ = os.Remove(tmpPath) }()

	if _, err := io.Copy(tmp, src); err != nil {
		_ = src.Close()
		_ = tmp.Close()
		return err
	}
	if err := src.Close(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, target); err != nil {
		return err
	}
	if err := os.Remove(source); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func executableFileValid(path string) bool {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() || !info.Mode().IsRegular() || info.Size() <= 0 {
		return false
	}
	if runtime.GOOS == "windows" {
		return true
	}
	return info.Mode().Perm()&0o111 != 0
}

func configFileValid(path string) bool {
	if !fileExists(path) {
		return false
	}
	_, err := loadConfigFrom(path)
	return err == nil
}

func fileExists(path string) bool {
	if strings.TrimSpace(path) == "" {
		return false
	}
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func sameFilePath(a, b string) bool {
	if strings.TrimSpace(a) == "" || strings.TrimSpace(b) == "" {
		return false
	}
	absA, errA := filepath.Abs(a)
	if errA != nil {
		absA = filepath.Clean(a)
	}
	absB, errB := filepath.Abs(b)
	if errB != nil {
		absB = filepath.Clean(b)
	}
	if runtime.GOOS == "windows" {
		return strings.EqualFold(absA, absB)
	}
	return absA == absB
}
