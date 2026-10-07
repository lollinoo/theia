package service

import (
	"context"
	"fmt"
	"github.com/lollinoo/theia/internal/domain"
	"github.com/lollinoo/theia/internal/snmp"
	"testing"
)

func TestAddDeviceProbeOwnsCredentialSnapshot(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	seen := make(chan string, 1)
	svc := NewDeviceService(newMockDeviceRepo(), newMockLinkRepo(), newMockSettingsRepo(),
		func(_ string, creds domain.SNMPCredentials, _ domain.TopologyDiscoveryMode) (*snmp.DiscoveryResult, error) {
			close(started)
			<-release
			seen <- creds.V2c.Community
			return nil, fmt.Errorf("probe failed")
		}, nil)
	t.Cleanup(svc.Stop)
	device, err := svc.AddDevice(context.Background(), "10.0.0.1", "router",
		domain.DeviceTypeRouter, domain.SNMPCredentials{Version: domain.SNMPVersionV2c, V2c: &domain.SNMPv2cCredentials{Community: "original"}},
		nil, "", domain.MetricsSourceSNMP, "", "", "", nil)
	if err != nil {
		close(release)
		t.Fatal(err)
	}
	<-started
	device.SNMPCredentials.V2c.Community = "changed by caller"
	close(release)
	svc.WaitForProbes()
	if got := <-seen; got != "original" {
		t.Fatalf("probe credential=%q, want original snapshot", got)
	}
}
