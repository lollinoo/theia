package scheduler

import (
	"container/heap"
	"context"
	"errors"
	"log"
	"math/rand"
	"net"
	"net/netip"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/domain"
	"github.com/lollinoo/theia/internal/observability"
	"github.com/lollinoo/theia/internal/polling"
	"github.com/lollinoo/theia/internal/pollingbudget"
)

const defaultInventoryRefreshInterval = 30 * time.Second
const defaultTaskBuffer = 128

var ErrAlreadyStarted = errors.New("scheduler: already started")

type DeviceSource interface {
	GetDevices() ([]domain.Device, error)
}

type Scheduler struct {
	source          DeviceSource
	settingsRepo    domain.SettingsRepository
	refreshInterval time.Duration
	tasks           chan PollTask
	completions     chan Completion
	redueRequests   chan reduePerformanceTaskRequest
	now             func() time.Time
	rnd             *rand.Rand
	items           map[TaskKey]*heapItem
	heap            taskHeap
	ready           [3][]*heapItem

	running            atomic.Bool
	lifecycleMu        sync.Mutex
	mu                 sync.RWMutex
	cancel             context.CancelFunc
	done               chan struct{}
	inFlight           int
	inFlightByClass    map[domain.VolatilityClass]int
	inFlightByKind     map[polling.TaskKind]int
	inFlightByDevice   map[string]int
	inFlightBySite     map[string]int
	inFlightBySubnet   map[string]int
	inFlightByProfile  map[string]int
	essentialByDevice  map[string]int
	essentialBySite    map[string]int
	essentialBySubnet  map[string]int
	essentialByProfile map[string]int
	runID              uint64

	deadlineMissTotal uint64
	lastWarnings      []polling.CapacityWarning
	degradedRisk      bool
}

type reduePerformanceTaskRequest struct {
	device    domain.Device
	changedAt time.Time
}

func NewScheduler(source DeviceSource, settingsRepo domain.SettingsRepository) *Scheduler {
	scheduler := &Scheduler{
		source:             source,
		settingsRepo:       settingsRepo,
		refreshInterval:    defaultInventoryRefreshInterval,
		tasks:              make(chan PollTask, defaultTaskBuffer),
		completions:        make(chan Completion, defaultTaskBuffer),
		redueRequests:      make(chan reduePerformanceTaskRequest, defaultTaskBuffer),
		now:                time.Now,
		rnd:                rand.New(rand.NewSource(1)),
		items:              make(map[TaskKey]*heapItem),
		done:               make(chan struct{}),
		inFlightByClass:    make(map[domain.VolatilityClass]int),
		inFlightByKind:     make(map[polling.TaskKind]int),
		inFlightByDevice:   make(map[string]int),
		inFlightBySite:     make(map[string]int),
		inFlightBySubnet:   make(map[string]int),
		inFlightByProfile:  make(map[string]int),
		essentialByDevice:  make(map[string]int),
		essentialBySite:    make(map[string]int),
		essentialBySubnet:  make(map[string]int),
		essentialByProfile: make(map[string]int),
	}
	heap.Init(&scheduler.heap)
	return scheduler
}

func (s *Scheduler) Tasks() <-chan PollTask {
	return s.tasks
}

func (s *Scheduler) Status() string {
	if s.running.Load() {
		return "running"
	}
	return "stopped"
}

func (s *Scheduler) PollingHealth() polling.HealthSnapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.pollingHealthLocked(s.now().UTC())
}

func (s *Scheduler) pollingHealthLocked(now time.Time) polling.HealthSnapshot {
	lag := s.queueLagForKind(polling.TaskKindEssential, now)
	active := s.inFlightByKind[polling.TaskKindEssential]
	configured := s.maxEssentialInFlight()

	return polling.HealthSnapshot{
		EssentialOverloaded:      lag > 0 && active >= configured,
		DegradedRisk:             s.degradedRisk,
		EssentialQueueLagSeconds: lag.Seconds(),
		DeadlineMissTotal:        s.deadlineMissTotal,
		ActiveWorkers:            active,
		ConfiguredWorkers:        configured,
		Warnings:                 append([]polling.CapacityWarning(nil), s.lastWarnings...),
	}
}

func (s *Scheduler) Start(ctx context.Context) error {
	s.lifecycleMu.Lock()
	defer s.lifecycleMu.Unlock()

	if s.cancel != nil {
		return ErrAlreadyStarted
	}

	derived, cancel := context.WithCancel(ctx)
	s.cancel = cancel
	if s.done == nil {
		s.done = make(chan struct{})
	}
	s.runID++
	s.running.Store(true)

	now := s.now().UTC()
	s.mu.Lock()
	if err := s.refreshDevices(now); err != nil {
		log.Printf("scheduler: initial refresh failed: %v", err)
	}
	s.recordMetricsLocked(now)
	s.mu.Unlock()

	go s.run(derived)
	return nil
}

func (s *Scheduler) Stop() {
	s.lifecycleMu.Lock()
	defer s.lifecycleMu.Unlock()

	if s.cancel == nil {
		return
	}

	s.running.Store(false)
	s.cancel()
	<-s.done
	s.cancel = nil
	s.mu.Lock()
	s.resetRuntimeState()
	s.mu.Unlock()
	s.done = make(chan struct{})
}

