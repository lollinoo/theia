package instance

import (
	"os"
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

func TestAcquireLeaseEmptyPath(t *testing.T) {
	for _, p := range []string{"", "   ", "\t\n"} {
		_, err := AcquireLease(p)
		if err == nil {
			t.Fatalf("expected error for empty statePath %q", p)
		}
	}
	if _, err := os.Stat(".runtime.lock"); err == nil {
		_ = os.Remove(".runtime.lock")
		t.Fatal(".runtime.lock was created for empty path")
	}
}
