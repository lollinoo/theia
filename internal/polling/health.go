package polling

// This file defines health polling policy and freshness-budget behavior.

type QueueSnapshot struct {
	ReadyDepth        int     `json:"ready_depth"`
	LagSeconds        float64 `json:"lag_seconds"`
	ActiveWorkers     int     `json:"active_workers"`
	ConfiguredWorkers int     `json:"configured_workers"`
}

// HealthSnapshot represents health snapshot data used by the package.
type HealthSnapshot struct {
	EssentialOverloaded      bool                     `json:"essential_overloaded"`
	DegradedRisk             bool                     `json:"degraded_risk"`
	EssentialQueueLagSeconds float64                  `json:"essential_queue_lag_seconds"`
	DeadlineMissTotal        uint64                   `json:"deadline_miss_total"`
	ActiveWorkers            int                      `json:"active_workers"`
	ConfiguredWorkers        int                      `json:"configured_workers"`
	Queues                   map[string]QueueSnapshot `json:"queues,omitempty"`
	Warnings                 []CapacityWarning        `json:"warnings,omitempty"`
}

func (h HealthSnapshot) Status() string {
	if h.EssentialOverloaded {
		return "overloaded"
	}
	if h.DegradedRisk || len(h.Warnings) > 0 {
		return "degraded-risk"
	}
	return "ok"
}
