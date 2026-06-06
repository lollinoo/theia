package scalelab

// This file defines runner behavior for its package.

import (
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/domain"
	"github.com/lollinoo/theia/internal/topology"
)

func Run(profile Profile, scenario Scenario, fixture ReplayFixture) (LabReport, error) {
	replay, err := runReplay(profile, scenario, fixture)
	if err != nil {
		return LabReport{}, err
	}

	workload := WorkloadReport{
		PerformanceTasksPerMinute: tasksPerMinute(profile.DeviceCount, profile.PerformanceInterval, scenario.SNMPTimeoutRate),
		OperationalTasksPerMinute: tasksPerMinute(profile.DeviceCount, profile.OperationalInterval, scenario.SNMPTimeoutRate),
		StaticTasksPerMinute:      tasksPerMinute(profile.DeviceCount, profile.StaticInterval, 0),
		BurstAdds:                 scenario.BurstAdds,
		BurstUnresolvedNeighbors:  scenario.BurstUnresolvedNeighbors,
	}

	return LabReport{
		Profile:  profile,
		Scenario: scenario,
		Workload: workload,
		Replay:   replay,
	}, nil
}

func runReplay(profile Profile, scenario Scenario, fixture ReplayFixture) (ReplayReport, error) {
	linkRepo := newReplayLinkWriter()

	observations, unresolved, selfNeighbors, err := toObservations(fixture)
	if err != nil {
		return ReplayReport{}, err
	}

	passes := scenario.ReplayPasses
	if passes <= 0 {
		passes = profile.DefaultReplayPasses
		if passes <= 0 {
			passes = 1
		}
	}

	linkEvents := make(map[string]int)
	passDurations := make([]float64, 0, passes)
	for pass := 0; pass < passes; pass++ {
		start := time.Now()
		result, err := topology.ApplyObservations(observations, linkRepo)
		if err != nil {
			return ReplayReport{}, fmt.Errorf("replay pass %d: %w", pass+1, err)
		}
		for _, event := range result.Events {
			linkEvents[string(event.Result.Kind)]++
		}

		duration := time.Since(start) + scenario.DatabaseSlowdown
		passDurations = append(passDurations, duration.Seconds()*1000)
	}

	resolved := 0
	for _, observation := range observations {
		if observation.RemoteDeviceID != uuid.Nil {
			resolved++
		}
	}

	return ReplayReport{
		FixtureName:       fixture.Name,
		ObservationCount:  len(fixture.Observations),
		ResolvedCount:     resolved,
		UnresolvedCount:   unresolved,
		SelfNeighborCount: selfNeighbors,
		LinkEvents:        linkEvents,
		Latency:           summarizeLatencies(passDurations),
	}, nil
}

func toObservations(fixture ReplayFixture) ([]topology.Observation, int, int, error) {
	observations := make([]topology.Observation, 0, len(fixture.Observations))
	unresolved := 0
	selfNeighbors := 0
	now := time.Now().UTC()

	for _, item := range fixture.Observations {
		localDeviceID, err := uuid.Parse(item.LocalDeviceID)
		if err != nil {
			return nil, 0, 0, fmt.Errorf("parsing local_device_id %q: %w", item.LocalDeviceID, err)
		}

		remoteDeviceID := uuid.Nil
		if item.RemoteDeviceID != "" {
			remoteDeviceID, err = uuid.Parse(item.RemoteDeviceID)
			if err != nil {
				return nil, 0, 0, fmt.Errorf("parsing remote_device_id %q: %w", item.RemoteDeviceID, err)
			}
		} else {
			unresolved++
		}

		if item.SelfNeighbor {
			selfNeighbors++
		}

		observations = append(observations, topology.Observation{
			ID:              uuid.New(),
			LocalDeviceID:   localDeviceID,
			RemoteIdentity:  topology.NormalizeRemoteIdentity(item.RemoteIdentity),
			RemoteDeviceID:  remoteDeviceID,
			LocalPort:       item.LocalPort,
			RemotePort:      item.RemotePort,
			Protocol:        domain.DiscoveryProtocol(item.Protocol),
			SelfNeighbor:    item.SelfNeighbor,
			FirstObservedAt: now,
			LastObservedAt:  now,
			CreatedAt:       now,
			UpdatedAt:       now,
		})
	}

	return observations, unresolved, selfNeighbors, nil
}

type replayLinkWriter struct {
	links map[string]domain.Link
}

func newReplayLinkWriter() *replayLinkWriter {
	return &replayLinkWriter{links: make(map[string]domain.Link)}
}

func (w *replayLinkWriter) UpsertDetailed(link *domain.Link) (domain.LinkUpsertResult, error) {
	key := replayLinkKey(link)
	if existing, ok := w.links[key]; ok {
		link.ID = existing.ID
		return domain.LinkUpsertResult{Kind: domain.LinkUpsertKindNoop}, nil
	}

	if link.ID == uuid.Nil {
		link.ID = uuid.New()
	}
	w.links[key] = *link
	return domain.LinkUpsertResult{
		Created: true,
		Changed: true,
		Kind:    domain.LinkUpsertKindCreated,
	}, nil
}

func replayLinkKey(link *domain.Link) string {
	if link == nil {
		return ""
	}
	left := link.SourceDeviceID.String() + "|" + link.SourceIfName
	right := link.TargetDeviceID.String() + "|" + link.TargetIfName
	if right < left {
		left, right = right, left
	}
	return string(link.DiscoveryProtocol) + "|" +
		left + "|" +
		right
}

func summarizeLatencies(values []float64) LatencySummary {
	if len(values) == 0 {
		return LatencySummary{}
	}

	sorted := append([]float64(nil), values...)
	sort.Float64s(sorted)
	return LatencySummary{
		P50Ms: percentile(sorted, 0.50),
		P95Ms: percentile(sorted, 0.95),
		P99Ms: percentile(sorted, 0.99),
		MaxMs: sorted[len(sorted)-1],
	}
}

func percentile(sorted []float64, q float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	if q <= 0 {
		return sorted[0]
	}
	if q >= 1 {
		return sorted[len(sorted)-1]
	}

	position := q * float64(len(sorted)-1)
	lower := int(math.Floor(position))
	upper := int(math.Ceil(position))
	if lower == upper {
		return sorted[lower]
	}

	weight := position - float64(lower)
	return sorted[lower] + (sorted[upper]-sorted[lower])*weight
}

func tasksPerMinute(deviceCount int, interval time.Duration, timeoutRate float64) float64 {
	if deviceCount <= 0 || interval <= 0 {
		return 0
	}

	base := float64(deviceCount) / interval.Minutes()
	if timeoutRate <= 0 {
		return base
	}
	return base * (1 + timeoutRate)
}
