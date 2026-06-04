package service

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestRestoreCoordinatorApplyPendingRestore_MalformedMarkerFailsBeforeSideEffects preserves marker parse fail-closed behavior.
func TestRestoreCoordinatorApplyPendingRestore_MalformedMarkerFailsBeforeSideEffects(t *testing.T) {
	stateDir := t.TempDir()
	stagingDir := filepath.Join(stateDir, ".restore-staging")
	if err := os.MkdirAll(stagingDir, 0700); err != nil {
		t.Fatalf("creating staging dir: %v", err)
	}

	markerPath := restoreMarkerFilePath(stateDir)
	if err := os.WriteFile(markerPath, []byte("{"), 0600); err != nil {
		t.Fatalf("writing malformed marker: %v", err)
	}

	externalCommandCalled := false
	stubExternalCommandsWithEnv(t, func(ctx context.Context, env []string, name string, args ...string) ([]byte, error) {
		externalCommandCalled = true
		t.Fatalf("external command %q should not run for a malformed marker", name)
		return nil, nil
	})

	terminationCalled := false
	originalTerminate := terminatePostgresConnections
	terminatePostgresConnections = func(ctx context.Context, dsn string) error {
		terminationCalled = true
		t.Fatal("postgres connection termination should not run for a malformed marker")
		return nil
	}
	t.Cleanup(func() { terminatePostgresConnections = originalTerminate })

	coordinator := NewRestoreCoordinatorWithDSN(
		stateDir,
		"postgres://theia:strong-password@localhost:5432/theia?sslmode=disable",
		filepath.Join(stateDir, "device-backups"),
		filepath.Join(stateDir, "known_hosts"),
	)

	applied, err := coordinator.ApplyPendingRestore()
	if err == nil {
		t.Fatal("ApplyPendingRestore() error = nil, want malformed marker error")
	}
	if !strings.Contains(err.Error(), "parse restore marker") {
		t.Fatalf("ApplyPendingRestore() error = %q, want parse restore marker error", err.Error())
	}
	if applied {
		t.Fatal("ApplyPendingRestore() applied = true, want false")
	}
	if externalCommandCalled {
		t.Fatal("external command ran before marker parse failed closed")
	}
	if terminationCalled {
		t.Fatal("postgres termination ran before marker parse failed closed")
	}
	if _, err := os.Stat(markerPath); err != nil {
		t.Fatalf("marker should remain for inspection after malformed marker failure: %v", err)
	}
	info, err := os.Stat(stagingDir)
	if err != nil {
		t.Fatalf("staging dir should remain after malformed marker failure: %v", err)
	}
	if !info.IsDir() {
		t.Fatal("staging path should remain a directory after malformed marker failure")
	}
}
