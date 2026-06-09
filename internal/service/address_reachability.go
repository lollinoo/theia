package service

// This file defines on-demand device address reachability diagnostics.

import (
	"context"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/domain"
	"github.com/lollinoo/theia/internal/polling"
)

// AddressReachabilityResult is the per-address outcome for a reachability probe.
type AddressReachabilityResult struct {
	AddressID      uuid.UUID
	Address        string
	Role           domain.DeviceAddressRole
	Label          string
	IsPrimary      bool
	ProbePorts     []int
	ReachablePorts []polling.NetworkProbeResult
	Reachable      bool
	Error          string
}

// CheckDeviceAddressReachability probes each normalized address for a device.
func (s *DeviceService) CheckDeviceAddressReachability(ctx context.Context, id uuid.UUID) ([]AddressReachabilityResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	device, err := s.GetDevice(ctx, id)
	if err != nil {
		return nil, err
	}
	domain.NormalizeDeviceAddresses(device)

	globalPorts := s.globalNetworkProbePorts()
	timeout := s.networkProbeTimeout()
	probe := s.networkProbe
	if probe == nil {
		probe = ProbeTCPReachability
	}

	results := make([]AddressReachabilityResult, len(device.Addresses))
	sem := make(chan struct{}, 4)
	var wg sync.WaitGroup
	for i, address := range device.Addresses {
		address := address
		target := strings.TrimSpace(address.Address)
		ports := domain.ResolveProbePorts(address.ProbePorts, device.ProbePorts, globalPorts)
		results[i] = AddressReachabilityResult{
			AddressID:  address.ID,
			Address:    target,
			Role:       domain.NormalizeDeviceAddressRole(address.Role),
			Label:      strings.TrimSpace(address.Label),
			IsPrimary:  address.IsPrimary,
			ProbePorts: ports,
		}
		if target == "" {
			results[i].Error = "address is empty"
			continue
		}
		select {
		case sem <- struct{}{}:
		case <-ctx.Done():
			results[i].Error = ctx.Err().Error()
			continue
		}

		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			defer func() { <-sem }()
			result := AddressReachabilityResult{
				AddressID:  address.ID,
				Address:    target,
				Role:       domain.NormalizeDeviceAddressRole(address.Role),
				Label:      strings.TrimSpace(address.Label),
				IsPrimary:  address.IsPrimary,
				ProbePorts: ports,
			}

			result.ReachablePorts = probeAddressPorts(ctx, probe, target, timeout, ports)
			if len(result.ReachablePorts) == 0 {
				result.Error = ""
			} else {
				for _, probeResult := range result.ReachablePorts {
					if !probeResult.Reachable {
						if result.Error == "" {
							result.Error = probeResult.Error
						}
					}
				}
				result.Reachable = true
				for _, probeResult := range result.ReachablePorts {
					if !probeResult.Reachable {
						result.Reachable = false
						break
					}
				}
			}

			results[index] = result
		}(i)
	}
	wg.Wait()
	return results, nil
}

func probeAddressPorts(ctx context.Context, probe func(context.Context, string, time.Duration, []int) error, target string, timeout time.Duration, ports []int) []polling.NetworkProbeResult {
	if probe == nil {
		return nil
	}

	results := make([]polling.NetworkProbeResult, 0, len(ports))
	for _, port := range ports {
		err := probe(ctx, target, timeout, []int{port})
		entry := polling.NetworkProbeResult{Port: port, Reachable: err == nil}
		if err != nil {
			entry.Error = err.Error()
		}
		results = append(results, entry)
	}

	if len(results) > 1 {
		sort.Slice(results, func(i, j int) bool {
			return results[i].Port < results[j].Port
		})
	}

	return results
}

func (s *DeviceService) globalNetworkProbePorts() []int {
	if s == nil || s.settingsRepo == nil {
		return domain.CoerceNetworkProbePortsCSV("")
	}
	value, err := s.settingsRepo.Get(domain.SettingNetworkProbePorts)
	if err != nil {
		return domain.CoerceNetworkProbePortsCSV("")
	}
	return domain.CoerceNetworkProbePortsCSV(value)
}

func (s *DeviceService) networkProbeTimeout() time.Duration {
	if s == nil || s.settingsRepo == nil {
		return 5 * time.Second
	}
	value, err := s.settingsRepo.Get(domain.SettingSNMPTimeout)
	if err != nil {
		return 5 * time.Second
	}
	seconds := domain.CoerceConstrainedInt(domain.SettingSNMPTimeout, value, 5)
	return time.Duration(seconds) * time.Second
}

func deviceTargetProbePorts(device domain.Device, target string, globalPorts []int) []int {
	return domain.ResolveProbePorts(deviceAddressProbePorts(device, target), device.ProbePorts, globalPorts)
}

func deviceAddressProbePorts(device domain.Device, target string) []int {
	normalizedTarget := domain.NormalizeDeviceAddressValue(target)
	if normalizedTarget == "" {
		return nil
	}
	for _, address := range device.Addresses {
		if domain.NormalizeDeviceAddressValue(address.Address) == normalizedTarget {
			return address.ProbePorts
		}
	}
	return nil
}
