package scheduler

import (
	"container/heap"
	"context"
	"log"
	"math/rand"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/lollinoo/theia/internal/domain"
)

const defaultInventoryRefreshInterval = 30 * time.Second
const defaultTaskBuffer = 128

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

	running     atomic.Bool
	lifecycleMu sync.Mutex
	cancel      context.CancelFunc
	done        chan struct{}
	inFlight    int
	runID       uint64
}

type reduePerformanceTaskRequest struct {
	device    domain.Device
	changedAt time.Time
}

func NewScheduler(source DeviceSource, settingsRepo domain.SettingsRepository) *Scheduler {
	scheduler := &Scheduler{
		source:          source,
		settingsRepo:    settingsRepo,
		refreshInterval: defaultInventoryRefreshInterval,
		tasks:           make(chan PollTask, defaultTaskBuffer),
		completions:     make(chan Completion, defaultTaskBuffer),
		redueRequests:   make(chan reduePerformanceTaskRequest, defaultTaskBuffer),
		now:             time.Now,
		rnd:             rand.New(rand.NewSource(1)),
		items:           make(map[TaskKey]*heapItem),
		done:            make(chan struct{}),
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

func (s *Scheduler) Start(ctx context.Context) {
	s.lifecycleMu.Lock()
	defer s.lifecycleMu.Unlock()

	if s.cancel != nil {
		panic("scheduler: Start called more than once")
	}

	derived, cancel := context.WithCancel(ctx)
	s.cancel = cancel
	if s.done == nil {
		s.done = make(chan struct{})
	}
	s.runID++
	s.running.Store(true)

	if err := s.refreshDevices(s.now().UTC()); err != nil {
		log.Printf("scheduler: initial refresh failed: %v", err)
	}

	go s.run(derived)
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
	s.resetRuntimeState()
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

func (s *Scheduler) run(ctx context.Context) {
	defer close(s.done)
	defer s.running.Store(false)

	refreshTicker := time.NewTicker(s.refreshInterval)
	defer refreshTicker.Stop()

	timer := time.NewTimer(time.Hour)
	defer stopTimer(timer)

	for {
		now := s.now().UTC()
		s.moveDueTasksToReady(now)
		s.dispatchReady(ctx, now)
		resetSchedulerTimer(timer, s.nextWakeDelay(now))

		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		case <-refreshTicker.C:
			if err := s.refreshDevices(s.now().UTC()); err != nil {
				log.Printf("scheduler: refresh failed: %v", err)
			}
		case request := <-s.redueRequests:
			s.handleReduePerformanceTask(request)
		case completion := <-s.completions:
			s.handleCompletion(completion)
		}
	}
}

func (s *Scheduler) refreshDevices(now time.Time) error {
	devices, err := s.source.GetDevices()
	if err != nil {
		return err
	}

	seen := make(map[TaskKey]struct{}, len(devices)*3)

	for _, device := range devices {
		if !device.Managed {
			continue
		}

		for _, volatility := range scheduledVolatilityClasses() {
			key := NewTaskKey(device.ID, volatility)
			seen[key] = struct{}{}

			interval := EffectiveInterval(device, volatility)
			if item, ok := s.items[key]; ok {
				item.disabled = false
				item.task.Key = key
				item.task.Device = device
				item.task.PollClass = device.PollClass
				item.task.VolatilityClass = volatility
				item.task.ExpectedInterval = interval
				item.interval = interval
				item.task.DueAt = item.dueAt
				continue
			}

			dueAt := now.Add(initialOffset(device.ID, interval))
			task := PollTask{
				Key:              key,
				Device:           device,
				PollClass:        device.PollClass,
				VolatilityClass:  volatility,
				ExpectedInterval: interval,
				DueAt:            dueAt,
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

	return nil
}

func (s *Scheduler) dispatchReady(ctx context.Context, now time.Time) {
	for s.inFlight < s.maxInFlight() {
		item := s.popReady()
		if item == nil {
			return
		}
		if item.disabled {
			delete(s.items, item.task.Key)
			continue
		}

		task := item.task
		task.RunID = s.runID
		task.DueAt = item.dueAt

		select {
		case s.tasks <- task:
			item.inFlight = true
			item.dispatchedAt = now
			item.runID = s.runID
			item.task.RunID = s.runID
			item.task.DueAt = item.dueAt
			s.inFlight++
		case <-ctx.Done():
			s.pushReadyFront(item)
			return
		}
	}
}

func (s *Scheduler) moveDueTasksToReady(now time.Time) {
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
			item.pending = true
			continue
		}
		if item.queued {
			continue
		}

		s.enqueueReady(item)
	}
}

func (s *Scheduler) enqueueReady(item *heapItem) {
	if item == nil || item.queued {
		return
	}

	priority := VolatilityPriority(item.task.VolatilityClass)
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

	priority := VolatilityPriority(item.task.VolatilityClass)
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

func (s *Scheduler) handleReduePerformanceTask(request reduePerformanceTaskRequest) {
	device := request.device
	if !device.Managed {
		return
	}

	changedAt := request.changedAt
	if changedAt.IsZero() {
		changedAt = s.now().UTC()
	} else {
		changedAt = changedAt.UTC()
	}

	key := NewTaskKey(device.ID, domain.VolatilityClassPerformance)
	interval := EffectiveInterval(device, domain.VolatilityClassPerformance)

	if item, ok := s.items[key]; ok {
		s.applyPerformanceRedue(item, device, changedAt, interval)

		switch {
		case item.inFlight:
			item.pending = true
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
			Device:           device,
			PollClass:        device.PollClass,
			VolatilityClass:  domain.VolatilityClassPerformance,
			ExpectedInterval: interval,
			DueAt:            changedAt,
		},
		dueAt:    changedAt,
		interval: interval,
		index:    -1,
	}
	s.items[key] = item
	s.pushReadyFront(item)
}

func (s *Scheduler) applyPerformanceRedue(item *heapItem, device domain.Device, changedAt time.Time, interval time.Duration) {
	item.disabled = false
	item.task.Key = NewTaskKey(device.ID, domain.VolatilityClassPerformance)
	item.task.Device = device
	item.task.PollClass = device.PollClass
	item.task.VolatilityClass = domain.VolatilityClassPerformance
	item.task.ExpectedInterval = interval
	item.interval = interval
	item.dueAt = changedAt
	item.task.DueAt = changedAt
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

	s.inFlight--
	item.inFlight = false
	item.dispatchedAt = time.Time{}
	item.runID = 0
	item.task.RunID = 0

	if item.disabled {
		delete(s.items, c.Key)
		return
	}
	if item.pending || (item.interval > 0 && finishedAt.After(item.dueAt.Add(item.interval))) {
		item.pending = false
		item.dueAt = finishedAt
		item.task.DueAt = finishedAt
		s.enqueueReady(item)
		return
	}

	next := jitteredNext(finishedAt, item.interval, s.rnd)
	item.dueAt = next
	item.task.DueAt = next
	heap.Push(&s.heap, item)
}

func (s *Scheduler) maxInFlight() int {
	if s.settingsRepo == nil {
		return 5
	}

	value, err := s.settingsRepo.Get(domain.SettingSNMPWorkerPoolSize)
	if err != nil {
		return 5
	}

	size, err := strconv.Atoi(value)
	if err != nil || size <= 0 {
		return 5
	}
	if limit := s.bufferLimit(); limit > 0 && size > limit {
		return limit
	}

	return size
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
			return
		}
	}
}
