package scheduler

import "time"

type heapItem struct {
	task           PollTask
	dueAt          time.Time
	dispatchedAt   time.Time
	runID          uint64
	interval       time.Duration
	queued         bool
	inFlight       bool
	pending        bool
	immediateRerun bool
	disabled       bool
	skippedWindows int
	index          int
}

type taskHeap []*heapItem

func (h taskHeap) Len() int {
	return len(h)
}

func (h taskHeap) Less(i, j int) bool {
	if !h[i].dueAt.Equal(h[j].dueAt) {
		return h[i].dueAt.Before(h[j].dueAt)
	}

	leftPriority := heapPriority(h[i].task)
	rightPriority := heapPriority(h[j].task)
	if leftPriority != rightPriority {
		return leftPriority < rightPriority
	}

	return h[i].task.Key.DeviceID.String() < h[j].task.Key.DeviceID.String()
}

func (h taskHeap) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
	h[i].index = i
	h[j].index = j
}

func (h *taskHeap) Push(x any) {
	item := x.(*heapItem)
	item.index = len(*h)
	*h = append(*h, item)
}

func (h *taskHeap) Pop() any {
	old := *h
	last := len(old) - 1
	item := old[last]
	old[last] = nil
	item.index = -1
	*h = old[:last]
	return item
}
