package scheduler

// This file exercises heap behavior so refactors preserve the documented contract.

import (
	"container/heap"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/lollinoo/theia/internal/domain"
)

func TestTaskHeap_OrdersByDueAtThenPriority(t *testing.T) {
	base := time.Unix(1_700_000_000, 0)
	sameDue := base.Add(time.Minute)

	items := buildTaskHeap(
		newHeapItem("00000000-0000-0000-0000-000000000005", domain.VolatilityClassStatic, base),
		newHeapItem("00000000-0000-0000-0000-000000000004", domain.VolatilityClassStatic, sameDue),
		newHeapItem("00000000-0000-0000-0000-000000000003", domain.VolatilityClassPerformance, sameDue),
		newHeapItem("00000000-0000-0000-0000-000000000002", domain.VolatilityClassOperational, sameDue),
		newHeapItem("00000000-0000-0000-0000-000000000001", domain.VolatilityClassOperational, sameDue),
	)

	want := []TaskKey{
		NewTaskKey(uuid.MustParse("00000000-0000-0000-0000-000000000005"), domain.VolatilityClassStatic),
		NewTaskKey(uuid.MustParse("00000000-0000-0000-0000-000000000003"), domain.VolatilityClassPerformance),
		NewTaskKey(uuid.MustParse("00000000-0000-0000-0000-000000000001"), domain.VolatilityClassOperational),
		NewTaskKey(uuid.MustParse("00000000-0000-0000-0000-000000000002"), domain.VolatilityClassOperational),
		NewTaskKey(uuid.MustParse("00000000-0000-0000-0000-000000000004"), domain.VolatilityClassStatic),
	}

	for i, wantKey := range want {
		got := heap.Pop(&items).(*heapItem)
		if got.task.Key != wantKey {
			t.Fatalf("pop %d key = %+v, want %+v", i, got.task.Key, wantKey)
		}
	}
}

func TestTaskHeap_FixAfterDueTimeChange(t *testing.T) {
	base := time.Unix(1_700_000_000, 0)
	first := newHeapItem("00000000-0000-0000-0000-000000000010", domain.VolatilityClassStatic, base.Add(2*time.Minute))
	second := newHeapItem("00000000-0000-0000-0000-000000000011", domain.VolatilityClassPerformance, base.Add(3*time.Minute))

	items := buildTaskHeap(first, second)

	second.dueAt = base.Add(30 * time.Second)
	heap.Fix(&items, second.index)

	got := heap.Pop(&items).(*heapItem)
	if got != second {
		t.Fatalf("heap.Pop() = %v, want updated item %v", got.task.Key, second.task.Key)
	}

	if second.index != -1 {
		t.Fatalf("popped item index = %d, want -1", second.index)
	}
}

func TestTaskHeap_RemoveDisabledItem(t *testing.T) {
	base := time.Unix(1_700_000_000, 0)
	first := newHeapItem("00000000-0000-0000-0000-000000000020", domain.VolatilityClassPerformance, base.Add(30*time.Second))
	disabled := newHeapItem("00000000-0000-0000-0000-000000000021", domain.VolatilityClassOperational, base.Add(time.Minute))
	last := newHeapItem("00000000-0000-0000-0000-000000000022", domain.VolatilityClassStatic, base.Add(2*time.Minute))

	items := buildTaskHeap(first, disabled, last)

	disabled.disabled = true
	removed := heap.Remove(&items, disabled.index).(*heapItem)
	if removed != disabled {
		t.Fatalf("heap.Remove() removed %v, want %v", removed.task.Key, disabled.task.Key)
	}
	if removed.index != -1 {
		t.Fatalf("removed item index = %d, want -1", removed.index)
	}

	gotFirst := heap.Pop(&items).(*heapItem)
	if gotFirst != first {
		t.Fatalf("first remaining item = %v, want %v", gotFirst.task.Key, first.task.Key)
	}

	gotLast := heap.Pop(&items).(*heapItem)
	if gotLast != last {
		t.Fatalf("second remaining item = %v, want %v", gotLast.task.Key, last.task.Key)
	}
}

func newHeapItem(deviceID string, volatility domain.VolatilityClass, dueAt time.Time) *heapItem {
	id := uuid.MustParse(deviceID)
	device := domain.Device{
		ID:        id,
		PollClass: domain.PollClassStandard,
	}
	interval := EffectiveInterval(device, volatility)

	return &heapItem{
		task: PollTask{
			Key:              NewTaskKey(id, volatility),
			Device:           device,
			PollClass:        device.PollClass,
			VolatilityClass:  volatility,
			ExpectedInterval: interval,
			DueAt:            dueAt,
		},
		dueAt:    dueAt,
		interval: interval,
		index:    -1,
	}
}

func buildTaskHeap(items ...*heapItem) taskHeap {
	queue := taskHeap(items)
	for i, item := range queue {
		item.index = i
	}
	heap.Init(&queue)
	return queue
}
