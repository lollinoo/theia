package service

import (
	"fmt"
	"path/filepath"
	"strings"
)

func pathContains(parent, child string) bool {
	rel, err := filepath.Rel(parent, child)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}

// validatePaths keeps control keys/journals and recovery archives outside the
// directories replaced by restore. Reject overlap before any live mutation.
func (m *Maintenance) validatePaths() error {
	paths := make([]string, 4)
	for i, path := range []string{filepath.Dir(m.StatePath), m.DataDir, m.DeviceBackupDir, m.BackupDir} {
		if path == "" {
			return fmt.Errorf("maintenance requires explicit persistent data paths")
		}
		abs, err := filepath.Abs(path)
		if err != nil {
			return err
		}
		if filepath.Dir(abs) == abs {
			return fmt.Errorf("maintenance data cannot use a filesystem root")
		}
		paths[i] = abs
	}
	for _, data := range paths[1:] {
		if pathContains(paths[0], data) || pathContains(data, paths[0]) {
			return fmt.Errorf("instance secrets and operation journals must be outside application and backup data directories")
		}
	}
	if pathContains(paths[2], paths[3]) || pathContains(paths[3], paths[2]) {
		return fmt.Errorf("instance archives and restored device backup directories must not overlap")
	}
	return nil
}