func (s *Scheduler) Complete(c Completion) {
	if !s.running.Load() {
		return
	}
	s.completions <- c
}

// ReduePerformanceTask makes the device's performance task immediately due after a poll cadence change.
func (s *Scheduler) ReduePerformanceTask(device domain.Device, changedAt time.Time) {
	if !s.running.Load() {
		return
	}
	if !shouldScheduleRecurringDevice(device) || device.DeviceType == domain.DeviceTypeVirtual {
		return
	}

	if changedAt.IsZero() {
		changedAt = s.now()
	}
	request := reduePerformanceTaskRequest{
		device:    device,
		changedAt: changedAt.UTC(),
	}

	select {
	case s.redueRequests <- request:
	case <-s.done:
	}
}

// ReconcileDeviceTasks immediately aligns scheduled recurring work for a device
// with its current polling eligibility.
func (s *Scheduler) ReconcileDeviceTasks(device domain.Device, changedAt time.Time) {
	if changedAt.IsZero() {
		changedAt = s.now().UTC()
	} else {
		changedAt = changedAt.UTC()
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.removeDeviceTasksLocked(device.ID)
	if shouldScheduleRecurringDevice(device) {
		s.scheduleRecurringDeviceLocked(device, changedAt, nil)
	}
	s.recordMetricsLocked(changedAt)
}

func (s *Scheduler) ScheduleBootstrap(device domain.Device, dueAt time.Time) bool {
	if !s.running.Load() {
		return false
	}
	if !shouldScheduleBootstrapTask(device) {
		return false
	}

	if dueAt.IsZero() {
		dueAt = s.now().UTC()
	}
	request := reduePerformanceTaskRequest{
		device:    device,
		changedAt: dueAt.UTC(),
	}

	select {
	case s.redueRequests <- request:
		return true
	case <-s.done:
		return false
	default:
		observability.Default().IncSchedulerBackpressure(domain.VolatilityClassStatic, "bootstrap_queue_full")
		return false
	}
}

func (s *Scheduler) run(ctx context.Context) {
	defer close(s.done)
	defer s.running.Store(false)

	refreshTicker := time.NewTicker(s.refreshInterval)
	defer refreshTicker.Stop()

	timer := time.NewTimer(time.Hour)
	defer stopTimer(timer)

	for {
		now := s.now().UTC()
		s.mu.Lock()
		s.moveDueTasksToReady(now)
		s.dispatchReady(ctx, now)
		s.recordMetricsLocked(now)
		delay := s.nextWakeDelay(now)
		s.mu.Unlock()
		resetSchedulerTimer(timer, delay)

		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		case <-refreshTicker.C:
			now := s.now().UTC()
			s.mu.Lock()
			if err := s.refreshDevices(now); err != nil {
				log.Printf("scheduler: refresh failed: %v", err)
			}
			s.recordMetricsLocked(now)
			s.mu.Unlock()
		case request := <-s.redueRequests:
			s.mu.Lock()
			s.handleReduePerformanceTask(request)
			s.recordMetricsLocked(s.now().UTC())
			s.mu.Unlock()
		case completion := <-s.completions:
			s.mu.Lock()
			s.handleCompletion(completion)
			s.recordMetricsLocked(s.now().UTC())
			s.mu.Unlock()
		}
	}
}

func (s *Scheduler) refreshDevices(now time.Time) error {
	devices, err := s.source.GetDevices()
	if err != nil {
		return err
	}

	seen := make(map[TaskKey]struct{}, len(devices)*5)

	for _, device := range devices {
		if !shouldScheduleRecurringDevice(device) {
			continue
		}
		s.scheduleRecurringDeviceLocked(device, now, seen)
	}

	for key, item := range s.items {
		if _, ok := seen[key]; ok {
			continue
		}

		if item.inFlight || item.queued {
			item.disabled = true
			continue
		}

		if item.index >= 0 {
			heap.Remove(&s.heap, item.index)
		}
		delete(s.items, key)
	}

	shortest := shortestEssentialInterval(devices)
	policy, warnings := polling.PolicyFromSettings(s.settingsRepo, managedDeviceCount(devices), 300*time.Millisecond, shortest)
	s.lastWarnings = warnings
	s.degradedRisk = policy.DegradedRisk

	return nil
}

