package collector

// This file defines essential metrics collection behavior and normalized collector output.

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/domain"
	"github.com/lollinoo/theia/internal/polling"
	"github.com/lollinoo/theia/internal/snmp"
	"github.com/lollinoo/theia/internal/vendor"
)

// EssentialResult represents essential result data used by the collector.
type EssentialResult struct {
	DeviceID         uuid.UUID
	PollStatus       polling.PollStatus
	NetworkReachable polling.TriState
	SNMPReachable    polling.TriState
	Uptime           polling.FieldState
	CPU              polling.FieldState
	Memory           polling.FieldState
	UptimeSecs       *float64
	CPUPercent       *float64
	MemPercent       *float64
	CollectedAt      time.Time
	Err              error
}

// EssentialCollector represents essential collector data used by the collector.
type EssentialCollector struct {
	registry     *vendor.Registry
	newClient    NewSNMPClientFunc
	networkProbe func(context.Context, string, time.Duration, []int) error
	now          func() time.Time
}

// NewEssentialCollector constructs essential collector state for the collector.
func NewEssentialCollector(registry *vendor.Registry, newClient NewSNMPClientFunc) *EssentialCollector {
	return &EssentialCollector{
		registry:     registry,
		newClient:    newClient,
		networkProbe: probeEssentialTCPReachability,
		now:          time.Now,
	}
}

func (c *EssentialCollector) Poll(ctx context.Context, device domain.Device, timeout time.Duration, retries int, probePorts []int) EssentialResult {
	if ctx == nil {
		ctx = context.Background()
	}

	collectedAt := time.Now().UTC()
	if c != nil && c.now != nil {
		collectedAt = c.now().UTC()
	}

	result := EssentialResult{
		DeviceID:         device.ID,
		PollStatus:       polling.PollStatusFailed,
		NetworkReachable: polling.TriStateUnknown,
		SNMPReachable:    polling.TriStateUnknown,
		Uptime:           polling.FieldStateMissing,
		CPU:              polling.FieldStateMissing,
		Memory:           polling.FieldStateMissing,
		CollectedAt:      collectedAt,
	}

	if err := ctx.Err(); err != nil {
		result.Err = err
		return result
	}
	if c == nil {
		result.Err = errors.New("essential collector is nil")
		return result
	}
	if c.registry == nil {
		result.Err = errors.New("essential collector registry is nil")
		return result
	}
	if c.newClient == nil {
		result.Err = errors.New("essential collector SNMP client factory is nil")
		return result
	}

	client, err := c.newClient(device.IP, device.SNMPCredentials, timeout, retries)
	if err != nil {
		result.Err = fmt.Errorf("create SNMP client: %w", err)
		result.SNMPReachable = polling.TriStateFalse
		return result
	}
	if client == nil {
		result.Err = errors.New("create SNMP client: nil client")
		result.SNMPReachable = polling.TriStateFalse
		return result
	}
	if err := client.Connect(); err != nil {
		result.Err = fmt.Errorf("connect SNMP client: %w", err)
		c.markSNMPFailureNetworkEvidence(ctx, device.IP, timeout, probePorts, &result)
		return result
	}
	defer func() {
		_ = client.Close()
	}()

	if err := ctx.Err(); err != nil {
		result.Err = err
		return result
	}

	vendorName := strings.TrimSpace(device.Vendor)
	if vendorName == "" {
		vendorName = "default"
	}

	metrics := snmp.PollEssentialMetrics(client, c.registry.ResolvePerformanceOIDs(vendorName))
	result.Uptime, result.UptimeSecs = convertEssentialField(metrics.Uptime)
	result.CPU, result.CPUPercent = convertEssentialField(metrics.CPU)
	result.Memory, result.MemPercent = convertEssentialField(metrics.Memory)
	if !essentialMetricsHaveSuccessfulRead(metrics) {
		c.markSNMPFailureNetworkEvidence(ctx, device.IP, timeout, probePorts, &result)
		result.PollStatus = polling.PollStatusFailed
		result.Err = essentialMetricsFailure(metrics)
		return result
	}

	result.SNMPReachable = polling.TriStateTrue
	result.NetworkReachable = polling.TriStateTrue
	result.PollStatus = essentialPollStatus(result)
	return result
}

func (c *EssentialCollector) markSNMPFailureNetworkEvidence(ctx context.Context, target string, timeout time.Duration, probePorts []int, result *EssentialResult) {
	result.SNMPReachable = polling.TriStateFalse
	if c == nil || c.networkProbe == nil {
		return
	}
	if err := c.networkProbe(ctx, target, timeout, probePorts); err == nil {
		result.NetworkReachable = polling.TriStateTrue
		return
	}
	result.NetworkReachable = polling.TriStateFalse
}

func essentialMetricsHaveSuccessfulRead(metrics snmp.EssentialMetricsResult) bool {
	return essentialFieldOK(metrics.Uptime) ||
		essentialFieldOK(metrics.CPU) ||
		essentialFieldOK(metrics.Memory)
}

func essentialFieldOK(field *snmp.EssentialMetricField) bool {
	return field != nil && field.State == "ok"
}

func essentialMetricsFailure(metrics snmp.EssentialMetricsResult) error {
	for _, field := range []*snmp.EssentialMetricField{metrics.Uptime, metrics.CPU, metrics.Memory} {
		if field != nil && field.State == "error" && field.Error != "" {
			return fmt.Errorf("essential SNMP read: %s", field.Error)
		}
	}
	return errors.New("essential SNMP read returned no data")
}

func probeEssentialTCPReachability(ctx context.Context, target string, timeout time.Duration, ports []int) error {
	target = strings.TrimSpace(target)
	if target == "" {
		return errors.New("network probe target is empty")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if timeout <= 0 {
		timeout = time.Second
	}

	var lastErr error
	for _, port := range domain.ResolveProbePorts(nil, nil, ports) {
		if err := ctx.Err(); err != nil {
			return err
		}

		dialer := net.Dialer{Timeout: timeout}
		conn, err := dialer.DialContext(ctx, "tcp", net.JoinHostPort(target, strconv.Itoa(port)))
		if err == nil {
			_ = conn.Close()
			return nil
		}
		lastErr = err
	}
	if lastErr != nil {
		return lastErr
	}
	return errors.New("network probe failed")
}

func convertEssentialField(field *snmp.EssentialMetricField) (polling.FieldState, *float64) {
	if field == nil {
		return polling.FieldStateMissing, nil
	}
	switch field.State {
	case "ok":
		return polling.FieldStateOK, cloneFloat64Ptr(field.Value)
	case "error":
		return polling.FieldStateError, nil
	default:
		return polling.FieldStateMissing, nil
	}
}

func essentialPollStatus(result EssentialResult) polling.PollStatus {
	if result.SNMPReachable != polling.TriStateTrue {
		return polling.PollStatusFailed
	}
	if result.Uptime == polling.FieldStateOK && result.CPU == polling.FieldStateOK && result.Memory == polling.FieldStateOK {
		return polling.PollStatusComplete
	}
	return polling.PollStatusPartial
}
