package ssh

// This file defines known hosts SSH connectivity and host-key trust behavior.

import (
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"strconv"
	"strings"
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

// RemoveHost removes known_hosts entries for one host and port.
// It returns false when the file or target entry does not exist.
func (s *KnownHostsStore) RemoveHost(host string, port int) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := os.ReadFile(s.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("reading known_hosts file: %w", err)
	}

	target := normalizeKnownHostTarget(host, port)
	content := string(data)
	rawLines := []string{}
	hadTrailingNewline := strings.HasSuffix(content, "\n")
	if content != "" {
		rawLines = strings.Split(strings.TrimSuffix(content, "\n"), "\n")
	}

	removed := false
	lines := make([]string, 0, len(rawLines))
	for _, line := range rawLines {
		updated, lineRemoved := removeKnownHostTargetFromLine(line, target)
		if lineRemoved {
			removed = true
		}
		if updated != "" {
			lines = append(lines, updated)
		}
	}
	if !removed {
		return false, nil
	}

	output := ""
	if len(lines) > 0 {
		output = strings.Join(lines, "\n")
		if hadTrailingNewline {
			output += "\n"
		}
	}
	mode := os.FileMode(0600)
	if info, statErr := os.Stat(s.filePath); statErr == nil {
		mode = info.Mode().Perm()
	}
	if err := os.WriteFile(s.filePath, []byte(output), mode); err != nil {
		return false, fmt.Errorf("writing known_hosts file: %w", err)
	}
	if err := os.Chmod(s.filePath, mode); err != nil {
		return false, fmt.Errorf("setting known_hosts file mode: %w", err)
	}
	return true, nil
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

func normalizeKnownHostTarget(host string, port int) string {
	if port <= 0 {
		port = 22
	}
	return knownhosts.Normalize(net.JoinHostPort(host, strconv.Itoa(port)))
}

func removeKnownHostTargetFromLine(line, target string) (string, bool) {
	if strings.TrimSpace(line) == "" || strings.HasPrefix(strings.TrimSpace(line), "#") {
		return line, false
	}

	fields := strings.Fields(line)
	hostIndex := 0
	if len(fields) == 0 {
		return line, false
	}
	if strings.HasPrefix(fields[0], "@") {
		if len(fields) < 4 {
			return line, false
		}
		hostIndex = 1
	} else if len(fields) < 3 {
		return line, false
	}

	hosts := strings.Split(fields[hostIndex], ",")
	kept := make([]string, 0, len(hosts))
	removed := false
	for _, host := range hosts {
		if host == target {
			removed = true
			continue
		}
		kept = append(kept, host)
	}
	if !removed {
		return line, false
	}
	if len(kept) == 0 {
		return "", true
	}
	fields[hostIndex] = strings.Join(kept, ",")
	return strings.Join(fields, " "), true
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
