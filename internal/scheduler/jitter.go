package scheduler

// This file defines jitter scheduling behavior, timing policy, and queue ownership.

import (
	"hash/fnv"
	"math/rand"
	"time"

	"github.com/google/uuid"
)

func initialOffset(deviceID uuid.UUID, interval time.Duration) time.Duration {
	if interval <= 0 {
		return 0
	}

	hasher := fnv.New64a()
	_, _ = hasher.Write(deviceID[:])

	return time.Duration(hasher.Sum64() % uint64(interval))
}

func initialOffsetForKey(key TaskKey, interval time.Duration) time.Duration {
	if interval <= 0 {
		return 0
	}

	h := fnv.New64a()
	_, _ = h.Write(key.DeviceID[:])
	_, _ = h.Write([]byte("|"))
	_, _ = h.Write([]byte(key.Kind))
	_, _ = h.Write([]byte("|"))
	_, _ = h.Write([]byte(key.VolatilityClass))

	return time.Duration(h.Sum64() % uint64(interval))
}

func jitteredNext(lastFire time.Time, interval time.Duration, rnd *rand.Rand) time.Time {
	if interval <= 0 {
		return lastFire
	}
	if rnd == nil {
		return lastFire.Add(interval)
	}

	jitterSpan := float64(interval) * 0.1
	sampledDelta := (rnd.Float64()*2 - 1) * jitterSpan
	return lastFire.Add(interval + time.Duration(sampledDelta))
}

func nextPhaseAfter(previousDue time.Time, interval time.Duration, after time.Time) time.Time {
	if interval <= 0 {
		return after
	}
	if previousDue.IsZero() {
		return after.Add(interval)
	}

	next := previousDue.Add(interval)
	if next.After(after) {
		return next
	}

	windows := after.Sub(previousDue)/interval + 1
	return previousDue.Add(windows * interval)
}
