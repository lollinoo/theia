package scheduler

import (
	"container/heap"
	"context"
	"log"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"

	"github.com/lollinoo/theia/internal/domain"
	"github.com/lollinoo/theia/internal/observability"
	"github.com/lollinoo/theia/internal/pollingbudget"
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

	running         atomic.Bool
	lifecycleMu     sync.Mutex
	cancel          context.CancelFunc
	done            chan struct{}
	inFlight        int
	inFlightByClass map[domain.VolatilityClass]int
	runID           uint64
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
		inFlightByClass: make(map[domain.VolatilityClass]int),
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
	s.recordMetrics()

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
		s.recordMetrics()
		resetSchedulerTimer(timer, s.nextWakeDelay(now))

		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		case <-refreshTicker.C:
			if err := s.refreshDevices(s.now().UTC()); err != nil {
				log.Printf("scheduler: refresh failed: %v", err)
			}
			s.recordMetrics()
		case request := <-s.redueRequests:
			s.handleReduePerformanceTask(request)
			s.recordMetrics()
		case completion := <-s.completions:
			s.handleCompletion(completion)
			s.recordMetrics()
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
		if domain.IsVirtualNoIPDevice(device) {
			continue
		}

		for _, volatility := range scheduledVolatilityClassesForDevice(device) {
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
	for {
		if s.inFlight >= s.maxInFlight() {
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
			s.inFlightByClass[item.task.VolatilityClass]++
			observability.Default().IncSchedulerTaskDispatch(task.VolatilityClass)
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

func (s *Scheduler) popReadyEligible() *heapItem {
	budgets := s.classBudgets()
	blocked := make(map[domain.VolatilityClass]struct{})

	for priority := range s.ready {
		if len(s.ready[priority]) == 0 {
			continue
		}

		item := s.ready[priority][0]
		if s.inFlightByClass[item.task.VolatilityClass] >= budgets[item.task.VolatilityClass] {
			blocked[item.task.VolatilityClass] = struct{}{}
			continue
		}

		s.ready[priority] = s.ready[priority][1:]
		item.queued = false
		return item
	}

	for volatility := range blocked {
		observability.Default().IncSchedulerBackpressure(volatility, "class_limit")
	}
	if len(blocked) > 0 {
		return s.popReady()
	}
	return nil
}

func (s *Scheduler) handleReduePerformanceTask(request reduePerformanceTaskRequest) {
	device := request.device
	if !device.Managed {
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
	if !item.dispatchedAt.IsZero() {
		observability.Default().ObserveSchedulerTaskDuration(item.task.VolatilityClass, finishedAt.Sub(item.dispatchedAt))
	}

	s.inFlight--
	if s.inFlightByClass[item.task.VolatilityClass] > 0 {
		s.inFlightByClass[item.task.VolatilityClass]--
	}
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
	return pollingbudget.Sum(s.classBudgets())
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
	if device.DeviceType == domain.DeviceTypeVirtual {
		return []domain.VolatilityClass{domain.VolatilityClassOperational}
	}
	return scheduledVolatilityClasses()
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
			s.recordMetrics()
			return
		}
	}
}

func (s *Scheduler) recordMetrics() {
	observability.Default().SetSchedulerInFlight(s.inFlight)
	for _, volatility := range scheduledVolatilityClasses() {
		priority := VolatilityPriority(volatility)
		depth := 0
		if priority >= 0 && priority < len(s.ready) {
			depth = len(s.ready[priority])
		}
		observability.Default().SetSchedulerReadyDepth(volatility, depth)
		observability.Default().SetSchedulerQueueLag(volatility, s.queueLag(volatility, s.now().UTC()))
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

func (s *Scheduler) recordBackpressure(reason string) {
	for _, volatility := range scheduledVolatilityClasses() {
		if len(s.ready[VolatilityPriority(volatility)]) == 0 {
			continue
		}
		observability.Default().IncSchedulerBackpressure(volatility, reason)
	}
}
