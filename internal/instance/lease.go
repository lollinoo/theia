package instance

import (
	"fmt"
	"os"
	"path/filepath"
)

// AcquireLease excludes another application or maintenance process for this
// instance. The OS releases it after a crash; no stale lock deletion is needed.
// The same persistent path must be mounted in application containers and Jobs.
func AcquireLease(statePath string) (func(), error) {
	if err := PrivateDirectory(filepath.Dir(statePath)); err != nil {
		return nil, err
	}
	path := statePath + ".runtime.lock"
	if _, err := os.Lstat(path); err == nil {
		if err := privateRegularFile(path); err != nil {
			return nil, err
		}
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err := lockFile(f); err != nil {
		f.Close()
		return nil, fmt.Errorf("instance is busy; stop the application or finish the active maintenance operation")
	}
	return func() { unlockFile(f); _ = f.Close() }, nil
}
