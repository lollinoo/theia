package ssh

import (
	"fmt"
	"net"
	"time"
)

// CheckReachable performs a lightweight TCP dial to verify that the SSH port is open.
// Returns nil if reachable, a descriptive error otherwise.
func CheckReachable(host string, port int, timeout time.Duration) error {
	addr := net.JoinHostPort(host, fmt.Sprintf("%d", port))
	conn, err := net.DialTimeout("tcp", addr, timeout)
	if err != nil {
		return fmt.Errorf("TCP dial %s failed: %w", addr, err)
	}
	conn.Close()
	return nil
}
