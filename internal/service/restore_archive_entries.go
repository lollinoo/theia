package service

import (
	"fmt"
	"path"
	"strings"
)

// validateRestoreArchiveEntryForExtraction normalizes and allowlists one tar entry.
func validateRestoreArchiveEntryForExtraction(name string, directory bool) (string, error) {
	cleanName, err := cleanRestoreArchiveEntryName(name)
	if err != nil {
		return "", err
	}
	if directory {
		if !isAllowedRestoreArchiveDirectory(cleanName) {
			return "", fmt.Errorf("disallowed restore archive entry: %s", cleanName)
		}
		return cleanName, nil
	}
	if isLegacySQLiteRestoreArchiveFile(cleanName) {
		return "", legacySQLiteRestoreArchiveError()
	}
	if !isAllowedRestoreArchiveFile(cleanName) {
		return "", fmt.Errorf("disallowed restore archive entry: %s", cleanName)
	}
	return cleanName, nil
}

// cleanRestoreArchiveEntryName rejects absolute, empty, and traversal archive paths.
func cleanRestoreArchiveEntryName(name string) (string, error) {
	entryName := strings.ReplaceAll(name, "\\", "/")
	for strings.HasPrefix(entryName, "./") {
		entryName = strings.TrimPrefix(entryName, "./")
	}

	if archiveEntryHasTraversal(entryName) {
		return "", fmt.Errorf("archive contains path traversal: %s", name)
	}
	cleanName := path.Clean(entryName)
	if cleanName == "." {
		return "", fmt.Errorf("disallowed restore archive entry: %s", name)
	}
	if strings.HasPrefix(cleanName, "/") {
		return "", fmt.Errorf("archive contains absolute path: %s", name)
	}
	return cleanName, nil
}

// archiveEntryHasTraversal detects .. segments before path.Clean can hide them.
func archiveEntryHasTraversal(name string) bool {
	normalized := strings.ReplaceAll(name, "\\", "/")
	for _, part := range strings.Split(normalized, "/") {
		if part == ".." {
			return true
		}
	}
	return false
}

// legacySQLiteRestoreArchiveError explains why pre-PostgreSQL instance archives cannot be restored.
func legacySQLiteRestoreArchiveError() error {
	return fmt.Errorf("legacy SQLite instance backup archives containing %s cannot be restored by this PostgreSQL-only runtime; matching THEIA_ENCRYPTION_KEY is not sufficient. Restore a PostgreSQL instance backup containing %s, or restore/migrate the SQLite backup with a 1.7.x build before upgrading", legacySQLiteArchiveDBEntry, postgresArchiveDBEntry)
}

// isLegacySQLiteRestoreArchiveFile identifies the retired SQLite database archive entry.
func isLegacySQLiteRestoreArchiveFile(name string) bool {
	return strings.ReplaceAll(name, "\\", "/") == legacySQLiteArchiveDBEntry
}

// isAllowedRestoreArchiveFile checks if a regular file entry matches the restore archive contract.
func isAllowedRestoreArchiveFile(name string) bool {
	normalized := strings.ReplaceAll(name, "\\", "/")
	switch normalized {
	case "manifest.json", postgresArchiveDBEntry, "known_hosts":
		return true
	default:
		return strings.HasPrefix(normalized, "backups/")
	}
}

// isAllowedRestoreArchiveDirectory checks if a directory entry matches the restore archive contract.
func isAllowedRestoreArchiveDirectory(name string) bool {
	normalized := strings.ReplaceAll(name, "\\", "/")
	return normalized == "backups" || strings.HasPrefix(normalized, "backups/")
}
