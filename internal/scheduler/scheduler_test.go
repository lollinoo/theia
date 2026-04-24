package scheduler

import (
	"container/heap"
	"context"
	"errors"
	"math/rand"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/lollinoo/theia/internal/domain"
	"github.com/lollinoo/theia/internal/polling"
)

func TestRefreshDevices_SeedsManagedDeviceAcrossAllThreeVolatilityClasses(t *testing.T) {
	device := domain.Device{
		ID:                   uuid.MustParse("10000000-0000-0000-0000-000000000001"),
		Hostname:             "core-router-1",
		Managed:              true,
		PollClass:            domain.PollClassCore,
		PollIntervalOverride: schedulerIntPtr(45),
	}
	source := &fakeDeviceSource{devices: []domain.Device{device}}
	scheduler := NewScheduler(source, nil)
	now := time.Unix(1_700_000_000, 0)

	if err := scheduler.refreshDevices(now); err != nil {
		t.Fatalf("refreshDevices() error = %v", err)
	}

	if got := len(scheduler.items); got != 4 {
		t.Fatalf("len(items) = %d, want 4", got)
	}
	if got := scheduler.heap.Len(); got != 4 {
		t.Fatalf("heap.Len() = %d, want 4", got)
	}

	essential := mustSchedulerItem(t, scheduler, NewEssentialTaskKey(device.ID))
	if essential.task.Kind != polling.TaskKindEssential {
		t.Fatalf("essential kind = %q, want essential", essential.task.Kind)
	}
	if essential.task.Lane != polling.LaneEssential {
		t.Fatalf("essential lane = %q, want essential", essential.task.Lane)
	}
	if essential.task.ExpectedInterval != 45*time.Second {
		t.Fatalf("essential expected interval = %v, want 45s", essential.task.ExpectedInterval)
	}

	tests := []struct {
		volatility domain.VolatilityClass
		interval   time.Duration
	}{
		{volatility: domain.VolatilityClassPerformance, interval: 45 * time.Second},
		{volatility: domain.VolatilityClassOperational, interval: domain.OperationalClassInterval},
		{volatility: domain.VolatilityClassStatic, interval: domain.StaticClassInterval},
	}

	for _, tc := range tests {
		key := NewTaskKey(device.ID, tc.volatility)
		item := mustSchedulerItem(t, scheduler, key)
		wantDueAt := now.Add(initialOffset(device.ID, tc.interval))

		if item.task.PollClass != device.PollClass {
			t.Fatalf("%s poll class = %q, want %q", tc.volatility, item.task.PollClass, device.PollClass)
		}
		if item.task.ExpectedInterval != tc.interval {
			t.Fatalf("%s expected interval = %v, want %v", tc.volatility, item.task.ExpectedInterval, tc.interval)
		}
		if item.interval != tc.interval {
			t.Fatalf("%s heap interval = %v, want %v", tc.volatility, item.interval, tc.interval)
		}
		if !item.dueAt.Equal(wantDueAt) {
			t.Fatalf("%s dueAt = %v, want %v", tc.volatility, item.dueAt, wantDueAt)
		}
		if !item.task.DueAt.Equal(wantDueAt) {
			t.Fatalf("%s task dueAt = %v, want %v", tc.volatility, item.task.DueAt, wantDueAt)
		}
		if item.task.Device.ID != device.ID || item.task.Device.Hostname != device.Hostname {
			t.Fatalf("%s device snapshot = %+v, want device %+v", tc.volatility, item.task.Device, device)
		}
	}
}

func TestRefreshDevices_SkipsUnmanagedDevices(t *testing.T) {
	managed := domain.Device{
		ID:        uuid.MustParse("20000000-0000-0000-0000-000000000001"),
		Hostname:  "managed-edge",
		Managed:   true,
		PollClass: domain.PollClassStandard,
	}
	unmanaged := domain.Device{
		ID:        uuid.MustParse("20000000-0000-0000-0000-000000000002"),
		Hostname:  "discovered-only",
		Managed:   false,
		PollClass: domain.PollClassCore,
	}
	source := &fakeDeviceSource{devices: []domain.Device{managed, unmanaged}}
	scheduler := NewScheduler(source, nil)

	if err := scheduler.refreshDevices(time.Unix(1_700_000_000, 0)); err != nil {
		t.Fatalf("refreshDevices() error = %v", err)
	}

	if got := len(scheduler.items); got != 4 {
		t.Fatalf("len(items) = %d, want only 4 managed tasks", got)
	}

	for _, volatility := range allVolatilityClasses() {
		if _, ok := scheduler.items[NewTaskKey(unmanaged.ID, volatility)]; ok {
			t.Fatalf("unmanaged key %v unexpectedly scheduled", NewTaskKey(unmanaged.ID, volatility))
		}
	}
	assertSchedulerKeyMissing(t, scheduler, NewEssentialTaskKey(unmanaged.ID))
}

func TestRefreshDevices_SkipsVirtualNoIPDevices(t *testing.T) {
	physical := domain.Device{
		ID:        uuid.MustParse("20000000-0000-0000-0000-000000000011"),
		Hostname:  "core-edge",
		Managed:   true,
		PollClass: domain.PollClassStandard,
		IP:        "192.0.2.10",
	}
	virtualNoIP := domain.Device{
		ID:         uuid.MustParse("20000000-0000-0000-0000-000000000012"),
		Hostname:   "internet-placeholder",
		Managed:    true,
		PollClass:  domain.PollClassCore,
		DeviceType: domain.DeviceTypeVirtual,
		IP:         "",
	}
	source := &fakeDeviceSource{devices: []domain.Device{physical, virtualNoIP}}
	scheduler := NewScheduler(source, nil)

	if err := scheduler.refreshDevices(time.Unix(1_700_000_000, 0)); err != nil {
		t.Fatalf("refreshDevices() error = %v", err)
	}

	if got := len(scheduler.items); got != 4 {
		t.Fatalf("len(items) = %d, want only 4 physical tasks", got)
	}

	for _, volatility := range allVolatilityClasses() {
		if _, ok := scheduler.items[NewTaskKey(virtualNoIP.ID, volatility)]; ok {
			t.Fatalf("virtual no-IP key %v unexpectedly scheduled", NewTaskKey(virtualNoIP.ID, volatility))
		}
	}
	assertSchedulerKeyMissing(t, scheduler, NewEssentialTaskKey(virtualNoIP.ID))
}

