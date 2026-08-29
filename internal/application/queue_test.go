package application

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestTestQueueRunsOneTaskAtATime(t *testing.T) {
	queue := NewTestQueue(4)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var mu sync.Mutex
	order := []string{}
	active, maxActive := 0, 0
	for _, id := range []string{"a", "b", "c"} {
		id := id
		if err := queue.Enqueue(TestRequest{ID: id, Run: func(context.Context) error {
			mu.Lock()
			active++
			if active > maxActive {
				maxActive = active
			}
			order = append(order, id)
			mu.Unlock()
			time.Sleep(time.Millisecond)
			mu.Lock()
			active--
			mu.Unlock()
			return nil
		}}); err != nil {
			t.Fatal(err)
		}
	}
	go queue.Run(ctx)
	wait, cancelWait := context.WithTimeout(context.Background(), time.Second)
	defer cancelWait()
	if !queue.WaitUntilIdle(wait) {
		t.Fatal("queue did not drain")
	}
	if maxActive != 1 || len(order) != 3 {
		t.Fatalf("queue was not serial: active=%d order=%v", maxActive, order)
	}
}

func TestTestQueueCancelsRunningTask(t *testing.T) {
	queue := NewTestQueue(2)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	started := make(chan struct{})
	finished := make(chan struct{})
	if err := queue.Enqueue(TestRequest{ID: "cancel-me", Run: func(ctx context.Context) error {
		close(started)
		<-ctx.Done()
		close(finished)
		return ctx.Err()
	}}); err != nil {
		t.Fatal(err)
	}
	go queue.Run(ctx)
	<-started
	found, running := queue.Cancel("cancel-me")
	if !found || !running {
		t.Fatalf("Cancel() = found=%v running=%v", found, running)
	}
	select {
	case <-finished:
	case <-time.After(time.Second):
		t.Fatal("running task did not stop")
	}
}
