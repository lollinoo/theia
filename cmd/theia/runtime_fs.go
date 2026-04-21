package main

import (
	"errors"
	"os"
)

const privateDirMode = 0o700

const privateFileMode = 0o600

func ensurePrivateDir(path string) error {
	info, err := os.Lstat(path)
	if err == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			return wrapEnsurePrivateDirError(errPrivateDirIsSymlink)
		}
	} else if !os.IsNotExist(err) {
		return wrapEnsurePrivateDirError(err)
	}

	if err := os.MkdirAll(path, privateDirMode); err != nil {
		info, statErr := os.Stat(path)
		if statErr == nil && !info.IsDir() {
			return wrapEnsurePrivateDirError(errPrivateDirNotDirectory)
		}
		return wrapEnsurePrivateDirError(err)
	}

	info, err = os.Lstat(path)
	if err != nil {
		return wrapEnsurePrivateDirError(err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return wrapEnsurePrivateDirError(errPrivateDirIsSymlink)
	}
	if !info.IsDir() {
		return wrapEnsurePrivateDirError(errPrivateDirNotDirectory)
	}
	if info.Mode().Perm() == privateDirMode {
		return nil
	}
	if err := os.Chmod(path, privateDirMode); err != nil {
		return wrapEnsurePrivateDirError(err)
	}

	return nil
}

func ensureFileMode(path string, mode os.FileMode) error {
	info, err := os.Stat(path)
	if err != nil {
		return wrapEnsureFileModeError(err)
	}
	if info.IsDir() {
		return wrapEnsureFileModeError(errFileModeIsDirectory)
	}
	if info.Mode().Perm() == mode {
		return nil
	}
	if err := os.Chmod(path, mode); err != nil {
		return wrapEnsureFileModeError(err)
	}

	return nil
}

var errPrivateDirNotDirectory = errors.New("path is not a directory")

var errPrivateDirIsSymlink = errors.New("path is a symlink")

var errFileModeIsDirectory = errors.New("path is a directory")

func wrapEnsurePrivateDirError(err error) error {
	if err == errPrivateDirNotDirectory {
		return &ensureRuntimePathError{prefix: "ensure private dir: ", message: "path is not a directory"}
	}
	if err == errPrivateDirIsSymlink {
		return &ensureRuntimePathError{prefix: "ensure private dir: ", message: "path is a symlink"}
	}
	return &ensureRuntimePathError{prefix: "ensure private dir: ", err: err}
}

func wrapEnsureFileModeError(err error) error {
	if err == errFileModeIsDirectory {
		return &ensureRuntimePathError{prefix: "ensure file mode: ", message: "path is a directory"}
	}
	return &ensureRuntimePathError{prefix: "ensure file mode: ", err: err}
}

type ensureRuntimePathError struct {
	prefix  string
	message string
	err     error
}

func (e *ensureRuntimePathError) Error() string {
	if e.message != "" {
		return e.prefix + e.message
	}
	return e.prefix + e.err.Error()
}

func (e *ensureRuntimePathError) Unwrap() error {
	return e.err
}
