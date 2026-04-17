package scalelab

import "time"

type Profile struct {
	Name                 string        `json:"name"`
	DeviceCount          int           `json:"device_count"`
	PerformanceInterval  time.Duration `json:"performance_interval"`
	OperationalInterval  time.Duration `json:"operational_interval"`
	StaticInterval       time.Duration `json:"static_interval"`
	DefaultReplayPasses  int           `json:"default_replay_passes"`
	DefaultBurstAdds     int           `json:"default_burst_adds"`
	DefaultUnresolvedAdd int           `json:"default_unresolved_add"`
}

type Scenario struct {
	Name                     string        `json:"name"`
	Duration                 time.Duration `json:"duration"`
	ReplayPasses             int           `json:"replay_passes"`
	DatabaseSlowdown         time.Duration `json:"database_slowdown"`
	SNMPTimeoutRate          float64       `json:"snmp_timeout_rate"`
	BurstAdds                int           `json:"burst_adds"`
	BurstUnresolvedNeighbors int           `json:"burst_unresolved_neighbors"`
}

type ReplayObservation struct {
	LocalDeviceID  string `json:"local_device_id"`
	RemoteIdentity string `json:"remote_identity"`
	RemoteDeviceID string `json:"remote_device_id,omitempty"`
	LocalPort      string `json:"local_port"`
	RemotePort     string `json:"remote_port"`
	Protocol       string `json:"protocol"`
	SelfNeighbor   bool   `json:"self_neighbor,omitempty"`
}

type ReplayFixture struct {
	Name         string              `json:"name"`
	Observations []ReplayObservation `json:"observations"`
}

type LatencySummary struct {
	P50Ms float64 `json:"p50_ms"`
	P95Ms float64 `json:"p95_ms"`
	P99Ms float64 `json:"p99_ms"`
	MaxMs float64 `json:"max_ms"`
}

type ReplayReport struct {
	FixtureName       string         `json:"fixture_name"`
	ObservationCount  int            `json:"observation_count"`
	ResolvedCount     int            `json:"resolved_count"`
	UnresolvedCount   int            `json:"unresolved_count"`
	SelfNeighborCount int            `json:"self_neighbor_count"`
	LinkEvents        map[string]int `json:"link_events"`
	Latency           LatencySummary `json:"latency"`
}

type WorkloadReport struct {
	PerformanceTasksPerMinute float64 `json:"performance_tasks_per_minute"`
	OperationalTasksPerMinute float64 `json:"operational_tasks_per_minute"`
	StaticTasksPerMinute      float64 `json:"static_tasks_per_minute"`
	BurstAdds                 int     `json:"burst_adds"`
	BurstUnresolvedNeighbors  int     `json:"burst_unresolved_neighbors"`
}

type LabReport struct {
	Profile  Profile        `json:"profile"`
	Scenario Scenario       `json:"scenario"`
	Workload WorkloadReport `json:"workload"`
	Replay   ReplayReport   `json:"replay"`
}
