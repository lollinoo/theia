package worker

import (
	"context"
	"testing"
	"time"

	"github.com/lollinoo/theia/internal/collector"
	"github.com/lollinoo/theia/internal/domain"
	"github.com/lollinoo/theia/internal/scheduler"
	"github.com/lollinoo/theia/internal/ws"
)

type hostnameOnlyPrometheusClient struct {
	fakePrometheusClient
	probeCalls int
}

func (c *hostnameOnlyPrometheusClient) QueryProbeStatus(context.Context, []string) (map[string]bool, error) {
	c.probeCalls++
	return nil, context.DeadlineExceeded
}

func TestPhysicalPerformancePollingKeepsHostnameWithoutProbeQuery(t *testing.T) {
	device := newDetailSubscriptionTestDevice()
	pipeline := newDetailSubscriptionTestPipeline(t, nil)
	client := &hostnameOnlyPrometheusClient{fakePrometheusClient: fakePrometheusClient{
		hostnames: map[string]string{device.IP: "prometheus-hostname"},
	}}
	pipeline.prometheus = collector.NewPrometheusCollector(client)
	pipeline.runtime.promStatus = ws.PrometheusStatusPayload{Enabled: true, Available: true}
	pipeline.runTask(context.Background(), scheduler.PollTask{
		Device: device, VolatilityClass: domain.VolatilityClassPerformance,
		ExpectedInterval: 30 * time.Second,
	})
	if client.probeCalls != 0 {
		t.Fatalf("unused probe calls = %d, want 0", client.probeCalls)
	}
	if got := pipeline.runtime.hostnames[device.ID]; got != "prometheus-hostname" {
		t.Fatalf("hostname override = %q", got)
	}
}
