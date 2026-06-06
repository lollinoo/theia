package api

// This file defines bridge downloads HTTP handler behavior and request/response boundaries.

import (
	"fmt"
	"os"
	"path/filepath"
)

type bridgeConnectorDownloadTarget struct {
	Label     string `json:"label"`
	OS        string `json:"os"`
	Arch      string `json:"arch"`
	URL       string `json:"url"`
	Available bool   `json:"available"`
}

var bridgeConnectorDownloadOptions = []struct {
	label string
	os    string
	arch  string
}{
	{label: "Linux x64", os: "linux", arch: "amd64"},
	{label: "Linux ARM64", os: "linux", arch: "arm64"},
	{label: "Windows x64", os: "windows", arch: "amd64"},
	{label: "Windows ARM64", os: "windows", arch: "arm64"},
	{label: "macOS Intel", os: "darwin", arch: "amd64"},
	{label: "macOS Apple Silicon", os: "darwin", arch: "arm64"},
}

func bridgeBinaryFilename(osName, arch string) string {
	filename := fmt.Sprintf("winbox-bridge-%s-%s", osName, arch)
	if osName == "windows" {
		filename += ".exe"
	}
	return filename
}

func bridgeBinaryFilePath(binariesDir, osName, arch string) string {
	if binariesDir == "" {
		return ""
	}
	return filepath.Join(binariesDir, bridgeBinaryFilename(osName, arch))
}

func bridgeBinaryAvailable(binariesDir, osName, arch string) bool {
	filePath := bridgeBinaryFilePath(binariesDir, osName, arch)
	if filePath == "" {
		return false
	}
	info, err := os.Stat(filePath)
	return err == nil && !info.IsDir()
}

func bridgeConnectorDownloadTargets(binariesDir, prefix string) []bridgeConnectorDownloadTarget {
	targets := make([]bridgeConnectorDownloadTarget, 0, len(bridgeConnectorDownloadOptions))
	for _, option := range bridgeConnectorDownloadOptions {
		targets = append(targets, bridgeConnectorDownloadTarget{
			Label:     option.label,
			OS:        option.os,
			Arch:      option.arch,
			URL:       prefix + option.os + "/" + option.arch,
			Available: bridgeBinaryAvailable(binariesDir, option.os, option.arch),
		})
	}
	return targets
}
