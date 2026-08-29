package application

import (
	"context"
	"errors"
	"sync"
	"time"
)

var ErrQueueFull = errors.New("test_queue_full")

type TestRequest struct {
	ID, NodeID, Kind string
	Run              func(context.Context) error
	OnStart          func()
}

type TestQueue struct {
	mu            sync.Mutex
	queue         []TestRequest
	running       bool
	runningID     string
	runningNodeID string
	runningKind   string
	runningCancel context.CancelFunc
	worker        bool
	max           int
	notify        chan struct{}
}

func (q *TestQueue) Max() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.max
}

func NewTestQueue(max int) *TestQueue {
	if max < 1 {
		max = 1
	}
	return &TestQueue{max: max, notify: make(chan struct{}, 1)}
}

func (q *TestQueue) Enqueue(request TestRequest) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.queue) >= q.max {
		return ErrQueueFull
	}
	q.queue = append(q.queue, request)
	select {
	case q.notify <- struct{}{}:
	default:
	}
	return nil
}

func (q *TestQueue) Run(ctx context.Context) {
	q.mu.Lock()
	if q.worker {
		q.mu.Unlock()
		return
	}
	q.worker = true
	q.mu.Unlock()
	defer func() {
		q.mu.Lock()
		q.worker = false
		q.mu.Unlock()
	}()
	for {
		select {
		case <-ctx.Done():
			return
		case <-q.notify:
			for {
				q.mu.Lock()
				if len(q.queue) == 0 {
					q.mu.Unlock()
					break
				}
				request := q.queue[0]
				q.queue = q.queue[1:]
				q.running = true
				q.runningID = request.ID
				q.runningNodeID = request.NodeID
				q.runningKind = request.Kind
				taskCtx, cancel := context.WithCancel(ctx)
				q.runningCancel = cancel
				q.mu.Unlock()
				if request.OnStart != nil {
					request.OnStart()
				}
				_ = request.Run(taskCtx)
				cancel()
				q.mu.Lock()
				q.running = false
				q.runningID = ""
				q.runningNodeID = ""
				q.runningKind = ""
				q.runningCancel = nil
				q.mu.Unlock()
			}
		}
	}
}

func (q *TestQueue) Snapshot() (pending int, running bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.queue), q.running
}

// Current returns the task being executed, if any.
func (q *TestQueue) Current() (id, nodeID, kind string) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if !q.running {
		return "", "", ""
	}
	return q.runningID, q.runningNodeID, q.runningKind
}

// Cancel removes a pending task or cancels the currently running task.
func (q *TestQueue) Cancel(id string) (found, running bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.running && q.runningID == id {
		if q.runningCancel != nil {
			q.runningCancel()
		}
		return true, true
	}
	for i, request := range q.queue {
		if request.ID != id {
			continue
		}
		q.queue = append(q.queue[:i], q.queue[i+1:]...)
		return true, false
	}
	return false, false
}

func (q *TestQueue) WaitUntilIdle(ctx context.Context) bool {
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		pending, running := q.Snapshot()
		if pending == 0 && !running {
			return true
		}
		select {
		case <-ctx.Done():
			return false
		case <-ticker.C:
		}
	}
}
