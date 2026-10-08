package worker

import "github.com/lollinoo/theia/internal/domain"

// SNMPPollFunc polls a single device via SNMP for live CPU/MEM/UPTIME/TEMP metrics.
// The unused bootstrap factory still requires this signature until it is removed.
// vendorName is used to resolve vendor-specific SNMP OIDs.
type SNMPPollFunc func(target string, creds domain.SNMPCredentials, vendorName string) (domain.DeviceMetrics, error)

// SNMPLinkPollFunc polls a single device via SNMP for interface octet counters
// (ifHCInOctets / ifHCOutOctets), as returned by the legacy SNMP factory.
type SNMPLinkPollFunc func(target string, creds domain.SNMPCredentials) ([]SNMPIfCounter, error)

// SNMPIfCounter holds a single poll's raw 64-bit counter values for one interface.
type SNMPIfCounter struct {
	IfName    string
	InOctets  uint64
	OutOctets uint64
}
