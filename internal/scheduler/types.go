package scheduler

import (
	"time"

	"github.com/google/uuid"

	"github.com/lollinoo/theia/internal/domain"
)

type TaskKey struct {
	DeviceID        uuid.UUID
	VolatilityClass domain.VolatilityClass
}

type PollTask struct {
	Key              TaskKey
	RunID            uint64
	Device           domain.Device
	PollClass        domain.PollClass
	VolatilityClass  domain.VolatilityClass
	ExpectedInterval time.Duration
	DueAt            time.Time
}

type Completion struct {
	RunID      uint64
	Key        TaskKey
	FinishedAt time.Time
}

func NewTaskKey(deviceID uuid.UUID, volatility domain.VolatilityClass) TaskKey {
	return TaskKey{
		DeviceID:        deviceID,
		VolatilityClass: volatility,
	}
}

func EffectiveInterval(device domain.Device, volatility domain.VolatilityClass) time.Duration {
	switch volatility {
	case domain.VolatilityClassPerformance:
		if device.PollIntervalOverride != nil && *device.PollIntervalOverride > 0 {
			return time.Duration(*device.PollIntervalOverride) * time.Second
		}
		return device.PollClass.Interval()
	case domain.VolatilityClassOperational:
		return domain.OperationalClassInterval
	case domain.VolatilityClassStatic:
		return domain.StaticClassInterval
	default:
		return device.PollClass.Interval()
	}
}

func VolatilityPriority(volatility domain.VolatilityClass) int {
	switch volatility {
	case domain.VolatilityClassPerformance:
		return 0
	case domain.VolatilityClassOperational:
		return 1
	case domain.VolatilityClassStatic:
		return 2
	default:
		return 99
	}
}