func TestRefreshDevices_SchedulesOperationalOnlyForVirtualIPDevices(t *testing.T) {
	virtualWithIP := domain.Device{
		ID:         uuid.MustParse("20000000-0000-0000-0000-000000000013"),
		Hostname:   "cloud-vpn",
		Managed:    true,
		PollClass:  domain.PollClassLow,
		DeviceType: domain.DeviceTypeVirtual,
		IP:         "192.0.2.30",
	}
	source := &fakeDeviceSource{devices: []domain.Device{virtualWithIP}}
	scheduler := NewScheduler(source, nil)
	now := time.Unix(1_700_000_000, 0)

	if err := scheduler.refreshDevices(now); err != nil {
		t.Fatalf("refreshDevices() error = %v", err)
	}

	if got := len(scheduler.items); got != 2 {
		t.Fatalf("len(items) = %d, want essential + operational task", got)
	}

	essentialKey := NewEssentialTaskKey(virtualWithIP.ID)
	essentialItem := mustSchedulerItem(t, scheduler, essentialKey)
	if essentialItem.task.Kind != polling.TaskKindEssential {
		t.Fatalf("essential kind = %q, want essential", essentialItem.task.Kind)
	}

	operationalKey := NewTaskKey(virtualWithIP.ID, domain.VolatilityClassOperational)
	item := mustSchedulerItem(t, scheduler, operationalKey)
	wantDueAt := now.Add(initialOffset(virtualWithIP.ID, domain.OperationalClassInterval))
	if item.task.ExpectedInterval != domain.OperationalClassInterval {
		t.Fatalf("operational expected interval = %v, want %v", item.task.ExpectedInterval, domain.OperationalClassInterval)
	}
	if !item.dueAt.Equal(wantDueAt) {
		t.Fatalf("operational dueAt = %v, want %v", item.dueAt, wantDueAt)
	}

	assertSchedulerKeyMissing(t, scheduler, NewTaskKey(virtualWithIP.ID, domain.VolatilityClassPerformance))
	assertSchedulerKeyMissing(t, scheduler, NewTaskKey(virtualWithIP.ID, domain.VolatilityClassStatic))
}

func TestRefreshDevices_RemovesMissingOrUnmanagedKeys(t *testing.T) {
	deviceA := domain.Device{
		ID:        uuid.MustParse("30000000-0000-0000-0000-000000000001"),
		Hostname:  "device-a",
		Managed:   true,
		PollClass: domain.PollClassStandard,
	}
	deviceB := domain.Device{
		ID:        uuid.MustParse("30000000-0000-0000-0000-000000000002"),
		Hostname:  "device-b",
		Managed:   true,
		PollClass: domain.PollClassCore,
	}
	deviceC := domain.Device{
		ID:        uuid.MustParse("30000000-0000-0000-0000-000000000003"),
		Hostname:  "device-c",
		Managed:   true,
		PollClass: domain.PollClassLow,
	}
	source := &fakeDeviceSource{devices: []domain.Device{deviceA, deviceB, deviceC}}
	scheduler := NewScheduler(source, nil)
	now := time.Unix(1_700_000_000, 0)

	if err := scheduler.refreshDevices(now); err != nil {
		t.Fatalf("initial refreshDevices() error = %v", err)
	}

	inFlightKey := NewTaskKey(deviceA.ID, domain.VolatilityClassPerformance)
	inFlightItem := mustSchedulerItem(t, scheduler, inFlightKey)
	heap.Remove(&scheduler.heap, inFlightItem.index)
	inFlightItem.inFlight = true

	source.devices = []domain.Device{
		{
			ID:        deviceA.ID,
			Hostname:  "device-a",
			Managed:   false,
			PollClass: domain.PollClassStandard,
		},
		{
			ID:        deviceB.ID,
			Hostname:  "device-b-new",
			Managed:   true,
			PollClass: domain.PollClassCore,
		},
	}

	if err := scheduler.refreshDevices(now.Add(5 * time.Minute)); err != nil {
		t.Fatalf("second refreshDevices() error = %v", err)
	}

	if got := len(scheduler.items); got != 5 {
		t.Fatalf("len(items) = %d, want 5 (4 active + 1 disabled in-flight)", got)
	}
	if got := scheduler.heap.Len(); got != 4 {
		t.Fatalf("heap.Len() = %d, want 4 active items", got)
	}

	disabledItem := mustSchedulerItem(t, scheduler, inFlightKey)
	if !disabledItem.disabled {
		t.Fatalf("in-flight item disabled = false, want true")
	}
	if disabledItem.index != -1 {
		t.Fatalf("disabled in-flight item index = %d, want -1", disabledItem.index)
	}

	assertSchedulerKeyMissing(t, scheduler, NewTaskKey(deviceA.ID, domain.VolatilityClassOperational))
	assertSchedulerKeyMissing(t, scheduler, NewTaskKey(deviceA.ID, domain.VolatilityClassStatic))
	assertSchedulerKeyMissing(t, scheduler, NewEssentialTaskKey(deviceA.ID))
	for _, volatility := range allVolatilityClasses() {
		assertSchedulerKeyMissing(t, scheduler, NewTaskKey(deviceC.ID, volatility))
	}
	assertSchedulerKeyMissing(t, scheduler, NewEssentialTaskKey(deviceC.ID))
}

func TestRefreshDevices_PreservesExistingDueTimeButUpdatesDeviceSnapshot(t *testing.T) {
	deviceID := uuid.MustParse("40000000-0000-0000-0000-000000000001")
	source := &fakeDeviceSource{
		devices: []domain.Device{
			{
				ID:        deviceID,
				Hostname:  "edge-old",
				IP:        "10.0.0.1",
				Managed:   true,
				PollClass: domain.PollClassStandard,
			},
		},
	}
	scheduler := NewScheduler(source, nil)
	now := time.Unix(1_700_000_000, 0)

	if err := scheduler.refreshDevices(now); err != nil {
		t.Fatalf("initial refreshDevices() error = %v", err)
	}

	key := NewTaskKey(deviceID, domain.VolatilityClassPerformance)
	original := mustSchedulerItem(t, scheduler, key)
	originalDueAt := original.dueAt
	original.disabled = true

	source.devices = []domain.Device{
		{
			ID:                   deviceID,
			Hostname:             "edge-new",
			IP:                   "10.0.0.9",
			Managed:              true,
			PollClass:            domain.PollClassCore,
			PollIntervalOverride: schedulerIntPtr(15),
		},
	}

	if err := scheduler.refreshDevices(now.Add(10 * time.Minute)); err != nil {
		t.Fatalf("second refreshDevices() error = %v", err)
	}

	if got := len(scheduler.items); got != 4 {
		t.Fatalf("len(items) = %d, want 4 without duplicate keys", got)
	}

	updated := mustSchedulerItem(t, scheduler, key)
	if !updated.dueAt.Equal(originalDueAt) {
		t.Fatalf("dueAt changed from %v to %v, want preserved", originalDueAt, updated.dueAt)
	}
	if !updated.task.DueAt.Equal(originalDueAt) {
		t.Fatalf("task dueAt = %v, want %v", updated.task.DueAt, originalDueAt)
	}
	if updated.disabled {
		t.Fatalf("disabled = true, want false when device returns")
	}
	if updated.task.Device.Hostname != "edge-new" || updated.task.Device.IP != "10.0.0.9" {
		t.Fatalf("device snapshot = %+v, want updated hostname/ip", updated.task.Device)
	}
	if updated.task.PollClass != domain.PollClassCore {
		t.Fatalf("poll class = %q, want %q", updated.task.PollClass, domain.PollClassCore)
	}
	if updated.interval != 15*time.Second {
		t.Fatalf("interval = %v, want 15s", updated.interval)
	}
	if updated.task.ExpectedInterval != 15*time.Second {
		t.Fatalf("expected interval = %v, want 15s", updated.task.ExpectedInterval)
	}
}

