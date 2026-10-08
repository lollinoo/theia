package observability

import (
	"database/sql"
	"fmt"
	"strings"
)

// DatabasePool names a process-local SQL pool for saturation metrics.
// Stats reads pool counters without making a database query.
type DatabasePool struct {
	Name string
	DB   *sql.DB
}

func marshalDatabasePoolMetrics(pools []DatabasePool) []byte {
	if len(pools) == 0 {
		return nil
	}
	var b strings.Builder
	metrics := []struct {
		name  string
		help  string
		kind  string
		value func(sql.DBStats) float64
	}{
		{"max_open_connections", "Configured SQL pool connection limit.", "gauge", func(s sql.DBStats) float64 { return float64(s.MaxOpenConnections) }},
		{"open_connections", "Open SQL pool connections.", "gauge", func(s sql.DBStats) float64 { return float64(s.OpenConnections) }},
		{"in_use_connections", "SQL pool connections in use.", "gauge", func(s sql.DBStats) float64 { return float64(s.InUse) }},
		{"idle_connections", "Idle SQL pool connections.", "gauge", func(s sql.DBStats) float64 { return float64(s.Idle) }},
		{"wait_total", "SQL pool waits for a connection.", "counter", func(s sql.DBStats) float64 { return float64(s.WaitCount) }},
		{"wait_seconds_total", "Cumulative SQL pool connection wait time.", "counter", func(s sql.DBStats) float64 { return s.WaitDuration.Seconds() }},
	}
	// Take one coherent snapshot per pool rather than reading Stats for each series.
	stats := make([]sql.DBStats, len(pools))
	for i, pool := range pools {
		if pool.DB != nil {
			stats[i] = pool.DB.Stats()
		}
	}
	for _, metric := range metrics {
		name := "theia_db_pool_" + metric.name
		fmt.Fprintf(&b, "# HELP %s %s\n# TYPE %s %s\n", name, metric.help, name, metric.kind)
		for i, pool := range pools {
			if pool.DB != nil {
				fmt.Fprintf(&b, "%s{pool=%q} %s\n", name, pool.Name, formatFloat(metric.value(stats[i])))
			}
		}
	}
	return []byte(b.String())
}
