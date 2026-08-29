package application

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/teddymail/bagualu/internal/domain"
)

type fakeBaseline struct {
	available bool
	errorCode string
}

func (f *fakeBaseline) Check(context.Context) BaselineResult {
	return BaselineResult{Available: f.available, ErrorCode: f.errorCode}
}

type fakeGuard struct{ status LoadStatus }

func (f *fakeGuard) Check(context.Context) LoadResult {
	return LoadResult{Status: f.status}
}

func TestOrchestratorBaselineFailureProducesInfrastructureOutcome(t *testing.T) {
	queue := NewTestQueue(8)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go queue.Run(ctx)

	results := make(chan domain.MeasurementOutcome, 3)
	orchestrator := NewOrchestrator(queue, &fakeBaseline{available: false, errorCode: domain.ErrCodeBaselineUnavailable}, &fakeGuard{status: LoadClean}, func(outcome domain.MeasurementOutcome) {
		results <- outcome
	})

	runCount := 0
	jobRun := func(context.Context) (domain.MeasurementOutcome, error) {
		runCount++
		return domain.MeasurementOutcome{Success: true}, nil
	}
	for _, tc := range []struct {
		id   string
		kind domain.TestKind
	}{{"c", domain.TestConnectivity}, {"p", domain.TestPing}, {"t", domain.TestThroughput}} {
		if err := orchestrator.Submit(TestJob{ID: tc.id, NodeID: "node-1", Kind: tc.kind, Run: jobRun}); err != nil {
			t.Fatalf("Submit(%s) error = %v", tc.kind, err)
		}
	}

	waitForIdle(t, queue)
	close(results)

	if runCount != 0 {
		t.Fatalf("expected jobs not to run, ran %d times", runCount)
	}
	count := 0
	for outcome := range results {
		count++
		if outcome.Success {
			t.Fatalf("expected unsuccessful outcome for %s", outcome.Kind)
		}
		if !outcome.Infrastructure {
			t.Fatalf("expected infrastructure outcome for %s", outcome.Kind)
		}
		if outcome.ErrorCode != domain.ErrCodeBaselineUnavailable {
			t.Fatalf("expected %q, got %q", domain.ErrCodeBaselineUnavailable, outcome.ErrorCode)
		}
	}
	if count != 3 {
		t.Fatalf("expected 3 outcomes, got %d", count)
	}
}

func TestOrchestratorLoadGuardBlocksThroughputOnly(t *testing.T) {
	queue := NewTestQueue(8)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go queue.Run(ctx)

	results := make(chan domain.MeasurementOutcome, 3)
	orchestrator := NewOrchestrator(queue, &fakeBaseline{available: true}, &fakeGuard{status: LoadBusy}, func(outcome domain.MeasurementOutcome) {
		results <- outcome
	})

	if err := orchestrator.Submit(TestJob{ID: "throughput", NodeID: "node-1", Kind: domain.TestThroughput, Run: func(context.Context) (domain.MeasurementOutcome, error) {
		return domain.MeasurementOutcome{Success: true, Evidence: domain.CoreEvidence{NodeName: "node-1", TrafficBefore: 1, TrafficAfter: 2}}, nil
	}}); err != nil {
		t.Fatal(err)
	}
	for _, kind := range []domain.TestKind{domain.TestConnectivity, domain.TestPing} {
		kind := kind
		if err := orchestrator.Submit(TestJob{ID: string(kind), NodeID: "node-1", Kind: kind, Run: func(context.Context) (domain.MeasurementOutcome, error) {
			return domain.MeasurementOutcome{Success: true, Evidence: domain.CoreEvidence{NodeName: "node-1", TrafficBefore: 1, TrafficAfter: 2}}, nil
		}}); err != nil {
			t.Fatal(err)
		}
	}

	waitForIdle(t, queue)
	close(results)

	seen := map[domain.TestKind]domain.MeasurementOutcome{}
	for outcome := range results {
		seen[outcome.Kind] = outcome
	}
	if got := seen[domain.TestThroughput]; !got.Infrastructure || got.ErrorCode != domain.ErrCodeNetworkBusy {
		t.Fatalf("unexpected throughput outcome %+v", got)
	}
	if got := seen[domain.TestConnectivity]; !got.Success || got.Infrastructure {
		t.Fatalf("unexpected connectivity outcome %+v", got)
	}
	if got := seen[domain.TestPing]; !got.Success || got.Infrastructure {
		t.Fatalf("unexpected ping outcome %+v", got)
	}
}

