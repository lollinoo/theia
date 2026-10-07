package ssh

// This file defines reachable SSH connectivity and host-key trust behavior.

import (
	"context"
	"fmt"
	"net"
	"time"
)

// CheckReachable performs a lightweight TCP dial to verify that the SSH port is open.
// Returns nil if reachable, a descriptive error otherwise.
func CheckReachable(host string, port int, timeout time.Duration) error {
	return CheckReachableContext(context.Background(), host, port, timeout)
}

// CheckReachableContext cancels the reachability dial with its caller.
func CheckReachableContext(ctx context.Context, host string, port int, timeout time.Duration) error {
	addr := net.JoinHostPort(host, fmt.Sprintf("%d", port))
	conn, err := (&net.Dialer{Timeout: timeout}).DialContext(ctx, "tcp", addr)
	if err != nil {
		return fmt.Errorf("TCP dial %s failed: %w", addr, err)
	}
	conn.Close()
	return nil
}
