package observability

import (
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/domain"
)

func TestDeviceMetricSeriesFollowFleetAndLabels(t *testing.T) {
	r := NewRegistry()
	id, peer := uuid.New(), uuid.New()
	r.SetDiscoveryNeighborCounts(id, map[domain.DiscoveryProtocol]int{domain.DiscoveryProtocolLLDP: 1})
	r.AddUnknownNeighbors(id, domain.DiscoveryProtocolLLDP, 1)
	r.ObserveSNMPCollectorDeviceOperation(id.String(), "before", "old-target", "performance", "get", "ok", time.Second, true)
	r.IncSchedulerScopedBackpressure("background", domain.VolatilityClassPerformance, "device_limit", "device", id.String(), "before")
	r.ObserveSNMPCollectorDeviceOperation(id.String(), "after", "new-target", "performance", "get", "ok", time.Second, true)
	r.IncSchedulerScopedBackpressure("background", domain.VolatilityClassPerformance, "device_limit", "device", id.String(), "after")
	r.ObserveSNMPCollectorDeviceOperation(peer.String(), "peer", "peer-target", "performance", "get", "ok", time.Second, true)
	metrics := string(r.MarshalPrometheus())
	if strings.Contains(metrics, `device="before"`) || strings.Contains(metrics, `scope_name="before"`) || strings.Contains(metrics, `target="old-target"`) {
		t.Fatal("old labels retained")
	}
	if len(r.snmpCollectorDeviceLast) != 2 || len(r.snmpCollectorDeviceSlow) != 2 || len(r.schedulerScopedBackpressureTotal) != 1 {
		t.Fatal("label churn multiplied series")
	}
	r.ForgetDevice(id)
	if strings.Contains(string(r.MarshalPrometheus()), id.String()) {
		t.Fatal("deleted device retained")
	}
	// A poll already in flight may finish after deletion; the next successful
	// scheduler fleet refresh must remove that late series too.
	r.ObserveSNMPCollectorDeviceOperation(id.String(), "late", "old-target", "performance", "get", "ok", time.Second, true)
	r.RetainDeviceMetrics([]domain.Device{{ID: peer}})
	if strings.Contains(string(r.MarshalPrometheus()), id.String()) || len(r.deviceMetricLabels) != 1 {
		t.Fatal("late deleted-device sample retained")
	}
}

func TestScrapeSnapshotOwnsMutableMetricData(t *testing.T) {
	r := NewRegistry()
	r.ObserveSchedulerTaskDuration(domain.VolatilityClassPerformance, time.Second)
	r.IncPollResult(domain.VolatilityClassPerformance, true)
	copy := r.snapshotForScrape()
	if !r.mu.TryLock() {
		t.Fatal("scrape snapshot kept live registry locked")
	}
	r.mu.Unlock()
	r.ObserveSchedulerTaskDuration(domain.VolatilityClassPerformance, time.Second)
	r.IncPollResult(domain.VolatilityClassPerformance, true)
	if copy.schedulerTaskDuration[domain.VolatilityClassPerformance].count != 1 || copy.pollResultsTotal[taskResultKey{VolatilityClass: string(domain.VolatilityClassPerformance), Outcome: "success"}] != 1 {
		t.Fatal("snapshot shares mutable data")
	}
	var group sync.WaitGroup
	group.Add(2)
	go func() {
		defer group.Done()
		for i := 0; i < 100; i++ {
			r.ObserveSchedulerTaskDuration(domain.VolatilityClassPerformance, time.Millisecond)
		}
	}()
	go func() {
		defer group.Done()
		for i := 0; i < 100; i++ {
			r.MarshalPrometheus()
		}
	}()
	group.Wait()
}

func BenchmarkRegistryDeviceScrape(b *testing.B) {
	r := NewRegistry()
	for i := 0; i < 1000; i++ {
		r.ObserveSNMPCollectorDeviceOperation(uuid.NewString(), "device", "target", "performance", "get", "ok", time.Second, true)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		r.MarshalPrometheus()
	}
}
