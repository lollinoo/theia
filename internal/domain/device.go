package domain

import (
	"time"

	"github.com/google/uuid"
)

// DeviceType represents the type of network device.
type DeviceType string

const (
	DeviceTypeRouter  DeviceType = "router"
	DeviceTypeSwitch  DeviceType = "switch"
	DeviceTypeAP      DeviceType = "ap"
	DeviceTypeVirtual DeviceType = "virtual"
	DeviceTypeUnknown DeviceType = "unknown"
)

// DeviceStatus represents the current operational status of a device.
type DeviceStatus string

const (
	DeviceStatusUp      DeviceStatus = "up"
	DeviceStatusDown    DeviceStatus = "down"
	DeviceStatusProbing DeviceStatus = "probing"
	DeviceStatusUnknown DeviceStatus = "unknown"
)

// MetricsSource indicates where live metrics are collected from for this device.
type MetricsSource string

const (
	MetricsSourcePrometheus             MetricsSource = "prometheus"
	MetricsSourceSNMP                   MetricsSource = "snmp"
	MetricsSourcePrometheusSNMPFallback MetricsSource = "prometheus_snmp_fallback"
	MetricsSourceNone                   MetricsSource = "none"
)

// TopologyDiscoveryMode controls whether static SNMP discovery should walk LLDP/CDP.
type TopologyDiscoveryMode string

const (
	TopologyDiscoveryModeInherit       TopologyDiscoveryMode = "inherit"
	TopologyDiscoveryModeOff           TopologyDiscoveryMode = "off"
	TopologyDiscoveryModeLLDP          TopologyDiscoveryMode = "lldp"
	TopologyDiscoveryModeLLDPCDP       TopologyDiscoveryMode = "lldp_cdp"
	TopologyDiscoveryModeBootstrapOnce TopologyDiscoveryMode = "bootstrap_once"
)

// TopologyBootstrapState tracks a one-shot discovery window for bootstrap/manual runs.
type TopologyBootstrapState string

const (
	TopologyBootstrapStateIdle              TopologyBootstrapState = "idle"
	TopologyBootstrapStatePending           TopologyBootstrapState = "pending"
	TopologyBootstrapStateFollowupScheduled TopologyBootstrapState = "followup_scheduled"
	TopologyBootstrapStateCompleted         TopologyBootstrapState = "completed"
)

func NormalizeTopologyDiscoveryMode(mode TopologyDiscoveryMode, fallback TopologyDiscoveryMode) TopologyDiscoveryMode {
	switch mode {
	case TopologyDiscoveryModeInherit,
		TopologyDiscoveryModeOff,
		TopologyDiscoveryModeLLDP,
		TopologyDiscoveryModeLLDPCDP,
		TopologyDiscoveryModeBootstrapOnce:
		return mode
	case "":
		if fallback != "" {
			return fallback
		}
		return TopologyDiscoveryModeInherit
	default:
		if fallback != "" {
			return fallback
		}
		return TopologyDiscoveryModeInherit
	}
}

func NormalizeTopologyBootstrapState(state TopologyBootstrapState) TopologyBootstrapState {
	switch state {
	case TopologyBootstrapStateIdle,
		TopologyBootstrapStatePending,
		TopologyBootstrapStateFollowupScheduled,
		TopologyBootstrapStateCompleted:
		return state
	default:
		return TopologyBootstrapStateIdle
	}
}

func ResolveTopologyDiscoveryMode(device *Device, defaultMode TopologyDiscoveryMode) TopologyDiscoveryMode {
	defaultMode = NormalizeTopologyDiscoveryMode(defaultMode, TopologyDiscoveryModeLLDPCDP)
	if device == nil {
		return defaultMode
	}
	switch NormalizeTopologyBootstrapState(device.TopologyBootstrapState) {
	case TopologyBootstrapStatePending, TopologyBootstrapStateFollowupScheduled:
		return TopologyDiscoveryModeBootstrapOnce
	}

	mode := NormalizeTopologyDiscoveryMode(device.TopologyDiscoveryMode, TopologyDiscoveryModeInherit)
	if mode == TopologyDiscoveryModeInherit {
		mode = defaultMode
	}
	if mode == TopologyDiscoveryModeBootstrapOnce &&
		NormalizeTopologyBootstrapState(device.TopologyBootstrapState) == TopologyBootstrapStateCompleted {
		return TopologyDiscoveryModeOff
	}
	return mode
}

