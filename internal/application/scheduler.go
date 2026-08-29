package application

import (
	"context"
	"time"
)

// Scheduler only coordinates timers; callbacks are the same application
// use-cases used by HTTP handlers, so scheduled work cannot bypass policy.
type Scheduler struct {
	RefreshInterval time.Duration
	TestInterval    time.Duration
	CleanupInterval time.Duration
	Refresh         func(context.Context) error
	Test            func(context.Context, time.Time) error
	Cleanup         func(context.Context) error
}

func (s Scheduler) Run(ctx context.Context) {
	refreshTicker, refresh := makeTicker(s.RefreshInterval)
	testTicker, test := makeTicker(s.TestInterval)
	cleanupTicker, cleanup := makeTicker(s.CleanupInterval)
	if refreshTicker != nil {
		defer refreshTicker.Stop()
	}
	if testTicker != nil {
		defer testTicker.Stop()
	}
	if cleanupTicker != nil {
		defer cleanupTicker.Stop()
	}
	for {
		select {
		case <-ctx.Done():
			return
		case <-refresh:
			if s.Refresh != nil {
				_ = s.Refresh(ctx)
			}
		case now := <-test:
			if s.Test != nil {
				_ = s.Test(ctx, now)
			}
		case <-cleanup:
			if s.Cleanup != nil {
				_ = s.Cleanup(ctx)
			}
		}
	}
}

func makeTicker(interval time.Duration) (*time.Ticker, <-chan time.Time) {
	if interval <= 0 {
		return nil, nil
	}
	ticker := time.NewTicker(interval)
	return ticker, ticker.C
}
