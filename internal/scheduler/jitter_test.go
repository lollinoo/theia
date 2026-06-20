package scheduler

// This file exercises jitter behavior so refactors preserve the documented contract.

import (
	"math"
	"math/rand"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/lollinoo/theia/internal/domain"
	"github.com/lollinoo/theia/internal/polling"
)

func TestInitialOffset_DeterministicAndBounded(t *testing.T) {
	deviceID := uuid.MustParse("11111111-2222-3333-4444-555555555555")
	interval := 60 * time.Second

	first := initialOffset(deviceID, interval)
	second := initialOffset(deviceID, interval)

	if first != second {
		t.Fatalf("initialOffset() not deterministic: first=%v second=%v", first, second)
	}

	if first < 0 || first >= interval {
		t.Fatalf("initialOffset() = %v, want within [0, %v)", first, interval)
	}

	if got := initialOffset(deviceID, 0); got != 0 {
		t.Fatalf("initialOffset(interval=0) = %v, want 0", got)
	}

	if got := initialOffset(deviceID, -time.Second); got != 0 {
		t.Fatalf("initialOffset(interval<0) = %v, want 0", got)
	}
}

func TestInitialOffset_DistributionAcrossBuckets(t *testing.T) {
	interval := 60 * time.Second
	bucketWidth := interval / 8
	counts := make([]int, 8)

	for i := 0; i < 256; i++ {
		var deviceID uuid.UUID
		deviceID[15] = byte(i)

		offset := initialOffset(deviceID, interval)
		bucket := int(offset / bucketWidth)
		if bucket < 0 || bucket >= len(counts) {
			t.Fatalf("bucket index out of range: offset=%v bucket=%d", offset, bucket)
		}

		counts[bucket]++
	}

	for bucket, count := range counts {
		if count < 16 || count > 48 {
			t.Fatalf("bucket %d count = %d, want between 16 and 48; counts=%v", bucket, count, counts)
		}
	}
}

func TestInitialOffsetForKeySpreadsSameDeviceTaskKinds(t *testing.T) {
	deviceID := uuid.MustParse("11111111-2222-3333-4444-555555555555")
	interval := 30 * time.Second
	keys := []TaskKey{
		NewBackgroundTaskKey(deviceID, domain.VolatilityClassPerformance),
		NewBackgroundTaskKey(deviceID, domain.VolatilityClassOperational),
		NewBackgroundTaskKey(deviceID, domain.VolatilityClassStatic),
		{
			DeviceID:        deviceID,
			Kind:            polling.TaskKindEssential,
			VolatilityClass: domain.VolatilityClassPerformance,
		},
		{
			DeviceID:        deviceID,
			Kind:            polling.TaskKindBootstrap,
			VolatilityClass: domain.VolatilityClassStatic,
		},
	}
	offsets := make(map[time.Duration]struct{}, len(keys))

	for _, key := range keys {
		offset := initialOffsetForKey(key, interval)
		if offset < 0 || offset >= interval {
			t.Fatalf("initialOffsetForKey(%+v) = %v, want within [0, %v)", key, offset, interval)
		}
		offsets[offset] = struct{}{}
	}

	if len(offsets) < 3 {
		t.Fatalf("initialOffsetForKey() produced %d unique offsets, want at least 3", len(offsets))
	}
}

func TestInitialOffsetForKeyIsDeterministic(t *testing.T) {
	deviceID := uuid.MustParse("aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee")
	key := NewBackgroundTaskKey(deviceID, domain.VolatilityClassPerformance)
	interval := 30 * time.Second

	first := initialOffsetForKey(key, interval)
	second := initialOffsetForKey(key, interval)

	if first != second {
		t.Fatalf("initialOffsetForKey() not deterministic: first=%v second=%v", first, second)
	}
}

func TestJitteredNext_UsesSuppliedRandomSource(t *testing.T) {
	lastFire := time.Unix(1_700_000_000, 0)
	interval := 60 * time.Second
	seed := int64(42)

	got := jitteredNext(lastFire, interval, rand.New(rand.NewSource(seed)))

	expectedRnd := rand.New(rand.NewSource(seed))
	jitterSpan := float64(interval) * 0.1
	sampledDelta := (expectedRnd.Float64()*2 - 1) * jitterSpan
	want := lastFire.Add(interval + time.Duration(sampledDelta))

	if !got.Equal(want) {
		t.Fatalf("jitteredNext() = %v, want %v", got, want)
	}

	if got := jitteredNext(lastFire, interval, nil); !got.Equal(lastFire.Add(interval)) {
		t.Fatalf("jitteredNext(nil) = %v, want %v", got, lastFire.Add(interval))
	}
}

func TestJitteredNext_BoundedToTenPercent(t *testing.T) {
	lastFire := time.Unix(1_700_000_000, 0)
	interval := 60 * time.Second
	lowerBound := lastFire.Add(time.Duration(math.Round(float64(interval) * 0.9)))
	upperBound := lastFire.Add(time.Duration(math.Round(float64(interval) * 1.1)))
	rnd := rand.New(rand.NewSource(7))

	for i := 0; i < 512; i++ {
		got := jitteredNext(lastFire, interval, rnd)
		if got.Before(lowerBound) || got.After(upperBound) {
			t.Fatalf("jitteredNext() = %v, want within [%v, %v]", got, lowerBound, upperBound)
		}
	}

	if got := jitteredNext(lastFire, 0, rand.New(rand.NewSource(1))); !got.Equal(lastFire) {
		t.Fatalf("jitteredNext(interval=0) = %v, want %v", got, lastFire)
	}
}
