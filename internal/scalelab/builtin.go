package scalelab

// This file defines builtin behavior for its package.

import (
	"fmt"
	"strings"
	"time"
)

func BuiltinProfile(name string) (Profile, error) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "100":
		return Profile{
			Name:                 "100",
			DeviceCount:          100,
			PerformanceInterval:  30 * time.Second,
			OperationalInterval:  60 * time.Second,
			StaticInterval:       5 * time.Minute,
			DefaultReplayPasses:  2,
			DefaultBurstAdds:     10,
			DefaultUnresolvedAdd: 20,
		}, nil
	case "300":
		return Profile{
			Name:                 "300",
			DeviceCount:          300,
			PerformanceInterval:  30 * time.Second,
			OperationalInterval:  60 * time.Second,
			StaticInterval:       5 * time.Minute,
			DefaultReplayPasses:  2,
			DefaultBurstAdds:     15,
			DefaultUnresolvedAdd: 60,
		}, nil
	case "500":
		return Profile{
			Name:                 "500",
			DeviceCount:          500,
			PerformanceInterval:  30 * time.Second,
			OperationalInterval:  60 * time.Second,
			StaticInterval:       5 * time.Minute,
			DefaultReplayPasses:  2,
			DefaultBurstAdds:     25,
			DefaultUnresolvedAdd: 100,
		}, nil
	case "1000":
		return Profile{
			Name:                 "1000",
			DeviceCount:          1000,
			PerformanceInterval:  30 * time.Second,
			OperationalInterval:  60 * time.Second,
			StaticInterval:       5 * time.Minute,
			DefaultReplayPasses:  2,
			DefaultBurstAdds:     50,
			DefaultUnresolvedAdd: 200,
		}, nil
	default:
		return Profile{}, fmt.Errorf("unknown profile %q", name)
	}
}

func BuiltinScenario(name string, profile Profile) (Scenario, error) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "", "baseline":
		return Scenario{
			Name:                     "baseline",
			Duration:                 30 * time.Minute,
			ReplayPasses:             profile.DefaultReplayPasses,
			BurstAdds:                0,
			BurstUnresolvedNeighbors: 0,
		}, nil
	case "db-slowdown":
		return Scenario{
			Name:                     "db-slowdown",
			Duration:                 30 * time.Minute,
			ReplayPasses:             profile.DefaultReplayPasses,
			DatabaseSlowdown:         75 * time.Millisecond,
			BurstAdds:                0,
			BurstUnresolvedNeighbors: 0,
		}, nil
	case "snmp-timeout-spike":
		return Scenario{
			Name:                     "snmp-timeout-spike",
			Duration:                 30 * time.Minute,
			ReplayPasses:             profile.DefaultReplayPasses,
			SNMPTimeoutRate:          0.15,
			BurstAdds:                0,
			BurstUnresolvedNeighbors: 0,
		}, nil
	case "burst-adds":
		return Scenario{
			Name:                     "burst-adds",
			Duration:                 20 * time.Minute,
			ReplayPasses:             profile.DefaultReplayPasses,
			BurstAdds:                profile.DefaultBurstAdds,
			BurstUnresolvedNeighbors: 0,
		}, nil
	case "burst-unresolved-neighbors":
		return Scenario{
			Name:                     "burst-unresolved-neighbors",
			Duration:                 20 * time.Minute,
			ReplayPasses:             profile.DefaultReplayPasses,
			BurstAdds:                0,
			BurstUnresolvedNeighbors: profile.DefaultUnresolvedAdd,
		}, nil
	case "soak-24h":
		return Scenario{
			Name:                     "soak-24h",
			Duration:                 24 * time.Hour,
			ReplayPasses:             4,
			DatabaseSlowdown:         25 * time.Millisecond,
			SNMPTimeoutRate:          0.05,
			BurstAdds:                profile.DefaultBurstAdds,
			BurstUnresolvedNeighbors: profile.DefaultUnresolvedAdd,
		}, nil
	default:
		return Scenario{}, fmt.Errorf("unknown scenario %q", name)
	}
}

func GenerateSyntheticFixture(profile Profile, scenario Scenario) ReplayFixture {
	totalDevices := profile.DeviceCount + scenario.BurstAdds
	observations := make([]ReplayObservation, 0, (totalDevices-1)*2+scenario.BurstUnresolvedNeighbors)

	for i := 0; i < totalDevices-1; i++ {
		local := deviceID(i)
		remote := deviceID(i + 1)
		localPort := fmt.Sprintf("ether%d", (i%24)+1)
		remotePort := fmt.Sprintf("ether%d", ((i+1)%24)+1)
		remoteIdentity := fmt.Sprintf("device-%04d", i+1)

		observations = append(observations,
			ReplayObservation{
				LocalDeviceID:  local,
				RemoteIdentity: remoteIdentity,
				RemoteDeviceID: remote,
				LocalPort:      localPort,
				RemotePort:     remotePort,
				Protocol:       "lldp",
			},
			ReplayObservation{
				LocalDeviceID:  remote,
				RemoteIdentity: fmt.Sprintf("device-%04d", i),
				RemoteDeviceID: local,
				LocalPort:      remotePort,
				RemotePort:     localPort,
				Protocol:       "lldp",
			},
		)
	}

	for i := 0; i < scenario.BurstUnresolvedNeighbors; i++ {
		observations = append(observations, ReplayObservation{
			LocalDeviceID:  deviceID(i % totalDevices),
			RemoteIdentity: fmt.Sprintf("unknown-neighbor-%04d", i),
			LocalPort:      fmt.Sprintf("ether%d", (i%24)+1),
			RemotePort:     "",
			Protocol:       "lldp",
		})
	}

	return ReplayFixture{
		Name:         fmt.Sprintf("synthetic-%s-%s", profile.Name, scenario.Name),
		Observations: observations,
	}
}

func deviceID(index int) string {
	return fmt.Sprintf("00000000-0000-0000-0000-%012x", index+1)
}
