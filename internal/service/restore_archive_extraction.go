package service

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// restoreArchiveHeaderValidation carries the sanitized tar entry metadata used by extraction.
type restoreArchiveHeaderValidation struct {
	cleanName string
	directory bool
}

// validateRestoreArchiveFile rejects missing, non-regular, or oversized restore archives.
func validateRestoreArchiveFile(archivePath string, limits RestoreArchiveLimits) error {
	info, err := os.Stat(archivePath)
	if err != nil {
		return fmt.Errorf("statting archive: %w", err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("restore archive must be a regular file")
	}
	if info.Size() > limits.MaxCompressedBytes {
		return newRestoreLimitError("compressed archive exceeds restore limit: %d bytes > %d bytes", info.Size(), limits.MaxCompressedBytes)
	}
	return nil
}

// extractArchive extracts a .tar.gz archive to the given directory with security validation and quotas.
func extractArchive(archivePath, destDir string, limits RestoreArchiveLimits) error {
	return extractArchiveContext(context.Background(), archivePath, destDir, limits)
}

// extractArchiveContext extracts a .tar.gz archive while enforcing entry safety and restore quotas.
func extractArchiveContext(ctx context.Context, archivePath, destDir string, limits RestoreArchiveLimits) error {
	f, err := os.Open(archivePath)
	if err != nil {
		return fmt.Errorf("opening archive: %w", err)
	}
	defer f.Close()

	gr, err := gzip.NewReader(f)
	if err != nil {
		return fmt.Errorf("creating gzip reader: %w", err)
	}
	defer gr.Close()

	tr := tar.NewReader(gr)
	var totalBytes int64
	var archiveEntries int
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("reading tar entry: %w", err)
		}

		validated, err := validateRestoreArchiveHeader(header)
		if err != nil {
			return err
		}
		cleanName := validated.cleanName

		targetPath := filepath.Join(destDir, filepath.FromSlash(cleanName))
		archiveEntries++
		if archiveEntries > limits.MaxFileEntries {
			return newRestoreLimitError(
				"archive file count exceeds restore limit (archive entry count exceeds): %d entries > %d entries",
				archiveEntries,
				limits.MaxFileEntries,
			)
		}

		if validated.directory {
			if err := os.MkdirAll(targetPath, 0700); err != nil {
				return fmt.Errorf("creating directory %s: %w", cleanName, err)
			}
			continue
		}

		if header.Size < 0 {
			return fmt.Errorf("archive entry %s has invalid negative size", cleanName)
		}
		if header.Size > limits.MaxEntryBytes {
			return newRestoreLimitError("archive entry %s exceeds per-entry restore limit: %d bytes > %d bytes", cleanName, header.Size, limits.MaxEntryBytes)
		}
		if header.Size > limits.MaxTotalBytes-totalBytes {
			return newRestoreLimitError("expanded archive exceeds restore limit: %d bytes > %d bytes", totalBytes+header.Size, limits.MaxTotalBytes)
		}

		// Ensure parent directory exists
		if err := os.MkdirAll(filepath.Dir(targetPath), 0700); err != nil {
			return fmt.Errorf("creating parent directory for %s: %w", cleanName, err)
		}

		outFile, err := os.OpenFile(targetPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
		if err != nil {
			return fmt.Errorf("creating file %s: %w", cleanName, err)
		}
		if _, err := copyWithContext(ctx, outFile, tr); err != nil {
			outFile.Close()
			return fmt.Errorf("writing file %s: %w", cleanName, err)
		}
		outFile.Close()
		totalBytes += header.Size
	}

	return nil
}

// validateRestoreArchiveHeader normalizes, type-checks, and allowlists a tar restore entry.
func validateRestoreArchiveHeader(header *tar.Header) (restoreArchiveHeaderValidation, error) {
	if header.Typeflag == tar.TypeSymlink || header.Typeflag == tar.TypeLink {
		return restoreArchiveHeaderValidation{}, fmt.Errorf("archive contains disallowed link entry: %s", header.Name)
	}

	regularFile := header.Typeflag == tar.TypeReg || header.Typeflag == tar.TypeRegA
	directory := header.Typeflag == tar.TypeDir
	if !regularFile && !directory {
		return restoreArchiveHeaderValidation{}, fmt.Errorf("unsupported restore archive entry type %c: %s", header.Typeflag, header.Name)
	}

	cleanName, err := validateRestoreArchiveEntryForExtraction(header.Name, directory)
	if err != nil {
		return restoreArchiveHeaderValidation{}, err
	}
	return restoreArchiveHeaderValidation{cleanName: cleanName, directory: directory}, nil
}