// SNMPVersion indicates which SNMP version is configured.
type SNMPVersion string

const (
	SNMPVersionV2c SNMPVersion = "2c"
	SNMPVersionV3  SNMPVersion = "3"
)

// SNMPv2cCredentials holds SNMP v2c authentication data.
type SNMPv2cCredentials struct {
	Community string `json:"community" yaml:"community"`
}

// SNMPv3Credentials holds SNMP v3 authentication and privacy data.
type SNMPv3Credentials struct {
	Username      string `json:"username" yaml:"username"`
	AuthProtocol  string `json:"auth_protocol" yaml:"auth_protocol"` // MD5, SHA
	AuthPassword  string `json:"auth_password" yaml:"auth_password"`
	PrivProtocol  string `json:"priv_protocol" yaml:"priv_protocol"` // DES, AES
	PrivPassword  string `json:"priv_password" yaml:"priv_password"`
	SecurityLevel string `json:"security_level" yaml:"security_level"` // noAuthNoPriv, authNoPriv, authPriv
}

// SNMPCredentials wraps both v2c and v3 credential options.
type SNMPCredentials struct {
	Version SNMPVersion         `json:"version" yaml:"version"`
	V2c     *SNMPv2cCredentials `json:"v2c,omitempty" yaml:"v2c,omitempty"`
	V3      *SNMPv3Credentials  `json:"v3,omitempty" yaml:"v3,omitempty"`
}

// Interface represents a network interface on a device.
type Interface struct {
	ID          uuid.UUID `json:"id"`
	DeviceID    uuid.UUID `json:"device_id"`
	IfIndex     int       `json:"if_index"`
	IfName      string    `json:"if_name"`
	IfDescr     string    `json:"if_descr"`
	Speed       int64     `json:"speed"` // bits per second
	AdminStatus string    `json:"admin_status"`
	OperStatus  string    `json:"oper_status"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// Device represents a managed or discovered network device.
type Device struct {
	ID                             uuid.UUID              `json:"id"`
	Hostname                       string                 `json:"hostname"`
	IP                             string                 `json:"ip"`
	Notes                          *string                `json:"notes"`
	SNMPCredentials                SNMPCredentials        `json:"snmp_credentials"`
	DeviceType                     DeviceType             `json:"device_type"`
	PollClass                      PollClass              `json:"poll_class"`
	PollIntervalOverride           *int                   `json:"poll_interval_override"`
	Status                         DeviceStatus           `json:"status"`
	SysName                        string                 `json:"sys_name"`
	SysDescr                       string                 `json:"sys_descr"`
	SysObjectID                    string                 `json:"sys_object_id"`
	HardwareModel                  string                 `json:"hardware_model"`
	Vendor                         string                 `json:"vendor"`  // vendor name from vendor registry (e.g. "mikrotik", "default")
	Managed                        bool                   `json:"managed"` // true=user-added, false=discovered placeholder
	Tags                           map[string]string      `json:"tags"`
	Interfaces                     []Interface            `json:"interfaces"`
	AreaIDs                        []uuid.UUID            `json:"area_ids"`
	MetricsSource                  MetricsSource          `json:"metrics_source"`
	PrometheusLabelName            string                 `json:"prometheus_label_name"`
	PrometheusLabelValue           string                 `json:"prometheus_label_value"`
	TopologyDiscoveryMode          TopologyDiscoveryMode  `json:"topology_discovery_mode"`
	EffectiveTopologyDiscoveryMode TopologyDiscoveryMode  `json:"effective_topology_discovery_mode"`
	TopologyBootstrapState         TopologyBootstrapState `json:"topology_bootstrap_state"`
	LastTopologyDiscoveryAt        *time.Time             `json:"last_topology_discovery_at"`
	LastTopologyDiscoveryResult    string                 `json:"last_topology_discovery_result"`
	CreatedAt                      time.Time              `json:"created_at"`
	UpdatedAt                      time.Time              `json:"updated_at"`
}

// DeviceRepository defines persistence operations for devices.
type DeviceRepository interface {
	Create(device *Device) error
	GetByID(id uuid.UUID) (*Device, error)
	GetByIP(ip string) (*Device, error)
	GetBySysName(sysName string) (*Device, error)
	GetAll() ([]Device, error)
	Update(device *Device) error
	Delete(id uuid.UUID) error
}