func TestSchedulerReduePerformanceTask_HeapItemBecomesImmediatelyDueWithUpdatedInterval(t *testing.T) {
	scheduler := NewScheduler(&fakeDeviceSource{}, nil)
	otherKey := NewTaskKey(uuid.MustParse("41000000-0000-0000-0000-000000000001"), domain.VolatilityClassPerformance)
	targetKey := NewTaskKey(uuid.MustParse("41000000-0000-0000-0000-000000000002"), domain.VolatilityClassPerformance)
	now := time.Unix(1_700_000_000, 0).UTC()
	changedAt := now.Add(5 * time.Second)

	other := &heapItem{
		task: PollTask{
			Key:              otherKey,
			Device:           domain.Device{ID: otherKey.DeviceID, Hostname: "other", Managed: true, PollClass: domain.PollClassCore},
			PollClass:        domain.PollClassCore,
			VolatilityClass:  domain.VolatilityClassPerformance,
			ExpectedInterval: 30 * time.Second,
			DueAt:            now.Add(20 * time.Second),
		},
		dueAt:    now.Add(20 * time.Second),
		interval: 30 * time.Second,
		index:    -1,
	}
	target := &heapItem{
		task: PollTask{
			Key:              targetKey,
			Device:           domain.Device{ID: targetKey.DeviceID, Hostname: "target-old", Managed: true, PollClass: domain.PollClassStandard},
			PollClass:        domain.PollClassStandard,
			VolatilityClass:  domain.VolatilityClassPerformance,
			ExpectedInterval: 60 * time.Second,
			DueAt:            now.Add(40 * time.Second),
		},
		dueAt:    now.Add(40 * time.Second),
		interval: 60 * time.Second,
		index:    -1,
	}

	scheduler.items[otherKey] = other
	scheduler.items[targetKey] = target
	heap.Push(&scheduler.heap, other)
	heap.Push(&scheduler.heap, target)

	updatedDevice := domain.Device{
		ID:                   targetKey.DeviceID,
		Hostname:             "target-new",
		Managed:              true,
		PollClass:            domain.PollClassCore,
		PollIntervalOverride: schedulerIntPtr(15),
	}

	scheduler.handleReduePerformanceTask(reduePerformanceTaskRequest{
		device:    updatedDevice,
		changedAt: changedAt,
	})

	if scheduler.heap[0] != target {
		t.Fatalf("heap root = %p, want target item %p after heap.Fix", scheduler.heap[0], target)
	}
	if !target.dueAt.Equal(changedAt) {
		t.Fatalf("dueAt = %v, want %v", target.dueAt, changedAt)
	}
	if !target.task.DueAt.Equal(changedAt) {
		t.Fatalf("task dueAt = %v, want %v", target.task.DueAt, changedAt)
	}
	if target.interval != 15*time.Second {
		t.Fatalf("interval = %v, want 15s", target.interval)
	}
	if target.task.ExpectedInterval != 15*time.Second {
		t.Fatalf("expected interval = %v, want 15s", target.task.ExpectedInterval)
	}
	if target.task.PollClass != domain.PollClassCore {
		t.Fatalf("poll class = %q, want %q", target.task.PollClass, domain.PollClassCore)
	}
	if target.task.Device.Hostname != "target-new" {
		t.Fatalf("device hostname = %q, want updated snapshot", target.task.Device.Hostname)
	}
	if !other.dueAt.Equal(now.Add(20 * time.Second)) {
		t.Fatalf("other dueAt = %v, want unchanged", other.dueAt)
	}
}

func TestSchedulerReduePerformanceTask_QueuedItemMovesToFront(t *testing.T) {
	scheduler := NewScheduler(&fakeDeviceSource{}, nil)
	firstKey := NewTaskKey(uuid.MustParse("42000000-0000-0000-0000-000000000001"), domain.VolatilityClassPerformance)
	targetKey := NewTaskKey(uuid.MustParse("42000000-0000-0000-0000-000000000002"), domain.VolatilityClassPerformance)
	now := time.Unix(1_700_000_000, 0).UTC()
	changedAt := now.Add(3 * time.Second)

	first := &heapItem{
		task: PollTask{
			Key:             firstKey,
			Device:          domain.Device{ID: firstKey.DeviceID, Hostname: "first", Managed: true, PollClass: domain.PollClassCore},
			PollClass:       domain.PollClassCore,
			VolatilityClass: domain.VolatilityClassPerformance,
			DueAt:           now.Add(30 * time.Second),
		},
		dueAt:    now.Add(30 * time.Second),
		interval: 30 * time.Second,
		queued:   true,
		index:    -1,
	}
	target := &heapItem{
		task: PollTask{
			Key:              targetKey,
			Device:           domain.Device{ID: targetKey.DeviceID, Hostname: "target-old", Managed: true, PollClass: domain.PollClassStandard},
			PollClass:        domain.PollClassStandard,
			VolatilityClass:  domain.VolatilityClassPerformance,
			ExpectedInterval: 60 * time.Second,
			DueAt:            now.Add(45 * time.Second),
		},
		dueAt:    now.Add(45 * time.Second),
		interval: 60 * time.Second,
		queued:   true,
		index:    -1,
	}

	scheduler.items[firstKey] = first
	scheduler.items[targetKey] = target
	scheduler.ready[VolatilityPriority(domain.VolatilityClassPerformance)] = []*heapItem{first, target}

	scheduler.handleReduePerformanceTask(reduePerformanceTaskRequest{
		device: domain.Device{
			ID:                   targetKey.DeviceID,
			Hostname:             "target-new",
			Managed:              true,
			PollClass:            domain.PollClassCore,
			PollIntervalOverride: schedulerIntPtr(12),
		},
		changedAt: changedAt,
	})

	queue := scheduler.ready[VolatilityPriority(domain.VolatilityClassPerformance)]
	if len(queue) != 2 {
		t.Fatalf("ready queue len = %d, want 2", len(queue))
	}
	if queue[0] != target {
		t.Fatalf("queue[0] = %p, want target item %p", queue[0], target)
	}
	if queue[1] != first {
		t.Fatalf("queue[1] = %p, want first item %p", queue[1], first)
	}
	if !target.queued {
		t.Fatalf("target queued = false, want true")
	}
	if !target.dueAt.Equal(changedAt) {
		t.Fatalf("dueAt = %v, want %v", target.dueAt, changedAt)
	}
	if target.interval != 12*time.Second {
		t.Fatalf("interval = %v, want 12s", target.interval)
	}
}