func TestOrchestratorSuccessWithEvidence(t *testing.T) {
	outcome := runSingleJob(t, TestJob{ID: "job-1", NodeID: "node-1", Kind: domain.TestConnectivity, Run: func(context.Context) (domain.MeasurementOutcome, error) {
		return domain.MeasurementOutcome{Success: true, Evidence: domain.CoreEvidence{NodeName: "node-1", TrafficBefore: 1, TrafficAfter: 2}}, nil
	}})
	if !outcome.Success {
		t.Fatalf("expected success, got %+v", outcome)
	}
	if outcome.ErrorCode != "" {
		t.Fatalf("expected empty error code, got %q", outcome.ErrorCode)
	}
}

func TestOrchestratorSuccessWithoutEvidenceBecomesRouteUnverified(t *testing.T) {
	outcome := runSingleJob(t, TestJob{ID: "job-2", NodeID: "node-1", Kind: domain.TestConnectivity, Run: func(context.Context) (domain.MeasurementOutcome, error) {
		return domain.MeasurementOutcome{Success: true}, nil
	}})
	if outcome.Success {
		t.Fatalf("expected failure, got %+v", outcome)
	}
	if outcome.ErrorCode != domain.ErrCodeCoreRouteUnverified {
		t.Fatalf("expected %q, got %q", domain.ErrCodeCoreRouteUnverified, outcome.ErrorCode)
	}
}

func TestOrchestratorThroughputWithoutTrafficGrowthBecomesRouteUnverified(t *testing.T) {
	outcome := runSingleJob(t, TestJob{ID: "job-3", NodeID: "node-1", Kind: domain.TestThroughput, Run: func(context.Context) (domain.MeasurementOutcome, error) {
		return domain.MeasurementOutcome{Success: true, Evidence: domain.CoreEvidence{NodeName: "node-1", TrafficBefore: 5, TrafficAfter: 5}}, nil
	}})
	if outcome.Success {
		t.Fatalf("expected failure, got %+v", outcome)
	}
	if outcome.ErrorCode != domain.ErrCodeCoreRouteUnverified {
		t.Fatalf("expected %q, got %q", domain.ErrCodeCoreRouteUnverified, outcome.ErrorCode)
	}
}

func TestOrchestratorAllKindsShareSameQueue(t *testing.T) {
	queue := NewTestQueue(8)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go queue.Run(ctx)

	results := make(chan domain.MeasurementOutcome, 3)
	orchestrator := NewOrchestrator(queue, &fakeBaseline{available: true}, &fakeGuard{status: LoadClean}, func(outcome domain.MeasurementOutcome) {
		results <- outcome
	})

	var mu sync.Mutex
	active := 0
	maxActive := 0
	for _, kind := range []domain.TestKind{domain.TestConnectivity, domain.TestPing, domain.TestThroughput} {
		kind := kind
		if err := orchestrator.Submit(TestJob{ID: string(kind), NodeID: "node-1", Kind: kind, Run: func(context.Context) (domain.MeasurementOutcome, error) {
			mu.Lock()
			active++
			if active > maxActive {
				maxActive = active
			}
			mu.Unlock()
			time.Sleep(10 * time.Millisecond)
			mu.Lock()
			active--
			mu.Unlock()
			outcome := domain.MeasurementOutcome{Success: true}
			if kind != domain.TestPing {
				outcome.Evidence = domain.CoreEvidence{NodeName: "node-1", TrafficBefore: 1, TrafficAfter: 2}
			}
			return outcome, nil
		}}); err != nil {
			t.Fatal(err)
		}
	}

	waitForIdle(t, queue)
	close(results)

	count := 0
	for range results {
		count++
	}
	if count != 3 {
		t.Fatalf("expected 3 outcomes, got %d", count)
	}
	if maxActive != 1 {
		t.Fatalf("expected serial execution, max active = %d", maxActive)
	}
}

