package service

// This file defines restore staging files backup and restore service behavior, including filesystem safety and cleanup expectations.

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

var renameRestoreStagingPath = os.Rename

// copyFile copies a single file from src to dst with private file permissions.
func copyFile(src, dst string) error {
	return copyFileContext(context.Background(), src, dst)
}

// copyFileContext copies one validated artifact with private permissions and cancellation checks.
func copyFileContext(ctx context.Context, src, dst string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("opening %s: %w", src, err)
	}
	defer in.Close()

	if err := os.MkdirAll(filepath.Dir(dst), 0700); err != nil {
		return fmt.Errorf("creating parent directory for %s: %w", dst, err)
	}

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return fmt.Errorf("creating %s: %w", dst, err)
	}
	defer out.Close()

	if _, err := copyWithContext(ctx, out, in); err != nil {
		return fmt.Errorf("copying %s to %s: %w", src, dst, err)
	}

	return os.Chmod(dst, 0600)
}

// moveOrCopyFileForRestoreStagingContext moves a regular file into staging or copies across devices.
func moveOrCopyFileForRestoreStagingContext(ctx context.Context, src, dst string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := validateRestoreStagingSourceFile(src); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0700); err != nil {
		return fmt.Errorf("creating parent directory for %s: %w", dst, err)
	}
	if err := renameRestoreStagingPath(src, dst); err == nil {
		return os.Chmod(dst, 0600)
	} else if !isCrossDeviceRenameError(err) {
		return fmt.Errorf("moving %s to %s: %w", src, dst, err)
	}
	return copyFileContext(ctx, src, dst)
}

// moveOrCopyDirForRestoreStagingContext moves a safe directory tree or copies it across devices.
func moveOrCopyDirForRestoreStagingContext(ctx context.Context, srcDir, dstDir string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := validateRestoreStagingSourceDir(ctx, srcDir); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dstDir), 0700); err != nil {
		return fmt.Errorf("creating parent directory for %s: %w", dstDir, err)
	}
	if err := renameRestoreStagingPath(srcDir, dstDir); err == nil {
		return nil
	} else if !isCrossDeviceRenameError(err) {
		return fmt.Errorf("moving %s to %s: %w", srcDir, dstDir, err)
	}
	return copyDirContext(ctx, srcDir, dstDir)
}

// validateRestoreStagingSourceFile rejects symlinks and non-regular files before activation.
func validateRestoreStagingSourceFile(src string) error {
	info, err := os.Lstat(src)
	if err != nil {
		return fmt.Errorf("stat restore staging source %s: %w", src, err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return fmt.Errorf("restore staging source must be a regular file: %s", src)
	}
	return nil
}

// validateRestoreStagingSourceDir rejects symlinks and special files in optional artifact trees.
func validateRestoreStagingSourceDir(ctx context.Context, srcDir string) error {
	info, err := os.Lstat(srcDir)
	if err != nil {
		return fmt.Errorf("stat restore staging source dir %s: %w", srcDir, err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return fmt.Errorf("restore staging source must be a directory: %s", srcDir)
	}
	return filepath.WalkDir(srcDir, func(entryPath string, entry os.DirEntry, err error) error {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 || (!info.IsDir() && !info.Mode().IsRegular()) {
			return fmt.Errorf("restore staging source entry must be a regular file or directory: %s", entryPath)
		}
		return nil
	})
}

// isCrossDeviceRenameError identifies rename failures that require the copy fallback.
func isCrossDeviceRenameError(err error) bool {
	return errors.Is(err, syscall.EXDEV)
}

// copyDir recursively copies a directory from srcDir to dstDir.
func copyDir(srcDir, dstDir string) error {
	return copyDirContext(context.Background(), srcDir, dstDir)
}

// copyDirContext recursively copies a directory tree with private directory and file permissions.
func copyDirContext(ctx context.Context, srcDir, dstDir string) error {
	return filepath.Walk(srcDir, func(path string, info os.FileInfo, err error) error {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		if err != nil {
			return err
		}

		rel, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dstDir, rel)

		if info.IsDir() {
			return os.MkdirAll(target, 0700)
		}

		return copyFileContext(ctx, path, target)
	})
}
