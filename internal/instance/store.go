package instance

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// Store persists private state atomically and serializes changes between processes.
// Its directory must live on persistent storage, outside any restore staging tree.
type Store struct{ Path string }

// PrivateDirectory rejects symlink components and restricts only the leaf directory.
func PrivateDirectory(path string) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	if filepath.Dir(abs) == abs {
		return fmt.Errorf("instance storage cannot be a filesystem root")
	}
	var check func(string) error
	check = func(p string) error {
		info, err := os.Lstat(p)
		if os.IsNotExist(err) {
			parent := filepath.Dir(p)
			if err := check(parent); err != nil {
				return err
			}
			if err := os.Mkdir(p, 0700); err != nil && !os.IsExist(err) {
				return err
			}
			info, err = os.Lstat(p)
		} else if err == nil && filepath.Dir(p) != p {
			if err := check(filepath.Dir(p)); err != nil {
				return err
			}
		}
		if err != nil {
			return err
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("private storage must contain only real directories")
		}
		return nil
	}
	if err := check(abs); err != nil {
		return err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSticky != 0 {
		return fmt.Errorf("instance storage cannot be a shared temporary directory")
	}
	return os.Chmod(abs, 0700)
}

func privateRegularFile(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("private state must be a regular file")
	}
	if info.Mode().Perm()&0077 != 0 {
		return fmt.Errorf("private state permissions must be 0600")
	}
	return nil
}

// Load never creates state. Missing, corrupt, and insecure state are fatal errors.
func (s Store) Load() (*State, error) {
	if err := privateRegularFile(s.Path); err != nil {
		return nil, fmt.Errorf("read instance state: %w", err)
	}
	f, err := os.Open(s.Path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var state State
	decoder := json.NewDecoder(io.LimitReader(f, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&state); err != nil {
		return nil, fmt.Errorf("invalid instance state JSON")
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return nil, fmt.Errorf("instance state has trailing data")
	}
	if err := state.Validate(); err != nil {
		return nil, err
	}
	return &state, nil
}

// Create refuses existing state, including corrupt state; it never rekeys an instance.
func (s Store) Create(state *State) error {
	return s.locked(func() error {
		if _, err := os.Lstat(s.Path); !os.IsNotExist(err) {
			if err != nil {
				return err
			}
			return fmt.Errorf("instance state already exists; use import or maintenance on the existing instance")
		}
		return s.write(state)
	})
}

// Update reloads under the lock to prevent lost rotations and partial state writes.
func (s Store) Update(change func(*State) error) error {
	return s.locked(func() error {
		state, err := s.Load()
		if err != nil {
			return err
		}
		if err := change(state); err != nil {
			return err
		}
		return s.write(state)
	})
}

func (s Store) locked(work func() error) error {
	if s.Path == "" {
		return fmt.Errorf("instance state path is required")
	}
	if err := PrivateDirectory(filepath.Dir(s.Path)); err != nil {
		return err
	}
	lockPath := s.Path + ".lock"
	if _, err := os.Lstat(lockPath); err == nil {
		if err := privateRegularFile(lockPath); err != nil {
			return err
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	defer f.Close()
	if err := lockFile(f); err != nil {
		return fmt.Errorf("instance state is busy: %w", err)
	}
	defer unlockFile(f)
	return work()
}

func (s Store) write(state *State) error {
	if err := state.Validate(); err != nil {
		return err
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return WritePrivateFile(s.Path, append(data, '\n'))
}

// WritePrivateFile publishes complete private bytes using fsync and atomic rename.
func WritePrivateFile(path string, data []byte) error {
	if err := PrivateDirectory(filepath.Dir(path)); err != nil {
		return err
	}
	if _, err := os.Lstat(path); err == nil {
		if err := privateRegularFile(path); err != nil {
			return err
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".private-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if err := preservePrivateFileOwner(f, path); err != nil {
		f.Close()
		return err
	}
	_, writeErr := f.Write(data)
	syncErr := f.Sync()
	closeErr := f.Close()
	if err := errors.Join(writeErr, syncErr, closeErr); err != nil {
		return err
	}
	if err := os.Rename(f.Name(), path); err != nil {
		return err
	}
	return syncDirectory(filepath.Dir(path))
}
