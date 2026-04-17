package observability

import (
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/domain"
)

func TestRegistryHandlerRendersPrometheusSeries(t *testing.T) {
	registry := NewRegistry()
	deviceID := uuid.New()

	registry.SetSchedulerReadyDepth(domain.VolatilityClassStatic, 3)
	registry.SetSchedulerInFlight(2)
	registry.SetSchedulerQueueLag(domain.VolatilityClassStatic, 15*time.Second)
	registry.IncSchedulerTaskDispatch(domain.VolatilityClassStatic)
	registry.IncSchedulerBackpressure(domain.VolatilityClassStatic, "class_limit")
	registry.ObserveSchedulerTaskDuration(domain.VolatilityClassStatic, 250*time.Millisecond)
	registry.IncPollResult(domain.VolatilityClassStatic, true)
	registry.SetDiscoveryNeighborCounts(deviceID, map[domain.DiscoveryProtocol]int{
		domain.DiscoveryProtocolLLDP: 2,
	})
	registry.IncLinkUpsert(domain.DiscoveryProtocolLLDP, domain.LinkUpsertKindCreated)
	registry.IncCacheInvalidation("link_repo")
	registry.IncCacheReload()
	registry.ObserveTopologyMaterialization(125*time.Millisecond, true)
	registry.ObserveWSMessage("broadcast", "snapshot", 512)
	registry.AddUnknownNeighbors(deviceID, domain.DiscoveryProtocolLLDP, 4)
	registry.AddDroppedStateChanges(7)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/metrics", nil)
	registry.ServeHTTP(rec, req)

	body := rec.Body.String()
	if got := rec.Header().Get("Content-Type"); !strings.Contains(got, "text/plain") {
		t.Fatalf("Content-Type = %q, want text/plain", got)
	}
	assertContainsMetric(t, body, `theia_scheduler_ready_queue_depth{volatility_class="static"} 3`)
	assertContainsMetric(t, body, `theia_scheduler_in_flight_tasks 2`)
	assertContainsMetric(t, body, `theia_scheduler_queue_lag_seconds{volatility_class="static"} 15`)
	assertContainsMetric(t, body, `theia_scheduler_task_dispatch_total{volatility_class="static"} 1`)
	assertContainsMetric(t, body, `theia_scheduler_backpressure_total{reason="class_limit",volatility_class="static"} 1`)
	assertContainsMetric(t, body, `theia_poll_results_total{outcome="success",volatility_class="static"} 1`)
	assertContainsMetric(t, body, `theia_discovery_neighbors{device_id="`+deviceID.String()+`",protocol="lldp"} 2`)
	assertContainsMetric(t, body, `theia_link_upserts_total{protocol="lldp",result="created"} 1`)
	assertContainsMetric(t, body, `theia_cache_invalidation_total{source="link_repo"} 1`)
	assertContainsMetric(t, body, `theia_cache_reload_total 1`)
	assertContainsMetric(t, body, `theia_topology_materialization_seconds_count{result="success"} 1`)
	assertContainsMetric(t, body, `theia_ws_messages_total{scope="broadcast",type="snapshot"} 1`)
	assertContainsMetric(t, body, `theia_unknown_neighbors_total{device_id="`+deviceID.String()+`",protocol="lldp"} 4`)
	assertContainsMetric(t, body, `theia_state_changes_dropped_total 7`)
}

func assertContainsMetric(t *testing.T, body, needle string) {
	t.Helper()
	if !strings.Contains(body, needle) {
		t.Fatalf("metrics output missing %q\n%s", needle, body)
	}
}
