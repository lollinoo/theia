package ssh

// This file defines known hosts SSH connectivity and host-key trust behavior.

import (
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"sync"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

// KnownHostsStore manages a file-based SSH known_hosts store with TOFU.
type KnownHostsStore struct {
	filePath string
	mu       sync.Mutex
}

// NewKnownHostsStore creates a known hosts store at the given file path.
// The file is created on demand when the first host key is recorded.
func NewKnownHostsStore(filePath string) (*KnownHostsStore, error) {
	return &KnownHostsStore{filePath: filePath}, nil
}

// HostKeyCallback returns an ssh.HostKeyCallback that verifies host keys
// against the store and auto-trusts new hosts (TOFU).
// On host key mismatch (potential MITM), it returns an error.
func (s *KnownHostsStore) HostKeyCallback() ssh.HostKeyCallback {
	return func(hostname string, remote net.Addr, key ssh.PublicKey) error {
		s.mu.Lock()
		defer s.mu.Unlock()

		// Try to parse existing known_hosts file
		cb, err := knownhosts.New(s.filePath)
		if err != nil {
			// File doesn't exist yet -- TOFU: trust and create
			log.Printf("TOFU: trusting first host key for %s (new known_hosts file)", hostname)
			return s.addHostKey(hostname, remote, key)
		}

		// Check against known hosts
		err = cb(hostname, remote, key)
		if err == nil {
			return nil // Key matches stored key
		}

		// Determine if unknown host or changed key
		var keyErr *knownhosts.KeyError
		if errors.As(err, &keyErr) {
			if len(keyErr.Want) == 0 {
				// Host not in file -- TOFU: trust and record
				log.Printf("TOFU: trusting new host key for %s", hostname)
				return s.addHostKey(hostname, remote, key)
			}
			// Key CHANGED -- potential MITM, reject
			return fmt.Errorf("SSH host key mismatch for %s (possible MITM attack, remove old entry from known_hosts to re-trust): %w", hostname, err)
		}
		return err
	}
}

func (s *KnownHostsStore) addHostKey(hostname string, remote net.Addr, key ssh.PublicKey) error {
	addr := knownhosts.Normalize(remote.String())
	line := knownhosts.Line([]string{addr}, key)

	f, err := os.OpenFile(s.filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return fmt.Errorf("opening known_hosts file: %w", err)
	}
	defer f.Close()

	if _, err := fmt.Fprintln(f, line); err != nil {
		return fmt.Errorf("writing known_hosts entry: %w", err)
	}
	return nil
}
