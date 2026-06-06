package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/domain"
	"golang.org/x/sys/unix"
)

// BulkDownloadEntry pairs a backup file with a device-derived folder name.
type BulkDownloadEntry struct {
	File      domain.BackupFile
	DeviceID  uuid.UUID
	DeviceDir string // sanitized device name for zip folder
	ZipPath   string // slash-separated, prevalidated zip entry path
	SizeBytes int64
}

// GetBulkDownloadFiles returns file entries from the latest successful backup of each given device.
// It enforces device/file/byte quotas before any streaming and precomputes safe zip paths.
func (s *BackupService) GetBulkDownloadFiles(ctx context.Context, deviceIDs []uuid.UUID) ([]BulkDownloadEntry, error) {
	limits := s.BulkOperationLimits()
	deviceIDs = dedupeUUIDs(deviceIDs)
	if len(deviceIDs) > limits.BulkDownloadMaxDevices {
		return nil, &BulkLimitError{
			Operation: "bulk download",
			Limit:     "devices",
			Max:       int64(limits.BulkDownloadMaxDevices),
			Actual:    int64(len(deviceIDs)),
		}
	}

	var entries []BulkDownloadEntry
	var totalBytes int64
	var backupRoot string
	for _, did := range deviceIDs {
		if err := contextError(ctx); err != nil {
			return nil, err
		}
		device, err := s.deviceRepo.GetByID(did)
		if err != nil || device == nil {
			continue
		}
		job, err := s.jobRepo.GetLatestByDeviceID(did)
		if err != nil || job == nil {
			continue
		}
		files, err := s.fileRepo.GetByJobID(job.ID)
		if err != nil {
			continue
		}
		dirName := device.Tags["display_name"]
		if dirName == "" {
			dirName = device.SysName
		}
		if dirName == "" {
			dirName = device.Hostname
		}
		if dirName == "" {
			dirName = device.IP
		}
		dirName = sanitizeHostname(dirName)

		for _, f := range files {
			if len(entries)+1 > limits.BulkDownloadMaxFiles {
				return nil, &BulkLimitError{
					Operation: "bulk download",
					Limit:     "files",
					Max:       int64(limits.BulkDownloadMaxFiles),
					Actual:    int64(len(entries) + 1),
				}
			}
			if backupRoot == "" {
				backupRoot, err = validatedBackupRoot(s.backupDir)
				if err != nil {
					return nil, err
				}
			}
			filePath, sizeBytes, err := validateBulkDownloadFile(backupRoot, f.FilePath)
			if err != nil {
				return nil, err
			}
			if totalBytes > limits.BulkDownloadMaxBytes-sizeBytes {
				return nil, &BulkLimitError{
					Operation: "bulk download",
					Limit:     "bytes",
					Max:       limits.BulkDownloadMaxBytes,
					Actual:    saturatedInt64Sum(totalBytes, sizeBytes),
				}
			}
			zipPath, err := safeBulkDownloadZipPath(dirName, f.FileName)
			if err != nil {
				return nil, err
			}
			totalBytes += sizeBytes
			f.FilePath = filePath
			entries = append(entries, BulkDownloadEntry{
				File:      f,
				DeviceID:  did,
				DeviceDir: dirName,
				ZipPath:   zipPath,
				SizeBytes: sizeBytes,
			})
		}
	}
	return entries, nil
}

// validatedBackupRoot resolves the configured backup directory once for path-containment checks.
// Symlink resolution is intentional: stored file paths must live under the real backup root.
func validatedBackupRoot(backupDir string) (string, error) {
	backupDir = strings.TrimSpace(backupDir)
	if backupDir == "" {
		return "", &BulkPathError{Reason: "backup directory is not configured"}
	}
	absRoot, err := filepath.Abs(backupDir)
	if err != nil {
		return "", fmt.Errorf("resolving backup directory: %w", err)
	}
	root, err := filepath.EvalSymlinks(absRoot)
	if err != nil {
		return "", fmt.Errorf("resolving backup directory symlinks: %w", err)
	}
	return root, nil
}

// validateBulkDownloadFile resolves symlinks and rejects entries outside the configured backup root.
// It returns the canonical disk path and size used for quota accounting before streaming begins.
func validateBulkDownloadFile(backupRoot, filePath string) (string, int64, error) {
	if strings.TrimSpace(filePath) == "" {
		return "", 0, &BulkPathError{Reason: "backup file path is empty"}
	}
	absPath, err := filepath.Abs(filePath)
	if err != nil {
		return "", 0, fmt.Errorf("resolving backup file path: %w", err)
	}
	resolvedPath, err := filepath.EvalSymlinks(absPath)
	if err != nil {
		return "", 0, fmt.Errorf("resolving backup file symlinks: %w", err)
	}
	if !pathIsUnderDir(backupRoot, resolvedPath) {
		return "", 0, &BulkPathError{Path: filePath, Reason: "backup file path is outside backup directory"}
	}
	info, err := os.Stat(resolvedPath)
	if err != nil {
		return "", 0, fmt.Errorf("stat backup file: %w", err)
	}
	if !info.Mode().IsRegular() {
		return "", 0, &BulkPathError{Path: filePath, Reason: "backup file path is not a regular file"}
	}
	return resolvedPath, info.Size(), nil
}