func TestSchedulerReduePerformanceTask_InFlightItemBecomesSingleImmediatePendingRerun(t *testing.T) {
	scheduler := NewScheduler(&fakeDeviceSource{}, nil)
	key := NewTaskKey(uuid.MustParse("43000000-0000-0000-0000-000000000001"), domain.VolatilityClassPerformance)
	now := time.Unix(1_700_000_000, 0).UTC()
	changedAt := now.Add(10 * time.Second)
	finishedAt := changedAt.Add(2 * time.Second)
	item := &heapItem{
		task: PollTask{
			Key:              key,
			Device:           domain.Device{ID: key.DeviceID, Hostname: "edge-old", Managed: true, PollClass: domain.PollClassStandard},
			PollClass:        domain.PollClassStandard,
			VolatilityClass:  domain.VolatilityClassPerformance,
			ExpectedInterval: 60 * time.Second,
			DueAt:            now,
			RunID:            3,
		},
		dueAt:        now,
		dispatchedAt: now.Add(-5 * time.Second),
		runID:        3,
		interval:     60 * time.Second,
		inFlight:     true,
		index:        -1,
	}

	scheduler.items[key] = item
	scheduler.inFlight = 1

	scheduler.handleReduePerformanceTask(reduePerformanceTaskRequest{
		device: domain.Device{
			ID:                   key.DeviceID,
			Hostname:             "edge-new",
			Managed:              true,
			PollClass:            domain.PollClassCore,
			PollIntervalOverride: schedulerIntPtr(15),
		},
		changedAt: changedAt,
	})

	if !item.pending {
		t.Fatalf("pending = false, want true")
	}
	if !item.inFlight {
		t.Fatalf("inFlight = false, want true until completion")
	}
	if !item.dueAt.Equal(changedAt) {
		t.Fatalf("dueAt = %v, want %v", item.dueAt, changedAt)
	}
	if item.interval != 15*time.Second {
		t.Fatalf("interval = %v, want 15s", item.interval)
	}
	if got := len(scheduler.ready[VolatilityPriority(domain.VolatilityClassPerformance)]); got != 0 {
		t.Fatalf("ready queue len = %d, want 0 before completion", got)
	}

	scheduler.handleCompletion(Completion{RunID: 3, Key: key, FinishedAt: finishedAt})

	if item.pending {
		t.Fatalf("pending = true, want false after completion")
	}
	if !item.queued {
		t.Fatalf("queued = false, want immediate rerun queued")
	}
	if item.inFlight {
		t.Fatalf("inFlight = true, want false after completion")
	}
	if scheduler.inFlight != 0 {
		t.Fatalf("inFlight count = %d, want 0", scheduler.inFlight)
	}
	if !item.dueAt.Equal(finishedAt) {
		t.Fatalf("dueAt = %v, want %v after immediate rerun enqueue", item.dueAt, finishedAt)
	}
	if item.task.ExpectedInterval != 15*time.Second {
		t.Fatalf("expected interval = %v, want 15s", item.task.ExpectedInterval)
	}
	if item.task.Device.Hostname != "edge-new" {
		t.Fatalf("device hostname = %q, want updated snapshot", item.task.Device.Hostname)
	}
	if got := len(scheduler.ready[VolatilityPriority(domain.VolatilityClassPerformance)]); got != 1 {
		t.Fatalf("ready queue len = %d, want 1 immediate rerun", got)
	}
}

func TestSchedulerReduePerformanceTask_MissingManagedDeviceCreatesImmediatePerformanceOnlyTask(t *testing.T) {
	scheduler := NewScheduler(&fakeDeviceSource{}, nil)
	device := domain.Device{
		ID:                   uuid.MustParse("44000000-0000-0000-0000-000000000001"),
		Hostname:             "new-managed",
		Managed:              true,
		PollClass:            domain.PollClassCore,
		PollIntervalOverride: schedulerIntPtr(20),
	}
	changedAt := time.Unix(1_700_000_123, 0).UTC()

	scheduler.handleReduePerformanceTask(reduePerformanceTaskRequest{
		device:    device,
		changedAt: changedAt,
	})

	if got := len(scheduler.items); got != 1 {
		t.Fatalf("len(items) = %d, want 1 performance-only item", got)
	}
	if got := scheduler.heap.Len(); got != 0 {
		t.Fatalf("heap.Len() = %d, want 0 for immediate ready task", got)
	}

	key := NewTaskKey(device.ID, domain.VolatilityClassPerformance)
	item := mustSchedulerItem(t, scheduler, key)
	if !item.queued {
		t.Fatalf("queued = false, want true")
	}
	if !item.dueAt.Equal(changedAt) {
		t.Fatalf("dueAt = %v, want %v", item.dueAt, changedAt)
	}
	if item.interval != 20*time.Second {
		t.Fatalf("interval = %v, want 20s", item.interval)
	}
	if got := len(scheduler.ready[VolatilityPriority(domain.VolatilityClassPerformance)]); got != 1 {
		t.Fatalf("ready queue len = %d, want 1", got)
	}
	assertSchedulerKeyMissing(t, scheduler, NewTaskKey(device.ID, domain.VolatilityClassOperational))
	assertSchedulerKeyMissing(t, scheduler, NewTaskKey(device.ID, domain.VolatilityClassStatic))
}

func TestSchedulerReduePerformanceTask_IgnoresUnmanagedDevice(t *testing.T) {
	scheduler := NewScheduler(&fakeDeviceSource{}, nil)

	scheduler.handleReduePerformanceTask(reduePerformanceTaskRequest{
		device: domain.Device{
			ID:        uuid.MustParse("45000000-0000-0000-0000-000000000001"),
			Hostname:  "unmanaged",
			Managed:   false,
			PollClass: domain.PollClassCore,
		},
		changedAt: time.Unix(1_700_000_200, 0).UTC(),
	})

	if got := len(scheduler.items); got != 0 {
		t.Fatalf("len(items) = %d, want 0", got)
	}
	if got := scheduler.heap.Len(); got != 0 {
		t.Fatalf("heap.Len() = %d, want 0", got)
	}
	for priority, queue := range scheduler.ready {
		if len(queue) != 0 {
			t.Fatalf("ready[%d] len = %d, want 0", priority, len(queue))
		}
	}
}

func TestSchedulerDispatchesPriorityOrder(t *testing.T) {
	scheduler := NewScheduler(&fakeDeviceSource{}, nil)
	now := time.Unix(1_700_000_000, 0).UTC()
	deviceID := uuid.MustParse("50000000-0000-0000-0000-000000000001")

	performance := &heapItem{
		task: PollTask{
			Key:              NewTaskKey(deviceID, domain.VolatilityClassPerformance),
			Device:           domain.Device{ID: deviceID, Hostname: "priority-device"},
			PollClass:        domain.PollClassCore,
			VolatilityClass:  domain.VolatilityClassPerformance,
			ExpectedInterval: 30 * time.Second,
		},
		index: -1,
	}
	operational := &heapItem{
		task: PollTask{
			Key:              NewTaskKey(deviceID, domain.VolatilityClassOperational),
			Device:           domain.Device{ID: deviceID, Hostname: "priority-device"},
			PollClass:        domain.PollClassCore,
			VolatilityClass:  domain.VolatilityClassOperational,
			ExpectedInterval: domain.OperationalClassInterval,
		},
		index: -1,
	}
	static := &heapItem{
		task: PollTask{
			Key:              NewTaskKey(deviceID, domain.VolatilityClassStatic),
			Device:           domain.Device{ID: deviceID, Hostname: "priority-device"},
			PollClass:        domain.PollClassCore,
			VolatilityClass:  domain.VolatilityClassStatic,
			ExpectedInterval: domain.StaticClassInterval,
		},
		index: -1,
	}

	scheduler.tasks = make(chan PollTask, 3)
	scheduler.ready[VolatilityPriority(domain.VolatilityClassStatic)] = append(scheduler.ready[VolatilityPriority(domain.VolatilityClassStatic)], static)
	scheduler.ready[VolatilityPriority(domain.VolatilityClassPerformance)] = append(scheduler.ready[VolatilityPriority(domain.VolatilityClassPerformance)], performance)
	scheduler.ready[VolatilityPriority(domain.VolatilityClassOperational)] = append(scheduler.ready[VolatilityPriority(domain.VolatilityClassOperational)], operational)

	scheduler.dispatchReady(context.Background(), now)

	if !performance.inFlight || !operational.inFlight || !static.inFlight {
		t.Fatalf("dispatchReady() did not mark all ready items in-flight")
	}
	if scheduler.inFlight != 3 {
		t.Fatalf("inFlight = %d, want 3", scheduler.inFlight)
	}
	if len(scheduler.ready[0]) != 0 || len(scheduler.ready[1]) != 0 || len(scheduler.ready[2]) != 0 {
		t.Fatalf("ready queues not drained after dispatch: %+v", scheduler.ready)
	}

	got := []domain.VolatilityClass{
		(<-scheduler.tasks).VolatilityClass,
		(<-scheduler.tasks).VolatilityClass,
		(<-scheduler.tasks).VolatilityClass,
	}
	want := []domain.VolatilityClass{
		domain.VolatilityClassPerformance,
		domain.VolatilityClassOperational,
		domain.VolatilityClassStatic,
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("dispatch order[%d] = %q, want %q (full order = %v)", i, got[i], want[i], got)
		}
	}
}

