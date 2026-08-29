package application

import (
	"context"
	"testing"
	"time"
)

func TestSchedulerRunsUnifiedTestCallback(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	scheduler := Scheduler{
		TestInterval: time.Millisecond,
		Test: func(context.Context, time.Time) error {
			cancel()
			close(done)
			return nil
		},
	}
	go scheduler.Run(ctx)
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("test callback was not scheduled")
	}
}
