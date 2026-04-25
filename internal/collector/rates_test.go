package collector

import (
	"testing"
	"time"

	"github.com/lollinoo/theia/internal/domain"
)

func TestComputeCounterRates_FirstSampleWarmsBaseline(t *testing.T) {
	collectedAt := time.Date(2026, 4, 12, 14, 0, 0, 0, time.UTC)

	metrics, next := ComputeCounterRates([]InterfaceCounterSnapshot{
		{
			IfName:    "ether1",
			InOctets:  100,
			OutOctets: 200,
			SpeedBps:  1_000_000_000,
		},
	}, nil, collectedAt, time.Second)

	if len(metrics) != 0 {
		t.Fatalf("expected no metrics on first sample, got %d", len(metrics))
	}

	baseline, ok := next["ether1"]
	if !ok {
		t.Fatal("expected baseline for ether1")
	}
	if baseline.InOctets != 100 || baseline.OutOctets != 200 {
		t.Fatalf("unexpected baseline counters: %+v", baseline)
	}
	if !baseline.SampledAt.Equal(collectedAt) {
		t.Fatalf("unexpected baseline time: got %v want %v", baseline.SampledAt, collectedAt)
	}
	if baseline.NeedsWarmup {
		t.Fatal("expected first baseline to be ready without warmup flag")
	}
}

func TestComputeCounterRates_ResetDiscardsAndRequiresWarmup(t *testing.T) {
	collectedAt := time.Date(2026, 4, 12, 14, 0, 1, 0, time.UTC)
	previous := map[string]CounterBaseline{
		"ether1": {
			InOctets:  200,
			OutOctets: 300,
			SampledAt: collectedAt.Add(-time.Second),
		},
	}

	metrics, next := ComputeCounterRates([]InterfaceCounterSnapshot{
		{
			IfName:    "ether1",
			InOctets:  150,
			OutOctets: 350,
			SpeedBps:  1_000_000_000,
		},
	}, previous, collectedAt, time.Second)

	if len(metrics) != 0 {
		t.Fatalf("expected reset sample to be discarded, got %d metrics", len(metrics))
	}

	baseline := next["ether1"]
	if !baseline.NeedsWarmup {
		t.Fatal("expected reset to require warmup")
	}
	if baseline.InOctets != 150 || baseline.OutOctets != 350 {
		t.Fatalf("unexpected baseline after reset: %+v", baseline)
	}
}

func TestComputeCounterRates_DelayedSampleStillEmitsRate(t *testing.T) {
	collectedAt := time.Date(2026, 4, 12, 14, 2, 0, 0, time.UTC)
	previous := map[string]CounterBaseline{
		"ether1": {
			InOctets:  100,
			OutOctets: 200,
			SampledAt: collectedAt.Add(-2 * time.Minute),
		},
	}

	metrics, next := ComputeCounterRates([]InterfaceCounterSnapshot{
		{
			IfName:    "ether1",
			InOctets:  200,
			OutOctets: 300,
			SpeedBps:  1_000_000_000,
		},
	}, previous, collectedAt, time.Second)

	if len(metrics) != 1 {
		t.Fatalf("expected delayed sample to emit one metric, got %d", len(metrics))
	}
	if metrics[0].RxBps == nil || *metrics[0].RxBps != 6.666666666666667 {
		t.Fatalf("RxBps = %#v, want 6.666666666666667", metrics[0].RxBps)
	}
	if metrics[0].TxBps == nil || *metrics[0].TxBps != 6.666666666666667 {
		t.Fatalf("TxBps = %#v, want 6.666666666666667", metrics[0].TxBps)
	}
	if next["ether1"].NeedsWarmup {
		t.Fatal("expected delayed sample to keep baseline ready")
	}
}

func TestComputeCounterRates_SpeedBoundDiscardsAndRequiresWarmup(t *testing.T) {
	collectedAt := time.Date(2026, 4, 12, 14, 0, 1, 0, time.UTC)
	previous := map[string]CounterBaseline{
		"ether1": {
			InOctets:  0,
			OutOctets: 0,
			SampledAt: collectedAt.Add(-time.Second),
		},
	}

	metrics, next := ComputeCounterRates([]InterfaceCounterSnapshot{
		{
			IfName:    "ether1",
			InOctets:  162_500_000,
			OutOctets: 0,
			SpeedBps:  1_000_000_000,
		},
	}, previous, collectedAt, time.Second)

	if len(metrics) != 0 {
		t.Fatalf("expected overspeed sample to be discarded, got %d metrics", len(metrics))
	}

	baseline := next["ether1"]
	if !baseline.NeedsWarmup {
		t.Fatal("expected overspeed sample to require warmup")
	}
	if baseline.InOctets != 162_500_000 {
		t.Fatalf("unexpected overspeed baseline: %+v", baseline)
	}
}

func TestComputeCounterRates_HappyPath(t *testing.T) {
	collectedAt := time.Date(2026, 4, 12, 14, 0, 1, 0, time.UTC)
	previous := map[string]CounterBaseline{
		"ether1": {
			InOctets:  0,
			OutOctets: 0,
			SampledAt: collectedAt.Add(-time.Second),
		},
	}

	metrics, next := ComputeCounterRates([]InterfaceCounterSnapshot{
		{
			IfName:    "ether1",
			InOctets:  125_000_000,
			OutOctets: 125_000_000,
			SpeedBps:  1_000_000_000,
		},
	}, previous, collectedAt, time.Second)

	if len(metrics) != 1 {
		t.Fatalf("expected one link metric, got %d", len(metrics))
	}

	want := domain.LinkMetrics{
		IfName:      "ether1",
		CollectedAt: collectedAt,
	}
	got := metrics[0]
	if got.IfName != want.IfName {
		t.Fatalf("unexpected ifname: got %q want %q", got.IfName, want.IfName)
	}
	if !got.CollectedAt.Equal(want.CollectedAt) {
		t.Fatalf("unexpected collectedAt: got %v want %v", got.CollectedAt, want.CollectedAt)
	}
	if got.TxBps == nil || *got.TxBps != 1_000_000_000 {
		t.Fatalf("unexpected tx rate: %v", got.TxBps)
	}
	if got.RxBps == nil || *got.RxBps != 1_000_000_000 {
		t.Fatalf("unexpected rx rate: %v", got.RxBps)
	}

	baseline := next["ether1"]
	if baseline.NeedsWarmup {
		t.Fatal("expected happy path to keep warmup cleared")
	}
	if baseline.InOctets != 125_000_000 || baseline.OutOctets != 125_000_000 {
		t.Fatalf("unexpected next baseline: %+v", baseline)
	}
}