func TestSchedulerDoesNotQueueOverlappingEssentialTasks(t *testing.T) {
	deviceID := uuid.New()
	device := domain.Device{
		ID:                   deviceID,
		Hostname:             "edge-1",
		IP:                   "10.0.0.1",
		Managed:              true,
		PollClass:            domain.PollClassCore,
		PollIntervalOverride: schedulerIntPtr(10),
	}
	scheduler := NewScheduler(&fakeDeviceSource{devices: []domain.Device{device}}, nil)
	now := time.Date(2026, 4, 24, 10, 0, 0, 0, time.UTC)
	scheduler.now = func() time.Time { return now }
	if err := scheduler.refreshDevices(now); err != nil {
		t.Fatalf("refreshDevices: %v", err)
	}

	key := NewEssentialTaskKey(deviceID)
	item := scheduler.items[key]
	item.dueAt = now.Add(-20 * time.Second)
	heap.Fix(&scheduler.heap, item.index)
	scheduler.moveDueTasksToReady(now)
	scheduler.dispatchReady(context.Background(), now)

	if !item.inFlight {
		t.Fatal("expected essential item in flight")
	}
	now = now.Add(10 * time.Second)
	item.dueAt = now.Add(-10 * time.Second)
	scheduler.moveDueTasksToReady(now)

	if got := readyCountForKind(scheduler, polling.TaskKindEssential); got != 0 {
		t.Fatalf("ready essential count = %d, want 0 while in flight", got)
	}
	if !item.pending {
		t.Fatal("expected pending skipped window marker")
	}
	if item.skippedWindows != 1 {
		t.Fatalf("skippedWindows = %d, want 1", item.skippedWindows)
	}
}

func TestSchedulerDispatchReady_RespectsPerClassBudgets(t *testing.T) {
	scheduler := NewScheduler(
		&fakeDeviceSource{},
		fakeSettingsRepo{values: map[string]string{
			domain.SettingSNMPWorkerPoolPerformance: "1",
			domain.SettingSNMPWorkerPoolOperational: "1",
			domain.SettingSNMPWorkerPoolStatic:      "1",
		}},
	)
	now := time.Unix(1_700_000_000, 0).UTC()
	deviceA := uuid.MustParse("51000000-0000-0000-0000-000000000001")
	deviceB := uuid.MustParse("51000000-0000-0000-0000-000000000002")
	deviceC := uuid.MustParse("51000000-0000-0000-0000-000000000003")
	deviceD := uuid.MustParse("51000000-0000-0000-0000-000000000004")

	perfOne := &heapItem{task: PollTask{Key: NewTaskKey(deviceA, domain.VolatilityClassPerformance), Device: domain.Device{ID: deviceA}, VolatilityClass: domain.VolatilityClassPerformance}, index: -1}
	perfTwo := &heapItem{task: PollTask{Key: NewTaskKey(deviceB, domain.VolatilityClassPerformance), Device: domain.Device{ID: deviceB}, VolatilityClass: domain.VolatilityClassPerformance}, index: -1}
	operational := &heapItem{task: PollTask{Key: NewTaskKey(deviceC, domain.VolatilityClassOperational), Device: domain.Device{ID: deviceC}, VolatilityClass: domain.VolatilityClassOperational}, index: -1}
	static := &heapItem{task: PollTask{Key: NewTaskKey(deviceD, domain.VolatilityClassStatic), Device: domain.Device{ID: deviceD}, VolatilityClass: domain.VolatilityClassStatic}, index: -1}

	scheduler.tasks = make(chan PollTask, 4)
	scheduler.ready[VolatilityPriority(domain.VolatilityClassPerformance)] = []*heapItem{perfOne, perfTwo}
	scheduler.ready[VolatilityPriority(domain.VolatilityClassOperational)] = []*heapItem{operational}
	scheduler.ready[VolatilityPriority(domain.VolatilityClassStatic)] = []*heapItem{static}

	scheduler.dispatchReady(context.Background(), now)

	if scheduler.inFlight != 3 {
		t.Fatalf("inFlight = %d, want 3", scheduler.inFlight)
	}
	if !perfOne.inFlight || !operational.inFlight || !static.inFlight {
		t.Fatalf("expected one task per class in flight")
	}
	if perfTwo.inFlight {
		t.Fatalf("second performance task should remain queued once performance budget is exhausted")
	}
	if got := len(scheduler.ready[VolatilityPriority(domain.VolatilityClassPerformance)]); got != 1 {
		t.Fatalf("performance ready queue len = %d, want 1", got)
	}
}

func TestSchedulerDispatchReady_AllowsLowerPriorityWhenHigherPriorityAtClassLimit(t *testing.T) {
	scheduler := NewScheduler(
		&fakeDeviceSource{},
		fakeSettingsRepo{values: map[string]string{
			domain.SettingSNMPWorkerPoolPerformance: "1",
			domain.SettingSNMPWorkerPoolOperational: "1",
			domain.SettingSNMPWorkerPoolStatic:      "1",
		}},
	)
	now := time.Unix(1_700_000_000, 0).UTC()
	perfKey := NewTaskKey(uuid.MustParse("52000000-0000-0000-0000-000000000001"), domain.VolatilityClassPerformance)
	operationalKey := NewTaskKey(uuid.MustParse("52000000-0000-0000-0000-000000000002"), domain.VolatilityClassOperational)
	perf := &heapItem{task: PollTask{Key: perfKey, VolatilityClass: domain.VolatilityClassPerformance}, queued: true, index: -1}
	operational := &heapItem{task: PollTask{Key: operationalKey, VolatilityClass: domain.VolatilityClassOperational}, queued: true, index: -1}

	scheduler.tasks = make(chan PollTask, 2)
	scheduler.inFlight = 1
	scheduler.inFlightByClass[domain.VolatilityClassPerformance] = 1
	scheduler.ready[VolatilityPriority(domain.VolatilityClassPerformance)] = []*heapItem{perf}
	scheduler.ready[VolatilityPriority(domain.VolatilityClassOperational)] = []*heapItem{operational}

	scheduler.dispatchReady(context.Background(), now)

	if !operational.inFlight {
		t.Fatal("operational task should dispatch when performance class is already at its ceiling")
	}
	if perf.inFlight {
		t.Fatal("performance task should remain queued when class ceiling is reached")
	}
}

