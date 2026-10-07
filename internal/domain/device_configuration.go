package domain

// DeviceConfigurationFields identifies fields explicitly changed by a request,
// plus runtime invariants changed by virtual-device normalization.
type DeviceConfigurationFields struct {
	Hostname, IP, Addresses, ProbePorts, Notes, Tags, SNMPCredentials bool
	Vendor, MetricsSource, PrometheusLabelName, PrometheusLabelValue  bool
	TopologyDiscoveryMode, TopologyBootstrapState                     bool
	PollingEnabled, PollIntervalOverride, AreaIDs, Status             bool
}