// OpenBulkDownloadEntry opens a previously selected bulk download entry and
// revalidates the opened file descriptor before any bytes are streamed.
func (s *BackupService) OpenBulkDownloadEntry(entry BulkDownloadEntry) (*os.File, error) {
	if entry.SizeBytes < 0 {
		return nil, &BulkPathError{Path: entry.File.FilePath, Reason: "backup file size is invalid"}
	}
	backupRoot, err := validatedBackupRoot(s.backupDir)
	if err != nil {
		return nil, err
	}
	f, sizeBytes, err := openValidatedBulkDownloadFile(backupRoot, entry.File.FilePath)
	if err != nil {
		return nil, err
	}
	if sizeBytes != entry.SizeBytes {
		f.Close()
		return nil, &BulkPathError{Path: entry.File.FilePath, Reason: "backup file changed after validation"}
	}
	return f, nil
}

// openValidatedBulkDownloadFile opens the selected file with O_NOFOLLOW and rechecks containment.
// This closes the time-of-check/time-of-use gap between selection and response streaming.
func openValidatedBulkDownloadFile(backupRoot, filePath string) (*os.File, int64, error) {
	if strings.TrimSpace(filePath) == "" {
		return nil, 0, &BulkPathError{Reason: "backup file path is empty"}
	}
	absPath, err := filepath.Abs(filePath)
	if err != nil {
		return nil, 0, fmt.Errorf("resolving backup file path: %w", err)
	}

	fd, err := unix.Open(absPath, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0)
	if err != nil {
		if errors.Is(err, unix.ELOOP) {
			return nil, 0, &BulkPathError{Path: filePath, Reason: "backup file path is a symlink"}
		}
		return nil, 0, fmt.Errorf("opening backup file: %w", err)
	}
	f := os.NewFile(uintptr(fd), absPath)
	if f == nil {
		unix.Close(fd)
		return nil, 0, fmt.Errorf("opening backup file: invalid file descriptor")
	}

	info, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, 0, fmt.Errorf("stat backup file: %w", err)
	}
	if !info.Mode().IsRegular() {
		f.Close()
		return nil, 0, &BulkPathError{Path: filePath, Reason: "backup file path is not a regular file"}
	}

	resolvedPath, err := resolveOpenedBackupPath(absPath, info)
	if err != nil {
		f.Close()
		return nil, 0, err
	}
	if !pathIsUnderDir(backupRoot, resolvedPath) {
		f.Close()
		return nil, 0, &BulkPathError{Path: filePath, Reason: "backup file path is outside backup directory"}
	}
	return f, info.Size(), nil
}

// resolveOpenedBackupPath verifies that the resolved path still names the file descriptor just opened.
func resolveOpenedBackupPath(absPath string, openedInfo os.FileInfo) (string, error) {
	resolved, err := filepath.EvalSymlinks(absPath)
	if err != nil {
		return "", fmt.Errorf("resolving opened backup file path: %w", err)
	}
	currentInfo, err := os.Stat(resolved)
	if err != nil {
		return "", fmt.Errorf("stat opened backup file path: %w", err)
	}
	if !os.SameFile(openedInfo, currentInfo) {
		return "", &BulkPathError{Path: absPath, Reason: "backup file changed after open"}
	}
	return filepath.Clean(resolved), nil
}

// pathIsUnderDir accepts only candidates that remain within root after filepath.Rel normalization.
func pathIsUnderDir(root, candidate string) bool {
	rel, err := filepath.Rel(root, candidate)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}

// safeBulkDownloadZipPath builds a slash-separated archive member name without absolute paths or traversal.
func safeBulkDownloadZipPath(deviceDir, fileName string) (string, error) {
	deviceDir = sanitizeHostname(deviceDir)
	if deviceDir == "" {
		deviceDir = "device"
	}
	name := strings.ReplaceAll(fileName, "\\", "/")
	if strings.TrimSpace(name) == "" {
		return "", &BulkPathError{Path: fileName, Reason: "backup file name is empty"}
	}
	if path.IsAbs(name) || hasWindowsDrivePrefix(name) {
		return "", &BulkPathError{Path: fileName, Reason: "backup file name is absolute"}
	}
	for _, segment := range strings.Split(name, "/") {
		if segment == ".." {
			return "", &BulkPathError{Path: fileName, Reason: "backup file name contains traversal"}
		}
	}
	cleanName := path.Clean(name)
	if cleanName == "." || cleanName == ".." || strings.HasPrefix(cleanName, "../") || path.IsAbs(cleanName) || hasWindowsDrivePrefix(cleanName) {
		return "", &BulkPathError{Path: fileName, Reason: "backup file name is unsafe"}
	}
	zipPath := path.Join(deviceDir, cleanName)
	if zipPath == "." || zipPath == ".." || strings.HasPrefix(zipPath, "../") || path.IsAbs(zipPath) {
		return "", &BulkPathError{Path: fileName, Reason: "zip entry path is unsafe"}
	}
	return zipPath, nil
}

func hasWindowsDrivePrefix(name string) bool {
	if len(name) < 2 || name[1] != ':' {
		return false
	}
	first := name[0]
	return (first >= 'a' && first <= 'z') || (first >= 'A' && first <= 'Z')
}

func saturatedInt64Sum(a, b int64) int64 {
	if b > 0 && a > (1<<63-1)-b {
		return 1<<63 - 1
	}
	return a + b
}
