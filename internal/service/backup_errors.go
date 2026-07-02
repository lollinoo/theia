package service

// This file defines stable backup error codes for API clients.

import "strings"

const (
	// BackupJobErrorCodeSSHHostKeyMismatch identifies a changed SSH host key.
	BackupJobErrorCodeSSHHostKeyMismatch = "ssh_host_key_mismatch"
)

// BackupJobErrorCode returns a stable frontend-facing code for recognized backup errors.
func BackupJobErrorCode(errorMessage string) string {
	if strings.Contains(strings.ToLower(errorMessage), "ssh host key mismatch") {
		return BackupJobErrorCodeSSHHostKeyMismatch
	}
	return ""
}
