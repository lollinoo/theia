package ssh

// This file exercises known hosts behavior so refactors preserve the documented contract.

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	gossh "golang.org/x/crypto/ssh"
)

// generateTestKey creates a random ECDSA key pair and returns the ssh.PublicKey.
func generateTestKey(t *testing.T) gossh.PublicKey {
	t.Helper()
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generating ECDSA key: %v", err)
	}
	pub, err := gossh.NewPublicKey(&priv.PublicKey)
	if err != nil {
		t.Fatalf("converting to ssh public key: %v", err)
	}
	return pub
}

func TestKnownHostsStoreTOFU(t *testing.T) {
	dir := t.TempDir()
	khPath := filepath.Join(dir, "known_hosts")

	store, err := NewKnownHostsStore(khPath)
	if err != nil {
		t.Fatalf("NewKnownHostsStore: %v", err)
	}

	keyA := generateTestKey(t)
	addr := &net.TCPAddr{IP: net.ParseIP("192.168.1.1"), Port: 22}

	cb := store.HostKeyCallback()
	err = cb("192.168.1.1:22", addr, keyA)
	if err != nil {
		t.Fatalf("TOFU callback should succeed for new host, got: %v", err)
	}

	// Verify file was created and contains the normalized address
	data, err := os.ReadFile(khPath)
	if err != nil {
		t.Fatalf("reading known_hosts file: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "192.168.1.1") {
		t.Fatalf("known_hosts should contain host address, got: %s", content)
	}
}

func TestKnownHostsStoreKnownHost(t *testing.T) {
	dir := t.TempDir()
	khPath := filepath.Join(dir, "known_hosts")

	store, err := NewKnownHostsStore(khPath)
	if err != nil {
		t.Fatalf("NewKnownHostsStore: %v", err)
	}

	keyA := generateTestKey(t)
	addr := &net.TCPAddr{IP: net.ParseIP("192.168.1.1"), Port: 22}

	cb := store.HostKeyCallback()

	// First call: TOFU
	if err := cb("192.168.1.1:22", addr, keyA); err != nil {
		t.Fatalf("TOFU callback failed: %v", err)
	}

	// Get file info after TOFU
	info1, err := os.Stat(khPath)
	if err != nil {
		t.Fatalf("stat known_hosts: %v", err)
	}

	// Second call: same host+key should succeed
	if err := cb("192.168.1.1:22", addr, keyA); err != nil {
		t.Fatalf("known host callback should succeed, got: %v", err)
	}

	// File should not have been modified (no additional lines)
	info2, err := os.Stat(khPath)
	if err != nil {
		t.Fatalf("stat known_hosts: %v", err)
	}
	if info2.Size() != info1.Size() {
		t.Fatalf("file size changed from %d to %d; known host should not modify file", info1.Size(), info2.Size())
	}
}

func TestKnownHostsStoreChangedKey(t *testing.T) {
	dir := t.TempDir()
	khPath := filepath.Join(dir, "known_hosts")

	store, err := NewKnownHostsStore(khPath)
	if err != nil {
		t.Fatalf("NewKnownHostsStore: %v", err)
	}

	keyA := generateTestKey(t)
	keyB := generateTestKey(t)
	addr := &net.TCPAddr{IP: net.ParseIP("192.168.1.1"), Port: 22}

	cb := store.HostKeyCallback()

	// TOFU with keyA
	if err := cb("192.168.1.1:22", addr, keyA); err != nil {
		t.Fatalf("TOFU callback failed: %v", err)
	}

	// Same host with keyB should fail
	err = cb("192.168.1.1:22", addr, keyB)
	if err == nil {
		t.Fatal("expected error for changed host key, got nil")
	}
	if !strings.Contains(err.Error(), "host key mismatch") {
		t.Fatalf("error should contain 'host key mismatch', got: %v", err)
	}
}

func TestKnownHostsStoreRemoveHostAllowsRetrustChangedKeyAndKeepsOtherHosts(t *testing.T) {
	dir := t.TempDir()
	khPath := filepath.Join(dir, "known_hosts")

	store, err := NewKnownHostsStore(khPath)
	if err != nil {
		t.Fatalf("NewKnownHostsStore: %v", err)
	}

	keyA := generateTestKey(t)
	keyB := generateTestKey(t)
	otherKey := generateTestKey(t)
	addr := &net.TCPAddr{IP: net.ParseIP("192.168.1.1"), Port: 22}
	otherAddr := &net.TCPAddr{IP: net.ParseIP("192.168.1.2"), Port: 22}

	cb := store.HostKeyCallback()
	if err := cb("192.168.1.1:22", addr, keyA); err != nil {
		t.Fatalf("TOFU callback failed: %v", err)
	}
	if err := cb("192.168.1.2:22", otherAddr, otherKey); err != nil {
		t.Fatalf("other TOFU callback failed: %v", err)
	}

	if err := cb("192.168.1.1:22", addr, keyB); err == nil {
		t.Fatal("expected changed key to fail before host reset")
	}

	removed, err := store.RemoveHost("192.168.1.1", 22)
	if err != nil {
		t.Fatalf("RemoveHost: %v", err)
	}
	if !removed {
		t.Fatal("RemoveHost removed = false, want true")
	}

	data, err := os.ReadFile(khPath)
	if err != nil {
		t.Fatalf("reading known_hosts after remove: %v", err)
	}
	content := string(data)
	if strings.Contains(content, "192.168.1.1") {
		t.Fatalf("removed host still present in known_hosts: %s", content)
	}
	if !strings.Contains(content, "192.168.1.2") {
		t.Fatalf("unrelated host removed from known_hosts: %s", content)
	}
	if err := cb("192.168.1.2:22", otherAddr, otherKey); err != nil {
		t.Fatalf("other host should still be trusted, got: %v", err)
	}

	if err := cb("192.168.1.1:22", addr, keyB); err != nil {
		t.Fatalf("changed key should be trusted after host reset, got: %v", err)
	}
	if err := cb("192.168.1.1:22", addr, keyA); err == nil {
		t.Fatal("old key should not remain trusted after re-trust")
	}
}

func TestKnownHostsStoreFileCreatedOnDemand(t *testing.T) {
	dir := t.TempDir()
	khPath := filepath.Join(dir, "subdir", "known_hosts")

	// Create the subdirectory (but not the file)
	if err := os.MkdirAll(filepath.Dir(khPath), 0755); err != nil {
		t.Fatalf("creating subdir: %v", err)
	}

	store, err := NewKnownHostsStore(khPath)
	if err != nil {
		t.Fatalf("NewKnownHostsStore should succeed with non-existent path: %v", err)
	}

	// File should not exist yet
	if _, err := os.Stat(khPath); !os.IsNotExist(err) {
		t.Fatal("known_hosts file should not exist before first callback")
	}

	keyA := generateTestKey(t)
	addr := &net.TCPAddr{IP: net.ParseIP("10.0.0.1"), Port: 22}

	cb := store.HostKeyCallback()
	if err := cb("10.0.0.1:22", addr, keyA); err != nil {
		t.Fatalf("TOFU callback failed: %v", err)
	}

	// File should exist now
	if _, err := os.Stat(khPath); err != nil {
		t.Fatalf("known_hosts file should exist after first TOFU: %v", err)
	}
}

func TestKnownHostsStoreConcurrentTOFU(t *testing.T) {
	dir := t.TempDir()
	khPath := filepath.Join(dir, "known_hosts")

	store, err := NewKnownHostsStore(khPath)
	if err != nil {
		t.Fatalf("NewKnownHostsStore: %v", err)
	}

	cb := store.HostKeyCallback()
	const numHosts = 10

	var wg sync.WaitGroup
	errs := make([]error, numHosts)

	for i := 0; i < numHosts; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			key := generateTestKey(t)
			ip := fmt.Sprintf("10.0.0.%d", idx+1)
			addr := &net.TCPAddr{IP: net.ParseIP(ip), Port: 22}
			errs[idx] = cb(ip+":22", addr, key)
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Errorf("goroutine %d failed: %v", i, err)
		}
	}

	// Verify file has 10 lines (one per host)
	data, err := os.ReadFile(khPath)
	if err != nil {
		t.Fatalf("reading known_hosts: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != numHosts {
		t.Fatalf("expected %d lines in known_hosts, got %d", numHosts, len(lines))
	}
}
