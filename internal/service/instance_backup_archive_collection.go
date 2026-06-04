package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type instanceBackupArchiveSources struct {
	deviceBackupFiles []archiveSourceFile
	backupFileCount   int
	knownHostsFile    *archiveSourceFile
	totalBytes        int64
	fileEntries       int
}

// checkBackupArchiveEntryQuota enforces the per-entry backup archive size limit.
func checkBackupArchiveEntryQuota(name string, size int64, limits BackupArchiveLimits) error {
	if size > limits.MaxEntryBytes {
		return newRestoreLimitError("backup archive entry %s exceeds per-entry backup limit: %d bytes > %d bytes", name, size, limits.MaxEntryBytes)
	}
	return nil
}

// checkBackupArchiveTotals enforces total expanded bytes and file-count limits.
func checkBackupArchiveTotals(totalBytes int64, fileEntries int, limits BackupArchiveLimits) error {
	if totalBytes > limits.MaxTotalBytes {
		return newRestoreLimitError("backup archive exceeds expanded backup limit: %d bytes > %d bytes", totalBytes, limits.MaxTotalBytes)
	}
	if fileEntries > limits.MaxFileEntries {
		return newRestoreLimitError("backup archive file count exceeds backup limit: %d entries > %d entries", fileEntries, limits.MaxFileEntries)
	}
	return nil
}

// checkedArchiveByteTotal adds bytes while detecting negative inputs and overflow past limits.
func checkedArchiveByteTotal(current int64, increment int64, maxTotal int64) (int64, error) {
	if increment < 0 {
		return 0, fmt.Errorf("archive byte increment must be non-negative")
	}
	if current < 0 {
		return 0, fmt.Errorf("archive byte total must be non-negative")
	}
	if current > maxTotal || increment > maxTotal-current {
		return 0, newRestoreLimitError("backup archive exceeds expanded backup limit: current %d bytes plus %d bytes exceeds %d bytes", current, increment, maxTotal)
	}
	return current + increment, nil
}

// isArchiveQuotaError detects quota errors that should abort archive creation.
func isArchiveQuotaError(err error) bool {
	var limitErr *RestoreLimitError
	return errors.As(err, &limitErr)
}

// collectArchiveSourceFiles gathers source files using the service's configured directories.
func (s *InstanceBackupService) collectArchiveSourceFiles(ctx context.Context, limits BackupArchiveLimits, initialBytes int64) ([]archiveSourceFile, int, *archiveSourceFile, int64, int, error) {
	sources, err := collectInstanceBackupArchiveSourceFiles(ctx, s.backupDir, s.deviceBackupDir, s.knownHostsPath, limits, initialBytes)
	if err != nil {
		return nil, 0, nil, 0, 0, err
	}
	return sources.deviceBackupFiles, sources.backupFileCount, sources.knownHostsFile, sources.totalBytes, sources.fileEntries, nil
}

// collectInstanceBackupArchiveSourceFiles walks optional backup artifacts while enforcing quotas.
func collectInstanceBackupArchiveSourceFiles(
	ctx context.Context,
	backupDir string,
	deviceBackupDir string,
	knownHostsPath string,
	limits BackupArchiveLimits,
	initialBytes int64,
) (instanceBackupArchiveSources, error) {
	var sources instanceBackupArchiveSources
	if err := checkBackupArchiveTotals(initialBytes, 1, limits); err != nil {
		return sources, err
	}

	sources.deviceBackupFiles = make([]archiveSourceFile, 0)
	sources.totalBytes = initialBytes
	sources.fileEntries = 1 // database entry

	if info, err := os.Stat(deviceBackupDir); err == nil && info.IsDir() {
		err := filepath.Walk(deviceBackupDir, func(path string, info os.FileInfo, err error) error {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return ctxErr
			}
			if err != nil {
				return nil // skip files we can't read
			}
			if info.IsDir() {
				// Skip the instance backup directory to prevent circular inclusion (T-15-10)
				cleanInstanceBackupDir := filepath.Clean(backupDir)
				cleanPath := filepath.Clean(path)
				if cleanPath == cleanInstanceBackupDir || strings.HasPrefix(cleanPath, cleanInstanceBackupDir+string(filepath.Separator)) {
					return filepath.SkipDir
				}
				return nil
			}
			if !info.Mode().IsRegular() {
				return nil
			}

			rel, err := filepath.Rel(deviceBackupDir, path)
			if err != nil {
				return nil
			}

			// Validate archive entry name: no absolute paths, no traversal (T-15-05)
			archiveName := filepath.ToSlash(filepath.Join("backups", rel))
			if strings.HasPrefix(archiveName, "/") || archiveEntryHasTraversal(archiveName) {
				return nil
			}
			if err := checkBackupArchiveEntryQuota(archiveName, info.Size(), limits); err != nil {
				return err
			}
			sources.totalBytes, err = checkedArchiveByteTotal(sources.totalBytes, info.Size(), limits.MaxTotalBytes)
			if err != nil {
				return err
			}
			sources.fileEntries++
			if err := checkBackupArchiveTotals(sources.totalBytes, sources.fileEntries, limits); err != nil {
				return err
			}

			sources.deviceBackupFiles = append(sources.deviceBackupFiles, archiveSourceFile{
				archiveName: archiveName,
				diskPath:    path,
				sizeBytes:   info.Size(),
			})
			sources.backupFileCount++
			return nil
		})
		if err != nil {
			return sources, err
		}
	} else if err != nil && !os.IsNotExist(err) {
		return sources, fmt.Errorf("statting device backup dir: %w", err)
	}

	if info, err := os.Stat(knownHostsPath); err == nil && !info.IsDir() && info.Mode().IsRegular() {
		if err := checkBackupArchiveEntryQuota("known_hosts", info.Size(), limits); err != nil {
			return sources, err
		}
		totalBytes, err := checkedArchiveByteTotal(sources.totalBytes, info.Size(), limits.MaxTotalBytes)
		if err != nil {
			return sources, err
		}
		sources.totalBytes = totalBytes
		sources.fileEntries++
		if err := checkBackupArchiveTotals(sources.totalBytes, sources.fileEntries, limits); err != nil {
			return sources, err
		}
		sources.knownHostsFile = &archiveSourceFile{
			archiveName: "known_hosts",
			diskPath:    knownHostsPath,
			sizeBytes:   info.Size(),
		}
	} else if err != nil && !os.IsNotExist(err) {
		return sources, fmt.Errorf("statting known_hosts: %w", err)
	}

	return sources, nil
}
