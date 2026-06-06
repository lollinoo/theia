package service

// This file exercises device discovery support behavior so refactors preserve the documented contract.

import (
	"testing"

	"github.com/lollinoo/theia/internal/domain"
	"github.com/lollinoo/theia/internal/snmp"
)

func TestDedupePreferredDiscoveredNeighbors_PrefersPhysicalRemotePortWhenLocalInterfaceIsMissingOnAllVariants(t *testing.T) {
	neighbors := dedupePreferredDiscoveredNeighbors([]snmp.NeighborInfo{
		{
			RemoteSysName:   "PRE-M-PZ-GALLITELLO_DORSALE",
			RemotePortID:    "br_eoip_radius_vlan/eoip_gallitello_uff",
			LocalIfName:     "",
			Protocol:        domain.DiscoveryProtocolLLDP,
			RemoteChassisID: "aa:bb:cc:dd:ee:ff",
		},
		{
			RemoteSysName:   "PRE-M-PZ-GALLITELLO_DORSALE",
			RemotePortID:    "ether6-Link_Ufficio",
			LocalIfName:     "",
			Protocol:        domain.DiscoveryProtocolLLDP,
			RemoteChassisID: "aa:bb:cc:dd:ee:ff",
		},
	})

	if len(neighbors) != 1 {
		t.Fatalf("expected only the physical neighbor to remain, got %d", len(neighbors))
	}
	if neighbors[0].RemotePortID != "ether6-Link_Ufficio" {
		t.Fatalf("expected physical remote port to survive, got %q", neighbors[0].RemotePortID)
	}
}