func (s *Scheduler) scheduleRecurringDeviceLocked(device domain.Device, now time.Time, seen map[TaskKey]struct{}) {
	if shouldScheduleEssentialTask(device) {
		essentialKey := NewEssentialTaskKey(device.ID)
		if seen != nil {
			seen[essentialKey] = struct{}{}
		}
		s.upsertScheduledItem(
			device,
			essentialKey,
			polling.TaskKindEssential,
			polling.LaneEssential,
			"",
			EssentialInterval(device),
			now,
		)
	}

	if shouldScheduleBootstrapTask(device) {
		bootstrapKey := NewBootstrapTaskKey(device.ID)
		if seen != nil {
			seen[bootstrapKey] = struct{}{}
		}
		if item, ok := s.items[bootstrapKey]; ok {
			s.applyBootstrapSchedule(item, device, item.dueAt, domain.StaticClassInterval)
		} else {
			s.scheduleBootstrapItem(device, now)
		}
	}

	for _, volatility := range scheduledBackgroundVolatilityClassesForDevice(device) {
		key := NewBackgroundTaskKey(device.ID, volatility)
		if seen != nil {
			seen[key] = struct{}{}
		}

		interval := EffectiveInterval(device, volatility)
		s.upsertScheduledItem(device, key, polling.TaskKindBackground, polling.LaneBackground, volatility, interval, now)
	}
}

func (s *Scheduler) upsertScheduledItem(device domain.Device, key TaskKey, kind polling.TaskKind, lane polling.Lane, volatility domain.VolatilityClass, interval time.Duration, now time.Time) {
	if item, ok := s.items[key]; ok {
		item.disabled = false
		item.task.Key = key
		item.task.Kind = kind
		item.task.Lane = lane
		item.task.Device = device
		item.task.PollClass = device.PollClass
		item.task.VolatilityClass = volatility
		item.task.ExpectedInterval = interval
		item.task.DueAt = item.dueAt
		item.task.DeadlineAt = item.dueAt.Add(interval)
		item.interval = interval
		return
	}

	dueAt := now.Add(initialOffset(device.ID, interval))
	task := PollTask{
		Key:              key,
		Kind:             kind,
		Lane:             lane,
		Device:           device,
		PollClass:        device.PollClass,
		VolatilityClass:  volatility,
		ExpectedInterval: interval,
		DueAt:            dueAt,
		DeadlineAt:       dueAt.Add(interval),
	}
	item := &heapItem{
		task:     task,
		dueAt:    dueAt,
		interval: interval,
		index:    -1,
	}

	heap.Push(&s.heap, item)
	s.items[key] = item
}

func (s *Scheduler) removeDeviceTasksLocked(deviceID uuid.UUID) {
	for key, item := range s.items {
		if key.DeviceID != deviceID {
			continue
		}
		if item.queued {
			s.removeReadyItem(item)
		}
		if item.index >= 0 {
			heap.Remove(&s.heap, item.index)
		}
		item.pending = false
		if item.inFlight {
			item.disabled = true
			continue
		}
		delete(s.items, key)
	}
}

func (s *Scheduler) dispatchReady(ctx context.Context, now time.Time) {
	for {
		if s.inFlight >= s.maxDispatchInFlight() {
			s.recordBackpressure("global_limit")
			return
		}

		item := s.popReadyEligible()
		if item == nil {
			return
		}
		if item.disabled {
			delete(s.items, item.task.Key)
			continue
		}

		task := item.task
		task = normalizeTask(task)
		task.RunID = s.runID
		task.DueAt = item.dueAt
		task.QueueLag = now.Sub(item.dueAt)
		if task.QueueLag < 0 {
			task.QueueLag = 0
		}
		task.DeadlineAt = item.dueAt.Add(item.interval)
		task.DeadlineMissed = !task.DeadlineAt.IsZero() && now.After(task.DeadlineAt)
		task.SkippedWindows = item.skippedWindows

		select {
		case s.tasks <- task:
			item.inFlight = true
			item.dispatchedAt = now
			item.runID = s.runID
			item.task = task
			s.inFlight++
			s.incrementInFlight(task)
			if task.Kind == polling.TaskKindEssential && task.DeadlineMissed {
				s.deadlineMissTotal++
				observability.Default().IncPollingDeadlineMiss()
			}
			observability.Default().SetPollingEssentialOverloaded(s.pollingHealthLocked(now).EssentialOverloaded)
			observability.Default().IncSchedulerTaskDispatch(taskVolatilityForMetrics(task))
		case <-ctx.Done():
			s.pushReadyFront(item)
			return
		}
	}
}

func (s *Scheduler) moveDueTasksToReady(now time.Time) {
	s.markElapsedInFlightWindows(now)

	for s.heap.Len() > 0 {
		next := s.heap[0]
		if next.dueAt.After(now) {
			return
		}

		item := heap.Pop(&s.heap).(*heapItem)
		if item.disabled {
			delete(s.items, item.task.Key)
			continue
		}
		if item.inFlight {
			s.markSkippedWindow(item)
			continue
		}
		if item.queued {
			s.markSkippedWindow(item)
			continue
		}

		s.enqueueReady(item)
	}
}

func (s *Scheduler) enqueueReady(item *heapItem) {
	if item == nil || item.queued {
		return
	}

	item.task = normalizeTask(item.task)
	priority := readyPriority(item.task)
	if priority < 0 || priority >= len(s.ready) {
		priority = len(s.ready) - 1
	}

	item.queued = true
	item.task.DueAt = item.dueAt
	s.ready[priority] = append(s.ready[priority], item)
}

