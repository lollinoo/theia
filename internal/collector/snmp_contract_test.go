package collector

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/lollinoo/theia/internal/domain"
	"github.com/lollinoo/theia/internal/snmp"
	"github.com/lollinoo/theia/internal/vendor"
)

type snmpContractFixture struct {
	Version    int      `json:"version"`
	DeviceIP   string   `json:"device_ip"`
	Interfaces []string `json:"interfaces"`
}

func TestSNMPCollectorContractCases(t *testing.T) {
	t.Parallel()

	registry, err := vendor.LoadRegistryFromEmbedded()
	if err != nil {
		t.Fatalf("LoadRegistryFromEmbedded() error = %v", err)
	}

	tests := []struct {
		name           string
		fixture        string
		wantInterfaces int
		wantErr        string
		timeout        time.Duration
		retries        int
	}{
		{name: "valid", fixture: "valid.json", wantInterfaces: 2, timeout: 5 * time.Second, retries: 1},
		{name: "partial", fixture: "partial.json", wantInterfaces: 1, timeout: 5 * time.Second, retries: 1},
		{name: "unreachable", fixture: "unreachable.json", wantErr: "discover device: getting sys info", timeout: 150 * time.Millisecond, retries: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fixture := loadSNMPContractFixture(t, tt.fixture)

			collector := NewStaticCollector(registry, func(target string, creds domain.SNMPCredentials, timeout time.Duration, retries int) (SNMPClient, error) {
				if target != fixture.DeviceIP {
					t.Fatalf("target = %q, want %q", target, fixture.DeviceIP)
				}
				return snmp.NewClient(target, creds, timeout, retries)
			})

			result := collector.Poll(context.Background(), domain.Device{
				ID: uuid.New(),
				IP: fixture.DeviceIP,
				SNMPCredentials: domain.SNMPCredentials{
					Version: domain.SNMPVersionV2c,
					V2c:     &domain.SNMPv2cCredentials{Community: "public"},
				},
			}, tt.timeout, tt.retries, domain.TopologyDiscoveryModeOff)

			if tt.wantErr != "" {
				if result.Err == nil {
					t.Fatalf("expected error containing %q", tt.wantErr)
				}
				if !strings.Contains(result.Err.Error(), tt.wantErr) {
					t.Fatalf("error = %q, want substring %q", result.Err.Error(), tt.wantErr)
				}
				return
			}

			if result.Err != nil {
				t.Fatalf("Poll() error = %v", result.Err)
			}
			if got := len(filterContractInterfaces(result.Interfaces, fixture.Interfaces)); got != tt.wantInterfaces {
				t.Fatalf("interface count = %d, want %d", got, tt.wantInterfaces)
			}
		})
	}
}

func loadSNMPContractFixture(t *testing.T, name string) snmpContractFixture {
	t.Helper()

	path := filepath.Join("testdata", "snmp", name)
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", path, err)
	}

	var fixture snmpContractFixture
	if err := json.Unmarshal(raw, &fixture); err != nil {
		t.Fatalf("json.Unmarshal(%q) error = %v", path, err)
	}
	if fixture.Version != 1 {
		t.Fatalf("fixture version = %d, want 1", fixture.Version)
	}

	return fixture
}

func filterContractInterfaces(interfaces []domain.Interface, allow []string) []domain.Interface {
	filtered := make([]domain.Interface, 0, len(allow))
	for _, iface := range interfaces {
		if slices.Contains(allow, iface.IfName) || slices.Contains(allow, iface.IfDescr) {
			filtered = append(filtered, iface)
		}
	}
	return filtered
}