func TestOrchestratorPingBypassesLoadGuard(t *testing.T) {
	outcome := runSingleJobWithGuard(t, &fakeGuard{status: LoadBusy}, TestJob{ID: "job-4", NodeID: "node-1", Kind: domain.TestPing, Run: func(context.Context) (domain.MeasurementOutcome, error) {
		return domain.MeasurementOutcome{Success: true}, nil
	}})
	if !outcome.Success || outcome.Infrastructure {
		t.Fatalf("expected ping to run successfully, got %+v", outcome)
	}
}

func TestOrchestratorQueueFull(t *testing.T) {
	queue := NewTestQueue(1)
	orchestrator := NewOrchestrator(queue, &fakeBaseline{available: true}, &fakeGuard{status: LoadClean}, nil)
	job := TestJob{ID: "job-1", NodeID: "node-1", Kind: domain.TestConnectivity, Run: func(context.Context) (domain.MeasurementOutcome, error) {
		return domain.MeasurementOutcome{}, nil
	}}
	if err := orchestrator.Submit(job); err != nil {
		t.Fatalf("first submit error = %v", err)
	}
	if err := orchestrator.Submit(job); !errors.Is(err, ErrQueueFull) {
		t.Fatalf("expected ErrQueueFull, got %v", err)
	}
}

type sequenceBaseline struct {
	mu      sync.Mutex
	results []BaselineResult
}

func (b *sequenceBaseline) Check(context.Context) BaselineResult {
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(b.results) == 0 {
		return BaselineResult{Available: true}
	}
	result := b.results[0]
	b.results = b.results[1:]
	return result
}

func TestOrchestratorRequiresTwoBaselineSuccessesAfterRecovery(t *testing.T) {
	baseline := &sequenceBaseline{results: []BaselineResult{
		{Available: true}, {Available: false, ErrorCode: domain.ErrCodeBaselineUnavailable},
		{Available: true}, {Available: true},
	}}
	orchestrator := NewOrchestrator(NewTestQueue(2), baseline, &fakeGuard{status: LoadClean}, nil)
	job := TestJob{ID: "recovery", NodeID: "node", Kind: domain.TestPing}
	if blocked := orchestrator.beforeAttempt(context.Background(), job); blocked != nil {
		t.Fatalf("first baseline should pass: %+v", blocked)
	}
	if blocked := orchestrator.beforeAttempt(context.Background(), job); blocked == nil || blocked.ErrorCode != domain.ErrCodeBaselineUnavailable {
		t.Fatalf("expected baseline pause, got %+v", blocked)
	}
	started := time.Now()
	if blocked := orchestrator.beforeAttempt(context.Background(), job); blocked != nil {
		t.Fatalf("baseline should recover after two successes: %+v", blocked)
	}
	if time.Since(started) < time.Second {
		t.Fatalf("recovery did not enforce one-second interval")
	}
}

func runSingleJob(t *testing.T, job TestJob) domain.MeasurementOutcome {
	t.Helper()
	return runSingleJobWithGuard(t, &fakeGuard{status: LoadClean}, job)
}

func runSingleJobWithGuard(t *testing.T, guard LoadGuard, job TestJob) domain.MeasurementOutcome {
	t.Helper()
	queue := NewTestQueue(4)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go queue.Run(ctx)

	results := make(chan domain.MeasurementOutcome, 1)
	orchestrator := NewOrchestrator(queue, &fakeBaseline{available: true}, guard, func(outcome domain.MeasurementOutcome) {
		results <- outcome
	})
	if err := orchestrator.Submit(job); err != nil {
		t.Fatalf("Submit() error = %v", err)
	}
	waitForIdle(t, queue)
	select {
	case outcome := <-results:
		return outcome
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for result")
		return domain.MeasurementOutcome{}
	}
}

func waitForIdle(t *testing.T, queue *TestQueue) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if !queue.WaitUntilIdle(ctx) {
		t.Fatal("queue did not become idle")
	}
}