func (s *Scheduler) pushReadyFront(item *heapItem) {
	if item == nil || item.queued {
		return
	}

	item.task = normalizeTask(item.task)
	priority := readyPriority(item.task)
	if priority < 0 || priority >= len(s.ready) {
		priority = len(s.ready) - 1
	}

	item.queued = true
	item.task.DueAt = item.dueAt
	s.ready[priority] = append([]*heapItem{item}, s.ready[priority]...)
}

func (s *Scheduler) popReady() *heapItem {
	for priority := range s.ready {
		if len(s.ready[priority]) == 0 {
			continue
		}

		item := s.ready[priority][0]
		s.ready[priority] = s.ready[priority][1:]
		item.queued = false
		return item
	}

	return nil
}

func (s *Scheduler) popReadyEligible() *heapItem {
	budgets := s.classBudgets()
	policy, _ := polling.PolicyFromSettings(s.settingsRepo, 0, 0, 0)
	blockedMetrics := make(map[struct {
		volatility domain.VolatilityClass
		reason     string
	}]struct{})

	for priority := range s.ready {
		if len(s.ready[priority]) == 0 {
			continue
		}

		for index, item := range s.ready[priority] {
			item.task = normalizeTask(item.task)
			if reason := s.dispatchBlockReason(item.task, budgets, policy); reason != "" {
				for _, metric := range blockedDispatchMetrics(item.task, reason) {
					blockedMetrics[metric] = struct{}{}
				}
				continue
			}

			copy(s.ready[priority][index:], s.ready[priority][index+1:])
			last := len(s.ready[priority]) - 1
			s.ready[priority][last] = nil
			s.ready[priority] = s.ready[priority][:last]
			item.queued = false
			return item
		}
	}

	for metric := range blockedMetrics {
		observability.Default().IncSchedulerBackpressure(metric.volatility, metric.reason)
	}
	return nil
}

func (s *Scheduler) handleReduePerformanceTask(request reduePerformanceTaskRequest) {
	device := request.device
	if !shouldScheduleRecurringDevice(device) {
		return
	}
	if device.DeviceType == domain.DeviceTypeVirtual {
		return
	}

	changedAt := request.changedAt
	if changedAt.IsZero() {
		changedAt = s.now().UTC()
	} else {
		changedAt = changedAt.UTC()
	}

	if shouldScheduleBootstrapTask(device) {
		s.scheduleBootstrapItem(device, changedAt)
		return
	}

	key := NewTaskKey(device.ID, domain.VolatilityClassPerformance)
	interval := EffectiveInterval(device, domain.VolatilityClassPerformance)

	if item, ok := s.items[key]; ok {
		s.applyPerformanceRedue(item, device, changedAt, interval)

		switch {
		case item.inFlight:
			item.pending = true
			item.immediateRerun = true
		case item.queued:
			s.removeReadyItem(item)
			s.pushReadyFront(item)
		case item.index >= 0:
			heap.Fix(&s.heap, item.index)
		default:
			s.pushReadyFront(item)
		}
		return
	}

	item := &heapItem{
		task: PollTask{
			Key:              key,
			Kind:             polling.TaskKindBackground,
			Lane:             polling.LaneBackground,
			Device:           device,
			PollClass:        device.PollClass,
			VolatilityClass:  domain.VolatilityClassPerformance,
			ExpectedInterval: interval,
			DueAt:            changedAt,
			DeadlineAt:       changedAt.Add(interval),
		},
		dueAt:    changedAt,
		interval: interval,
		index:    -1,
	}
	s.items[key] = item
	s.pushReadyFront(item)
}

func (s *Scheduler) scheduleBootstrapItem(device domain.Device, dueAt time.Time) {
	key := NewBootstrapTaskKey(device.ID)
	interval := domain.StaticClassInterval

	if item, ok := s.items[key]; ok {
		s.applyBootstrapSchedule(item, device, dueAt, interval)
		switch {
		case item.inFlight:
			item.pending = true
			item.immediateRerun = true
		case item.queued:
			s.removeReadyItem(item)
			s.pushReadyFront(item)
		case item.index >= 0:
			heap.Fix(&s.heap, item.index)
		default:
			s.pushReadyFront(item)
		}
		return
	}

	item := &heapItem{
		task: PollTask{
			Key:              key,
			Kind:             polling.TaskKindBootstrap,
			Lane:             polling.LaneBootstrap,
			Device:           device,
			PollClass:        device.PollClass,
			VolatilityClass:  domain.VolatilityClassStatic,
			ExpectedInterval: interval,
			DueAt:            dueAt,
			DeadlineAt:       dueAt.Add(interval),
		},
		dueAt:    dueAt,
		interval: interval,
		index:    -1,
	}
	s.items[key] = item
	s.pushReadyFront(item)
}

func (s *Scheduler) applyBootstrapSchedule(item *heapItem, device domain.Device, dueAt time.Time, interval time.Duration) {
	item.disabled = false
	item.task.Key = NewBootstrapTaskKey(device.ID)
	item.task.Kind = polling.TaskKindBootstrap
	item.task.Lane = polling.LaneBootstrap
	item.task.Device = device
	item.task.PollClass = device.PollClass
	item.task.VolatilityClass = domain.VolatilityClassStatic
	item.task.ExpectedInterval = interval
	item.interval = interval
	item.dueAt = dueAt
	item.task.DueAt = dueAt
	item.task.DeadlineAt = dueAt.Add(interval)
}

