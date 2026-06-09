package service

// This file defines virtual reachability service behavior and domain orchestration rules.

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/lollinoo/theia/internal/domain"
)

// ProbeTCPReachability performs a lightweight TCP reachability probe.
func ProbeTCPReachability(ctx context.Context, target string, timeout time.Duration, ports []int) error {
	target = strings.TrimSpace(target)
	if target == "" {
		return fmt.Errorf("reachability probe requires a target")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if timeout <= 0 {
		timeout = 5 * time.Second
	}

	var lastErr error
	for _, port := range domain.ResolveProbePorts(nil, nil, ports) {
		if err := ctx.Err(); err != nil {
			return err
		}

		dialer := net.Dialer{Timeout: timeout}
		addr := net.JoinHostPort(target, strconv.Itoa(port))
		conn, err := dialer.DialContext(ctx, "tcp", addr)
		if err == nil {
			_ = conn.Close()
			return nil
		}
		lastErr = err
	}

	if lastErr != nil {
		return fmt.Errorf("reachability probe failed for %s: %w", target, lastErr)
	}

	return fmt.Errorf("reachability probe failed for %s", target)
}

// ProbeVirtualReachability performs a lightweight TCP reachability probe for a virtual node.
func ProbeVirtualReachability(ctx context.Context, ip string, timeout time.Duration, ports []int) error {
	return ProbeTCPReachability(ctx, ip, timeout, ports)
}
