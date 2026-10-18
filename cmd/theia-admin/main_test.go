package main

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPlatformSelection(t *testing.T) {
	dir := t.TempDir()
	ctrl := filepath.Join(dir, "control")
	if err := os.MkdirAll(ctrl, 0700); err != nil {
		t.Fatal(err)
	}
	validKubeConfig := `{
		"name": "theia",
		"namespace": "theia",
		"context": "test-context",
		"release": "v1.0.0",
		"backend_image": "theia-backend:v1.0.0",
		"frontend_image": "theia-frontend:v1.0.0",
		"hostname": "theia.local",
		"tls_secret": "theia-tls"
	}`
	if err := os.WriteFile(filepath.Join(ctrl, "kubernetes.json"), []byte(validKubeConfig), 0600); err != nil {
		t.Fatal(err)
	}

	// 1. Explicitly invalid platform with kubernetes.json present should return platform error
	err := run([]string{"unknown-test-cmd", "-dir", dir, "-platform", "bogus"}, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "platform must be docker or kubernetes") {
		t.Fatalf("expected 'platform must be docker or kubernetes', got: %v", err)
	}

	// 2. Explicitly docker platform with kubernetes.json present should route to docker
	err = run([]string{"unknown-test-cmd", "-dir", dir, "-platform", "docker"}, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "unknown administration command") {
		t.Fatalf("expected docker routing ('unknown administration command'), got: %v", err)
	}

	// 3. Unspecified platform with kubernetes.json present should auto-route to kubernetes
	err = run([]string{"unknown-test-cmd", "-dir", dir}, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "unsupported Kubernetes administration command") {
		t.Fatalf("expected kubernetes routing ('unsupported Kubernetes administration command'), got: %v", err)
	}

	// 4. Unspecified platform without kubernetes.json present should default to docker
	emptyDir := t.TempDir()
	err = run([]string{"unknown-test-cmd", "-dir", emptyDir}, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "unknown administration command") {
		t.Fatalf("expected docker routing ('unknown administration command'), got: %v", err)
	}
}