func (s *Scheduler) applyPerformanceRedue(item *heapItem, device domain.Device, changedAt time.Time, interval time.Duration) {
	item.disabled = false
	item.task.Key = NewTaskKey(device.ID, domain.VolatilityClassPerformance)
	item.task.Kind = polling.TaskKindBackground
	item.task.Lane = polling.LaneBackground
	item.task.Device = device
	item.task.PollClass = device.PollClass
	item.task.VolatilityClass = domain.VolatilityClassPerformance
	item.task.ExpectedInterval = interval
	item.interval = interval
	item.dueAt = changedAt
	item.task.DueAt = changedAt
	item.task.DeadlineAt = changedAt.Add(interval)
}

func (s *Scheduler) removeReadyItem(item *heapItem) {
	for priority := range s.ready {
		queue := s.ready[priority]
		for index, queued := range queue {
			if queued != item {
				continue
			}

			copy(queue[index:], queue[index+1:])
			queue[len(queue)-1] = nil
			s.ready[priority] = queue[:len(queue)-1]
			item.queued = false
			return
		}
	}
}

func (s *Scheduler) handleCompletion(c Completion) {
	item, ok := s.items[c.Key]
	if !ok || !item.inFlight {
		return
	}

	finishedAt := c.FinishedAt.UTC()
	if c.FinishedAt.IsZero() {
		finishedAt = s.now().UTC()
	}
	if !item.dispatchedAt.IsZero() && finishedAt.Before(item.dispatchedAt) {
		return
	}
	if c.RunID != item.runID {
		return
	}
	if !item.dispatchedAt.IsZero() {
		observability.Default().ObserveSchedulerTaskDuration(item.task.VolatilityClass, finishedAt.Sub(item.dispatchedAt))
	}

	s.inFlight--
	s.decrementInFlight(item.task)
	item.inFlight = false
	item.dispatchedAt = time.Time{}
	item.runID = 0
	item.task.RunID = 0

	if item.disabled {
		delete(s.items, c.Key)
		return
	}
	pending := item.pending
	immediateRerun := item.immediateRerun
	overdue := item.interval > 0 && finishedAt.After(item.dueAt.Add(item.interval))
	item.pending = false
	item.immediateRerun = false

	if immediateRerun || (item.task.Kind == polling.TaskKindEssential && (pending || overdue)) {
		item.dueAt = finishedAt
		item.task.DueAt = finishedAt
		item.task.DeadlineAt = finishedAt.Add(item.interval)
		s.enqueueReady(item)
		return
	}

	next := jitteredNext(finishedAt, item.interval, s.rnd)
	item.dueAt = next
	item.task.DueAt = next
	item.task.DeadlineAt = next.Add(item.interval)
	item.skippedWindows = 0
	heap.Push(&s.heap, item)
}

func (s *Scheduler) maxInFlight() int {
	return pollingbudget.Sum(s.classBudgets())
}

func (s *Scheduler) maxDispatchInFlight() int {
	limit := s.maxInFlight() + s.maxEssentialInFlight()
	if bufferLimit := s.bufferLimit(); bufferLimit > 0 && limit > bufferLimit {
		return bufferLimit
	}
	return limit
}

func (s *Scheduler) maxEssentialInFlight() int {
	policy, _ := polling.PolicyFromSettings(s.settingsRepo, 0, 0, 0)
	limit := policy.EssentialWorkers
	if bufferLimit := s.bufferLimit(); bufferLimit > 0 && limit > bufferLimit {
		return bufferLimit
	}
	if limit <= 0 {
		return 1
	}
	return limit
}

func (s *Scheduler) canDispatch(task PollTask, budgets map[domain.VolatilityClass]int, policy polling.Policy) bool {
	return s.dispatchBlockReason(task, budgets, policy) == ""
}

func (s *Scheduler) dispatchBlockReason(task PollTask, budgets map[domain.VolatilityClass]int, policy polling.Policy) string {
	task = normalizeTask(task)
	if task.Kind == polling.TaskKindEssential {
		if s.inFlightByKind[polling.TaskKindEssential] >= s.maxEssentialInFlight() {
			return "essential_limit"
		}
		return s.isolationBlockReason(task, policy)
	}
	if s.inFlightByClass[task.VolatilityClass] >= budgets[task.VolatilityClass] {
		return "class_limit"
	}
	return s.isolationBlockReason(task, policy)
}

func (s *Scheduler) incrementInFlight(task PollTask) {
	if task.Kind == polling.TaskKindEssential {
		s.inFlightByKind[polling.TaskKindEssential]++
	} else {
		s.inFlightByClass[task.VolatilityClass]++
	}
	s.incrementIsolationCounts(task)
}

func (s *Scheduler) decrementInFlight(task PollTask) {
	if task.Kind == polling.TaskKindEssential {
		if s.inFlightByKind[polling.TaskKindEssential] > 0 {
			s.inFlightByKind[polling.TaskKindEssential]--
		}
	} else if s.inFlightByClass[task.VolatilityClass] > 0 {
		s.inFlightByClass[task.VolatilityClass]--
	}
	s.decrementIsolationCounts(task)
}

