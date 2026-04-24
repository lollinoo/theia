package polling

type HealthSnapshot struct {
	EssentialOverloaded      bool              `json:"essential_overloaded"`
	DegradedRisk             bool              `json:"degraded_risk"`
	EssentialQueueLagSeconds float64           `json:"essential_queue_lag_seconds"`
	DeadlineMissTotal        uint64            `json:"deadline_miss_total"`
	ActiveWorkers            int               `json:"active_workers"`
	ConfiguredWorkers        int               `json:"configured_workers"`
	Warnings                 []CapacityWarning `json:"warnings,omitempty"`
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