func TestSchedulerMaxInFlight_DefaultAndConfigured(t *testing.T) {
	tests := []struct {
		name     string
		repo     domain.SettingsRepository
		expected int
	}{
		{
			name:     "nil repo falls back",
			expected: 5,
		},
		{
			name:     "missing value falls back",
			repo:     fakeSettingsRepo{err: errors.New("missing")},
			expected: 5,
		},
		{
			name:     "invalid value falls back",
			repo:     fakeSettingsRepo{values: map[string]string{domain.SettingSNMPWorkerPoolSize: "not-a-number"}},
			expected: 5,
		},
		{
			name:     "non-positive falls back",
			repo:     fakeSettingsRepo{values: map[string]string{domain.SettingSNMPWorkerPoolSize: "0"}},
			expected: 5,
		},
		{
			name:     "configured value",
			repo:     fakeSettingsRepo{values: map[string]string{domain.SettingSNMPWorkerPoolSize: "7"}},
			expected: 7,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			scheduler := NewScheduler(&fakeDeviceSource{}, tc.repo)
			if got := scheduler.maxInFlight(); got != tc.expected {
				t.Fatalf("maxInFlight() = %d, want %d", got, tc.expected)
			}
		})
	}
}

func TestSchedulerMaxInFlight_UsesVolatilityBudgetSum(t *testing.T) {
	scheduler := NewScheduler(
		&fakeDeviceSource{},
		fakeSettingsRepo{values: map[string]string{
			domain.SettingSNMPWorkerPoolSize:        "9",
			domain.SettingSNMPWorkerPoolPerformance: "4",
			domain.SettingSNMPWorkerPoolOperational: "2",
			domain.SettingSNMPWorkerPoolStatic:      "1",
		}},
	)

	if got := scheduler.maxInFlight(); got != 7 {
		t.Fatalf("maxInFlight() = %d, want 7", got)
	}
}

func TestSchedulerMaxInFlight_CappedByInternalBuffers(t *testing.T) {
	scheduler := NewScheduler(
		&fakeDeviceSource{},
		fakeSettingsRepo{values: map[string]string{domain.SettingSNMPWorkerPoolSize: "9"}},
	)
	scheduler.tasks = make(chan PollTask, 4)
	scheduler.completions = make(chan Completion, 3)

	if got := scheduler.maxInFlight(); got != 3 {
		t.Fatalf("maxInFlight() = %d, want 3", got)
	}
}

func TestSchedulerStartStop_ReusableWithoutLeak(t *testing.T) {
	source := &fakeDeviceSource{
		devices: []domain.Device{
			{
				ID:        uuid.MustParse("50000000-0000-0000-0000-000000000010"),
				Hostname:  "reusable-device",
				Managed:   true,
				PollClass: domain.PollClassStandard,
			},
		},
	}
	scheduler := NewScheduler(source, fakeSettingsRepo{values: map[string]string{domain.SettingSNMPWorkerPoolSize: "1"}})
	scheduler.refreshInterval = 10 * time.Millisecond
	scheduler.tasks = make(chan PollTask, 16)
	scheduler.completions = make(chan Completion, 16)
	scheduler.now = func() time.Time { return time.Unix(1_700_000_000, 0).UTC() }

	parent, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := scheduler.Start(parent); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	if !scheduler.running.Load() {
		t.Fatalf("running = false after Start(), want true")
	}
	firstDone := scheduler.done
	if firstDone == nil {
		t.Fatalf("done channel not initialized on first Start()")
	}

	scheduler.Stop()

	if scheduler.running.Load() {
		t.Fatalf("running = true after Stop(), want false")
	}
	if scheduler.cancel != nil {
		t.Fatalf("cancel not cleared by Stop()")
	}
	if scheduler.done == nil {
		t.Fatalf("done channel not recreated by Stop()")
	}
	if scheduler.done == firstDone {
		t.Fatalf("Stop() did not recreate done channel")
	}

	secondDone := scheduler.done
	if err := scheduler.Start(parent); err != nil {
		t.Fatalf("restart Start() error = %v", err)
	}

	if scheduler.done != secondDone {
		t.Fatalf("Start() replaced the recreated done channel unexpectedly")
	}
	if !scheduler.running.Load() {
		t.Fatalf("running = false after restart, want true")
	}

	scheduler.Stop()
	scheduler.Stop()
}