func (s *Scheduler) withinIsolationBudgets(task PollTask, policy polling.Policy) bool {
	return s.isolationBlockReason(task, policy) == ""
}

func (s *Scheduler) isolationBlockReason(task PollTask, policy polling.Policy) string {
	deviceCounts := s.inFlightByDevice
	siteCounts := s.inFlightBySite
	subnetCounts := s.inFlightBySubnet
	profileCounts := s.inFlightByProfile
	if normalizeTask(task).Kind == polling.TaskKindEssential {
		deviceCounts = s.essentialByDevice
		siteCounts = s.essentialBySite
		subnetCounts = s.essentialBySubnet
		profileCounts = s.essentialByProfile
	}

	deviceKey := task.Device.ID.String()
	if deviceKey != "" && policy.MaxWorkersPerDevice > 0 && deviceCounts[deviceKey] >= policy.MaxWorkersPerDevice {
		return "device_limit"
	}
	if policy.MaxWorkersPerSite > 0 {
		for _, siteKey := range taskSiteKeys(task) {
			if siteCounts[siteKey] >= policy.MaxWorkersPerSite {
				return "site_limit"
			}
		}
	}
	if subnetKey := taskSubnetKey(task); subnetKey != "" && policy.MaxWorkersPerSubnet > 0 && subnetCounts[subnetKey] >= policy.MaxWorkersPerSubnet {
		return "subnet_limit"
	}
	if profileKey := taskProfileKey(task); profileKey != "" && policy.MaxInflightPerProfile > 0 && profileCounts[profileKey] >= policy.MaxInflightPerProfile {
		return "profile_limit"
	}
	return ""
}

func blockedDispatchMetrics(task PollTask, reason string) []struct {
	volatility domain.VolatilityClass
	reason     string
} {
	task = normalizeTask(task)
	if task.Kind != polling.TaskKindEssential {
		return []struct {
			volatility domain.VolatilityClass
			reason     string
		}{
			{volatility: taskVolatilityForMetrics(task), reason: reason},
		}
	}

	metrics := []struct {
		volatility domain.VolatilityClass
		reason     string
	}{
		{volatility: domain.VolatilityClassPerformance, reason: "essential_limit"},
	}
	if reason != "essential_limit" {
		metrics = append(metrics, struct {
			volatility domain.VolatilityClass
			reason     string
		}{volatility: domain.VolatilityClassPerformance, reason: "essential_" + reason})
	}
	return metrics
}

func (s *Scheduler) incrementIsolationCounts(task PollTask) {
	incrementCount(s.inFlightByDevice, task.Device.ID.String())
	for _, siteKey := range taskSiteKeys(task) {
		incrementCount(s.inFlightBySite, siteKey)
	}
	incrementCount(s.inFlightBySubnet, taskSubnetKey(task))
	incrementCount(s.inFlightByProfile, taskProfileKey(task))

	if normalizeTask(task).Kind != polling.TaskKindEssential {
		return
	}
	incrementCount(s.essentialByDevice, task.Device.ID.String())
	for _, siteKey := range taskSiteKeys(task) {
		incrementCount(s.essentialBySite, siteKey)
	}
	incrementCount(s.essentialBySubnet, taskSubnetKey(task))
	incrementCount(s.essentialByProfile, taskProfileKey(task))
}

func (s *Scheduler) decrementIsolationCounts(task PollTask) {
	decrementCount(s.inFlightByDevice, task.Device.ID.String())
	for _, siteKey := range taskSiteKeys(task) {
		decrementCount(s.inFlightBySite, siteKey)
	}
	decrementCount(s.inFlightBySubnet, taskSubnetKey(task))
	decrementCount(s.inFlightByProfile, taskProfileKey(task))

	if normalizeTask(task).Kind != polling.TaskKindEssential {
		return
	}
	decrementCount(s.essentialByDevice, task.Device.ID.String())
	for _, siteKey := range taskSiteKeys(task) {
		decrementCount(s.essentialBySite, siteKey)
	}
	decrementCount(s.essentialBySubnet, taskSubnetKey(task))
	decrementCount(s.essentialByProfile, taskProfileKey(task))
}

func incrementCount(counts map[string]int, key string) {
	if key == "" {
		return
	}
	counts[key]++
}

func decrementCount(counts map[string]int, key string) {
	if key == "" {
		return
	}
	if counts[key] <= 1 {
		delete(counts, key)
		return
	}
	counts[key]--
}

func (s *Scheduler) markElapsedInFlightWindows(now time.Time) {
	for _, item := range s.items {
		if item == nil || !item.inFlight || item.dueAt.After(now) {
			continue
		}
		if item.interval > 0 && now.Before(item.dueAt.Add(item.interval)) {
			continue
		}
		s.markSkippedWindow(item)
	}
}

