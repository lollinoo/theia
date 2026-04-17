package scalelab

import (
	"database/sql"
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/domain"
	sqliterepo "github.com/lollinoo/theia/internal/repository/sqlite"
	"github.com/lollinoo/theia/internal/topology"
	_ "github.com/mattn/go-sqlite3"
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
	db, err := sql.Open("sqlite3", ":memory:?_foreign_keys=on")
	if err != nil {
		return ReplayReport{}, fmt.Errorf("opening in-memory sqlite db: %w", err)
	}
	defer db.Close()

	if err := sqliterepo.RunMigrations(db); err != nil {
		return ReplayReport{}, fmt.Errorf("running migrations: %w", err)
	}

	deviceRepo := sqliterepo.NewDeviceRepo(db, []byte("0123456789abcdef0123456789abcdef"), nil)
	linkRepo := sqliterepo.NewLinkRepo(db, nil)

	if err := seedReplayDevices(deviceRepo, fixture); err != nil {
		return ReplayReport{}, err
	}

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

func seedReplayDevices(repo *sqliterepo.DeviceRepo, fixture ReplayFixture) error {
	seen := make(map[string]struct{})
	for _, observation := range fixture.Observations {
		if observation.LocalDeviceID != "" {
			seen[observation.LocalDeviceID] = struct{}{}
		}
		if observation.RemoteDeviceID != "" {
			seen[observation.RemoteDeviceID] = struct{}{}
		}
	}

	for rawID := range seen {
		id, err := uuid.Parse(rawID)
		if err != nil {
			return fmt.Errorf("parsing replay device id %q: %w", rawID, err)
		}
		device := &domain.Device{
			ID:       id,
			Hostname: rawID,
			IP:       fmt.Sprintf("10.%d.%d.%d", len(rawID)%250, len(rawID)%100, len(rawID)%200),
			SNMPCredentials: domain.SNMPCredentials{
				Version: domain.SNMPVersionV2c,
				V2c:     &domain.SNMPv2cCredentials{Community: "public"},
			},
			DeviceType: domain.DeviceTypeRouter,
			Status:     domain.DeviceStatusUp,
			Managed:    true,
			SysName:    rawID,
		}
		if err := repo.Create(device); err != nil {
			return fmt.Errorf("creating replay device %s: %w", rawID, err)
		}
	}

	return nil
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
