package service

// This file exercises per-address reachability diagnostics for device services.

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/domain"
	"github.com/lollinoo/theia/internal/polling"
)

func TestDeviceServiceCheckAddressReachabilityUsesAddressOverrideAndDeviceFallback(t *testing.T) {
	deviceRepo := newMockDeviceRepo()
	linkRepo := newMockLinkRepo()
	settingsRepo := newMockSettingsRepo()
	if err := settingsRepo.Set(domain.SettingNetworkProbePorts, "8291"); err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	deviceID := uuid.New()
	device := &domain.Device{
		ID:         deviceID,
		IP:         "192.0.2.10",
		ProbePorts: []int{22, 443},
		Addresses: []domain.DeviceAddress{
			{
				ID:         uuid.New(),
				Address:    "192.0.2.10",
				Role:       domain.DeviceAddressRolePrimary,
				IsPrimary:  true,
				ProbePorts: []int{2222},
			},
			{
				ID:      uuid.New(),
				Address: "198.51.100.10",
				Role:    domain.DeviceAddressRoleManagement,
			},
		},
	}
	if err := deviceRepo.Create(device); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	var mu sync.Mutex
	capturedPorts := map[string][]int{}
	svc := NewDeviceService(deviceRepo, linkRepo, settingsRepo, nil, nil, WithNetworkReachabilityProbe(
		func(_ context.Context, target string, _ time.Duration, ports []int) error {
			mu.Lock()
			defer mu.Unlock()
			capturedPorts[target] = append(capturedPorts[target], ports...)
			return nil
		},
	))

	results, err := svc.CheckDeviceAddressReachability(context.Background(), deviceID)
	if err != nil {
		t.Fatalf("CheckDeviceAddressReachability() error = %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("result count = %d, want 2", len(results))
	}

	mu.Lock()
	defer mu.Unlock()
	if !reflect.DeepEqual(capturedPorts["192.0.2.10"], []int{2222}) {
		t.Fatalf("primary probe ports = %v, want [2222]", capturedPorts["192.0.2.10"])
	}
	if !reflect.DeepEqual(capturedPorts["198.51.100.10"], []int{22, 443}) {
		t.Fatalf("management probe ports = %v, want [22 443]", capturedPorts["198.51.100.10"])
	}
	for _, result := range results {
		if len(result.ReachablePorts) == 0 {
			t.Fatalf("result for %s missing reachable port details", result.Address)
		}
		if !result.Reachable {
			t.Fatalf("result for %s reachable = false, want true", result.Address)
		}
	}

	if !reflect.DeepEqual(results[0].ReachablePorts, []polling.NetworkProbeResult{{Port: 2222, Reachable: true}}) {
		t.Fatalf("primary reachable ports = %#v", results[0].ReachablePorts)
	}
	if !reflect.DeepEqual(results[1].ReachablePorts, []polling.NetworkProbeResult{{Port: 22, Reachable: true}, {Port: 443, Reachable: true}}) {
		t.Fatalf("management reachable ports = %#v", results[1].ReachablePorts)
	}
}

func TestDeviceServiceCheckAddressReachabilityFallsBackToGlobalPorts(t *testing.T) {
	deviceRepo := newMockDeviceRepo()
	linkRepo := newMockLinkRepo()
	settingsRepo := newMockSettingsRepo()
	if err := settingsRepo.Set(domain.SettingNetworkProbePorts, "8291"); err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	deviceID := uuid.New()
	device := &domain.Device{
		ID: deviceID,
		IP: "203.0.113.10",
		Addresses: []domain.DeviceAddress{
			{
				ID:        uuid.New(),
				Address:   "203.0.113.10",
				Role:      domain.DeviceAddressRolePrimary,
				IsPrimary: true,
			},
		},
	}
	if err := deviceRepo.Create(device); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	var capturedPorts []int
	svc := NewDeviceService(deviceRepo, linkRepo, settingsRepo, nil, nil, WithNetworkReachabilityProbe(
		func(_ context.Context, _ string, _ time.Duration, ports []int) error {
			capturedPorts = append([]int(nil), ports...)
			return nil
		},
	))

	results, err := svc.CheckDeviceAddressReachability(context.Background(), deviceID)
	if err != nil {
		t.Fatalf("CheckDeviceAddressReachability() error = %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("result count = %d, want 1", len(results))
	}
	if !reflect.DeepEqual(capturedPorts, []int{8291}) {
		t.Fatalf("probe ports = %v, want [8291]", capturedPorts)
	}
	if !reflect.DeepEqual(results[0].ProbePorts, []int{8291}) {
		t.Fatalf("result probe ports = %v, want [8291]", results[0].ProbePorts)
	}
	if !reflect.DeepEqual(results[0].ReachablePorts, []polling.NetworkProbeResult{{Port: 8291, Reachable: true}}) {
		t.Fatalf("result reachable ports = %v, want [8291]", results[0].ReachablePorts)
	}
}

func TestDeviceServiceCheckAddressReachabilityReturnsMixedPortResults(t *testing.T) {
	deviceRepo := newMockDeviceRepo()
	linkRepo := newMockLinkRepo()
	settingsRepo := newMockSettingsRepo()

	deviceID := uuid.New()
	device := &domain.Device{
		ID:         deviceID,
		IP:         "10.10.10.10",
		ProbePorts: []int{22, 443, 26},
		Addresses: []domain.DeviceAddress{
			{
				ID:         uuid.New(),
				Address:    "10.10.10.10",
				Role:       domain.DeviceAddressRolePrimary,
				IsPrimary:  true,
				ProbePorts: []int{22, 26},
			},
		},
	}
	if err := deviceRepo.Create(device); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	svc := NewDeviceService(deviceRepo, linkRepo, settingsRepo, nil, nil, WithNetworkReachabilityProbe(
		func(_ context.Context, _ string, _ time.Duration, ports []int) error {
			if len(ports) != 1 {
				return errors.New("expected single port")
			}
			switch ports[0] {
			case 22:
				return nil
			case 26:
				return errors.New("closed")
			default:
				return errors.New("unexpected port")
			}
		},
	))

	results, err := svc.CheckDeviceAddressReachability(context.Background(), deviceID)
	if err != nil {
		t.Fatalf("CheckDeviceAddressReachability() error = %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("result count = %d, want 1", len(results))
	}

	if results[0].Reachable {
		t.Fatalf("result reachable = true, want false when mixed probe result")
	}
	if !reflect.DeepEqual(results[0].ReachablePorts, []polling.NetworkProbeResult{
		{Port: 22, Reachable: true},
		{Port: 26, Reachable: false, Error: "closed"},
	}) {
		t.Fatalf("reachable ports = %#v", results[0].ReachablePorts)
	}
}
