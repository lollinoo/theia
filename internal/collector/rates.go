package collector

import (
	"math"
	"time"

	"github.com/lollinoo/theia/internal/domain"
)

// CounterBaseline stores the last usable raw counter sample for an interface.
// NeedsWarmup forces the next sample to become a fresh baseline without
// emitting user-visible throughput.
type CounterBaseline struct {
	InOctets    uint64
	OutOctets   uint64
	SampledAt   time.Time
	NeedsWarmup bool
}

// ComputeCounterRates converts raw counter snapshots into per-interface link
// throughput while carrying baseline state explicitly in the returned map.
func ComputeCounterRates(
	current []InterfaceCounterSnapshot,
	previous map[string]CounterBaseline,
	collectedAt time.Time,
	expectedInterval time.Duration,
) (metrics []domain.LinkMetrics, next map[string]CounterBaseline) {
	_ = expectedInterval
	next = cloneCounterBaselines(previous)
	metrics = make([]domain.LinkMetrics, 0, len(current))

	for _, counter := range current {
		baseline := CounterBaseline{
			InOctets:  counter.InOctets,
			OutOctets: counter.OutOctets,
			SampledAt: collectedAt,
		}

		prev, ok := previous[counter.IfName]
		if !ok {
			next[counter.IfName] = baseline
			continue
		}

		if prev.NeedsWarmup {
			next[counter.IfName] = baseline
			continue
		}

		if !collectedAt.After(prev.SampledAt) {
			next[counter.IfName] = warmupBaseline(baseline)
			continue
		}

		if counter.InOctets < prev.InOctets || counter.OutOctets < prev.OutOctets {
			next[counter.IfName] = warmupBaseline(baseline)
			continue
		}

		if counter.SpeedBps <= 0 {
			next[counter.IfName] = warmupBaseline(baseline)
			continue
		}

		dt := collectedAt.Sub(prev.SampledAt).Seconds()
		rxBps := float64(counter.InOctets-prev.InOctets) * 8.0 / dt
		txBps := float64(counter.OutOctets-prev.OutOctets) * 8.0 / dt
		if math.Max(txBps, rxBps) > float64(counter.SpeedBps)*1.1 {
			next[counter.IfName] = warmupBaseline(baseline)
			continue
		}

		tx := txBps
		rx := rxBps
		metrics = append(metrics, domain.LinkMetrics{
			IfName:      counter.IfName,
			TxBps:       &tx,
			RxBps:       &rx,
			CollectedAt: collectedAt,
		})
		next[counter.IfName] = baseline
	}

	return metrics, next
}

func cloneCounterBaselines(previous map[string]CounterBaseline) map[string]CounterBaseline {
	if len(previous) == 0 {
		return make(map[string]CounterBaseline)
	}

	next := make(map[string]CounterBaseline, len(previous))
	for ifName, baseline := range previous {
		next[ifName] = baseline
	}
	return next
}

func warmupBaseline(baseline CounterBaseline) CounterBaseline {
	baseline.NeedsWarmup = true
	return baseline
}
