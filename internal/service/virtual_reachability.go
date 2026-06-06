package service

// This file defines virtual reachability service behavior and domain orchestration rules.

import (
	"context"
	"fmt"
	"net"
	"strings"
	"time"
)

// virtualReachabilityPorts is the set of TCP ports attempted when probing a
// virtual node for basic reachability. The probe succeeds if any port accepts
// a connection.
var virtualReachabilityPorts = []string{"80", "443", "22"}

// ProbeVirtualReachability performs a lightweight TCP reachability probe for a
// virtual node. It returns nil as soon as any common port accepts a
// connection; otherwise it returns the last dial error.
func ProbeVirtualReachability(ctx context.Context, ip string, timeout time.Duration) error {
	ip = strings.TrimSpace(ip)
	if ip == "" {
		return fmt.Errorf("virtual reachability probe requires an IP")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if timeout <= 0 {
		timeout = 5 * time.Second
	}

	var lastErr error
	for _, port := range virtualReachabilityPorts {
		if err := ctx.Err(); err != nil {
			return err
		}

		addr := net.JoinHostPort(ip, port)
		conn, err := net.DialTimeout("tcp", addr, timeout)
		if err == nil {
			_ = conn.Close()
			return nil
		}
		lastErr = err
	}

	if lastErr != nil {
		return fmt.Errorf("virtual reachability probe failed for %s: %w", ip, lastErr)
	}

	return fmt.Errorf("virtual reachability probe failed for %s", ip)
}