func (s *Scheduler) markSkippedWindow(item *heapItem) {
	if item.pending {
		return
	}
	item.pending = true
	item.skippedWindows++
	observability.Default().IncSchedulerBackpressure(taskVolatilityForMetrics(item.task), "skipped_window")
}

func (s *Scheduler) nextWakeDelay(now time.Time) time.Duration {
	if s.heap.Len() == 0 {
		return s.refreshInterval
	}

	delay := s.heap[0].dueAt.Sub(now)
	if delay < 0 {
		return 0
	}
	return delay
}

func scheduledVolatilityClasses() []domain.VolatilityClass {
	return []domain.VolatilityClass{
		domain.VolatilityClassPerformance,
		domain.VolatilityClassOperational,
		domain.VolatilityClassStatic,
	}
}

func scheduledVolatilityClassesForDevice(device domain.Device) []domain.VolatilityClass {
	return scheduledBackgroundVolatilityClassesForDevice(device)
}

func scheduledBackgroundVolatilityClassesForDevice(device domain.Device) []domain.VolatilityClass {
	if device.DeviceType == domain.DeviceTypeVirtual {
		return []domain.VolatilityClass{domain.VolatilityClassOperational}
	}
	return scheduledVolatilityClasses()
}

func managedDeviceCount(devices []domain.Device) int {
	count := 0
	for _, device := range devices {
		if shouldScheduleEssentialTask(device) {
			count++
		}
	}
	return count
}

func shortestEssentialInterval(devices []domain.Device) time.Duration {
	var shortest time.Duration
	for _, device := range devices {
		if !shouldScheduleEssentialTask(device) {
			continue
		}
		interval := EssentialInterval(device)
		if interval <= 0 {
			continue
		}
		if shortest == 0 || interval < shortest {
			shortest = interval
		}
	}
	return shortest
}

func normalizeTask(task PollTask) PollTask {
	if task.Kind == "" {
		task.Kind = task.Key.Kind
	}
	if task.Kind == "" {
		task.Kind = polling.TaskKindBackground
	}
	if task.Lane == "" {
		switch task.Kind {
		case polling.TaskKindEssential:
			task.Lane = polling.LaneEssential
		case polling.TaskKindBootstrap:
			task.Lane = polling.LaneBootstrap
		default:
			task.Lane = polling.LaneBackground
		}
	}
	if task.Kind == polling.TaskKindBackground && task.VolatilityClass == "" {
		task.VolatilityClass = task.Key.VolatilityClass
	}
	return task
}

func readyPriority(task PollTask) int {
	task = normalizeTask(task)
	if task.Kind == polling.TaskKindEssential {
		return 0
	}
	return VolatilityPriority(task.VolatilityClass)
}

func heapPriority(task PollTask) int {
	task = normalizeTask(task)
	if task.Kind == polling.TaskKindEssential {
		return -1
	}
	return VolatilityPriority(task.VolatilityClass)
}

func taskVolatilityForMetrics(task PollTask) domain.VolatilityClass {
	task = normalizeTask(task)
	if task.VolatilityClass != "" {
		return task.VolatilityClass
	}
	return domain.VolatilityClassPerformance
}

func shouldScheduleEssentialTask(device domain.Device) bool {
	return shouldScheduleRecurringDevice(device) && !domain.IsVirtualWithIPDevice(device)
}

func shouldScheduleRecurringDevice(device domain.Device) bool {
	return device.Managed && domain.DevicePollingEnabled(device) && !domain.IsVirtualNoIPDevice(device)
}

func taskSiteKeys(task PollTask) []string {
	if len(task.Device.AreaIDs) == 0 {
		return nil
	}
	keys := make([]string, 0, len(task.Device.AreaIDs))
	for _, areaID := range task.Device.AreaIDs {
		if areaID == uuid.Nil {
			continue
		}
		keys = append(keys, areaID.String())
	}
	sort.Strings(keys)
	return keys
}

func taskSubnetKey(task PollTask) string {
	rawIP := strings.TrimSpace(task.Device.IP)
	if rawIP == "" {
		return ""
	}
	if host, _, err := net.SplitHostPort(rawIP); err == nil {
		rawIP = host
	}
	rawIP = strings.Trim(rawIP, "[]")
	addr, err := netip.ParseAddr(rawIP)
	if err != nil {
		return ""
	}
	bits := 64
	if addr.Is4() {
		bits = 24
	}
	return netip.PrefixFrom(addr, bits).Masked().String()
}

func taskProfileKey(task PollTask) string {
	creds := task.Device.SNMPCredentials
	switch creds.Version {
	case domain.SNMPVersionV2c:
		if creds.V2c == nil {
			return ""
		}
		return "2c|" + creds.V2c.Community
	case domain.SNMPVersionV3:
		if creds.V3 == nil {
			return ""
		}
		return strings.Join([]string{
			"3",
			creds.V3.Username,
			creds.V3.SecurityLevel,
			creds.V3.AuthProtocol,
			creds.V3.PrivProtocol,
		}, "|")
	default:
		return ""
	}
}

