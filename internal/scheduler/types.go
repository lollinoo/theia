package scheduler

// This file defines types scheduling behavior, timing policy, and queue ownership.

import (
	"time"

	"github.com/google/uuid"

	"github.com/lollinoo/theia/internal/domain"
	"github.com/lollinoo/theia/internal/polling"
)

// TaskKey represents task key data used by the scheduler.
type TaskKey struct {
	DeviceID        uuid.UUID
	Kind            polling.TaskKind
	VolatilityClass domain.VolatilityClass
}

// PollTask represents poll task data used by the scheduler.
type PollTask struct {
	Key              TaskKey
	RunID            uint64
	Kind             polling.TaskKind
	Lane             polling.Lane
	Device           domain.Device
	PollClass        domain.PollClass
	VolatilityClass  domain.VolatilityClass
	ExpectedInterval time.Duration
	DueAt            time.Time
	DeadlineAt       time.Time
	QueueLag         time.Duration
	DeadlineMissed   bool
	SkippedWindows   int
}

// Completion represents completion data used by the scheduler.
type Completion struct {
	RunID      uint64
	Key        TaskKey
	FinishedAt time.Time
}

// NewTaskKey constructs task key state for the scheduler.
func NewTaskKey(deviceID uuid.UUID, volatility domain.VolatilityClass) TaskKey {
	return NewBackgroundTaskKey(deviceID, volatility)
}

// NewEssentialTaskKey constructs essential task key state for the scheduler.
func NewEssentialTaskKey(deviceID uuid.UUID) TaskKey {
	return TaskKey{
		DeviceID: deviceID,
		Kind:     polling.TaskKindEssential,
	}
}

// NewBackgroundTaskKey constructs background task key state for the scheduler.
func NewBackgroundTaskKey(deviceID uuid.UUID, volatility domain.VolatilityClass) TaskKey {
	return TaskKey{
		DeviceID:        deviceID,
		Kind:            polling.TaskKindBackground,
		VolatilityClass: volatility,
	}
}

// NewBootstrapTaskKey constructs bootstrap task key state for the scheduler.
func NewBootstrapTaskKey(deviceID uuid.UUID) TaskKey {
	return TaskKey{
		DeviceID: deviceID,
		Kind:     polling.TaskKindBootstrap,
	}
}

func EssentialInterval(device domain.Device) time.Duration {
	return EffectiveInterval(device, domain.VolatilityClassPerformance)
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