func TestSchedulerStartReturnsErrAlreadyStarted(t *testing.T) {
	scheduler := NewScheduler(&fakeDeviceSource{}, nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := scheduler.Start(ctx); err != nil {
		t.Fatalf("first Start() error = %v", err)
	}
	defer scheduler.Stop()

	if err := scheduler.Start(ctx); !errors.Is(err, ErrAlreadyStarted) {
		t.Fatalf("second Start() error = %v, want ErrAlreadyStarted", err)
	}
}

func TestSchedulerCoalescesInFlightDueEventsToSinglePendingRerun(t *testing.T) {
	scheduler := NewScheduler(&fakeDeviceSource{}, nil)
	now := time.Unix(1_700_000_000, 0).UTC()
	item := &heapItem{
		task: PollTask{
			Key:              NewTaskKey(uuid.MustParse("60000000-0000-0000-0000-000000000001"), domain.VolatilityClassPerformance),
			VolatilityClass:  domain.VolatilityClassPerformance,
			ExpectedInterval: 30 * time.Second,
		},
		dueAt:    now.Add(-1 * time.Second),
		interval: 30 * time.Second,
		inFlight: true,
		index:    -1,
	}

	scheduler.items[item.task.Key] = item
	heap.Push(&scheduler.heap, item)

	scheduler.moveDueTasksToReady(now)
	scheduler.moveDueTasksToReady(now.Add(2 * time.Second))

	if !item.pending {
		t.Fatalf("pending = false, want true")
	}
	if item.queued {
		t.Fatalf("queued = true, want false")
	}
	if got := scheduler.heap.Len(); got != 0 {
		t.Fatalf("heap.Len() = %d, want 0", got)
	}
	if got := len(scheduler.ready[VolatilityPriority(domain.VolatilityClassPerformance)]); got != 0 {
		t.Fatalf("ready queue length = %d, want 0", got)
	}
}

func TestSchedulerComplete_RequeuesImmediatePendingRerun(t *testing.T) {
	scheduler := NewScheduler(&fakeDeviceSource{}, nil)
	finishedAt := time.Unix(1_700_000_030, 0).UTC()
	item := &heapItem{
		task: PollTask{
			Key:             NewTaskKey(uuid.MustParse("60000000-0000-0000-0000-000000000002"), domain.VolatilityClassOperational),
			VolatilityClass: domain.VolatilityClassOperational,
		},
		dueAt:        finishedAt.Add(-30 * time.Second),
		dispatchedAt: finishedAt.Add(-35 * time.Second),
		interval:     60 * time.Second,
		inFlight:     true,
		pending:      true,
		index:        -1,
	}

	scheduler.items[item.task.Key] = item
	scheduler.inFlight = 1

	scheduler.handleCompletion(Completion{Key: item.task.Key, FinishedAt: finishedAt})

	if scheduler.inFlight != 0 {
		t.Fatalf("inFlight = %d, want 0", scheduler.inFlight)
	}
	if item.pending {
		t.Fatalf("pending = true, want false")
	}
	if !item.queued {
		t.Fatalf("queued = false, want true")
	}
	if item.inFlight {
		t.Fatalf("inFlight flag = true, want false")
	}
	if !item.dueAt.Equal(finishedAt) {
		t.Fatalf("dueAt = %v, want %v", item.dueAt, finishedAt)
	}
	if !item.task.DueAt.Equal(finishedAt) {
		t.Fatalf("task dueAt = %v, want %v", item.task.DueAt, finishedAt)
	}
	if got := scheduler.heap.Len(); got != 0 {
		t.Fatalf("heap.Len() = %d, want 0", got)
	}
	if got := len(scheduler.ready[VolatilityPriority(domain.VolatilityClassOperational)]); got != 1 {
		t.Fatalf("ready queue length = %d, want 1", got)
	}
}

func TestSchedulerComplete_ReinsertsFromFinishedAt(t *testing.T) {
	scheduler := NewScheduler(&fakeDeviceSource{}, nil)
	scheduler.rnd = rand.New(rand.NewSource(7))

	previousDue := time.Unix(1_700_000_000, 0).UTC()
	finishedAt := previousDue.Add(5 * time.Minute)
	item := &heapItem{
		task: PollTask{
			Key:             NewTaskKey(uuid.MustParse("60000000-0000-0000-0000-000000000003"), domain.VolatilityClassStatic),
			VolatilityClass: domain.VolatilityClassStatic,
		},
		dueAt:        previousDue,
		dispatchedAt: previousDue.Add(-10 * time.Second),
		interval:     5 * time.Minute,
		inFlight:     true,
		index:        -1,
	}

	scheduler.items[item.task.Key] = item
	scheduler.inFlight = 1

	want := jitteredNext(finishedAt, item.interval, rand.New(rand.NewSource(7)))
	notWant := jitteredNext(previousDue, item.interval, rand.New(rand.NewSource(7)))

	scheduler.handleCompletion(Completion{Key: item.task.Key, FinishedAt: finishedAt})

	if scheduler.inFlight != 0 {
		t.Fatalf("inFlight = %d, want 0", scheduler.inFlight)
	}
	if !item.dueAt.Equal(want) {
		t.Fatalf("dueAt = %v, want %v", item.dueAt, want)
	}
	if !item.task.DueAt.Equal(want) {
		t.Fatalf("task dueAt = %v, want %v", item.task.DueAt, want)
	}
	if item.dueAt.Equal(notWant) {
		t.Fatalf("dueAt = %v, matched previous-due reinsertion %v", item.dueAt, notWant)
	}
	if got := scheduler.heap.Len(); got != 1 {
		t.Fatalf("heap.Len() = %d, want 1", got)
	}
	if item.index < 0 {
		t.Fatalf("item index = %d, want heap index >= 0", item.index)
	}
}

func TestSchedulerComplete_DropsDisabledItem(t *testing.T) {
	scheduler := NewScheduler(&fakeDeviceSource{}, nil)
	key := NewTaskKey(uuid.MustParse("60000000-0000-0000-0000-000000000004"), domain.VolatilityClassPerformance)
	item := &heapItem{
		task:         PollTask{Key: key, VolatilityClass: domain.VolatilityClassPerformance},
		dispatchedAt: time.Unix(1_700_000_030, 0).UTC(),
		interval:     30 * time.Second,
		inFlight:     true,
		disabled:     true,
		index:        -1,
	}

	scheduler.items[key] = item
	scheduler.inFlight = 1

	scheduler.handleCompletion(Completion{Key: key, FinishedAt: time.Unix(1_700_000_060, 0).UTC()})

	if scheduler.inFlight != 0 {
		t.Fatalf("inFlight = %d, want 0", scheduler.inFlight)
	}
	if _, ok := scheduler.items[key]; ok {
		t.Fatalf("disabled key %+v still present after completion", key)
	}
	if got := scheduler.heap.Len(); got != 0 {
		t.Fatalf("heap.Len() = %d, want 0", got)
	}
}

func TestSchedulerComplete_CoalescesElapsedIntervalsToSingleImmediateRerun(t *testing.T) {
	scheduler := NewScheduler(&fakeDeviceSource{}, nil)
	dueAt := time.Unix(1_700_000_000, 0).UTC()
	finishedAt := dueAt.Add(90 * time.Second)
	item := &heapItem{
		task: PollTask{
			Key:              NewTaskKey(uuid.MustParse("60000000-0000-0000-0000-000000000005"), domain.VolatilityClassPerformance),
			VolatilityClass:  domain.VolatilityClassPerformance,
			ExpectedInterval: 30 * time.Second,
			DueAt:            dueAt,
		},
		dueAt:        dueAt,
		dispatchedAt: dueAt.Add(30 * time.Second),
		interval:     30 * time.Second,
		inFlight:     true,
		index:        -1,
	}

	scheduler.items[item.task.Key] = item
	scheduler.inFlight = 1

	scheduler.handleCompletion(Completion{Key: item.task.Key, FinishedAt: finishedAt})

	if !item.queued {
		t.Fatalf("queued = false, want true for an overlapped run")
	}
	if item.inFlight {
		t.Fatalf("inFlight flag = true, want false after completion")
	}
	if scheduler.inFlight != 0 {
		t.Fatalf("inFlight = %d, want 0", scheduler.inFlight)
	}
	if got := scheduler.heap.Len(); got != 0 {
		t.Fatalf("heap.Len() = %d, want 0 immediate-rerun heap entries", got)
	}
	if got := len(scheduler.ready[VolatilityPriority(domain.VolatilityClassPerformance)]); got != 1 {
		t.Fatalf("ready queue length = %d, want 1", got)
	}
	if !item.dueAt.Equal(finishedAt) {
		t.Fatalf("dueAt = %v, want %v", item.dueAt, finishedAt)
	}
}

func TestSchedulerHandleCompletion_IgnoresDuplicateCompletionForNonInFlightTask(t *testing.T) {
	scheduler := NewScheduler(&fakeDeviceSource{}, nil)
	key := NewTaskKey(uuid.MustParse("60000000-0000-0000-0000-000000000006"), domain.VolatilityClassPerformance)
	dueAt := time.Unix(1_700_000_000, 0).UTC()
	item := &heapItem{
		task: PollTask{
			Key:             key,
			VolatilityClass: domain.VolatilityClassPerformance,
			DueAt:           dueAt,
		},
		dueAt:    dueAt,
		interval: 30 * time.Second,
		index:    -1,
	}

	scheduler.items[key] = item
	heap.Push(&scheduler.heap, item)

	scheduler.handleCompletion(Completion{
		Key:        key,
		FinishedAt: dueAt.Add(15 * time.Second),
	})

	if got := scheduler.heap.Len(); got != 1 {
		t.Fatalf("heap.Len() = %d, want 1 without duplicate requeue", got)
	}
	if !item.dueAt.Equal(dueAt) {
		t.Fatalf("dueAt = %v, want %v unchanged", item.dueAt, dueAt)
	}
	if item.inFlight {
		t.Fatalf("inFlight = true, want false")
	}
}

func TestSchedulerHandleCompletion_IgnoresStaleRunID(t *testing.T) {
	scheduler := NewScheduler(&fakeDeviceSource{}, nil)
	key := NewTaskKey(uuid.MustParse("60000000-0000-0000-0000-000000000008"), domain.VolatilityClassStatic)
	item := &heapItem{
		task: PollTask{
			Key:             key,
			RunID:           2,
			VolatilityClass: domain.VolatilityClassStatic,
		},
		dueAt:        time.Unix(1_700_000_000, 0).UTC(),
		dispatchedAt: time.Unix(1_700_000_030, 0).UTC(),
		runID:        2,
		interval:     5 * time.Minute,
		inFlight:     true,
		index:        -1,
	}

	scheduler.items[key] = item
	scheduler.inFlight = 1

	scheduler.handleCompletion(Completion{
		RunID:      1,
		Key:        key,
		FinishedAt: time.Unix(1_700_000_060, 0).UTC(),
	})

	if scheduler.inFlight != 1 {
		t.Fatalf("inFlight = %d, want 1", scheduler.inFlight)
	}
	if !item.inFlight {
		t.Fatalf("inFlight = false, want true")
	}
	if item.runID != 2 {
		t.Fatalf("runID = %d, want 2", item.runID)
	}
}

func TestSchedulerComplete_ReturnsImmediatelyWhenStopped(t *testing.T) {
	scheduler := NewScheduler(&fakeDeviceSource{}, nil)

	for i := 0; i < cap(scheduler.completions); i++ {
		scheduler.completions <- Completion{}
	}

	done := make(chan struct{})
	go func() {
		scheduler.Complete(Completion{Key: NewTaskKey(uuid.Nil, domain.VolatilityClassPerformance)})
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Complete() blocked while scheduler was stopped")
	}
}

func TestSchedulerResetRuntimeState_ClearsVolatileQueues(t *testing.T) {
	scheduler := NewScheduler(&fakeDeviceSource{}, nil)
	key := NewTaskKey(uuid.MustParse("60000000-0000-0000-0000-000000000007"), domain.VolatilityClassOperational)
	item := &heapItem{
		task: PollTask{
			Key:             key,
			VolatilityClass: domain.VolatilityClassOperational,
		},
		dueAt:    time.Unix(1_700_000_000, 0).UTC(),
		interval: time.Minute,
		queued:   true,
		index:    -1,
	}

	scheduler.items[key] = item
	heap.Push(&scheduler.heap, item)
	scheduler.ready[VolatilityPriority(domain.VolatilityClassOperational)] = append(
		scheduler.ready[VolatilityPriority(domain.VolatilityClassOperational)],
		item,
	)
	scheduler.inFlight = 2
	scheduler.tasks <- PollTask{Key: key, RunID: 1}
	scheduler.completions <- Completion{Key: key}

	scheduler.resetRuntimeState()

	if got := len(scheduler.items); got != 0 {
		t.Fatalf("len(items) = %d, want 0", got)
	}
	if got := scheduler.heap.Len(); got != 0 {
		t.Fatalf("heap.Len() = %d, want 0", got)
	}
	if scheduler.inFlight != 0 {
		t.Fatalf("inFlight = %d, want 0", scheduler.inFlight)
	}
	if got := len(scheduler.tasks); got != 0 {
		t.Fatalf("len(tasks) = %d, want 0", got)
	}
	for priority, queue := range scheduler.ready {
		if len(queue) != 0 {
			t.Fatalf("ready[%d] len = %d, want 0", priority, len(queue))
		}
	}
	if got := len(scheduler.completions); got != 0 {
		t.Fatalf("len(completions) = %d, want 0", got)
	}
}

func TestSchedulerDispatchReady_UnblocksOnCanceledContext(t *testing.T) {
	scheduler := NewScheduler(&fakeDeviceSource{}, nil)
	scheduler.tasks = make(chan PollTask, 1)
	scheduler.tasks <- PollTask{Key: NewTaskKey(uuid.MustParse("60000000-0000-0000-0000-000000000009"), domain.VolatilityClassPerformance)}

	item := &heapItem{
		task: PollTask{
			Key:             NewTaskKey(uuid.MustParse("60000000-0000-0000-0000-000000000010"), domain.VolatilityClassOperational),
			VolatilityClass: domain.VolatilityClassOperational,
		},
		dueAt:    time.Unix(1_700_000_000, 0).UTC(),
		interval: time.Minute,
		queued:   true,
		index:    -1,
	}
	scheduler.ready[VolatilityPriority(domain.VolatilityClassOperational)] = append(
		scheduler.ready[VolatilityPriority(domain.VolatilityClassOperational)],
		item,
	)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	scheduler.dispatchReady(ctx, time.Unix(1_700_000_000, 0).UTC())

	if scheduler.inFlight != 0 {
		t.Fatalf("inFlight = %d, want 0", scheduler.inFlight)
	}
	if item.inFlight {
		t.Fatalf("inFlight = true, want false")
	}
	if !item.queued {
		t.Fatalf("queued = false, want true")
	}
	if got := len(scheduler.ready[VolatilityPriority(domain.VolatilityClassOperational)]); got != 1 {
		t.Fatalf("ready queue length = %d, want 1", got)
	}
}

type fakeDeviceSource struct {
	devices []domain.Device
	err     error
}

func (s *fakeDeviceSource) GetDevices() ([]domain.Device, error) {
	devices := make([]domain.Device, len(s.devices))
	copy(devices, s.devices)
	return devices, s.err
}

func mustSchedulerItem(t *testing.T, scheduler *Scheduler, key TaskKey) *heapItem {
	t.Helper()

	item, ok := scheduler.items[key]
	if !ok {
		t.Fatalf("missing scheduler item for key %+v", key)
	}

	return item
}

func assertSchedulerKeyMissing(t *testing.T, scheduler *Scheduler, key TaskKey) {
	t.Helper()

	if _, ok := scheduler.items[key]; ok {
		t.Fatalf("scheduler key %+v still present", key)
	}
}

func allVolatilityClasses() []domain.VolatilityClass {
	return []domain.VolatilityClass{
		domain.VolatilityClassPerformance,
		domain.VolatilityClassOperational,
		domain.VolatilityClassStatic,
	}
}

func readyCountForKind(scheduler *Scheduler, kind polling.TaskKind) int {
	count := 0
	for _, queue := range scheduler.ready {
		for _, item := range queue {
			if normalizeTask(item.task).Kind == kind {
				count++
			}
		}
	}
	return count
}

func schedulerIntPtr(value int) *int {
	return &value
}

type fakeSettingsRepo struct {
	values map[string]string
	err    error
}

func (r fakeSettingsRepo) Get(key string) (string, error) {
	if r.err != nil {
		return "", r.err
	}
	if r.values == nil {
		return "", errors.New("missing key")
	}
	value, ok := r.values[key]
	if !ok {
		return "", errors.New("missing key")
	}
	return value, nil
}

func (r fakeSettingsRepo) Set(key, value string) error {
	if r.values == nil {
		r.values = make(map[string]string)
	}
	r.values[key] = value
	return nil
}

func (r fakeSettingsRepo) GetAll() (map[string]string, error) {
	if r.err != nil {
		return nil, r.err
	}
	out := make(map[string]string, len(r.values))
	for key, value := range r.values {
		out[key] = value
	}
	return out, nil
}
