package collector

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/domain"
	"github.com/lollinoo/theia/internal/polling"
	"github.com/lollinoo/theia/internal/snmp"
	"github.com/lollinoo/theia/internal/vendor"
)

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

type EssentialCollector struct {
	registry  *vendor.Registry
	newClient NewSNMPClientFunc
	now       func() time.Time
}

func NewEssentialCollector(registry *vendor.Registry, newClient NewSNMPClientFunc) *EssentialCollector {
	return &EssentialCollector{
		registry:  registry,
		newClient: newClient,
		now:       time.Now,
	}
}

func (c *EssentialCollector) Poll(ctx context.Context, device domain.Device, timeout time.Duration, retries int) EssentialResult {
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
		result.SNMPReachable = polling.TriStateFalse
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
	result.SNMPReachable = polling.TriStateTrue
	result.NetworkReachable = polling.TriStateTrue
	result.Uptime, result.UptimeSecs = convertEssentialField(metrics.Uptime)
	result.CPU, result.CPUPercent = convertEssentialField(metrics.CPU)
	result.Memory, result.MemPercent = convertEssentialField(metrics.Memory)
	result.PollStatus = essentialPollStatus(result)
	return result
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