func shouldScheduleBootstrapTask(device domain.Device) bool {
	if !shouldScheduleRecurringDevice(device) {
		return false
	}
	if device.DeviceType == domain.DeviceTypeVirtual {
		return false
	}
	if strings.TrimSpace(device.IP) == "" {
		return false
	}
	if device.MetricsSource == domain.MetricsSourcePrometheus || device.MetricsSource == domain.MetricsSourceNone {
		return false
	}
	return domain.NormalizeTopologyBootstrapState(device.TopologyBootstrapState) == domain.TopologyBootstrapStatePending
}

func resetSchedulerTimer(timer *time.Timer, delay time.Duration) {
	stopTimer(timer)
	timer.Reset(delay)
}

func stopTimer(timer *time.Timer) {
	if timer == nil {
		return
	}
	if !timer.Stop() {
		select {
		case <-timer.C:
		default:
		}
	}
}

func (s *Scheduler) bufferLimit() int {
	limit := cap(s.tasks)
	if completionsCap := cap(s.completions); limit == 0 || (completionsCap > 0 && completionsCap < limit) {
		limit = completionsCap
	}
	return limit
}

func (s *Scheduler) resetRuntimeState() {
	s.items = make(map[TaskKey]*heapItem)
	s.heap = nil
	heap.Init(&s.heap)
	s.ready = [3][]*heapItem{}
	s.inFlight = 0
	s.inFlightByClass = make(map[domain.VolatilityClass]int)
	s.inFlightByKind = make(map[polling.TaskKind]int)
	s.inFlightByDevice = make(map[string]int)
	s.inFlightBySite = make(map[string]int)
	s.inFlightBySubnet = make(map[string]int)
	s.inFlightByProfile = make(map[string]int)
	s.essentialByDevice = make(map[string]int)
	s.essentialBySite = make(map[string]int)
	s.essentialBySubnet = make(map[string]int)
	s.essentialByProfile = make(map[string]int)
	s.deadlineMissTotal = 0
	s.lastWarnings = nil
	s.degradedRisk = false

	for {
		select {
		case <-s.tasks:
		default:
			goto drainCompletions
		}
	}

drainCompletions:
	for {
		select {
		case <-s.completions:
		default:
			goto drainRedueRequests
		}
	}

drainRedueRequests:
	for {
		select {
		case <-s.redueRequests:
		default:
			s.recordMetricsLocked(s.now().UTC())
			return
		}
	}
}

func (s *Scheduler) recordMetrics() {
	now := s.now().UTC()
	s.mu.RLock()
	defer s.mu.RUnlock()
	s.recordMetricsLocked(now)
}

func (s *Scheduler) recordMetricsLocked(now time.Time) {
	observability.Default().SetSchedulerInFlight(s.inFlight)
	observability.Default().SetPollingEssentialOverloaded(s.pollingHealthLocked(now).EssentialOverloaded)
	for _, volatility := range scheduledVolatilityClasses() {
		priority := VolatilityPriority(volatility)
		depth := 0
		if priority >= 0 && priority < len(s.ready) {
			depth = len(s.ready[priority])
		}
		observability.Default().SetSchedulerReadyDepth(volatility, depth)
		observability.Default().SetSchedulerQueueLag(volatility, s.queueLag(volatility, now))
	}
}

func (s *Scheduler) classBudgets() map[domain.VolatilityClass]int {
	budgets := pollingbudget.Resolve(s.settingsRepo)
	if limit := s.bufferLimit(); limit > 0 {
		budgets = pollingbudget.Clamp(budgets, limit)
	}
	return budgets
}

func (s *Scheduler) queueLag(volatility domain.VolatilityClass, now time.Time) time.Duration {
	priority := VolatilityPriority(volatility)
	if priority < 0 || priority >= len(s.ready) || len(s.ready[priority]) == 0 {
		return 0
	}

	oldestDue := s.ready[priority][0].dueAt
	for _, item := range s.ready[priority][1:] {
		if item.dueAt.Before(oldestDue) {
			oldestDue = item.dueAt
		}
	}
	if oldestDue.After(now) {
		return 0
	}
	return now.Sub(oldestDue)
}

func (s *Scheduler) queueLagForKind(kind polling.TaskKind, now time.Time) time.Duration {
	var oldest *time.Time
	consider := func(item *heapItem) {
		if item == nil || normalizeTask(item.task).Kind != kind {
			return
		}
		dueAt := item.dueAt
		if oldest == nil || dueAt.Before(*oldest) {
			oldest = &dueAt
		}
	}

	for priority := range s.ready {
		for _, item := range s.ready[priority] {
			consider(item)
		}
	}
	for _, item := range s.items {
		if item == nil || !item.inFlight {
			continue
		}
		if item.interval > 0 && now.Before(item.dueAt.Add(item.interval)) {
			continue
		}
		consider(item)
	}

	if oldest == nil || oldest.After(now) {
		return 0
	}
	return now.Sub(*oldest)
}

func (s *Scheduler) recordBackpressure(reason string) {
	for _, volatility := range scheduledVolatilityClasses() {
		if len(s.ready[VolatilityPriority(volatility)]) == 0 {
			continue
		}
		observability.Default().IncSchedulerBackpressure(volatility, reason)
	}
}
