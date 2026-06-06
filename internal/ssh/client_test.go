package ssh

// This file exercises client behavior so refactors preserve the documented contract.

import (
	"context"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
)

// mockDialer records dial attempts without making real connections.
type mockDialer struct {
	dialCalled bool
	addr       string
	err        error
}

func (m *mockDialer) Dial(addr string, config *ssh.ClientConfig) (*ssh.Client, error) {
	m.dialCalled = true
	m.addr = addr
	if m.err != nil {
		return nil, m.err
	}
	// Return nil client — we can't create a real one without a server,
	// but we can verify the dial was attempted with correct params.
	return nil, nil
}

func TestNewClientDialAddress(t *testing.T) {
	d := &mockDialer{}
	// This will return a nil client (no real server), which is fine for
	// verifying address formatting.
	_, _ = NewClient(d, "192.168.1.1", 22, "admin", "pass", 5*time.Second, ssh.InsecureIgnoreHostKey())

	if !d.dialCalled {
		t.Fatal("expected Dial to be called")
	}
	if d.addr != "192.168.1.1:22" {
		t.Fatalf("expected addr 192.168.1.1:22, got %s", d.addr)
	}
}

func TestNewClientDialError(t *testing.T) {
	d := &mockDialer{err: context.DeadlineExceeded}
	_, err := NewClient(d, "10.0.0.1", 2222, "user", "pw", time.Second, ssh.InsecureIgnoreHostKey())

	if err == nil {
		t.Fatal("expected error from failing dial")
	}
}

func TestNewClientCustomPort(t *testing.T) {
	d := &mockDialer{}
	_, _ = NewClient(d, "10.0.0.1", 2222, "user", "pw", time.Second, ssh.InsecureIgnoreHostKey())

	if d.addr != "10.0.0.1:2222" {
		t.Fatalf("expected addr 10.0.0.1:2222, got %s", d.addr)
	}
}
