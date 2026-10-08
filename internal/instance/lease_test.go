package instance

import (
	"path/filepath"
	"testing"
)

func TestRuntimeAndMaintenanceCannotOverlap(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	release, err := AcquireLease(path)
	if err != nil {
		t.Fatal(err)
	}
	if second, err := AcquireLease(path); err == nil {
		second()
		t.Fatal("concurrent instance process admitted")
	}
	release()
	release, err = AcquireLease(path)
	if err != nil {
		t.Fatal("lease was not released")
	}
	release()
}
